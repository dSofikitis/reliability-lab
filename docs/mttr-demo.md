# The MTTR demo

> Lands in phase 13 — the scripted chaos drill that walks
> chaos → SLO burn → operator remedy → SLO recovery end-to-end,
> with each step printed live and Grafana panels timestamped.

The shape, in order:

1. **t=0**: k6 is already driving steady-state load against
   `POST /orders`. Grafana shows the orders SLO sitting at ~99.7%
   (well inside the 99.5% target), 0% burn rate, full budget.
2. **t=0+5s**: `chaos/payments-latency.yaml` is applied. Half of
   payments-svc traffic gets 500 ms of injected latency. The orders
   p99 climbs above 300 ms within ~15 s.
3. **t=0+30s**: The fast-burn alert fires (`orders-error-budget-fast`
   recording rule crosses 14.4× burn over the 1h window).
   AlertManager POSTs the alert payload to the remediation-operator's
   webhook.
4. **t=0+45s**: The operator classifies the alert (`reason=slow
   upstream`, `service=orders`, `dependency=payments`) and selects
   the **rollback** remedy. It patches the `payments` Argo Rollouts
   resource with an `abort: true` annotation, which rolls payments
   back to the previous stable revision (the one without the
   regression that introduced the high timeout).
5. **t=0+90s**: New payments pods come up, the chaos experiment is
   still in flight but the rolled-back version short-circuits the
   downstream path that was sensitive to it. p99 drops back under
   300 ms.
6. **t=0+120s**: The fast-burn alert clears. Slow-burn alert never
   fires. Grafana shows the burn curve flatten. SLO recovers; the
   budget has spent ~12 minutes of the 30-day allowance.

CI asserts the MTTR with `mttr_drill_total_seconds < 180`. If the
operator picks the wrong remedy or the rollback is slow, the build
fails and the drill log gets uploaded as an artifact.

The two **other** remedies have their own demos:

- `chaos/inventory-retry-storm.yaml` → operator picks **HPA scale-up**
  (retry storms are absorbed by more replicas, not by rollback).
- `chaos/email-worker-oom.yaml` → operator picks **ConfigMap
  circuit-break** (orders-svc stops emitting new mail jobs to NATS
  until the worker drains).
