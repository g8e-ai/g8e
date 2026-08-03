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

# =============================================================================
# Multi-stage Dockerfile for g8e Gateway
# Modern, minimal, and secure container image
# =============================================================================

# Stage 1: Build
FROM golang:1.26.5 AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git make protobuf-compiler curl \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /build

# Copy go mod files and vendored dependencies for air-gapped builds
COPY go.mod go.sum vendor/ ./

# Use vendored modules — no network access required
ENV GOFLAGS=-mod=vendor

# Copy entire source code
COPY . .

# Build the binary
# Use build flags from Makefile for consistency
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor \
    -ldflags "-s -w -X main.version=$(cat VERSION) -X main.buildID=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=linux_amd64" \
    -o /build/g8e \
    ./cmd/g8e

# Verify the binary
RUN /build/g8e --help

# =============================================================================
# Stage 2: Runtime
# The compiled binary is statically linked (CGO_ENABLED=0) and has zero runtime
# dependencies. The packages below are container conveniences only (health-check
# curl, CA certs for outbound TLS) — they are NOT required by the binary itself.
FROM debian@sha256:30482e873082e906a4908c10529180aefb6f77620aea7404b909829fadc5d168

# Install container utilities (not binary dependencies)
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl wget ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary from builder
COPY --from=builder /build/g8e /g8e

# Copy protocol constants (required for doctrine mode)
COPY --from=builder /build/protocol/constants /protocol/constants

# Copy docs reference data (KSI catalog, COSAiS overlays) for compliance CLI
COPY --from=builder /build/docs/reference /docs/reference

# Expose default ports (can be overridden at runtime)
EXPOSE 8080 8443

# Health check - use HTTP endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/api/v1/health || exit 1

# Set entrypoint
# The same binary can run in gateway mode (doctrine) or operator mode (standard)
# Mode is selected via command-line flags in docker-compose.yml
ENTRYPOINT ["/g8e"]
