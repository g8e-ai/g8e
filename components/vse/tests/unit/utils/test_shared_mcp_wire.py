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

"""Contract tests for MCP wire protocol compatibility."""

import json
from pathlib import Path
from typing import Any, Dict

import pytest
from app.constants.paths import PATHS
from app.services.mcp.types import (
    CallToolParams,
    CallToolResult,
    Content,
    JSONRPCError,
    JSONRPCRequest,
    JSONRPCResponse,
    Resource,
    ResourceContent,
)

# Resolve path to shared constants
SHARED_MODELS_DIR = Path(PATHS["infra"]["shared_models_dir"]) / "wire"
MCP_JSON_PATH = SHARED_MODELS_DIR / "mcp.json"


@pytest.fixture
def mcp_schema() -> Dict[str, Any]:
    """Load canonical MCP wire schema."""
    assert MCP_JSON_PATH.exists(), f"Schema file not found at {MCP_JSON_PATH}"
    with open(MCP_JSON_PATH, "r") as f:
        return json.load(f)


def test_mcp_json_exists():
    """Verify shared mcp.json exists."""
    assert MCP_JSON_PATH.exists()


def test_jsonrpc_request_schema(mcp_schema):
    """Verify JSONRPCRequest matches schema."""
    fields = mcp_schema["jsonrpc"]["request"]["fields"]
    model_fields = JSONRPCRequest.model_fields
    
    assert "jsonrpc" in model_fields
    assert "id" in model_fields
    assert "method" in model_fields
    assert "params" in model_fields
    
    # Required checks - fields with defaults are not strictly 'required' by Pydantic
    assert model_fields["id"].is_required() == fields["id"]["required"]
    assert model_fields["method"].is_required() == fields["method"]["required"]
    
    # Const/Default check for jsonrpc
    assert model_fields["jsonrpc"].default == fields["jsonrpc"]["const"]


def test_call_tool_result_schema(mcp_schema):
    """Verify CallToolResult matches schema including g8e extension."""
    fields = mcp_schema["tools"]["call"]["result"]["fields"]
    model_fields = CallToolResult.model_fields
    
    assert "content" in model_fields
    assert "isError" in model_fields
    assert "metadata" in model_fields
    
    # Metadata alias check
    assert model_fields["metadata"].alias == "_metadata"
    
    # Type checks
    assert model_fields["isError"].annotation == bool


def test_content_schema(mcp_schema):
    """Verify Content model matches schema."""
    fields = mcp_schema["types"]["Content"]["fields"]
    model_fields = Content.model_fields
    
    assert "type" in model_fields
    assert "text" in model_fields
    assert "data" in model_fields
    assert "mimeType" in model_fields
    assert "resource" in model_fields
    
    # enum check for type
    assert fields["type"]["enum"] == ["text", "image", "resource"]


def test_resource_content_schema(mcp_schema):
    """Verify ResourceContent model matches schema."""
    fields = mcp_schema["types"]["ResourceContent"]["fields"]
    model_fields = ResourceContent.model_fields
    
    assert "uri" in model_fields
    assert "mimeType" in model_fields
    assert "text" in model_fields
    assert "blob" in model_fields
