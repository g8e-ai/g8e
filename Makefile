# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

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
# Integration tests: the gateway package alone has 996 integration tests that
# exercise real SQLite (modernc, pure-Go) via setupTestHTTPHandler. Under `-race`
# on a 2-vCPU CI runner, race-detector background threads compete with test
# goroutines for CPU, producing a ~3x slowdown versus multicore local machines
# (~95s local → ~285s CI). 180s was too tight and caused spurious "test timed
# out" panics where the victim test had only just started (0s elapsed). 360s
# gives ~25% headroom over the heaviest package; a genuine hang still trips this.
TEST_TIMEOUT := 360s
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
	/test/ \
	/cmd/g8e \
	/internal/constants \
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
	g8e/v2/protocol/proto \
	g8e/v2/protocol/examples \
	adapters/lattice/gen \
	node_modules

# All packages excluded from coverage: test exclusions + coverage-only exclusions.
COVERAGE_EXCLUDE_PKGS := $(TEST_EXCLUDE_PKGS) $(COVERAGE_ONLY_EXCLUDE_PKGS)

# Files excluded from coverage only (belong to otherwise-tested packages).
EXCLUDE_FILES := \
	internal/cli/cmd/demos.go \
	internal/cli/cmd/demo_dhs.go \
	internal/cli/cmd/demo_finance.go \
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
	@echo "  build-fips		Build g8e with FIPS 140-3 approved mode (linux/amd64, GOFIPS140=v1.0.0)"
	@echo "  verify-fips		Build the FIPS variant and run the g8e version --fips self-check"
	@echo "  fmt			Format all Go source files (gofmt -w .)"
	@echo ""
	@echo "Test:"
	@echo "  test                  Run all tests (unit + integration)"
	@echo "  test-coverage         Run tests with coverage (enforces $(COVERAGE_THRESHOLD)% threshold). Use PKG=./path/to/pkg for specific package, VERBOSE=true for verbose output"
	@echo "  test-integration      Run Tier 2 (In-Process Integration) tests - no external dependencies"
	@echo "  test-docker           Run Tier 3 (Docker E2E) steady-state tests against an approved platform"
	@echo ""
	@echo "Lint & Quality:"
	@echo "  lint          Run all linting and quality checks"
	@echo "  vulncheck     Run Operator vulnerability check"
	@echo "  validate-doctrines Validate doctrine JSON schema"
	@echo "  validate-cosais     Validate COSAiS overlay coverage (Phase 8 CI guard)"
	@echo "  swagger-generate Generate Swagger/OpenAPI documentation from code annotations"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean         Remove all build artifacts and runtime state"
	@echo "  clean-docker  Stop all profile containers and remove volumes (--profile bootstrapped down -v --remove-orphans)"
	@echo ""
	@echo "Docker Compose:"
	@echo "  up            Build and start the full stack (docker compose up -d --build)"
	@echo "  down          Stop all profile containers, keep volumes (--profile bootstrapped down --remove-orphans)"
	@echo ""
	@echo "Demos:"
	@echo "  demo-verify         Build and run all 5 demo environments (requires Docker)"
	@echo ""
	@echo "Python Protocol:"
	@echo "  python-build  Copy constants and build the Python protocol package"
	@echo ""
	@echo "Ensemble (g8ee):"
	@echo "  ensemble-test   Run the ensemble pytest unit + in-process integration suite (Tier 1 + Tier 2)"
	@echo "  evals-test      Run standalone eval Tier 1 + Tier 2 tests"
	@echo "  evals-test-unit Run standalone eval Tier 1 tests"
	@echo "  evals-test-integration Run standalone eval Tier 2 tests"
	@echo "  test-external   Run the ensemble external test suite (Tier 4: real LLM/API, gated on credentials)"
	@echo "  ensemble-lint   Run ruff + pyright on the ensemble"
	@echo "  evals-lint      Run ruff + pyright on the standalone eval package"
	@echo "  build-ensemble  Build the ensemble Docker image"
	@echo ""
	@echo "Dashboard (g8ed):"
	@echo "  dashboard-test   Run the dashboard vitest suite (requires npm ci in dashboard/)"
	@echo "  build-dashboard  Build the dashboard Docker image"

.PHONY: python-build
python-build:
	@echo "Building Python protocol package..."
	@mkdir -p protocol/python/g8e/_data
	@cp protocol/constants/*.json protocol/python/g8e/_data/
	@cp -r protocol/constants/doctrine protocol/python/g8e/_data/
	@cd protocol/python && uv build
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
	@if ! $(PYTHON) -c "import grpc_tools" &> /dev/null; then \
		echo "Error: grpc_tools not found in $(PYTHON). Install with: pip install grpcio-tools" >&2; \
		exit 1; \
	fi
	@mkdir -p protocol/python/g8e_protocol
	@$(PYTHON) -m grpc_tools.protoc \
		--python_out=protocol/python/g8e_protocol \
		--proto_path=protocol/proto \
		protocol/proto/g8e/common/v1/common.proto \
		protocol/proto/g8e/compliance/v1/compliance.proto \
		protocol/proto/g8e/operator/v1/operator.proto \
		protocol/proto/g8e/pubsub/v1/pubsub.proto
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
	@echo "Building g8e Operator for all platforms (FIPS 140-3 for linux)..."
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		NODE_BINARY=$(BIN_DIR)/g8e-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then \
			NODE_BINARY=$$NODE_BINARY.exe; \
		fi; \
		echo "Building $$platform -> $$NODE_BINARY..."; \
		if [ "$$GOOS" = "linux" ]; then \
			FIPS_ENV="GOFIPS140=$(GOFIPS140_VERSION)"; \
		else \
			FIPS_ENV="-u GOFIPS140"; \
		fi; \
		env $$FIPS_ENV CGO_ENABLED=$(CGO_ENABLED) GOOS=$$GOOS GOARCH=$$GOARCH go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=$$platform" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@HOST_NODE_BINARY=$(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH); \
	if [ "$(HOST_OS)" = "windows" ]; then \
		HOST_NODE_BINARY=$$HOST_NODE_BINARY.exe; \
		ROOT_COPY=g8e.exe; \
	else \
		ROOT_COPY=g8e; \
	fi; \
	if [ -f "./$$ROOT_COPY" ] && pgrep -f "$$ROOT_COPY --doctrine" > /dev/null 2>&1; then \
		echo "Error: Unable to copy host binary - g8e gateway is currently running. Please stop it first with: ./$$ROOT_COPY gw stop"; \
		exit 1; \
	fi; \
	cp $$HOST_NODE_BINARY $$ROOT_COPY; \
	mkdir -p demos/bin; \
	cp $$ROOT_COPY demos/bin/g8e
	@echo "Multi-platform build complete. Checksums: $(BIN_DIR)/g8e-*.sha256"
	@echo "Host binary copied: ./g8e ($(HOST_OS)/$(HOST_ARCH))"
	@echo "Demo binary: demos/bin/g8e"

.PHONY: build-darwin
build-darwin:
	@echo "Building g8e for Darwin..."
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
	@mkdir -p $(BIN_DIR)
	@for arch in $(WINDOWS_ARCHS); do \
		NODE_BINARY=$(BIN_DIR)/g8e-windows-$$arch.exe; \
		echo "Building windows/$$arch -> $$NODE_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=$$arch go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=windows_$$arch" -o $$NODE_BINARY $(MAIN_PKG); \
		sha256sum $$NODE_BINARY > $$NODE_BINARY.sha256; \
	done
	@echo "Windows build complete. Binaries: $(BIN_DIR)/g8e-windows-*.exe"

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
# FIPS 140-3 approved mode AND enforcement are active via the native
# crypto/fips140 module API. GODEBUG=fips140=only is set at runtime to switch
# the module into enforcement mode (rejecting non-approved primitives);
# GOFIPS140 alone only enables approved mode (GODEBUG defaults to fips140=on).
# Exits non-zero if the self-check fails. Intended for CI and release gates.
.PHONY: verify-fips
verify-fips: build-fips
	@echo "Verifying FIPS 140-3 approved mode and enforcement in the built binary..."
	@GODEBUG=fips140=only ./g8e-fips version --fips
	@echo "FIPS 140-3 self-check passed."

# Format all Go source files. Build targets no longer format the working tree;
# run this explicitly or wire it into a pre-commit hook.
.PHONY: fmt
fmt:
	@gofmt -w .
	@echo "Formatting complete."

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

# Tier 3: Docker E2E Tests - requires a running platform.
# Start the platform first (docker compose up or ./g8e gw start), approve all
# enrollment requests, then run this target. The test binary connects to the
# running platform and fails fast if it is not reachable. Per docs/devs/devs.md,
# platform tests run through ./g8e test, never go test directly.
#
# The default target runs the steady-state suite: tests that exercise an
# approved stack (gateway, auth, operator registry, heartbeat, command
# roundtrip, ensemble, dashboard, compliance, approved-restart). Stateful
# scenario tests (pending-discovery, denial, restart-during-pending, headless)
# require specific platform states and are run individually via:
#   ./g8e test e2e --run TestPlatformEnrollment_PendingDiscovery
#   ./g8e test e2e --run TestPlatformEnrollment_Denial
#   ./g8e test e2e --run TestPlatformEnrollment_RestartDuringPending
#   ./g8e test e2e --run TestPlatformEnrollment_Headless
.PHONY: test-docker
test-docker:
	@echo "Running Tier 3 (Docker E2E) steady-state tests..."
	@./g8e test e2e --run 'TestGateway|TestAuth|TestOperatorRegistry|TestPubSub|TestCommandRoundtrip|TestEnsemble|TestDashboard|TestCompliance|TestApprovedRestart'


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
# Requires Docker. Builds the binary, then runs all 5 demo environments.
# Each demo is torn down (with volumes) before the next starts to avoid
# port conflicts and stale PKI state.
DEMO_ORGS := healthcare finance dhs fedramp frontend

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

# =============================================================================
# ENSEMBLE (g8ee) — Python first-party component
# =============================================================================
# The ensemble depends on the in-tree protocol/python package. Install it first:
#   pip install -e protocol/python
#   pip install -e 'ensemble[test]'
# The targets prefer the repo-root .venv if present (development), falling back
# to system python3 (CI installs protocol/python + ensemble into system python).

PYTHON := $(shell if [ -f .venv/bin/python ]; then echo $(CURDIR)/.venv/bin/python; else echo python3; fi)
ENSEMBLE_RUFF := $(shell if [ -f .venv/bin/ruff ]; then echo $(CURDIR)/.venv/bin/ruff; else command -v ruff 2>/dev/null || echo ruff; fi)
ENSEMBLE_PYRIGHT := $(shell if [ -f .venv/bin/pyright ]; then echo $(CURDIR)/.venv/bin/pyright; else command -v pyright 2>/dev/null || echo pyright; fi)
EVALS_UV := $(shell command -v uv 2>/dev/null || echo uv)

.PHONY: ensemble-test
ensemble-test:
	@echo "Running ensemble (g8ee) pytest unit + in-process integration suite (Tier 1 + Tier 2)..."
	@cd ensemble && $(PYTHON) -m pytest tests/unit/ tests/integration/ -q -m "not ai_integration and not requires_web_search and not requires_api"

.PHONY: evals-test
evals-test: evals-test-unit evals-test-integration

.PHONY: evals-test-unit
evals-test-unit:
	@echo "Running standalone eval Tier 1 tests..."
	@cd ensemble/evals && $(EVALS_UV) run --locked --extra test pytest -q -m unit

.PHONY: evals-test-integration
evals-test-integration:
	@echo "Running standalone eval Tier 2 tests..."
	@cd ensemble/evals && $(EVALS_UV) run --locked --extra test pytest -q -m integration

.PHONY: test-external
test-external:
	@echo "Running ensemble (g8ee) external test suite (Tier 4: real LLM/API calls)..."
	@cd ensemble && $(PYTHON) -m pytest tests/integration/ -q -m "ai_integration or requires_web_search or requires_api"

.PHONY: ensemble-lint
ensemble-lint:
	@echo "Running ruff on ensemble..."
	@cd ensemble && $(ENSEMBLE_RUFF) check app
	@echo "Running pyright on ensemble..."
	@cd ensemble && $(ENSEMBLE_PYRIGHT) app

.PHONY: evals-lint
evals-lint:
	@echo "Running ruff on standalone evals..."
	@cd ensemble/evals && $(EVALS_UV) run --locked --extra test ruff check g8e_evals tests
	@echo "Running pyright on standalone evals..."
	@cd ensemble/evals && $(EVALS_UV) run --locked --extra test pyright --project pyproject.toml

.PHONY: build-ensemble
build-ensemble:
	@echo "Building ensemble (g8ee) Docker image..."
	@DOCKER_BUILDKIT=1 docker build -f ensemble/Dockerfile -t g8e-ensemble:$(VERSION) .
	@echo "Ensemble image built: g8e-ensemble:$(VERSION)"

# =============================================================================
# DASHBOARD (g8ed) — Node.js first-party component
# =============================================================================
# The dashboard requires node_modules installed first:
#   cd dashboard && npm ci

.PHONY: dashboard-test
dashboard-test:
	@echo "Running dashboard (g8ed) vitest suite..."
	@cd dashboard && npm test

.PHONY: build-dashboard
build-dashboard:
	@echo "Building dashboard (g8ed) Docker image..."
	@DOCKER_BUILDKIT=1 docker build -f dashboard/Dockerfile -t g8e-dashboard:$(VERSION) .
	@echo "Dashboard image built: g8e-dashboard:$(VERSION)"

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
lint: lint-no-embedded-newlines vulncheck validate-doctrines validate-cosais swagger-generate
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

.PHONY: validate-cosais
validate-cosais:
	@echo "Validating COSAiS overlay coverage..."
	@bash scripts/validate-cosais-overlays.sh

.PHONY: swagger-generate
swagger-generate:
	@echo "Generating Swagger/OpenAPI documentation..."
	@if command -v swag &> /dev/null || [ -f "$$(go env GOPATH)/bin/swag" ]; then \
		SWAG_CMD=$$(command -v swag 2>/dev/null || echo "$$(go env GOPATH)/bin/swag"); \
		$$SWAG_CMD init --dir cmd/g8e,internal/services/gateway,internal/models,internal/constants --output internal/services/gateway/docs --outputTypes json,yaml --parseInternal --parseDependencyLevel 1 --packagePrefix github.com/g8e-ai/g8e/v2,github.com/go-webauthn,encoding/json 2>/dev/null; \
	else \
		echo "swag not found, installing via go install..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
		$$(go env GOPATH)/bin/swag init --dir cmd/g8e,internal/services/gateway,internal/models,internal/constants --output internal/services/gateway/docs --outputTypes json,yaml --parseInternal --parseDependencyLevel 1 --packagePrefix github.com/g8e-ai/g8e/v2,github.com/go-webauthn,encoding/json 2>/dev/null; \
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
# DOCKER COMPOSE LIFECYCLE
# =============================================================================
# Convenience wrappers around docker compose. Docker-first: `docker compose
# up -d --build` works standalone; these targets are not prerequisites.
.PHONY: up
up:
	@echo "Building and starting the full stack..."
	@docker compose up -d --build
	@echo "Stack started. The gateway is healthy; workloads remain not-ready until bootstrapped."
	@echo "Bootstrap the platform with: ./g8e auth enroll user -e localhost"
	@echo "Then: ./g8e auth pending-platform-enrollments && ./g8e auth approve-platform-enrollment <id> --yes"

.PHONY: down
down:
	@echo "Stopping the full stack including bootstrapped workloads (volumes preserved)..."
	@docker compose --profile bootstrapped down --remove-orphans
	@echo "Stack stopped. Volumes preserved; rerun 'make up' to resume."

.PHONY: clean-docker
clean-docker:
	@echo "Stopping the full stack (all profiles) and removing volumes..."
	@docker compose --profile bootstrapped down -v --remove-orphans
	@echo "Stack stopped and volumes removed. The next 'make up' re-bootstraps the CA and requires re-enrollment."


# =============================================================================
# HELPER FUNCTIONS
# =============================================================================


# =============================================================================
# CI/CD (LOCAL)
# =============================================================================
.PHONY: ci
ci: ci-platform ci-ensemble ci-dashboard
	@echo "CI complete."

.PHONY: ci-platform
ci-platform: _ci-verify-proto _ci-swagger _ci-lint _ci-vulncheck _ci-test
	@echo "Platform CI complete."

.PHONY: ci-ensemble
ci-ensemble: ensemble-lint ensemble-test
	@echo "Ensemble CI complete."

.PHONY: ci-dashboard
ci-dashboard: dashboard-test
	@echo "Dashboard CI complete."

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
	@G8E_STRICT_CONSTANTS_LINT=1 go test -tags=integration $(TEST_RACE) -timeout $(TEST_TIMEOUT) \
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
# The Go module is part of the root module (github.com/g8e-ai/g8e/v2)
# and is versioned by the v* tag. External consumers use:
#   go get github.com/g8e-ai/g8e/v2@vX.Y.Z
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
