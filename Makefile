GO ?= go
GOLANGCI_LINT ?= golangci-lint
BINARY ?= godrain
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build test lint vet tidy check demo-leaky demo-cached clean

all: check build

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY)$(shell go env GOEXE) ./cmd/godrain

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

tidy:
	$(GO) mod tidy

check: tidy vet test lint

demo-leaky:
	$(GO) run ./testdata/leaky

demo-cached:
	$(GO) run ./testdata/cached

clean:
	rm -f $(BINARY) $(BINARY).exe
