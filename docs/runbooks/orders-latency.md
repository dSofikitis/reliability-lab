# Alert: OrdersLatencyHigh

orders-svc p99 latency exceeded 300 ms (the SLO threshold) over a
5-minute window. Tracked separately from availability so a latency
regression with no error increase still pages.

**Auto-remedy:** [rollback](rollback.md) — same path as
`OrdersSLOFastBurn`. A latency regression on the canary is the most
common cause; rolling back is cheap and fast.

If the cause is upstream (a slow payments-svc dragging the inbound
p99 up), the rollback is a no-op and the next alert
(`PaymentsSLOFastBurn`) will fire and route to its scale-up remedy.
