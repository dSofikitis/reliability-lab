# Alert: PaymentsSLOSlowBurn

payments-svc has consumed 5% of its 30-day budget over a 6-hour
window. Slow drift, not a sudden failure.

**Auto-remedy:** none. Slow burn = human investigation.

## Investigation checklist

1. Same as [orders-slow-burn.md](orders-slow-burn.md) but on payments-svc.
2. Check inventory: a low-grade flake on inventory-svc that doesn't
   trip its own fast-burn can still drag payments above its tighter
   99.9% bar.
3. Capacity creep: payments-svc HPA `currentReplicas` against
   `desiredReplicas` over 24h; rising baseline means real load growth
   that needs a higher `maxReplicas` ceiling, not just per-incident
   coercion.
