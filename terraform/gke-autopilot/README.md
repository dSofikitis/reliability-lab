# terraform/gke-autopilot

Single-cluster Terraform module for the GKE Autopilot path. Driven
by `make cloud-up` / `make cloud-down` from the repo root; the bare
`terraform apply` form lives here for poking at it directly.

## Run

```bash
cd terraform/gke-autopilot
terraform init
terraform apply -var project_id=YOUR_PROJECT
```

Then `terraform output kubeconfig_cmd` and run that to point kubectl
at the cluster.

## Why Autopilot

This cluster only exists for short-lived demo runs. Autopilot's
per-pod billing collapses to roughly zero between `make cloud-up`
and `make cloud-down`; a Standard GKE cluster would charge for the
underlying node-pool VMs the entire time, even with zero pods
scheduled.

Autopilot also enforces workload identity, shielded nodes, and
RegularChannel-or-better release channels by default — every one of
these is something we'd have asked for explicitly anyway, so
accepting them as defaults removes config surface rather than
removing control.

## What's NOT here

- Backend config. State lives in the local working dir by default;
  for a real deployment add a `backend "gcs" {}` block in your own
  fork. Keeping it local keeps the demo `apply` self-contained.
- Networking overrides (subnets, secondary ranges, VPC). Autopilot
  manages all of these. Overriding them gets you a cluster that's
  harder to support, not easier.
- IAM bindings. The caller's `gcloud auth` identity needs
  `container.admin` + `iam.serviceAccountUser` on the project; the
  module deliberately doesn't grant or assume more.
