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

from typing import Any, Optional

from pydantic import Field

from app.models.base import G8eBaseModel


# MCP JSON-RPC 2.0 wire types
# See: https://modelcontextprotocol.io/docs/concepts/transports#json-rpc-20

class JSONRPCError(G8eBaseModel):
    code: int
    message: str
    data: Any | None = None


class JSONRPCRequest(G8eBaseModel):
    jsonrpc: str = "2.0"
    id: str
    method: str
    params: dict[str, Any] | None = None


class JSONRPCResponse(G8eBaseModel):
    jsonrpc: str = "2.0"
    id: str
    result: Optional[object] = None
    error: Optional[JSONRPCError] = None


# Content and Resource types

class ResourceContent(G8eBaseModel):
    uri: str
    mimeType: str | None = None
    text: str | None = None
    blob: str | None = None  # Base64 encoded


class Content(G8eBaseModel):
    type: str  # text, image, resource
    text: str | None = None
    data: str | None = None  # Base64 for images
    mimeType: str | None = None
    resource: ResourceContent | None = None


# Tool call types

class CallToolParams(G8eBaseModel):
    name: str
    arguments: dict[str, Any] | None = None


class CallToolResult(G8eBaseModel):
    content: list[Content]
    isError: bool = False
    metadata: dict[str, Any] | None = Field(None, alias="_metadata")

    @property
    def execution_id(self) -> Optional[str]:
        if self.metadata:
            return self.metadata.get("execution_id")
        return None


# Resource types

class Resource(G8eBaseModel):
    uri: str
    name: str
    description: Optional[str] = None
    mimeType: Optional[str] = None


class ListResourcesParams(G8eBaseModel):
    cursor: str | None = None


class ListResourcesResult(G8eBaseModel):
    resources: list[Resource]
    nextCursor: str | None = None


class ReadResourceParams(G8eBaseModel):
    uri: str


class ReadResourceResult(G8eBaseModel):
    contents: list[ResourceContent]
