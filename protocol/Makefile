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

.PHONY: test
test:
	@$(GO) test ./...

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
	elif [ -f "../services/g8eo/bin/golangci-lint" ]; then \
		../services/g8eo/bin/golangci-lint run; \
	else \
		echo "golangci-lint not found"; \
		exit 1; \
	fi
