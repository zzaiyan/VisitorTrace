GO ?= go
VERSION ?= 0.1.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
UPDATE_PUBLIC_KEY ?=
LDFLAGS = -s -w \
	-X github.com/zzaiyan/VisitorTrace/internal/buildinfo.Version=$(VERSION) \
	-X github.com/zzaiyan/VisitorTrace/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/zzaiyan/VisitorTrace/internal/buildinfo.BuildTime=$(BUILD_TIME) \
	-X github.com/zzaiyan/VisitorTrace/internal/buildinfo.UpdatePublicKey=$(UPDATE_PUBLIC_KEY)

.PHONY: build test vet check clean

build:
	mkdir -p bin
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/visitortrace ./cmd/visitortrace

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

check: test vet

clean:
	$(GO) clean -testcache
