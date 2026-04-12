# MCP Client Integration Quickstart

Connect Claude Code, Windsurf, Cursor, or any MCP-compatible AI tool to your g8e instance.

> For deep architecture details, see [mcp.md](mcp.md).

---

## Prerequisites

1. **g8e platform running** -- `./g8e platform start`
2. **A user account** with a G8eKey (API key)
3. **At least one operator bound** to your user (via the dashboard or API key auto-binding)

---

## Quick Setup via CLI

The `./g8e mcp` command generates ready-to-paste configuration for your AI tool.

### Generate Config

```bash
# Claude Code
./g8e mcp config --client claude-code --email you@example.com

# Windsurf
./g8e mcp config --client windsurf --email you@example.com

# Cursor
./g8e mcp config --client cursor --email you@example.com

# Generic MCP client
./g8e mcp config --client generic --email you@example.com
```

Each command outputs a JSON config block you can paste directly into the tool's MCP settings.

### Test Connectivity

```bash
./g8e mcp test --email you@example.com
```

Runs `initialize`, `tools/list`, and `ping` against the MCP endpoint using your G8eKey. Confirms the endpoint is reachable and shows available tools.

### Check Status

```bash
./g8e mcp status
```

Shows endpoint URL, transport type, protocol version, supported methods, and compatible clients.

---

## Manual Configuration

If you prefer to configure manually, the MCP endpoint details are:

| Property | Value |
|----------|-------|
| **Endpoint** | `https://<your-g8e-host>/mcp` |
| **Transport** | Streamable HTTP (`POST`) |
| **Protocol** | MCP 2025-03-26 (JSON-RPC 2.0) |
| **Auth Header** | `x-oauth-client-id: <your-g8e-key>` |

### Claude Code

Add to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "g8e": {
      "type": "streamable-http",
      "url": "https://your-g8e-host/mcp",
      "headers": {
        "x-oauth-client-id": "your-g8e-key-here"
      }
    }
  }
}
```

Or via CLI:

```bash
claude mcp add g8e --transport streamable-http \
  --url "https://your-g8e-host/mcp" \
  --header "x-oauth-client-id: your-g8e-key-here"
```

### Windsurf

Add to your Windsurf MCP configuration:

```json
{
  "mcpServers": {
    "g8e": {
      "serverUrl": "https://your-g8e-host/mcp",
      "headers": {
        "x-oauth-client-id": "your-g8e-key-here"
      }
    }
  }
}
```

### Cursor

Add to your Cursor MCP configuration:

```json
{
  "mcpServers": {
    "g8e": {
      "transport": "streamable-http",
      "url": "https://your-g8e-host/mcp",
      "headers": {
        "x-oauth-client-id": "your-g8e-key-here"
      }
    }
  }
}
```

### Any MCP Client (Generic)

Any client that supports the MCP Streamable HTTP transport can connect:

```json
{
  "mcpServers": {
    "g8e": {
      "transport": {
        "type": "streamable-http",
        "url": "https://your-g8e-host/mcp",
        "headers": {
          "x-oauth-client-id": "your-g8e-key-here"
        }
      }
    }
  }
}
```

---

## Getting Your G8eKey

Your G8eKey is the API key used for MCP authentication. You can find or refresh it via:

**Dashboard:** Navigate to your user profile and click "Refresh G8eKey".

**CLI:**
```bash
# View current user info (includes key status)
./g8e data users get --email you@example.com
```

The `g8e_key` field is your API key. If it shows "not set", refresh it through the dashboard.

---

## Operator Binding

For MCP tool calls that interact with remote systems (commands, file ops, port checks), you need at least one operator **bound** to your user.

When using API key auth (G8eKey), bound operators are resolved **by user ID** -- no active browser session is required. This means:

- Bind an operator in the dashboard once
- The binding persists across sessions
- MCP clients can use operator tools without the dashboard being open

Without a bound operator, only non-operator tools (e.g. `g8e_web_search`) are available.

---

## Available Tools

Once connected, your AI tool has access to these g8e tools:

| Tool | Description |
|------|-------------|
| `run_commands_with_operator` | Execute commands on bound operators |
| `file_create_on_operator` | Create files on bound operators |
| `file_write_on_operator` | Write/overwrite files on bound operators |
| `file_read_on_operator` | Read files from bound operators |
| `file_update_on_operator` | Patch files on bound operators |
| `check_port_status` | Check port reachability from operators |
| `list_files_and_directories_with_detailed_metadata` | List filesystem entries |
| `fetch_file_history` | Get file change history from operator ledger |
| `fetch_file_diff` | Get file diffs from operator ledger |
| `grant_intent_permission` | Grant intent-scoped permissions to operators |
| `revoke_intent_permission` | Revoke intent-scoped permissions |
| `g8e_web_search` | Web search (if configured) |

All tool calls go through the full g8e governance pipeline: Sentinel threat detection, Tribunal command verification, human approval for state-changing operations, and LFAA audit logging.

---

## Governance and Approval

MCP tool calls follow the same security model as the dashboard:

- **Sentinel mode** is enabled by default -- commands are analyzed for risk before execution
- **Human approval** is required for state-changing operations (the user must approve in the dashboard)
- **30-second timeout** -- if approval is pending when the timeout expires, the MCP client receives a graceful error; the approval request remains active server-side
- **Audit logging** -- all MCP tool calls are recorded in the LFAA audit trail

---

## Troubleshooting

### "No tools returned"
Ensure at least one operator is bound to your user in the dashboard.

### "OAuth Client ID authentication failed"
Your G8eKey may be expired or invalid. Refresh it via the dashboard.

### "Tool call timed out"
The operation requires human approval. Check the g8e dashboard for pending approval requests.

### Connection refused / TLS errors
- Ensure the platform is running: `./g8e platform status`
- If using a self-signed CA, the MCP client must trust the platform CA certificate
- Download the CA cert at `https://<host>/ca.crt` or `http://<host>/ca.crt`

### Test with curl

```bash
curl -X POST https://your-g8e-host/mcp \
  -H "x-oauth-client-id: your-g8e-key" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"1","method":"initialize","params":{}}'
```
