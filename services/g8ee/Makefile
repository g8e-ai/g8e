# g8ee Engine Makefile

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
PYTHON := $(shell if [ -f .venv/bin/python ]; then echo .venv/bin/python; else echo python3; fi)
RUFF := $(shell if [ -f .venv/bin/ruff ]; then echo .venv/bin/ruff; else command -v ruff 2>/dev/null || echo ruff; fi)
PYRIGHT := $(shell if [ -f .venv/bin/pyright ]; then echo .venv/bin/pyright; else command -v pyright 2>/dev/null || echo pyright; fi)

# =============================================================================
# HELP
# =============================================================================
.PHONY: help
help:
	@echo "g8ee Engine Makefile"
	@echo ""
	@echo "Quality:"
	@echo "  test          Run tests"
	@echo "  lint          Run ruff and pyright"
	@echo "  format        Run ruff format"
	@echo "  check         Run format, lint, test"
	@echo ""
	@echo "Environment:"
	@echo "  install-tools Install ruff and pyright"

# =============================================================================
# QUALITY
# =============================================================================
.PHONY: test
test:
	@../../g8e test g8ee

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
