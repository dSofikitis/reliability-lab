# Chaos experiments

Every experiment in this directory is a `chaos-mesh.org/v1alpha1`
CRD. Apply with `make chaos-run EXP=<name>` (drops the `.yaml`),
clean up with `kubectl delete -f chaos/<name>.yaml`.

| Experiment | Targets | Trips the alert | Operator picks |
|---|---|---|---|
| [`payments-latency.yaml`](payments-latency.yaml) | payments-svc (network delay 500ms × 50%) | `OrdersSLOFastBurn` | **rollback** (Argo Rollouts undo of payments-svc) |
| [`inventory-network-drop.yaml`](inventory-network-drop.yaml) | payments→inventory link (20% packet loss) | `InventorySLOFastBurn` + `PaymentsSLOFastBurn` | **scale-up** (HPA patch on payments-svc) |
| [`email-worker-oom.yaml`](email-worker-oom.yaml) | email-worker (CPU stress until OOM) | `EmailWorkerBacklogGrowing` + `EmailWorkerOOMRestart` | **circuit-break** (ConfigMap flag on orders-svc) |
| [`payments-pod-kill.yaml`](payments-pod-kill.yaml) | random payments-svc pod every 30s | *none expected* | *(none — Linkerd retries absorb the churn)* |

The mapping between alerts and remedies is the operator's
classification table — see [`../operator/internal/classify/`](../operator/internal/classify/).

## Running the MTTR drill

```bash
make mttr-drill        # combines all three remediable experiments into a
                       # narrated end-to-end demo
```

The drill applies each experiment in turn, waits for the alert to
fire, verifies the operator picked the right remedy, waits for the
SLO to recover, then moves on. Total runtime: ~10 minutes. CI
asserts MTTR per experiment.

## Writing a new experiment

1. Pick the chaos-mesh CRD that fits the failure shape
   ([reference](https://chaos-mesh.org/docs/simulate-pod-chaos-on-kubernetes/)).
2. Keep the `duration:` short enough that an accidentally-applied
   experiment self-cleans (≤ 10 min).
3. Add an entry to the table above, including which alert it
   should trip and which remedy the operator should pick.
4. If the operator should ignore it (like `payments-pod-kill`), say
   so explicitly in the table — the absence of a remedy is a
   testable assertion too.
