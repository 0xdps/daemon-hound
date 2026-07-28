VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
	-X github.com/0xdps/daemon-hound/internal/cmd.version=$(VERSION) \
	-X github.com/0xdps/daemon-hound/internal/cmd.commit=$(COMMIT) \
	-X github.com/0xdps/daemon-hound/internal/cmd.date=$(DATE)

.PHONY: run
run:
	go run ./cmd/dhd

.PHONY: build
build:
	PAGER=cat BAT_PAGER=cat go build -ldflags "$(LDFLAGS)" -o bin/dhd ./cmd/dhd

.PHONY: build-macos
build-macos:
	@mkdir -p build/DaemonHound.app/Contents/{MacOS,Resources}
	cp bin/dhd build/DaemonHound.app/Contents/MacOS/dhd
	cp macos/Info.plist build/DaemonHound.app/Contents/
	cp macos/AppIcon.icns build/DaemonHound.app/Contents/Resources/
	@if [ -n "$$APPLE_DEVELOPER_IDENTITY" ]; then \
		codesign --force --deep --options runtime --sign "$$APPLE_DEVELOPER_IDENTITY" build/DaemonHound.app; \
		echo "✓ Signed macOS app bundle with $$APPLE_DEVELOPER_IDENTITY"; \
	fi
	@echo "✓ macOS app bundle created: build/DaemonHound.app"

.PHONY: install-macos
install-macos: build-macos
	@rm -rf ~/Applications/DaemonHound.app
	@cp -r build/DaemonHound.app ~/Applications/DaemonHound.app
	@chmod +x ~/Applications/DaemonHound.app/Contents/MacOS/dhd
	@echo "✓ Daemon Hound installed to ~/Applications/DaemonHound.app"

.PHONY: launch-daemon
launch-daemon: install-macos
	@open ~/Applications/DaemonHound.app
	@echo "✓ Daemon Hound launched"


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
	rm -rf bin build dist

.PHONY: snapshot
snapshot:
	goreleaser release --snapshot --clean

.PHONY: release
release:
	goreleaser release --clean

.PHONY: tag
tag:
ifndef TAG
	@echo "Error: TAG is required. Usage: make tag TAG=v1.0.0"
	@exit 1
endif
	@echo "Creating and pushing tag $(TAG)..."
	git tag -a $(TAG) -m "Release $(TAG)"
	git push origin $(TAG)
	@echo "✓ Tag $(TAG) created and pushed"

.PHONY: docker-build
docker-build:
	docker build -t ghcr.io/0xdps/daemon-hound:latest .

.PHONY: install
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/dhd
