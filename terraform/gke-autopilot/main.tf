# GKE Autopilot — pay-per-pod, no node-pool baseline cost. Picked
# over standard GKE on purpose: this cluster only exists for short-
# lived demo runs, and Autopilot's per-pod billing collapses to
# ~zero between `make cloud-up` and `make cloud-down`. Standard GKE
# would charge for the underlying node-pool VMs whether or not any
# pods were running.
#
# Autopilot also disables the workload-identity opt-out by default,
# enforces shielded nodes, and makes you pick `RegularChannel`-or-
# better — all things this repo would have asked for explicitly
# anyway, so accepting them as defaults removes config surface.

provider "google" {
  project = var.project_id
  region  = var.region
}

resource "google_container_cluster" "this" {
  name     = var.cluster_name
  location = var.region

  enable_autopilot    = true
  deletion_protection = var.deletion_protection

  release_channel { channel = var.release_channel }

  resource_labels = var.labels

  # Autopilot manages networking, node pools, and security defaults.
  # We deliberately don't override them: the whole point of Autopilot
  # is "Google's opinion is the supported configuration" — overriding
  # gets us a cluster that's harder to support, not easier.
}
