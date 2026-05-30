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
export PATH := $(shell go env GOPATH)/bin:$(HOME)/go/bin:$(PATH)
.DEFAULT_GOAL := help

# =============================================================================
# TOOLS
# =============================================================================
# Protocol buffer tool versions - update these when upgrading tools
PROTOC_VERSION := v35.0
PROTOC_GEN_GO_VERSION := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.6.2
PROTOC_GEN_DOC_VERSION := v1.5.1
PROTOC_MIN_VERSION := 21
UPX_VERSION := v4.2.4

BUF := $(shell command -v buf 2>/dev/null || echo "./buf")
PROTOC := $(shell command -v protoc 2>/dev/null || echo "/usr/local/bin/protoc")
PROTOC_GEN_GO := $(shell go list -m -f '{{.Version}}' google.golang.org/protobuf 2>/dev/null || echo "$(PROTOC_GEN_GO_VERSION)")
UPX := $(shell command -v upx 2>/dev/null || echo "/usr/local/bin/upx")

# =============================================================================
# HELP
# =============================================================================
.PHONY: help
help:
	@echo "g8e Platform Root Makefile"
	@echo ""
	@echo "CI/CD (Local):"
	@echo "  ci            Run full CI pipeline locally (mirrors GitHub Actions)"
	@echo "  ci-platform   Run platform-only CI (g8eo, protocol, proto, docs)"
	@echo ""
	@echo "Protocol Generation:"
	@echo "  generate      Generate all protocol artifacts (proto)"
	@echo "  proto         Generate all Protobuf code (Go)"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  protoc-install Install protoc compiler"
	@echo "  upx-install   Install UPX compressor"
	@echo ""
	@echo "Build:"
	@echo "  build         Build g8e binary"
	@echo "  build-compressed Build g8e with UPX compression"
	@echo ""
	@echo "Test:"
	@echo "  test          Run all tests with race detection"
	@echo "  test-short    Run short tests with race detection"
	@echo "  test-coverage Run tests with coverage (enforces 60% threshold). Use PKG=./path/to/pkg for specific package, VERBOSE=true for verbose output"
	@echo "  test-shuffle  Run all tests with randomized order"
	@echo "  update-golden Update scenario test golden files"
	@echo ""
	@echo "Lint & Quality:"
	@echo "  lint          Run all linting and quality checks"
	@echo "  vulncheck     Run Operator vulnerability check"
	@echo "  validate-doctrines Validate doctrine JSON schema"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean         Remove all build artifacts and runtime state"
	@echo ""
	@echo "Governance Auditor (via CLI):"
	@echo "  ./g8e auditor list              List available scenarios"
	@echo "  ./g8e auditor run              Run scenarios against a real Gateway/Operator"
	@echo "  ./g8e auditor audit            Audit signed receipts from the Operator"
	@echo "  ./g8e auditor self-test        Start self-contained gateway+operator and run tests"
	@echo ""
	@echo "Chaos Tester (via CLI):"
	@echo "  ./g8e chaos --count N          Generate governance events against the audit stack"

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
			GOBIN=$$(pwd) go install github.com/bufbuild/buf/cmd/buf@v1.70.0; \
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
		cd /tmp && curl -fSL https://github.com/protocolbuffers/protobuf/releases/download/$(PROTOC_VERSION)/protoc-35.0-linux-x86_64.zip -o protoc.zip && \
		unzip -o protoc.zip -d protoc && \
		sudo cp protoc/bin/protoc /usr/local/bin/protoc && \
		sudo chmod +x /usr/local/bin/protoc && \
		rm -rf /tmp/protoc /tmp/protoc.zip; \
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

.PHONY: upx-install
upx-install:
	@if ! command -v upx &> /dev/null; then \
		echo "Installing UPX $(UPX_VERSION)..."; \
		cd /tmp && curl -fSL https://github.com/upx/upx/releases/download/$(UPX_VERSION)/upx-$(UPX_VERSION:v%=%)-linux_amd64.tar.xz -o upx.tar.xz && \
		tar -xf upx.tar.xz && \
		sudo cp upx-$(UPX_VERSION:v%=%)-linux_amd64/upx /usr/local/bin/upx && \
		sudo chmod +x /usr/local/bin/upx && \
		rm -rf /tmp/upx-$(UPX_VERSION:v%=%)-linux_amd64 /tmp/upx.tar.xz; \
	fi
	@echo "UPX installed."

# =============================================================================
# BUILD
# =============================================================================
PLATFORMS := linux/amd64 linux/arm64 linux/386

.PHONY: build
build:
	@echo "Building g8e..."
	@mkdir -p bin
	@echo "Building from: cmd/g8eo"
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	echo "Building output: bin/g8e"; \
	if go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -o bin/g8e ./cmd/g8eo; then \
		ln -sf bin/g8e g8e; \
		sha256sum bin/g8e > bin/g8e.sha256; \
		echo "Build complete. Output: bin/g8e"; \
		echo "Checksum: bin/g8e.sha256"; \
	else \
		exit 1; \
	fi

.PHONY: build-compressed
build-compressed: upx-install
	@echo "Building g8e with compression for $(PLATFORMS)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		BINARY=bin/g8e-$$GOOS-$$GOARCH; \
		echo "Building $$platform -> $$BINARY..."; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$platform" -o $$BINARY ./cmd/g8eo || exit 1; \
		echo "Compressing $$BINARY with UPX..."; \
		$(UPX) --best --lzma $$BINARY; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@HOST_OS=$$(go env GOOS); \
	HOST_ARCH=$$(go env GOARCH); \
	if [ -f bin/g8e-$$HOST_OS-$$HOST_ARCH ]; then \
		ln -sf bin/g8e-$$HOST_OS-$$HOST_ARCH g8e; \
		echo "Created symlink: g8e -> bin/g8e-$$HOST_OS-$$HOST_ARCH"; \
	fi
	@echo "Compressed multi-platform build complete. Checksums: bin/g8e-*.sha256"

# =============================================================================
# TEST
# =============================================================================
.PHONY: test
test:
	@go test -race -count=1 -timeout 180s ./...

.PHONY: test-short
test-short:
	@go test -race -short -count=1 -timeout 60s ./...

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	@if [ -n "$(PKG)" ]; then \
		echo "Testing package: $(PKG)"; \
		if [ "$(VERBOSE)" = "true" ]; then \
			go test -v -race -count=1 -timeout 180s -coverprofile=coverage.out -covermode=atomic $(PKG); \
		else \
			go test -race -count=1 -timeout 180s -coverprofile=coverage.out -covermode=atomic $(PKG); \
		fi; \
	else \
		echo "Testing all packages (excluding mocks, cmd, testutil, test, proto)"; \
		if [ "$(VERBOSE)" = "true" ]; then \
			go test -v -race -count=1 -timeout 180s -coverprofile=coverage.out -covermode=atomic $$(go list ./... | grep -v mocks | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | grep -v "^github.com/g8e-ai/g8e/internal/protocol/proto/"); \
		else \
			go test -race -count=1 -timeout 180s -coverprofile=coverage.out -covermode=atomic $$(go list ./... | grep -v mocks | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | grep -v "^github.com/g8e-ai/g8e/internal/protocol/proto/"); \
		fi; \
	fi
	@echo "Coverage report generated in coverage.out"
	@go tool cover -func=coverage.out | tail -1
	@if [ -z "$(PKG)" ]; then \
		COVERAGE=$$(go tool cover -func=coverage.out | grep -v "internal/protocol/proto" | grep -v "mocks" | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | tail -1 | awk '{print $$3}' | sed 's/%//'); \
		if [ $$(echo "$$COVERAGE < 60" | bc -l) -eq 1 ]; then \
			echo "Coverage $$COVERAGE% is below 60% threshold"; \
			exit 1; \
		fi; \
		echo "Coverage $$COVERAGE% meets 60% threshold"; \
	fi

.PHONY: test-shuffle
test-shuffle:
	@go test -race -count=1 -shuffle=on -timeout 180s ./...

.PHONY: update-golden
update-golden:
	@echo "Updating scenario test golden files..."
	@G8E_UPDATE_GOLDEN=1 go test -tags=integration -count=1 ./test/scenario -run TestScenarios
	@echo "Golden files updated."

# =============================================================================
# LINT & QUALITY
# =============================================================================
.PHONY: lint
lint: lint-no-embedded-newlines vulncheck validate-doctrines
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


# =============================================================================
# DOCTRINE MANAGEMENT
# =============================================================================
.PHONY: ingest-doctrines
ingest-doctrines:
	@echo "Doctrine ingestion scripts removed. Use manual ingestion if needed."

.PHONY: update-doctrines
update-doctrines:
	@echo "Updating doctrine sources..."
	@if [ -d "/tmp/coreruleset" ]; then \
		cd /tmp/coreruleset && git pull; \
	else \
		git clone --depth 1 https://github.com/coreruleset/coreruleset.git /tmp/coreruleset; \
	fi
	@curl -sSL https://raw.githubusercontent.com/gitleaks/gitleaks/master/config/gitleaks.toml -o /tmp/gitleaks.toml
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
	@rm -f g8e
	@rm -rf build/
	@rm -f *.sha256
	@echo "Clean complete."


# =============================================================================
# CI/CD (LOCAL)
# =============================================================================
.PHONY: ci
ci: ci-platform
	@echo "CI complete."

.PHONY: ci-platform
ci-platform: _ci-verify-proto _ci-lint _ci-vulncheck _ci-test
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
	@./bin/g8e platform start
	@G8E_STRICT_CONSTANTS_LINT=1 go test -race -timeout 180s -coverprofile=coverage.out -covermode=atomic $$(go list ./... | grep -v mocks | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | grep -v "^github.com/g8e-ai/g8e/internal/protocol/proto/")
	@COVERAGE=$$(go tool cover -func=coverage.out | grep -v "internal/protocol/proto" | grep -v "mocks" | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | tail -1 | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < 60" | bc -l) -eq 1 ]; then \
		echo "Coverage $$COVERAGE% is below 60% threshold"; \
		exit 1; \
	fi; \
	echo "Coverage $$COVERAGE% meets 60% threshold"
	@./bin/g8e platform stop
