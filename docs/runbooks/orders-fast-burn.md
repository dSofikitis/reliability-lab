# Alert: OrdersSLOFastBurn

The orders-svc availability SLO is burning at >14.4× budget over a
1-hour window AND >6× over a 6-hour window. At this rate the 30-day
budget clears in ~50 hours.

**Auto-remedy:** [rollback](rollback.md) — operator aborts the in-flight
canary on the orders-svc Argo Rollout, returning traffic to the
previous stable.

If the rollback is a no-op (no canary in flight), the cause is
upstream — the next alert (`PaymentsSLOFastBurn` or
`InventorySLOFastBurn`) will route to its own remedy. Sit tight 1-2
minutes before doing anything by hand.
