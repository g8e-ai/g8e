# SSE Event Fixtures

This directory contains shared SSE event fixture definitions used across g8e components for contract testing.

## Event Architecture

### SSE-Delivered Events (agent_sse.py)

The SSE delivery mechanism in `components/g8ee/app/services/ai/agent_sse.py` emits the following events:

- **LLM Chat Iteration Events**: Emits generic events for all AI chat iterations
  - `LLM_CHAT_ITERATION_STARTED` - Signals AI processing has begun
  - `LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED` - Streaming text chunks
  - `LLM_CHAT_ITERATION_TEXT_COMPLETED` - Final response text with metadata
  - `LLM_CHAT_ITERATION_FAILED` - Error during AI processing
  - `LLM_CHAT_ITERATION_CITATIONS_RECEIVED` - Grounding/citation metadata
  - `LLM_CHAT_ITERATION_THINKING_STARTED` - AI thinking phases
  - `LLM_CHAT_ITERATION_RETRY` - Retry attempts
  - `LLM_CHAT_ITERATION_TOOL_CALL_STARTED` - Generic tool call indicator (all tools)
  - `LLM_CHAT_ITERATION_TOOL_CALL_COMPLETED` - Generic tool completion (all tools)
  - `LLM_CHAT_ITERATION_COMPLETED` - End of a tool iteration

**Important**: Per-tool REQUESTED events (e.g., `g8e_web_search_requested`, `port_check_requested`, `operator_command_requested`) are **NOT emitted during SSE delivery**. The generic `LLM_CHAT_ITERATION_TOOL_CALL_STARTED` event carries display metadata for all tools and owns the UI indicator lifecycle. See `agent_sse.py` lines 202-206.

### Tool Service Events

Per-tool REQUESTED/STARTED/COMPLETED/FAILED events are emitted by individual tool services during execution:

- **Web Search Tool** (tool_service.py):
  - `LLM_TOOL_G8E_WEB_SEARCH_REQUESTED` - Emitted by tool service
  - `LLM_TOOL_G8E_WEB_SEARCH_COMPLETED` - Emitted by tool service
  - `LLM_TOOL_G8E_WEB_SEARCH_FAILED` - Emitted by tool service

- **Port Check Tool** (port_service.py):
  - `OPERATOR_NETWORK_PORT_CHECK_REQUESTED` - Emitted by port_service
  - `OPERATOR_NETWORK_PORT_CHECK_COMPLETED` - Emitted by port_service
  - `OPERATOR_NETWORK_PORT_CHECK_FAILED` - Emitted by port_service

- **Operator Command Tool** (operator_command_service.py):
  - `OPERATOR_COMMAND_REQUESTED` - Emitted by operator_command_service
  - `OPERATOR_COMMAND_STARTED` - Emitted by operator_command_service
  - `OPERATOR_COMMAND_COMPLETED` - Emitted by operator_command_service
  - `OPERATOR_COMMAND_FAILED` - Emitted by operator_command_service

### Platform Events

- `PLATFORM_SSE_CONNECTION_ESTABLISHED` - SSE connection established
- `PLATFORM_SSE_KEEPALIVE_SENT` - SSE keepalive heartbeat

### LLM Lifecycle Events

- `LLM_LIFECYCLE_STARTED` - LLM processing started
- `LLM_LIFECYCLE_COMPLETED` - LLM processing completed

### Tribunal Events

- Tribunal-specific events for multi-model verification workflows

## Fixture Files

- `sse-events.json` - Event payload fixtures for contract testing
- `sse-events-schema.json` - JSON schema for event validation

## Testing Guidelines

When writing SSE contract tests:
1. Only test events that are actually emitted by SSE delivery
2. Do not expect per-tool REQUESTED events in SSE delivery tests
3. Test generic tool events (`LLM_CHAT_ITERATION_TOOL_CALL_STARTED/COMPLETED`) instead
4. For per-tool events, write integration tests against the specific tool service

See `components/g8ee/tests/integration/test_sse_event_contract_integration.py` for examples.
