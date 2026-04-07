PKG_DIR := ./cmd/xf
DIST_DIR := ./dist/

VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%d 2>/dev/null || echo "unknown")

LDFLAGS := -X github.com/xenforo-ltd/cli/internal/version.Version=$(VERSION) \
	-X github.com/xenforo-ltd/cli/internal/version.Commit=$(COMMIT) \
	-X github.com/xenforo-ltd/cli/internal/version.Date=$(DATE)

.DEFAULT_GOAL := all
.PHONY: all
all: lint test build

.PHONY: build
## Build the binary
build:
	go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR) $(PKG_DIR)

.PHONY: clean
## Remove build artifacts
clean:
	rm -rf $(DIST_DIR)

.PHONY: fix
## Apply automated fixes
fix:
	go fix ./...

.PHONY: fmt
## Format source files
fmt:
	go fmt ./...

.PHONY: fmt-check
## Check formatting without making changes
fmt-check:
	gofmt -l .

.PHONY: help
## Show this help message
help:
	@echo 'Usage:'
	@awk '/^## /{help=substr($$0,4); next} /^[[:alnum:]_.-]+:/{if(help){printf "  %-16s %s\n", $$1, help; help=""}}' $(MAKEFILE_LIST)

.PHONY: install
## Install the binary
install:
	cd "$(PKG_DIR)" && go install -ldflags "$(LDFLAGS)" .

.PHONY: lint
## Run linters
lint: vet fmt-check

.PHONY: lint-ci
## Run linters (golangci-lint)
lint-ci:
	golangci-lint run

.PHONY: list
## List module dependencies
list:
	go list -m -u all

.PHONY: mod-tidy
## Tidy module dependencies
mod-tidy:
	go mod tidy

.PHONY mod-tidy-diff
## Check module dependencies without making changes
mod-tidy-diff:
	go mod tidy -diff

.PHONY: mod-verify
## Verify module dependencies
mod-verify:
	go mod verify

.PHONY: test
## Run the test suite
test:
	go test ./...

.PHONY: test-coverage
## Run tests with coverage
test-coverage:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...

.PHONY: test-race
## Run tests with the race detector
test-race:
	go test -race ./...

.PHONY: test-v
## Run tests with verbose output
test-v:
	go test -v ./...

.PHONY: uninstall
## Remove the installed binary
uninstall:
	cd "$(PKG_DIR)" && go clean -i

.PHONY: vet
## Check for common mistakes
vet:
	go vet ./...
