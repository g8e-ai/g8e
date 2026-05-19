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
BUILD_DIR := build
BLOB_NAMESPACE := operator-binary
OPERATOR_URL := https://localhost:9000

# =============================================================================
# ARCHITECTURES
# =============================================================================
ARCHS := amd64 arm64 386

# =============================================================================
# TOOLS
# =============================================================================
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || ([ -f "$$(go env GOPATH)/bin/golangci-lint" ] && echo "$$(go env GOPATH)/bin/golangci-lint") || ([ -f "./bin/golangci-lint" ] && echo "./bin/golangci-lint"))
GOVULNCHECK := $(shell command -v govulncheck 2>/dev/null || ([ -f "$$(go env GOPATH)/bin/govulncheck" ] && echo "$$(go env GOPATH)/bin/govulncheck"))

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
	@echo "  build-local       Build linux/amd64 locally and upload to operator"
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
	@echo "  vulncheck     Run govulncheck"
	@echo "  check         Run fmt, vet, lint, vulncheck, test"
	@echo ""
	@echo "Tool Install:"
	@echo "  install-tools  Install golangci-lint and govulncheck"
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
	@mkdir -p build/linux-amd64
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-buildvcs=false \
		-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/amd64'" \
		-o build/linux-amd64/g8e.operator ./cmd/g8eo
	@echo 'Built: build/linux-amd64/g8e.operator'
	@SIZE=$$(ls -lh build/linux-amd64/g8e.operator | awk '{print $$5}') && \
		echo "  Size: $$SIZE"

.PHONY: build-all
build-all:
	@echo "Building Operator (all architectures)..."
	@for arch in amd64 arm64 386; do \
		echo "Building linux/$$arch..."; \
		mkdir -p $(BUILD_DIR)/linux-$$arch && \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build \
			-trimpath \
			-buildvcs=false \
			-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/$$arch'" \
			-o $(BUILD_DIR)/linux-$$arch/g8e.operator ./cmd/g8eo || exit 1; \
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

# =============================================================================
# BUILD TARGETS — local Go
# =============================================================================
.PHONY: build-local
build-local: _build-local-binary

.PHONY: _build-local-binary
_build-local-binary:
	@echo "Building Operator for local testing (linux/amd64)..."
	@mkdir -p build/linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build \
		-trimpath \
		-buildvcs=false \
		-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/amd64'" \
		-o build/linux-amd64/$(BINARY_NAME) ./cmd/g8eo
	@echo "Built: build/linux-amd64/$(BINARY_NAME)"


.PHONY: build-local-all
build-local-all:
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
build-linux-amd64:
	$(MAKE) _build-release ARCH=amd64

.PHONY: build-linux-arm64
build-linux-arm64:
	$(MAKE) _build-release ARCH=arm64

.PHONY: build-linux-386
build-linux-386:
	$(MAKE) _build-release ARCH=386

.PHONY: _build-release
_build-release:
	@echo "Building linux/$(ARCH)..."
	@mkdir -p build/linux-$(ARCH)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) $(GO) build \
		-trimpath \
		-buildvcs=false \
		-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/$(ARCH)'" \
		-o build/linux-$(ARCH)/$(BINARY_NAME) ./cmd/g8eo

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
	@if [ -z "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint not found. Run 'make install-tools' to install it locally."; \
		exit 1; \
	fi
	@$(GOLANGCI_LINT) run

.PHONY: vulncheck
vulncheck:
	@if [ -z "$(GOVULNCHECK)" ]; then \
		echo "govulncheck not found. Run 'make install-tools' to install it."; \
		exit 1; \
	fi
	@$(GOVULNCHECK) ./...

.PHONY: check
check: fmt vet lint vulncheck test

# =============================================================================
# TOOL INSTALL
# =============================================================================
.PHONY: install-tools
install-tools:
	@echo "Installing development tools..."
	@mkdir -p bin
	@if [ -z "$$(command -v golangci-lint)" ] && [ ! -f "./bin/golangci-lint" ]; then \
		echo "Installing golangci-lint (building from source for Go 1.25 compatibility)..."; \
		$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.0; \
	fi
	@if [ -z "$$(command -v govulncheck)" ]; then \
		echo "Installing govulncheck..."; \
		$(GO) install golang.org/x/vuln/cmd/govulncheck@latest; \
	fi
	@echo "Tools installed successfully."

# =============================================================================
# UTILITY
# =============================================================================
.PHONY: clean
clean:
	@rm -rf build/
	@rm -f $(BINARY_NAME) coverage.out coverage.html *.test

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
