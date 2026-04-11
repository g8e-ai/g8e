# g8eo Operator Makefile

SHELL := /bin/bash
.DEFAULT_GOAL := help

# =============================================================================
# =============================================================================
# Use go1.24 if available (local dev), otherwise fall back to system go (Docker/CI)
GO := $(shell command -v go1.24 2>/dev/null || ([ -x "$$(go env GOPATH)/bin/go1.24" ] && echo "$$(go env GOPATH)/bin/go1.24") || command -v go 2>/dev/null || echo "go")

# =============================================================================
# VERSION
# =============================================================================
VERSION := $(shell cat ../../VERSION 2>/dev/null | tr -d '[:space:]' || echo 'v0.0.0')

# =============================================================================
# PATHS AND NAMES
# =============================================================================
BINARY_NAME := g8e.operator
CERTS_DIR := certs
G8ED_SSL_CA_CERT := ../../ssl/ca.crt
BUILD_DIR := /home/g8e/build
BLOB_NAMESPACE := operator-binary
G8ES_URL := https://g8es:9000
CA_CERT := /g8es/ca.crt
AUTH_TOKEN := $(shell cat /g8es/internal_auth_token 2>/dev/null | tr -d ' \n\r')

# =============================================================================
# ARCHITECTURES
# =============================================================================
ARCHS := amd64 arm64 386

# =============================================================================
# HELP
# =============================================================================
.PHONY: help
help:
	@echo "g8eo Operator Makefile"
	@echo ""
	@echo "Build (g8ep container — no local Go required):"
	@echo "  build             Build linux/amd64 inside g8ep container"
	@echo "  build-all         Build all architectures inside g8ep container (CI)"
	@echo ""
	@echo "Build (local Go):"
	@echo "  build-local       Build linux/amd64 locally and upload to g8es"
	@echo "  build-local-all   Build all Linux architectures locally (release)"
	@echo "  build-linux-amd64 Build linux/amd64 only"
	@echo "  build-linux-arm64 Build linux/arm64 only"
	@echo "  build-linux-386   Build linux/386 only"
	@echo ""
	@echo "Quality:"
	@echo "  test          Run tests"
	@echo "  test-sudo     Run tests with sudo (for privileged operations)"
	@echo "  fmt           Format code"
	@echo "  vet           Run go vet"
	@echo "  lint          Run golangci-lint"
	@echo "  check         Run fmt, vet, lint, test"
	@echo ""
	@echo "Certs:"
	@echo "  clean-certs     Remove certificates"
	@echo ""
	@echo "Utility:"
	@echo "  clean            Clean build artifacts"
	@echo "  clean-go-cache   Clean Go build cache (reclaim disk space)"
	@echo "  clean-deep       Deep clean including all caches"
	@echo "  deps          Update dependencies"
	@echo "  version       Show version info"

# =============================================================================
# BUILD TARGETS — g8ep container
# =============================================================================
.PHONY: build
build:
	@echo "Building Operator (linux/amd64)..."
	@mkdir -p certs
	@[ -f /g8es/ssl/ca.crt ] && cp /g8es/ssl/ca.crt certs/server_ca.crt || true
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-buildvcs=false \
		-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/amd64'" \
		-o /home/g8e/g8e.operator .
	@echo 'Built: /home/g8e/g8e.operator'
	@SIZE=$$(ls -lh /home/g8e/g8e.operator | awk '{print $$5}') && \
		echo "  Size: $$SIZE"
	@$(MAKE) _upload-blob ARCH=amd64 BIN_PATH=/home/g8e/g8e.operator

.PHONY: build-all
build-all:
	@echo "Building Operator (all architectures)..."
	@mkdir -p certs
	@[ -f /g8es/ssl/ca.crt ] && cp /g8es/ssl/ca.crt certs/server_ca.crt || true
	@for arch in amd64 arm64 386; do \
		echo "Building linux/$$arch..."; \
		mkdir -p $(BUILD_DIR)/linux-$$arch && \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build \
			-trimpath \
			-buildvcs=false \
			-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/$$arch'" \
			-o $(BUILD_DIR)/linux-$$arch/g8e.operator . || exit 1; \
		SIZE=$$(ls -lh $(BUILD_DIR)/linux-$$arch/g8e.operator | awk '{print $$5}'); \
		echo "  linux/$$arch: $$SIZE (uncompressed)"; \
	done
	@if command -v upx >/dev/null 2>&1; then \
		echo "Compressing binaries with upx..."; \
		for arch in amd64 arm64 386; do \
			upx --best --lzma $(BUILD_DIR)/linux-$$arch/g8e.operator || exit 1; \
			SIZE=$$(ls -lh $(BUILD_DIR)/linux-$$arch/g8e.operator | awk '{print $$5}'); \
			echo "  linux/$$arch: $$SIZE (compressed)"; \
		done; \
	else \
		echo "UPX not available - skipping compression (binaries are already stripped)"; \
	fi
	@echo ""
	@echo "All architectures built successfully."
	@echo "Uploading binaries to g8es blob store..."
	@for arch in amd64 arm64 386; do \
		$(MAKE) _upload-blob ARCH=$$arch BIN_PATH=$(BUILD_DIR)/linux-$$arch/g8e.operator || exit 1; \
	done
	@echo "All binaries uploaded to g8es blob store."

# =============================================================================
# BUILD TARGETS — local Go
# =============================================================================
.PHONY: build-local
build-local: check-certs _build-local-binary
	@$(MAKE) _upload-blob ARCH=amd64 BIN_PATH=build/linux-amd64/$(BINARY_NAME)

.PHONY: _build-local-binary
_build-local-binary:
	@echo "Building Operator for local testing (linux/amd64)..."
	@mkdir -p build/linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build \
		-trimpath \
		-buildvcs=false \
		-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/amd64'" \
		-o build/linux-amd64/$(BINARY_NAME) .
	@echo "Built: build/linux-amd64/$(BINARY_NAME)"

.PHONY: _upload-blob
_upload-blob:
	@if [ -z "$(AUTH_TOKEN)" ]; then \
		echo "  WARNING: No internal auth token found — skipping blob upload"; \
	else \
		SIZE=$$(ls -lh $(BIN_PATH) | awk '{print $$5}'); \
		HTTP_CODE=$$(curl -sf -o /dev/null -w '%{http_code}' \
			-X PUT \
			--cacert $(CA_CERT) \
			-H 'Content-Type: application/octet-stream' \
			-H 'X-Internal-Auth: $(AUTH_TOKEN)' \
			--data-binary @$(BIN_PATH) \
			'$(G8es_URL)/blob/$(BLOB_NAMESPACE)/linux-$(ARCH)'); \
		if [ "$$HTTP_CODE" = "200" ] || [ "$$HTTP_CODE" = "201" ]; then \
			echo "  Uploaded linux/$(ARCH) to g8es blob store ($$SIZE)"; \
		else \
			echo "  ERROR: Upload failed for linux/$(ARCH) (HTTP $$HTTP_CODE)" >&2; \
			exit 1; \
		fi; \
	fi

.PHONY: build-local-all
build-local-all: check-certs
	@echo "Building Operator for all Linux platforms..."
	@for arch in $(ARCHS); do \
		$(MAKE) _build-release ARCH=$$arch; \
	done
	@echo ""
	@echo "Build sizes:"
	@for arch in $(ARCHS); do \
		if [ -f "build/linux-$$arch/$(BINARY_NAME)" ]; then \
			size=$$(ls -lh "build/linux-$$arch/$(BINARY_NAME)" | awk '{print $$5}'); \
			printf "  linux/%-6s %s\n" "$$arch:" "$$size"; \
		fi; \
	done
	@echo ""
	@echo "Build complete"

.PHONY: build-linux-amd64
build-linux-amd64: check-certs
	$(MAKE) _build-release ARCH=amd64

.PHONY: build-linux-arm64
build-linux-arm64: check-certs
	$(MAKE) _build-release ARCH=arm64

.PHONY: build-linux-386
build-linux-386: check-certs
	$(MAKE) _build-release ARCH=386

.PHONY: _build-release
_build-release:
	@echo "Building linux/$(ARCH)..."
	@mkdir -p build/linux-$(ARCH)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) $(GO) build \
		-trimpath \
		-buildvcs=false \
		-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/$(ARCH)'" \
		-o build/linux-$(ARCH)/$(BINARY_NAME) .

# =============================================================================
# CERTIFICATE TARGETS
# =============================================================================
.PHONY: check-certs
check-certs:
	@if [ ! -f "$(G8ED_SSL_CA_CERT)" ]; then \
		echo "ERROR: g8ed server CA not found at $(G8ED_SSL_CA_CERT)"; \
		echo "  Run: scripts/security/generate-certs.sh"; \
		echo "  Or:  scripts/core/build.sh --generate-ssl"; \
		exit 1; \
	fi
	@mkdir -p $(CERTS_DIR)
	@cp $(G8ED_SSL_CA_CERT) $(CERTS_DIR)/server_ca.crt
	@echo "Synced g8ed server CA to $(CERTS_DIR)/server_ca.crt"

.PHONY: clean-certs
clean-certs:
	@rm -f $(CERTS_DIR)/server_ca.crt
	@echo "Removed $(CERTS_DIR)/server_ca.crt (source at $(G8ED_SSL_CA_CERT) is unchanged)"

# =============================================================================
# CODE QUALITY
# =============================================================================
.PHONY: test
test:
	@$(GO) test -race -timeout 180s ./...

.PHONY: test-sudo
test-sudo:
	@sudo $(GO) test -race -timeout 180s ./...

.PHONY: fmt
fmt:
	@gofmt -s -w .

.PHONY: vet
vet:
	@$(GO) vet ./...

.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found"; \
		exit 1; \
	fi

.PHONY: check
check: fmt vet lint test

# =============================================================================
# UTILITY
# =============================================================================
.PHONY: clean
clean:
	@rm -rf build/
	@rm -f $(BINARY_NAME) coverage.out coverage.html

.PHONY: clean-go-cache
clean-go-cache:
	@echo "Go build cache before: $$(du -sh $$($(GO) env GOCACHE) 2>/dev/null | cut -f1)"
	@$(GO) clean -cache
	@echo "Go build cache cleared"

.PHONY: clean-deep
clean-deep: clean clean-go-cache
	@$(GO) clean -testcache -modcache
	@echo "All caches cleared"

.PHONY: deps
deps:
	@$(GO) get -u ./...
	@$(GO) mod tidy
	@$(GO) mod vendor

.PHONY: version
version:
	@echo "Operator version: $(VERSION)"
