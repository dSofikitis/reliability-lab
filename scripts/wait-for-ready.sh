#!/usr/bin/env bash
# Wait for every pod in the application namespace to become Ready.
# Stronger than `kubectl wait pods --all`:
#   - longer default timeout (cold-cache CI runners need more than the
#     240s we used to bake in — image pulls + linkerd identity bootstrap
#     + per-pod sidecar startup compounds), and
#   - on failure, dumps the diagnostic surface needed to attribute the
#     stall in a single CI run (pod statuses, describe events, container
#     logs incl. previous-instance logs, linkerd check, node resource
#     usage). Without this, a single unready pod triggers a rerun-and-
#     guess loop that costs more than the diagnostic noise.
#
# Exit 0 when all pods are Ready; exit 1 (with the diagnostic block
# already printed) when the wait times out.

set -euo pipefail

NS="${NS:-reliability-lab}"
TIMEOUT="${TIMEOUT:-600s}"

echo "[wait] waiting up to ${TIMEOUT} for pods in ns=${NS} to become Ready..."
if kubectl wait --for=condition=Ready pods --all -n "${NS}" --timeout="${TIMEOUT}" 2>&1; then
  echo "[wait] all pods Ready"
  exit 0
fi

# ─────────────────────── diagnostics ───────────────────────
echo ""
echo "============================================================"
echo "  pod readiness failed in ns=${NS} after ${TIMEOUT}"
echo "============================================================"

echo ""
echo "--- pod summary ---"
kubectl get pods -n "${NS}" -o wide || true

echo ""
echo "--- events (recent) ---"
kubectl get events -n "${NS}" --sort-by=.lastTimestamp 2>/dev/null | tail -40 || true

# Identify pods that are not Ready. A pod is Ready iff every container
# reports ready=true; jsonpath here surfaces pods whose conditions
# array doesn't include {type:Ready, status:True}. Falls back to
# "everything not Running" for pods that haven't reached steady state.
NOT_READY=$(kubectl get pods -n "${NS}" \
  -o jsonpath='{range .items[?(@.status.phase!="Running")]}{.metadata.name}{"\n"}{end}' 2>/dev/null)
NOT_READY="${NOT_READY}
$(kubectl get pods -n "${NS}" \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[*].ready}{"\n"}{end}' 2>/dev/null \
  | awk '$2 ~ /false/ {print $1}')"
NOT_READY=$(echo "${NOT_READY}" | sort -u | sed '/^$/d')

if [ -n "${NOT_READY}" ]; then
  echo ""
  echo "--- not-ready pods: describe + logs ---"
  for pod in ${NOT_READY}; do
    echo ""
    echo "==================== pod/${pod} ===================="
    # Tail of describe is enough — it ends with the events that
    # actually explain why scheduling / pulling / probes are stuck.
    kubectl describe pod -n "${NS}" "${pod}" 2>/dev/null | tail -80 || true

    echo ""
    echo "--- logs ---"
    # All containers (regular + init) so a stuck linkerd-proxy native
    # sidecar surfaces as a log block, not a silent unknown.
    CONTAINERS=$(kubectl get pod -n "${NS}" "${pod}" \
      -o jsonpath='{.spec.initContainers[*].name} {.spec.containers[*].name}' 2>/dev/null)
    for c in ${CONTAINERS}; do
      [ -z "${c}" ] && continue
      echo "[${c}] (current)"
      kubectl logs -n "${NS}" "${pod}" -c "${c}" --tail=60 2>&1 | sed 's/^/    /' || true
      # --previous catches CrashLoopBackOff cases where the live
      # container's logs are empty because it's still restarting.
      echo "[${c}] (previous)"
      kubectl logs -n "${NS}" "${pod}" -c "${c}" --tail=60 --previous 2>&1 \
        | grep -v 'previous terminated container' | sed 's/^/    /' || true
    done
  done
fi

echo ""
echo "--- linkerd check (control plane health) ---"
# Sidecar readiness is downstream of identity / destination / policy
# being healthy; if any of them are degraded every meshed pod stalls
# until they recover. linkerd check surfaces that in one go.
if command -v linkerd >/dev/null 2>&1; then
  linkerd check 2>&1 | tail -40 || true
else
  echo "linkerd CLI not on PATH; skipping"
fi

echo ""
echo "--- node resource pressure ---"
kubectl top nodes 2>&1 | head -10 || echo "metrics-server unavailable"

echo ""
echo "============================================================"
echo "  end diagnostics — exiting 1"
echo "============================================================"
exit 1
