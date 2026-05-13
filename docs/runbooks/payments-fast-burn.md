# Alert: PaymentsSLOFastBurn

payments-svc availability SLO is burning fast. With a 99.9% target,
the budget is 43 minutes per 30 days — fast burn here is severe.

**Auto-remedy:** [scale-up](scale.md) — operator bumps the
`payments-svc` HPA's `minReplicas` by 2, capped at maxReplicas (10).

The canonical cause of this alert is the inventory retry storm: a
flaky inventory-svc causes payments-svc to retry, retries pile up
against the slow upstream, the per-pod queue grows, and the SLO
burns. More payments-svc pods absorb the queue.

If `inventory-svc` is the real source, you'll also see
`InventorySLOFastBurn` shortly — that one targets inventory's HPA
directly. Both remedies running in parallel is the correct pattern.
