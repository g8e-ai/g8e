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

# g8e Protocol Makefile

SHELL := /bin/bash
.DEFAULT_GOAL := help

GO := $(shell command -v go1.24 2>/dev/null || ([ -x "$$(go env GOPATH)/bin/go1.24" ] && echo "$$(go env GOPATH)/bin/go1.24") || command -v go 2>/dev/null || echo "go")

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
