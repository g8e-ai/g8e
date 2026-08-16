---
title: g8e Protocol Library
---

# g8e Protocol Library

Last Updated: 2026-08-16
Version: v1.7.5

The g8e Protocol Library is the canonical wire contract for all mutations in the g8e zero-trust execution platform. It provides schema definitions, JSON constant registries, JSON model schemas, Pydantic models, dynamic enum generation, SPIFFE workload identity helpers, and example programs for building compatible clients and services. Every mutation passing through the platform flows through a 5-layer interlock sequence:

- **L1 Doctrine**: Hard gates, forbidden pattern matching, MITRE threat detection.
- **L2 Consensus**: Multi-agent consensus signature verification (Ed25519).
- **L3 Notary**: Human-in-the-loop authorization (WebAuthn or signed CLI proofs).
- **L4 Warden**: Pre-dispatch verification (signatures, replay prevention, expiry, nonces, Merkle root).
- **L5 Actuator**: Isolated tool dispatch (MCP/A2A), JIT capability minting, and signed receipt production.

The protocol publishes as two independent packages: a Go module sharing the platform root module path and a Python package. Both packages share a single unified version number with the platform binary. There are no separate protocol-only releases. Every release ships the platform binary, the Go module, and the Python package at the same version.

---

## Table of Contents

- [Go Protocol Package](#go-protocol-package)
  - [Requirements & Installation](#go-requirements--installation)
  - [Module Path & Versioning](#go-module-path--versioning)
  - [Package Overview](#go-package-overview)
  - [Workload Identity Helpers](#go-workload-identity-helpers)
  - [Development & Tooling](#go-development--tooling)
- [Python Protocol Package](#python-protocol-package)
  - [Requirements & Installation](#python-requirements--installation)
  - [Package Overview](#python-package-overview)
  - [Constants & Enums](#python-constants--enums)
  - [Pydantic Models](#python-pydantic-models)
  - [Environment Configuration](#python-environment-configuration)
- [Shared Protocol Assets](#shared-protocol-assets)
  - [Constants Registries](#constants-registries)
  - [JSON Model Schemas](#json-model-schemas)
  - [MCP Server Configurations](#mcp-server-configurations)
- [Protobuf Code Generation](#protobuf-code-generation)
- [Release & Distribution](#release--distribution)
  - [Unified Versioning](#unified-versioning)
  - [Release Workflow](#release-workflow)
  - [CI Workflows](#ci-workflows)
  - [Version Sync Enforcement](#version-sync-enforcement)
- [Conformance Tests](#conformance-tests)
- [Directory Structure](#directory-structure)
- [References](#references)

---

## Go Protocol Package

### Go Requirements & Installation

The Go protocol package requires Go 1.26.6 or later. Direct dependencies include `google.golang.org/grpc v1.83.0` and `google.golang.org/protobuf v1.36.11`; remaining dependencies are managed through the root `go.mod`.

Install or update the Go module using standard Go tooling:

```bash
go get github.com/g8e-ai/g8e@v1.7.5
```

To fetch the latest release:

```bash
go get github.com/g8e-ai/g8e@latest
```

### Go Module Path & Versioning

The Go protocol implementation belongs to the root module `github.com/g8e-ai/g8e`. There is no separate module file for the protocol directory. Module versions derive from git tags of the form `vX.Y.Z`, created during the release workflow. The Go module proxy resolves release versions directly from these tags.

Import paths use `github.com/g8e-ai/g8e/protocol/...`. Consumers configure their module requirements by referencing the root module `github.com/g8e-ai/g8e vX.Y.Z`. See [Release & Distribution](#release--distribution) for version sync details.

### Go Package Overview

The Go protocol package provides the canonical wire structures and helpers for platform interaction.

- **Generated Protobuf Types**: Contains compiled Go structs and gRPC client stubs generated from protobuf schemas.
- **Governance Types**: Provides structures for the canonical transaction envelope, governance metadata, consensus votes, and threat pattern options.
- **Operator Services**: Defines gRPC service interfaces for command execution, file modifications, filesystem inspection, and governance verification.
- **Pub/Sub Messages**: Defines pub/sub message and event transport structures for event distribution across nodes.

### Go Workload Identity Helpers

Workload identity helpers manage SPIFFE identity generation and validation for the `g8e.local` trust domain. Six workload identity types are supported:

- **Operator**: Identifies target execution node instances.
- **CLI**: Identifies authenticated command line interface sessions.
- **App**: Identifies external application integrations.
- **User**: Identifies human user sessions.
- **Hub**: Identifies central gateway listener endpoints.
- **GatewayPeer**: Identifies peer gateway nodes in distributed setups.

Workload helpers format SPIFFE identifiers and validate incoming request identities against expected trust domain schemes. Session validation verifies CLI identities during initial authentication before loading broader user context.

### Go Development & Tooling

Protocol development uses standard Make targets defined in the protocol build configuration:

- `make test`: Runs unit tests with race detection enabled.
- `make fmt`: Formats source files using standard formatting rules.
- `make vet`: Executes Go static analysis checks.
- `make lint`: Runs configured linter checks across the package.
- `make openapi`: Stub target that prints setup instructions for OpenAPI generation from protobuf; use `make proto` to regenerate protobuf artifacts.

---

## Python Protocol Package

### Python Requirements & Installation

The Python package requires Python 3.10 or later (tested on 3.10 through 3.14). Runtime dependencies are `pydantic>=2.0.0` and `protobuf>=4.0.0`. The build system uses `setuptools>=61.0` and `wheel`.

Install the package from PyPI using pip:

```bash
pip install g8e
```

To pin a specific release version:

```bash
pip install g8e==1.7.5
```

### Python Package Overview

The Python package installs as `g8e` and provides type-checked models and runtime constants for Python applications. It includes standard type markers for static type-checker support. Unit tests cover constant loading, enum generation, model validation, and cross-language parity.

### Python Constants & Enums

The constants module loads protocol JSON constant files at import time. It exports dictionaries for event types, status codes, database collections, HTTP headers, pub/sub channels, intent categories, prompt templates, timestamps, document identifiers, platform settings, and network configurations.

Dynamic enum generation builds string and integer enums from the underlying constant registries. Key features include:

- Member names use uppercase snake case formatting.
- Values preserve raw wire format strings and integer status codes.
- Integer categories produce integer enums, while text categories produce string enums.
- Enums are evaluated lazily upon access and cached in memory.

### Python Pydantic Models

Pydantic v2 models define protocol structures with strict field validation. All models extend a common base class that ignores extra fields and formats UTC timestamps to standard ISO 8601 strings with a `Z` suffix.

- **Request Context**: Validates session identity for client requests, requiring valid session identifiers and user context.
- **Event Models**: Defines event payload structures for streaming events, background execution, and tool interactions.
- **Governance Envelope**: Represents the canonical transaction container, carrying identity, intent, state Merkle roots, and governance proofs.
- **Settings Models**: Represents platform and user configuration parameters for search, evaluation judges, and execution limits.

### Python Environment Configuration

The constant loader resolves the protocol definition directory using a four-level priority hierarchy:

1. `G8E_PROTOCOL_DIR` environment variable: Uses specified filesystem path if set.
2. Bundled package data: Uses packaged JSON data included with PyPI installations.
3. Relative checkout path: Uses relative repository paths during local development.
4. Container fallback: Uses standard container path locations in deployment images.

Override the resolution directory by setting the environment variable:

```bash
export G8E_PROTOCOL_DIR=/custom/path/to/protocol
```

---

## Shared Protocol Assets

### Constants Registries

JSON files serve as the single source of truth for protocol identifiers, endpoint paths, and default configurations. Both Go and Python packages consume these definitions to maintain cross-language alignment.

Registries cover event names, status codes, collection names, API paths, authentication parameters, HTTP headers, key-value keys, channels, pubsub definitions, intents, prompt templates, agent roles, platform settings, senders, exit codes, field paths, document types, network parameters, output formats, default ports, timestamp formats, and environment variable names. Threat detection pattern registries define forbidden execution patterns for L1 Doctrine evaluation.

### JSON Model Schemas

JSON Schema files define structural validation rules for data structures across the platform. Managed schemas include account locks, application policies, bound sessions, cases, CLI sessions, conversations, operator sessions, organizations, passkey challenges, personas, platform settings, reputation commitments, security constraints, tasks, consensus configurations, users, and web sessions.

Per-agent role schemas define tailored models for primary, assistant, lite, triage, title generator, and agent harness roles.

### MCP Server Configurations

Example Model Context Protocol (MCP) server configurations demonstrate deployment topologies:

- **Gateway mTLS Configuration**: HTTP with mTLS using certificate paths for production deployments.
- **Containerized mTLS Configuration**: HTTP with mTLS using environment variables for container environments.
- **Stdio Configuration**: Local development configuration proxying local tool calls to a gateway over mTLS.
- **Agent Governance Configuration**: Enforces L1 through L5 governance by disabling ungoverned native agent tools and routing all tool operations through governed gateway endpoints.

---

## Protobuf Code Generation

Protobuf code generation is managed with `buf`. Generation behavior is configured in the root code generation configuration file `buf.gen.yaml`.

Compile schemas from the repository root using the Buf-based `proto` target:

```bash
make proto
```

`make proto` installs the Buf CLI if it is not present and then runs `buf generate protocol/proto` to produce the Go and Markdown artifacts.

Code generation produces Go structs, gRPC stubs, and Markdown API reference documentation in target protocol directories. Module configuration resides in the protobuf schema root.

---

## Release & Distribution

### Unified Versioning

The protocol packages and platform binary share a single version number tracked in the `VERSION` file at the repository root. Versioning follows Semantic Versioning guidelines:

- **MAJOR**: Breaking protocol changes requiring client updates.
- **MINOR**: Backward-compatible new protocol features.
- **PATCH**: Backward-compatible bug fixes and minor updates.

Version strings use a `v` prefix (`v1.7.5`) in tags, repository version files, and documentation headers. Python distribution files omit the prefix (`1.7.5`).

### Release Workflow

Releases are executed through a single orchestration command:

```bash
make release
```

The release target executes the following sequence:

1. Syncs Python package metadata files with the current version file.
2. Confirms that the working tree contains no uncommitted changes.
3. Verifies that the release notes file exists for the targeted version.
4. Confirms that git tags for the platform and protocol do not exist.
5. Creates platform and protocol git tags on the current commit.
6. Pushes both tags to the remote repository origin.

### CI Workflows

Tag pushes trigger automated GitHub Actions release pipelines:

- **Binary Release Workflow**: Builds platform binaries for supported platforms and architectures, generates SHA-256 checksums, signs assets with cosign, and creates GitHub releases.
- **Python Protocol Workflow**: Validates package metadata, bundles JSON constant registries, builds source distributions and wheels, uploads packages to PyPI via trusted publishing, and verifies cross-platform installation.

### Version Sync Enforcement

The release task verifies version alignment between Python package metadata and the root version file before creating git tags. CI test workflows enforce version alignment on pull requests and pushes, failing the build if package versions diverge. Go module versions derive directly from git tags without manual version file duplication.

---

## Conformance Tests

Cross-language conformance tests validate that constants and models remain identical across Go and Python implementations. Test scripts verify:

- Constant file structural integrity, field presence, and uniqueness rules across all registries.
- Model schema alignment and validation rule enforcement between Pydantic models and JSON schemas.
- SHA-256 transaction hash parity between Python and Go implementations using shared test vector files.

CI workflows run conformance tests on all supported Python runtime versions during pull request validation.

---

## Directory Structure

The protocol implementation is organized into functional subdirectories within the repository:

- `protocol/`: Root directory containing package documentation and SPIFFE workload identity helpers.
- `protocol/proto/`: Protobuf schema definitions and generated Go code for common, operator, and pub/sub packages.
- `protocol/constants/`: JSON protocol constant registries, including L1 Doctrine threat detection patterns.
- `protocol/models/`: JSON Schema definitions for platform models and agent role schemas.
- `protocol/conformance/`: Cross-language test suites for constant structure, model schema, and hash parity verification.
- `protocol/python/`: Python package implementation, including constants loaders, dynamic enums, and Pydantic models.
- `protocol/examples/`: Sample programs demonstrating envelope construction, workload identity, and MCP server configurations.
- `protocol/docs/`: Protocol specification documents and generated API reference documentation.

---

## References

- [Protocol Specification](../../protocol/docs/spec.md): Canonical envelope structure and 5-layer interlock sequence details.
- [Constants Reference](../../protocol/docs/constants.md): Platform constant system details.
- [A2A Protocol](../../protocol/docs/a2a.md): Agent-to-Agent protocol integration guide.
- [MCP Protocol](../../protocol/docs/mcp.md): Model Context Protocol integration guide.
- [Release Process](../devs/release_process.md): Release procedures and version management guidelines.
- [Network Architecture](./network.md): Network topology, mTLS, and SPIFFE identity details.
