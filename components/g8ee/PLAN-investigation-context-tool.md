# Plan: `query_investigation_context` Tool

## Objective

Add a first-class AI tool that lets any agent query investigation data on-demand from InvestigationService (backed by CacheAside), instead of relying solely on pre-loaded conversation history. The tool requires no operator approval and is available to all agents in all modes.

## Design Decisions

- **Single tool with `data_type` parameter** rather than N separate tools. Keeps the tool surface small while giving the AI flexible access to different data domains. The AI selects what it needs via `data_type`.
- **No approval required** -- this is a read-only local data query, not an operator action.
- **Available in ALL agent modes** (OPERATOR_BOUND, OPERATOR_NOT_BOUND, CLOUD_OPERATOR_BOUND) since it queries g8ee's own data, not operator data.
- **Leverages existing InvestigationService methods** directly -- no new data access code needed.
- **Uses the investigation already passed into `execute_tool_call`** for the current investigation's ID, so no new context propagation needed.

## Data Types Exposed

| `data_type` value | InvestigationService method | Returns |
|---|---|---|
| `conversation_history` | `get_chat_messages(investigation_id)` | Full conversation history (messages, senders, timestamps) |
| `investigation_status` | `get_investigation(investigation_id)` | Investigation metadata: status, priority, severity, case title/description, customer/technical context |
| `history_trail` | `get_investigation(investigation_id)` | Event trail: command executions, file edits, status changes |
| `operator_actions` | `get_operator_actions_for_ai_context(investigation_id)` | Formatted operator action history |

## Files to Modify

### 1. `app/constants/status.py` -- Add enum member
```python
class OperatorToolName(str, Enum):
    ...
    QUERY_INVESTIGATION_CONTEXT = "query_investigation_context"
```

### 2. `app/constants/prompts.py` -- Add prompt file reference
```python
class PromptFile(str, Enum):
    ...
    TOOL_QUERY_INVESTIGATION_CONTEXT = "tools/query_investigation_context.txt"
```

### 3. `app/prompts_data/tools/query_investigation_context.txt` -- New prompt file
Describes the tool, its parameters, and when to use each `data_type`.

### 4. `app/models/command_payloads.py` -- Add args model
```python
class QueryInvestigationContextArgs(G8eBaseModel):
    data_type: str = Field(..., description="Type of data to retrieve: conversation_history, investigation_status, history_trail, operator_actions")
    limit: int | None = Field(default=None, description="Max number of items to return (for conversation_history and history_trail)")
```

### 5. `app/models/tool_results.py` -- Add result model + update Union
```python
class InvestigationContextResult(G8eBaseModel):
    success: bool = True
    data_type: str
    data: dict[str, Any] | list[dict[str, Any]] | str
    item_count: int | None = None
    investigation_id: str | None = None

ToolResult = Union[..., InvestigationContextResult]
```

### 6. `app/services/ai/tool_service.py` -- Register tool + add execution logic
- Add `_build_query_investigation_context_tool()` method
- Register in `__init__`
- Add `_handle_query_investigation_context()` method (follows uniform handler signature: `(self, tool_args, investigation, g8e_context, request_settings) -> ToolResult`)
- Add entry to `_build_tool_handlers()` dispatch table: `OperatorToolName.QUERY_INVESTIGATION_CONTEXT: self._handle_query_investigation_context`
- Add to `get_tools` for ALL modes (new "universal tools" list that's always included)
- **IMPORTANT**: The new tool does not require binding, therefore must NOT be in `OPERATOR_TOOLS`, since `OPERATOR_TOOLS` is currently auto-generated from `OperatorToolName`, change it to an explicit list of operator-requiring tools.

### 7. `app/services/ai/agent_tool_loop.py` -- Add display metadata
```python
_TOOL_DISPLAY_METADATA[OperatorToolName.QUERY_INVESTIGATION_CONTEXT] = (
    "Querying investigation", "database", ToolDisplayCategory.GENERAL
)
```

### 8. `shared/constants/agents.json` -- Add tool to primary and assistant agent tool lists

### 9. `docs/components/g8ee.md` -- Update tool inventory if relevant section exists

## Execution Flow

1. AI calls `query_investigation_context(data_type="conversation_history", limit=50)`
2. `orchestrate_tool_execution` dispatches to `tool_executor.execute_tool_call`
3. `execute_tool_call` checks `OPERATOR_TOOLS` for operator binding requirement (new tool is excluded, so no check)
4. `execute_tool_call` looks up handler in `_tool_handlers` dispatch table
5. `_handle_query_investigation_context` validates args and calls the appropriate InvestigationService method
6. Returns `InvestigationContextResult` which gets `flatten_for_llm()` back to the model

## Code Smell Refactors Applied (Pre-Implementation)

The codebase was refactored to address technical debt, which simplifies the new tool implementation:

1. **Dispatch table in `tool_service.py`**: The `execute_tool_call` method now uses a `_tool_handlers` dict (built by `_build_tool_handlers()`) instead of a long if/elif chain. Adding the new tool only requires one line in the dispatch table.
2. **Uniform handler methods**: Each tool has a `_handle_*` method with a consistent signature `(self, tool_args, investigation, g8e_context, request_settings) -> ToolResult`. The new tool follows this pattern.
3. **Tribunal error helper**: The 4 near-identical Tribunal exception handlers in `agent_tool_loop.py` were consolidated into `_tribunal_error_result(tool_name, original_command, error_msg)`.
4. **`OPERATOR_TOOLS` constant**: The operator tools set is now a single `frozenset` constant in `status.py` imported by both files, eliminating duplication.

**Impact on this implementation**: The new tool only needs:
- One entry in `_build_tool_handlers()` dispatch table
- One `_handle_query_investigation_context()` method
- No changes to `execute_tool_call` (dispatch is automatic)

**Open issue**: `OPERATOR_TOOLS` is currently auto-generated from all `OperatorToolName` members. Since `query_investigation_context` is NOT an operator tool, we must change `OPERATOR_TOOLS` to an explicit list of operator-requiring tools.

## What This Does NOT Change

- The existing `build_contents_from_history` flow in `chat_pipeline.py` remains intact (the tool supplements, not replaces)
- No new CacheAside or DB code needed
- No changes to the Tribunal pipeline
- No approval flow integration needed
