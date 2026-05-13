# Remedy: rollback

**Triggered by:** `OrdersSLOFastBurn`, `OrdersLatencyHigh`.
**Code:** [`operator/internal/remedy/rollback.go`](../../operator/internal/remedy/rollback.go).

## What the operator does

Patches `status.abort=true` on the named Argo Rollout (default
`orders-svc`). The argo-rollouts controller reacts by scaling the
canary ReplicaSet to zero and returning 100% of traffic to the
previous stable. From the operator's perspective the action is one
PATCH; the actual rollback is finished a few seconds later when the
controller reconciles.

If no canary is in progress (rollout is at 100% stable already), the
abort is a no-op — the controller has nothing to scale down. The
operator logs and moves on; this is the case where a human takes over.

## Why this remedy for these alerts

`OrdersSLOFastBurn` and `OrdersLatencyHigh` both fire when the
orders-svc inbound success ratio (or p99 latency) regresses fast.
The most common cause is the most-recently-rolled-out change: a new
canary that's worse than what it's replacing. Rolling that canary
back is the cheapest, fastest way to clear the burn.

If the cause is upstream (payments-svc latency or inventory retry
storm), the rollback is a no-op (orders' own template hasn't
changed) and the next alert (payments fast-burn or inventory
fast-burn) routes to its scale-up remedy. Either way the outage
moves toward resolution.

## What to check while the rollback runs

- `kubectl argo rollouts get rollout orders-svc -n reliability-lab`
  — should show `Status: Degraded -> Healthy` within ~30s as the
  canary scales to zero.
- Grafana → orders dashboard → success ratio panel should turn
  upward within ~1m as the burn rate clears.
- AlertManager → the firing alert should resolve within the
  alert's `for: 2m` window once burn drops below threshold.

## How to roll the rollback back

If the rollback was the wrong call (the canary was good, the cause
was elsewhere), promote the next deploy:

```bash
kubectl argo rollouts promote orders-svc -n reliability-lab
```

The operator's dedupe (10-minute cooldown) prevents it from
re-aborting the same rollout — you have a clear window to ship
forward without fighting the operator.

## Disabling the remedy

If the rollback remedy itself is misbehaving and you want to
disengage it without taking the operator down:

```bash
# Pause AlertManager from forwarding rollback-eligible alerts:
kubectl -n monitoring exec sts/alertmanager-kube-prometheus-alertmanager -- \
  amtool silence add alertname=OrdersSLOFastBurn --duration=2h
```

The other remedies stay live. Lift the silence with `amtool silence expire`.
