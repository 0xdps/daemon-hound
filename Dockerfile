# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o dh ./cmd/dh

# Final stage
FROM alpine:latest

RUN apk add --no-cache git ca-certificates

COPY --from=builder /build/dh /usr/local/bin/dh

# Create a non-root user for running dh
RUN adduser -D -s /bin/sh dhuser

# Set up volumes for config and working directory
VOLUME ["/home/dhuser/.dh", "/work"]
WORKDIR /work

USER dhuser

ENTRYPOINT ["dh"]
CMD ["--help"]
