---
doc_id: troubleshooting
title: Troubleshooting Guide
audience: maintainers and coding agents
status: current
last_updated: 2026-09-26
version: v2.2.0
owners:
  - internal/cli/cmd/
  - internal/services/gateway/
  - internal/services/auth/
  - internal/services/vault/
related:
  - docs/devs/devs.md
  - docs/devs/codemap.md
  - docs/devs/tests.md
  - docs/devs/docs.md
  - docs/devs/release_process.md
when_to_read: Diagnosing development, build, test, gateway startup, authentication, enrollment, or runtime errors.
do_not_use_for:
  - Platform coding standards and invariants (docs/devs/devs.md)
  - Full documentation catalog and writing rules (docs/devs/docs.md)
  - Release execution and compliance bundles (docs/devs/release_process.md)
  - Package and runtime ownership maps (docs/devs/codemap.md)
---

# Troubleshooting Guide

## Purpose

Diagnose and resolve common contributor setup, build, test, Gateway startup, authentication, vault, governance, and deployment failures across the g8e platform.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Environment and binary](#environment-and-binary-inv-trouble-env), [Gateway startup and vault](#gateway-startup-and-vault-inv-trouble-gw), [Authentication and trust](#authentication-and-trust-inv-trouble-auth).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### Environment and binary (`INV-TROUBLE-ENV`)

| ID | Rule |
| --- | --- |
| INV-TROUBLE-ENV-01 | CLI commands MUST be run from the checkout root. Running from a subdirectory resolves a different `.g8e/` runtime tree. |
| INV-TROUBLE-ENV-02 | The Go toolchain MUST satisfy the `go` directive in `go.mod` (Go 1.26.6). |
| INV-TROUBLE-ENV-03 | A newly built binary MUST be produced with `make build`, which compiles `cmd/g8e` and places the runnable executable at `./g8e`. |

### Gateway startup and vault (`INV-TROUBLE-GW`)

| ID | Rule |
| --- | --- |
| INV-TROUBLE-GW-01 | The Gateway requires an unlocked vault during startup. On first initialization, `.g8e/vault/` and `.g8e/vault/key` are generated. Lost vault keys make data unrecoverable; MUST NOT overwrite or delete vault keys without explicit data disposal intent. |
| INV-TROUBLE-GW-02 | `gw restart` reads the launch profile at `.g8e/pids/operator-launch-profile.json`. If missing or invalid, restart fails closed; start with explicit flags to generate a new launch profile. |
| INV-TROUBLE-GW-03 | `gw reset` and `gw clean` are destructive commands that remove the local `.g8e/` runtime tree. MUST NOT use either command to repair retained state. |

### Authentication and trust (`INV-TROUBLE-AUTH`)

| ID | Rule |
| --- | --- |
| INV-TROUBLE-AUTH-01 | The canonical trust bundle path is `.g8e/pki/trust/g8eg-ca-bundle.pem`. Tests and tools MUST NOT use deprecated bundle paths or mutate host PKI to fix test failures. |
| INV-TROUBLE-AUTH-02 | CLI sessions and certificates expire in 7 days; web sessions expire in 24 hours. When a CLI certificate is valid but session expired, run `g8e auth refresh` rather than re-enrolling. |
| INV-TROUBLE-AUTH-03 | Headless enrollment (`g8e auth enroll user --headless`) skips OS trust and passkey ceremony, producing an mTLS-capable CLI identity that does not support web Console sessions. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Go version requirement | `go.mod` | `go 1.26.6` |
| Platform build target | `Makefile`, `cmd/g8e` | `make build` |
| Gateway launch profile | `internal/cli/serve/gateway.go` | `.g8e/pids/operator-launch-profile.json` |
| Vault key path | `internal/constants/paths.go` | `VaultKeyRel` (`vault/key`) |
| CA trust bundle | `internal/constants/paths.go` | `PKITrustDirRel/g8eg-ca-bundle.pem` |

## Procedures

### Basic Health and Toolchain Triage

```bash
pwd
ls README.md g8e Makefile VERSION
./g8e version
go version
```

### Build and Generation Triage

```bash
# Rebuild the root CLI binary
make build
chmod +x g8e

# Validate prerequisites for protocol regeneration
command -v go
command -v buf || test -x ./buf
command -v uv
make proto
```

### Gateway Process Diagnosis

```bash
# Check running Gateway status and logs
./g8e gw status
./g8e gw logs

# Run in the foreground with debug logging
./g8e gw stop
./g8e gw start --follow --log debug
```

### Vault Validation

```bash
# Validate existing vault header and key pair
./g8e vault unlock --vault-dir .g8e/vault --key-path .g8e/vault/key

# Start Gateway with an explicit absolute vault key path
./g8e gw start --vault-key "$PWD/.g8e/vault/key"
```

### CLI Authentication and Session Recovery

```bash
# Refresh an expired CLI session with a valid certificate
./g8e auth refresh

# Force certificate rotation before expiry
./g8e auth enroll user --rotate-cli

# Headless enrollment on a remote or non-browser server
./g8e auth enroll user --headless
```

## Anti-patterns

- Running `./g8e` from outside the repository root, creating disjoint `.g8e/` state trees (INV-TROUBLE-ENV-01).
- Running `gw reset` or `gw clean` to fix transient errors when state must be retained (INV-TROUBLE-GW-03).
- Mutating OS trust or developer PKI state to bypass test failures (INV-TROUBLE-AUTH-01).
- Re-enrolling and wiping valid certificates when a simple `auth refresh` suffices (INV-TROUBLE-AUTH-02).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Code Map](codemap.md): package and runtime ownership maps.
- [Testing Guide](tests.md): test tiers, fixtures, and CI scope.
- [Documentation Guide](docs.md): documentation audit, catalog, and formatting rules.
- [Release Process](release_process.md): versioning and release verification.
