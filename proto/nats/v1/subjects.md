# NATS subject contracts

The email-worker doesn't speak gRPC — it's a JetStream consumer. The
subject schema is hand-written rather than proto-generated because
the payload is small, the consumer is the only reader, and the
ordering/retention guarantees come from JetStream stream config
rather than the wire format. The schema lives next to the proto files
so all four services' contracts are reviewed together.

## Stream

```
Name:        ORDER_EVENTS
Subjects:    orders.events.>
Retention:   limits  (max-age 24h, max-msgs 1_000_000)
Discard:     old
Storage:     file
Replicas:    1 (kind), 3 (gke overlay)
```

## Subjects

### `orders.events.created`

Published by **orders-svc** after an order is fully authorized (charge
+ reservation both succeeded).

Payload (JSON):

```json
{
  "order_id": "0193fde0-...",
  "customer_id": "cust_42",
  "customer_email": "alice@example.com",
  "amount_minor": 4999,
  "currency": "USD",
  "items": [{ "sku": "WIDGET-7", "quantity": 2 }],
  "created_at": "2026-05-08T10:00:00Z"
}
```

The chaos experiment `chaos/email-worker-oom.yaml` slows the consumer
to a crawl while orders-svc keeps publishing at full rate. The
backlog grows, memory pressure mounts on the worker, the OOM-killer
fires, and the SLO `email_delivery_within_60s` breaks. The
remediation-operator's circuit-break remedy writes
`{publish_enabled: false}` to `orders-svc`'s circuit-break ConfigMap;
orders-svc watches the ConfigMap and pauses publishing until the
backlog drains.

### `orders.events.refunded` (placeholder)

Reserved for a future refund flow. Not in scope for the MTTR demo.

## Consumer config

```
Name:        email-worker
Filter:      orders.events.created
Deliver:     pull
AckPolicy:   explicit
AckWait:     30s
MaxAckPending: 100   (kind), 1000 (gke)
```

`MaxAckPending` is the load-bearing knob the operator's HPA-scale
remedy works against: more replicas means more in-flight messages
the stream allows the consumer group, which is how scale-up actually
drains a backlog.
