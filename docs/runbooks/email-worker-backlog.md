# Alert: EmailWorkerBacklogGrowing

`nats_consumer_num_pending{consumer_name="email-worker"} > 500` and
the derivative is positive — the queue is growing, not just deep.
By Little's law this means average wait > 60s at the current ack
rate, breaking the email_delivery_within_60s SLO.

**Auto-remedy:** [circuit-break](circuit-break.md) — operator writes
`publish_enabled=false` into the `orders-svc-flags` ConfigMap.
orders-svc stops publishing new events; the worker drains the
backlog at its actual rate; SLO returns to budget.

The brake stays engaged until lifted by hand or by a follow-up
process. See [circuit-break.md](circuit-break.md#how-to-lift-the-brake).
