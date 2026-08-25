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
# This Dockerfile runs `make build-all` to produce all platform binaries
# (linux/amd64, linux/arm64, linux/386, windows/amd64, windows/arm64,
# darwin/amd64, darwin/arm64). Linux binaries are built with FIPS 140-3
# approved mode enabled via GOFIPS140 (Go Cryptographic Module v1.0.0,
# CMVP Cert #5247, CAVP A6650); non-linux binaries are built without it.
# The gateway serves these binaries via /.well-known/g8e/bin/{filename} so
# operators can deploy g8e on any supported platform by fetching the
# appropriate binary from the gateway.
#
# FIPS 140-3 enforcement is a RUNTIME setting, off by default: non-approved
# primitives (Ed25519 for consensus/receipts/PKI, ChaCha20-Poly1305 for SSH
# streaming) still work. Operators who need strict enforcement set
# GODEBUG=fips140=only in the container environment (see `make verify-fips`).
#
# The FIPS 140-3 compliance claim is restricted to linux/amd64, which is the
# tested operating environment for CMVP Cert #5247. Do not build linux/arm64 or
# linux/386 under a FIPS compliance claim — only linux/amd64 is tested.
#
# Verify the deployed binary with:  g8e version --fips
# =============================================================================

# Stage 1: Build (FIPS module linked in here for linux targets)
FROM golang:1.26.6 AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git make protobuf-compiler curl \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /build

# Copy go mod files and vendored dependencies for air-gapped builds.
# vendor/ must be copied as a directory (COPY vendor/ ./vendor/), not flattened
# into the workspace root — the Go toolchain expects vendor/modules.txt at
# ./vendor/modules.txt when GOFLAGS=-mod=vendor is set.
COPY go.mod go.sum ./
COPY vendor/ ./vendor/

# Use vendored modules — no network access required
ENV GOFLAGS=-mod=vendor

# Copy only the source the Go build needs. vendor/ was already copied above;
# don't re-copy it. This keeps the layer minimal and prevents unrelated file
# changes (tests, docs, scripts) from invalidating the build cache.
COPY Makefile ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY protocol/ ./protocol/
COPY docs/reference/ ./docs/reference/

# Build all platform binaries via `make build-all`. The Makefile sets
# GOFIPS140=v1.0.0 for linux targets and explicitly unsets it for non-linux
# targets. CGO_ENABLED=0 is intentional: the Go FIPS module is pure Go and
# does not require CGO. Binaries are written to /build/bin/g8e-{os}-{arch}.
RUN --mount=type=cache,target=/root/.cache/go-build \
    make build-all

# Verify the linux/amd64 binary built and runs.
RUN /build/bin/g8e-linux-amd64 --help

# Verify FIPS 140-3 approved mode is active via the native crypto/fips140 module
# API. This is the same self-check operators run in production
# (`g8e version --fips`); it inspects module state, not env vars. It exits 0
# with a warning when enforcement is off — that is the expected posture, not a
# failure.
RUN /build/bin/g8e-linux-amd64 version --fips

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

# Copy the linux/amd64 binary as the entrypoint
COPY --from=builder /build/bin/g8e-linux-amd64 /g8e

# Copy all platform binaries for node deployment via /.well-known/g8e/bin/
COPY --from=builder /build/bin/ /opt/g8e/bin/

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
