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
	@echo "  ci-apps       Run app-layer CI (g8ee tests, requires LLM creds)"
	@echo ""
	@echo "Development:"
	@echo "  generate      Generate all protocol artifacts (proto + constants + docs)"
	@echo "  proto         Generate all Protobuf code (Go and Python)"
	@echo "  constants     Generate all constants and sync documentation ports"
	@echo "  clean-constants  Remove generated constants files"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  lint-no-bare-session-id  Check for bare session_id regression"
	@echo "  lint-no-hand-authored-events  Check for hand-authored events.py regression"
	@echo "  first-issues  Find good first issues in the codebase"
	@echo "  clean         Remove build artifacts and runtime state"
	@echo ""
	@echo "Documentation:"
	@echo "  docs-build    Build MkDocs documentation site"
	@echo "  docs-serve    Serve MkDocs documentation locally (live reload)"
	@echo ""
	@echo "Services:"
	@echo "  build         Build the Operator service (g8e binary)"
	@echo "  test-g8eo     Run Operator tests"
	@echo "  lint-g8eo     Run Operator linters (golangci-lint)"
	@echo "  lint-g8ee     Run Engine linters (ruff, pyright)"
	@echo "  vulncheck-g8eo Run Operator vulnerability check"
	@echo "  test-g8ee     Run Engine tests"

# =============================================================================
# PROTOBUF GENERATION
# =============================================================================
.PHONY: generate
generate: proto constants

.PHONY: constants
constants:
	@echo "Generating Go constants from JSON source..."
	@cd services/g8eo/internal/constants && go run generate_registry.go
	@echo "Exporting constants to JSON and Python via Go exporter..."
	@cd services/g8eo/cmd/exporter && go run main.go -root $(PWD)
	@echo "Constants generation complete."

.PHONY: proto
proto: buf-install
	@if command -v buf &> /dev/null || [ -f "./buf" ]; then \
		echo "Generating Go Protobuf code with Buf..."; \
		$(BUF) generate protocol/proto; \
		if [ -f ".venv/bin/python" ]; then \
			echo "Generating Python Protobuf code locally..."; \
			.venv/bin/python -m grpc_tools.protoc -Iprotocol/proto --python_out=services/g8ee/app/proto protocol/proto/*.proto; \
		fi \
	elif [ -d "services/g8ee/app/proto" ] && [ -f "services/g8ee/app/proto/common_pb2.py" ]; then \
		echo "Buf not found and system is offline/air-gapped. Utilizing pre-generated protocol files."; \
	else \
		echo "Error: Buf not found and no pre-generated protocol files found. Network access required for initial setup." >&2; \
		exit 1; \
	fi
	@echo "Post-processing Python code..."
	@touch services/g8ee/app/proto/__init__.py
	@# Also generate for the evals harness
	@mkdir -p evals/g8e_evals/proto
	@cp services/g8ee/app/proto/*_pb2*.py evals/g8e_evals/proto/
	@touch evals/g8e_evals/proto/__init__.py
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		find services/g8ee/app/proto -name "*_pb2*.py" -exec sed -i '' 's/^import \(.*_pb2\)/from . import \1/' {} +; \
		find evals/g8e_evals/proto -name "*_pb2*.py" -exec sed -i '' 's/^import \(.*_pb2\)/from . import \1/' {} +; \
	else \
		find services/g8ee/app/proto -name "*_pb2*.py" -exec sed -i 's/^import \(.*_pb2\)/from . import \1/' {} +; \
		find evals/g8e_evals/proto -name "*_pb2*.py" -exec sed -i 's/^import \(.*_pb2\)/from . import \1/' {} +; \
	fi
	@echo "Protobuf generation complete."

.PHONY: proto-force
proto-force: buf-install
	@echo "Force generating Protobuf code..."
	@$(BUF) generate protocol/proto
	@if [ -f ".venv/bin/python" ]; then \
		.venv/bin/python -m grpc_tools.protoc -Iprotocol/proto --python_out=services/g8ee/app/proto protocol/proto/*.proto; \
	fi
	@echo "Post-processing Python code..."
	@touch services/g8ee/app/proto/__init__.py
	@# Also generate for the evals harness
	@mkdir -p evals/g8e_evals/proto
	@cp services/g8ee/app/proto/*_pb2*.py evals/g8e_evals/proto/
	@touch evals/g8e_evals/proto/__init__.py
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		find services/g8ee/app/proto -name "*_pb2*.py" -exec sed -i '' 's/^import \(.*_pb2\)/from . import \1/' {} +; \
		find evals/g8e_evals/proto -name "*_pb2*.py" -exec sed -i '' 's/^import \(.*_pb2\)/from . import \1/' {} +; \
	else \
		find services/g8ee/app/proto -name "*_pb2*.py" -exec sed -i 's/^import \(.*_pb2\)/from . import \1/' {} +; \
		find evals/g8e_evals/proto -name "*_pb2*.py" -exec sed -i 's/^import \(.*_pb2\)/from . import \1/' {} +; \
	fi
	@echo "Protobuf generation complete."

.PHONY: buf-install
buf-install:
	@if ! command -v buf &> /dev/null && [ ! -f "./buf" ]; then \
		if command -v go &> /dev/null; then \
			echo "Installing Buf natively via Go toolchain..."; \
			GOBIN=$$(pwd) go install github.com/bufbuild/buf/cmd/buf@v1.30.0; \
		else \
			echo "Go not found, attempting direct download..."; \
			curl -sSL "https://github.com/bufbuild/buf/releases/latest/download/buf-$$(uname -s)-$$(uname -m)" -o ./buf && chmod +x ./buf || \
			echo "Warning: Failed to download Buf. Proceeding with existing protocol files if available."; \
		fi \
	fi

# =============================================================================
# LINTING
# =============================================================================
.PHONY: lint-no-bare-session-id
lint-no-bare-session-id:
	@echo "Checking for bare session_id regression..."
	@if grep -rE "\bsession_id\b" . \
		--exclude-dir={.git,vendor,node_modules,.g8e,.ruff_cache,.venv,dist,build,__pycache__,.local.dev,.github} \
		--exclude={*.pb.go,*_pb2.py,*_pb2_grpc.py,*.pyc,Makefile,*.sh,*.json} \
		-I; then \
		echo "Error: Bare 'session_id' found. Use 'operator_session_id', 'cli_session_id', or 'web_session_id' instead."; \
		exit 1; \
	fi
	@echo "No bare session_id found."

.PHONY: lint-no-hand-authored-events
lint-no-hand-authored-events:
	@echo "Checking for hand-authored events.py regression..."
	@if [ -f "services/g8ee/app/constants/events.py" ]; then \
		echo "Error: Hand-authored 'services/g8ee/app/constants/events.py' found. Use 'generated_events.py' instead."; \
		exit 1; \
	fi
	@echo "No hand-authored events.py found."

.PHONY: first-issues
first-issues:
	@echo "Searching for good first issues (TODO comments)..."
	@grep -rni 'TODO' . \
		--exclude-dir={.git,vendor,node_modules,.g8e,.ruff_cache,.venv,dist,build,__pycache__,.local.dev,.github} \
		--exclude={*.pb.go,*_pb2.py,*_pb2_grpc.py,*.pyc,Makefile} \
		-I || echo "No TODOs found."

.PHONY: clean-constants
clean-constants:
	@echo "Cleaning generated constants..."
	@rm -rf services/g8eo/internal/constants/headers_generated.go
	@rm -rf services/g8eo/internal/constants/status_generated.go
	@rm -rf services/g8eo/internal/constants/registry.go
	@rm -rf services/g8ee/app/constants/generated_*.py
	@echo "Constants clean complete."

.PHONY: clean
clean:
	@echo "Cleaning up build artifacts and runtime state..."
	@$(MAKE) --no-print-directory -C services/g8eo clean
	@$(MAKE) clean-constants
	@rm -rf .g8e/
	@rm -f ./g8e
	@find . -name "*.pyc" -delete
	@find . -name "__pycache__" -type d -exec rm -rf {} +
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
	@echo "Running full CI pipeline (substrate + apps)..."
	@echo "Note: apps-g8ee requires G8E_LLM_PRIMARY_API_KEY environment variable"
	@if [ -n "$$G8E_LLM_PRIMARY_API_KEY" ]; then \
		$(MAKE) ci-apps; \
	else \
		echo "Skipping apps-g8ee (G8E_LLM_PRIMARY_API_KEY not set)"; \
		echo "Set it with: export G8E_LLM_PRIMARY_API_KEY=your_key"; \
	fi
	@echo "CI complete."

.PHONY: ci-substrate
ci-substrate: _ci-verify-proto _ci-lint-g8eo _ci-vulncheck-g8eo _ci-test-g8eo _ci-docs
	@echo "Substrate CI complete."

.PHONY: ci-apps
ci-apps: _ci-apps-g8ee
	@echo "Apps CI complete."

.PHONY: _ci-verify-proto
_ci-verify-proto:
	@echo "=== verify-proto ==="
	@$(MAKE) proto
	@$(MAKE) constants
	@CHANGES=$$(git status --porcelain | grep -E "^\s*M.*\.go$$|^\s*M.*\.py$$|^\s*M.*\.sh$$" || true); \
	if [ -n "$$CHANGES" ]; then \
		echo "Error: Generated constant files are out of sync with protocol/constants/*.json"; \
		echo "$$CHANGES"; \
		git diff; \
		exit 1; \
	fi
	@CHANGES=$$(git status --porcelain | grep -E "^\s*M.*\.pb\.go$$|^\s*M.*_pb2.*\.py$$|^\s*M.*\.proto$$" || true); \
	if [ -n "$$CHANGES" ]; then \
		echo "Error: Generated proto files are out of sync with protocol/proto/*.proto"; \
		echo "$$CHANGES"; \
		git diff -- $$(git status --porcelain | grep -E "^\s*M" | awk '{print $$2}'); \
		exit 1; \
	fi
	@$(MAKE) lint-no-bare-session-id
	@$(MAKE) lint-no-hand-authored-events
	@$(MAKE) validate-doctrines
	@cd services/g8eo/internal/constants && go run check_registry.go

.PHONY: _ci-lint-g8eo
_ci-lint-g8eo:
	@echo "=== lint-g8eo ==="
	@$(MAKE) lint-g8eo
	@cd protocol && golangci-lint run

.PHONY: _ci-vulncheck-g8eo
_ci-vulncheck-g8eo:
	@echo "=== vulncheck-g8eo ==="
	@$(MAKE) vulncheck-g8eo

.PHONY: _ci-test-g8eo
_ci-test-g8eo:
	@echo "=== test-g8eo ==="
	@./g8e platform start
	@cd services/g8eo && go test -race -timeout 180s -coverprofile=coverage.out -covermode=atomic ./...
	@COVERAGE=$$(cd services/g8eo && go tool cover -func=coverage.out | grep -v "internal/protocol/proto" | grep -v "/mocks/" | tail -1 | awk '{print $$3}' | sed 's/%//'); \
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
		markdownlint . -c .markdownlint.json --ignore node_modules; \
	else \
		echo "markdownlint not found, skipping docs-lint. Install with: npm install -g markdownlint-cli"; \
	fi
	@echo "=== docs-build ==="
	@$(MAKE) docs-build

.PHONY: _ci-apps-g8ee
_ci-apps-g8ee:
	@echo "=== apps-g8ee ==="
	@./g8e platform start
	@./g8e apps start g8ee
	@./g8e test g8ee -p gemini -k "$$G8E_LLM_PRIMARY_API_KEY" -m gemini-3.1-pro-preview-customtools -a gemini-3-flash-preview -l gemini-3.1-flash-lite -j auto -- tests
	@./g8e platform stop

# =============================================================================
# SERVICE DISPATCH
# =============================================================================
.PHONY: build
build: build-g8eo
	@echo "All builds complete."

.PHONY: build-g8eo
build-g8eo:
	@$(MAKE) --no-print-directory -C services/g8eo build
	@ln -sf services/g8eo/build/linux-amd64/g8e ./g8e

.PHONY: test-g8eo
test-g8eo:
	@$(MAKE) --no-print-directory -C services/g8eo test


.PHONY: lint-g8eo
lint-g8eo:
	@$(MAKE) --no-print-directory -C services/g8eo lint

.PHONY: lint-g8ee
lint-g8ee:
	@$(MAKE) --no-print-directory -C services/g8ee lint

.PHONY: vulncheck-g8eo
vulncheck-g8eo:
	@$(MAKE) --no-print-directory -C services/g8eo vulncheck

.PHONY: test-g8ee
test-g8ee:
	@./g8e test g8ee

# =============================================================================
# DOCUMENTATION
# =============================================================================
.PHONY: docs-build
docs-build: constants
	@echo "Building MkDocs documentation..."
	@if [ -f ".venv/bin/python" ]; then \
		.venv/bin/python -m mkdocs build -f docs/mkdocs.yml; \
	else \
		echo "Error: Python venv not found. Run setup first."; \
		exit 1; \
	fi

.PHONY: docs-serve
docs-serve:
	@echo "Serving MkDocs documentation locally..."
	@if [ -f ".venv/bin/python" ]; then \
		.venv/bin/python -m mkdocs serve -f docs/mkdocs.yml -a 0.0.0.0:8000; \
	else \
		echo "Error: Python venv not found. Run setup first."; \
		exit 1; \
	fi
