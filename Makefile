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
TMPDIR ?= /tmp
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
UPX_VERSION := v4.2.4

BUF := $(shell command -v buf 2>/dev/null || echo "./buf")
PROTOC := $(shell command -v protoc 2>/dev/null || echo "/usr/local/bin/protoc")
PROTOC_GEN_GO := $(shell go list -m -f '{{.Version}}' google.golang.org/protobuf 2>/dev/null || echo "$(PROTOC_GEN_GO_VERSION)")
UPX := $(shell command -v upx 2>/dev/null || echo "/usr/local/bin/upx")

# =============================================================================
# HELP
# =============================================================================
.PHONY: help
help:
	@echo "g8e Platform Root Makefile"
	@echo ""
	@echo "Note: On Windows, use build.ps1 instead of make"
	@echo ""
	@echo "CI/CD (Local):"
	@echo "  ci            Run full CI pipeline locally (mirrors GitHub Actions)"
	@echo "  ci-platform   Run platform-only CI (operator, protocol, proto, docs)"
	@echo ""
	@echo "Protocol Generation:"
	@echo "  generate      Generate all protocol artifacts (proto)"
	@echo "  proto         Generate all Protobuf code (Go)"
	@echo "  buf-install   Install Buf CLI locally if not found"
	@echo "  protoc-install Install protoc compiler"
	@echo "  upx-install   Install UPX compressor"
	@echo ""
	@echo "Build:"
	@echo "  build			Build g8e for current OS and architecture"
	@echo "  build-all			Build g8e for all platforms (linux, windows, darwin)"
	@echo "  build-linux		Build g8e for Linux (amd64, arm64, 386)"
	@echo "  build-windows		Build g8e for Windows (amd64, arm64)"
	@echo "  build-darwin		Build g8e for Darwin (amd64, arm64)"
	@echo "  build-compressed		Build g8e for all platforms with UPX compression"
	@echo "  build-linux-compressed	Build g8e for Linux with UPX compression"
	@echo "  build-windows-compressed	Build g8e for Windows with UPX compression"
	@echo "  build-darwin-compressed	Build g8e for Darwin with UPX compression"
	@echo ""
	@echo "Test:"
	@echo "  test                  Run all tests with race detection (unit + gateway)"
	@echo "  test-short            Run short tests with race detection"
	@echo "  test-coverage         Run tests with coverage (enforces 60% threshold). Use PKG=./path/to/pkg for specific package, VERBOSE=true for verbose output"
	@echo "  test-shuffle          Run all tests with randomized order"
	@echo "  test-integration      Run integration tests (requires platform running and auth login)"
	@echo "  test-scenario         Run scenario integration tests (requires platform running)"
	@echo "  test-gateway          Run gateway tests"
	@echo "  test-mcp              Run MCP tests (requires platform running and auth login)"
	@echo "  test-a2a              Run A2A tests (requires platform running and auth login)"
	@echo "  test-universal-gateway Run universal gateway integration tests (requires platform running and auth login)"
	@echo "  test-byo              Run BYO client tests (requires platform running and auth login)"
	@echo "  test-native           Run native real operator tests (requires platform running and auth login)"
	@echo ""
	@echo "Lint & Quality:"
	@echo "  lint          Run all linting and quality checks"
	@echo "  vulncheck     Run Operator vulnerability check"
	@echo "  validate-doctrines Validate doctrine JSON schema"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean         Remove all build artifacts and runtime state"

# =============================================================================
# PROTOCOL GENERATION
# =============================================================================
.PHONY: generate
generate: proto


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

.PHONY: proto-python
proto-python:
	@echo "Generating Python Protobuf code..."
	@if command -v python3 &> /dev/null; then \
		python3 -m grpc_tools.protoc \
			--python_out=protocol/python/g8e_protocol \
			--proto_path=protocol/proto \
			protocol/proto/g8e/common/v1/common.proto \
			protocol/proto/g8e/operator/v1/operator.proto \
			protocol/proto/g8e/pubsub/v1/pubsub.proto; \
	else \
		echo "Error: python3 not found. Install grpc_tools.protoc." >&2; \
		exit 1; \
	fi
	@echo "Python Protobuf generation complete."

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
		cd $(TMPDIR) && curl -fSL https://github.com/protocolbuffers/protobuf/releases/download/$(PROTOC_VERSION)/protoc-35.0-linux-x86_64.zip -o protoc.zip && \
		unzip -o protoc.zip -d protoc && \
		sudo cp protoc/bin/protoc /usr/local/bin/protoc && \
		sudo chmod +x /usr/local/bin/protoc && \
		rm -rf $(TMPDIR)/protoc $(TMPDIR)/protoc.zip; \
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

.PHONY: upx-install
upx-install:
	@if ! command -v upx &> /dev/null; then \
		echo "Installing UPX $(UPX_VERSION)..."; \
		cd $(TMPDIR) && curl -fSL https://github.com/upx/upx/releases/download/$(UPX_VERSION)/upx-$(UPX_VERSION:v%=%)-linux_amd64.tar.xz -o upx.tar.xz && \
		tar -xf upx.tar.xz && \
		sudo cp upx-$(UPX_VERSION:v%=%)-linux_amd64/upx /usr/local/bin/upx && \
		sudo chmod +x /usr/local/bin/upx && \
		rm -rf $(TMPDIR)/upx-$(UPX_VERSION:v%=%)-linux_amd64 $(TMPDIR)/upx.tar.xz; \
	fi
	@echo "UPX installed."

# =============================================================================
# BUILD
# =============================================================================
PLATFORMS := linux/amd64 linux/arm64 linux/386 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64

.PHONY: build
build:
	@echo "Building g8e operator for current platform..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	HOST_OS=$$(go env GOOS); \
	HOST_ARCH=$$(go env GOARCH); \
	BINARY=bin/g8e-$$HOST_OS-$$HOST_ARCH; \
	if [ "$$HOST_OS" = "windows" ]; then \
		BINARY=$$BINARY.exe; \
	fi; \
	echo "Building $$HOST_OS/$$HOST_ARCH -> $$BINARY..."; \
	GOOS=$$HOST_OS GOARCH=$$HOST_ARCH go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$HOST_OS_$$HOST_ARCH" -o $$BINARY ./cmd/operator || exit 1; \
	sha256sum $$BINARY > $$BINARY.sha256; \
	if [ "$$HOST_OS" = "windows" ]; then \
		ln -sf g8e-$$HOST_OS-$$HOST_ARCH.exe bin/g8e; \
		echo "Created symlink: bin/g8e -> bin/g8e-$$HOST_OS-$$HOST_ARCH.exe"; \
		cp bin/g8e-$$HOST_OS-$$HOST_ARCH.exe g8e.exe 2>/dev/null || true; \
	else \
		ln -sf g8e-$$HOST_OS-$$HOST_ARCH bin/g8e; \
		echo "Created symlink: bin/g8e -> bin/g8e-$$HOST_OS-$$HOST_ARCH"; \
		cp bin/g8e-$$HOST_OS-$$HOST_ARCH g8e 2>/dev/null || true; \
	fi
	@echo "Build complete. Binary: $$BINARY"

.PHONY: docker-build
docker-build:
	@echo "Building g8e operator Docker image..."
	@docker build -f Dockerfile -t g8e:$$(cat VERSION) -t g8e:latest .
	@echo "Operator image built: g8e:$$(cat VERSION)"

.PHONY: build-all
build-all:
	@echo "Building g8e operator for all platforms..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		BINARY=bin/g8e-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then \
			BINARY=$$BINARY.exe; \
		fi; \
		echo "Building $$platform -> $$BINARY..."; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$platform" -o $$BINARY ./cmd/operator || exit 1; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@HOST_OS=$$(go env GOOS); \
	HOST_ARCH=$$(go env GOARCH); \
	if [ "$$HOST_OS" = "windows" ]; then \
		if [ -f bin/g8e-$$HOST_OS-$$HOST_ARCH.exe ]; then \
			ln -sf g8e-$$HOST_OS-$$HOST_ARCH.exe bin/g8e; \
			echo "Created symlink: bin/g8e -> bin/g8e-$$HOST_OS-$$HOST_ARCH.exe"; \
			cp bin/g8e-$$HOST_OS-$$HOST_ARCH.exe g8e.exe 2>/dev/null || true; \
		fi; \
	else \
		if [ -f bin/g8e-$$HOST_OS-$$HOST_ARCH ]; then \
			ln -sf g8e-$$HOST_OS-$$HOST_ARCH bin/g8e; \
			echo "Created symlink: bin/g8e -> bin/g8e-$$HOST_OS-$$HOST_ARCH"; \
			cp bin/g8e-$$HOST_OS-$$HOST_ARCH g8e 2>/dev/null || true; \
		fi; \
	fi
	@echo "Multi-platform build complete. Checksums: bin/g8e-*.sha256"

.PHONY: build-compressed
build-compressed: upx-install
	@echo "Building g8e operator with compression for $(PLATFORMS)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		BINARY=bin/g8e-$$GOOS-$$GOARCH; \
		if [ "$$GOOS" = "windows" ]; then \
			BINARY=$$BINARY.exe; \
		fi; \
		echo "Building $$platform -> $$BINARY..."; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=$$platform" -o $$BINARY ./cmd/operator || exit 1; \
		echo "Compressing $$BINARY with UPX..."; \
		$(UPX) --best --lzma $$BINARY; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@HOST_OS=$$(go env GOOS); \
	HOST_ARCH=$$(go env GOARCH); \
	if [ "$$HOST_OS" = "windows" ]; then \
		if [ -f bin/g8e-$$HOST_OS-$$HOST_ARCH.exe ]; then \
			ln -sf g8e-$$HOST_OS-$$HOST_ARCH.exe bin/g8e; \
			echo "Created symlink: bin/g8e -> bin/g8e-$$HOST_OS-$$HOST_ARCH.exe"; \
			cp bin/g8e-$$HOST_OS-$$HOST_ARCH.exe g8e.exe 2>/dev/null || true; \
		fi; \
	else \
		if [ -f bin/g8e-$$HOST_OS-$$HOST_ARCH ]; then \
			ln -sf g8e-$$HOST_OS-$$HOST_ARCH bin/g8e; \
			echo "Created symlink: bin/g8e -> bin/g8e-$$HOST_OS-$$HOST_ARCH"; \
			cp bin/g8e-$$HOST_OS-$$HOST_ARCH g8e 2>/dev/null || true; \
		fi; \
	fi
	@echo "Compressed multi-platform build complete. Checksums: bin/g8e-*.sha256"

.PHONY: build-linux-compressed
build-linux-compressed: upx-install
	@echo "Building g8e for Linux with UPX compression..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for arch in amd64 arm64 386; do \
		BINARY=bin/g8e-linux-$$arch; \
		echo "Building linux/$$arch -> $$BINARY..."; \
		GOOS=linux GOARCH=$$arch go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=linux_$$arch" -o $$BINARY ./cmd/operator || exit 1; \
		echo "Compressing $$BINARY with UPX..."; \
		$(UPX) --best --lzma $$BINARY; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@echo "Linux compressed build complete. Binaries: bin/g8e-linux-*"

.PHONY: build-darwin-compressed
build-darwin-compressed: upx-install
	@echo "Building g8e for Darwin with UPX compression..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for arch in amd64 arm64; do \
		BINARY=bin/g8e-darwin-$$arch; \
		echo "Building darwin/$$arch -> $$BINARY..."; \
		GOOS=darwin GOARCH=$$arch go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=darwin_$$arch" -o $$BINARY ./cmd/operator || exit 1; \
		echo "Compressing $$BINARY with UPX..."; \
		$(UPX) --best --lzma $$BINARY; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@echo "Darwin compressed build complete. Binaries: bin/g8e-darwin-*"

.PHONY: build-windows-compressed
build-windows-compressed: upx-install
	@echo "Building g8e for Windows with UPX compression..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for arch in amd64 arm64; do \
		BINARY=bin/g8e-windows-$$arch.exe; \
		echo "Building windows/$$arch -> $$BINARY..."; \
		GOOS=windows GOARCH=$$arch go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=windows_$$arch -s -w" -o $$BINARY ./cmd/operator || exit 1; \
		echo "Compressing $$BINARY with UPX..."; \
		$(UPX) --best --lzma $$BINARY; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@echo "Windows compressed build complete. Binaries: bin/g8e-windows-*.exe"

.PHONY: build-darwin
build-darwin:
	@echo "Building g8e for Darwin..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for arch in amd64 arm64; do \
		BINARY=bin/g8e-darwin-$$arch; \
		echo "Building darwin/$$arch -> $$BINARY..."; \
		GOOS=darwin GOARCH=$$arch go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=darwin_$$arch" -o $$BINARY ./cmd/operator || exit 1; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@echo "Darwin build complete. Binaries: bin/g8e-darwin-*"

.PHONY: build-linux
build-linux:
	@echo "Building g8e for Linux..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for arch in amd64 arm64 386; do \
		BINARY=bin/g8e-linux-$$arch; \
		echo "Building linux/$$arch -> $$BINARY..."; \
		GOOS=linux GOARCH=$$arch go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=linux_$$arch" -o $$BINARY ./cmd/operator || exit 1; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@echo "Linux build complete. Binaries: bin/g8e-linux-*"

.PHONY: build-windows
build-windows:
	@echo "Building g8e for Windows (no compression to avoid Defender false positives)..."
	@mkdir -p bin
	@VERSION=$$(cat VERSION | tr -d '\n'); \
	BUILD_ID=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	for arch in amd64 arm64; do \
		BINARY=bin/g8e-windows-$$arch.exe; \
		echo "Building windows/$$arch -> $$BINARY..."; \
		GOOS=windows GOARCH=$$arch go build -ldflags "-X main.version=$$VERSION -X main.buildID=$$BUILD_ID -X main.buildTime=$$BUILD_TIME -X main.platform=windows_$$arch -s -w" -o $$BINARY ./cmd/operator || exit 1; \
		sha256sum $$BINARY > $$BINARY.sha256; \
	done
	@echo "Windows build complete. Binaries: bin/g8e-windows-*.exe"

# =============================================================================
# TEST
# =============================================================================
.PHONY: test
test: test-unit test-gateway
	@echo "All tests completed successfully."

.PHONY: test-unit
test-unit:
	@echo "Running unit tests..."
	@go test -race -count=1 -timeout 180s $$(go list ./... | grep -v mocks | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | grep -v "^github.com/g8e-ai/g8e/internal/protocol/proto/")

.PHONY: test-short
test-short:
	@go test -race -short -count=1 -timeout 60s $$(go list ./... | grep -v mocks | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | grep -v "^github.com/g8e-ai/g8e/internal/protocol/proto/")

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
	@go test -race -count=1 -shuffle=on -timeout 180s $$(go list ./... | grep -v mocks | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | grep -v "^github.com/g8e-ai/g8e/internal/protocol/proto/")

.PHONY: test-integration
test-integration:
	@echo "Running integration tests (requires platform running and auth login)..."
	@go test -tags=integration -race -count=1 -timeout 180s ./test/...

.PHONY: test-scenario
test-scenario:
	@echo "Running scenario integration tests (requires platform running)..."
	@go test -tags=integration -race -count=1 -timeout 180s ./test/scenario/...

.PHONY: test-gateway
test-gateway:
	@echo "Running gateway tests..."
	@go test -race -count=1 -timeout 180s ./test/a2a_gateway_test.go ./test/mcp_gateway_test.go ./test/mcp_stdio_test.go

.PHONY: test-mcp
test-mcp:
	@echo "Running MCP tests (requires platform running and auth login)..."
	@go test -tags=integration -race -count=1 -timeout 180s ./test/integration_helper.go ./test/mcp_gateway_test.go ./test/mcp_real_operator_test.go ./test/mcp_stdio_test.go

.PHONY: test-a2a
test-a2a:
	@echo "Running A2A tests (requires platform running and auth login)..."
	@go test -tags=integration -race -count=1 -timeout 180s ./test/integration_helper.go ./test/a2a_gateway_test.go ./test/a2a_real_operator_test.go

.PHONY: test-universal-gateway
test-universal-gateway:
	@echo "Running universal gateway integration tests (requires platform running and auth login)..."
	@go test -tags=integration -race -count=1 -timeout 180s ./test/universal_gateway_integration_test.go

.PHONY: test-byo
test-byo:
	@echo "Running BYO client tests (requires platform running and auth login)..."
	@go test -tags=integration -race -count=1 -timeout 180s ./test/byo_client_test.go

.PHONY: test-native
test-native:
	@echo "Running native real operator tests (requires platform running and auth login)..."
	@go test -tags=integration -race -count=1 -timeout 180s ./test/integration_helper.go ./test/native_real_operator_test.go


# =============================================================================
# LINT & QUALITY
# =============================================================================
.PHONY: lint
lint: lint-no-embedded-newlines vulncheck validate-doctrines
	@golangci-lint run
	@echo "All linting and quality checks complete."

.PHONY: lint-no-embedded-newlines
lint-no-embedded-newlines:
	@echo "Checking for compilation errors (including embedded newlines)..."
	@go build ./... || { echo "Error: Go build failed. This may be caused by embedded newlines or other syntax errors."; exit 1; }
	@echo "Build successful - no embedded newlines or syntax errors detected."

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
	@if [ -d "$(TMPDIR)/coreruleset" ]; then \
		cd $(TMPDIR)/coreruleset && git pull; \
	else \
		git clone --depth 1 https://github.com/coreruleset/coreruleset.git $(TMPDIR)/coreruleset; \
	fi
	@curl -sSL https://raw.githubusercontent.com/gitleaks/gitleaks/master/config/gitleaks.toml -o $(TMPDIR)/gitleaks.toml
	@$(MAKE) ingest-doctrines
	@echo "Doctrine update complete."

# =============================================================================
# CLEANUP
# =============================================================================
.PHONY: clean
clean:
	@echo "Cleaning up build artifacts and runtime state..."
	@rm -rf .g8e/
	@rm -f g8e
	@rm -f g8e-*
	@rm -f *.sha256
	@rm -f *.test
	@rm -f coverage.out
	@rm -f buf
	@rm -rf .g8e-harness-*/
	@echo "Clean complete."

.PHONY: clean-harness
clean-harness:
	@echo "Cleaning up stale harness directories..."
	@rm -rf .g8e-harness-*/
	@echo "Clean complete."


# =============================================================================
# CI/CD (LOCAL)
# =============================================================================
.PHONY: ci
ci: ci-platform
	@echo "CI complete."

.PHONY: ci-platform
ci-platform: _ci-verify-proto _ci-lint _ci-vulncheck _ci-test
	@echo "Platform CI complete."

.PHONY: _ci-verify-proto
_ci-verify-proto:
	@echo "=== verify-proto ==="
	@$(MAKE) protoc-install
	@$(MAKE) proto
	@CHANGES=$$(git status --porcelain | grep -E "^\s*M.*\.pb\.go$$|^\s*M.*\.proto$$" || true); \
	if [ -n "$$CHANGES" ]; then \
		echo "Error: Generated proto files are out of sync with protocol/proto/*.proto"; \
		echo "$$CHANGES"; \
		git diff -- $$(git status --porcelain | grep -E "^\s*M" | awk '{print $$2}'); \
		exit 1; \
	fi
	@$(MAKE) validate-doctrines

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
	@./g8e gw start --cert-mode localhost
	@G8E_STRICT_CONSTANTS_LINT=1 go test -race -timeout 180s -coverprofile=coverage.out -covermode=atomic $$(go list ./... | grep -v mocks | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | grep -v "^github.com/g8e-ai/g8e/internal/protocol/proto/")
	@COVERAGE=$$(go tool cover -func=coverage.out | grep -v "internal/protocol/proto" | grep -v "mocks" | grep -v "^github.com/g8e-ai/g8e/cmd/" | grep -v "^github.com/g8e-ai/g8e/internal/testutil/" | grep -v "^github.com/g8e-ai/g8e/test/" | tail -1 | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < 60" | bc -l) -eq 1 ]; then \
		echo "Coverage $$COVERAGE% is below 60% threshold"; \
		exit 1; \
	fi; \
	echo "Coverage $$COVERAGE% meets 60% threshold"
	@./g8e gw stop
