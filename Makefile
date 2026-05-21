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
	@echo "Development:"
	@echo "  generate      Generate all protocol artifacts (proto + constants + docs)"
	@echo "  proto         Generate all Protobuf code (Go and Python)"
	@echo "  constants     Generate all constants and sync documentation ports"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  lint-no-bare-session-id  Check for bare session_id regression"
	@echo "  first-issues  Find good first issues in the codebase"
	@echo "  clean         Remove build artifacts and runtime state"
	@echo ""
	@echo "Services:"
	@echo "  build-g8eo    Build the Operator service"
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
	@echo "Building constants exporter from Go SSOT..."
	@cd services/g8eo && go build -o ../../bin/g8e.exporter ./cmd/exporter
	@./bin/g8e.exporter --root .

.PHONY: proto
proto: buf-install
	@if command -v buf &> /dev/null || [ -f "./buf" ]; then \
		echo "Generating Go Protobuf code with Buf..."; \
		$(BUF) generate protocol/proto; \
		if [ -f "services/g8ee/.venv/bin/python" ]; then \
			echo "Generating Python Protobuf code locally..."; \
			services/g8ee/.venv/bin/python -m grpc_tools.protoc -Iprotocol/proto --python_out=services/g8ee/app/proto protocol/proto/*.proto; \
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
	@if [ -f "services/g8ee/.venv/bin/python" ]; then \
		services/g8ee/.venv/bin/python -m grpc_tools.protoc -Iprotocol/proto --python_out=services/g8ee/app/proto protocol/proto/*.proto; \
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
		--exclude={*.pb.go,*_pb2.py,*_pb2_grpc.py,*.pyc,Makefile} \
		-I; then \
		echo "Error: Bare 'session_id' found. Use 'operator_session_id', 'cli_session_id', or 'web_session_id' instead."; \
		exit 1; \
	fi
	@echo "No bare session_id found."

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
	@rm -rf protocol/constants/*.json
	@rm -rf protocol/python/g8e_protocol/generated_*.py
	@rm -rf services/g8ee/app/constants/generated_*.py
	@rm -rf scripts/cmd/env_vars.sh scripts/cmd/paths.sh scripts/cmd/headers.sh
	@rm -rf ./bin/g8e.exporter
	@echo "Constants clean complete."

.PHONY: clean
clean:
	@echo "Cleaning up build artifacts and runtime state..."
	@$(MAKE) -C services/g8eo clean
	@$(MAKE) -C services/g8eg clean
	@$(MAKE) clean-constants
	@rm -rf .g8e/
	@find . -name "*.pyc" -delete
	@find . -name "__pycache__" -type d -exec rm -rf {} +
	@echo "Clean complete."

# =============================================================================
# SERVICE DISPATCH
# =============================================================================
.PHONY: build-g8eo
build-g8eo:
	@$(MAKE) -C services/g8eo build

.PHONY: build-g8eg
build-g8eg:
	@$(MAKE) -C services/g8eg build

.PHONY: test-g8eo
test-g8eo:
	@$(MAKE) -C services/g8eo test

.PHONY: lint-g8eo
lint-g8eo:
	@$(MAKE) -C services/g8eo lint

.PHONY: lint-g8ee
lint-g8ee:
	@$(MAKE) -C services/g8ee lint

.PHONY: vulncheck-g8eo
vulncheck-g8eo:
	@$(MAKE) -C services/g8eo vulncheck

.PHONY: test-g8ee
test-g8ee:
	@./g8e test g8ee
