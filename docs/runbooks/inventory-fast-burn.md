# Alert: InventorySLOFastBurn

inventory-svc availability SLO is burning fast. Most often the
direct cause of `PaymentsSLOFastBurn` (payments retries against a
flaky inventory).

**Auto-remedy:** [scale-up](scale.md) — operator bumps the
`inventory-svc` HPA's `minReplicas` by 2, capped at maxReplicas (8).

If both `InventorySLOFastBurn` and `PaymentsSLOFastBurn` fire (likely),
both scale-up remedies run in parallel; that's the right behavior —
absorbing load on both sides of the dependency edge is faster than
either alone.

## When scale isn't the right answer

Inventory's failure mode in this lab is a network drop (chaos
experiment `inventory-network-drop.yaml`), not a load problem. The
scale-up remedy doesn't fix the network — it gives the system more
parallel paths through the bad link, which statistically helps a
percentage-based packet drop. If the underlying cause is a hard
network partition rather than a partial drop, scaling won't help;
investigate the network policy / chaos experiments / mesh config.
