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
BUF := $(shell command -v buf 2>/dev/null || echo "./buf")
PROTOC := $(shell command -v protoc 2>/dev/null || echo "/usr/local/bin/protoc")
PROTOC_GEN_GO := $(shell go list -m -f '{{.Version}}' google.golang.org/protobuf 2>/dev/null || echo "v1.35.2")
PROTOC_MIN_VERSION := 21.0

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
	@echo "Development:"
	@echo "  generate      Generate all protocol artifacts (proto + constants + docs)"
	@echo "  proto         Generate all Protobuf code (Go)"
	@echo "  constants     Generate all constants and sync documentation ports"
	@echo "  clean-constants  Remove generated constants files"
	@echo "  update-golden Update scenario test golden files (use after intentional changes)"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  lint-no-bare-session-id  Check for bare session_id regression"
	@echo "  first-issues  Find good first issues in the codebase"
	@echo "  clean         Remove build artifacts and runtime state"
	@echo ""
	@echo "Documentation:"
	@echo "  docs-cli      Auto-generate CLI reference documentation"
	@echo "  docs-build    Build MkDocs documentation site (via Docker)"
	@echo "  docs-serve    Serve MkDocs documentation locally at :8000 (via Docker)"
	@echo ""
	@echo "Services:"
	@echo "  build         Build the Operator service (g8e binary)"
	@echo "  test-g8eo     Run Operator tests"
	@echo "  lint-g8eo     Run Operator linters (golangci-lint)"
	@echo "  vulncheck-g8eo Run Operator vulnerability check"

# =============================================================================
# PROTOBUF GENERATION
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

.PHONY: buf-install
buf-install:
	@if ! command -v buf &> /dev/null && [ ! -f "./buf" ]; then \
		if command -v go &> /dev/null; then \
			echo "Installing Buf natively via Go toolchain..."; \
			GOBIN=$$(pwd) go install github.com/bufbuild/buf/cmd/buf@v1.69.0; \
		else \
			echo "Go not found, attempting direct download..."; \
			curl -sSL "https://github.com/bufbuild/buf/releases/latest/download/buf-$$(uname -s)-$$(uname -m)" -o ./buf && chmod +x ./buf || \
			echo "Warning: Failed to download Buf. Proceeding with existing protocol files if available."; \
		fi \
	fi

.PHONY: protoc-install
protoc-install:
	@if ! command -v protoc &> /dev/null; then \
		echo "Installing protoc v28.3..."; \
		cd /tmp && curl -sSL https://github.com/protocolbuffers/protobuf/releases/download/v28.3/protoc-28.3-linux-x86_64.zip -o protoc.zip && \
		unzip -o protoc.zip -d protoc && \
		sudo cp protoc/bin/protoc /usr/local/bin/protoc && \
		sudo chmod +x /usr/local/bin/protoc && \
		rm -rf /tmp/protoc /tmp/protoc.zip; \
	fi
	@echo "Verifying protoc version compatibility..."
	@PROTOC_VERSION=$$($(PROTOC) --version | grep -oP '\d+\.\d+'); \
	PROTOC_MAJOR=$$(echo $$PROTOC_VERSION | cut -d. -f1); \
	if [ "$$(echo "$$PROTOC_MAJOR < $(PROTOC_MIN_VERSION)" | bc -l)" -eq 1 ]; then \
		echo "Error: protoc version $$PROTOC_VERSION is too old. Minimum required: $(PROTOC_MIN_VERSION)"; \
		exit 1; \
	fi
	@echo "protoc version $$PROTOC_VERSION is compatible."

# =============================================================================
# LINTING
# =============================================================================
.PHONY: lint-no-bare-session-id
lint-no-bare-session-id:
	@echo "Checking for bare session_id regression..."
	@if grep -rE "\bsession_id\b" . \
		--exclude-dir={.git,vendor,node_modules,.g8e,.local.dev,.github,docs,site} \
		--exclude={*.pb.go,Makefile,*.json} \
		-I; then \
		echo "Error: Bare 'session_id' found. Use 'operator_session_id', 'cli_session_id', or 'web_session_id' instead."; \
		exit 1; \
	fi
	@echo "No bare session_id found."


.PHONY: first-issues
first-issues:
	@echo "Searching for good first issues (TODO comments)..."
	@grep -rni 'TODO' . \
		--exclude-dir={.git,vendor,node_modules,.g8e,.local.dev,.github} \
		--exclude={*.pb.go,Makefile} \
		-I || echo "No TODOs found."

.PHONY: clean-constants
clean-constants:
	@echo "Cleaning generated constants..."
	@rm -rf internal/constants/headers_generated.go
	@rm -rf internal/constants/status_generated.go
	@rm -rf internal/constants/registry.go
	@echo "Constants clean complete."

.PHONY: update-golden
update-golden:
	@echo "Updating scenario test golden files..."
	@G8E_UPDATE_GOLDEN=1 go test -tags=integration ./test/scenario -run TestScenarios
	@echo "Golden files updated."

.PHONY: clean
clean:
	@echo "Cleaning up build artifacts and runtime state..."
	@$(MAKE) clean-constants
	@rm -rf .g8e/
	@rm -f ./g8e
	@rm -rf bin/
	@rm -rf build/
	@echo "Clean complete."

# =============================================================================
# DOCTRINE INGESTION
# =============================================================================
.PHONY: ingest-doctrines
ingest-doctrines:
	@echo "Doctrine ingestion scripts removed. Use manual ingestion if needed."

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
# CI/CD (LOCAL)
# =============================================================================
.PHONY: ci
ci: ci-substrate
	@echo "CI complete."

.PHONY: ci-substrate
ci-substrate: _ci-verify-proto _ci-lint-g8eo _ci-vulncheck-g8eo _ci-test-g8eo _ci-docs
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
	@$(MAKE) lint-no-bare-session-id
	@$(MAKE) validate-doctrines
	@cd internal/constants && go run check_registry.go

.PHONY: _ci-lint-g8eo
_ci-lint-g8eo:
	@echo "=== lint-g8eo ==="
	@$(MAKE) lint-g8eo

.PHONY: _ci-vulncheck-g8eo
_ci-vulncheck-g8eo:
	@echo "=== vulncheck-g8eo ==="
	@$(MAKE) vulncheck-g8eo

.PHONY: _ci-test-g8eo
_ci-test-g8eo:
	@echo "=== test-g8eo ==="
	@./g8e platform start
	@G8E_STRICT_CONSTANTS_LINT=1 go test -race -timeout 180s -coverprofile=coverage.out -covermode=atomic ./...
	@COVERAGE=$$(go tool cover -func=coverage.out | grep -v "internal/protocol/proto" | grep -v "/mocks/" | tail -1 | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < 85" | bc -l) -eq 1 ]; then \
		echo "Coverage $$COVERAGE% is below 85% threshold"; \
		exit 1; \
	fi; \
	echo "Coverage $$COVERAGE% meets 85% threshold"
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
# SERVICE DISPATCH
# =============================================================================
.PHONY: build
build: build-g8eo
	@echo "All builds complete."

.PHONY: build-cli
build-cli:
	@echo "Building g8e CLI wrapper..."
	@mkdir -p bin
	@go build -o bin/g8e ./cmd/g8e
	@ln -sf bin/g8e ./g8e
	@echo "CLI wrapper build complete."

.PHONY: build-g8eo
build-g8eo:
	@echo "Building g8e operator..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	PLATFORM=$$(uname -s)_$$(uname -m); \
	go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$PLATFORM" -o bin/g8e ./cmd/g8eo
	@ln -sf bin/g8e ./g8e
	@echo "Build complete."

.PHONY: test-g8eo
test-g8eo:
	@go test -race -timeout 180s ./...

.PHONY: lint-g8eo
lint-g8eo:
	@golangci-lint run

.PHONY: vulncheck-g8eo
vulncheck-g8eo:
	@govulncheck ./...

# =============================================================================
# DOCUMENTATION
# =============================================================================
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
