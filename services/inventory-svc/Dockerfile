FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/inventory-svc ./services/inventory-svc

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/inventory-svc /usr/bin/inventory-svc
USER nonroot:nonroot
EXPOSE 9000 8080
ENTRYPOINT ["/usr/bin/inventory-svc"]
