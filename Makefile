# =============================================================================
# PROTOCOL GENERATION
# =============================================================================

GOBIN := $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

.PHONY: generate
generate: proto

.PHONY: proto
proto: buf-install protoc-install
	@echo "Generating Go Protobuf code with Buf..."
	@buf generate protocol/proto
	@echo "Protobuf generation complete."

.PHONY: buf-install
buf-install:
	@if ! command -v buf &> /dev/null; then \
		echo "Installing buf CLI..."; \
		go install github.com/bufbuild/buf/cmd/buf@latest; \
	else \
		echo "buf already installed: $$(buf --version)"; \
	fi

.PHONY: protoc-install
protoc-install:
	@if ! command -v protoc-gen-go &> /dev/null; then \
		echo "Installing protoc-gen-go..."; \
		go install google.golang.org/protobuf/cmd/protoc-gen-go@latest; \
	fi
	@if ! command -v protoc-gen-go-grpc &> /dev/null; then \
		echo "Installing protoc-gen-go-grpc..."; \
		go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest; \
	fi
	@if ! command -v protoc-gen-doc &> /dev/null; then \
		echo "Installing protoc-gen-doc..."; \
		go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc@latest; \
	fi
