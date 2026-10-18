# photon — developer Makefile.
#
# A thin wrapper over the Go toolchain: `go build` is the source of truth, these targets give the
# common invocations one-word names.

BINARY := photon
PKG    := ./cmd/photon

# Caps the Go runtime's heap so a test run fits a small host; override with `make test GOMEMLIMIT=...`.
GOMEMLIMIT ?= 2GiB
export GOMEMLIMIT

# golangci-lint runs the standard linter set plus lll over the module, configured by
# `.golangci.yml`. It is pinned by module version and fetched by `go run`, so nothing needs
# installing; after the first run the module cache serves it. CI reads this version from here, so
# it is the only literal.
GOLANGCI_LINT_VERSION := v2.13.2
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: build test vet lint fmt

## build: compile the photon binary into the repo root
build:
	go build -o $(BINARY) $(PKG)

## test: run every package's tests
test:
	go test ./...

## vet: run go vet over every package
vet:
	go vet ./...

## lint: run golangci-lint (standard set plus lll and gofmt, .golangci.yml) over the module
lint:
	$(GOLANGCI_LINT) run ./...

## fmt: rewrite every Go file with gofmt
fmt:
	gofmt -l -w .
