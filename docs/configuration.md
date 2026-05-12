# Configuration reference

> Fills in as components land. The Makefile and the Kustomize
> overlays both read from the same env-var set so the same `make
> demo` invocation can target kind or GKE Autopilot.

## Top-level

| Var | Default | What |
|---|---|---|
| `CLUSTER` | `kind` | Selects the Kustomize overlay under `k8s/overlays/`. Currently `kind` or `gke`. |
| `IMAGE_REGISTRY` | `ghcr.io/dsofikitis/reliability-lab` | Where built images are pushed. The Kyverno policy is keyed to images under this prefix. |
| `IMAGE_TAG` | `dev` | Tag stamped onto every service. CI overrides with the git SHA. |

## Cluster overlays

| Var | Where | What |
|---|---|---|
| `K8S_NAMESPACE` | `k8s/base/kustomization.yaml` | Default `reliability-lab`. |
| `LINKERD_VERSION` | `Makefile` `mesh-install` | Linkerd CLI version pinned for reproducibility. |
| `OTEL_COLLECTOR_ENDPOINT` | service env | Defaults to the in-cluster collector. |

## Cloud (terraform/gke-autopilot)

| Var | Default | What |
|---|---|---|
| `gcp_project_id` | unset | Required. GCP project to deploy into. |
| `gcp_region` | `europe-north1` | Cheap-tier region. Override for latency or compliance. |
| `cluster_name` | `reliability-lab` | GKE cluster name. |

## Supply chain

| Var | Default | What |
|---|---|---|
| `COSIGN_EXPERIMENTAL` | `1` | Required for keyless OIDC signing in CI. |
| `COSIGN_IDENTITY` | `https://github.com/dSofikitis/reliability-lab/.github/workflows/release.yml@refs/heads/main` | The OIDC identity the Kyverno policy trusts. |
