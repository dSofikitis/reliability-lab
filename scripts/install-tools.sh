#!/usr/bin/env bash
# Install the toolchain `make demo` and the CI workflows depend on.
# Idempotent — re-running skips anything already on $PATH.
#
# Supports macOS (Homebrew) and Linux (apt fallback + direct binary
# downloads). On Windows, run under WSL2.

set -euo pipefail

OS=""
case "$(uname -s)" in
  Darwin) OS=mac ;;
  Linux)  OS=linux ;;
  *) echo "Unsupported OS: $(uname -s). Run under WSL2 on Windows." >&2; exit 1 ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;;
esac

have() { command -v "$1" >/dev/null 2>&1; }

install_mac() {
  have brew || { echo "Homebrew required on macOS"; exit 1; }
  local pkg
  for pkg in kind kubectl kustomize helm linkerd cosign trivy bufbuild/buf/buf k6 argo-rollouts; do
    local bin="${pkg##*/}"
    if have "$bin"; then echo "skip $bin (already installed)"; else brew install "$pkg"; fi
  done
}

install_linux() {
  # kubectl, helm, kustomize, k6: available via apt/snap on most distros.
  # The rest pull GitHub release tarballs directly to keep the script
  # portable across distros without root for the install step.
  local bin_dir="${HOME}/.local/bin"
  mkdir -p "$bin_dir"

  fetch() {
    # fetch <name> <url> <archive-member-or-empty>
    local name="$1" url="$2" member="${3:-}"
    if have "$name"; then echo "skip $name"; return; fi
    local tmp; tmp="$(mktemp -d)"
    echo "install $name from $url"
    curl -fsSL "$url" -o "$tmp/dl"
    if [[ "$url" =~ \.tar\.gz$ ]]; then
      tar -xzf "$tmp/dl" -C "$tmp"
      install -m 0755 "$tmp/${member:-$name}" "$bin_dir/$name"
    else
      install -m 0755 "$tmp/dl" "$bin_dir/$name"
    fi
    rm -rf "$tmp"
  }

  fetch kind     "https://kind.sigs.k8s.io/dl/v0.24.0/kind-linux-${ARCH}"
  fetch kubectl  "https://dl.k8s.io/release/v1.31.0/bin/linux/${ARCH}/kubectl"
  fetch kustomize "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv5.4.3/kustomize_v5.4.3_linux_${ARCH}.tar.gz" "kustomize"
  fetch helm     "https://get.helm.sh/helm-v3.16.1-linux-${ARCH}.tar.gz" "linux-${ARCH}/helm"
  fetch linkerd  "https://github.com/linkerd/linkerd2/releases/download/edge-25.1.1/linkerd2-cli-edge-25.1.1-linux-${ARCH}"
  fetch cosign   "https://github.com/sigstore/cosign/releases/download/v2.4.1/cosign-linux-${ARCH}"
  fetch trivy    "https://github.com/aquasecurity/trivy/releases/download/v0.55.2/trivy_0.55.2_Linux-64bit.tar.gz" "trivy"
  fetch buf      "https://github.com/bufbuild/buf/releases/download/v1.45.0/buf-Linux-x86_64"
  fetch k6       "https://github.com/grafana/k6/releases/download/v0.53.0/k6-v0.53.0-linux-${ARCH}.tar.gz" "k6-v0.53.0-linux-${ARCH}/k6"

  echo
  echo "Tools installed under $bin_dir."
  echo "Add to PATH if not already:  export PATH=\"$bin_dir:\$PATH\""
}

case "$OS" in
  mac)   install_mac ;;
  linux) install_linux ;;
esac

echo
echo "All tools ready. Verify with: kind version && kubectl version --client && linkerd version --client"
