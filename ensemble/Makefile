# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8ee Ensemble Makefile

SHELL := /bin/bash
.DEFAULT_GOAL := help

# =============================================================================
# PATHS
# =============================================================================
APP_DIR := app
TEST_DIR := tests

# =============================================================================
# TOOLS
# =============================================================================
PYTHON := $(shell if [ -f .venv/bin/python ]; then echo $(CURDIR)/.venv/bin/python; else echo python3; fi)
RUFF := $(shell if [ -f .venv/bin/ruff ]; then echo $(CURDIR)/.venv/bin/ruff; else command -v ruff 2>/dev/null || echo ruff; fi)
PYRIGHT := $(shell if [ -f .venv/bin/pyright ]; then echo $(CURDIR)/.venv/bin/pyright; else command -v pyright 2>/dev/null || echo pyright; fi)

# =============================================================================
# HELP
# =============================================================================
.PHONY: help
help:
	@echo "g8ee Ensemble Makefile"
	@echo ""
	@echo "Quality:"
	@echo "  test          Run tests"
	@echo "  lint          Run ruff and pyright"
	@echo "  format        Run ruff format"
	@echo "  check         Run format, lint, test"
	@echo ""
	@echo "Environment:"
	@echo "  install-tools Install ruff and pyright"
	@echo "  clean         Remove __pycache__ and .pyc files"
	@echo "  proto         Generate Python protobuf stubs from g8e protocol"
	@echo "  vendor        Initialize/update git submodules"

# =============================================================================
# QUALITY
# =============================================================================
.PHONY: test
test:
	@$(PYTHON) -m pytest $(TEST_DIR)

.PHONY: lint
lint:
	@echo "Running ruff..."
	@$(RUFF) check $(APP_DIR)
	@echo "Running pyright..."
	@$(PYRIGHT) $(APP_DIR)

.PHONY: format
format:
	@echo "Running ruff format..."
	@$(RUFF) format $(APP_DIR) $(TEST_DIR)

.PHONY: check
check: format lint test

# =============================================================================
# TOOL INSTALL
# =============================================================================
.PHONY: install-tools
install-tools:
	@echo "Installing python quality tools..."
	@pip install ruff pyright

# =============================================================================
# CLEAN
# =============================================================================
.PHONY: clean
clean:
	@echo "Cleaning __pycache__ and .pyc files..."
	@find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
	@find . -type f -name "*.pyc" -delete 2>/dev/null || true
	@find . -type f -name "*.pyo" -delete 2>/dev/null || true
	@find . -type d -name "*.egg-info" -exec rm -rf {} + 2>/dev/null || true
	@echo "Done."

# =============================================================================
# SETUP
# =============================================================================
.PHONY: setup
setup:
	@echo "Installing g8e protocol package (editable)..."
	@pip install -e ../protocol/python
	@echo "Installing ensemble with dev/test extras..."
	@pip install -e ".[dev,test]"

# =============================================================================
# PROTO
# =============================================================================
.PHONY: proto
proto:
	@echo "Verifying canonical Python protobuf stubs..."
	@cd ../protocol && $(PYTHON) python/scripts/generate_protos.py --check

# =============================================================================
# VENDOR
# =============================================================================
.PHONY: vendor
vendor:
	@echo "Initializing/updating git submodules..."
	@git submodule update --init --recursive
