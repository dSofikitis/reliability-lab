#!/usr/bin/env bash
# MTTR drill: inject latency, watch the SLO alert fire, watch the
# operator pick a remedy, watch the SLO return to budget. Print the
# elapsed time at each milestone so the "auto-remediation works"
# claim is provable, not aspirational.
#
# All polling has a hard timeout — if the chain breaks anywhere, the
# script reports which step stalled rather than spinning forever.
# The exit code reflects success (0) or the stage that failed (1+),
# so this script is also the smoke test the CI chaos-drill workflow
# wraps around.

set -euo pipefail

NS="${NS:-reliability-lab}"
CHAOS_FILE="${CHAOS_FILE:-chaos/payments-latency.yaml}"
ALERT_NAME="${ALERT_NAME:-PaymentsSLOFastBurn}"
TARGET_DEPLOY="${TARGET_DEPLOY:-payments-svc}"
TIMEOUT_ALERT="${TIMEOUT_ALERT:-300}"     # 5 minutes for burn-rate alert to trip
TIMEOUT_REMEDY="${TIMEOUT_REMEDY:-60}"    # 1 minute for operator to act
TIMEOUT_RECOVER="${TIMEOUT_RECOVER:-300}" # 5 minutes for SLO to return to budget

PROM_URL="${PROM_URL:-http://localhost:31300/api/datasources/proxy/1/api/v1/query}"
# Fall back to in-cluster Prometheus via port-forward if NodePort
# isn't exposing it. Easier than maintaining two parallel paths.
if ! curl -sf --max-time 2 "${PROM_URL}?query=up" >/dev/null; then
  echo "[drill] starting kubectl port-forward to prometheus"
  kubectl -n monitoring port-forward svc/kube-prometheus-prometheus 9090:9090 >/dev/null 2>&1 &
  PF_PID=$!
  trap "kill ${PF_PID} >/dev/null 2>&1 || true" EXIT
  PROM_URL="http://localhost:9090/api/v1/query"
  sleep 2
fi

now() { date +%s; }
elapsed() { echo "$(( $(now) - $1 ))s"; }

prom_query() {
  local q="$1"
  curl -sf --max-time 5 --data-urlencode "query=${q}" "${PROM_URL}" \
    | sed -n 's/.*"value":\[[0-9.]*,"\([^"]*\)".*/\1/p'
}

wait_until() {
  local desc="$1" pred="$2" timeout="$3"
  local start; start=$(now)
  while (( $(now) - start < timeout )); do
    if eval "${pred}"; then return 0; fi
    sleep 5
  done
  echo "[drill] TIMEOUT after ${timeout}s waiting for: ${desc}"
  return 1
}

T0=$(now)

echo ""
echo "===================================================="
echo "  MTTR drill — chaos -> burn -> remedy -> recover"
echo "===================================================="
echo ""
echo "[drill t+0s] applying chaos: ${CHAOS_FILE}"
kubectl apply -f "${CHAOS_FILE}"

echo ""
echo "[drill t+$(elapsed ${T0})] waiting for ${ALERT_NAME} to fire (timeout ${TIMEOUT_ALERT}s)"
wait_until "alert ${ALERT_NAME} firing" \
  "[ \"\$(prom_query \"ALERTS{alertname='${ALERT_NAME}',alertstate='firing'}\")\" = '1' ]" \
  "${TIMEOUT_ALERT}" || { echo "[drill] alert never fired — burn-rate threshold may need tuning"; exit 2; }
T_ALERT=$(now)
echo "[drill t+$(elapsed ${T0})] alert firing (took $(( T_ALERT - T0 ))s from chaos)"

echo ""
echo "[drill t+$(elapsed ${T0})] waiting for operator to apply a remedy (timeout ${TIMEOUT_REMEDY}s)"
wait_until "operator log shows remedy applied" \
  "kubectl -n ${NS} logs deploy/remediation-operator --since=10m 2>/dev/null | grep -q -E 'remedy|circuit-break|rollback|scale-up'" \
  "${TIMEOUT_REMEDY}" || { echo "[drill] operator did not act — check operator logs + AlertManager routing"; exit 3; }
T_REMEDY=$(now)
echo "[drill t+$(elapsed ${T0})] remedy applied (took $(( T_REMEDY - T_ALERT ))s from alert fire)"

echo ""
echo "[drill t+$(elapsed ${T0})] waiting for SLO to return to budget (timeout ${TIMEOUT_RECOVER}s)"
# The burn rate dropping back below 1 is the recovery signal — SLO
# is again being honored at sustainable budget rate.
wait_until "burn rate back under 1" \
  "[ \"\$(prom_query 'slo:payments_burn_rate:1h <bool 1' | head -1)\" = '1' ]" \
  "${TIMEOUT_RECOVER}" || { echo "[drill] SLO did not recover in budget — remedy may not have addressed the cause"; exit 4; }
T_RECOVER=$(now)

echo ""
echo "[drill t+$(elapsed ${T0})] tearing down chaos"
kubectl delete -f "${CHAOS_FILE}" --ignore-not-found

echo ""
echo "===================================================="
echo "  MTTR summary"
echo "===================================================="
printf "  chaos -> alert fire    : %ds\n" "$(( T_ALERT - T0 ))"
printf "  alert -> remedy apply  : %ds\n" "$(( T_REMEDY - T_ALERT ))"
printf "  remedy -> SLO recover  : %ds\n" "$(( T_RECOVER - T_REMEDY ))"
printf "  total MTTR             : %ds\n" "$(( T_RECOVER - T0 ))"
echo ""
echo "[drill] watch the burn curve in Grafana: http://localhost:31300"
