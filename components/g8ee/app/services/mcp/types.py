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

"""
MCP service types.

This module re-exports MCP data models from app.models.mcp for backward compatibility.
The actual model definitions live in the models layer to avoid circular dependencies.
"""

# Re-export all MCP types from the models layer
from app.models.mcp import (
    JSONRPCError,
    JSONRPCRequest,
    JSONRPCResponse,
    ResourceContent,
    Content,
    CallToolParams,
    CallToolResult,
    Resource,
    ListResourcesParams,
    ListResourcesResult,
    ReadResourceParams,
    ReadResourceResult,
)

__all__ = [
    "JSONRPCError",
    "JSONRPCRequest",
    "JSONRPCResponse",
    "ResourceContent",
    "Content",
    "CallToolParams",
    "CallToolResult",
    "Resource",
    "ListResourcesParams",
    "ListResourcesResult",
    "ReadResourceParams",
    "ReadResourceResult",
]
