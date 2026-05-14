#!/usr/bin/env bash
# MTTR drill: inject real failures, watch the SLO alert fire, watch
# the operator pick a remedy, verify the remedy landed in cluster
# state. Print elapsed time at each milestone so the "auto-remediation
# works" claim is provable, not aspirational.
#
# Each polling step has a hard timeout — if the chain breaks anywhere,
# the script reports which stage stalled and exits with a stage-coded
# status (1=preflight, 2=alert, 3=remedy log, 4=remedy state) so the
# CI smoke job can attribute failures.
#
# Defaults are tuned for the chaos-drill CI workflow but every key
# parameter is env-overridable so the same script drives a local demo
# (different chaos file, different alert, looser timeouts).

set -euo pipefail

NS="${NS:-reliability-lab}"
CHAOS_FILE="${CHAOS_FILE:-chaos/drill-burn.yaml}"
ALERT_NAME="${ALERT_NAME:-OrdersSLOFastBurn}"
# Which kube object the remedy mutates, so we can verify the change
# landed in cluster state rather than just trust the operator's log.
# Defaults match what classifier.go routes OrdersSLOFastBurn to:
# the rollback remedy, which patches the orders-svc Rollout's
# status.abort field.
REMEDY_KIND="${REMEDY_KIND:-rollout}"        # rollout | hpa | configmap
REMEDY_TARGET="${REMEDY_TARGET:-orders-svc}"
TIMEOUT_PREFLIGHT="${TIMEOUT_PREFLIGHT:-60}"
TIMEOUT_ALERT="${TIMEOUT_ALERT:-360}"        # for: 2m + scrape/eval + chaos inject lag
TIMEOUT_REMEDY_LOG="${TIMEOUT_REMEDY_LOG:-60}"
TIMEOUT_REMEDY_STATE="${TIMEOUT_REMEDY_STATE:-30}"

# ─────────────────────────── prometheus ──────────────────────────
# Find the Prometheus svc by label rather than by chart-mangled name
# — kube-prometheus-stack's service name has changed across releases
# and pinning a literal name ties the script to a specific chart
# version. Label selector is the stable contract.
PROM_NS="${PROM_NS:-monitoring}"
PROM_SVC="$(kubectl -n "${PROM_NS}" get svc \
  -l app.kubernetes.io/name=prometheus \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [ -z "${PROM_SVC}" ]; then
  PROM_SVC="$(kubectl -n "${PROM_NS}" get svc \
    -l operated-prometheus=true \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
fi
if [ -z "${PROM_SVC}" ]; then
  echo "[drill] could not locate prometheus service in ns=${PROM_NS}" >&2
  exit 1
fi
echo "[drill] using prometheus service: ${PROM_NS}/${PROM_SVC}"

PF_PORT=9090
kubectl -n "${PROM_NS}" port-forward "svc/${PROM_SVC}" ${PF_PORT}:9090 >/dev/null 2>&1 &
PF_PID=$!
trap 'kill ${PF_PID} >/dev/null 2>&1 || true' EXIT
PROM_URL="http://localhost:${PF_PORT}/api/v1/query"
# Wait for the port-forward to actually accept connections — kicking
# off requests against a not-yet-ready forwarder gives flaky early
# failures that look like Prom is down when really we just raced.
for _ in $(seq 1 20); do
  curl -sf --max-time 2 "${PROM_URL}?query=up" >/dev/null 2>&1 && break
  sleep 0.5
done

# ─────────────────────────── helpers ─────────────────────────────
now() { date +%s; }
elapsed() { echo "$(( $(now) - $1 ))s"; }

prom_query() {
  local q="$1"
  curl -sf --max-time 5 --data-urlencode "query=${q}" "${PROM_URL}" \
    | sed -n 's/.*"value":\[[0-9.]*,"\([^"]*\)".*/\1/p' | head -1
}

wait_until() {
  local desc="$1" pred="$2" timeout="$3"
  local start; start=$(now)
  while (( $(now) - start < timeout )); do
    if eval "${pred}"; then return 0; fi
    sleep 5
  done
  echo "[drill] TIMEOUT after ${timeout}s waiting for: ${desc}" >&2
  return 1
}

# Kube-state checks for each remedy. Returns 0 if the remedy's
# expected mutation is visible on the target object.
remedy_landed() {
  case "${REMEDY_KIND}" in
    rollout)
      # Rollback remedy patches status.abort=true on the Rollout.
      [ "$(kubectl -n "${NS}" get rollout "${REMEDY_TARGET}" \
        -o jsonpath='{.status.abort}' 2>/dev/null)" = "true" ]
      ;;
    hpa)
      # Scale-up remedy bumps minReplicas above its declared baseline.
      # Baseline read at preflight time and stashed in BASELINE_MIN.
      local cur
      cur="$(kubectl -n "${NS}" get hpa "${REMEDY_TARGET}" \
        -o jsonpath='{.spec.minReplicas}' 2>/dev/null)"
      [ -n "${cur}" ] && [ "${cur}" -gt "${BASELINE_MIN:-0}" ]
      ;;
    configmap)
      # Circuit-break remedy writes publish_enabled=false.
      [ "$(kubectl -n "${NS}" get configmap "${REMEDY_TARGET}" \
        -o jsonpath='{.data.publish_enabled}' 2>/dev/null)" = "false" ]
      ;;
    *)
      echo "[drill] unknown REMEDY_KIND=${REMEDY_KIND}" >&2
      return 2
      ;;
  esac
}

# ─────────────────────────── preflight ───────────────────────────
# Confirm the load actually reaches the backend. Without this, a
# silent k6 misconfig (wrong BASE_URL, missing NodePort) translates
# into "alert never fired" 5 minutes later, which is hours of CI to
# diagnose. Prometheus telling us linkerd_request_total > 0 is the
# load-bearing signal that everything upstream of the SLO actually
# works.
echo ""
echo "===================================================="
echo "  preflight"
echo "===================================================="
wait_until "linkerd_request_total registers traffic on orders-svc" \
  "[ -n \"\$(prom_query 'sum(rate(linkerd_request_total{deployment=\"orders-svc\",direction=\"inbound\"}[1m]))')\" ] && \
   awk \"BEGIN{exit !(\$(prom_query 'sum(rate(linkerd_request_total{deployment=\\\"orders-svc\\\",direction=\\\"inbound\\\"}[1m]))') > 0)}\"" \
  "${TIMEOUT_PREFLIGHT}" || {
    echo "[drill] preflight failed: no traffic on orders-svc. Is k6 running and pointed at the NodePort?" >&2
    exit 1
  }
echo "[drill] preflight OK — orders-svc is receiving traffic"

# Stash the HPA baseline so the remedy_landed check has a "before"
# value to compare against.
if [ "${REMEDY_KIND}" = "hpa" ]; then
  BASELINE_MIN="$(kubectl -n "${NS}" get hpa "${REMEDY_TARGET}" \
    -o jsonpath='{.spec.minReplicas}' 2>/dev/null || echo 0)"
  echo "[drill] hpa baseline minReplicas: ${BASELINE_MIN}"
fi

# ─────────────────────────── drill ───────────────────────────────
T0=$(now)

echo ""
echo "===================================================="
echo "  MTTR drill — chaos -> burn -> remedy applied"
echo "===================================================="
echo ""
echo "[drill t+0s] applying chaos: ${CHAOS_FILE}"
kubectl apply -f "${CHAOS_FILE}"

echo ""
echo "[drill t+$(elapsed ${T0})] waiting for ${ALERT_NAME} to fire (timeout ${TIMEOUT_ALERT}s)"
wait_until "alert ${ALERT_NAME} firing" \
  "[ \"\$(prom_query \"ALERTS{alertname='${ALERT_NAME}',alertstate='firing'}\")\" = '1' ]" \
  "${TIMEOUT_ALERT}" || { kubectl delete -f "${CHAOS_FILE}" --ignore-not-found; exit 2; }
T_ALERT=$(now)
echo "[drill t+$(elapsed ${T0})] alert firing (took $(( T_ALERT - T0 ))s from chaos)"

echo ""
echo "[drill t+$(elapsed ${T0})] waiting for operator to log a remedy (timeout ${TIMEOUT_REMEDY_LOG}s)"
# Operator's logr/zap output is JSON; the remedy keywords appear in
# the structured msg field, so a plain grep against the raw stdout
# matches reliably regardless of formatter changes.
wait_until "operator log mentions a remedy" \
  "kubectl -n ${NS} logs deploy/remediation-operator --since=10m 2>/dev/null | grep -q -E 'remedy|circuit-break|rollback|scale-up'" \
  "${TIMEOUT_REMEDY_LOG}" || { kubectl delete -f "${CHAOS_FILE}" --ignore-not-found; exit 3; }
T_REMEDY_LOG=$(now)
echo "[drill t+$(elapsed ${T0})] remedy logged (took $(( T_REMEDY_LOG - T_ALERT ))s from alert)"

echo ""
echo "[drill t+$(elapsed ${T0})] verifying remedy landed in cluster state (${REMEDY_KIND}/${REMEDY_TARGET}, timeout ${TIMEOUT_REMEDY_STATE}s)"
# Verify the remedy landed in cluster state — the operator could log
# success and still hit a kube API error, so cluster state is the
# only honest "did it actually work" signal. Replaces the previous
# SLO-recovery wait, which couldn't succeed in reasonable time
# because rate(...[1h]) doesn't drop below threshold for ~30 min
# even after chaos stops (rolling-window math, not a remedy bug).
wait_until "${REMEDY_KIND}/${REMEDY_TARGET} reflects the remedy" \
  "remedy_landed" \
  "${TIMEOUT_REMEDY_STATE}" || { kubectl delete -f "${CHAOS_FILE}" --ignore-not-found; exit 4; }
T_STATE=$(now)
echo "[drill t+$(elapsed ${T0})] remedy verified in cluster state (took $(( T_STATE - T_REMEDY_LOG ))s)"

echo ""
echo "[drill t+$(elapsed ${T0})] tearing down chaos"
kubectl delete -f "${CHAOS_FILE}" --ignore-not-found

echo ""
echo "===================================================="
echo "  MTTR summary"
echo "===================================================="
printf "  chaos -> alert fire    : %ds\n" "$(( T_ALERT - T0 ))"
printf "  alert -> remedy logged : %ds\n" "$(( T_REMEDY_LOG - T_ALERT ))"
printf "  log   -> state visible : %ds\n" "$(( T_STATE - T_REMEDY_LOG ))"
printf "  total auto-remediation : %ds\n" "$(( T_STATE - T0 ))"
echo ""
echo "[drill] note: full SLO recovery happens on the rolling-window's"
echo "        own schedule (~30+ min for a 1h burn-rate window to drain)"
echo "        — see docs/runbooks/ for the manual SLO observation steps."
