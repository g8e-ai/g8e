# Model Context Protocol (MCP) Integration

g8e implements the Model Context Protocol (MCP) as a provider-agnostic translation layer on top of its internal event system. The platform is designed to support multiple protocol translators (MCP, and future protocols) through a unified event-based architecture.

> **Note:** The file `docs/reference/mcp.yaml` is the official MCP Registry API specification, not g8e-specific documentation. This document describes g8e' MCP implementation.

---

## Provider-Agnostic Event System Design

g8e uses an internal event system as the canonical communication layer between components. Protocol-specific translators map external protocol messages to internal event types, enabling the platform to support multiple provider protocols with minimal code changes.

### Architecture Pattern

```
External Protocol (e.g., MCP)
           │
           ▼
    Protocol Translator
    (e.g., mcp/translator.go)
           │
           ▼
    Internal Event Type
    (e.g., operator.command.requested)
           │
           ▼
    g8e Event System
    (g8es pub/sub)
           │
           ▼
    Component Execution
    (g8ee, g8eo, g8ed)
```

**Key Design Principle:** The platform never speaks external protocols directly to its core logic. All external protocols are translated to internal event types at the edge, and core components only understand event types. This enables adding new protocols by implementing a new translator without touching core business logic.

### Current Protocol Support

| Protocol | Translator Location | Status |
|----------|-------------------|--------|
| MCP (Model Context Protocol) | `components/g8eo/services/mcp/translator.go` | ✅ Implemented |
| Future protocols | TBD | 🔜 Extensible |

---

## MCP Implementation Overview

g8e implements MCP in three layers:

1. **g8eo (Go)** - Protocol translation layer on the operator
2. **g8ee (Python)** - Gateway service for external MCP clients
3. **Shared** - Canonical wire format models

### Component Breakdown

#### g8eo - Protocol Translator

**Location:** `components/g8eo/services/mcp/`

| File | Purpose |
|------|---------|
| `translator.go` | Core translation logic: MCP tool names → g8e event types, g8e payloads → MCP JSON-RPC responses |
| `types.go` | Go structs for MCP wire format (JSON-RPC 2.0, Content, Resources) |

**Key Function: `MCPToolNameToEventType()`**

Maps MCP tool names to internal g8e event types:

```go
func MCPToolNameToEventType(toolName string) string {
    switch toolName {
    case "run_commands_with_operator":
        return constants.Event.Operator.Command.Requested
    case "file_create_on_operator", "file_write_on_operator", "file_read_on_operator", "file_update_on_operator":
        return constants.Event.Operator.FileEdit.Requested
    case "check_port_status":
        return constants.Event.Operator.PortCheck.Requested
    case "list_files_and_directories_with_detailed_metadata":
        return constants.Event.Operator.FsList.Requested
    case "fetch_file_history":
        return constants.Event.Operator.FetchFileHistory.Requested
    case "fetch_file_diff":
        return constants.Event.Operator.FetchFileDiff.Requested
    case "grant_intent_permission":
        return constants.Event.Operator.Intent.ApprovalRequested
    default:
        return ""
    }
}
```

**Translation Functions:**

- `TranslateToolCallToCommand()` - Converts MCP `tools/call` JSON-RPC to internal `G8eMessage`
- `TranslateResourceReadToCommand()` - Converts MCP `resources/read` to internal event
- `TranslateResourceListToCommand()` - Converts MCP `resources/list` to internal event
- `TranslateResultToMCP()` - Converts internal g8e payload to MCP JSON-RPC response
- `WrapResult()` - Convenience wrapper for result translation

**Metadata Preservation:**

The translator preserves original payload information in MCP responses via `_metadata`:

```go
type MCPResultMetadata struct {
    OriginalPayload interface{} `json:"original_payload"`
    EventType       string      `json:"event_type"`
    ExecutionID     string      `json:"execution_id"`
}
```

g8ee uses this metadata to reconstruct typed payloads from MCP results (see `_parse_g8eo_payload` in `pubsub_service.py`).

#### g8ee - Gateway Service

**Location:** `components/g8ee/app/services/mcp/`

| File | Purpose |
|------|---------|
| `gateway_service.py` | `MCPGatewayService` - enables external MCP clients (e.g., Claude Code) to execute g8e tools |
| `adapter.py` | Helper functions for building/parsing MCP JSON-RPC payloads |
| `types.py` | Pydantic models for MCP wire format (matches Go types) |

**MCPGatewayService Endpoints:**

| Endpoint | Purpose |
|----------|---------|
| `POST /api/internal/mcp/tools/list` | Returns tool declarations formatted as MCP `tools/list` response |
| `POST /api/internal/mcp/tools/call` | Executes a tool call through `AIToolService.execute_tool()` with full governance |

**Tool Listing:**

```python
def list_tools(self, agent_mode: AgentMode) -> list[dict[str, Any]]:
    tool_groups = self._tool_service.get_tools(agent_mode, model_name="")
    mcp_tools: list[dict[str, Any]] = []
    for group in tool_groups:
        for decl in group.tools:
            mcp_tools.append({
                "name": decl.name,
                "description": decl.description,
                "inputSchema": decl.parameters if isinstance(decl.parameters, dict) else {},
            })
    return mcp_tools
```

**Tool Calling:**

```python
async def call_tool(
    self,
    tool_name: str,
    arguments: dict[str, Any],
    g8e_context: G8eHttpContext,
    user_settings: G8eeUserSettings | None = None,
    sentinel_mode: bool = True,
) -> dict[str, Any]:
    investigation = await self._build_investigation_context(g8e_context, sentinel_mode)
    
    context_token = self._tool_service.start_invocation_context(g8e_context=g8e_context)
    try:
        result = await asyncio.wait_for(
            self._tool_service.execute_tool(
                tool_name=tool_name,
                tool_args=arguments,
                g8e_context=g8e_context,
                investigation=investigation,
                request_settings=user_settings,
            ),
            timeout=MCP_TOOL_CALL_TIMEOUT_SECONDS,
        )
    except (asyncio.TimeoutError, TimeoutError):
        return {
            "content": [{
                "type": "text",
                "text": f"Tool call timed out after {MCP_TOOL_CALL_TIMEOUT_SECONDS}s. "
                        "This operation requires human approval in the g8e dashboard. "
                        "The approval request is still pending and will be processed "
                        "when the user responds.",
            }],
            "isError": True,
        }
    finally:
        self._tool_service.reset_invocation_context(context_token)
    
    return self._tool_result_to_mcp(result)
```

**Timeout Handling:**

MCP tool calls have a 30-second timeout (`MCP_TOOL_CALL_TIMEOUT_SECONDS`). If a tool requires human approval and the user is away, the MCP client receives a graceful error explaining the approval is pending, rather than blocking indefinitely.

#### g8ed - HTTP Gateway

**Location:** `components/g8ed/routes/platform/mcp_routes.js`

**Endpoint:** `POST /mcp`

Dispatch table:

| Method | Behaviour |
|--------|-----------|
| `initialize` | Returns server info: `{ protocolVersion: "2025-03-26", serverInfo: { name: "g8e", version: "4.3.0" } }` |
| `notifications/initialized` | Returns 204 (notification, no body) |
| `ping` | Returns `{}` |
| `tools/list` | Proxies to g8ee `POST /api/internal/mcp/tools/list` |
| `tools/call` | Proxies to g8ee `POST /api/internal/mcp/tools/call` |

**Authentication:** Supports two methods:

1. **Session Token** (standard web authentication)
   - Uses `requireAuth` middleware
   - Caller passes session token as `Authorization: Bearer <token>`
   - Requires an active web session

2. **OAuth Client ID** (for Claude Code connector)
   - Caller passes OAuth Client ID via `x-oauth-client-id` header or `oauth_client_id` query parameter
   - OAuth Client ID is validated as a G8eKey (API key) via `ApiKeyService.validateKey()`
   - Does not require a web session
   - Bound operators are resolved by user ID instead of web session ID

**Context:** `buildG8eContext` resolves bound operators before every request:
- For session authentication: resolves by web session ID via `resolveBoundOperators(webSessionId)`
- For OAuth Client ID authentication: resolves by user ID via `resolveBoundOperatorsForUser(userId)`

If no operators are bound, `tools/list` returns only non-operator tools; `tools/call` for operator tools returns an error.

#### Shared Wire Models

**Location:** `shared/models/wire/mcp.json`

Canonical MCP wire format definitions used by both Python and Go implementations:

- JSON-RPC 2.0 structures (request, response, error)
- Tool call types (CallToolParams, CallToolResult)
- Resource types (ListResources, ReadResource, Resource, ResourceContent)
- Content types (text, image, resource)

**Contract Tests:**

- Python: `components/g8ee/tests/unit/utils/test_shared_mcp_wire.py`
- Go: `components/g8eo/contracts/shared_wire_models_test.go`

These tests verify that both language implementations match the canonical JSON schema.

---

## Event System Integration

MCP events are integrated into the g8e event system via dedicated event types under the `operator.mcp.*` namespace.

### MCP Event Types

**Location:** `shared/constants/events.json` and `components/g8ee/app/constants/events.py`

| Event Type | Purpose |
|------------|---------|
| `g8e.v1.operator.mcp.tools.call` | MCP tool call request from external client |
| `g8e.v1.operator.mcp.tools.result` | MCP tool call result returned to external client |
| `g8e.v1.operator.mcp.resources.list` | MCP resource list request |
| `g8e.v1.operator.mcp.resources.read` | MCP resource read request |
| `g8e.v1.operator.mcp.resources.result` | MCP resource operation result |

### Event Flow

**External MCP Client → g8eo:**

```
Claude Code (MCP client)
    │ POST /mcp
    ▼
g8ed (mcp_routes.js)
    │ POST /api/internal/mcp/tools/call
    ▼
g8ee (MCPGatewayService.call_tool)
    │ AIToolService.execute_tool
    ▼
g8es pub/sub: g8e.v1.operator.mcp.tools.call
    │
    ▼
g8eo (pubsub_commands.go)
    │ handleMCPToolsCall()
    │ mcp.TranslateToolCallToCommand()
    ▼
g8eo executes command (using translated event type)
    │ (dispatches to: Command.Requested, FileEdit.Requested,
    │  FsList.Requested, FsRead.Requested, PortCheck.Requested,
    │  FetchFileHistory.Requested, FetchFileDiff.Requested)
    │
    ▼
g8es pub/sub: g8e.v1.operator.mcp.tools.result
    │
    ▼
g8ee (pubsub_service.py)
    │ _parse_g8eo_payload (handles OPERATOR_MCP_TOOLS_RESULT)
    ▼
g8ee returns result to MCP client
```

---

## Adding New Protocol Translators

The provider-agnostic design enables adding new protocol translators with minimal code changes. To add a new protocol:

### Step 1: Define Protocol-Specific Event Types

Add event types to `shared/constants/events.json` and `components/g8ee/app/constants/events.py`:

```json
"new_protocol": {
  "tools": {
    "call": "operator.new_protocol.tools.call",
    "result": "operator.new_protocol.tools.result"
  }
}
```

### Step 2: Implement Translator in g8eo

Create `components/g8eo/services/new_protocol/translator.go`:

```go
package new_protocol

import (
    "github.com/g8e-ai/g8e/components/g8eo/constants"
    "github.com/g8e-ai/g8e/components/g8eo/models"
)

// NewProtocolToolNameToEventType maps protocol tool names to g8e event types
func NewProtocolToolNameToEventType(toolName string) string {
    switch toolName {
    case "protocol_specific_tool":
        return constants.Event.Operator.Command.Requested
    default:
        return ""
    }
}

// TranslateToolCallToCommand converts protocol request to internal G8eMessage
func TranslateToolCallToCommand(req *ProtocolRequest) (*models.G8eMessage, error) {
    eventType := NewProtocolToolNameToEventType(req.ToolName)
    if eventType == "" {
        return nil, fmt.Errorf("unsupported tool: %s", req.ToolName)
    }
    
    return &models.G8eMessage{
        ID:        req.ID,
        EventType: eventType,
        Payload:   req.Arguments,
    }, nil
}
```

### Step 3: Integrate Translator into PubSub Command Service

Update `components/g8eo/services/pubsub/pubsub_commands.go` to call the new translator when detecting the protocol:

```go
if msg.EventType == constants.Event.Operator.NewProtocolToolsCall {
    g8eMsg, err := new_protocol.TranslateToolCallToCommand(&req)
    if err != nil {
        rs.logger.Error("Failed to translate new protocol tool call", "error", err)
        return
    }
    // Dispatch translated message
}
```

### Step 4: Add Gateway Service in g8ee (Optional)

If the protocol needs external client access, create `components/g8ee/app/services/new_protocol/gateway_service.py` following the MCP pattern.

### Step 5: Add g8ed Routes (Optional)

Create `components/g8ed/routes/platform/new_protocol_routes.js` to expose HTTP endpoints for the protocol.

---

## MCP Wire Format

g8e implements MCP JSON-RPC 2.0 as specified in the [MCP specification](https://modelcontextprotocol.io/docs/concepts/transports#json-rpc-20).

### JSON-RPC 2.0 Structures

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": "request-id",
  "method": "tools/call",
  "params": {
    "name": "run_commands_with_operator",
    "arguments": {
      "command": "ls -la"
    }
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": "request-id",
  "result": {
    "content": [
      {
        "type": "text",
        "text": "file1.txt\nfile2.txt\n"
      }
    ],
    "isError": false,
    "_metadata": {
      "original_payload": { /* internal payload */ },
      "event_type": "g8e.v1.operator.mcp.tools.result",
      "execution_id": "cmd_abc123_1234567890"
    }
  }
}
```

**Error:**
```json
{
  "jsonrpc": "2.0",
  "id": "request-id",
  "error": {
    "code": -32601,
    "message": "Method not found",
    "data": null
  }
}
```

### Content Types

| Type | Fields |
|------|--------|
| `text` | `type`, `text` |
| `image` | `type`, `data` (base64), `mimeType` |
| `resource` | `type`, `resource` (ResourceContent) |

---

## Security Considerations

### Authentication

g8e supports two authentication methods for the MCP endpoint:

1. **Session Token** (standard web authentication)
   - External MCP clients authenticate via session token (`Authorization: Bearer <token>`)
   - Same session token used by the browser works for MCP clients
   - Requires an active web session

2. **OAuth Client ID** (for Claude Code connector)
   - The OAuth Client ID field provided by Claude Code is treated as a G8eKey (API key)
   - Validated via `ApiKeyService.validateKey()` against the existing API key system
   - Enables API key-based authentication without requiring a web session
   - Bound operators are resolved by user ID instead of web session ID
   - API key usage is recorded for audit tracking

**OAuth Client ID Authentication Flow:**

```
Claude Code sends MCP request with x-oauth-client-id header
    ▼
g8ed mcp_routes.js validates OAuth Client ID as a G8eKey
    ▼
ApiKeyService.validateKey() checks key format, expiry, revocation status
    ▼
Extract user_id and organization_id from API key data
    ▼
UserService.getUser() verifies user exists
    ▼
ApiKeyService.recordUsage() tracks API key usage
    ▼
BoundSessionsService.resolveBoundOperatorsForUser() resolves bound operators
    ▼
Request proceeds with authenticated user context
```

**Authentication Methods:**

| Method | Header/Param | Bound Operator Resolution | Requires Web Session |
|--------|--------------|---------------------------|---------------------|
| Session Token | `Authorization: Bearer <token>` | By web session ID | Yes |
| OAuth Client ID | `x-oauth-client-id` header or `oauth_client_id` query param | By user ID | No |

**Rate Limiting:**

- All MCP requests are rate-limited via `apiRateLimiter` (60 requests per minute per IP)
- Rate limiting is applied before authentication to prevent brute force attacks
- OAuth Client ID authentication does not bypass rate limiting

### Governance

All MCP tool calls pass through the full g8e governance pipeline:

1. **Security validation** - Sentinel pre-execution threat detection
2. **Operator binding check** - Only bound operators can be targeted
3. **Risk analysis** - AI-powered command risk classification
4. **Human approval** - State-changing operations require explicit user approval
5. **Audit logging** - LFAA audit events dispatched to operator local vault

### Timeout Protection

MCP tool calls timeout after 30 seconds to prevent indefinite blocking when awaiting human approval. The approval request remains active server-side; the MCP client receives a graceful error and can retry.

---

## CLI Integration (`./g8e mcp`)

The `./g8e mcp` CLI commands simplify MCP client setup by auto-detecting the platform URL and resolving the user's G8eKey.

| Command | Description |
|---------|-------------|
| `./g8e mcp config --client <name> --email <email>` | Generate ready-to-paste MCP config for a specific client |
| `./g8e mcp test --email <email>` | Test MCP endpoint connectivity (initialize + tools/list + ping) |
| `./g8e mcp status` | Show endpoint URL, transport, protocol, and supported clients |

Supported `--client` values: `claude-code`, `windsurf`, `cursor`, `generic`.

**Implementation:** `scripts/data/manage-mcp.py`, dispatched via `manage-g8es.py mcp`.

> For a step-by-step setup guide, see [mcp-quickstart.md](mcp-quickstart.md).

---

## Usage Examples

### Claude Code Integration with OAuth Client ID

To configure Claude Code to connect to g8e via the MCP endpoint:

1. **Generate a G8eKey (API Key)**
   - Use the g8e UI to create an API key for your user, or run `./g8e mcp config --client claude-code --email you@example.com` to generate the config automatically
   - The API key will be used as the OAuth Client ID in Claude Code

2. **Configure Claude Code MCP Server**
   ```json
   {
     "mcpServers": {
       "g8e": {
         "transport": {
           "type": "streamable-http",
           "url": "https://your-g8e-instance.com/mcp",
           "headers": [
             {
               "name": "x-oauth-client-id",
               "value": "your-dropkey-api-key-here"
             }
           ]
         }
       }
     }
   }
   ```

3. **Alternative: Query Parameter**
   ```json
   {
     "mcpServers": {
       "g8e": {
         "transport": {
           "type": "streamable-http",
           "url": "https://your-g8e-instance.com/mcp?oauth_client_id=your-dropkey-api-key-here"
         }
       }
     }
   }
   ```

### Session Token Authentication (Alternative)

For testing or when a web session is available, you can use session token authentication:

```bash
curl -X POST https://your-g8e-instance.com/mcp \
  -H "Authorization: Bearer <your-session-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
  }'
```

### Testing the MCP Endpoint

```bash
# Using OAuth Client ID
curl -X POST https://your-g8e-instance.com/mcp \
  -H "x-oauth-client-id: your-dropkey-api-key-here" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {}
  }'

# Using session token
curl -X POST https://your-g8e-instance.com/mcp \
  -H "Authorization: Bearer your-session-token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
  }'
```

---

## Testing

### Unit Tests

**g8eo (Go):**
- `components/g8eo/services/mcp/translator_test.go` - Translation logic tests

**g8ee (Python):**
- `components/g8ee/tests/unit/services/mcp/test_gateway_service.py` - Gateway service tests
- `components/g8ee/tests/unit/services/mcp/test_adapter.py` - Adapter function tests
- `components/g8ee/tests/unit/services/mcp/test_types.py` - Type model tests

**g8ed (JavaScript):**
- `components/g8ed/test/unit/routes/platform/mcp_routes.unit.test.js` - Route handler tests

### Contract Tests

**Shared Wire Models:**
- Python: `components/g8ee/tests/unit/utils/test_shared_mcp_wire.py`
- Go: `components/g8eo/contracts/shared_wire_models_test.go`

These verify that Python and Go implementations match the canonical `shared/models/wire/mcp.json` schema.

---

## References

- [MCP Specification](https://modelcontextprotocol.io)
- [MCP JSON-RPC Transport](https://modelcontextprotocol.io/docs/concepts/transports#json-rpc-20)
- [g8ee Component Documentation](../components/g8ee.md)
- [g8eo Component Documentation](../components/g8eo.md)
- [g8ed Component Documentation](../components/g8ed.md)
