# Variables. Defaults are tuned for the cheapest viable demo:
# us-central1 (lowest GCP egress within the US), one regional
# Autopilot cluster (pay-per-pod, no node-pool baseline cost).
#
# `project_id` is the only required input — everything else has
# sensible defaults so `make cloud-up` is one variable from working.

variable "project_id" {
  description = "GCP project the cluster lives in. The caller's gcloud credentials must have container.admin + iam.serviceAccountUser on this project."
  type        = string
}

variable "region" {
  description = "GCP region. Autopilot is regional-only — there's no zonal flavor."
  type        = string
  default     = "us-central1"
}

variable "cluster_name" {
  description = "GKE cluster name. Visible in the GCP console + the kubeconfig context."
  type        = string
  default     = "reliability-lab"
}

variable "release_channel" {
  description = "Autopilot release channel. RAPID gets newer Kubernetes versions sooner; STABLE lags by a month or two. RAPID is fine for a demo cluster — we want the latest behaviour, not the patched-for-six-months one."
  type        = string
  default     = "RAPID"
}

variable "deletion_protection" {
  description = "Whether to forbid `terraform destroy` on the cluster. Off by default because the whole point of this module is `make cloud-up` / `make cloud-down`."
  type        = bool
  default     = false
}

variable "labels" {
  description = "Labels applied to the cluster. Useful for cost-allocation queries in BigQuery billing exports."
  type        = map(string)
  default = {
    project = "reliability-lab"
    purpose = "portfolio-demo"
  }
}
