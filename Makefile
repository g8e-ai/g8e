# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e Platform Root Makefile
# Industry standard orchestration for multi-component proto generation and builds.

SHELL := /bin/bash
# `go install` writes to GOBIN, else GOPATH/bin. Put it (and ~/.local/bin, where
# the uv installer places `uv`) on PATH so the dev tools installed by
# `make dev-tools` resolve for every recipe even when the invoking shell has not
# sourced the profile the setup scripts updated.
GO_BIN_DIR := $(or $(shell go env GOBIN 2>/dev/null),$(if $(shell go env GOPATH 2>/dev/null),$(shell go env GOPATH)/bin,$(HOME)/go/bin))
export PATH := $(GO_BIN_DIR):$(HOME)/.local/bin:$(PATH)
GOTOOLCHAIN ?= auto
export GOTOOLCHAIN
TMPDIR ?= /tmp
.DEFAULT_GOAL := help

# =============================================================================
# BUILD VARIABLES
# =============================================================================
VERSION := $(shell cat VERSION | tr -d '\n')
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
ifeq ($(strip $(BUILD_TIME)),)
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
endif
ifeq ($(strip $(BUILD_TIME)),unknown)
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
endif
BIN_DIR := bin
MAIN_PKG := ./cmd/g8e

# Source provenance stamps use explicit non-Git values and an explicit source
# manifest. Callers may override all three values with reviewed candidate data.
PROVENANCE_SOURCE_PATHS := cmd internal protocol ensemble scripts test vendor Makefile VERSION go.mod go.sum buf.gen.yaml Dockerfile docker-compose.yml
PROVENANCE_EXCLUDES := .git,.env,.g8e*,.local.dev,.venv,node_modules,__pycache__,*.egg-info,.pytest_cache,.ruff_cache,.mypy_cache,bin,build,dist,site,coverage,reports,test-results,auditor-out,*.out,*.test
SOURCE_TREE_HASH ?= $(shell go run ./internal/tools/treehash -mode manifest -base . -exclude '$(PROVENANCE_EXCLUDES)' $(wildcard $(PROVENANCE_SOURCE_PATHS)) 2>/dev/null || echo "unknown")
BUILD_ID ?= $(SOURCE_TREE_HASH)
SOURCE_REVISION ?= unknown
LDFLAGS = -X main.version=$(VERSION) -X main.buildID=$(BUILD_ID) -X main.buildTime=$(BUILD_TIME) -X main.sourceRevision=$(SOURCE_REVISION) -X main.sourceTreeHash=$(SOURCE_TREE_HASH)
HOST_OS := $(shell go env GOOS)
HOST_ARCH := $(shell go env GOARCH)

# Platform and architecture lists are emitted by the typed g8e-binary catalog.
PLATFORMS := $(shell go run ./internal/tools/g8ebinaries -list-targets 2>/dev/null)
LINUX_ARCHS := $(patsubst linux/%,%,$(filter linux/%,$(PLATFORMS)))
DARWIN_ARCHS := $(patsubst darwin/%,%,$(filter darwin/%,$(PLATFORMS)))
WINDOWS_ARCHS := $(patsubst windows/%,%,$(filter windows/%,$(PLATFORMS)))

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
# Per-package deadlock backstop for unit tests.
TEST_SHORT_TIMEOUT := 180s
# Race detector for Tier 2 integration tests (non-Windows). Tier 1 unit tests
# omit -race for speed; use CI=1 make test-unit or integration tests when
# hunting data races.
TEST_RACE := $(if $(filter windows,$(HOST_OS)),,-race)
# Integration and coverage always disable caching. Unit tests use the Go test
# cache locally; CI sets CI=true and passes -count=1.
TEST_COUNT := -count=1
TEST_UNIT_COUNT := $(if $(CI),-count=1,)
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
	/internal/services/storage/storagetest \
	/node_modules

# Packages excluded from the coverage profile but NOT from test discovery.
# These compile and may be tested, but their statements should not affect
# the coverage threshold (e.g. generated protobuf code, example programs).
COVERAGE_ONLY_EXCLUDE_PKGS := \
	g8e/v2/protocol/proto \
	g8e/v2/examples \
	adapters/lattice/gen \
	node_modules

# All packages excluded from coverage: test exclusions + coverage-only exclusions.
COVERAGE_EXCLUDE_PKGS := $(TEST_EXCLUDE_PKGS) $(COVERAGE_ONLY_EXCLUDE_PKGS)

# Files excluded from coverage only (belong to otherwise-tested packages).
EXCLUDE_FILES := \
	internal/cli/cmd/demos/demos.go \
	internal/cli/cmd/demos/demo_dhs.go \
	internal/cli/cmd/demos/demo_finance.go \
	internal/cli/cmd/demos/demo_healthcare.go

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

# Developer toolchain pins. Keep in sync with .github/workflows/build-and-test.yml;
# `make dev-tools`, `make dev-python`, and `make dev-check` read these.
BUF_VERSION := v1.70.0
GOLANGCI_LINT_VERSION := v2.12.2
UV_VERSION := 0.11.21
PYTHON_VERSION := 3.12

# buf runs these as local plugins from PATH (see buf.gen.yaml); the committed
# generated code is produced by exactly these versions.
PROTO_PLUGIN_PKGS := \
	google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION) \
	google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION) \
	github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@$(PROTOC_GEN_DOC_VERSION)
# govulncheck and swag track @latest, matching CI.
GO_DEV_TOOL_PKGS := \
	github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION) \
	$(PROTO_PLUGIN_PKGS) \
	github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) \
	golang.org/x/vuln/cmd/govulncheck@latest \
	github.com/swaggo/swag/cmd/swag@latest

# Recursive (=) so the lookup runs when a recipe uses it, after buf-install has
# had the chance to put buf on PATH. $(shell) does not see the exported PATH on
# GNU make < 4.4, so the Go bin dir is added explicitly.
BUF = $(shell export PATH="$(GO_BIN_DIR):$$PATH"; command -v buf 2>/dev/null || echo "./buf")
PROTOC := $(shell command -v protoc 2>/dev/null || echo "/usr/local/bin/protoc")
PROTOC_GEN_GO := $(shell go list -m -f '{{.Version}}' google.golang.org/protobuf 2>/dev/null || echo "$(PROTOC_GEN_GO_VERSION)")

# =============================================================================
# HELP
# =============================================================================
.PHONY: help help-legacy
help-legacy:
	@printf '%s\\n' \\
		'Compatibility aliases (prefer the canonical target on the right)' \\
		'' \\
		'  build-ensemble       -> ensemble-build' \\
		'  build-fips           -> fips-build' \\
		'  agent-tool-registry  -> agent-tool-registry-generate' \\
		'  check-bsl-headers    -> bsl-headers-check' \\
		'  clean-docker         -> docker-clean' \\
		'  clean-harness        -> harness-clean' \\
		'  constants            -> constants-generate' \\
		'  docker               -> docker-up' \\
		'  embed-console        -> console-embed' \\
		'  embed-explorer       -> explorer-embed' \\
		'  explorer-catalog     -> explorer-catalog-generate' \\
		'  generate             -> proto-generate' \\
		'  proto                -> proto-generate' \\
		'  proto-force          -> proto-generate' \\
		'  python-build         -> protocol-python-build' \\
		'  stop                 -> down' \\
		'  test-external        -> ensemble-test-external' \\
		'  validate-cosais      -> cosais-validate' \\
		'  validate-doctrines   -> doctrines-validate' \\
		'  verify-fips          -> fips-verify'

help:
	@printf '%s\n' \
		'g8e developer commands' \
		'' \
		'Usage: make <target> [VARIABLE=value]' \
		'Windows: use build.ps1 instead of make.' \
		'' \
		'Setup' \
		'  dev-setup                 Install everything required by make ci' \
		'  dev-check                 Verify the local development toolchain' \
		'  dev-tools                 Install pinned Go development tools' \
		'  dev-python                Create .venv with protocol and ensemble dependencies' \
		'  dev-node                  Install Node dependencies for all workspaces' \
		'' \
		'CI' \
		'  ci                        Run the complete local CI pipeline' \
		'  ci-platform               Run platform, protocol, and documentation CI' \
		'  ci-ensemble               Run ensemble lint and tests' \
		'  ci-console                Run console lint, tests, build, and embed' \
		'' \
		'Build and release' \
		'  build                     Build g8e for the host platform' \
		'  build-all                 Build g8e for every supported platform' \
		'  build-target              Build one target (requires GOOS and GOARCH)' \
		'  build-linux               Build every supported Linux architecture' \
		'  build-darwin              Build every supported macOS architecture' \
		'  build-windows             Build every supported Windows architecture' \
		'  build-compressed          Build for the host and compress with UPX' \
		'  fips-build                Build the Linux AMD64 FIPS variant' \
		'  fips-verify               Build and verify the FIPS variant' \
		'  release                   Tag and push the release in VERSION' \
		'' \
		'Test and quality' \
		'  test                      Run unit and in-process integration tests' \
		'  test-unit                 Run Tier 1 unit tests' \
		'  test-integration          Run Tier 2 in-process integration tests' \
		'  test-coverage             Run tests and enforce $(COVERAGE_THRESHOLD)% coverage (PKG=..., VERBOSE=true)' \
		'  test-docker               Run Tier 3 Docker E2E tests' \
		'  test-cross-enrollment     Run Tier 3 cross-enrollment E2E tests' \
		'  test-airgap               Verify the vendored air-gap build' \
		'  lint                      Run all lint and quality checks' \
		'  fmt                       Format all Go source files' \
		'  vulncheck                 Check Go dependencies for vulnerabilities' \
		'  bsl-headers-check         Verify first-party BSL 1.1 headers' \
		'  doctrines-validate        Validate doctrine JSON and references' \
		'  cosais-validate           Validate COSAiS overlay coverage' \
		'' \
		'Generated artifacts' \
		'  proto-generate            Generate Go, Python, and Node protobuf artifacts' \
		'  protocol-python-build     Build the Python protocol package' \
		'  constants-generate        Generate constants from the event registry' \
		'  constants-check           Check generated constants for drift' \
		'  swagger-generate          Generate the Gateway OpenAPI specification' \
		'  website-build             Render g8e.ai from README.md' \
		'  website-test              Test the website generator and Worker' \
		'' \
		'Components' \
		'  console-build             Build the console SPA' \
		'  console-embed             Build and embed the console SPA' \
		'  console-embed-check       Check the committed console embed for drift' \
		'  console-test              Run console tests' \
		'  console-lint              Typecheck and lint the console' \
		'  ensemble-build            Build the ensemble Docker image' \
		'  ensemble-test             Run ensemble unit and integration tests' \
		'  ensemble-test-external    Run tests that require real providers' \
		'  ensemble-lint             Run ruff and pyright on the ensemble' \
		'  demo-verify               Build and run all demo environments' \
		'' \
		'Run locally' \
		'  up                        Build and start the Gateway on this host' \
		'  full                      Start host stack unattended using .env endpoints' \
		'  full-setup                Start host stack with interactive operator setup' \
		'  down                      Stop the host Gateway' \
		'  stop                      Stop the host Gateway and local workloads (alias for down)' \
		'  docker-up                 Build and start the Docker Compose stack' \
		'  docker-down               Stop the stack and preserve volumes' \
		'  docker-restart-operators  Restart the Data and Inference Operators' \
		'  docker-build              Build images and export the g8e binary' \
		'' \
		'Maintenance' \
		'  clean                     Remove build artifacts and Go caches' \
		'  harness-clean             Remove stale test harness directories' \
		'  docker-clean              Stop the stack and remove its volumes' \
		'  buf-install               Install Buf when it is unavailable' \
		'  protoc-install            Install protoc (optional)' \
		'' \
		'Run make help-legacy to list compatibility aliases.'

.PHONY: protocol-python-build
protocol-python-build:
	@echo "Building Python protocol package..."
	@mkdir -p protocol/python/g8e/_data
	@cp protocol/constants/*.json protocol/python/g8e/_data/
	@cp -r protocol/constants/compliance protocol/python/g8e/_data/
	@cp -r protocol/constants/doctrine protocol/python/g8e/_data/
	@cd protocol/python && uv build
	@echo "Python package built. Check protocol/python/dist/"

.PHONY: constants-generate constants-check
constants-generate:
	@echo "Regenerating protocol constants from protocol/constants/events.json..."
	@go run ./internal/tools/constgen -write

constants-check:
	@echo "Checking protocol/constants/events.json registry and generated constants..."
	@go run ./internal/tools/constgen -check

# The agent tool registry (tool schemas plus frozen model-visible guidance
# vectors) is generated by g8ee from its real tool specs and handlers; the Go
# evaluation catalog reads it. Never hand-edit
# protocol/constants/agenttools/agent-tool-registry.json.
.PHONY: agent-tool-registry-generate agent-tool-registry-check
agent-tool-registry-generate:
	@echo "Regenerating protocol/constants/agenttools/agent-tool-registry.json from g8ee..."
	@cd ensemble && $(PYTHON) -m app.services.evaluation.agent_tool_registry_export --write

agent-tool-registry-check:
	@echo "Checking protocol/constants/agenttools/agent-tool-registry.json against g8ee..."
	@cd ensemble && $(PYTHON) -m app.services.evaluation.agent_tool_registry_export --check

# The evaluation explorer's scenario catalog module is generated from the Go
# scenario catalog. Never hand-edit scenario-catalog.generated.ts.
.PHONY: explorer-catalog-generate explorer-catalog-check
explorer-catalog-generate:
	@echo "Regenerating the evaluation explorer scenario catalog from the Go catalog..."
	@go run ./internal/tools/explorercatalog -write

explorer-catalog-check:
	@echo "Checking the evaluation explorer scenario catalog against the Go catalog..."
	@go run ./internal/tools/explorercatalog -check

.PHONY: website-build
website-build:
	@echo "Rendering g8e.ai from README.md..."
	@cd website && npm run build

.PHONY: website-test
website-test:
	@echo "Testing the g8e.ai generator and Worker..."
	@cd website && npm run check

# =============================================================================
# PROTOCOL GENERATION
# =============================================================================
# Note: buf has its own built-in compiler (protocompile) and invokes the
# protoc-gen-* plugins directly, so the standalone protoc binary is NOT required.
.PHONY: proto-generate
proto-generate: proto-go proto-python proto-node proto-lockfiles
	@echo "Protobuf generation complete."

.PHONY: proto-go
proto-go: buf-install proto-tools-install
	@echo "Generating Go Protobuf code with Buf..."
	@$(BUF) generate protocol/proto
	@echo "Go Protobuf generation complete."

.PHONY: proto-python
proto-python:
	@echo "Generating Python Protobuf code..."
	@if ! $(PYTHON) -c "import grpc_tools" &> /dev/null; then \
		echo "Error: grpc_tools not found in $(PYTHON). Install with: pip install grpcio-tools" >&2; \
		exit 1; \
	fi
	@$(PYTHON) protocol/python/scripts/generate_protos.py
	@echo "Python Protobuf generation complete."

.PHONY: proto-node-install
proto-node-install:
	@if [ ! -x "protocol/node/node_modules/.bin/protoc-gen-es" ]; then \
		echo "Installing Node Protobuf generator..."; \
		npm ci --prefix protocol/node; \
	fi

.PHONY: proto-node
proto-node: buf-install proto-node-install
	@echo "Generating Node TypeScript Protobuf code with Buf..."
	@cd protocol/node && $(abspath $(BUF)) generate ../proto --template buf.gen.yaml
	@echo "Node TypeScript Protobuf generation complete."

# Regenerate the ensemble uv.lock file that depends on protocol/python through
# a directory dependency. The protocol/python package version is authoritative.
.PHONY: proto-lockfiles
proto-lockfiles:
	@echo "Regenerating the ensemble uv.lock file..."
	@cd ensemble && uv lock --quiet
	@echo "Ensemble uv.lock regenerated."

# proto-force is a compatibility alias of proto-generate. It previously ran only `buf generate`
# for Go, which left the Python stubs, TypeScript stubs, and ensemble
# lockfiles stale.

# =============================================================================
# TOOL INSTALLATION
#
# NOTE: protoc-install is OPTIONAL. `make proto-generate` uses buf, which ships its own
# compiler and does not require the standalone protoc binary. This target exists
# only for manual use (e.g. invoking protoc directly for debugging).
# =============================================================================
.PHONY: buf-install
buf-install:
	@if ! command -v buf &> /dev/null && [ ! -f "./buf" ]; then \
		if command -v go &> /dev/null; then \
			echo "Installing Buf $(BUF_VERSION) natively via Go toolchain..."; \
			go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION); \
		else \
			echo "Go not found, attempting direct download..."; \
			curl -sSL "https://github.com/bufbuild/buf/releases/latest/download/buf-$$(uname -s)-$$(uname -m)" -o ./buf && chmod +x ./buf || \
			echo "Warning: Failed to download Buf. Proceeding with existing protocol files if available."; \
		fi \
	fi

# Installs any missing protoc plugin that buf.gen.yaml invokes. Existing binaries
# are left alone; `make dev-tools` reinstalls them at the pinned versions.
.PHONY: proto-tools-install
proto-tools-install:
	@for pkg in $(PROTO_PLUGIN_PKGS); do \
		bin=$${pkg%@*}; bin=$${bin##*/}; \
		if ! command -v $$bin &> /dev/null; then \
			echo "Installing $$bin ($$pkg)..."; \
			go install $$pkg || exit 1; \
		fi; \
	done

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
# DEVELOPER TOOLCHAIN
#
# OS-level prerequisites (git, make, go, node, python3, uv, rg, bc, a C compiler)
# are installed by scripts/linux-setup.sh, scripts/macos-setup.sh, or
# scripts/windows-setup.ps1. Everything those need on top is pinned above and
# installed here, so the setup scripts and a manual install share one definition.
# =============================================================================
.PHONY: dev-setup
dev-setup: dev-tools dev-python dev-node
	@echo "Developer toolchain installed. Verify with: make dev-check"

# Go-based tools (buf, protoc plugins, golangci-lint, govulncheck, swag), always
# reinstalled at the pinned versions.
.PHONY: dev-tools
dev-tools:
	@echo "Installing Go dev tools into $(GO_BIN_DIR)..."
	@for pkg in $(GO_DEV_TOOL_PKGS); do \
		echo "  go install $$pkg"; \
		go install $$pkg || exit 1; \
	done

# Repo-root .venv (the interpreter the ensemble and proto targets prefer, see
# PYTHON below) with the in-tree protocol package and ensemble test/lint deps.
# uv provisions Python $(PYTHON_VERSION) itself when the system has none.
.PHONY: dev-python
dev-python:
	@command -v uv &> /dev/null || { echo "Error: uv not found. Run the setup script for your platform in scripts/ or see https://docs.astral.sh/uv/" >&2; exit 1; }
	@echo "Preparing .venv (Python $(PYTHON_VERSION)) with protocol and ensemble dependencies..."
	@uv venv --python $(PYTHON_VERSION) --seed --allow-existing .venv
	@# --no-sources: ensemble's [tool.uv.sources] pins g8e to a non-editable path, which
	@# conflicts with the editable protocol/python install that lets protocol edits show up live.
	@uv pip install --python .venv/bin/python --no-sources -e protocol/python -e "ensemble[test]"

.PHONY: dev-node
dev-node:
	@echo "Installing Node dependencies (protocol/node, console, g8e-adapter)..."
	@npm ci --prefix protocol/node
	@npm ci --prefix console
	@npm ci --prefix g8e-adapter
	@npm run build --prefix g8e-adapter

# Preflight for `make ci`: reports every missing or mismatched tool at once.
.PHONY: dev-check
dev-check:
	@bash scripts/dev-check.sh

# =============================================================================
# BUILD
# =============================================================================

# Install a built binary over an existing path without stopping a running copy.
# Direct cp fails with ETXTBSY when the target is executing; rename replaces the
# directory entry while the old inode stays mapped for the running process.
INSTALL_EXECUTABLE = \
	if [ "$(HOST_OS)" = "windows" ]; then \
		cp "$$INSTALL_SRC" "$$INSTALL_DST"; \
	else \
		cp "$$INSTALL_SRC" "$$INSTALL_DST.new" && chmod +x "$$INSTALL_DST.new" && mv -f "$$INSTALL_DST.new" "$$INSTALL_DST"; \
	fi

EXPLORER_DIST := evaluation-explorer/dist
EXPLORER_EMBED := internal/services/gateway/explorer/static

.PHONY: explorer-embed
explorer-embed:
	@test -f $(EXPLORER_DIST)/index.html || { echo "ERROR: build evaluation explorer first: cd $(EXPLORER_DIST)/.. && npm run build"; exit 1; }
	@rm -rf $(EXPLORER_EMBED)
	@cp -a $(EXPLORER_DIST) $(EXPLORER_EMBED)

# The console embed is committed, like the explorer's. When console/dist has
# not been built (Go-only checkouts), the committed embed is used as-is.
CONSOLE_DIST := console/dist
CONSOLE_EMBED := internal/services/gateway/console/static

.PHONY: console-embed
console-embed: console-build
	@rm -rf $(CONSOLE_EMBED) && cp -a $(CONSOLE_DIST) $(CONSOLE_EMBED)
	@echo "Embedded console updated from fresh console build."

.PHONY: _embed-console-if-built
_embed-console-if-built:
	@if [ -f $(CONSOLE_DIST)/index.html ]; then \
		rm -rf $(CONSOLE_EMBED) && cp -a $(CONSOLE_DIST) $(CONSOLE_EMBED); \
	else \
		echo "console/dist not built; using the committed console embed (run 'make console-embed' to refresh)"; \
	fi

.PHONY: build
build: explorer-embed _embed-console-if-built
	@echo "Building g8e Operator for current platform..."
	@mkdir -p $(BIN_DIR)
	@rm -f $(BIN_DIR)/g8e-binaries.json
	@set -e; \
	G8E_BINARY=$(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH); \
	if [ "$(HOST_OS)" = "windows" ]; then \
		G8E_BINARY=$$G8E_BINARY.exe; \
		ROOT_COPY=g8e.exe; \
	else \
		ROOT_COPY=g8e; \
	fi; \
	echo "Building $(HOST_OS)/$(HOST_ARCH) -> $$G8E_BINARY..."; \
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=$(HOST_OS)_$(HOST_ARCH)" -o $$G8E_BINARY $(MAIN_PKG); \
	sha256sum $$G8E_BINARY > $$G8E_BINARY.sha256; \
	INSTALL_SRC=$$G8E_BINARY INSTALL_DST=$$ROOT_COPY; $(INSTALL_EXECUTABLE); \
	INSTALL_SRC=$$G8E_BINARY INSTALL_DST=$(BIN_DIR)/$$ROOT_COPY; $(INSTALL_EXECUTABLE)
	@echo "Build complete. Binary: $(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH)$(if $(filter windows,$(HOST_OS)),.exe,)"

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

.PHONY: build-target
build-target:
	@test -n "$(GOOS)" || { echo "ERROR: GOOS is required for build-target"; exit 1; }
	@test -n "$(GOARCH)" || { echo "ERROR: GOARCH is required for build-target"; exit 1; }
	@echo "Building g8e for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BIN_DIR)
	@set -e; \
	G8E_BINARY=$(BIN_DIR)/g8e-$(GOOS)-$(GOARCH); \
	if [ "$(GOOS)" = "windows" ]; then \
		G8E_BINARY=$$G8E_BINARY.exe; \
	fi; \
	echo "Building $(GOOS)/$(GOARCH) -> $$G8E_BINARY..."; \
	if [ "$(GOOS)" = "linux" ]; then \
		FIPS_ENV="GOFIPS140=$(GOFIPS140_VERSION)"; \
	else \
		FIPS_ENV="-u GOFIPS140"; \
	fi; \
	env $$FIPS_ENV CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=$(GOOS)_$(GOARCH)" -o $$G8E_BINARY $(MAIN_PKG); \
	sha256sum $$G8E_BINARY > $$G8E_BINARY.sha256
	@echo "Target build complete: $(BIN_DIR)/g8e-$(GOOS)-$(GOARCH)$(if $(filter windows,$(GOOS)),.exe,)"

.PHONY: build-all
build-all:
	@echo "Building g8e Operator for all platforms (FIPS 140-3 for linux)..."
	@mkdir -p $(BIN_DIR)
	@rm -f $(BIN_DIR)/g8e-binaries.json
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		G8E_BINARY=$(BIN_DIR)/g8e-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then \
			G8E_BINARY=$$G8E_BINARY.exe; \
		fi; \
		echo "Building $$platform -> $$G8E_BINARY..."; \
		if [ "$$GOOS" = "linux" ]; then \
			FIPS_ENV="GOFIPS140=$(GOFIPS140_VERSION)"; \
		else \
			FIPS_ENV="-u GOFIPS140"; \
		fi; \
		env $$FIPS_ENV CGO_ENABLED=$(CGO_ENABLED) GOOS=$$GOOS GOARCH=$$GOARCH go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=$$platform" -o $$G8E_BINARY $(MAIN_PKG); \
		sha256sum $$G8E_BINARY > $$G8E_BINARY.sha256; \
	done
	@HOST_G8E_BINARY=$(BIN_DIR)/g8e-$(HOST_OS)-$(HOST_ARCH); \
	if [ "$(HOST_OS)" = "windows" ]; then \
		HOST_G8E_BINARY=$$HOST_G8E_BINARY.exe; \
		ROOT_COPY=g8e.exe; \
	else \
		ROOT_COPY=g8e; \
	fi; \
	INSTALL_SRC=$$HOST_G8E_BINARY INSTALL_DST=$$ROOT_COPY; $(INSTALL_EXECUTABLE); \
	INSTALL_SRC=$$HOST_G8E_BINARY INSTALL_DST=$(BIN_DIR)/$$ROOT_COPY; $(INSTALL_EXECUTABLE)
	@go run ./internal/tools/g8ebinaries --root $(BIN_DIR) --version "$(VERSION)" --build-id "$(BUILD_ID)" --build-time "$(BUILD_TIME)" --source-revision "$(SOURCE_REVISION)" --source-tree-hash "$(SOURCE_TREE_HASH)"
	@echo "Multi-platform build complete. Manifest and checksums: $(BIN_DIR)/g8e-binaries.json"
	@echo "Host binary copied: ./g8e ($(HOST_OS)/$(HOST_ARCH))"

.PHONY: build-darwin
build-darwin:
	@echo "Building g8e for Darwin..."
	@mkdir -p $(BIN_DIR)
	@rm -f $(BIN_DIR)/g8e-binaries.json
	@for arch in $(DARWIN_ARCHS); do \
		G8E_BINARY=$(BIN_DIR)/g8e-darwin-$$arch; \
		echo "Building darwin/$$arch -> $$G8E_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=$$arch go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=darwin_$$arch" -o $$G8E_BINARY $(MAIN_PKG); \
		sha256sum $$G8E_BINARY > $$G8E_BINARY.sha256; \
	done
	@echo "Darwin build complete. Binaries: $(BIN_DIR)/g8e-darwin-*"

.PHONY: build-linux
build-linux:
	@echo "Building g8e for Linux..."
	@mkdir -p $(BIN_DIR)
	@rm -f $(BIN_DIR)/g8e-binaries.json
	@for arch in $(LINUX_ARCHS); do \
		G8E_BINARY=$(BIN_DIR)/g8e-linux-$$arch; \
		echo "Building linux/$$arch -> $$G8E_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=$$arch go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=linux_$$arch" -o $$G8E_BINARY $(MAIN_PKG); \
		sha256sum $$G8E_BINARY > $$G8E_BINARY.sha256; \
	done
	@echo "Linux build complete. Binaries: $(BIN_DIR)/g8e-linux-*"

.PHONY: build-windows
build-windows:
	@echo "Building g8e for Windows..."
	@mkdir -p $(BIN_DIR)
	@rm -f $(BIN_DIR)/g8e-binaries.json
	@for arch in $(WINDOWS_ARCHS); do \
		G8E_BINARY=$(BIN_DIR)/g8e-windows-$$arch.exe; \
		echo "Building windows/$$arch -> $$G8E_BINARY..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=$$arch go build $(TRIMPATH) -tags $(BUILD_TAGS) -ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=windows_$$arch" -o $$G8E_BINARY $(MAIN_PKG); \
		sha256sum $$G8E_BINARY > $$G8E_BINARY.sha256; \
	done
	@echo "Windows build complete. Binaries: $(BIN_DIR)/g8e-windows-*.exe"

# FIPS 140-3 build variant.
# Produces a linux/amd64 binary linked against the Go Cryptographic Module
# v1.0.0 (CMVP Cert #5247) with FIPS 140-3 approved mode enabled by default.
# GOFIPS140 is set at BUILD TIME ONLY — the resulting binary enters approved
# mode on startup without any runtime env var. See:
#   https://go.dev/doc/security/fips140
# Verify the deployed binary with: ./g8e version --fips
.PHONY: fips-build
fips-build:
	@echo "Building g8e with FIPS 140-3 approved mode (GOFIPS140=$(GOFIPS140_VERSION), $(FIPS_GOOS)/$(FIPS_GOARCH))..."
	@mkdir -p $(BIN_DIR)
	@G8E_BINARY=$(BIN_DIR)/g8e-fips-$(FIPS_GOOS)-$(FIPS_GOARCH); \
	echo "Building $(FIPS_GOOS)/$(FIPS_GOARCH) -> $$G8E_BINARY..."; \
	GOFIPS140=$(GOFIPS140_VERSION) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(FIPS_GOOS) GOARCH=$(FIPS_GOARCH) \
		go build $(TRIMPATH) -tags $(BUILD_TAGS) \
		-ldflags "$(LDFLAGS) $(STRIP_FLAGS) -X main.platform=$(FIPS_GOOS)_$(FIPS_GOARCH)" \
		-o $$G8E_BINARY $(MAIN_PKG); \
	sha256sum $$G8E_BINARY > $$G8E_BINARY.sha256; \
	INSTALL_SRC=$$G8E_BINARY INSTALL_DST=g8e-fips; $(INSTALL_EXECUTABLE)
	@echo "FIPS build complete. Binary: $(BIN_DIR)/g8e-fips-$(FIPS_GOOS)-$(FIPS_GOARCH)"
	@echo "Verify with: ./g8e-fips version --fips"

# Quick FIPS self-check: build the FIPS variant and confirm the binary reports
# FIPS 140-3 approved mode AND enforcement are active via the native
# crypto/fips140 module API. GODEBUG=fips140=only is set at runtime to switch
# the module into enforcement mode (rejecting non-approved primitives);
# GOFIPS140 alone only enables approved mode (GODEBUG defaults to fips140=on).
# Exits non-zero if the self-check fails. Intended for CI and release gates.
.PHONY: fips-verify
fips-verify: fips-build
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
test-unit: constants-check
	@echo "Running Tier 1 (Unit) tests..."
	@go test -tags=!integration $(TEST_UNIT_COUNT) -timeout $(TEST_SHORT_TIMEOUT) $(TEST_PKGS)


# Tier 2: In-Process Integration Tests - no external dependencies
.PHONY: test-integration
test-integration:
	@echo "Running Tier 2 (In-Process Integration) tests..."
	@go test $(if $(TEST_P),-p=$(TEST_P),) -tags=integration $(TEST_RACE) $(TEST_COUNT) -timeout $(TEST_TIMEOUT) $(TEST_PKGS)

# Tier 3: Docker E2E Tests - requires a running platform.
# Start the platform first (docker compose up or ./g8e gw start), approve all
# enrollment requests, then run this target. The test binary connects to the
# running platform and fails fast if it is not reachable. Per docs/devs/devs.md,
# platform tests run through ./g8e test, never go test directly.
#
# The default target runs the steady-state suite: tests that exercise an
# approved stack (gateway, auth, operator registry, heartbeat, command
# roundtrip, ensemble, console, compliance, approved-restart). Stateful
# scenario tests (pending-discovery, denial, restart-during-pending, headless)
# require specific platform states and are run individually via:
#   ./g8e test e2e --run TestPlatformEnrollment_PendingDiscovery
#   ./g8e test e2e --run TestPlatformEnrollment_Denial
#   ./g8e test e2e --run TestPlatformEnrollment_RestartDuringPending
#   ./g8e test e2e --run TestPlatformEnrollment_Headless
#
# Cross-enrollment scenarios (require --profile cross-enrollment):
#   docker compose --profile cross-enrollment up -d
#   ./g8e auth enroll user --headless
#   ./g8e test e2e --run TestCrossEnrollment_GatewayAsOperator_PendingDiscovery
#   ./g8e test e2e --run TestCrossEnrollment_GatewayAsOperator_ApproveAndActivate
#   ./g8e test e2e --run TestCrossEnrollment_GatewayAsOperator_Denial
#   ./g8e test e2e --run TestCrossEnrollment_GatewayAsOperator_RestartDuringPending
.PHONY: test-docker
test-docker:
	@echo "Running Tier 3 (Docker E2E) steady-state tests..."
	@./g8e test e2e --run 'TestGateway|TestAuth|TestOperatorRegistry|TestPubSub|TestCommandRoundtrip|TestEnsemble|TestConsole|TestCompliance|TestApprovedRestart'

# Tier 3: Cross-Enrollment E2E Tests - a gateway enrolling as an operator of
# another gateway. Requires the cross-enrollment profile, which starts a
# secondary gateway container in operator mode against the primary gateway.
# The full lifecycle variant (./g8e test e2e-full --cross-enrollment) manages
# the compose stack automatically; the manual variant below assumes the user
# has already started the stack with the cross-enrollment profile and
# bootstrapped the owner. See the comment block above test-docker for the
# per-scenario commands.
.PHONY: test-cross-enrollment
test-cross-enrollment:
	@echo "Running Tier 3 cross-enrollment E2E tests..."
	@./g8e test e2e --run 'TestCrossEnrollment'


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

.PHONY: ensemble-test
ensemble-test:
	@echo "Running ensemble (g8ee) pytest unit + in-process integration suite (Tier 1 + Tier 2)..."
	@cd ensemble && $(PYTHON) -m pytest tests/unit/ tests/integration/ -m "not ai_integration and not requires_web_search and not requires_api"

.PHONY: ensemble-test-external
ensemble-test-external:
	@echo "Running ensemble (g8ee) external test suite (Tier 4: real LLM/API calls)..."
	@cd ensemble && $(PYTHON) -m pytest tests/integration/ -q -m "ai_integration or requires_web_search or requires_api or requires_system_one"

.PHONY: ensemble-lint
ensemble-lint:
	@echo "Running ruff on ensemble..."
	@cd ensemble && $(ENSEMBLE_RUFF) check app
	@echo "Running pyright on ensemble..."
	@cd ensemble && $(ENSEMBLE_PYRIGHT) app

.PHONY: ensemble-build
ensemble-build:
	@echo "Building ensemble (g8ee) Docker image..."
	@DOCKER_BUILDKIT=1 docker build -f ensemble/Dockerfile -t g8e-ensemble:$(VERSION) .
	@echo "Ensemble image built: g8e-ensemble:$(VERSION)"

# =============================================================================
# CONSOLE — Gateway-embedded browser frontend (console/, served at /console/)
# =============================================================================
# Requires node_modules installed first:
#   cd console && npm ci

.PHONY: console-build
console-build:
	@echo "Building the console SPA..."
	@cd console && npm run build

.PHONY: console-lint
console-lint:
	@echo "Typechecking and linting the console..."
	@cd console && npm run typecheck && npm run lint

.PHONY: console-test
console-test:
	@echo "Running the console vitest suite..."
	@cd console && npm test

# Rebuilds the console and fails if the committed embed is stale.
.PHONY: console-embed-check
console-embed-check: console-build
	@diff -r $(CONSOLE_DIST) $(CONSOLE_EMBED) >/dev/null || { echo "ERROR: $(CONSOLE_EMBED) is stale; run 'make console-embed' and commit the result"; exit 1; }
	@echo "Embedded console is current."

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
lint: lint-no-embedded-newlines vulncheck doctrines-validate cosais-validate swagger-generate
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

.PHONY: doctrines-validate
doctrines-validate:
	@echo "Validating doctrine JSON schema..."
	@for file in protocol/constants/doctrine/*.json; do \
		if [ -f "$$file" ]; then \
			echo "Validating $$file..."; \
			python3 -m json.tool "$$file" > /dev/null || exit 1; \
		fi \
	done
	@go run ./internal/tools/doctrine_validator
	@echo "All doctrine files and compliance references are valid."

.PHONY: cosais-validate
cosais-validate:
	@echo "Validating COSAiS overlay coverage..."
	@go run ./internal/tools/cosais_validator

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
# Build cleanup must preserve the Gateway CA, databases, and workload trust.
# Use ./g8e gw clean explicitly to archive and reset the Gateway runtime.
clean:
	@echo "Cleaning up build artifacts..."
	@rm -rf .g8e-test-tmp/
	@rm -rf bin/
	@rm -f *.sha256 *.test coverage.out coverage_filtered.out buf
	@rm -rf .g8e-harness-*/
	@GOTOOLCHAIN=local go clean -cache
	@GOTOOLCHAIN=local go clean -modcache
	@echo "Clean complete."

.PHONY: harness-clean
harness-clean:
	@echo "Cleaning up stale harness directories..."
	@rm -rf .g8e-harness-*/
	@echo "Clean complete."

# =============================================================================
# HOST PLATFORM LIFECYCLE
# =============================================================================
# These targets run the platform directly on the development host. They must
# not call Docker or Docker Compose; the Docker lifecycle is defined separately
# below under explicitly Docker-named targets.
.PHONY: up
up: build
	@echo "Starting the g8e Gateway on this host..."
	@./g8e gw start
	@echo "Host platform started. Check it with: ./g8e gw status"
	@echo "Bootstrap the platform with: ./g8e auth enroll user -e localhost"

.PHONY: full full-setup
# FULL_ARGS carries explicit path flags or --dry-run; .env is parsed as data by
# the launcher, never included by Make or sourced as executable shell code.
full: build
	@$(PYTHON) scripts/full.py --start-gateway $(if $(filter 1,$(RESET_IDENTITIES)),--reset-identities,) $(FULL_ARGS)

full-setup: build
	@$(PYTHON) scripts/full.py --setup --start-gateway $(if $(filter 1,$(RESET_IDENTITIES)),--reset-identities,) $(FULL_ARGS)

.PHONY: down stop
down:
	@test -x ./g8e || { echo "ERROR: ./g8e is missing; run 'make build' first" >&2; exit 1; }
	@./g8e ensemble stop 2>/dev/null || true
	@./g8e operator stop 2>/dev/null || true
	@echo "Stopping the g8e Gateway running on this host..."
	@./g8e gw stop
	@echo "Host platform stopped. Runtime state in .g8e/ is preserved."

stop: down

# =============================================================================
# DOCKER COMPOSE LIFECYCLE
# =============================================================================
# Docker is deliberately isolated behind Docker-named targets. The host-native
# `up` and `down` targets above never invoke these recipes.
.PHONY: docker-up
docker-up:
	@echo "Building and starting the Docker Compose unified stack..."
	@docker compose up -d --build
	@echo "Docker stack started. Workloads await owner approval."
	@echo "Bootstrap the platform with: ./g8e auth enroll user -e localhost"
	@echo "Then approve workloads: ./g8e auth enroll pending && ./g8e auth enroll approve <id> --yes"

.PHONY: docker-down
docker-down:
	@echo "Stopping the Docker Compose unified stack (volumes preserved)..."
	@docker compose down --remove-orphans
	@echo "Docker stack stopped. Volumes preserved; rerun 'make docker-up' to resume."

.PHONY: docker-clean
docker-clean:
	@echo "Stopping the unified stack and removing volumes..."
	@docker compose down -v --remove-orphans
	@echo "Stack stopped and volumes removed. The next 'make docker-up' re-bootstraps the CA and requires re-enrollment."

.PHONY: docker-restart-operators
docker-restart-operators:
	@echo "Restarting Data and Inference Operators to align with current binary..."
	@docker compose restart g8e-data-operator g8e-inference-operator
	@echo "Operators restarted."

.PHONY: docker-build
docker-build:
	@echo "Building Docker images using in-container Makefile..."
	@docker compose build
	@mkdir -p $(BIN_DIR)
	@echo "Exporting runtime binary to $(BIN_DIR)/g8e and ./g8e..."
	@docker run --rm --entrypoint cp -v $(CURDIR):/out g8e-gateway /g8e /out/$(BIN_DIR)/g8e
	@cp -f $(BIN_DIR)/g8e ./g8e
	@echo "Build complete. Host ./g8e and container images are aligned."


# =============================================================================
# COMPATIBILITY ALIASES
# =============================================================================
# Keep established entry points working for scripts and downstream users. New
# documentation and Makefile dependencies should use the canonical targets.
.PHONY: \
	agent-tool-registry build-ensemble build-fips check-bsl-headers clean-docker \
	clean-harness constants docker embed-console embed-explorer \
	explorer-catalog generate proto proto-force python-build stop test-external \
	validate-cosais validate-doctrines verify-fips

agent-tool-registry: agent-tool-registry-generate
build-ensemble: ensemble-build
build-fips: fips-build
check-bsl-headers: bsl-headers-check
clean-docker: docker-clean
clean-harness: harness-clean
constants: constants-generate
docker: docker-up
embed-console: console-embed
embed-explorer: explorer-embed
explorer-catalog: explorer-catalog-generate
generate: proto-generate
proto: proto-generate
proto-force: proto-generate
python-build: protocol-python-build
test-external: ensemble-test-external
validate-cosais: cosais-validate
validate-doctrines: doctrines-validate
verify-fips: fips-verify


# =============================================================================
# CI/CD (LOCAL)
# =============================================================================
.PHONY: ci
ci: ci-console ci-platform ci-ensemble
	@echo "CI complete."

.PHONY: ci-platform
ci-platform: dev-check _ci-verify-proto _ci-swagger _ci-lint _ci-vulncheck _ci-test bsl-headers-check
	@echo "Platform CI complete."

.PHONY: ci-ensemble
ci-ensemble: dev-check ensemble-lint ensemble-test
	@echo "Ensemble CI complete."

.PHONY: ci-console
ci-console: dev-check console-lint console-test console-embed
	@echo "Console CI complete."

.PHONY: bsl-headers-check
bsl-headers-check:
	@python3 scripts/check-bsl-headers.py

.PHONY: _ci-verify-proto
_ci-verify-proto:
	@echo "=== verify-proto ==="
	@$(MAKE) proto-generate
	@CHANGES=$$(git status --porcelain | grep -E "^\s*M.*\.pb\.go$$|^\s*M.*\.proto$$" || true); \
	if [ -n "$$CHANGES" ]; then \
		echo "Error: Generated proto files are out of sync with protocol/proto/*.proto"; \
		echo "$$CHANGES"; \
		git diff -- $$(git status --porcelain | grep -E "^\s*M" | awk '{print $$2}'); \
		exit 1; \
	fi
	@$(MAKE) doctrines-validate

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
# VERSION is the single source of truth. `make release` syncs pyproject.toml,
# __init__.py, the Python uv.lock package entry, and maintained protocol specification
# Version: headers from VERSION (if needed), tags the current commit as
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
	PY_LOCK=protocol/python/uv.lock; \
	PY_VERSION=$$(grep -E '^version = ' $$PY_FILE | head -1 | sed -E 's/.*"([^"]+)".*/\1/'); \
	PY_INIT_VERSION=$$(grep -E '^__version__ = ' $$PY_INIT | head -1 | sed -E 's/.*"([^"]+)".*/\1/'); \
	PY_LOCK_VERSION=$$(sed -n -E '/^name = "g8e"$$/{n;s/^version = "([^"]+)"/\1/p;}' $$PY_LOCK); \
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
	if [ "$$PY_LOCK_VERSION" != "$$VERSION" ]; then \
		echo "Syncing $$PY_LOCK: $$PY_LOCK_VERSION -> $$VERSION"; \
		sed -i.bak -E '/^name = "g8e"$$/{n;s/^version = "[^"]+"/version = "'$$VERSION'"/;}' $$PY_LOCK; \
		rm -f $$PY_LOCK.bak; \
		echo "  uv.lock synced."; \
	else \
		echo "  uv.lock already in sync."; \
	fi; \
	for doc in a2a.md constants.md mcp.md spec.md; do \
		DOC_FILE=protocol/docs/$$doc; \
		DOC_VERSION=$$(grep -E '^Version: v' $$DOC_FILE | head -1 | sed 's/^Version: v//'); \
		if [ "$$DOC_VERSION" != "$$VERSION" ]; then \
			echo "Syncing $$DOC_FILE: $$DOC_VERSION -> $$VERSION"; \
			sed -i.bak -E 's/^Version: v[^[:space:]]+/Version: v'$$VERSION'/' $$DOC_FILE; \
			rm -f $$DOC_FILE.bak; \
			echo "  $$DOC_FILE synced."; \
		else \
			echo "  $$DOC_FILE already in sync."; \
		fi; \
	done; \
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
