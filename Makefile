# Mello CLI daemon — build/test/release targets.

BINARY      := mello
CMD         := ./cmd/mello
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test vet lint clean install snapshot

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

test:
	go test ./...

vet:
	go vet ./...

# Optional; requires golangci-lint to be installed.
lint:
	golangci-lint run ./... || echo "golangci-lint not installed; skipping"

install:
	go install -ldflags "$(LDFLAGS)" $(CMD)

# Cross-compile a local snapshot via goreleaser (no publish).
snapshot:
	goreleaser build --snapshot --clean

clean:
	rm -rf bin dist
