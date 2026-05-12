# Linkerd mesh

`make mesh-install` runs:

```bash
linkerd install --crds | kubectl apply -f -
linkerd install | kubectl apply -f -
linkerd check
linkerd viz install | kubectl apply -f -
```

Then patches the `reliability-lab` namespace with
`linkerd.io/inject=enabled` so every pod gets the proxy sidecar.

[`namespace-patch.yaml`](namespace-patch.yaml) is the kustomize patch
the kind / gke overlays both layer on top of the base namespace once
the control plane is up.

## Why Linkerd, not Istio

- Way smaller resource footprint (15 MB Rust proxy vs 100+ MB Envoy);
  important when the same manifests run on a 16 GB laptop and a
  GKE Autopilot cluster.
- Out-of-the-box mTLS with zero config — every pod in
  `reliability-lab` gets a workload identity, no envoy filter
  gymnastics needed.
- Golden RED metrics (rate / errors / duration) are exposed in the
  `linkerd_*` Prometheus namespace automatically; the SLO recording
  rules in [`../prometheus/rules/`](../prometheus/rules) consume
  `linkerd_request_total` and `linkerd_response_latency_ms_seconds`
  directly — no app-side instrumentation needed for the L7 SLOs.

## What the proxy gets us for free

Every request between orders → payments → inventory carries:

- `request_id` propagated across hops (good for distributed traces).
- An mTLS identity claim (cluster-local CA, automatic rotation).
- Retry budget enforcement (configurable per-`ServiceProfile`).
- Per-route latency histograms scraped by Prometheus.

The chaos experiments in [`../../chaos/`](../../chaos) target the
network layer below the proxy (`NetworkChaos`), so the proxy's
golden metrics keep working — that's how we **see** the latency
injection rather than just observe a service hang.
