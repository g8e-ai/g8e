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

from app.services.mcp.adapter import build_tool_call_request, parse_tool_call_result
from app.services.mcp.types import CallToolResult, JSONRPCRequest

def test_build_tool_call_request():
    tool_name = "run_commands_with_operator"
    arguments = {"command": "ls -la"}
    execution_id = "cmd-123"

    payload = build_tool_call_request(tool_name, execution_id, arguments)

    assert isinstance(payload, JSONRPCRequest)
    assert payload.jsonrpc == "2.0"
    assert payload.id == execution_id
    assert payload.method == "tools/call"
    assert payload.params is not None
    assert payload.params["name"] == tool_name
    # execution_id must be auto-injected into arguments to guarantee correlation
    assert payload.params["arguments"]["execution_id"] == execution_id
    assert payload.params["arguments"]["command"] == "ls -la"


def test_build_tool_call_request_injects_execution_id_when_arguments_none():
    payload = build_tool_call_request("some_tool", "exec-42")

    assert payload.params is not None
    assert payload.params["arguments"] == {"execution_id": "exec-42"}


def test_build_tool_call_request_overrides_stale_execution_id():
    payload = build_tool_call_request(
        "some_tool",
        "exec-correct",
        arguments={"execution_id": "exec-stale", "x": 1},
    )

    assert payload.id == "exec-correct"
    assert payload.params is not None
    assert payload.params["arguments"]["execution_id"] == "exec-correct"
    assert payload.params["arguments"]["x"] == 1

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
    execution_id = "cmd-123"

    mcp_request = build_tool_call_request(tool_name, execution_id, arguments)

    # Create G8eMessage with the JSONRPCRequest as payload
    g8e_msg = G8eMessage(
        id=execution_id,
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
    assert wire_payload["payload"]["id"] == execution_id
    assert wire_payload["payload"]["params"]["name"] == tool_name
    assert wire_payload["payload"]["params"]["arguments"]["execution_id"] == execution_id
    assert wire_payload["payload"]["params"]["arguments"]["command"] == "ls -la"
