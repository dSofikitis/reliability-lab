# loadgen / k6

The k6 script that drives orders-svc with the traffic shape the SLOs
assume. The MTTR drill expects a sustained load to be running in the
background so chaos has something to perturb — without it, an empty
service trivially satisfies its SLO and the demo is pointless.

## Run against `make demo` (kind, NodePort 31080)

```bash
k6 run loadgen/k6/orders.js
```

## Tune

```bash
TARGET_VUS=50 HOLD_SEC=900 k6 run loadgen/k6/orders.js
BASE_URL=http://orders-svc.reliability-lab.svc.cluster.local:8080 k6 run ...
```

## Why these thresholds

The k6 thresholds mirror the orders-svc SLO (99.5% protocol success,
p99 < 300ms). A clean run with no chaos means the SLO is provably
honored for the duration of the test. A failed run with no chaos
means the service itself regressed — so the script doubles as a
service-level regression check, not just a chaos-demo input.

402 (declined) counts as protocol success because payments rejection
is a business outcome, not a service availability hit. This matches
how Linkerd's `classification` label categorizes responses (4xx is
not a server error), so the k6 view and the SLO recording rules
agree on what "success" means.
