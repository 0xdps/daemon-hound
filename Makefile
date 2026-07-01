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
build-macos: build-macos-icon
	@mkdir -p build/DaemonHound.app/Contents/{MacOS,Resources}
	cp bin/dhd build/DaemonHound.app/Contents/MacOS/dhd
	cp build/Info.plist build/DaemonHound.app/Contents/
	cp build/AppIcon.icns build/DaemonHound.app/Contents/Resources/
	@echo "✓ macOS app bundle created: build/DaemonHound.app"

.PHONY: install-macos
install-macos: build-macos
	@cp -r build/DaemonHound.app ~/Applications/DaemonHound.app
	@chmod +x ~/Applications/DaemonHound.app/Contents/MacOS/dhd
	@echo "✓ Daemon Hound installed to ~/Applications/DaemonHound.app"

.PHONY: launch-daemon
launch-daemon: install-macos
	@open ~/Applications/DaemonHound.app
	@echo "✓ Daemon Hound launched"

.PHONY: build-macos-icon
build-macos-icon:
	@mkdir -p build/AppIcon.iconset
	@if [ ! -f build/AppIcon.icns ]; then \
		magick images/logo.png -resize 16x16 build/AppIcon.iconset/icon_16x16.png; \
		magick images/logo.png -resize 32x32 build/AppIcon.iconset/icon_32x32.png; \
		magick images/logo.png -resize 64x64 build/AppIcon.iconset/icon_64x64.png; \
		magick images/logo.png -resize 128x128 build/AppIcon.iconset/icon_128x128.png; \
		magick images/logo.png -resize 256x256 build/AppIcon.iconset/icon_256x256.png; \
		magick images/logo.png -resize 512x512 build/AppIcon.iconset/icon_512x512.png; \
		magick images/logo.png -resize 1024x1024 build/AppIcon.iconset/icon_1024x1024.png; \
		iconutil -c icns build/AppIcon.iconset -o build/AppIcon.icns; \
		echo "✓ App icon generated"; \
	fi

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
