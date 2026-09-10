BINARY := keel
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/shiftu/keel/internal/cli.Version=$(VERSION)

.PHONY: build test fmt vet check clean

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/keel

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

check: fmt vet test

clean:
	rm -f $(BINARY)
	rm -rf dist
