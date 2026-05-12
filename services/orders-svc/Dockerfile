# Multi-stage build: compile statically against alpine, run on
# distroless. Each service has its own Dockerfile rather than a
# shared one — they're small, and per-service Dockerfiles make the
# build context tighter (no risk of one service's local changes
# invalidating another service's image cache).

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/orders-svc ./services/orders-svc

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/orders-svc /usr/bin/orders-svc
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/bin/orders-svc"]
