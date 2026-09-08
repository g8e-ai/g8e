# Protocol Examples

This directory contains runnable Go examples and MCP client configuration templates for the g8e protocol. Run the Go examples from the repository root because the governance envelope example imports the repository's internal transaction-hash implementation.

## Directory Structure

| Directory | Description |
| --- | --- |
| `governance_envelope/` | Constructs, signs, serializes, and parses a `GovernanceEnvelope` with a `CommandRequested` payload |
| `mcp_server/` | Provides JSON templates for direct gateway, governed stdio, and agent-launch MCP configurations |
| `workload_identity/` | Generates, matches, extracts fields from, and parses g8e SPIFFE workload identities |

## Governance Envelope

The governance envelope example demonstrates these operations:

1. Generates ephemeral Ed25519 keys for an L2 consensus signer and an operator, then derives a mock CLI certificate fingerprint.
2. Constructs a representative `GovernanceEnvelope` with identity, intent, state, application context, L1 metadata, one L2 vote, and a CLI/mTLS L3 proof.
3. Marshals a `CommandRequested` protobuf message into the envelope's `payload` bytes.
4. Computes the transaction hash from the hash-bound envelope fields with `governance.GenerateMessageID`.
5. Signs `<transaction_hash>|true` for the L2 vote, signs the transaction hash for the L3 CLI proof, and assigns the hash to both `id` and `transaction_hash`.
6. Serializes the envelope with protojson, parses it back, and unmarshals the command payload.

Run `go run ./protocol/examples/governance_envelope/` from the repository root. The program uses generated keys, mock identity values, and a mock certificate fingerprint; it does not submit the envelope to a gateway or run the five-layer verification sequence. The canonical envelope schema is `protocol/proto/g8e/common/v1/common.proto`, and the transaction-hash implementation is `internal/governance/envelope.go`.

## MCP Server Configurations

The `mcp_server/` directory contains four templates:

| File | Transport | Use case |
| --- | --- | --- |
| `g8e_gateway_mcp_config.json` | Streamable HTTP with mTLS certificate paths | Connect to `https://g8e.local:8443/mcp` after configuring DNS or `/etc/hosts` and replacing the placeholder certificate paths |
| `g8e_gateway_mcp_config_env.json` | Streamable HTTP with mTLS certificate-path environment variables | Supply `G8E_CLIENT_CERT`, `G8E_CLIENT_KEY`, and `G8E_CA_BUNDLE` in environments that inject certificate paths at runtime |
| `g8e_stdio_mcp_config.json` | Stdio subprocess | Start `g8e mcp stdio`, which proxies requests to a running gateway over mTLS and applies L1 through L5 governance |
| `g8e_agent_mcp_config.json` | Stdio subprocess plus native-tool exclusions | Show the temporary JSON shape used when `g8e mcp agent run` launches Claude or Codex; the command also applies strict MCP and native-tool-disabling launch flags |

The Go types that produce the gateway and stdio configurations are in `internal/services/mcp/config.go`. Agent-specific configuration writers and launch arguments are in `internal/cli/cmd/mcp.go`; Goose, Gemini, and Devin use different configuration formats or tool-disabling mechanisms from the agent JSON template.

Use `g8e mcp agent list` to list supported agents. `g8e mcp agent show <agent>` validates a supported agent name and prints the available `g8e.local` mTLS, direct-IP mTLS, and stdio configurations. `g8e mcp agent run <agent>` starts the gateway when necessary, enrolls the CLI and agent identities, configures the agent to use `g8e mcp stdio`, and launches the agent with L1 through L5 governance. By contrast, `g8e mcp agent run -- <command>` and `g8e mcp agent run --url <url>` wrap an external MCP server with L1 doctrine screening only.

See the [MCP protocol documentation](../docs/mcp.md) for transport behavior, authentication, and governance details.

## Workload Identity

The workload identity example demonstrates these operations in the `g8e.local` trust domain:

- Generates operator, CLI, application, ensemble, hub, user, and gateway peer SPIFFE IDs.
- Matches complete identities and CLI sessions, and recognizes application, ensemble, and user SAN identities.
- Extracts CLI session IDs, CLI and user-SAN user IDs, operator session IDs, and gateway IDs.
- Produces parsed `url.URL` values for operator, CLI, application, user, hub, and gateway peer identities.

Run `go run ./protocol/examples/workload_identity/` from the repository root. See the [protocol overview](../README.md#workload-identities) for the identity formats.
