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
from app.services.mcp.types import CallToolResult

def test_build_tool_call_request():
    tool_name = "run_commands_with_operator"
    arguments = {"command": "ls -la"}
    request_id = "cmd-123"
    
    payload = build_tool_call_request(tool_name, arguments, request_id)
    
    assert payload["jsonrpc"] == "2.0"
    assert payload["id"] == request_id
    assert payload["method"] == "tools/call"
    assert payload["params"]["name"] == tool_name
    assert payload["params"]["arguments"] == arguments

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
