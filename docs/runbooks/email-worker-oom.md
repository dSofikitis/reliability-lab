# Alert: EmailWorkerOOMRestart

email-worker has restarted more than once in the last 10 minutes.
Almost certainly the OOM-killer — the worker's memory limit is set
intentionally tight (128Mi in the kind overlay) so the OOM chaos
experiment has a real boundary to push against.

**Auto-remedy:** [circuit-break](circuit-break.md) — same brake as
`EmailWorkerBacklogGrowing`. Pausing the publisher stops the queue
from growing; the worker stops being load-pressured into restart
loops.

If the brake is engaged and the worker still OOMs, the queue
already in NATS is too deep — the worker is being overwhelmed by
the existing backlog, not new arrivals. See
[circuit-break.md → "When the brake isn't enough"](circuit-break.md#when-the-brake-isnt-enough).
