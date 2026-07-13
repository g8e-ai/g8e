# Protocol Examples

Reference implementations and configuration templates for the g8e protocol.

## Directory Structure

| Directory | Description |
|---|---|
| `governance_envelope/` | Go example demonstrating `GovernanceEnvelope` construction, protojson serialization, and round-trip parsing with a `CommandRequested` payload |
| `mcp_server/` | JSON configuration templates for MCP client integration |
| `workload_identity/` | Go example demonstrating SPIFFE workload identity generation, matching, and field extraction |

## Governance Envelope

Demonstrates the canonical governance envelope pattern:

1. Constructs a `GovernanceEnvelope` with all identity, intent, state, and governance proof fields
2. Creates a `CommandRequested` payload and marshals it as the envelope's `payload` bytes
3. Serializes the envelope to canonical protojson wire format
4. Parses the JSON back and extracts the command payload

**Run:**

```bash
cd protocol
go run ./examples/governance_envelope/
```

## MCP Server Configurations

Four configuration templates are provided:

| File | Transport | Use Case |
|---|---|---|
| `g8e_gateway_mcp_config.json` | HTTP + mTLS (literal cert paths) | Production with DNS or `/etc/hosts` configured for `g8e.local` |
| `g8e_gateway_mcp_config_env.json` | HTTP + mTLS (env-var cert paths) | Containerized deployments where cert paths are injected at runtime |
| `g8e_stdio_mcp_config.json` | Stdio (subprocess) | Stdio proxy to the gateway with full L1-L5 governance; requires a running gateway |
| `g8e_agent_mcp_config.json` | Stdio (subprocess) | Agent-specific config written by `g8e mcp agent run` with native tool exclusion for governance |

The canonical schemas are defined in `internal/services/mcp/config.go` (gateway and stdio) and `internal/cli/cmd/mcp.go` (agent). Use `g8e mcp agent show <agent>` to generate agent-specific configurations, or `g8e mcp agent run <agent>` to launch an agent with governance automatically.

## Workload Identity

Demonstrates SPIFFE workload identity operations:

- Generates SPIFFE IDs for operator, CLI, app, hub, user, and gateway peer workloads
- Validates identities with `Matches*` methods
- Extracts session IDs, user IDs, and gateway IDs from SPIFFE IDs
- Parses SPIFFE URLs

**Run:**

```bash
cd protocol
go run ./examples/workload_identity/
```
