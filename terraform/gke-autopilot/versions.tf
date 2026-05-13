# Pinned providers + Terraform version. Pinning matters here because
# the GKE Autopilot resource shape evolves between provider releases
# (regional vs zonal defaults, deletion-protection defaults, deletion
# timeout shape) and silent drift would surface as failed `make
# cloud-up` runs months later.

terraform {
  required_version = ">= 1.7"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.10"
    }
  }
}
