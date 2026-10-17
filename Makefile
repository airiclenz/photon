# photon — developer Makefile.
#
# A thin wrapper over the Go toolchain: `go build` is the source of truth, these targets give the
# common invocations one-word names.

BINARY := photon
PKG    := ./cmd/photon

# Caps the Go runtime's heap so a test run fits a small host; override with `make test GOMEMLIMIT=...`.
GOMEMLIMIT ?= 2GiB
export GOMEMLIMIT

.PHONY: build test vet fmt

## build: compile the photon binary into the repo root
build:
	go build -o $(BINARY) $(PKG)

## test: run every package's tests
test:
	go test ./...

## vet: run go vet over every package
vet:
	go vet ./...

## fmt: rewrite every Go file with gofmt
fmt:
	gofmt -l -w .
