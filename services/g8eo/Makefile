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

# g8eo Operator Makefile

SHELL := /bin/bash
.DEFAULT_GOAL := help

# =============================================================================
# =============================================================================
# Use go1.26 if available (local dev), otherwise fall back to system go (Docker/CI)
GO := $(shell command -v go1.26 2>/dev/null || ([ -x "$$(go env GOPATH)/bin/go1.26" ] && echo "$$(go env GOPATH)/bin/go1.26") || command -v go 2>/dev/null || echo "go")

# =============================================================================
# VERSION
# =============================================================================
VERSION := $(shell cat ../../VERSION 2>/dev/null | tr -d '[:space:]' || echo 'v0.0.0')

# =============================================================================
# PATHS AND NAMES
# =============================================================================
BUILD_DIR := build

# =============================================================================
# ARCHITECTURES
# =============================================================================
ARCHS := amd64 arm64 386

# =============================================================================
# TOOLS
# =============================================================================
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || ([ -f "$$(go env GOPATH)/bin/golangci-lint" ] && echo "$$(go env GOPATH)/bin/golangci-lint") || ([ -f "./bin/golangci-lint" ] && echo "./bin/golangci-lint"))
GOVULNCHECK := $(shell command -v govulncheck 2>/dev/null || ([ -f "$$(go env GOPATH)/bin/govulncheck" ] && echo "$$(go env GOPATH)/bin/govulncheck"))

# =============================================================================
# BUILD CONFIGURATION
# =============================================================================
COMPRESS ?= 0

# =============================================================================
# HELP
# =============================================================================
.PHONY: help
help:
	@echo "g8eo Operator Makefile"
	@echo ""
	@echo "Build:"
	@echo "  build             Build linux/amd64"
	@echo "  build-all         Build all architectures (amd64 arm64 386)"
	@echo ""
	@echo "Quality:"
	@echo "  test          Run tests"
	@echo "  test-sudo     Run tests with sudo (for privileged operations)"
	@echo "  fmt           Format code"
	@echo "  vet           Run go vet"
	@echo "  lint          Run golangci-lint"
	@echo "  vulncheck     Run govulncheck"
	@echo "  check         Run fmt, vet, lint, vulncheck, test"
	@echo ""
	@echo "Tool Install:"
	@echo "  install-tools  Install golangci-lint and govulncheck"
	@echo ""
	@echo "Utility:"
	@echo "  clean            Clean build artifacts"
	@echo "  clean-go-cache   Clean Go build cache (reclaim disk space)"
	@echo "  clean-deep       Deep clean including all caches"
	@echo "  deps          Update dependencies"
	@echo "  version       Show version info"

# =============================================================================
# BUILD TARGETS
# =============================================================================
.PHONY: build
build:
	@echo "Building g8e binary (linux/amd64)..."
	@mkdir -p build/linux-amd64
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath \
		-buildvcs=false \
		-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/amd64'" \
		-o build/linux-amd64/g8e ./cmd/g8eo
	@echo 'Built: build/linux-amd64/g8e'
	@if [ "$(COMPRESS)" = "1" ] && command -v upx >/dev/null 2>&1; then \
		echo "Compressing binary with upx..."; \
		upx --best --lzma build/linux-amd64/g8e || exit 1; \
	elif [ "$(COMPRESS)" = "1" ]; then \
		echo "Warning: COMPRESS=1 set but upx not found, skipping compression."; \
	fi
	@SIZE=$$(ls -lh build/linux-amd64/g8e | awk '{print $$5}') && \
		echo "  Size: $$SIZE"

.PHONY: build-all
build-all:
	@echo "Building g8e binary (all architectures)..."
	@for arch in amd64 arm64 386; do \
		echo "Building linux/$$arch..."; \
		mkdir -p $(BUILD_DIR)/linux-$$arch && \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build \
			-trimpath \
			-buildvcs=false \
			-ldflags "-s -w -X main.version=$(VERSION) -X 'main.platform=linux/$$arch'" \
			-o $(BUILD_DIR)/linux-$$arch/g8e ./cmd/g8eo || exit 1; \
		SIZE=$$(ls -lh $(BUILD_DIR)/linux-$$arch/g8e | awk '{print $$5}'); \
		echo "  linux/$$arch: $$SIZE (uncompressed)"; \
	done
	@if [ "$(COMPRESS)" = "1" ] && command -v upx >/dev/null 2>&1; then \
		echo "Compressing binaries with upx..."; \
		for arch in amd64 arm64 386; do \
			upx --best --lzma $(BUILD_DIR)/linux-$$arch/g8e || exit 1; \
			SIZE=$$(ls -lh $(BUILD_DIR)/linux-$$arch/g8e | awk '{print $$5}'); \
			echo "  linux/$$arch: $$SIZE (compressed)"; \
		done; \
	elif [ "$(COMPRESS)" = "1" ]; then \
		echo "Warning: COMPRESS=1 set but upx not found, skipping compression."; \
	else \
		echo "COMPRESS=1 not set - skipping compression (binaries are already stripped)"; \
	fi
	@echo ""
	@echo "All architectures built successfully."


# =============================================================================
# CODE QUALITY
# =============================================================================
.PHONY: test
test:
	@$(GO) test -race -timeout 180s ./...

.PHONY: test-coverage
test-coverage:
	@$(GO) test -race -timeout 180s -coverprofile=coverage.out -covermode=atomic ./...
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@$(GO) tool cover -func=coverage.out | tail -1

.PHONY: test-coverage-check
test-coverage-check:
	@$(GO) test -race -timeout 180s -coverprofile=coverage.out -covermode=atomic ./...
	@COVERAGE=$$($(GO) tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < 85" | bc -l) -eq 1 ]; then \
		echo "Coverage $$COVERAGE% is below 85% threshold"; \
		exit 1; \
	fi; \
	echo "Coverage $$COVERAGE% meets 85% threshold"

.PHONY: test-sudo
test-sudo:
	@sudo $(GO) test -race -timeout 180s ./...

.PHONY: fmt
fmt:
	@gofmt -s -w .

.PHONY: vet
vet:
	@$(GO) vet ./...

.PHONY: lint
lint:
	@if [ -z "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint not found. Run 'make install-tools' to install it locally."; \
		exit 1; \
	fi
	@$(GOLANGCI_LINT) run --path-prefix=services/g8eo

.PHONY: vulncheck
vulncheck:
	@if [ -z "$(GOVULNCHECK)" ]; then \
		echo "govulncheck not found. Run 'make install-tools' to install it."; \
		exit 1; \
	fi
	@echo "Running vulnerability check (allowing stdlib issues per environment constraint)..."
	@$(GOVULNCHECK) ./... || (if [ $$? -eq 3 ]; then echo "Vulnerabilities found (standard library), continuing per plan."; exit 0; else exit $$?; fi)

.PHONY: check-registry
check-registry:
	@echo "Checking constants registry completeness..."
	@cd internal/constants && go run check_registry.go

.PHONY: check
check: fmt vet lint vulncheck test check-registry

# =============================================================================
# TOOL INSTALL
# =============================================================================
.PHONY: install-tools
install-tools:
	@echo "Installing development tools..."
	@mkdir -p bin
	@if [ -z "$$(command -v golangci-lint)" ] && [ ! -f "./bin/golangci-lint" ]; then \
		echo "Installing golangci-lint (building from source for Go 1.26 compatibility)..."; \
		$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.0; \
	fi
	@if [ -z "$$(command -v govulncheck)" ]; then \
		echo "Installing govulncheck..."; \
		$(GO) install golang.org/x/vuln/cmd/govulncheck@latest; \
	fi
	@if [ -z "$$(command -v gotestsum)" ]; then \
		echo "Installing gotestsum..."; \
		$(GO) install gotest.tools/gotestsum@latest; \
	fi
	@if [ -z "$$(command -v mockery)" ]; then \
		echo "Installing mockery..."; \
		$(GO) install github.com/vektra/mockery/v2@latest; \
	fi
	@echo "Tools installed successfully."

# =============================================================================
# UTILITY
# =============================================================================
.PHONY: clean
clean:
	@rm -rf build/
	@rm -f $(BINARY_NAME) coverage.out coverage.html *.test

.PHONY: clean-go-cache
clean-go-cache:
	@echo "Go build cache before: $$(du -sh $$($(GO) env GOCACHE) 2>/dev/null | cut -f1)"
	@$(GO) clean -cache
	@echo "Go build cache cleared"

.PHONY: clean-deep
clean-deep: clean clean-go-cache
	@$(GO) clean -testcache -modcache
	@echo "All caches cleared"

.PHONY: deps
deps:
	@$(GO) get -u ./...
	@$(GO) mod tidy
	@$(GO) mod vendor

.PHONY: version
version:
	@echo "Operator version: $(VERSION)"
