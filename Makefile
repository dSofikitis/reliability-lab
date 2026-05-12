# Reliability Lab — top-level entrypoint.
#
# Targets are grouped by lifecycle (build → cluster → install → drill).
# Many are stubs at this point in the build — each one carries the
# phase number where it gets real implementation, so `make <target>`
# never silently no-ops without telling you where to look.

SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

CLUSTER ?= kind
IMAGE_REGISTRY ?= ghcr.io/dsofikitis/reliability-lab
IMAGE_TAG ?= dev
SERVICES := orders-svc payments-svc inventory-svc email-worker

# ───────────────────────── help ─────────────────────────

.PHONY: help
help: ## Show all targets, grouped, with their one-line description.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
	  /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Toolchain
.PHONY: tools
tools: ## Install kind / kubectl / kustomize / helm / linkerd / cosign / trivy / buf.
	@bash scripts/install-tools.sh

##@ Build (phase 2-3)
.PHONY: proto proto-lint build test lint images
proto: proto-lint ## Regenerate Go from proto/ via buf into gen/go.
	cd proto && buf generate

proto-lint: ## buf lint + breaking-change check against main.
	cd proto && buf lint

build: ## go build every service binary into bin/.
	@mkdir -p bin
	@for svc in $(SERVICES); do \
	  echo "build $$svc"; \
	  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/$$svc ./services/$$svc; \
	done

test: ## go test ./... for every service and the operator.
	go test ./...

lint: proto-lint ## go vet + buf lint.
	go vet ./...

images: ## docker build every service image and tag with $(IMAGE_TAG).
	@for svc in $(SERVICES); do \
	  echo "image $$svc"; \
	  docker build -f services/$$svc/Dockerfile -t $(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG) .; \
	done

##@ Cluster (phase 4)
.PHONY: kind-up kind-down apply
kind-up: ## Create a 3-node kind cluster and load images.
	@echo "[phase 4] kind-up not yet wired"

kind-down: ## Destroy the kind cluster.
	@echo "[phase 4] kind-down not yet wired"

apply: ## kubectl apply -k k8s/overlays/$(CLUSTER)
	@echo "[phase 4] apply not yet wired"

##@ Stack installs (phase 5-11)
.PHONY: mesh-install obs-install chaos-install rollouts-install policy-install
mesh-install: ## Install Linkerd CRDs + control plane.
	@echo "[phase 5] linkerd install not yet wired"

obs-install: ## Install kube-prometheus-stack + Grafana dashboards.
	@echo "[phase 6] observability install not yet wired"

chaos-install: ## Install chaos-mesh CRDs + control plane.
	@echo "[phase 8] chaos-mesh install not yet wired"

rollouts-install: ## Install Argo Rollouts CRDs + controller.
	@echo "[phase 10] argo rollouts install not yet wired"

policy-install: ## Install Kyverno CRDs + verifyImages cluster policy.
	@echo "[phase 12] kyverno install not yet wired"

##@ End-to-end (phase 13+)
.PHONY: demo mttr-drill chaos-run slo-check
demo: ## Full local demo: kind → installs → apply → loadgen → open Grafana.
	@echo "[phase 13] demo not yet wired"

mttr-drill: ## Chaos → SLO burn → operator remedy → SLO recovery, narrated.
	@echo "[phase 13] mttr-drill not yet wired"

chaos-run: ## Apply a single chaos experiment: make chaos-run EXP=payments-latency
	@echo "[phase 8] chaos-run not yet wired"

slo-check: ## Tail Prometheus for the SLO recording rule values.
	@echo "[phase 7] slo-check not yet wired"

##@ Cloud (phase 14)
.PHONY: cloud-up cloud-down
cloud-up: ## terraform apply against terraform/gke-autopilot.
	@echo "[phase 14] terraform apply not yet wired"

cloud-down: ## terraform destroy.
	@echo "[phase 14] terraform destroy not yet wired"
