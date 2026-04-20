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

from app.services.mcp.types import JSONRPCRequest, JSONRPCResponse, JSONRPCError, CallToolResult, Content

def test_jsonrpc_request_validation():
    # Valid request
    req = JSONRPCRequest(
        id="123",
        method="tools/call",
        params={"name": "test-tool", "arguments": {"foo": "bar"}}
    )
    assert req.jsonrpc == "2.0"
    assert req.id == "123"
    assert req.method == "tools/call"
    assert req.params["name"] == "test-tool"

def test_jsonrpc_response_validation():
    # Valid result response
    res = JSONRPCResponse(
        id="123",
        result={"content": [{"type": "text", "text": "hello"}]}
    )
    assert res.jsonrpc == "2.0"
    assert res.id == "123"
    assert res.result["content"][0]["text"] == "hello"
    assert res.error is None

    # Valid error response
    res_err = JSONRPCResponse(
        id="123",
        error=JSONRPCError(code=-32601, message="Method not found")
    )
    assert res_err.error.code == -32601
    assert res_err.result is None

def test_call_tool_result_validation():
    result = CallToolResult(
        content=[Content(type="text", text="output")],
        isError=False
    )
    assert len(result.content) == 1
    assert result.content[0].type == "text"
    assert result.content[0].text == "output"
    assert not result.isError
