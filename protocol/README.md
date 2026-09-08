# g8e Protocol

The protocol directory contains the shared wire contracts, external constant registries, model schemas, workload identity helpers, generated language bindings, canonicalization vectors, and integration specifications for g8e-compatible clients and services.

## Packages

### Go

The Go protocol packages are part of the root module `github.com/g8e-ai/g8e/v2`, which requires Go 1.26.6. Install the module with `go get github.com/g8e-ai/g8e/v2`. Import `github.com/g8e-ai/g8e/v2/protocol` for workload identity helpers or a generated package under `github.com/g8e-ai/g8e/v2/protocol/proto/g8e/.../v1` for protobuf messages and services.

### Python

The `g8e` package requires Python 3.10 or later. Install it with `pip install g8e`. It provides generated protobuf modules and type stubs, selected protocol constants and enums, Pydantic models, canonical compliance protojson helpers, and Ed25519 receipt verification. See the [Python package README](python/README.md) for its public modules and runtime configuration.

### TypeScript

`protocol/node/` contains private generated TypeScript protobuf artifacts for repository consumers. It is not a published package. The generated modules use `@bufbuild/protobuf` and cover the common, compliance, operator, and pub/sub schemas.

## Sources of Truth

The protocol has several source types with distinct responsibilities:

- `proto/g8e/` defines canonical protobuf messages, enums, field options, and gRPC services.
- `internal/constants/` defines the Go protocol constants. The JSON registries in `constants/` are external protocol definitions and reference data; contract tests keep mirrored Go values aligned with them.
- `models/` defines JSON wire shapes used outside protobuf surfaces. The Python Pydantic models implement the supported subset and conformance tests verify their schema and serialization parity.
- `schemas/oscal/` embeds the authenticated NIST OSCAL 1.1.2 assessment-results schema and its provenance for offline validation.
- `vectors/` and `conformance/hash_vectors.json` define shared canonicalization and transaction-hash examples consumed by Go and Python tests.

The Python package loads its supported JSON registries from the bundled `g8e/_data/` directory. `make python-build` refreshes that bundle from `protocol/constants/` before building the package.

## Directory Map

- `proto/`: Protobuf schemas and generated Go code.
- `constants/`: External JSON registries, doctrine definitions, compliance catalogs, and compliance path definitions.
- `models/`: JSON model schemas and Python error enums.
- `schemas/`: Authenticated third-party schemas and provenance used by protocol consumers.
- `python/`: Installable Python package, generated protobuf modules, examples, and tests.
- `node/`: Generated TypeScript protobuf modules and their generation configuration.
- `docs/`: Governance, MCP, A2A, constants, and generated protobuf reference documentation.
- `examples/`: Go examples and MCP client configuration templates.
- `conformance/`: Cross-language constants, model, and transaction-hash tests.
- `vectors/`: Cross-language receipt, persistence-attestation, and compliance canonicalization vectors.

## Protocol Surfaces

### Protobuf

The protobuf module is `buf.build/g8e/platform`, configured in `proto/buf.yaml`.

- `g8e/common/v1/common.proto` defines governance envelopes and metadata, component identifiers, command intent, and platform enrollment messages.
- `g8e/operator/v1/operator.proto` defines the `OperatorService`, operator request and result payloads, execution and governance status enums, deterministic stage evidence, action receipts, commitment attestations, and persistence attestations.
- `g8e/pubsub/v1/pubsub.proto` defines the shared pub/sub message and event envelopes.
- `g8e/compliance/v1/compliance.proto` defines compliance catalogs, assessment scope and evidence, control assessments, trust policies, signed report bundles, verification results, analysis records, framework profiles, and demo scenario evidence.

Generated Go code lives beside each schema. Generated Python modules and `.pyi` stubs live under `python/g8e/`, generated TypeScript modules live under `node/src/gen/`, and generated Markdown references live under `docs/reference/api/`.

### Governance and receipts

`GovernanceEnvelope` carries identity, intent, replay-protection, state, and governance proof data for governed mutations. The [protocol specification](docs/spec.md) defines its wire contract and canonical transaction hash.

The execution path uses five interlocked layers:

1. **L1 Doctrine** applies hard gates, forbidden-pattern matching, and MITRE threat detection.
2. **L2 Consensus** verifies multi-agent consensus signatures with Ed25519.
3. **L3 Notary** verifies human authorization through WebAuthn or signed CLI proofs.
4. **L4 Warden** performs pre-dispatch signature, replay, expiry, nonce, and Merkle-root checks.
5. **L5 Actuator** dispatches isolated MCP or A2A operations, mints just-in-time capabilities, and produces signed receipts.

The Python `g8e.receipts` module strictly parses protojson receipts, reproduces canonical signed bytes, verifies Ed25519 signatures, and verifies final persistence attestations. Callers establish trust in the verification key separately.

### Constants and models

The top-level JSON registries cover events, statuses, collections, API paths, authentication values, headers, key-value keys, channels, pub/sub values, intents, prompts, agents, platform settings and enrollment, senders, exit codes, field paths, document identifiers, network values, output formats, ports, timestamps, environment variables, compliance paths, and platform enrollment transcript vectors. `constants/doctrine/` contains L1 detection and allowlist registries. `constants/compliance/` contains digest-verified assertion, framework, crosswalk, and demo scenario catalogs exposed to Go through `constants/compliance/catalogs.go` and bundled into the Python package.

The JSON schemas in `models/` cover persisted records, runtime and security configuration, governance data, filesystem and execution results, SSE payloads and wire messages, platform enrollment, WebAuthn data, and per-agent role definitions. `schemas/oscal/` provides the official NIST OSCAL 1.1.2 assessment-results schema with pinned SHA-256 and provenance metadata for offline, fail-closed validation. See the [constants reference](docs/constants.md) for registry conventions.

### Workload identities

The Go `protocol` package generates, matches, and extracts SPIFFE identities in the `g8e.local` trust domain:

- Operator: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`
- CLI: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`
- Application: `spiffe://g8e.local/app/<operator_id>`
- User: `spiffe://g8e.local/user/<user_id>`
- Hub: `spiffe://g8e.local/hub/operator-listen`
- Gateway peer: `spiffe://g8e.local/gateway/<gateway_id>`

The ensemble uses the application identity `spiffe://g8e.local/app/g8ee`. See the [workload identity example](examples/workload_identity/main.go) for generation, matching, extraction, and URL parsing.

### MCP and A2A

The [MCP specification](docs/mcp.md) defines governed tool, resource, and prompt integration. The [A2A specification](docs/a2a.md) defines typed agent-to-agent skill invocation. Reference JSON schemas and the upstream A2A protobuf are stored beside these specifications, while deployable MCP configuration templates live in `examples/mcp_server/`.

## Generation

Run generation from the repository root:

- `make proto` regenerates Go protobuf messages and gRPC stubs, Python protobuf modules and type stubs, TypeScript protobuf modules, generated Markdown API references, and downstream Python lockfiles.
- `make proto-go` regenerates Go protobuf outputs and Markdown API references.
- `make proto-python` regenerates Python protobuf outputs.
- `make proto-node` installs the Node generator when needed and regenerates TypeScript protobuf outputs.
- `make python-build` refreshes the bundled JSON data and builds the Python distribution.

The root `buf.gen.yaml` configures Go and Markdown generation. `node/buf.gen.yaml` configures TypeScript generation. From `protocol/`, `make python-proto` regenerates Python protobuf outputs and `make proto-check` verifies that committed Python outputs match the schemas.

## Verification

Run the checks that correspond to the changed protocol surface:

- `make -C protocol test` runs the Go protocol tests with race detection.
- `make -C protocol vet` runs Go static analysis for the protocol package.
- `make -C protocol proto-check` checks generated Python protobuf synchronization.
- `uv run --project protocol/python --extra dev pytest protocol/python/tests -v` runs the Python package tests from the repository root.
- `uv run --project protocol/python --extra dev pytest protocol/conformance -v` runs cross-language constants, model, and transaction-hash conformance tests from the repository root.
- `npm ci --prefix protocol/node && npm --prefix protocol/node run typecheck` verifies the generated TypeScript modules.

See the [conformance test guide](conformance/README.md) for the contracts covered by the cross-language suite.

## Examples

The [examples overview](examples/README.md) describes the Go governance-envelope and workload-identity programs and the MCP configuration templates. Python examples for constants and models live in `python/examples/`.

## Versioning

The protocol packages share the repository version in `VERSION`. Breaking wire changes increment the major version, backward-compatible protocol additions increment the minor version, and backward-compatible fixes increment the patch version. Python package metadata and generated dependency lockfiles remain synchronized with that version.

## Documentation Contributions

Follow the repository [documentation guide](../docs/devs/docs.md) when changing protocol prose, generated API references, schemas, registries, examples, or package documentation. Protocol changes require coordinated updates across every affected language binding, consumer, conformance test, and canonical reference.

## License

The protocol is licensed under the Business Source License 1.1 and converts to Apache License 2.0 on 2030-08-18. See `LICENSE`.

## Support

Open protocol questions and issue reports at https://github.com/g8e-ai/g8e.
