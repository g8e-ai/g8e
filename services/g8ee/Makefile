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
PYTHON := $(shell if [ -f .venv/bin/python ]; then echo .venv/bin/python; else echo python3; fi)
RUFF := $(shell if [ -f .venv/bin/ruff ]; then echo .venv/bin/ruff; else command -v ruff 2>/dev/null || echo ruff; fi)
PYRIGHT := $(shell if [ -f .venv/bin/pyright ]; then echo .venv/bin/pyright; else command -v pyright 2>/dev/null || echo pyright; fi)

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
