# Remedy: circuit-break

**Triggered by:** `EmailWorkerBacklogGrowing`, `EmailWorkerOOMRestart`.
**Code:** [`operator/internal/remedy/circuitbreak.go`](../../operator/internal/remedy/circuitbreak.go).

## What the operator does

Writes `publish_enabled=false` into the `orders-svc-flags`
ConfigMap (creates it if missing). orders-svc mounts that ConfigMap
as a volume and `fsnotify`-watches the mount directory; within
~kubelet sync seconds it flips its in-process `publishingPaused`
flag and stops calling `js.Publish` on new orders. The order itself
still gets authorized; only the async email-worker side-effect is
shed.

The email-worker queue stops growing, the worker catches up, the
SLO returns to budget.

## Why pausing the publisher (not the worker)

The worker isn't the failure surface — it's the symptom. Killing it
would lose in-flight messages on its consumer. Pausing the publisher
upstream contains the cause without losing data; queued messages
get drained at the worker's actual rate.

## Why this isn't auto-cleared

The remedy only writes the brake on. Lifting it requires observing
that the SLO has returned to budget — a separate signal from the
original alert. Auto-clearing on a "resolved" webhook would risk
yo-yoing against a flapping SLO; we explicitly leave the lift to
either a human, a follow-up remedy, or a periodic cron.

## What to check while the brake is engaged

- `kubectl get configmap orders-svc-flags -n reliability-lab -o yaml`
  — `publish_enabled` should read `"false"`.
- `kubectl logs -n reliability-lab -l app=orders-svc | grep "circuit-break flag changed"`
  — orders-svc should log the flip within ~60s.
- Grafana → email-worker dashboard → `nats_consumer_num_pending`
  panel should plateau, then drop.
- `kubectl exec -n reliability-lab nats-0 -- nats stream info ORDER_EVENTS`
  — message rate inbound to the stream should drop to zero.

## How to lift the brake

```bash
# Lift the brake (operator's preferred form: delete the key, not set it true):
kubectl patch configmap orders-svc-flags -n reliability-lab \
  --type=json -p='[{"op": "remove", "path": "/data/publish_enabled"}]'
```

Why delete rather than set `true`: the orders-svc watcher treats the
absence of the key as "publishing enabled". Deleting means future
operator stamps don't have to reason about prior state.

After lifting, `kubectl logs -l app=orders-svc` should show
`publish_enabled=true paused=false` within ~60s.

## When the brake isn't enough

If the worker is OOM-killed even with publishing paused, the queue
already in NATS is too deep. Options:

1. Bump `email-worker` memory in [`k8s/base/email-worker.yaml`](../../k8s/base/email-worker.yaml).
2. Drain the existing queue manually:
   ```bash
   kubectl exec -n reliability-lab nats-0 -- \
     nats consumer purge ORDER_EVENTS email-worker
   ```
   (Loses messages — acceptable here because the workload is "send
   email", not "process payment".)
3. Scale up email-worker (bump replicas) so multiple consumers chip
   away at the backlog.
