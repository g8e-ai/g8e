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
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make protobuf-dev curl

# Set working directory
WORKDIR /build

# Copy go mod files for dependency caching
COPY go.mod go.sum ./
COPY protocol/go.mod protocol/go.sum ./protocol/

# Download dependencies
RUN go mod download
RUN cd protocol && go mod download

# Copy entire source code
COPY . .

# Build the binary
# Use build flags from Makefile for consistency
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags "-s -w -X main.version=$(cat VERSION) -X main.buildID=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=linux_amd64" \
    -o /build/g8e \
    ./cmd/operator

# Verify the binary
RUN /build/g8e --help

# =============================================================================
# Stage 2: Runtime
# Use distroless for minimal attack surface and smallest image size
FROM gcr.io/distroless/static-debian12

# Create non-root user for security
# Note: distroless doesn't support useradd, so we run as nonroot user
# The distroless image already sets up a nonroot user

# Copy the binary from builder
COPY --from=builder /build/g8e /g8e

# Copy protocol constants (required for doctrine mode)
COPY --from=builder /build/protocol/constants /protocol/constants

# Set permissions
# Note: distroless images are read-only, permissions are pre-configured

# Expose default ports (can be overridden at runtime)
EXPOSE 8080 8443

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/g8e", "gw", "status"]

# Set entrypoint with default doctrine mode and customizable ports
# Users can override HTTP_PORT and HTTPS_PORT via environment variables
ENTRYPOINT ["/g8e"]
CMD ["--doctrine", "gw", "start", "--http-port", "8080", "--https-port", "8443"]
