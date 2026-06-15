# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# g8e Platform Root Makefile
# Industry standard orchestration for multi-component proto generation and builds.

SHELL := /bin/bash
export PATH := $(HOME)/go/bin:$(PATH)
TMPDIR ?= /tmp
.DEFAULT_GOAL := help

# =============================================================================
# BUILD VARIABLES
# =============================================================================
VERSION := $(shell cat VERSION | tr -d '\n')
BUILD_ID := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
BIN_DIR := bin
MAIN_PKG := ./cmd/operator
LDFLAGS := -X main.version=$(VERSION) -X main.buildID=$(BUILD_ID) -X main.buildTime=$(BUILD_TIME)
HOST_OS := $(shell go env GOOS)
HOST_ARCH := $(shell go env GOARCH)

# Platform and architecture lists
PLATFORMS := linux/amd64 linux/arm64 linux/386 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64
LINUX_ARCHS := amd64 arm64 386
DARWIN_ARCHS := amd64 arm64
WINDOWS_ARCHS := amd64 arm64

# Build flags
CGO_ENABLED := 0
WINDOWS_EXTRA_FLAGS := -s -w

# Test configuration
TEST_TIMEOUT := 180s
TEST_SHORT_TIMEOUT := 60s
TEST_RACE := $(if $(filter windows,$(HOST_OS)),,-race)
TEST_COUNT := -count=1
COVERAGE_THRESHOLD := 65

# =============================================================================
# COVERAGE EXCLUSIONS — single source of truth
# =============================================================================
# Packages excluded from both test runs and coverage profile.
# Each pattern is matched against Go import paths and coverage profile file paths.
EXCLUDE_PKGS := \
	mocks \
	/cmd/ \
	/internal/test \
	/test/ \
	/internal/protocol/proto \
	/internal/contracts \
	/internal/interfaces \
	/internal/services/gateway/docs \
	/internal/services/gateway/scripts \
	/internal/services/storage/storagetest

# Files excluded from coverage only (belong to otherwise-tested packages).
EXCLUDE_FILES := internal/cli/cmd/demos.go

# Grep chains derived from the lists above — do not edit directly.
_PKG_GREP  := $(foreach p,$(EXCLUDE_PKGS),| grep -v "$(p)")
_FILE_GREP := $(foreach f,$(EXCLUDE_FILES),| grep -v "$(f)")
_COV_GREP  := $(_PKG_GREP) $(_FILE_GREP)

# Packages passed to go test.
TEST_PKGS := $$(go list ./... $(_PKG_GREP))

# Filter coverage.out (the raw profile) to remove excluded paths, then report %.
# We operate on the profile data — not on the formatted output of go tool cover.
FILTER_PROFILE = { head -1 coverage.out; tail -n +2 coverage.out $(_COV_GREP); } > coverage_filtered.out
COVERAGE_PCT   = go tool cover -func=coverage_filtered.out | tail -1 | awk '{print $$3}' | sed 's/%//'

# =============================================================================
# TOOLS
# =============================================================================
# Protocol buffer tool versions - update these when upgrading tools
PROTOC_VERSION := v35.0
PROTOC_GEN_GO_VERSION := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.6.2
PROTOC_GEN_DOC_VERSION := v1.5.1
PROTOC_MIN_VERSION := 21

BUF := $(shell command -v buf 2>/dev/null || echo "./buf")
PROTOC := $(shell command -v protoc 2>/dev/null || echo "/usr/local/bin/protoc")
PROTOC_GEN_GO := $(shell go list -m -f '{{.Version}}' google.golang.org/protobuf 2>/dev/null || echo "$(PROTOC_GEN_GO_VERSION)")

# =============================================================================
# HELP
# =============================================================================
.PHONY: help
help:
	@echo "g8e Platform Root Makefile"
	@echo ""
	@echo "Note: On Windows, use build.ps1 instead of make"
	@echo ""
	@echo "CI/CD (Local):"
	@echo "  ci            Run full CI pipeline locally (mirrors GitHub Actions)"
	@echo "  ci-platform   Run platform-only CI (operator, protocol, proto, docs)"
	@echo ""
	@echo "Protocol Generation:"
	@echo "  generate      Generate all protocol artifacts (proto)"
	@echo "  proto         Generate all Protobuf code (Go)"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  protoc-install Install protoc compiler"
	@echo ""
	@echo "Build:"
	@echo "  build			Build g8e for current OS and architecture"
	@echo "  build-all			Build g8e for all platforms (linux, windows, darwin)"
	@echo "  build-linux		Build g8e for Linux (amd64, arm64, 386)"
	@echo "  build-windows		Build g8e for Windows (amd64, arm64)"
	@echo "  build-darwin		Build g8e for Darwin (amd64, arm64)"
	@echo "  build-docker		Build g8e binary in Docker (linux/amd64)"
	@echo "  build-linux-docker		Build g8e for Linux in Docker (amd64, arm64, 386)"
	@echo "  build-windows-docker	Build g8e for Windows in Docker (amd64, arm64)"
	@echo "  build-darwin-docker		Build g8e for Darwin in Docker (amd64, arm64)"
	@echo "  build-all-docker		Build g8e for all platforms in Docker"
	@echo ""
	@echo "Test:"
	@echo "  test                  Run all tests (unit + integration)"
	@echo "  test-short            Run short tests with race detection"
	@echo "  test-pkg-<path>       Run tests for a specific package (e.g., make test-pkg-internal/services/auth)"
	@echo "  test-coverage         Run tests with coverage (enforces 60% threshold). Use PKG=./path/to/pkg for specific package, VERBOSE=true for verbose output"
	@echo "  test-shuffle          Run all tests with randomized order"
	@echo "  test-integration      Run Tier 2 (In-Memory Integration) tests - no external dependencies"
	@echo "  test-docker           Run Tier 3 (Docker E2E) tests - requires Docker"
	@echo "  test-gov              Run Tier 3 (Gov Demo E2E) tests - requires Docker"
	@echo "  test-gateway          Run gateway-specific integration tests"
	@echo "  test-mcp              Run MCP integration tests (legacy - redirects to test-integration)"
	@echo "  test-a2a              Run A2A integration tests (legacy - redirects to test-integration)"
	@echo "  test-universal-gateway Run universal gateway integration tests (legacy - redirects to test-integration)"
	@echo "  test-byo              Run BYO client integration tests (legacy - redirects to test-integration)"
	@echo "  test-native           Run native tool integration tests (legacy - redirects to test-integration)"
	@echo "  test-scenario         Run scenario integration tests (legacy - redirects to test-integration)"
	@echo ""
	@echo "Lint & Quality:"
	@echo "  lint          Run all linting and quality checks"
	@echo "  vulncheck     Run Operator vulnerability check"
	@echo "  validate-doctrines Validate doctrine JSON schema"
	@echo "  swagger-generate Generate Swagger/OpenAPI documentation from code annotations"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean         Remove all build artifacts and runtime state"

# =============================================================================
# PROTOCOL GENERATION
# =============================================================================
.PHONY: generate
generate: proto


.PHONY: proto
proto: buf-install protoc-install
	@if command -v buf &> /dev/null || [ -f "./buf" ]; then \
		echo "Generating Go Protobuf code with Buf..."; \
		$(BUF) generate protocol/proto; \
	else \
		echo "Error: Buf not found. Network access required for initial setup." >&2; \
		exit 1; \
	fi
	@echo "Protobuf generation complete."

.PHONY: proto-python
proto-python:
	@echo "Generating Python Protobuf code..."
	@if command -v python3 &> /dev/null; then \
		python3 -m grpc_tools.protoc \
			--python_out=protocol/python/g8e_protocol \
			--proto_path=protocol/proto \
			protocol/proto/g8e/common/v1/common.proto \
			protocol/proto/g8e/operator/v1/operator.proto \
			protocol/proto/g8e/pubsub/v1/pubsub.proto; \
	else \
		echo "Error: python3 not found. Install grpc_tools.protoc." >&2; \
		exit 1; \
	fi
	@echo "Python Protobuf generation complete."

.PHONY: proto-force
proto-force: buf-install
	@echo "Force generating Protobuf code..."
	@$(BUF) generate protocol/proto
	@echo "Protobuf generation complete."

# =============================================================================
# TOOL INSTALLATION
# =============================================================================
.PHONY: buf-install
buf-install:
	@if ! command -v buf &> /dev/null && [ ! -f "./buf" ]; then \
		if command -v go &> /dev/null; then \
			echo "Installing Buf natively via Go toolchain..."; \
			GOBIN=$(HOME)/go/bin go install github.com/bufbuild/buf/cmd/buf@v1.70.0; \
		else \
			echo "Go not found, attempting direct download..."; \
			curl -sSL "https://github.com/bufbuild/buf/releases/latest/download/buf-$$(uname -s)-$$(uname -m)" -o ./buf && chmod +x ./buf || \
			echo "Warning: Failed to download Buf. Proceeding with existing protocol files if available."; \
		fi \
	fi

.PHONY: protoc-install
protoc-install:
	@if ! command -v protoc &> /dev/null; then \
		echo "Installing protoc $(PROTOC_VERSION)..."; \
		cd $(TMPDIR) && curl -fSL https://github.com/protocolbuffers/protobuf/releases/download/$(PROTOC_VERSION)/protoc-35.0-linux-x86_64.zip -o protoc.zip && \
		unzip -o protoc.zip -d protoc && \
		sudo cp protoc/bin/protoc /usr/local/bin/protoc && \
		sudo chmod +x /usr/local/bin/protoc && \
		rm -rf $(TMPDIR)/protoc $(TMPDIR)/protoc.zip; \
	fi
	@echo "Verifying protoc version compatibility..."
	@PROTOC_VERSION=$$($(PROTOC) --version | grep -oE '[0-9]+\.[0-9]+'); \
	PROTOC_MAJOR=$$(echo $$PROTOC_VERSION | cut -d. -f1); \
	PROTOC_MIN=$(PROTOC_MIN_VERSION); \
	if [ "$$PROTOC_MAJOR" -lt "$$PROTOC_MIN" ]; then \
		echo "Error: protoc version $$PROTOC_VERSION is too old. Minimum required: $(PROTOC_MIN_VERSION)"; \
		exit 1; \
	fi
	@echo "protoc version $$PROTOC_VERSION is compatible."

# =============================================================================
# BUILD
# =============================================================================

.PHONY: build
build:
	@echo "Building g8e Operator for current platform..."
	@mkdir -p $(BIN_DIR)
	@NODE_BINARY=$(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH); \
	if [ "$(HOST_OS)" = "windows" ]; then \
		NODE_BINARY=$$NODE_BINARY.exe; \
		ROOT_COPY=g8e.exe; \
	else \
		ROOT_COPY=g8e; \
	fi; \
	echo "Building $(HOST_OS)/$(HOST_ARCH) -> $$NODE_BINARY..."; \
	if [ "$(HOST_OS)" = "windows" ]; then \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -ldflags "$(LDFLAGS) $(WINDOWS_EXTRA_FLAGS) -X main.platform=$(HOST_OS)_$(HOST_ARCH)" -o $$NODE_BINARY $(MAIN_PKG); \
	else \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -ldflags "$(LDFLAGS) -X main.platform=$(HOST_OS)_$(HOST_ARCH)" -o $$NODE_BINARY $(MAIN_PKG); \
	fi; \
	sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	if [ -f "./$$ROOT_COPY" ] && pgrep -f "$$ROOT_COPY --doctrine" > /dev/null 2>&1; then \
		echo "Error: Unable to copy binary - g8e gateway is currently running. Please stop it first with: ./$$ROOT_COPY gw stop"; \
		exit 1; \
	fi; \
	cp $$NODE_BINARY $$ROOT_COPY
	@echo "Build complete. Binary: $(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH)$(if $(filter windows,$(HOST_OS)),.exe,)"

.PHONY: build-all
build-all:
	@echo "Building g8e Operator for all platforms..."
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		NODE_BINARY=$(BIN_DIR)/g8e-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then \
			NODE_BINARY=$$NODE_BINARY.exe; \
		fi; \
		echo "Building $$platform -> $$NODE_BINARY..."; \
		if [ "$$GOOS" = "windows" ]; then \
			CGO_ENABLED=$(CGO_ENABLED) GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "$(LDFLAGS) $(WINDOWS_EXTRA_FLAGS) -X main.platform=$$platform" -o $$NODE_BINARY $(MAIN_PKG); \
		else \
			CGO_ENABLED=$(CGO_ENABLED) GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "$(LDFLAGS) -X main.platform=$$platform" -o $$NODE_BINARY $(MAIN_PKG); \
		fi; \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Multi-platform build complete. Checksums: $(BIN_DIR)/g8e-*.sha256"

.PHONY: build-darwin
build-darwin:
	@echo "Building g8e for Darwin..."
	@mkdir -p $(BIN_DIR)
	@for arch in $(DARWIN_ARCHS); do \
		NODE_BINARY=$(BIN_DIR)/g8e-darwin-$$arch; \
		echo "Building darwin/$$arch -> $$NODE_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=$$arch go build -ldflags "$(LDFLAGS) -X main.platform=darwin_$$arch" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Darwin build complete. Binaries: $(BIN_DIR)/g8e-darwin-*"

.PHONY: build-linux
build-linux:
	@echo "Building g8e for Linux..."
	@mkdir -p $(BIN_DIR)
	@for arch in $(LINUX_ARCHS); do \
		NODE_BINARY=$(BIN_DIR)/g8e-linux-$$arch; \
		echo "Building linux/$$arch -> $$NODE_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=$$arch go build -ldflags "$(LDFLAGS) -X main.platform=linux_$$arch" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Linux build complete. Binaries: $(BIN_DIR)/g8e-linux-*"

.PHONY: build-windows
build-windows:
	@echo "Building g8e for Windows..."
	@mkdir -p $(BIN_DIR)
	@for arch in $(WINDOWS_ARCHS); do \
		NODE_BINARY=$(BIN_DIR)/g8e-windows-$$arch.exe; \
		echo "Building windows/$$arch -> $$NODE_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=$$arch go build -ldflags "$(LDFLAGS) $(WINDOWS_EXTRA_FLAGS) -X main.platform=windows_$$arch" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Windows build complete. Binaries: $(BIN_DIR)/g8e-windows-*.exe"

.PHONY: build-docker
build-docker:
	@echo "Building g8e binary in Docker (linux/amd64)..."
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@docker run --rm -e GOOS=linux -e GOARCH=amd64 -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/operator && cp /build/g8e /out/g8e-linux-amd64"
	@sha256sum $(BIN_DIR)/g8e-linux-amd64 > $(BIN_DIR)/g8e-linux-amd64.sha256
	@echo "Docker build complete. Binary: $(BIN_DIR)/g8e-linux-amd64"

.PHONY: build-linux-docker
build-linux-docker:
	@echo "Building g8e for Linux in Docker..."
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@for arch in $(LINUX_ARCHS); do \
		echo "Building linux/$$arch in Docker..."; \
		docker run --rm -e GOOS=linux -e GOARCH=$$arch -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/operator && cp /build/g8e /out/g8e-linux-$$arch"; \
		sha256sum $(BIN_DIR)/g8e-linux-$$arch > $(BIN_DIR)/g8e-linux-$$arch.sha256; \
	done
	@echo "Linux Docker build complete. Binaries: $(BIN_DIR)/g8e-linux-*"

.PHONY: build-windows-docker
build-windows-docker:
	@echo "Building g8e for Windows in Docker..."
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@for arch in $(WINDOWS_ARCHS); do \
		echo "Building windows/$$arch in Docker..."; \
		docker run --rm -e GOOS=windows -e GOARCH=$$arch -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/operator && cp /build/g8e /out/g8e-windows-$$arch.exe"; \
		sha256sum $(BIN_DIR)/g8e-windows-$$arch.exe > $(BIN_DIR)/g8e-windows-$$arch.exe.sha256; \
	done
	@echo "Windows Docker build complete. Binaries: $(BIN_DIR)/g8e-windows-*.exe"

.PHONY: build-darwin-docker
build-darwin-docker:
	@echo "Building g8e for Darwin in Docker..."
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@for arch in $(DARWIN_ARCHS); do \
		echo "Building darwin/$$arch in Docker..."; \
		docker run --rm -e GOOS=darwin -e GOARCH=$$arch -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/operator && cp /build/g8e /out/g8e-darwin-$$arch"; \
		sha256sum $(BIN_DIR)/g8e-darwin-$$arch > $(BIN_DIR)/g8e-darwin-$$arch.sha256; \
	done
	@echo "Darwin Docker build complete. Binaries: $(BIN_DIR)/g8e-darwin-*"

.PHONY: build-all-docker
build-all-docker:
	@echo "Building g8e for all platforms in Docker..."
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		NODE_BINARY=g8e-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then \
			NODE_BINARY=$$NODE_BINARY.exe; \
		fi; \
		echo "Building $$platform in Docker..."; \
		docker run --rm -e GOOS=$$GOOS -e GOARCH=$$GOARCH -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/operator && cp /build/g8e /out/$$NODE_BINARY"; \
		sha256sum $(BIN_DIR)/$$NODE_BINARY > $(BIN_DIR)/$$NODE_BINARY.sha256; \
	done
	@echo "All-platform Docker build complete. Binaries: $(BIN_DIR)/g8e-*"

# =============================================================================
# TEST
# =============================================================================
# Core test targets
.PHONY: test
test: test-unit test-integration
	@echo "All tests completed successfully."

# Unit Tests: Run immediately without any build tags (excludes integration and e2e)
.PHONY: test-unit
test-unit:
	@echo "Running Tier 1 (Unit) tests..."
	@go test $(TEST_RACE) $(TEST_COUNT) -timeout $(TEST_SHORT_TIMEOUT) $(TEST_PKGS)

.PHONY: test-short
test-short:
	@echo "Running short unit tests (skips long-running tests)..."
	@go test $(TEST_RACE) -short $(TEST_COUNT) -timeout $(TEST_SHORT_TIMEOUT) $(TEST_PKGS)

# Tier 2: In-Memory Integration Tests - no external dependencies
.PHONY: test-integration
test-integration:
	@echo "Running Tier 2 (In-Memory Integration) tests..."
	@go test -tags=integration $(TEST_RACE) $(TEST_COUNT) -timeout $(TEST_TIMEOUT) ./...

# Tier 3a: Docker E2E Tests - requires Docker
.PHONY: test-docker
test-docker:
	@echo "Running Tier 3 (Docker E2E) tests..."
	@go test -tags=e2e $(TEST_RACE) $(TEST_COUNT) -timeout 300s ./test/e2e/...

# Tier 3b: Gov Demo E2E Tests - requires Docker
.PHONY: test-gov
test-gov:
	@echo "Running Tier 3 (Gov Demo E2E) tests..."
	@go test -tags=e2e -run TestDockerGateway_GovDemo $(TEST_RACE) $(TEST_COUNT) -timeout 300s ./test/e2e/...

# Legacy targets - redirect to honest names
.PHONY: test-mcp test-a2a test-byo test-native test-scenario test-universal-gateway
test-mcp test-a2a test-byo test-native test-scenario test-universal-gateway:
	@echo "Running integration tests (legacy target)..."
	@go test -tags=integration $(TEST_RACE) $(TEST_COUNT) -timeout $(TEST_TIMEOUT) ./...

# Gateway tests (subset of integration tests)
.PHONY: test-gateway
test-gateway:
	@echo "Running gateway-specific tests (no platform required)..."
	@go test -tags=integration $(TEST_RACE) $(TEST_COUNT) -timeout $(TEST_TIMEOUT) ./test/a2a_gateway_test.go ./test/mcp_gateway_test.go ./test/mcp_stdio_test.go

# Coverage tests
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage (threshold: $(COVERAGE_THRESHOLD)%)..."
	@go test -tags=integration $(TEST_RACE) -timeout $(TEST_TIMEOUT) \
		-coverprofile=coverage.out -covermode=atomic \
		$(if $(VERBOSE),-v,) \
		$(if $(PKG),$(PKG),$(TEST_PKGS))
	@$(FILTER_PROFILE)
	@COVERAGE=$$($(COVERAGE_PCT)); \
	if [ $$(echo "$$COVERAGE < $(COVERAGE_THRESHOLD)" | bc -l) -eq 1 ]; then \
		echo "Coverage $$COVERAGE% is below $(COVERAGE_THRESHOLD)% threshold"; \
		exit 1; \
	fi; \
	echo "Coverage $$COVERAGE% meets $(COVERAGE_THRESHOLD)% threshold"

# =============================================================================
# LINT & QUALITY
# =============================================================================
.PHONY: lint
lint: lint-no-embedded-newlines vulncheck validate-doctrines swagger-generate
	@golangci-lint run
	@echo "All linting and quality checks complete."

.PHONY: lint-no-embedded-newlines
lint-no-embedded-newlines:
	@echo "Checking for compilation errors (including embedded newlines)..."
	@go build ./... || { echo "Error: Go build failed. This may be caused by embedded newlines or other syntax errors."; exit 1; }
	@echo "Build successful - no embedded newlines or syntax errors detected."

.PHONY: vulncheck
vulncheck:
	@govulncheck ./...

.PHONY: validate-doctrines
validate-doctrines:
	@echo "Validating doctrine JSON schema..."
	@for file in protocol/constants/doctrine/*.json; do \
		if [ -f "$$file" ]; then \
			echo "Validating $$file..."; \
			python3 -m json.tool "$$file" > /dev/null || exit 1; \
		fi \
	done
	@echo "All doctrine files are valid JSON."

.PHONY: swagger-generate
swagger-generate:
	@echo "Generating Swagger/OpenAPI documentation..."
	@if command -v swag &> /dev/null || [ -f "$$(go env GOPATH)/bin/swag" ]; then \
		SWAG_CMD=$$(command -v swag 2>/dev/null || echo "$$(go env GOPATH)/bin/swag"); \
		$$SWAG_CMD init --dir cmd/operator,internal/services/gateway,internal/models,internal/constants --output internal/services/gateway/docs --parseInternal; \
	else \
		echo "swag not found, installing via go install..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
		$$(go env GOPATH)/bin/swag init --dir cmd/operator,internal/services/gateway,internal/models,internal/constants --output internal/services/gateway/docs --parseInternal; \
	fi
	@echo "Swagger documentation generated successfully."


# =============================================================================
# DOCTRINE MANAGEMENT
# =============================================================================
.PHONY: ingest-doctrines
ingest-doctrines:
	@echo "Doctrine ingestion scripts removed. Use manual ingestion if needed."

.PHONY: update-doctrines
update-doctrines:
	@echo "Updating doctrine sources..."
	@if [ -d "$(TMPDIR)/coreruleset" ]; then \
		cd $(TMPDIR)/coreruleset && git pull; \
	else \
		git clone --depth 1 https://github.com/coreruleset/coreruleset.git $(TMPDIR)/coreruleset; \
	fi
	@curl -sSL https://raw.githubusercontent.com/gitleaks/gitleaks/master/config/gitleaks.toml -o $(TMPDIR)/gitleaks.toml
	@$(MAKE) ingest-doctrines
	@echo "Doctrine update complete."

# =============================================================================
# CLEANUP
# =============================================================================
.PHONY: clean
clean:
	@echo "Cleaning up build artifacts and runtime state..."
	@rm -rf .g8e/
	@rm -rf bin/
	@rm -f *.sha256 *.test coverage.out coverage_filtered.out buf
	@rm -rf .g8e-harness-*/
	@echo "Clean complete."

.PHONY: clean-harness
clean-harness:
	@echo "Cleaning up stale harness directories..."
	@rm -rf .g8e-harness-*/
	@echo "Clean complete."


# =============================================================================
# HELPER FUNCTIONS
# =============================================================================


# =============================================================================
# CI/CD (LOCAL)
# =============================================================================
.PHONY: ci
ci: ci-platform
	@echo "CI complete."

.PHONY: ci-platform
ci-platform: _ci-verify-proto _ci-swagger _ci-lint _ci-vulncheck _ci-test
	@echo "Platform CI complete."

.PHONY: _ci-verify-proto
_ci-verify-proto:
	@echo "=== verify-proto ==="
	@$(MAKE) protoc-install
	@$(MAKE) proto
	@CHANGES=$$(git status --porcelain | grep -E "^\s*M.*\.pb\.go$$|^\s*M.*\.proto$$" || true); \
	if [ -n "$$CHANGES" ]; then \
		echo "Error: Generated proto files are out of sync with protocol/proto/*.proto"; \
		echo "$$CHANGES"; \
		git diff -- $$(git status --porcelain | grep -E "^\s*M" | awk '{print $$2}'); \
		exit 1; \
	fi
	@$(MAKE) validate-doctrines

.PHONY: _ci-swagger
_ci-swagger:
	@echo "=== swagger ==="
	@$(MAKE) swagger-generate
	@CHANGES=$$(git status --porcelain | grep -E "^\s*M.*internal/services/gateway/docs/" || true); \
	if [ -n "$$CHANGES" ]; then \
		echo "Error: Generated swagger files are out of sync with code annotations"; \
		echo "$$CHANGES"; \
		git diff -- $$(git status --porcelain | grep -E "^\s*M" | awk '{print $$2}'); \
		exit 1; \
	fi

.PHONY: _ci-lint
_ci-lint:
	@echo "=== lint ==="
	@$(MAKE) lint

.PHONY: _ci-vulncheck
_ci-vulncheck:
	@echo "=== vulncheck ==="
	@$(MAKE) vulncheck

.PHONY: _ci-test
_ci-test:
	@echo "=== test ==="
	@G8E_STRICT_CONSTANTS_LINT=1 go test $(TEST_RACE) -timeout $(TEST_TIMEOUT) \
		-coverprofile=coverage.out -covermode=atomic $(TEST_PKGS)
	@$(FILTER_PROFILE)
	@COVERAGE=$$($(COVERAGE_PCT)); \
	if [ $$(echo "$$COVERAGE < $(COVERAGE_THRESHOLD)" | bc -l) -eq 1 ]; then \
		echo "Coverage $$COVERAGE% is below $(COVERAGE_THRESHOLD)% threshold"; \
		exit 1; \
	fi; \
	echo "Coverage $$COVERAGE% meets $(COVERAGE_THRESHOLD)% threshold"
