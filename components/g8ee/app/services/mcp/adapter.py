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

from .types import CallToolResult, Content, JSONRPCRequest


def build_tool_call_request(
    tool_name: str,
    execution_id: str,
    arguments: dict[str, Any] | None = None,
) -> JSONRPCRequest:
    """Constructs an MCP CallToolRequest JSON-RPC payload.

    ``execution_id`` is required and is stamped into both the JSON-RPC envelope
    ``id`` and ``params.arguments["execution_id"]`` so the g8eo side can always
    correlate the tool call to its execution. Callers pass tool-specific
    parameters via ``arguments``; any ``execution_id`` in ``arguments`` is
    overwritten to guarantee it matches the envelope id.
    """
    merged_arguments: dict[str, Any] = dict(arguments or {})
    merged_arguments["execution_id"] = execution_id
    return JSONRPCRequest(
        id=execution_id,
        method="tools/call",
        params={
            "name": tool_name,
            "arguments": merged_arguments,
        },
    )


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
            isError=True,
            _metadata=None,
        )

    if result_raw is None:
        return CallToolResult(content=[], isError=True, _metadata=None)

    return CallToolResult.model_validate(result_raw)
