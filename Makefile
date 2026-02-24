.PHONY: build release run test test-v test-cover fmt vet clean tidy help all

BINARY := xf
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%d)
LDFLAGS := -ldflags "-X github.com/xenforo-ltd/cli/internal/version.Version=$(VERSION) \
	-X github.com/xenforo-ltd/cli/internal/version.Commit=$(COMMIT) \
	-X github.com/xenforo-ltd/cli/internal/version.Date=$(DATE)"

all: fmt vet test build

build:
	go build -o $(BINARY) .

release:
	go build $(LDFLAGS) -o $(BINARY) .

run:
	go run . $(ARGS)

test:
	go test ./...

test-v:
	go test ./... -v

test-cover:
	go test ./... -cover

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)

tidy:
	go mod tidy

help:
	@echo "Available targets:"
	@echo "  make build      - Build the binary"
	@echo "  make release    - Build with version info (VERSION=x.x.x)"
	@echo "  make run        - Run without building (ARGS='version')"
	@echo "  make test       - Run all tests"
	@echo "  make test-v     - Run tests with verbose output"
	@echo "  make test-cover - Run tests with coverage"
	@echo "  make fmt        - Format code"
	@echo "  make vet        - Check for common mistakes"
	@echo "  make clean      - Remove built binary"
	@echo "  make tidy       - Update dependencies"
	@echo "  make all        - Format, vet, test, and build"
