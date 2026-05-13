# Alert: OrdersSLOSlowBurn

The orders-svc availability SLO has consumed 5% of its 30-day budget
in a 6-hour window. Slow regression — there's runway, but something
real is drifting.

**Auto-remedy:** none. The operator is configured to leave slow burns
for human investigation. Slow drift usually means a config change, a
dependency change, or a gradual capacity issue — all things worth
diagnosing rather than auto-remediating.

## Investigation checklist

1. Recent deploys: `kubectl argo rollouts history rollout orders-svc -n reliability-lab`.
2. Recent changes to upstream services (payments, inventory) — check
   their dashboards for elevated tail latency or error rates.
3. Capacity: `kubectl top pods -n reliability-lab -l app=orders-svc`
   — pod CPU should be well under limit.
4. Mesh-level retries: Linkerd dashboard's traffic-shifting view
   often surfaces a noisy upstream that's only slightly off-budget.
