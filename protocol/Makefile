# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e Protocol Makefile

SHELL := /bin/bash
.DEFAULT_GOAL := help

GO := go

.PHONY: help
help:
	@echo "g8e Protocol Makefile"
	@echo ""
	@echo "Quality:"
	@echo "  test          Run tests"
	@echo "  fmt           Format code"
	@echo "  vet           Run go vet"
	@echo "  lint          Run golangci-lint"
	@echo ""
	@echo "Documentation:"
	@echo "  openapi       Generate OpenAPI spec from protobuf"

.PHONY: test
test:
	@$(GO) test -race -count=1 ./...

.PHONY: fmt
fmt:
	@gofmt -s -w .

.PHONY: vet
vet:
	@$(GO) vet ./...

.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found"; \
		exit 1; \
	fi

.PHONY: openapi
openapi:
	@echo "Generating OpenAPI spec from protobuf..."
	@echo "Note: Requires buf or google protobuf dependencies. Use 'buf generate' instead for full protobuf tooling."
	@mkdir -p docs/reference/api
	@echo "OpenAPI generation from protobuf requires additional setup. See protocol/docs/ for instructions."
