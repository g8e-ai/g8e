# g8e Platform Root Makefile
# Industry standard orchestration for multi-component proto generation and builds.

SHELL := /bin/bash
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
	@echo "  proto         Generate all Protobuf code (Go and Python)"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  lint-no-bare-session-id  Check for bare session_id regression"
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
.PHONY: proto
proto: buf-install
	@echo "Generating Protobuf code with Buf..."
	@$(BUF) generate protocol/proto
	@echo "Post-processing Python code..."
	@touch services/g8ee/app/proto/__init__.py
	@find services/g8ee/app/proto -name "*_pb2*.py" -exec sed -i 's/^import \(.*_pb2\)/from . import \1/' {} +
	@# Also generate for the evals harness
	@mkdir -p evals/g8e_evals/proto
	@cp services/g8ee/app/proto/*_pb2*.py evals/g8e_evals/proto/
	@touch evals/g8e_evals/proto/__init__.py
	@find evals/g8e_evals/proto -name "*_pb2*.py" -exec sed -i 's/^import \(.*_pb2\)/from . import \1/' {} +
	@echo "Protobuf generation complete."

.PHONY: buf-install
buf-install:
	@if ! command -v buf &> /dev/null && [ ! -f "./buf" ]; then \
		echo "Buf not found, downloading..."; \
		curl -sSL "https://github.com/bufbuild/buf/releases/latest/download/buf-$$(uname -s)-$$(uname -m)" -o ./buf; \
		chmod +x ./buf; \
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

# =============================================================================
# SERVICE DISPATCH
# =============================================================================
.PHONY: build-g8eo
build-g8eo:
	@$(MAKE) -C services/g8eo build

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
