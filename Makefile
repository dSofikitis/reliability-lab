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

operator-build: ## Build the remediation-operator binary into bin/.
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/remediation-operator ./operator/cmd/manager

operator-test: ## go test the operator package only.
	go test ./operator/...

test: ## go test ./... for every service and the operator.
	go test ./...

lint: proto-lint ## go vet + buf lint.
	go vet ./...

images: ## docker build every service + operator image and tag with $(IMAGE_TAG).
	@for svc in $(SERVICES); do \
	  echo "image $$svc"; \
	  docker build -f services/$$svc/Dockerfile -t $(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG) .; \
	done
	@echo "image remediation-operator"
	@docker build -f operator/Dockerfile -t $(IMAGE_REGISTRY)/remediation-operator:$(IMAGE_TAG) .

##@ Cluster (phase 4)
.PHONY: kind-up kind-down kind-load apply
kind-up: ## Create the 3-node kind cluster.
	kind create cluster --config k8s/kind-cluster.yaml

kind-down: ## Destroy the kind cluster.
	kind delete cluster --name reliability-lab

kind-load: ## docker save + kind load every service + operator image into the cluster.
	@for svc in $(SERVICES); do \
	  echo "load $$svc"; \
	  kind load docker-image $(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG) --name reliability-lab; \
	done
	@echo "load remediation-operator"
	@kind load docker-image $(IMAGE_REGISTRY)/remediation-operator:$(IMAGE_TAG) --name reliability-lab

apply: ## kubectl apply -k k8s/overlays/$(CLUSTER)
	kubectl apply -k k8s/overlays/$(CLUSTER)

##@ Stack installs (phase 5-11)
.PHONY: mesh-install obs-install chaos-install rollouts-install policy-install
mesh-install: ## Install Linkerd CRDs + control plane + viz, then inject the namespace.
	linkerd install --crds | kubectl apply -f -
	linkerd install | kubectl apply -f -
	linkerd check
	linkerd viz install | kubectl apply -f -
	kubectl apply -f k8s/mesh/namespace-patch.yaml

obs-install: ## Install kube-prometheus-stack, OTel collector, and SLO rules.
	helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
	helm repo update
	helm upgrade --install kube-prometheus prometheus-community/kube-prometheus-stack \
	  --namespace monitoring --create-namespace \
	  -f k8s/observability/kube-prometheus-stack-values.yaml
	kubectl apply -n reliability-lab -f k8s/observability/otel-collector.yaml
	kubectl apply -n monitoring -f k8s/prometheus/rules/

chaos-install: ## Install chaos-mesh CRDs + control plane.
	helm repo add chaos-mesh https://charts.chaos-mesh.org
	helm repo update
	kubectl create ns chaos-mesh --dry-run=client -o yaml | kubectl apply -f -
	helm upgrade --install chaos-mesh chaos-mesh/chaos-mesh \
	  --namespace chaos-mesh \
	  --set chaosDaemon.runtime=containerd \
	  --set chaosDaemon.socketPath=/run/containerd/containerd.sock

rollouts-install: ## Install Argo Rollouts CRDs + controller (argo helm chart).
	helm repo add argo https://argoproj.github.io/argo-helm
	helm repo update
	helm upgrade --install argo-rollouts argo/argo-rollouts \
	  --namespace argo-rollouts --create-namespace \
	  --set dashboard.enabled=true \
	  --set controller.metrics.serviceMonitor.enabled=false
	kubectl -n argo-rollouts rollout status deploy/argo-rollouts

policy-install: ## Install Kyverno CRDs + controller, then apply the verifyImages cluster policy.
	helm repo add kyverno https://kyverno.github.io/kyverno
	helm repo update
	helm upgrade --install kyverno kyverno/kyverno \
	  --namespace kyverno --create-namespace \
	  --set replicaCount=1
	kubectl -n kyverno rollout status deploy/kyverno-admission-controller
	kubectl apply -f k8s/policy/kyverno-verify-images.yaml

##@ End-to-end
.PHONY: demo mttr-drill chaos-run slo-check
# Full local demo. Sequence matters:
#   1. cluster + images first (everything below depends on the
#      Kustomize apply having images to pull)
#   2. mesh + obs + chaos + rollouts BEFORE apply, so the namespace
#      annotation injection + CRDs (Rollout, AnalysisTemplate,
#      PrometheusRule, Chaos*) are all known to the apiserver when
#      our manifests reference them
#   3. apply
#   4. wait for pods
#
# Kyverno's verifyImages policy is deliberately NOT installed here:
# kind-loaded local images don't carry Cosign signatures and the
# policy would refuse them at admission. The policy belongs in the
# CI / cloud path; for local laptop work we trust kind.
demo: ## Full local demo: kind cluster + every install + manifests + readiness wait.
	$(MAKE) kind-up
	$(MAKE) images
	$(MAKE) kind-load
	$(MAKE) mesh-install
	$(MAKE) obs-install
	$(MAKE) chaos-install
	$(MAKE) rollouts-install
	$(MAKE) apply
	@echo "waiting for pods to become ready..."
	@kubectl wait --for=condition=Ready pods --all -n reliability-lab --timeout=240s || true
	@echo ""
	@echo "  Grafana:     http://localhost:31300  (admin / admin)"
	@echo "  orders-svc:  http://localhost:31080"
	@echo ""
	@echo "  Drive load:  k6 run loadgen/k6/orders.js"
	@echo "  Run drill:   make mttr-drill"

mttr-drill: ## Chaos → SLO burn → operator remedy → SLO recovery, narrated with timestamps.
	@bash scripts/mttr-drill.sh

chaos-run: ## Apply a single chaos experiment: make chaos-run EXP=payments-latency
	@test -n "$(EXP)" || { echo "usage: make chaos-run EXP=payments-latency" >&2; exit 1; }
	kubectl apply -f chaos/$(EXP).yaml

slo-check: ## Tail Prometheus for the SLO recording rule values (success ratio + burn rate).
	@bash scripts/slo-check.sh

##@ Cloud
.PHONY: cloud-up cloud-down cloud-kubeconfig
# GCP_PROJECT is required for cloud-up / cloud-down. Surfacing the
# guard here rather than letting terraform error with a less obvious
# message — the failure mode is "you forgot to set GCP_PROJECT", and
# saying so explicitly saves a debugging cycle.
GCP_PROJECT ?=

cloud-up: ## Provision the GKE Autopilot cluster: terraform apply -var project_id=$(GCP_PROJECT).
	@test -n "$(GCP_PROJECT)" || { echo "GCP_PROJECT=<project-id> is required" >&2; exit 1; }
	cd terraform/gke-autopilot && terraform init -upgrade && terraform apply -auto-approve -var project_id=$(GCP_PROJECT)
	@echo ""
	@echo "  cluster ready. point kubectl at it with:"
	@echo "    \$$(cd terraform/gke-autopilot && terraform output -raw kubeconfig_cmd)"

cloud-down: ## terraform destroy the GKE Autopilot cluster.
	@test -n "$(GCP_PROJECT)" || { echo "GCP_PROJECT=<project-id> is required" >&2; exit 1; }
	cd terraform/gke-autopilot && terraform destroy -auto-approve -var project_id=$(GCP_PROJECT)

cloud-kubeconfig: ## Print the gcloud command that wires kubectl to the cluster.
	@cd terraform/gke-autopilot && terraform output -raw kubeconfig_cmd
