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

from pydantic import Field

from app.models.attachments import AttachmentMetadata
from app.models.base import VSOBaseModel
from app.models.cases import CaseModel
from app.models.operators import PendingApproval


class ChatMessageRequest(VSOBaseModel):
    """Request model for chat messages -- user-provided content only.

    Identity and business context (case_id, investigation_id, web_session_id,
    user_id) come exclusively from VSOHttpContext headers set by VSOD.
    The request body carries only user-controlled content.

    Whether to create a new case+investigation is derived from vso_context.case_id
    being empty — no flag needed in the body.
    """
    message: str = Field(..., description="Chat message content")
    attachments: list[AttachmentMetadata] | None = Field(default_factory=list, description="File attachments")
    sentinel_mode: bool = Field(default=True, description="Sentinel mode - when True, data is scrubbed before storage and AI sees redacted data")
    llm_primary_model: str | None = Field(default=None, description="Primary LLM model override for complex tasks - null uses server default")
    llm_assistant_model: str | None = Field(default=None, description="Assistant LLM model override for simple tasks - null uses server default")


class ChatStartedResponse(VSOBaseModel):
    """Response for POST /chat — returns the case and investigation IDs created or resolved."""
    success: bool
    case_id: str
    investigation_id: str


class StopAIResponse(VSOBaseModel):
    """Response for POST /chat/stop."""
    success: bool
    investigation_id: str
    was_active: bool


class ApprovalRespondedResponse(VSOBaseModel):
    """Response for POST /operator/approval/respond."""
    success: bool
    approval_id: str
    approved: bool


class DirectCommandSentResponse(VSOBaseModel):
    """Response for POST /operator/direct-command."""
    success: bool
    execution_id: str


class CaseResponse(VSOBaseModel):
    """Response for GET/PATCH /cases/{case_id}."""
    success: bool
    case: CaseModel


class OperatorStoppedResponse(VSOBaseModel):
    """Response for POST /operators/stop."""
    success: bool
    operator_id: str
    subscribers: int


class OperatorSessionRegisteredResponse(VSOBaseModel):
    """Response for POST /operators/register-operator-session and deregister-operator-session."""
    success: bool
    operator_id: str
    operator_session_id: str


class OperatorApprovalResponse(VSOBaseModel):
    """Request model for operator command approval response.

    Identity/business context (case_id, investigation_id, etc.) comes from
    VSOHttpContext headers. Only approval-specific fields are in the body.
    The router enriches this with operator_session_id / operator_id from
    the bound operator before passing it to the approval service.
    """
    approval_id: str = Field(..., description="Approval ID")
    approved: bool = Field(..., description="Whether the command was approved")
    reason: str = Field(default="Approval denied by user", description="Reason for denial if not approved")
    operator_session_id: str = Field(default="", description="Operator session ID (set by router from bound operator)")
    operator_id: str = Field(default="", description="Operator ID (set by router from bound operator)")


class PendingApprovalsResponse(VSOBaseModel):
    """Response for GET /operator/approval/pending.

    Returns all pending approvals currently waiting for user response.
    """
    pending_approvals: dict[str, PendingApproval] = Field(default_factory=dict, description="Pending approvals keyed by approval_id")


class StopAIRequest(VSOBaseModel):
    """Request model for stopping active AI processing."""
    investigation_id: str = Field(..., description="Investigation ID to stop processing for")
    reason: str = Field(default="User requested stop", description="Reason for stopping")
    web_session_id: str | None = Field(default=None, description="Web session ID for SSE routing")


class StopOperatorRequest(VSOBaseModel):
    """Request model for stopping an operator via pub/sub shutdown command."""
    operator_id: str = Field(..., description="Operator ID")
    operator_session_id: str = Field(..., description="Operator session ID")
    user_id: str | None = Field(default=None, description="User ID")


class OperatorSessionRegistrationRequest(VSOBaseModel):
    """Request model for registering or deregistering an operator session heartbeat subscription.

    Called by VSOD when an operator authenticates (register) or goes offline/stops (deregister).
    Triggers g8ee to subscribe or unsubscribe from the heartbeat pub/sub channel for this session.
    """
    operator_id: str = Field(..., description="Operator ID")
    operator_session_id: str = Field(..., description="Operator session ID")


class MCPToolCallRequest(VSOBaseModel):
    """Request model for MCP tool/call via the gateway.

    The external MCP client sends a JSON-RPC tools/call; VSOD unwraps it and
    forwards tool_name + arguments here. Identity comes from VSOHttpContext headers.
    """
    tool_name: str = Field(..., description="MCP tool name (e.g. run_commands_with_operator)")
    arguments: dict = Field(default_factory=dict, description="Tool arguments")
    request_id: str = Field(..., description="JSON-RPC request id for correlation")
    sentinel_mode: bool = Field(default=True, description="Sentinel mode - when True, data is scrubbed before storage and AI sees redacted data")


class MCPToolListResponse(VSOBaseModel):
    """Response for POST /mcp/tools/list."""
    tools: list = Field(default_factory=list, description="MCP tool definitions")


class MCPToolCallResponse(VSOBaseModel):
    """Response for POST /mcp/tools/call -- wraps the result as MCP JSON-RPC."""
    jsonrpc: str = Field(default="2.0")
    id: str = Field(..., description="Correlated JSON-RPC request id")
    result: dict | None = Field(default=None, description="MCP CallToolResult on success")
    error: dict | None = Field(default=None, description="JSON-RPC error on failure")


class DirectCommandRequest(VSOBaseModel):
    """Request model for direct command execution (bypasses AI).

    Body carries only command-specific payload. All identity and context
    (user_id, web_session_id, operator_id, operator_session_id, case_id,
    investigation_id) come from VSOHttpContext headers.
    """
    command: str = Field(..., description="Command to execute on operator")
    execution_id: str = Field(..., description="Execution ID for tracking")
    hostname: str | None = Field(default=None, description="Hostname of the target operator for result display")
    source: str = Field(default="anchored_terminal", description="Source of the command")
