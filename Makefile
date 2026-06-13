VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
	-X github.com/0xdps/daemon-hound/internal/cmd.version=$(VERSION) \
	-X github.com/0xdps/daemon-hound/internal/cmd.commit=$(COMMIT) \
	-X github.com/0xdps/daemon-hound/internal/cmd.date=$(DATE)

.PHONY: run
run:
	go run ./cmd/dh

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o bin/dh ./cmd/dh

.PHONY: test
test:
	go test -v ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: clean
clean:
	rm -rf bin dist

.PHONY: snapshot
snapshot:
	goreleaser release --snapshot --clean

.PHONY: release
release:
	goreleaser release --clean

.PHONY: docker-build
docker-build:
	docker build -t ghcr.io/0xdps/daemon-hound:latest .

.PHONY: install
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/dh
