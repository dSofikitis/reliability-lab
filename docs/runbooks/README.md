# Runbooks

Each Prometheus alert in [`k8s/prometheus/rules/`](../../k8s/prometheus/rules)
carries a `runbook:` annotation that points at one file in this
directory. Two layers:

1. **Per-alert pages** — `orders-fast-burn.md`, `payments-slow-burn.md`,
   etc. Short. Explain what the alert means and which remedy (if any)
   the operator dispatches automatically. Link to the remedy page for
   the rest.
2. **Per-remedy pages** — `rollback.md`, `scale.md`, `circuit-break.md`.
   Long-form. Explain what the operator does, why, what to check
   while it's running, and how to roll the remedy back if the operator
   guessed wrong.

The split exists because most of the depth is in the remedy, not the
alert. If you're paged for the orders fast-burn, you don't need three
separate pages explaining "the operator will roll back" — one
rollback runbook covers every alert that triggers it.

## Map

| Alert                          | Auto-remedy   | Runbook |
|---                             |---            |---      |
| `OrdersSLOFastBurn`            | rollback      | [orders-fast-burn.md](orders-fast-burn.md) |
| `OrdersSLOSlowBurn`            | human         | [orders-slow-burn.md](orders-slow-burn.md) |
| `OrdersLatencyHigh`            | rollback      | [orders-latency.md](orders-latency.md) |
| `PaymentsSLOFastBurn`          | scale         | [payments-fast-burn.md](payments-fast-burn.md) |
| `PaymentsSLOSlowBurn`          | human         | [payments-slow-burn.md](payments-slow-burn.md) |
| `InventorySLOFastBurn`         | scale         | [inventory-fast-burn.md](inventory-fast-burn.md) |
| `EmailWorkerBacklogGrowing`    | circuit-break | [email-worker-backlog.md](email-worker-backlog.md) |
| `EmailWorkerOOMRestart`        | circuit-break | [email-worker-oom.md](email-worker-oom.md) |

Slow-burn alerts route to a human on purpose. They give us a few
hours of runway and the underlying drift usually deserves
investigation, not auto-remediation.
