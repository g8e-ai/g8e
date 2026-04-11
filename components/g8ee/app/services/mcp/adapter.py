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

from typing import Any

from pydantic import Field

from app.models.base import VSOBaseModel
from .types import CallToolResult, Content


def build_tool_call_request(tool_name: str, arguments: dict[str, Any], request_id: str) -> dict[str, Any]:
    """Constructs an MCP CallToolRequest JSON-RPC payload."""
    return {
        "jsonrpc": "2.0",
        "id": request_id,
        "method": "tools/call",
        "params": {
            "name": tool_name,
            "arguments": arguments
        }
    }


def parse_tool_call_result(payload: dict[str, Any]) -> CallToolResult:
    """Unwraps an MCP JSON-RPC response into a CallToolResult."""
    # JSON-RPC 2.0 structure
    result_raw = payload.get("result")
    error_raw = payload.get("error")

    if error_raw:
        code = error_raw.get("code", -32603)
        message = error_raw.get("message", "Unknown MCP Error")
        return CallToolResult(
            content=[Content(type="text", text=f"MCP Error ({code}): {message}")],
            isError=True
        )

    if result_raw is None:
        return CallToolResult(content=[], isError=True)

    return CallToolResult.model_validate(result_raw)
