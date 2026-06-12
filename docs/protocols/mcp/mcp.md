---
title: MCP Protocol
---

# MCP Protocol

Last Updated: 2026-06-11

The g8e Operator in gateway mode supports Model Context Protocol (MCP) integration. MCP clients send JSON-RPC tool calls to the gateway, which wraps them in the g8e governance envelope, runs them through the 5-layer governance verification sequence (L1Doctrine/L2Consensus/L3Notary/L4Warden/L5Actuator), and dispatches verified payloads to downstream MCP servers or to the in-process execution service for local execution.

---

## Protocol Overview

MCP is a JSON-RPC 2.0 protocol for AI tool invocation. MCP clients connect to the gateway via standard JSON-RPC endpoints with structured tool invocation payloads.

### Request Structure

MCP requests follow a JSON-RPC 2.0 pattern:

- **Transport**: JSON-RPC 2.0 over HTTP
- **Authentication**: mTLS certificate for MCP ingress routes
- **Payload**: JSON-RPC request with method, params, and id

### Gateway Integration

The gateway translates MCP tool invocations into governance envelopes:

1. **Inbound**: MCP client sends JSON-RPC tool invocation to gateway
2. **Envelope Construction**: Gateway wraps payload in `GovernanceEnvelope` with action type `MCP_CALL`, `MCP_RESOURCE_READ`, `MCP_RESOURCE_LIST`, `MCP_PROMPT_GET`, `MCP_PROMPT_LIST`, or `A2A_CALL`
3. **Verification**: Envelope passes through L1/L2/L3/L4/L5 verification gates
4. **Dispatch**: Verified envelope forwarded to downstream MCP server, local execution, or A2A peer

### Local Tool Execution

The gateway handles certain tools locally without downstream proxy:

- **read_field**: JIT field resolution from governed collections with L1 field path validation, L3 session validation, and audit vault logging. Requires `collection`, `document_id`, `field_path`, and `operator_session_id` parameters.
- **Native tools**: The Operator includes 29 native tools that execute within the Operator's execution boundary without proxying to downstream MCP servers:
  - `db_discover_topology`: Automatically scans database schemas, tables, and column data types, returning a highly compressed JSON map
  - `db_query_validate`: Validates SQL queries using EXPLAIN QUERY PLAN to detect full table scans and performance issues
  - `db_isolated_read`: Executes SELECT statements in read-only mode against a SQLite database
  - `db_index_triage`: Queries database fragmentation statistics and index information
  - `log_stream_filter`: Reads log files and applies regex filtering with sensitive data scrubbing
  - `sys_oom_detect`: Scans system logs for OOM (Out of Memory) killer events
  - `config_diff_mask`: Compares configuration files with automatic secret masking for sensitive values
  - `proc_metric_top`: Parses /proc to extract top resource-consuming processes by CPU and memory
  - `fs_disk_profile`: Recursively calculates directory sizes and disk usage
  - `proc_signal_safe`: Sends signals to processes with denylist enforcement for protected PIDs
  - `net_socket_audit`: Inspects active network sockets (TCP/UDP) from /proc/net
  - `net_endpoint_ping`: Performs TCP handshake to verify network endpoint connectivity and measure latency
  - `net_http_probe`: Performs lightweight HTTP requests to probe web endpoints
  - `sys_info`: Provides system information including hostname, OS version, kernel, uptime, and load average
  - `net_dns_resolve`: Performs DNS resolution (dig/nslookup equivalent) for network debugging
  - `tls_cert_inspect`: Parses TLS certificates, verifies chains, and checks expiration (critical for PKI debugging)
  - `sys_env_vars`: Reads environment variables for configuration debugging
  - `fs_file_checksum`: Computes SHA256/MD5 checksums for file integrity verification
  - `sys_service_status`: Checks systemd service status (operator, gateway, etc.)
  - `sys_container_status`: Checks container health status (podman)
  - `fs_disk_usage`: Provides df-style free space reporting for mounted filesystems
  - `sys_time_clock`: Provides NTP sync status and system time verification
  - `proc_tree`: Provides parent-child process relationships and process tree
  - `git_ops`: Provides git repository operations including status, log, branch info, and remote management for GitHub/GitLab workflows
  - `cloud_metadata`: Detects cloud provider (AWS, Azure, GCP) and retrieves instance metadata including region, instance type, and availability zone
  - `k8s_inspect`: Provides Kubernetes cluster inspection including pods, nodes, services, and deployment status
  - `shell_execute`: Executes shell commands with denylist enforcement for dangerous operations and timeout limits. Supports multi-host execution via SSH with optional `hostnames` parameter (defaults to localhost)
  - `net_ssh_known_hosts`: Lists known hosts from SSH config and known_hosts files based on OS type
  - `operator_deploy`: Deploys the g8e operator to a list of remote hosts via SSH

---

## MCP Payload Types

### MCP_CALL

The gateway maps MCP tool invocations to the `MCP_CALL` action type with the `McpCallRequested` proto payload:

| Field | Type | Description |
|---|---|---|
| `tool_name` | string | Name of the tool to invoke (with L1 forbidden patterns) |
| `arguments_json` | string | JSON-encoded arguments object exactly as client supplied |
| `execution_id` | string | Optional client-supplied invocation identifier |

### MCP_RESOURCE_READ

Resource read operations map to `MCP_RESOURCE_READ` with `McpResourceReadRequested`:

| Field | Type | Description |
|---|---|---|
| `uri` | string | Resource URI to read (e.g., "file:///path", "memory://var") |
| `execution_id` | string | Optional client-supplied invocation identifier |

### MCP_PROMPT_GET

Prompt template retrieval maps to `MCP_PROMPT_GET` with `McpPromptGetRequested`:

| Field | Type | Description |
|---|---|---|
| `name` | string | Prompt name/template identifier |
| `execution_id` | string | Optional client-supplied invocation identifier |

### MCP_RESOURCE_LIST

Resource listing maps to `MCP_RESOURCE_LIST` with `McpResourceListRequested`:

| Field | Type | Description |
|---|---|---|
| `execution_id` | string | Optional client-supplied invocation identifier |

### MCP_PROMPT_LIST

Prompt listing maps to `MCP_PROMPT_LIST` with `McpPromptListRequested`:

| Field | Type | Description |
|---|---|---|
| `execution_id` | string | Optional client-supplied invocation identifier |

### A2A_CALL

Agent-to-Agent skill invocations map to `A2A_CALL` with `A2aCallRequested`:

| Field | Type | Description |
|---|---|---|
| `skill_name` | string | Target agent skill or capability name (with L1 forbidden patterns) |
| `payload_json` | string | JSON-encoded A2A task payload |
| `execution_id` | string | Optional client-supplied invocation identifier |

### Canonical JSON Wire Format

All envelopes use canonical JSON (protojson) encoding for client-facing surfaces:

- Schema source of truth: `.proto` files in `protocol/proto/`
- Wire format: canonical JSON (protojson)
- Signing basis: deterministic `transaction_hash` computed from normalized envelope fields
- Internal storage: protobuf bytes (implementation detail)

This ensures compatibility with JSON-based ecosystems while maintaining typed schema validation.

---

## Client Integration

### Stdio Transport Modes

g8e provides two stdio MCP transport modes. Choose based on your deployment requirements:

| Command | Governance | Gateway required | Downstream |
|---|---|---|---|
| `g8e mcp stdio` | L1-L5 full stack | Yes (running) | g8e gateway with full governance |
| `g8e mcp agent run` | L1 inline (MITRE ATT&CK) | No | Any MCP server (subprocess or HTTP) |

### MCP Client Configuration

The g8e Gateway provides configuration commands for different agent integrations. Use the `g8e mcp agent show` command to display configuration options for specific AI coding tools.

#### Configuration Display

To view MCP client configurations for a specific agent:

```bash
g8e mcp agent show <agent>
```

Replace `<agent>` with one of the supported agents:
- `claude` - Anthropic Claude Desktop / Claude Code
- `codex` - OpenAI Codex AI coding assistant
- `cursor` - Cursor AI IDE
- `devin` - Devin AI IDE (formerly Windsurf)
- `vscode` - Visual Studio Code with MCP extension
- `continue` - Continue.dev AI coding assistant
- `aider` - Aider AI pair programmer
- `codeium` - Codeium AI assistant
- `tabby` - Tabby AI autocomplete
- `ollama` - Ollama local LLM runner
- `generic` - Generic MCP-compatible agent

To list all supported agents:

```bash
g8e mcp agent list
```

The `show` command displays three configuration modes:

**g8e.local (mTLS)**: Production environments with DNS configured. Requires DNS or `/etc/hosts` entry for `g8e.local` resolution. Suitable for Cursor, Devin, VS Code MCP clients.

**IP Address (mTLS)**: Environments without DNS or for direct IP access. Uses external interface IP without DNS setup. Suitable for Cursor, Devin, VS Code MCP clients.

**Stdio Transport**: Direct native tool access without gateway. Requires g8e binary in PATH or full path in config. Suitable for Claude Code, Cursor, Devin, VS Code MCP clients.

#### Claude Code & Codex Custom Connector Registration

The g8e Gateway provides a unified MCP Streamable HTTP endpoint at `/mcp` that is compatible with Claude Code and Codex custom connectors. This endpoint implements the standard MCP JSON-RPC 2.0 protocol with method dispatch, ID echoing, notification handling, and SSE support.

**HTTP Transport (Recommended for Gateway Mode)**

To register the g8e Gateway as a custom connector in Claude Code or Codex using HTTP transport:

```bash
# Claude Code
claude mcp add --transport http g8e http://localhost:8080/mcp

# Codex
codex mcp add --transport http g8e http://localhost:8080/mcp
```

Replace `8080` with your configured `--http-port` if different from the default.

The unified `/mcp` endpoint supports:
- **Initialize handshake**: Protocol version negotiation and capability exchange
- **Method dispatch**: All MCP methods (tools/list, tools/call, resources/list, prompts/list, etc.)
- **ID echoing**: Preserves client request IDs for request-response correlation
- **Notification handling**: Accepts notifications (e.g., `notifications/initialized`) with 202 Accepted
- **SSE support**: GET requests for server-sent events streaming
- **Origin validation**: DNS-rebinding protection via Origin header validation (rejects non-loopback origins)

**Note**: The `/mcp` endpoint is available on both gateway surfaces (mTLS port 8443 and plain HTTP port 8080). For Claude Code, use the plain HTTP port (8080) for development or the public TLS port (8443) with JWT authentication for production.

**Stdio Transport (Recommended for Local Development)**

For local development without running the gateway, g8e can run as a stdio MCP server exposing all native tools:

```bash
claude mcp add g8e-stdio g8e mcp stdio
```

This runs g8e in stdio mode with no additional flags required. All 29 native tools are available including system diagnostics, database operations, network tools, and shell execution with governance safety features.

**Governance Proxy for Third-Party MCP Servers**

`g8e mcp agent run` wraps any external MCP server in L1 doctrine as a stdio reverse proxy — no gateway required. Every `tools/call` the AI makes is screened through the MITRE ATT&CK threat detection engine before being forwarded to the real server. Blocked calls return an MCP error with the violation category and MITRE ID; all other methods pass through unchanged.

```bash
# Wrap a subprocess MCP server (stdio)
claude mcp add g8e-fs -- g8e mcp agent run -- npx -y @modelcontextprotocol/server-filesystem /home/user

# Wrap an HTTP MCP server
claude mcp add g8e-proxy g8e mcp agent run --url http://localhost:3000
```

The downstream's `tools/list` is passed through unmodified so the AI sees the real tool's capabilities. Use this mode when you want L1 hard-gate protection around a third-party MCP server without deploying the full g8e stack. For L2-L5 governance (consensus signing, WebAuthn approval), use `g8e mcp stdio` with the gateway running.

### MCP Client Connection

MCP clients connect to the gateway via:

- **JSON-RPC 2.0**: Standard JSON-RPC over HTTP to gateway endpoints
- **Authentication**: mTLS certificate (RequireAndVerifyClientCert) for MCP ingress routes

### Tool Invocation

Invoke MCP tools via JSON-RPC POST to the unified `/mcp` endpoint or via `/api/v1/mcp/tools/call` for direct tool invocation. The `/api/v1/mcp/tools/call/sse` endpoint provides server-sent events for real-time streaming responses.

```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "example_tool",
    "arguments": {
      "param1": "value1",
      "param2": "value2"
    }
  },
  "id": 1
}
```

### Tool Discovery

The gateway provides merged tool discovery that combines native tools with downstream MCP server tools:

- `tools/list`: Returns available tools with schemas (native tools merged with downstream tools when downstream is configured, native tools only when no downstream)
- `prompts/list`: Returns available prompt templates
- `resources/list`: Returns available resources

When a downstream MCP server is configured, the gateway:
1. Proxies the `tools/list` request to the downstream server
2. Parses the downstream response
3. Merges native tools with downstream tools (deduplicating by tool name)
4. Returns the combined tool list to the client

If the downstream server is unavailable (circuit open, connection error, or invalid response), the gateway falls back to returning only native tools. This ensures clients always have access to native tools even when downstream services are degraded.

The unified endpoint uses a functional options pattern for configuration and improved test coverage. Discovery endpoints are proxied from the downstream server when configured. Tool calls are wrapped in GovernanceEnvelope before verification.

### Tool Schema

Tools are defined by the downstream MCP server with schemas that include:

- **name**: Tool identifier
- **description**: Human-readable description
- **inputSchema**: JSON Schema for input validation

The Gateway applies L1 forbidden pattern checks to tool names before forwarding requests.

---

## Security and Verification

### Input Validation Framework

The gateway implements a comprehensive input validation system (`internal/services/mcp/validation.go`) with fail-closed security principles for MCP tool inputs. This framework validates:

- **SQL query validation**: Enforces read-only operations and prevents injection in `db_isolated_read` and `db_query_validate` tools
- **URL validation**: Prevents SSRF attacks in `net_http_probe` by validating URL schemes and destinations
- **Protocol validation**: Prevents path traversal in `net_socket_audit` by validating protocol strings
- **Request forgery protection**: Validates all tool inputs to prevent request forgery attacks

Validation failures are rejected before envelope construction, ensuring malicious inputs never reach the governance pipeline.

### L1 Doctrine (Hard Gates)

- **Forbidden patterns**: Protobuf field options with regex constraints on tool names and URIs
- **Runtime scanning**: Runtime scanning of field values for forbidden patterns (sudo, password, api_key, etc.)
- **Field path validation**: Allowlist/denylist enforcement for `read_field` tool via `FieldPathRegistry`

### L2 Consensus

- **Ed25519 signatures**: Consensus agents sign envelopes with their private keys
- **Signer verification**: Gateway verifies signatures against trusted signers in SQLite store
- **Gateway signing**: In doctrine mode, Gateway signs envelopes locally (single-agent consensus) with `gateway_signed=true` flag
- **Reputation staking**: Signers stake reputation on their decisions

### L3 Notary (Authorization)

- **mTLS fingerprint**: CLI sessions use mTLS certificate fingerprint as proof via CLIL3Notary
- **WebAuthn**: Browser-based clients authenticate with passkeys via PasskeyService
- **Composite verifier**: CompositeL3Verifier handles both web and CLI session types
- **Auto-approval**: Benign diagnostic commands may skip human prompt after L1/L2 pass

### L4 Warden (Pre-Dispatch Verification)

- **Transaction hash verification**: Validates the deterministic transaction hash
- **Expiry checking**: Rejects expired transactions
- **Replay detection**: Prevents transaction replay via nonce tracking
- **State root validation**: Verifies state root consistency
- **L1/L2/L3 verification**: Orchestrates all previous layer verifications

### L5 Actuator (Execution Boundary)

- **Fail-closed dispatch**: Single execution path for verified envelopes
- **Signed ActionReceipts**: Returns signed receipts for all executed mutations
- **Native tool execution**: Executes native tools within Operator's execution boundary
- **Downstream proxy**: Forwards verified calls to downstream MCP servers

---

## Error Handling

MCP protocol errors follow JSON-RPC 2.0 conventions:

- **Parse error**: `-32700` (Parse error)
- **Invalid request**: `-32600` (Invalid Request)
- **Method not found**: `-32601` (Method not found)
- **Invalid params**: `-32602` (Invalid params)
- **Internal error**: `-32603` (Internal error)

### Gateway-Specific Error Codes

The gateway uses reserved error codes in the `-32000` to `-32099` range for governance verification failures:

- **Invalid envelope**: `-32000` (Invalid envelope structure or missing fields)
- **Hash mismatch**: `-32001` (Transaction hash mismatch)
- **Expired**: `-32002` (Transaction expired)
- **Replay**: `-32003` (Transaction replay detected)
- **State mismatch**: `-32004` (State root mismatch)
- **L1 validation failed**: `-32005` (L1 forbidden pattern violation)
- **L2 signature invalid**: `-32006` (L2 signature verification failed)
- **L3 proof invalid**: `-32007` (L3 WebAuthn proof verification failed)
- **Payload decode failed**: `-32008` (Failed to decode protobuf payload)
- **Resource not found**: `-32100` (Requested resource not found)
- **Gateway not ready**: `-32101` (Gateway not ready to process requests)

### Error Mapping

Gateway verification errors are mapped to JSON-RPC error responses:

- **Invalid envelope**: `-32000` (Invalid envelope structure)
- **L1 rejection**: `-32005` (L1 forbidden pattern violation)
- **L2 rejection**: `-32006` (L2 signature verification failed)
- **L3 rejection**: `-32007` (L3 proof verification failed)
- **Downstream unavailable**: `-32603` (Internal error, circuit breaker)

### L3 Suspension

When L3 proof is missing (`ErrL3ProofMissing`), the gateway suspends the transaction and returns an approval URL for out-of-band WebAuthn authorization. The client must retry after approval.

---

## Configuration

### Gateway Modes

The Operator runs in gateway mode with three posture options:

| Mode | Posture | Purpose |
|---|---|---|
| **Doctrine** | `PostureDoctrine` | L1 enforced, L2/L3 audited (default) |
| **Consensus** | `PostureConsensus` | L1/L2 enforced, L3 audited |
| **Notary** | `PostureNotary` | L1/L2/L3 strictly enforced |

### Port Configuration

Default ports (configurable via flags or paths.json):

| Port | Purpose | Auth |
|---|---|---|
| `8080` | HTTP (bootstrap + MCP) | Plain HTTP (no TLS) |
| `8443` | HTTPS (mTLS API + public) | mTLS (RequireAndVerifyClientCert) |

### Configuration

The g8e platform uses **ZERO environment variables** for production configuration. All paths are computed relative to project root, and all configuration is via CLI flags:

- `--data-dir <dir>`: Data directory for SQLite database (default: `.g8e/data` in working directory)
- `--pki-dir <dir>`: Directory for TLS certificates (default: `.g8e/pki`)
- `--secrets-dir <dir>`: Directory for platform secrets (default: `.g8e/secrets`)
- `--http-port <port>`: HTTP port for bootstrap and MCP routes (default: 8080)
- `--https-port <port>`: HTTPS port for mTLS API and public surface (default: 8443)

### Circuit Breaker

The Gateway implements a circuit breaker for downstream MCP servers:

- **Max failures**: 5 consecutive failures before opening circuit
- **Cooldown**: 1 minute before attempting recovery
- **Behavior**: Rejects requests with error when circuit is open

### Incremental State Tracking

The gateway implements incremental state tracking via database schema (`internal/services/gateway/db/schema.sql`) to support efficient state root calculation. This avoids full recomputation of the state Merkle root on each transaction, improving gateway performance for high-throughput scenarios.

---

## Session Management

The Gateway enforces strict session separation via SQLite-backed session store:

| Session Type | Identifier | Authentication | Use Case |
|---|---|---|---|
| **Web Session** | `web_session_id` | WebAuthn (passkey) | Browser-based clients |
| **CLI Session** | `cli_session_id` | mTLS certificate | CLI/BYO clients |
| **Operator Session** | `operator_session_id` | mTLS certificate | In-process execution context |

Sessions are cryptographically bound to their authentication mechanism and cannot be conflated. SSE and pub/sub routing uses these identifiers to prevent cross-tenant data leakage.

---

## Implementation Reference

| Concern | File |
|---|---|
| Governance proxy (agent run) | `internal/cli/cmd/mcp.go` (runMCPAgentRun) |
| Gateway entry | `cmd/operator/main.go` (runGatewayMode) |
| Gateway service | `internal/services/gateway/gateway_service.go` |
| HTTP routing | `internal/services/gateway/gateway_http.go` |
| MCP/A2A translation | `internal/services/mcp/gateway.go` |
| MCP models | `internal/services/mcp/models.go` |
| Unified MCP endpoint | `internal/services/mcp/mcp_endpoint.go` |
| Input validation | `internal/services/mcp/validation.go` |
| Native tool registry | `internal/services/mcp/registry.go` |
| Native tool registration | `internal/services/mcp/native_tool_registry.go` |
| Native tool handlers | `internal/services/mcp/native_handlers.go` |
| Native tool definitions | `internal/services/mcp/*.go` (individual tool files) |
| Field path registry | `internal/services/mcp/field_parser.go` |
| Envelope construction | `internal/services/mcp/gateway.go` (processGatewayTransaction) |
| Transaction verification | `internal/services/governance/l4_warden.go` |
| Envelope processor | `internal/services/governance/processor.go` |
| Pub/Sub command service | `internal/services/pubsub/command_service.go` |
| CLI L3 verification | `internal/services/gateway/cli_l3_notary.go` |
| Composite L3 verifier | `internal/services/gateway/composite_l3_verifier.go` |
| Passkey L3 brokerage | `internal/services/gateway/passkey_service.go` |
| Error mapping | `internal/services/mcp/gateway.go` (mapGatewayError) |
| Suspended transaction store | `internal/services/gateway/gateway_db.go` |
| Gateway database schema | `internal/services/gateway/db/schema.sql` |
| API path constants | `internal/constants/api_paths.go` |
| Port constants | `internal/constants/ports.go` |
| Action type constants | `internal/constants/action_types.go` |
| Protobuf schemas | `protocol/proto/g8e/operator/v1/operator.proto` |
| MCP config generation | `internal/services/mcp/config.go` |
| CLI MCP commands | `internal/cli/cmd/mcp.go` (mcpCmd, agentCmd, mcpStdioCmd) |
| Gateway lifecycle commands | `internal/cli/cmd/gateway.go` (gatewayCmd) |

---

## Adding a New Native Tool

To add a new native tool to the Operator, follow this sequence:

1. **Create tool file**: Copy `docs/protocols/mcp/tool_template.go` to `internal/services/mcp/your_tool_name.go`
2. **Implement interface**: Replace the template with your tool's logic by implementing the `NativeTool` interface methods:
   - `Name() string`: Returns the unique tool identifier (snake_case, lowercase letters, digits, underscores only)
   - `Description() string`: Returns a human-readable description of the tool's purpose
   - `InputSchema() map[string]interface{}`: Returns the JSON Schema for input validation
   - `Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error)`: Implements the tool logic
3. **Add input validation**: If the tool accepts user input, add validation logic in `internal/services/mcp/validation.go` following the fail-closed security principles. The validation framework enforces SQL query validation, URL validation, protocol validation, and request forgery protection.
4. **Register tool**: Add your tool to the tools list in `RegisterNativeTools()` in `internal/services/mcp/native_tool_registry.go`. The registry validates tool name format and input schema at registration time.
5. **Test**: Add unit tests in `internal/services/mcp/native_handlers_test.go` and validation tests in `internal/services/mcp/validation_test.go`. Use table-driven tests for deterministic behavior enumeration.

The tool automatically becomes available via the MCP tools/list endpoint when no downstream MCP server is configured. The registry performs runtime validation of tool names (must be valid identifiers) and input schemas (must have type "object" with valid properties structure).

### Tool Implementation Requirements

All native tools must comply with the following requirements:

- **Tool naming**: Use snake_case format with lowercase letters, digits, and underscores only. The first character must be a letter.
- **Input validation**: Tools that accept user input must include validation logic in `internal/services/mcp/validation.go` to prevent injection attacks, path traversal, and request forgery.
- **Error handling**: Return typed, structured errors with context using `fmt.Errorf("component: action: %w", err)` wrapping.
- **Context cancellation**: Respect context cancellation in long-running operations by checking `ctx.Err()` in loops and blocking operations.
- **Path validation**: Use `security.ValidatePath()` for file path operations to prevent directory traversal attacks.
- **Secret scrubbing**: Apply secret scrubbing to log output and configuration data using the scrubbing functions in `native_handlers.go`.
- **Fail-closed behavior**: Reject invalid inputs before execution rather than attempting to sanitize or proceed with unsafe data.

### Template Reference

The template file at `docs/protocols/mcp/tool_template.go` provides a complete starting point with:

- Proper copyright header and Apache 2.0 license
- Build tags to exclude the template from compilation (`//go:build ignore`)
- Usage instructions in comments
- Complete `NativeTool` interface implementation
- JSON Schema input validation example
- Error handling pattern
- Result marshaling pattern

---

## Related Documentation

- [**g8e Protocol**](../../architecture/protocol.md) - The wire contract and governance hierarchy
- [**g8e Operator (g8eo)**](../../architecture/operator.md) - Operator architecture and gateway mode
- [**A2A Protocol**](../a2a/a2a.md) - A2A protocol specification and integration
