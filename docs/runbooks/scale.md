# Remedy: scale-up

**Triggered by:** `PaymentsSLOFastBurn`, `InventorySLOFastBurn`.
**Code:** [`operator/internal/remedy/scale.go`](../../operator/internal/remedy/scale.go).

## What the operator does

Reads the target HPA (`payments-svc` or `inventory-svc`), bumps
`spec.minReplicas` by `Step` (default 2), capped at the HPA's
`maxReplicas`. The HPA controller scales the deployment up to the
new floor; load disperses across more pods.

If `current_min` is already at or above `max`, the operator logs a
no-op and returns nil — the brake on growth is real.

## Why minReplicas, not spec.replicas

The HPA actively manages `spec.replicas` based on its own metrics.
A direct write to `spec.replicas` is silently reverted on the HPA's
next reconcile (default 15s). `minReplicas` is the supported coercion
knob: lifting it forces the HPA's floor up; the controller scales the
deployment to honor it; once load returns to normal the HPA *can*
scale back down (it won't, until the human or a follow-up remedy
lowers `minReplicas`, which is the explicit-by-design behavior).

## Why this remedy for these alerts

`PaymentsSLOFastBurn` is canonically the inventory retry storm — a
flaky inventory-svc causes payments-svc to retry, retries pile up
against the slow upstream, the queue per-pod grows, and the SLO
burns. Throwing more payments-svc pods at the queue absorbs it.

`InventorySLOFastBurn` is the same pattern one hop earlier. Same
remedy, different target.

## What to check while the scale-up runs

- `kubectl get hpa -n reliability-lab` — `MINPODS` should reflect
  the bump within seconds.
- `kubectl get pods -n reliability-lab -l app=payments-svc -w`
  (or inventory-svc) — should show new pods spinning up; ready in
  ~10s on a warm node.
- Grafana → service dashboard → request rate and CPU panels should
  show the load redistributing.

## How to roll the scale-up back

Once the SLO is back inside its budget and load has subsided, lower
`minReplicas` back to its baseline:

```bash
kubectl patch hpa payments-svc -n reliability-lab \
  --type=merge -p '{"spec":{"minReplicas":2}}'
```

The HPA will scale the deployment back down to the floor on its next
reconcile. The operator does NOT do this automatically — recovery
recognition is a separate signal from the original alert, and we
don't want to yo-yo against a flapping SLO.

## When the scale-up isn't enough

If `minReplicas == maxReplicas` and the SLO is still burning, the
ceiling is the bottleneck. Options:

1. Raise `maxReplicas` in [`k8s/base/payments-svc.yaml`](../../k8s/base/payments-svc.yaml) and re-apply.
2. Investigate the upstream — if inventory is the actual cause,
   scaling payments only redistributes the same retry storm.
3. Engage the circuit-break remedy on a different surface (drain
   non-critical traffic) while you fix the root cause.
