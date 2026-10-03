# g8e Examples

Runnable examples, client configuration templates, and reference shapes for integrating with g8e. This is the single home for examples. Sealed, deployable demo environments live separately in [`demos/`](../demos/README.md).

## Index

| Directory | Kind | Description | Run |
| --- | --- | --- | --- |
| [`external-console/`](external-console/README.md) | Browser app | Static SPA that talks directly to a local Gateway (WebAuthn, SSE, approvals) | `examples/external-console/serve.sh` |
| [`governance-envelope/`](governance-envelope/main.go) | Go program | Builds, signs, serializes, and parses a `GovernanceEnvelope` | `go run ./examples/governance-envelope/` |
| [`workload-identity/`](workload-identity/main.go) | Go program | Generates, matches, extracts, and parses g8e SPIFFE workload identities | `go run ./examples/workload-identity/` |
| [`mcp-client-configs/`](mcp-client-configs/) | JSON templates | MCP client configurations for direct Gateway, governed stdio, and agent launch | Copy and edit |
| [`python/`](python/) | Python scripts | Usage of the `g8e` Python protocol package (constants, models) | `python examples/python/constants_example.py` |
| [`eval/`](eval/) | JSON template | Minimal init-campaign queue shape | Reference only |

Run the Go examples from the repository root: the governance envelope example imports the repository's internal transaction-hash implementation. The Python examples require the package installed from `protocol/python` (`pip install -e protocol/python`), and CI runs them as a smoke test.

## Governance Envelope

The governance envelope example demonstrates these operations:

1. Generates ephemeral Ed25519 keys for an L2 consensus signer and an operator, then derives a mock CLI certificate fingerprint.
2. Constructs a representative `GovernanceEnvelope` with identity, intent, state, application context, L1 metadata, one L2 vote, and a CLI/mTLS L3 proof.
3. Marshals a `CommandRequested` protobuf message into the envelope's `payload` bytes.
4. Computes the transaction hash from the hash-bound envelope fields with `governance.GenerateMessageID`.
5. Signs `<transaction_hash>|true` for the L2 vote, signs the transaction hash for the L3 CLI proof, and assigns the hash to both `id` and `transaction_hash`.
6. Serializes the envelope with protojson, parses it back, and unmarshals the command payload.

The program uses generated keys, mock identity values, and a mock certificate fingerprint; it does not submit the envelope to a gateway or run the five-layer verification sequence. The canonical envelope schema is `protocol/proto/g8e/common/v1/common.proto`, and the transaction-hash implementation is `internal/governance/envelope.go`.

## MCP Client Configurations

The `mcp-client-configs/` directory contains three templates:

| File | Transport | Use case |
| --- | --- | --- |
| `g8e_gateway_mcp_config.json` | Streamable HTTP with mTLS certificate paths | Connect to `https://g8e.local:8443/mcp` after configuring DNS or `/etc/hosts` and replacing the placeholder certificate paths |
| `g8e_stdio_mcp_config.json` | Stdio subprocess | Start `g8e mcp stdio`, which proxies requests to a running gateway over mTLS and applies L1 through L5 governance |
| `g8e_agent_mcp_config.json` | Stdio subprocess plus native-tool exclusions | Show the temporary JSON shape used when `g8e mcp agent run` launches Claude or Codex, where `--app claude` selects the agent's owner-approved application identity; the command also applies strict MCP and native-tool-disabling launch flags |

The Go types that produce the gateway and stdio configurations are in `internal/services/mcp/config.go`. Agent-specific configuration writers and launch arguments are in `internal/cli/cmd/mcp/`; Goose, Gemini, and Devin use different configuration formats or tool-disabling mechanisms from the agent JSON template.

Use `g8e mcp agent list` to list supported agents. `g8e mcp agent show <agent>` validates a supported agent name and prints the available `g8e.local` mTLS, direct-IP mTLS, and stdio configurations. `g8e mcp agent run <agent>` starts the gateway when necessary, enrolls the CLI and agent identities, configures the agent to use `g8e mcp stdio`, and launches the agent with L1 through L5 governance. Third-party MCP servers are governed through Gateway downstream egress.

See the [MCP protocol documentation](../protocol/docs/mcp.md) for transport behavior, authentication, and governance details.

## Workload Identity

The workload identity example demonstrates these operations in the `g8e.local` trust domain:

- Generates operator, CLI, application, ensemble, hub, user, and gateway peer SPIFFE IDs.
- Matches complete identities and CLI sessions, and recognizes application, ensemble, and user SAN identities.
- Extracts CLI session IDs, CLI and user-SAN user IDs, operator session IDs, and gateway IDs.
- Produces parsed `url.URL` values for operator, CLI, application, user, hub, and gateway peer identities.

See the [protocol overview](../protocol/README.md#workload-identities) for the identity formats.

## Python

`python/constants_example.py` shows constants and headers usage; `python/models_example.py` shows model instantiation, serialization, validation, and observe producer request construction. Observe API read models, producer request types, observe event payloads, and public spectator feed schemas are defined in `protocol/models/observe_api.json`, `protocol/models/observe_event_payloads.json`, and `protocol/models/public_feed.json`. Browser integration uses the audited `g8e-adapter` contract pack under `g8e-adapter/contract-pack/`.

## Eval

`eval/init-campaign-queue.example.json` is a minimal queue shape reference. The full checked-in evaluation data layout is documented in [`eval/README.md`](../eval/README.md).
