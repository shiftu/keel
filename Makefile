BINARY := keel
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/shiftu/keel/internal/cli.Version=$(VERSION)
TARGETS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: build test fmt vet check release install clean

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/keel
	@echo "./$(BINARY) ($(VERSION))"

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

check: fmt vet test

# 跨平台产物 + 校验和，供 gh release upload。发版流程见 docs/release.md。
release: vet test
	@mkdir -p dist
	@for t in $(TARGETS); do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=.exe; \
	  echo "  $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/$(BINARY)_$${os}_$${arch}$$ext ./cmd/keel || exit 1; \
	done
	@cd dist && shasum -a 256 $(BINARY)_* > SHA256SUMS && cat SHA256SUMS

install: build
	install -m 0755 $(BINARY) $${KEEL_INSTALL_DIR:-$$HOME/.local/bin}/$(BINARY)
	@echo "已安装到 $${KEEL_INSTALL_DIR:-$$HOME/.local/bin}/$(BINARY)"

clean:
	rm -f $(BINARY)
	rm -rf dist
