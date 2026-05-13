#!/usr/bin/env bash
# Tail Prometheus for the SLO recording rule values. Refreshes every
# REFRESH seconds and prints a one-screen view of every SLO's
# success ratio + burn rate. Intended for the second terminal during
# `make demo` so you can watch the burn curve flatten in real time
# while the chaos drill runs in the first.
#
# Reads from the kube-prometheus-stack via NodePort:31300 (the
# Grafana datasource proxy) by default, with a port-forward fallback
# to the in-cluster Prometheus svc.

set -euo pipefail

REFRESH="${REFRESH:-5}"
PROM_URL="${PROM_URL:-http://localhost:31300/api/datasources/proxy/1/api/v1/query}"
if ! curl -sf --max-time 2 "${PROM_URL}?query=up" >/dev/null; then
  echo "[slo-check] starting kubectl port-forward to prometheus"
  kubectl -n monitoring port-forward svc/kube-prometheus-prometheus 9090:9090 >/dev/null 2>&1 &
  PF_PID=$!
  trap "kill ${PF_PID} >/dev/null 2>&1 || true" EXIT
  PROM_URL="http://localhost:9090/api/v1/query"
  sleep 2
fi

q() {
  local query="$1"
  curl -sf --max-time 5 --data-urlencode "query=${query}" "${PROM_URL}" \
    | sed -n 's/.*"value":\[[0-9.]*,"\([^"]*\)".*/\1/p' | head -1
}

fmt() {
  # 0.9942... -> 99.42%
  awk -v v="$1" 'BEGIN { if (v == "") printf "  n/a "; else printf "%5.2f%%", v*100 }'
}

fmt_burn() {
  # raw float -> "0.42" or "5.30x" (>= 1 = burning)
  awk -v v="$1" 'BEGIN {
    if (v == "") printf "  n/a "
    else if (v >= 1) printf "%4.2fx", v
    else printf "%5.2f", v
  }'
}

while true; do
  clear
  printf "Reliability Lab — SLO live view  (refresh %ds, ^C to quit)\n\n" "${REFRESH}"
  printf "  %-12s  %-12s  %-12s  %-12s\n" "service" "success(1h)" "burn(1h)" "burn(6h)"
  printf "  %-12s  %-12s  %-12s  %-12s\n" "────────────" "────────────" "────────────" "────────────"
  for svc in orders payments inventory; do
    sr=$(q "slo:${svc}_success_ratio:rate1h")
    b1=$(q "slo:${svc}_burn_rate:1h")
    b6=$(q "slo:${svc}_burn_rate:6h")
    printf "  %-12s  %-12s  %-12s  %-12s\n" "${svc}" "$(fmt "${sr}")" "$(fmt_burn "${b1}")" "$(fmt_burn "${b6}")"
  done
  echo ""
  pending=$(q 'nats_consumer_num_pending{consumer_name="email-worker"}')
  printf "  email-worker pending : %s msg\n" "${pending:-n/a}"
  echo ""
  echo "  burn rates >= 1.0x are burning the budget. >= 14.4x trips fast-burn pages."
  sleep "${REFRESH}"
done
