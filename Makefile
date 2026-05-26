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
	@echo "CI/CD (Local):"
	@echo "  ci            Run full CI pipeline locally (mirrors GitHub Actions)"
	@echo "  ci-substrate  Run substrate-only CI (g8eo, protocol, proto, docs)"
	@echo ""
	@echo "Protocol Generation:"
	@echo "  generate      Generate all protocol artifacts (proto + constants)"
	@echo "  proto         Generate all Protobuf code (Go)"
	@echo "  constants     Generate all constants and sync documentation ports"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  protoc-install Install protoc compiler"
	@echo ""
	@echo "Build:"
	@echo "  build         Build all services (cli + operator)"
	@echo "  build-cli     Build g8e CLI wrapper"
	@echo "  build-operator Build g8e operator binary (system type)"
	@echo "  build-operator-system Build g8e operator (system type)"
	@echo "  build-operator-cloud Build g8e operator (cloud type)"
	@echo "  build-operator-aws Build g8e operator (AWS)"
	@echo "  build-operator-gcp Build g8e operator (GCP)"
	@echo "  build-operator-azure Build g8e operator (Azure)"
	@echo "  build-operator-g8ep Build g8e operator (g8ep)"
	@echo "  build-compressed Build g8e operator with compression (-s -w -trimpath)"
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
	@echo "Documentation:"
	@echo "  docs          Run all documentation tasks (cli + build)"
	@echo "  docs-cli      Auto-generate CLI reference documentation"
	@echo "  docs-build    Build MkDocs documentation site (via Docker)"
	@echo "  docs-serve    Serve MkDocs documentation locally at :8000 (via Docker)"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean         Remove all build artifacts, runtime state, and generated files"
	@echo "  clean-constants Remove generated constants files only"

# =============================================================================
# PROTOCOL GENERATION
# =============================================================================
.PHONY: generate
generate: proto constants

.PHONY: constants
constants:
	@echo "Generating Go constants from JSON source..."
	@cd internal/constants && go run generate_registry.go
	@echo "Exporting constants to JSON and Python via Go exporter..."
	@cd cmd/exporter && go run main.go -root $(PWD)
	@echo "Constants generation complete."

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

# =============================================================================
# BUILD
# =============================================================================
.PHONY: build
build: constants build-cli build-operator
	@echo "All builds complete."

.PHONY: build-cli
build-cli:
	@echo "Building g8e CLI wrapper..."
	@mkdir -p bin
	@go build -o bin/g8e ./cmd/g8e
	@ln -sf bin/g8e ./g8e
	@echo "CLI wrapper build complete."

.PHONY: build-operator
build-operator:
	@echo "Building g8e operator (default)..."
	@$(MAKE) build-operator-system

.PHONY: build-operator-system
build-operator-system:
	@echo "Building g8e operator (system type)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -o bin/g8e ./cmd/g8eo
	@ln -sf bin/g8e ./g8e
	@echo "System operator build complete."

.PHONY: build-operator-cloud
build-operator-cloud:
	@echo "Building g8e operator (cloud type)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -o bin/g8e ./cmd/g8eo
	@ln -sf bin/g8e ./g8e
	@echo "Cloud operator build complete."

.PHONY: build-operator-aws
build-operator-aws:
	@echo "Building g8e operator (AWS)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -tags aws -o bin/g8e ./cmd/g8eo
	@ln -sf bin/g8e ./g8e
	@echo "AWS operator build complete."

.PHONY: build-operator-gcp
build-operator-gcp:
	@echo "Building g8e operator (GCP)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -tags gcp -o bin/g8e ./cmd/g8eo
	@ln -sf bin/g8e ./g8e
	@echo "GCP operator build complete."

.PHONY: build-operator-azure
build-operator-azure:
	@echo "Building g8e operator (Azure)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -tags azure -o bin/g8e ./cmd/g8eo
	@ln -sf bin/g8e ./g8e
	@echo "Azure operator build complete."

.PHONY: build-operator-g8ep
build-operator-g8ep:
	@echo "Building g8e operator (g8ep)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -tags g8ep -o bin/g8e ./cmd/g8eo
	@ln -sf bin/g8e ./g8e
	@echo "g8ep operator build complete."

.PHONY: build-compressed
build-compressed:
	@echo "Building g8e operator with compression..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	go build -ldflags "-s -w -X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -trimpath -o bin/g8e ./cmd/g8eo
	@ln -sf bin/g8e ./g8e
	@echo "Compressed operator build complete."

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
lint: vulncheck validate-doctrines
	@golangci-lint run
	@echo "All linting and quality checks complete."

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
	@$(MAKE) clean-constants
	@rm -rf .g8e/
	@rm -f ./g8e
	@rm -rf bin/
	@rm -rf build/
	@echo "Clean complete."

.PHONY: clean-constants
clean-constants:
	@echo "Cleaning generated constants..."
	@rm -rf internal/constants/headers_generated.go
	@rm -rf internal/constants/status_generated.go
	@rm -rf internal/constants/registry.go
	@echo "Constants clean complete."

# =============================================================================
# CI/CD (LOCAL)
# =============================================================================
.PHONY: ci
ci: ci-substrate
	@echo "CI complete."

.PHONY: ci-substrate
ci-substrate: _ci-verify-proto _ci-lint _ci-vulncheck _ci-test _ci-docs
	@echo "Substrate CI complete."

.PHONY: _ci-verify-proto
_ci-verify-proto:
	@echo "=== verify-proto ==="
	@$(MAKE) protoc-install
	@$(MAKE) proto
	@$(MAKE) constants
	@CHANGES=$$(git status --porcelain | grep -E "^\s*M.*\.go$$|^\s*M.*\.sh$$" || true); \
	if [ -n "$$CHANGES" ]; then \
		echo "Error: Generated constant files are out of sync with protocol/constants/*.json"; \
		echo "$$CHANGES"; \
		git diff; \
		exit 1; \
	fi
	@CHANGES=$$(git status --porcelain | grep -E "^\s*M.*\.pb\.go$$|^\s*M.*\.proto$$" || true); \
	if [ -n "$$CHANGES" ]; then \
		echo "Error: Generated proto files are out of sync with protocol/proto/*.proto"; \
		echo "$$CHANGES"; \
		git diff -- $$(git status --porcelain | grep -E "^\s*M" | awk '{print $$2}'); \
		exit 1; \
	fi
	@$(MAKE) validate-doctrines
	@cd internal/constants && go run check_registry.go

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
	@./g8e platform start
	@G8E_STRICT_CONSTANTS_LINT=1 go test -race -timeout 180s -coverprofile=coverage.out -covermode=atomic $$(go list ./... | grep -v mocks | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | grep -v "^github.com/g8e-ai/g8e/internal/protocol/proto/")
	@COVERAGE=$$(go tool cover -func=coverage.out | grep -v "internal/protocol/proto" | grep -v "mocks" | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | tail -1 | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < 60" | bc -l) -eq 1 ]; then \
		echo "Coverage $$COVERAGE% is below 60% threshold"; \
		exit 1; \
	fi; \
	echo "Coverage $$COVERAGE% meets 60% threshold"
	@./g8e platform stop

.PHONY: _ci-docs
_ci-docs:
	@echo "=== docs-lint ==="
	@if command -v markdownlint >/dev/null 2>&1; then \
		markdownlint . -c docs/.markdownlint.json --ignore node_modules; \
	else \
		echo "markdownlint not found, skipping docs-lint. Install with: npm install -g markdownlint-cli"; \
	fi
	@echo "=== docs-build ==="
	@if command -v docker >/dev/null 2>&1; then \
		$(MAKE) docs-build; \
	else \
		echo "Warning: docker not found, skipping docs-build."; \
		if [ "$$CI" = "true" ]; then \
			echo "Error: docker must be available in CI environment." >&2; \
			exit 1; \
		fi \
	fi

# =============================================================================
# DOCUMENTATION
# =============================================================================
.PHONY: docs
docs: docs-cli docs-build
	@echo "All documentation tasks complete."
.PHONY: docs-cli
docs-cli:
	@echo "Building g8e binary for CLI help generation..."
	@mkdir -p bin
	@go build -o bin/g8e ./cmd/g8e
	@echo "Generating CLI reference documentation..."
	@echo "# CLI Reference" > docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "This reference is auto-generated from the Cobra CLI help output." >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "## g8e Root Help" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "## setup" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e setup --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "## platform" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### platform start" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform start --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### platform stop" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform stop --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### platform status" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform status --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### platform restart" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform restart --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### platform logs" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform logs --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### platform settings" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform settings --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### platform reset" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform reset --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### platform clean" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e platform clean --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "## auth" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e auth --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### auth bootstrap" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e auth bootstrap --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### auth login" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e auth login --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### auth logout" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e auth logout --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "## data" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### data users" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data users --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### data operators" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data operators --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### data device-links" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data device-links --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "#### data device-links list" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data device-links list --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "#### data device-links create" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data device-links create --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "#### data device-links delete" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data device-links delete --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### data settings" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data settings --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### data store" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data store --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### data audit" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data audit --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "#### data audit list" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data audit list --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "#### data audit summary" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e data audit summary --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "## test" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e test --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### test unit" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e test unit --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### test integration" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e test integration --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### test g8eo" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e test g8eo --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### test ci" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e test ci --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### test chaos" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e test chaos --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### test scenario" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e test scenario --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "## security" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e security --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### security validate" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e security validate --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "## vars" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e vars --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### vars list" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e vars list --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### vars set" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e vars set --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### vars get" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e vars get --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "### vars unset" >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@./bin/g8e vars unset --help >> docs/guides/cli.md
	@echo "\`\`\`" >> docs/guides/cli.md
	@echo "" >> docs/guides/cli.md
	@echo "CLI reference documentation generated successfully."

.PHONY: docs-build
docs-build: constants
	@echo "Building MkDocs documentation site via Docker..."
	@docker run --rm \
		-v $(PWD):/repo \
		-w /repo \
		squidfunk/mkdocs-material:9.7.5 build --strict --quiet

.PHONY: docs-serve
docs-serve:
	@echo "Serving MkDocs documentation at http://localhost:8000 ..."
	@docker run --rm -it \
		-p 8000:8000 \
		-v $(PWD):/repo \
		-w /repo \
		squidfunk/mkdocs-material:9.7.5 serve --dev-addr 0.0.0.0:8000 --quiet
