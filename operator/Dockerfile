# Multi-stage: alpine-based static build, distroless nonroot runtime.
# Same shape as services/*/Dockerfile so the release pipeline treats
# every image identically — Cosign signs them all the same way and
# Trivy scans them with the same baseline.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/remediation-operator ./operator/cmd/manager

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/remediation-operator /usr/bin/remediation-operator
USER nonroot:nonroot
# 8088 = AlertManager webhook, 8090 = metrics, 8091 = healthz/readyz
EXPOSE 8088 8090 8091
ENTRYPOINT ["/usr/bin/remediation-operator"]
