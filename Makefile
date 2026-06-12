.PHONY: run
run:
	go run ./cmd/dh

.PHONY: build
build:
	go build -o bin/dh ./cmd/dh

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
	rm -rf bin
