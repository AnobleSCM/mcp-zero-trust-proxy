# Stage 1: Build. Runs on the build machine's own CPU and cross-compiles for
# the target platform, so a multi-platform build needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /mcpproxy \
    ./cmd/mcpproxy

# Stage 2: Runtime
# No RUN step here: copying the CA bundle from the builder keeps this stage
# free of target-architecture commands, so no emulation is needed.
FROM alpine:3.19
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /mcpproxy /usr/local/bin/mcpproxy
COPY configs/example.yaml /etc/mcpproxy/config.yaml
EXPOSE 8080
ENTRYPOINT ["mcpproxy"]
CMD ["--config", "/etc/mcpproxy/config.yaml"]
