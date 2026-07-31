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
GOTOOLCHAIN ?= auto
export GOTOOLCHAIN
TMPDIR ?= /tmp
.DEFAULT_GOAL := help

# =============================================================================
# BUILD VARIABLES
# =============================================================================
VERSION := $(shell cat VERSION | tr -d '\n')
BUILD_ID := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
BIN_DIR := bin
MAIN_PKG := ./cmd/g8e
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
STRIP_FLAGS := -s -w
TRIMPATH := -trimpath
BUILD_TAGS := netgo,osusergo

# FIPS 140-3 build configuration.
# GOFIPS140 is a BUILD-TIME setting consumed by the go toolchain (not the
# running binary). Pinning to v1.0.0 links the Go Cryptographic Module
# (CMVP Cert #5247, CAVP A6650) and enables FIPS 140-3 approved mode by
# default — the binary enters approved mode on startup and runs its
# integrity/CAST self-tests at init, with no runtime env var required.
# v1.26.0 (frozen from Go 1.26, CAVP A8028) is also available in Go 1.26.5
# but is still Pending Review on the CMVP Modules-In-Process list and must
# NOT be used for a compliance claim.
# The FIPS compliance claim is restricted to linux/amd64 (the tested OE).
GOFIPS140_VERSION := v1.0.0
FIPS_GOOS := linux
FIPS_GOARCH := amd64

# Test configuration
TEST_TIMEOUT := 180s
# Per-package deadlock backstop for unit tests. Kept generous because `-race`
# slows the pure-Go SQLite (modernc) used by DB-heavy packages (e.g.
# internal/services/gateway), and CI runs the whole module in parallel, starving
# any one package of CPU. 60s was too tight and produced flaky "test timed out"
# failures; a real hang still trips this well before 180s.
TEST_SHORT_TIMEOUT := 180s
TEST_RACE := $(if $(filter windows,$(HOST_OS)),,-race)
TEST_COUNT := -count=1
COVERAGE_THRESHOLD := 75

# =============================================================================
# TEST & COVERAGE EXCLUSIONS — single source of truth
# =============================================================================
# Packages excluded from test runs (and implicitly from coverage too).
# Each pattern is matched against Go import paths.
TEST_EXCLUDE_PKGS := \
	mocks \
	/test/ \
	/cmd/g8e \
	/internal/protocol/proto \
	/internal/interfaces \
	/internal/constants \
	/internal/contracts \
	/internal/httpclient \
	/internal/models \
	/internal/testutil \
	/internal/tools/chaos \
	/internal/tools/agent_harness/scenarios \
	/internal/services/gateway/docs \
	/internal/services/gateway/scripts \
	/internal/services/storage/storagetest

# Packages excluded from the coverage profile but NOT from test discovery.
# These compile and may be tested, but their statements should not affect
# the coverage threshold (e.g. generated protobuf code, example programs).
COVERAGE_ONLY_EXCLUDE_PKGS := \
	g8e-ai/g8e/protocol/

# All packages excluded from coverage: test exclusions + coverage-only exclusions.
COVERAGE_EXCLUDE_PKGS := $(TEST_EXCLUDE_PKGS) $(COVERAGE_ONLY_EXCLUDE_PKGS)

# Files excluded from coverage only (belong to otherwise-tested packages).
EXCLUDE_FILES := \
	internal/cli/cmd/demos.go \
	internal/cli/cmd/demo_dhs.go \
	internal/cli/cmd/demo_finance.go \
	internal/cli/cmd/demo_gov.go \
	internal/cli/cmd/demo_healthcare.go \
	internal/cli/cmd/mcp_backup.go

# Grep chains derived from the lists above — do not edit directly.
_TEST_PKG_GREP := $(foreach p,$(TEST_EXCLUDE_PKGS),| grep -v "$(p)")
_COV_PKG_GREP  := $(foreach p,$(COVERAGE_EXCLUDE_PKGS),| grep -v "$(p)")
_FILE_GREP     := $(foreach f,$(EXCLUDE_FILES),| grep -v "$(f)")
_COV_GREP      := $(_COV_PKG_GREP) $(_FILE_GREP)

# Packages passed to go test.
TEST_PKGS := $$(go list ./... $(_TEST_PKG_GREP))

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
	@echo "Release:"
	@echo "  release          Tag v<VERSION> + protocol/v<VERSION>, push, create GitHub release"
	@echo ""
	@echo "Protocol Generation:"
	@echo "  generate      Generate all protocol artifacts (proto)"
	@echo "  proto         Generate all Protobuf code (Go)"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  protoc-install Install protoc compiler (optional; buf does not require it)"
	@echo ""
	@echo "Build:"
	@echo "  build			Build g8e for current OS and architecture"
	@echo "  build-all			Build g8e for all platforms (linux, windows, darwin)"
	@echo "  build-linux		Build g8e for Linux (amd64, arm64, 386)"
	@echo "  build-windows		Build g8e for Windows (amd64, arm64)"
	@echo "  build-darwin		Build g8e for Darwin (amd64, arm64)"
	@echo "  build-compressed	Build g8e then compress with UPX (requires upx installed)"
	@echo "  build-docker		Build g8e binary in Docker (linux/amd64)"
	@echo "  build-linux-docker		Build g8e for Linux in Docker (amd64, arm64, 386)"
	@echo "  build-windows-docker	Build g8e for Windows in Docker (amd64, arm64)"
	@echo "  build-darwin-docker		Build g8e for Darwin in Docker (amd64, arm64)"
	@echo "  build-all-docker		Build g8e for all platforms in Docker"
	@echo "  build-fips		Build g8e with FIPS 140-3 approved mode (linux/amd64, GOFIPS140=v1.0.0)"
	@echo "  verify-fips		Build the FIPS variant and run the g8e version --fips self-check"
	@echo ""
	@echo "Test:"
	@echo "  test                  Run all tests (unit + integration)"
	@echo "  test-coverage         Run tests with coverage (enforces $(COVERAGE_THRESHOLD)% threshold). Use PKG=./path/to/pkg for specific package, VERBOSE=true for verbose output"
	@echo "  test-integration      Run Tier 2 (In-Process Integration) tests - no external dependencies"
	@echo "  test-docker           Run Tier 3 (Docker E2E) tests - requires Docker"
	@echo ""
	@echo "Lint & Quality:"
	@echo "  lint          Run all linting and quality checks"
	@echo "  vulncheck     Run Operator vulnerability check"
	@echo "  validate-doctrines Validate doctrine JSON schema"
	@echo "  swagger-generate Generate Swagger/OpenAPI documentation from code annotations"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean         Remove all build artifacts and runtime state"
	@echo ""
	@echo "Demos:"
	@echo "  demo-verify         Build and run all 6 demo environments (requires Docker)"
	@echo ""
	@echo "Python Protocol:"
	@echo "  python-build  Copy constants and build the Python protocol package"

.PHONY: python-build
python-build:
	@echo "Building Python protocol package..."
	@mkdir -p protocol/python/g8e/_data
	@cp protocol/constants/*.json protocol/python/g8e/_data/
	@cp -r protocol/constants/doctrine protocol/python/g8e/_data/
	@cd protocol/python && python -m build
	@echo "Python package built. Check protocol/python/dist/"

# =============================================================================
# PROTOCOL GENERATION
# =============================================================================
.PHONY: generate
generate: proto


# Note: buf has its own built-in compiler (protocompile) and invokes the
# protoc-gen-* plugins directly, so the standalone protoc binary is NOT required.
.PHONY: proto
proto: buf-install
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
#
# NOTE: protoc-install is OPTIONAL. `make proto` uses buf, which ships its own
# compiler and does not require the standalone protoc binary. This target exists
# only for manual use (e.g. invoking protoc directly for debugging).
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
		PROTOC_VER=$$(echo "$(PROTOC_VERSION)" | sed 's/^v//'); \
		case "$(HOST_OS)" in \
			linux)   PROTOC_OS=linux ;; \
			darwin)  PROTOC_OS=osx ;; \
			windows) PROTOC_OS=win64 ;; \
			*) echo "Error: unsupported OS $(HOST_OS) for protoc install" >&2; exit 1 ;; \
		esac; \
		case "$(HOST_ARCH)" in \
			amd64) PROTOC_ARCH=x86_64 ;; \
			arm64) PROTOC_ARCH=aarch_64 ;; \
			*) echo "Error: unsupported arch $(HOST_ARCH) for protoc install" >&2; exit 1 ;; \
		esac; \
		if [ "$(HOST_OS)" = "windows" ]; then PROTOC_ASSET="protoc-$$PROTOC_VER-win64.zip"; \
		else PROTOC_ASSET="protoc-$$PROTOC_VER-$$PROTOC_OS-$$PROTOC_ARCH.zip"; fi; \
		cd $(TMPDIR) && curl -fSL "https://github.com/protocolbuffers/protobuf/releases/download/$(PROTOC_VERSION)/$$PROTOC_ASSET" -o protoc.zip && \
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
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@NODE_BINARY=$(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH); \
	if [ "$(HOST_OS)" = "windows" ]; then \
		NODE_BINARY=$$NODE_BINARY.exe; \
		ROOT_COPY=g8e.exe; \
	else \
		ROOT_COPY=g8e; \
	fi; \
	echo "Building $(HOST_OS)/$(HOST_ARCH) -> $$NODE_BINARY..."; \
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=$(HOST_OS)_$(HOST_ARCH)" -o $$NODE_BINARY $(MAIN_PKG); \
	sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	if [ -f "./$$ROOT_COPY" ] && pgrep -f "$$ROOT_COPY --doctrine" > /dev/null 2>&1; then \
		echo "Error: Unable to copy binary - g8e gateway is currently running. Please stop it first with: ./$$ROOT_COPY gw stop"; \
		exit 1; \
	fi; \
	cp $$NODE_BINARY $$ROOT_COPY; \
	mkdir -p demos/bin; \
	cp $$ROOT_COPY demos/bin/g8e
	@echo "Build complete. Binary: $(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH)$(if $(filter windows,$(HOST_OS)),.exe,)"
	@echo "Demo binary: demos/bin/g8e"

.PHONY: build-compressed
build-compressed: build
	@echo "Compressing binary with UPX..."
	@if ! command -v upx &> /dev/null; then \
		echo "Error: UPX is not installed. Install it with:"; \
		echo "  Debian/Ubuntu: sudo apt-get install upx-ucl"; \
		echo "  macOS:         brew install upx"; \
		echo "  Arch:          sudo pacman -S upx"; \
		exit 1; \
	fi
	@BINARY=$(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH); \
	if [ "$(HOST_OS)" = "windows" ]; then \
		BINARY=$$BINARY.exe; \
	fi; \
	upx --best --lzma $$BINARY; \
	echo "Compressed binary: $$BINARY"

.PHONY: build-all
build-all:
	@echo "Building g8e Operator for all platforms..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		NODE_BINARY=$(BIN_DIR)/g8e-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then \
			NODE_BINARY=$$NODE_BINARY.exe; \
		fi; \
		echo "Building $$platform -> $$NODE_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$$GOOS GOARCH=$$GOARCH go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=$$platform" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Multi-platform build complete. Checksums: $(BIN_DIR)/g8e-*.sha256"

.PHONY: build-darwin
build-darwin:
	@echo "Building g8e for Darwin..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@for arch in $(DARWIN_ARCHS); do \
		NODE_BINARY=$(BIN_DIR)/g8e-darwin-$$arch; \
		echo "Building darwin/$$arch -> $$NODE_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=$$arch go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=darwin_$$arch" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Darwin build complete. Binaries: $(BIN_DIR)/g8e-darwin-*"

.PHONY: build-linux
build-linux:
	@echo "Building g8e for Linux..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@for arch in $(LINUX_ARCHS); do \
		NODE_BINARY=$(BIN_DIR)/g8e-linux-$$arch; \
		echo "Building linux/$$arch -> $$NODE_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=$$arch go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=linux_$$arch" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Linux build complete. Binaries: $(BIN_DIR)/g8e-linux-*"

.PHONY: build-windows
build-windows:
	@echo "Building g8e for Windows..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@for arch in $(WINDOWS_ARCHS); do \
		NODE_BINARY=$(BIN_DIR)/g8e-windows-$$arch.exe; \
		echo "Building windows/$$arch -> $$NODE_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=$$arch go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=windows_$$arch" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Windows build complete. Binaries: $(BIN_DIR)/g8e-windows-*.exe"

.PHONY: build-docker
build-docker:
	@echo "Building g8e binary in Docker (linux/amd64)..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@docker run --rm -e GOOS=linux -e GOARCH=amd64 -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -trimpath -tags netgo,osusergo -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/g8e && cp /build/g8e /out/g8e-linux-amd64"
	@sha256sum $(BIN_DIR)/g8e-linux-amd64 > $(BIN_DIR)/g8e-linux-amd64.sha256
	@echo "Docker build complete. Binary: $(BIN_DIR)/g8e-linux-amd64"

.PHONY: build-linux-docker
build-linux-docker:
	@echo "Building g8e for Linux in Docker..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@for arch in $(LINUX_ARCHS); do \
		echo "Building linux/$$arch in Docker..."; \
		docker run --rm -e GOOS=linux -e GOARCH=$$arch -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -trimpath -tags netgo,osusergo -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/g8e && cp /build/g8e /out/g8e-linux-$$arch"; \
		sha256sum $(BIN_DIR)/g8e-linux-$$arch > $(BIN_DIR)/g8e-linux-$$arch.sha256; \
	done
	@echo "Linux Docker build complete. Binaries: $(BIN_DIR)/g8e-linux-*"

.PHONY: build-windows-docker
build-windows-docker:
	@echo "Building g8e for Windows in Docker..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@for arch in $(WINDOWS_ARCHS); do \
		echo "Building windows/$$arch in Docker..."; \
		docker run --rm -e GOOS=windows -e GOARCH=$$arch -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -trimpath -tags netgo,osusergo -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/g8e && cp /build/g8e /out/g8e-windows-$$arch.exe"; \
		sha256sum $(BIN_DIR)/g8e-windows-$$arch.exe > $(BIN_DIR)/g8e-windows-$$arch.exe.sha256; \
	done
	@echo "Windows Docker build complete. Binaries: $(BIN_DIR)/g8e-windows-*.exe"

.PHONY: build-darwin-docker
build-darwin-docker:
	@echo "Building g8e for Darwin in Docker..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@DOCKER_BUILDKIT=1 docker build --target builder -t g8e-builder:$(VERSION) .
	@for arch in $(DARWIN_ARCHS); do \
		echo "Building darwin/$$arch in Docker..."; \
		docker run --rm -e GOOS=darwin -e GOARCH=$$arch -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -trimpath -tags netgo,osusergo -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/g8e && cp /build/g8e /out/g8e-darwin-$$arch"; \
		sha256sum $(BIN_DIR)/g8e-darwin-$$arch > $(BIN_DIR)/g8e-darwin-$$arch.sha256; \
	done
	@echo "Darwin Docker build complete. Binaries: $(BIN_DIR)/g8e-darwin-*"

.PHONY: build-all-docker
build-all-docker:
	@echo "Building g8e for all platforms in Docker..."
	@gofmt -w .
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
		docker run --rm -e GOOS=$$GOOS -e GOARCH=$$GOARCH -v $(PWD)/$(BIN_DIR):/out g8e-builder:$(VERSION) sh -c "CGO_ENABLED=0 GOOS=\$$GOOS GOARCH=\$$GOARCH go build -trimpath -tags netgo,osusergo -ldflags \"-s -w -X main.version=\$$(cat VERSION) -X main.buildID=\$$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=\$$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=\$${GOOS}_\$$GOARCH\" -o /build/g8e ./cmd/g8e && cp /build/g8e /out/$$NODE_BINARY"; \
		sha256sum $(BIN_DIR)/$$NODE_BINARY > $(BIN_DIR)/$$NODE_BINARY.sha256; \
	done
	@echo "All-platform Docker build complete. Binaries: $(BIN_DIR)/g8e-*"

# FIPS 140-3 build variant.
# Produces a linux/amd64 binary linked against the Go Cryptographic Module
# v1.0.0 (CMVP Cert #5247) with FIPS 140-3 approved mode enabled by default.
# GOFIPS140 is set at BUILD TIME ONLY — the resulting binary enters approved
# mode on startup without any runtime env var. See:
#   https://go.dev/doc/security/fips140
# Verify the deployed binary with: ./g8e version --fips
.PHONY: build-fips
build-fips:
	@echo "Building g8e with FIPS 140-3 approved mode (GOFIPS140=$(GOFIPS140_VERSION), $(FIPS_GOOS)/$(FIPS_GOARCH))..."
	@gofmt -w .
	@mkdir -p $(BIN_DIR)
	@NODE_BINARY=$(BIN_DIR)/g8e-fips-$(FIPS_GOOS)-$(FIPS_GOARCH); \
	echo "Building $(FIPS_GOOS)/$(FIPS_GOARCH) -> $$NODE_BINARY..."; \
	GOFIPS140=$(GOFIPS140_VERSION) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(FIPS_GOOS) GOARCH=$(FIPS_GOARCH) \
		go build $(TRIMPATH) -tags $(BUILD_TAGS) \
		-ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=$(FIPS_GOOS)_$(FIPS_GOARCH)" \
		-o $$NODE_BINARY $(MAIN_PKG); \
	sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	cp $$NODE_BINARY g8e-fips
	@echo "FIPS build complete. Binary: $(BIN_DIR)/g8e-fips-$(FIPS_GOOS)-$(FIPS_GOARCH)"
	@echo "Verify with: ./g8e-fips version --fips"

# Quick FIPS self-check: build the FIPS variant and confirm the binary reports
# FIPS 140-3 approved mode is active via the native crypto/fips140 module API.
# Exits non-zero if the self-check fails. Intended for CI and release gates.
.PHONY: verify-fips
verify-fips: build-fips
	@echo "Verifying FIPS 140-3 approved mode in the built binary..."
	@./g8e-fips version --fips
	@echo "FIPS 140-3 self-check passed."

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
	@go test -p=1 -tags=!integration $(TEST_RACE) $(TEST_COUNT) -timeout $(TEST_SHORT_TIMEOUT) $(TEST_PKGS)


# Tier 2: In-Process Integration Tests - no external dependencies
.PHONY: test-integration
test-integration:
	@echo "Running Tier 2 (In-Process Integration) tests..."
	@go test -p=1 -tags=integration $(TEST_RACE) $(TEST_COUNT) -timeout $(TEST_TIMEOUT) ./...

# Tier 3: Docker E2E Tests - requires Docker
.PHONY: test-docker
test-docker:
	@echo "Running Tier 3 (Docker E2E) tests..."
	@go test -tags=e2e $(TEST_RACE) $(TEST_COUNT) -timeout 300s ./test/e2e/...


# Air-Gap Verification: verify vendored build works without network access
.PHONY: test-airgap
test-airgap:
	@echo "Running air-gap verification..."
	@echo "  1. Verifying vendor directories exist..."
	@test -d vendor/ || { echo "ERROR: vendor/ directory missing — run 'go mod vendor'"; exit 1; }
	@echo "  2. Building with vendored modules (-mod=vendor)..."
	@go build -mod=vendor ./... || { echo "ERROR: vendored build failed"; exit 1; }
	@echo "  3. Verifying images.json manifest exists..."
	@test -f demos/images.json || { echo "ERROR: demos/images.json missing"; exit 1; }
	@echo "  4. Checking compose files have no unpinned image references..."
	@! grep -rn 'image:.*:latest\|image:.*:alpine\|image:.*:slim\|image:.*:bookworm' demos/*/compose.yml || { echo "ERROR: found unpinned image references in compose files"; exit 1; }
	@echo "  5. Verifying no pip install or requests imports remain in demos..."
	@! grep -rn 'pip install\|import requests' demos/ --include='*.py' || { echo "ERROR: found pip install or requests import in demo Python files"; exit 1; }
	@echo "Air-gap verification PASSED."

# =============================================================================
# DEMO VERIFICATION
# =============================================================================
# Requires Docker. Builds the binary, then runs all 6 demo environments.
# Each demo is torn down (with volumes) before the next starts to avoid
# port conflicts and stale PKI state.
DEMO_ORGS := healthcare gov finance dhs fedramp frontend

.PHONY: demo-verify
demo-verify: build
	@echo "=== demo-verify: running all $(words $(DEMO_ORGS)) demos ==="
	@for org in $(DEMO_ORGS); do \
		echo ""; \
		echo "========================================================"; \
		echo "  Demo: $$org"; \
		echo "========================================================"; \
		./g8e demos stop $$org 2>/dev/null || true; \
		docker compose -f demos/$$org/compose.yml down -v --remove-orphans 2>/dev/null || true; \
		if ! ./g8e demos run $$org; then \
			echo "FAIL: demo $$org did not pass all scenarios"; \
			exit 1; \
		fi; \
		./g8e demos stop $$org 2>/dev/null || true; \
		docker compose -f demos/$$org/compose.yml down -v --remove-orphans 2>/dev/null || true; \
		echo "PASS: demo $$org completed successfully"; \
	done
	@echo ""; \
	echo "========================================================"; \
	echo "  All $(words $(DEMO_ORGS)) demos PASSED"; \
	echo "========================================================"

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
		$$SWAG_CMD init --dir cmd/g8e,internal/services/gateway,internal/models,internal/constants --output internal/services/gateway/docs --outputTypes json,yaml --parseInternal --parseDependencyLevel 1 --packagePrefix github.com/g8e-ai/g8e,github.com/go-webauthn,encoding/json 2>/dev/null; \
	else \
		echo "swag not found, installing via go install..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
		$$(go env GOPATH)/bin/swag init --dir cmd/g8e,internal/services/gateway,internal/models,internal/constants --output internal/services/gateway/docs --outputTypes json,yaml --parseInternal --parseDependencyLevel 1 --packagePrefix github.com/g8e-ai/g8e,github.com/go-webauthn,encoding/json 2>/dev/null; \
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
	@rm -rf .g8e-test-tmp/
	@rm -rf bin/
	@rm -f *.sha256 *.test coverage.out coverage_filtered.out buf
	@rm -rf .g8e-harness-*/
	@GOTOOLCHAIN=local go clean -cache
	@GOTOOLCHAIN=local go clean -modcache
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

# =============================================================================
# RELEASE
# =============================================================================
# VERSION is the single source of truth. `make release` syncs pyproject.toml
# and __init__.py from VERSION (if needed), tags the current commit as
# v<VERSION> + protocol/v<VERSION>, and pushes both tags. The GitHub Actions
# workflows create the GitHub release and upload binary assets.
#
# Workflow:
#   1. Merge PRs (CI enforces version sync, proto/swagger generation, tests)
#   2. git pull origin main
#   3. make release        (tags, pushes — workflows create the release)
#
# The protocol/v* tag triggers the Python PyPI release workflow.
# The Go module is part of the root module (github.com/g8e-ai/g8e)
# and is versioned by the v* tag. External consumers use:
#   go get github.com/g8e-ai/g8e@vX.Y.Z
# The protocol/v* tag is NOT used for Go module versioning.

.PHONY: release
release:
	@VERSION=$$(cat VERSION | tr -d '\n' | sed 's/^v//'); \
	TAG="v$$VERSION"; \
	PYTHON_TAG="protocol/v$$VERSION"; \
	MAJOR_MINOR=$$(echo $$VERSION | cut -d. -f1-2); \
	NOTES_FILE="docs/release_notes/v$$MAJOR_MINOR.x/v$$VERSION.md"; \
	echo "=== release: $$TAG ==="; \
	\
	PY_FILE=protocol/python/pyproject.toml; \
	PY_INIT=protocol/python/g8e/__init__.py; \
	PY_VERSION=$$(grep -E '^version = ' $$PY_FILE | head -1 | sed -E 's/.*"([^"]+)".*/\1/'); \
	PY_INIT_VERSION=$$(grep -E '^__version__ = ' $$PY_INIT | head -1 | sed -E 's/.*"([^"]+)".*/\1/'); \
	if [ "$$PY_VERSION" != "$$VERSION" ]; then \
		echo "Syncing $$PY_FILE: $$PY_VERSION -> $$VERSION"; \
		sed -i.bak -E 's/^version = "[^"]+"/version = "'$$VERSION'"/' $$PY_FILE; \
		rm -f $$PY_FILE.bak; \
		echo "  pyproject.toml synced."; \
	else \
		echo "  pyproject.toml already in sync."; \
	fi; \
	if [ "$$PY_INIT_VERSION" != "$$VERSION" ]; then \
		echo "Syncing $$PY_INIT: $$PY_INIT_VERSION -> $$VERSION"; \
		sed -i.bak -E 's/^__version__ = "[^"]+"/__version__ = "'$$VERSION'"/' $$PY_INIT; \
		rm -f $$PY_INIT.bak; \
		echo "  __init__.py synced."; \
	else \
		echo "  __init__.py already in sync."; \
	fi; \
	\
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is dirty after version sync."; \
		echo "Python versions were out of sync. Commit synced files, push, merge PR, then retry."; exit 1; \
	fi; \
	if [ ! -f "$$NOTES_FILE" ]; then \
		echo "Error: release notes file $$NOTES_FILE not found."; exit 1; \
	fi; \
	if git rev-parse -q --verify "refs/tags/$$TAG" >/dev/null; then \
		echo "Error: tag $$TAG already exists."; exit 1; \
	fi; \
	if git rev-parse -q --verify "refs/tags/$$PYTHON_TAG" >/dev/null; then \
		echo "Error: tag $$PYTHON_TAG already exists."; exit 1; \
	fi; \
	echo "Tagging $$TAG + $$PYTHON_TAG..."; \
	git tag "$$TAG" && git tag "$$PYTHON_TAG"; \
	git push origin "$$TAG" && git push origin "$$PYTHON_TAG"; \
	\
	echo ""; \
	echo "Tags $$TAG + $$PYTHON_TAG pushed."; \
	echo "The GitHub Actions workflows will create the release and upload assets."; \
	echo "Monitor: gh run watch --workflow=release-binary.yml"; \
	echo "Monitor: gh run watch --workflow=release-python-protocol.yml"
