VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/mansooranis/kode/internal/buildinfo.Version=$(VERSION)

.PHONY: build install test

build:
	go build -ldflags "$(LDFLAGS)" -o kode ./cmd/kode

# Mirrors what a Homebrew formula's install step should run, so the released
# binary reports a real version and syncs its bundled skills accordingly.
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/kode

test:
	go test ./...
