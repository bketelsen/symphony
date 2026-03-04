.PHONY: all build clean fmt lint test test-cover tidy generate check help

# Build variables
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Go commands
GO := go
GOFMT := gofmt
GOFILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./.worktrees/*")

all: fmt build

## build: Build the binary
build:
	$(GO) build $(LDFLAGS) -o build/symphony ./cmd/symphony

## install: Install binary to GOPATH/bin
install:
	$(GO) install $(LDFLAGS) ./cmd/symphony

## clean: Remove build artifacts
clean:
	rm -rf build/
	$(GO) clean

## fmt: Format Go source files
fmt:
	$(GOFMT) -w $(GOFILES)

## lint: Run linter
lint:
	@golangci-lint run || $(GO) vet ./...

## test: Run tests
test:
	$(GO) test -race -count=1 ./...

## test-cover: Run tests with coverage
test-cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## tidy: Tidy go modules
tidy:
	$(GO) mod tidy

## generate: Run code generation (templ)
generate:
	@templ generate 2>/dev/null || echo "templ not installed, skipping"

## check: Run fmt, lint, and test
check: fmt lint test

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
