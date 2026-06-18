# Multi-stage Dockerfile for local development and CI builds
# For GoReleaser releases, see Dockerfile.goreleaser which uses pre-built binaries
#
# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Enable Go toolchain auto-download to get the exact version needed
ENV GOTOOLCHAIN=auto

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X github.com/0xdps/daemon-hound/internal/cmd.version=${VERSION} -X github.com/0xdps/daemon-hound/internal/cmd.commit=${COMMIT} -X github.com/0xdps/daemon-hound/internal/cmd.date=${DATE}" \
    -o dhd ./cmd/dhd

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache git ca-certificates

COPY --from=builder /build/dhd /usr/local/bin/dhd
RUN chmod +x /usr/local/bin/dh

# Create a non-root user for running dh
RUN adduser -D -s /bin/sh dhuser

# Set up volumes for config and working directory
VOLUME ["/home/dhuser/.dh", "/work"]
WORKDIR /work

USER dhuser

ENTRYPOINT ["dh"]
CMD ["--help"]
