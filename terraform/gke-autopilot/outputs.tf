# Outputs the Makefile reads to build the kubectl-config command and
# the cluster URL. Sensitive=true on the endpoint to suppress it from
# `terraform output` accidentally leaking into shared terminals.

output "cluster_name" {
  description = "GKE cluster name."
  value       = google_container_cluster.this.name
}

output "region" {
  description = "GCP region the cluster lives in."
  value       = google_container_cluster.this.location
}

output "endpoint" {
  description = "Cluster API endpoint (private + public depending on Autopilot defaults)."
  value       = google_container_cluster.this.endpoint
  sensitive   = true
}

# The single command needed to point local kubectl at this cluster.
# Printing it as an output (rather than hoping the operator
# remembers the gcloud syntax) is the whole reason this output exists.
output "kubeconfig_cmd" {
  description = "Run this to point kubectl at the cluster."
  value       = "gcloud container clusters get-credentials ${google_container_cluster.this.name} --region ${google_container_cluster.this.location} --project ${var.project_id}"
}
