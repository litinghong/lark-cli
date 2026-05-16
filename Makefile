# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

BINARY   := lark-cli
MODULE   := github.com/larksuite/cli
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DATE     := $(shell date +%Y-%m-%d)
LDFLAGS  := -s -w -X $(MODULE)/internal/build.Version=$(VERSION) -X $(MODULE)/internal/build.Date=$(DATE)
PREFIX   ?= /usr/local

.PHONY: all build build-shared-darwin build-shared-linux build-shared-catalog vet test unit-test integration-test python-sdk-smoke install uninstall clean fetch_meta gitleaks

all: test

fetch_meta:
	python3 scripts/fetch_meta.py

build: fetch_meta
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

build-shared-darwin: fetch_meta
	mkdir -p dist/darwin
	CGO_ENABLED=1 GOOS=darwin GOARCH=$$(go env GOARCH) go build -trimpath -buildmode=c-shared -ldflags "$(LDFLAGS)" -o dist/darwin/liblarkcli.dylib ./cshared

build-shared-linux: fetch_meta
	mkdir -p dist/linux
	CGO_ENABLED=1 GOOS=linux GOARCH=$$(go env GOARCH) go build -trimpath -buildmode=c-shared -ldflags "$(LDFLAGS)" -o dist/linux/liblarkcli.so ./cshared

build-shared-catalog: fetch_meta
	go run ./tools/dump_command_catalog > python_sdk/lark_cli_sdk/command_catalog.json

vet: fetch_meta
	go vet ./...

unit-test: fetch_meta
	go test -race -gcflags="all=-N -l" -count=1 ./cmd/... ./internal/... ./shortcuts/...

integration-test: build
	go test -v -count=1 ./tests/...

test: vet unit-test integration-test

python-sdk-smoke: fetch_meta
	@set -e; \
	if [ "$$(uname -s)" = "Darwin" ]; then \
		$(MAKE) build-shared-darwin; \
		LIB_PATH=$$(pwd)/dist/darwin/liblarkcli.dylib; \
	else \
		$(MAKE) build-shared-linux; \
		LIB_PATH=$$(pwd)/dist/linux/liblarkcli.so; \
	fi; \
	PYTHONPATH=python_sdk LARK_CLI_SHARED_LIB=$$LIB_PATH python3 -m unittest python_sdk/tests/test_client_smoke.py

install: build
	install -d $(PREFIX)/bin
	install -m755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "OK: $(PREFIX)/bin/$(BINARY) ($(VERSION))"

uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)

# Run secret-leak checks locally before pushing.
# Step 1: check-doc-tokens catches realistic-looking example tokens in reference
#         docs and asks you to use _EXAMPLE_TOKEN placeholders instead.
# Step 2: gitleaks scans the full repo for real leaked secrets.
# Install gitleaks: https://github.com/gitleaks/gitleaks#installing
gitleaks:
	@bash scripts/check-doc-tokens.sh
	@command -v gitleaks >/dev/null 2>&1 || { echo "gitleaks not found. Install: brew install gitleaks"; exit 1; }
	gitleaks detect --redact -v --exit-code=2
