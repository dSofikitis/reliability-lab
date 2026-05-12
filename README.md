# Reliability Lab

> A self-contained SRE sandbox: a four-service Go order-flow chain
> running on Kubernetes with SLOs encoded as PromQL recording rules,
> chaos-mesh experiments as version-controlled YAML, and a small Go
> operator that watches AlertManager and picks among rollback / scale /
> circuit-break when an error budget burns.

The system under test is deliberately failure-shaped: a slow payments
service cascades into a missed orders SLO, a flaky inventory triggers
retry storms, a backed-up NATS queue eats memory until the email
worker OOMs. The reliability stack — Linkerd mesh, OpenTelemetry,
Prometheus + Grafana, chaos-mesh, Argo Rollouts, a custom controller
— turns those failure modes into something measurable, alertable, and
remediable. Every CI build signs its image with Cosign, scans with
Trivy, attaches SLSA provenance, and a Kyverno admission policy in
the cluster refuses anything unsigned.

Two paths from the same manifests: `make demo` brings up a 3-node
[kind](https://kind.sigs.k8s.io/) cluster on your laptop in ~30s;
`make cloud-up` runs Terraform against a GKE Autopilot module that
builds the real thing and tears down with `make cloud-down` so the
demo costs single-digit dollars.

## What this is for

This repo is explicitly about **operating** a system rather than
building features. The load-bearing showpieces are SLO math, error
budgets, chaos as code, and an auto-remediation control loop;
everything else (k6 load tests, NATS for the async tail, Cosign at
the build edge, GitOps via Argo CD) is supporting cast that ties the
story together. The MTTR demo is the one-liner: a chaos experiment
burns budget, the operator picks the right remedy, the SLO recovers
— shown live in Grafana and scripted in CI as a smoke chaos drill.

| Capability | Where it shows up |
|---|---|
| **Order-flow chain** | [`services/orders-svc`](services/orders-svc), [`payments-svc`](services/payments-svc), [`inventory-svc`](services/inventory-svc) over gRPC; [`email-worker`](services/email-worker) consumes a NATS JetStream queue. Four services that fail in interesting, dependent ways. |
| **Two runtimes** | [`make demo`](Makefile) → kind cluster; [`make cloud-up`](Makefile) → [`terraform/gke-autopilot`](terraform/gke-autopilot). Same Kustomize overlays apply to both. |
| **Service mesh** | [`k8s/mesh/`](k8s/mesh) — Linkerd install + `linkerd.io/inject: enabled` on the app namespace. mTLS + golden RED metrics out of the box, no Istio overhead. |
| **Observability** | OTel SDK in every service → [`k8s/observability/otel-collector.yaml`](k8s/observability) → Prometheus + Grafana with [provisioned dashboards](k8s/observability/grafana-dashboards/) per SLO. |
| **SLOs as code** | [`k8s/prometheus/rules/`](k8s/prometheus/rules) — PromQL recording rules + multi-window multi-burn-rate alerts (the Google SRE-book pattern, not a flat 5%-error-rate threshold). |
| **Chaos as code** | [`chaos/`](chaos) — version-controlled `PodChaos`, `NetworkChaos`, `StressChaos`, `DNSChaos` CRDs alongside the manifests they break. |
| **Auto-remediation** | [`operator/`](operator) — ~500 LOC Go controller-runtime operator. Subscribes to AlertManager webhooks, classifies the burning SLO, picks among **Argo Rollouts rollback**, **HPA scale up**, or **ConfigMap circuit-break entry** the upstream service watches. |
| **Supply chain** | [`.github/workflows/release.yml`](.github/workflows/release.yml) — Cosign keyless OIDC sign, Trivy HIGH+ CVE gate, SLSA provenance attestation attached to the image. |
| **Admission policy** | [`k8s/policy/kyverno-verify-images.yaml`](k8s/policy) — `verifyImages` `ClusterPolicy` that refuses any image not signed by this repo's GitHub Actions OIDC identity. |
| **GitOps** | [`gitops/app-of-apps.yaml`](gitops) — an Argo CD `Application` that fans out to every component in this repo. |
| **Load gen** | [`loadgen/k6/`](loadgen/k6) — scripted traffic that drives the SLO numbers, so chaos has something to perturb. |
| **MTTR drill** | [`.github/workflows/chaos-drill.yml`](.github/workflows/chaos-drill.yml) — chaos → burn → remediate → recover, run on every push. |

## Architecture at a glance

```
   ┌────────────────────────────────────────────────────────────────────┐
   │                      Kubernetes (kind | GKE Autopilot)             │
   │                                                                    │
   │   ┌──────────┐       ┌────────────┐       ┌─────────────┐          │
   │   │  orders  │──gRPC▶│  payments  │──gRPC▶│  inventory  │          │
   │   │   (HTTP) │       │            │       │             │          │
   │   └─────┬────┘       └────────────┘       └─────────────┘          │
   │         │                                                          │
   │         │ NATS JetStream                                           │
   │         ▼                                                          │
   │   ┌──────────────┐         ┌────────────────┐                      │
   │   │ email-worker │         │ Linkerd mesh   │ ◀── mTLS + golden    │
   │   └──────────────┘         │ (sidecar/pod)  │      metrics         │
   │                            └────────────────┘                      │
   │                                                                    │
   │                       OTel SDK on every service                    │
   │                                  │                                 │
   │                    ┌─────────────▼─────────────┐                   │
   │                    │      OTel Collector       │                   │
   │                    └──────────┬────────────────┘                   │
   │                               │ traces + metrics                   │
   │              ┌────────────────▼────────────────┐                   │
   │              │ Prometheus + recording rules    │                   │
   │              │ + multi-window burn-rate alerts │                   │
   │              └────────────────┬────────────────┘                   │
   │                               │ webhook on alert                   │
   │              ┌────────────────▼─────────────────┐                  │
   │              │   remediation-operator (Go)      │                  │
   │              │   controller-runtime, ~500 LOC   │                  │
   │              └──┬──────────────┬─────────────┬──┘                  │
   │                 ▼              ▼             ▼                     │
   │           Argo Rollouts    HPA scale    ConfigMap                  │
   │            rollback        replicas     circuit-break              │
   │                                                                    │
   │   ── chaos-mesh experiments injected as YAML (pod-kill,            │
   │      latency, OOM, network drop, DNS poison) ──                    │
   │                                                                    │
   │   ── Kyverno ClusterPolicy verifies Cosign signatures              │
   │      at admission; unsigned images are rejected ──                 │
   └────────────────────────────────────────────────────────────────────┘
```

See [`docs/runbooks/`](docs/runbooks) for the playbook each remedy
runs, [`docs/slo-math.md`](docs/slo-math.md) for the burn-rate
derivation, and [`docs/mttr-demo.md`](docs/mttr-demo.md) for the
scripted chaos drill.

## Quickstart

```bash
make tools             # installs kind, kubectl, kustomize, helm, linkerd, cosign, trivy, buf
make demo              # spins kind, applies everything, starts k6, opens Grafana
make mttr-drill        # chaos → burn → remediate → recover, ~3 min, narrates each step
make kind-down         # tear down
```

For the cloud path:

```bash
make cloud-up          # terraform apply against GKE Autopilot (~10 min)
make demo CLUSTER=gke  # same manifests, real cluster
make cloud-down        # terraform destroy
```

## Configuration

The whole stack reads from env vars and Kustomize patches. The
defaults are tuned so `make demo` works out of the box on a 16 GB
laptop; production-ish values live in the `overlays/gke` overlay.

| Var | Default | What |
|---|---|---|
| `CLUSTER` | `kind` | Selects the Kustomize overlay: `kind` or `gke`. |
| `IMAGE_REGISTRY` | `ghcr.io/dsofikitis/reliability-lab` | Image registry for built containers. |
| `IMAGE_TAG` | `dev` | Tag applied to all four service images. |
| `GRAFANA_ADMIN_PASSWORD` | `admin` | Local-only default; overridden by a sealed-secret in `gke`. |
| `COSIGN_EXPERIMENTAL` | `1` | Required for keyless OIDC signing in CI. |

The full env reference lives in [`docs/configuration.md`](docs/configuration.md).

## Development

Polyglot monorepo. Each component builds and tests independently.

| Component | Toolchain | Build | Test |
|---|---|---|---|
| `services/*` | Go 1.23 | `go build ./...` | `go test ./...` |
| `operator/` | Go 1.23 + controller-runtime | `make operator-build` | `make operator-test` (envtest) |
| `proto/` | buf + protoc-gen-go-grpc | `make proto` | n/a |
| `terraform/` | Terraform 1.7 | `terraform plan` | `terraform validate` |
| `loadgen/k6/` | k6 | n/a | `k6 run loadgen/k6/orders.js` |

`make help` lists every target. CI runs lint + test on every push;
[`.github/workflows/release.yml`](.github/workflows/release.yml) signs
and ships images on tag; [`.github/workflows/chaos-drill.yml`](.github/workflows/chaos-drill.yml)
spins a kind cluster on the GitHub runner and walks the MTTR demo
end-to-end as a smoke test.

## The MTTR demo, in one paragraph

`make mttr-drill` runs `chaos/payments-latency.yaml`, which injects
500 ms of latency on 50% of payments-svc traffic. Within ~30 seconds
the orders SLO's fast burn-rate alert fires (1h window at 14.4× the
budget). AlertManager POSTs the alert to the remediation-operator's
webhook, which classifies it (slow upstream → roll back the canary)
and triggers an Argo Rollouts undo to the previous stable revision.
The new pods carry the older payments client version with the lower
timeout; the SLO recovers in under two minutes. Grafana shows the
burn curve flatten in real time. Total MTTR is asserted by the CI
job — if it exceeds the threshold, the chaos drill fails the build.

## License

MIT. Copyright (c) 2026 Dimitris Sofikitis @dSofikitis.
