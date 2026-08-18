# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# =============================================================================
# Multi-stage Dockerfile for g8e Gateway
# Modern, minimal, and secure container image.
#
# This Dockerfile always builds with FIPS 140-3 approved mode enabled. The Go
# Cryptographic Module v1.0.0 (CMVP Cert #5247, CAVP A6650) is linked in at
# build time via GOFIPS140, and the binary enters approved mode on startup
# without any runtime env var. Enforcement is a RUNTIME setting, off by default:
# non-approved primitives (Ed25519 for consensus/receipts/PKI, ChaCha20-Poly1305
# for SSH streaming) still work. Operators who need strict enforcement set
# GODEBUG=fips140=only in the container environment (see `make verify-fips`).
#
# The FIPS 140-3 compliance claim is restricted to linux/amd64, which is the
# tested operating environment for CMVP Cert #5247. This Dockerfile hardcodes
# GOOS=linux GOARCH=amd64, so the restriction is satisfied. Do not build it for
# other architectures under a FIPS compliance claim.
#
# Verify the deployed binary with:  g8e version --fips
# =============================================================================

# Stage 1: Build (FIPS module linked in here)
FROM golang:1.26.6 AS builder

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

# Build the binary with the Go Cryptographic Module v1.0.0 linked in and FIPS
# 140-3 approved mode enabled by default. GOFIPS140 is set ONLY in the builder
# stage — it is a build-time toolchain setting, not a runtime control. The
# binary enters approved mode on startup without any runtime env var.
# CGO_ENABLED=0 is intentional: the Go FIPS module is pure Go and does not
# require CGO.
ENV GOFIPS140=v1.0.0

# Build the binary
# Use build flags from Makefile for consistency
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=vendor \
    -ldflags "-s -w -X main.version=$(cat VERSION) -X main.buildID=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.buildTime=$(date -u '+%Y-%m-%dT%H:%M:%SZ') -X main.platform=linux_amd64" \
    -o /build/g8e \
    ./cmd/g8e

# Verify the binary built.
RUN /build/g8e --help

# Verify FIPS 140-3 approved mode is active via the native crypto/fips140 module
# API. This is the same self-check operators run in production
# (`g8e version --fips`); it inspects module state, not env vars. It exits 0
# with a warning when enforcement is off — that is the expected posture, not a
# failure.
RUN /build/g8e version --fips

# =============================================================================
# Stage 2: Runtime
# The compiled binary is statically linked (CGO_ENABLED=0) and has zero runtime
# dependencies. The packages below are container conveniences only (health-check
# curl, CA certs for outbound TLS) — they are NOT required by the binary itself.
# GOFIPS140 is deliberately NOT set here: the binary already entered approved
# mode at build time and needs no runtime env var.
#
# OE MATRIX (CMVP Cert #5247, Go Cryptographic Module v1.0.0):
# - Tested OE: Alpine Linux 3.20 on Intel x86-64 (Dell PowerEdge R660, Xeon
#   Silver 4410Y), Podman 4 on RHEL (Table 2).
# - Vendor-Affirmed OEs (Table 3): Debian GNU/Linux 12, Ubuntu 22.04, Ubuntu
#   24.04, and others.
# - Vendor-Affirmed platforms (Table 4): Linux 3.10+ on x86-64 and ARMv7/8/9.
#
# This image is pinned to Debian GNU/Linux 12 (Bookworm) via digest, a
# vendor-affirmed OE. The slim variant reduces attack surface. Do not change to
# a different OS or version without confirming it is a tested or
# vendor-affirmed OE for the applicable CMVP certificate.
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

# No image-level HEALTHCHECK: the same image runs as both gateway (HTTP server
# on 8080) and operator (outbound WebSocket client that listens on nothing).
# A healthcheck baked into the image would always fail for the operator.
# Healthchecks are declared per-service in docker-compose.yml, where each
# service can express its own liveness signal.

# Set entrypoint
# The same binary can run in gateway mode (doctrine) or operator mode (standard)
# Mode is selected via command-line flags in docker-compose.yml
ENTRYPOINT ["/g8e"]
