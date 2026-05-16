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
	@echo ""
	@echo "Services:"
	@echo "  build-g8eo    Build the Operator service"
	@echo "  test-g8eo     Run Operator tests"
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
	@echo "Protobuf generation complete."

.PHONY: buf-install
buf-install:
	@if ! command -v buf &> /dev/null && [ ! -f "./buf" ]; then \
		echo "Buf not found, downloading..."; \
		curl -sSL "https://github.com/bufbuild/buf/releases/latest/download/buf-$$(uname -s)-$$(uname -m)" -o ./buf; \
		chmod +x ./buf; \
	fi

# =============================================================================
# SERVICE DISPATCH
# =============================================================================
.PHONY: build-g8eo
build-g8eo:
	@$(MAKE) -C services/g8eo build

.PHONY: test-g8eo
test-g8eo:
	@$(MAKE) -C services/g8eo test

.PHONY: test-g8ee
test-g8ee:
	@./g8e test g8ee
