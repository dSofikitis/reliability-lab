FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/email-worker ./services/email-worker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/email-worker /usr/bin/email-worker
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/bin/email-worker"]
