# Localias Makefile
# Author: Thiru K
# Build targets for the localias reverse proxy CLI tool

BINARY_NAME=localias
MODULE=github.com/thirukguru/localias
LDFLAGS=-s -w

.PHONY: build release test install clean vet

## build: Build for current platform
build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .

## release: Build for all supported platforms
release: clean
	@mkdir -p dist
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-amd64  .
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-arm64  .
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-amd64   .
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-arm64   .

## test: Run all tests
test:
	go test ./... -v -count=1 -race

## vet: Run go vet
vet:
	go vet ./...

## install: Install to /usr/local/bin (run 'make build' first)
install:
	@test -f $(BINARY_NAME) || (echo "Error: run 'make build' first" && exit 1)
	sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "✓ Installed $(BINARY_NAME) to /usr/local/bin"

## clean: Remove build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
