# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import pytest
from app.services.mcp.adapter import build_tool_call_request, parse_tool_call_result
from app.services.mcp.types import CallToolResult, Content, JSONRPCRequest

def test_build_tool_call_request():
    tool_name = "run_commands_with_operator"
    arguments = {"command": "ls -la"}
    request_id = "cmd-123"
    
    payload = build_tool_call_request(tool_name, arguments, request_id)
    
    assert isinstance(payload, JSONRPCRequest)
    assert payload.jsonrpc == "2.0"
    assert payload.id == request_id
    assert payload.method == "tools/call"
    assert payload.params["name"] == tool_name
    assert payload.params["arguments"] == arguments

def test_parse_tool_call_result_success():
    payload = {
        "jsonrpc": "2.0",
        "id": "cmd-123",
        "result": {
            "content": [{"type": "text", "text": "success output"}],
            "isError": False
        }
    }
    
    result = parse_tool_call_result(payload)
    
    assert isinstance(result, CallToolResult)
    assert not result.isError
    assert len(result.content) == 1
    assert result.content[0].text == "success output"

def test_parse_tool_call_result_error():
    payload = {
        "jsonrpc": "2.0",
        "id": "cmd-123",
        "error": {
            "code": -32601,
            "message": "Method not found"
        }
    }
    
    result = parse_tool_call_result(payload)
    
    assert isinstance(result, CallToolResult)
    assert result.isError
    assert "MCP Error (-32601): Method not found" in result.content[0].text

def test_build_tool_call_request_serialization():
    """Verify JSONRPCRequest serializes correctly with method field preserved.
    
    Regression test for: method field was empty when dict was used instead of model,
    causing g8ep to fail with "method not found" error during MCP tool call translation.
    """
    from app.models.pubsub_messages import G8eMessage
    from app.constants import ComponentName, EventType
    
    tool_name = "run_commands_with_operator"
    arguments = {"command": "ls -la"}
    request_id = "cmd-123"
    
    mcp_request = build_tool_call_request(tool_name, arguments, request_id)
    
    # Create G8eMessage with the JSONRPCRequest as payload
    g8e_msg = G8eMessage(
        id=request_id,
        source_component=ComponentName.G8EE,
        event_type=EventType.OPERATOR_MCP_TOOLS_CALL,
        case_id="test-case",
        task_id="test-task",
        investigation_id="test-investigation",
        web_session_id="test-session",
        operator_session_id="test-operator-session",
        operator_id="test-operator",
        payload=mcp_request
    )
    
    # Serialize to wire format (what gets sent to g8ep)
    wire_payload = g8e_msg.model_dump(mode="json")

    # Verify the method field is preserved in the serialized payload
    assert "payload" in wire_payload
    assert wire_payload["payload"]["method"] == "tools/call"
    assert wire_payload["payload"]["id"] == request_id
    assert wire_payload["payload"]["params"]["name"] == tool_name
    assert wire_payload["payload"]["params"]["arguments"] == arguments
