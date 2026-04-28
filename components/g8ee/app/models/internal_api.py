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

from pydantic import Field, ConfigDict

from app.models.attachments import AttachmentMetadata
from app.models.base import G8eBaseModel
from app.models.cases import CaseModel
from app.models.operators import PendingApproval


class ChatMessageRequest(G8eBaseModel):
    """Request model for chat messages -- user-provided content only.

    Identity and business context (case_id, investigation_id, web_session_id,
    user_id) come exclusively from G8eHttpContext headers set by g8ed.
    The request body carries only user-controlled content.

    Whether to create a new case+investigation is derived from g8e_context.case_id
    being empty — no flag needed in the body.
    """
    message: str = Field(..., description="Chat message content")
    attachments: list[AttachmentMetadata] | None = Field(default_factory=list, description="File attachments")
    sentinel_mode: bool = Field(default=True, description="Sentinel mode - when True, data is scrubbed before storage and AI sees redacted data")
    llm_primary_provider: str | None = Field(default=None, description="Primary LLM provider override for complex tasks - null uses server default")
    llm_assistant_provider: str | None = Field(default=None, description="Assistant LLM provider override for simple tasks - null uses server default")
    llm_lite_provider: str | None = Field(default=None, description="Lite LLM provider override for quick tasks - null uses server default")
    llm_primary_model: str | None = Field(default=None, description="Primary LLM model override for complex tasks - null uses server default")
    llm_assistant_model: str | None = Field(default=None, description="Assistant LLM model override for simple tasks - null uses server default")
    llm_lite_model: str | None = Field(default=None, description="Lite LLM model override for quick tasks - null uses server default")


class ChatStartedResponse(G8eBaseModel):
    """Response for POST /chat — returns the case and investigation IDs created or resolved."""
    success: bool
    case_id: str
    investigation_id: str


class StopAIResponse(G8eBaseModel):
    """Response for POST /chat/stop."""
    success: bool
    investigation_id: str
    was_active: bool


class ApprovalRespondedResponse(G8eBaseModel):
    """Response for POST /operator/approval/respond."""
    success: bool
    approval_id: str
    approved: bool


class DirectCommandSentResponse(G8eBaseModel):
    """Response for POST /operator/direct-command."""
    success: bool
    execution_id: str


class CaseResponse(G8eBaseModel):
    """Response for GET/PATCH /cases/{case_id}."""
    success: bool
    case: CaseModel


class OperatorStoppedResponse(G8eBaseModel):
    """Response for POST /operators/stop."""
    success: bool
    operator_id: str
    subscribers: int


class OperatorSessionRegisteredResponse(G8eBaseModel):
    """Response for POST /operators/register-operator-session and deregister-operator-session."""
    success: bool
    operator_id: str
    operator_session_id: str


class OperatorApprovalResponse(G8eBaseModel):
    """Request model for operator command approval response.

    Identity/business context (case_id, investigation_id, etc.) comes from
    G8eHttpContext headers. Only approval-specific fields are in the body.
    The router enriches this with operator_session_id / operator_id from
    the bound operator before passing it to the approval service.
    """
    approval_id: str = Field(..., description="Approval ID")
    approved: bool = Field(..., description="Whether the command was approved")
    reason: str = Field(default="Approval denied by user", description="Reason for denial if not approved")
    operator_session_id: str | None = Field(default=None, description="Operator session ID (set by router from bound operator)")
    operator_id: str | None = Field(default=None, description="Operator ID (set by router from bound operator)")


class OperatorSlotCreationRequest(G8eBaseModel):
    """Request model for operator slot creation.

    Called by g8ed during user initialization and device link creation.
    g8ee handles the actual write to the operator document.
    """
    user_id: str = Field(..., description="User ID")
    organization_id: str = Field(..., description="Organization ID")
    slot_number: int = Field(..., description="Slot number")
    operator_type: str = Field(..., description="Operator type (CLOUD, SYSTEM)")
    cloud_subtype: str | None = Field(default=None, description="Cloud operator subtype")
    name_prefix: str = Field(default="operator", description="Name prefix")
    is_g8e_node: bool = Field(default=False, description="Is g8e pod operator")


class OperatorSlotCreationResponse(G8eBaseModel):
    """Response for operator slot creation."""
    success: bool
    operator_id: str | None = None
    api_key: str | None = None
    error: str | None = None


class OperatorSlotClaimRequest(G8eBaseModel):
    """Request model for operator slot claiming.

    Called by g8ed during device registration.
    g8ee handles the actual write to the operator document.
    """
    operator_id: str = Field(..., description="Operator ID")
    operator_session_id: str = Field(..., description="Operator session ID")
    bound_web_session_id: str | None = Field(default=None, description="Bound web session ID")
    operator_type: str = Field(..., description="Operator type")


class OperatorSlotClaimResponse(G8eBaseModel):
    """Response for operator slot claiming."""
    success: bool
    error: str | None = None


class OperatorUpdateApiKeyRequest(G8eBaseModel):
    """Request model for updating an operator's API key.

    Called by g8ed to issue API keys for existing slots that were created without keys.
    g8ee handles the actual write to the operator document.
    """
    operator_id: str = Field(..., description="Operator ID")
    api_key: str = Field(..., description="New API key")


class OperatorUpdateApiKeyResponse(G8eBaseModel):
    """Response for updating an operator's API key."""
    success: bool
    error: str | None = None


class OperatorBindRequest(G8eBaseModel):
    """Request model for operator binding.

    Called by g8ed during operator bind operations.
    g8ee handles the actual write to the operator document.
    """
    operator_ids: list[str] = Field(..., description="Operator IDs to bind")
    web_session_id: str = Field(..., description="Web session ID")
    user_id: str = Field(..., description="User ID")


class OperatorBindResponse(G8eBaseModel):
    """Response for operator binding."""
    success: bool
    bound_count: int = 0
    failed_count: int = 0
    bound_operator_ids: list[str] = Field(default_factory=list)
    failed_operator_ids: list[str] = Field(default_factory=list)
    errors: list[dict] = Field(default_factory=list)


class OperatorUnbindRequest(G8eBaseModel):
    """Request model for operator unbinding.

    Called by g8ed during operator unbind operations.
    g8ee handles the actual write to the operator document.
    """
    operator_ids: list[str] = Field(..., description="Operator IDs to unbind")
    web_session_id: str = Field(..., description="Web session ID")
    user_id: str = Field(..., description="User ID")


class OperatorUnbindResponse(G8eBaseModel):
    """Response for operator unbinding."""
    success: bool
    unbound_count: int = 0
    failed_count: int = 0
    unbound_operator_ids: list[str] = Field(default_factory=list)
    failed_operator_ids: list[str] = Field(default_factory=list)
    errors: list[dict] = Field(default_factory=list)
    operator_session_id: str = Field(default="", description="Operator session ID (set by router from bound operator)")
    operator_id: str = Field(default="", description="Operator ID (set by router from bound operator)")


class InternalOperatorAuthCall(G8eBaseModel):
    """Request model for operator authentication via API key (Bearer) relayed through g8ed.

    Aligned with shared/models/wire/operator_auth_call.json (InternalOperatorAuthCall)
    """
    model_config = ConfigDict(extra="forbid")

    authorization: str = Field(..., description="The Bearer token (API key) for the operator")
    runtime_config: dict | None = Field(default=None)


class OperatorAuthenticateResponse(G8eBaseModel):
    """Response model for operator authentication."""
    success: bool
    operator_session_id: str | None = None
    operator_id: str | None = None
    user_id: str | None = None
    api_key: str | None = None
    config: dict | None = None
    session: dict | None = None
    operator_cert: str | None = None
    operator_cert_key: str | None = None
    error: str | None = None


class OperatorDeviceLinkRegisterRequest(G8eBaseModel):
    """Request model for device-link operator registration.

    Called by g8ed after device-link token consumption.
    Trust model: caller is g8ed via internal mTLS. No authorization header.
    """
    operator_id: str = Field(..., description="Operator ID")
    user_id: str = Field(..., description="User ID")
    organization_id: str | None = Field(default=None, description="Organization ID")
    operator_type: str = Field(default="SYSTEM", description="Operator type")
    system_fingerprint: str | None = Field(default=None, description="System fingerprint")


class OperatorDeviceLinkRegisterResponse(G8eBaseModel):
    """Response model for device-link operator registration."""
    success: bool
    operator_id: str | None = None
    operator_session_id: str | None = None
    user_id: str | None = None
    api_key: str | None = None
    operator_cert: str | None = None
    operator_cert_key: str | None = None
    session: dict | None = None
    error: str | None = None


class OperatorSessionValidateRequest(G8eBaseModel):
    """Request model for operator session validation."""
    operator_session_id: str = Field(..., description="Operator session ID")


class OperatorSessionValidateResponse(G8eBaseModel):
    """Response model for operator session validation."""
    success: bool
    valid: bool
    user_id: str | None = None
    operator_id: str | None = None
    session_type: str | None = None
    error: str | None = None


class OperatorSessionRefreshRequest(G8eBaseModel):
    """Request model for operator session refresh."""
    operator_session_id: str = Field(..., description="Operator session ID")


class OperatorSessionRefreshResponse(G8eBaseModel):
    """Response model for operator session refresh."""
    success: bool
    operator_id: str | None = None
    session: dict | None = None
    error: str | None = None


class PendingApprovalsResponse(G8eBaseModel):
    """Response for GET /operator/approval/pending.

    Returns all pending approvals currently waiting for user response.
    """
    pending_approvals: dict[str, PendingApproval] = Field(default_factory=dict, description="Pending approvals keyed by approval_id")


class StopAIRequest(G8eBaseModel):
    """Request model for stopping active AI processing."""
    investigation_id: str = Field(..., description="Investigation ID to stop processing for")
    reason: str = Field(default="User requested stop", description="Reason for stopping")
    web_session_id: str | None = Field(default=None, description="Web session ID for SSE routing")


class StopOperatorRequest(G8eBaseModel):
    """Request model for stopping an operator via pub/sub shutdown command."""
    operator_id: str = Field(..., description="Operator ID")
    operator_session_id: str = Field(..., description="Operator session ID")
    user_id: str | None = Field(default=None, description="User ID")


class OperatorSessionRegistrationRequest(G8eBaseModel):
    """Request model for registering or deregistering an operator session heartbeat subscription.

    Called by g8ed when an operator authenticates (register) or goes offline/stops (deregister).
    Triggers g8ee to subscribe or unsubscribe from the heartbeat pub/sub channel for this session.
    """
    operator_id: str = Field(..., description="Operator ID")
    operator_session_id: str = Field(..., description="Operator session ID")


class OperatorTerminateRequest(G8eBaseModel):
    """Request model for operator termination."""
    operator_id: str = Field(..., description="Operator ID")


class OperatorTerminateResponse(G8eBaseModel):
    """Response for operator termination."""
    success: bool
    error: str | None = None


class ApiKeyGenerationRequest(G8eBaseModel):
    """Request model for API key generation.
    
    Authority: g8ee.
    """
    prefix: str = Field(default="g8e_", description="API key prefix")


class ApiKeyGenerationResponse(G8eBaseModel):
    """Response model for API key generation."""
    success: bool
    api_key: str | None = None
    error: str | None = None


class G8epOperatorActivationRequest(G8eBaseModel):
    """Request model for g8ep operator activation.
    
    Authority: g8ee.
    Aligned with shared/models/wire/internal_requests.json (g8ep_operator_activation)
    """
    user_id: str = Field(..., description="ID of the user whose g8ep operator should be activated")


class G8epOperatorActivationResponse(G8eBaseModel):
    """Response model for g8ep operator activation.
    
    Aligned with shared/models/wire/operator_management_responses.json (g8ep_operator_activation_response)
    """
    success: bool
    error: str | None = Field(default=None, description="Error message when unsuccessful")


class G8epOperatorRelaunchRequest(G8eBaseModel):
    """Request model for g8ep operator relaunch.
    
    Authority: g8ee.
    Aligned with shared/models/wire/internal_requests.json (g8ep_operator_relaunch)
    """
    user_id: str = Field(..., description="ID of the user whose g8ep operator should be relaunched")


class G8epOperatorRelaunchResponse(G8eBaseModel):
    """Response model for g8ep operator relaunch.
    
    Aligned with shared/models/wire/operator_management_responses.json (g8ep_operator_relaunch_response)
    """
    success: bool
    operator_id: str | None = Field(default=None, description="The ID of the relaunched operator slot")
    error: str | None = Field(default=None, description="Error message when unsuccessful")


class OperatorListenSessionAuthRequest(G8eBaseModel):
    """Request model for starting a session auth listener.
    
    Called by g8ed during device registration bootstrap.
    """
    operator_session_id: str = Field(..., description="Operator session ID")
    operator_id: str = Field(..., description="Operator ID")
    user_id: str = Field(..., description="User ID")
    organization_id: str | None = Field(default=None, description="Organization ID")


class OperatorCertificateRevokeRequest(G8eBaseModel):
    """Request model for operator certificate revocation."""
    serial: str = Field(..., description="Certificate serial number to revoke")
    reason: str = Field(default="revoked", description="Reason for revocation")
    operator_id: str | None = Field(default=None, description="Optional operator ID")


class OperatorCertificateRevokeResponse(G8eBaseModel):
    """Response model for operator certificate revocation."""
    success: bool
    error: str | None = None


class DirectCommandRequest(G8eBaseModel):
    """Request model for direct command execution (bypasses AI).

    Body carries only command-specific payload. All identity and context
    (user_id, web_session_id, operator_id, operator_session_id, case_id,
    investigation_id) come from G8eHttpContext headers.
    """
    command: str = Field(..., description="Command to execute on operator")
    execution_id: str = Field(..., description="Execution ID for tracking")
    hostname: str | None = Field(default=None, description="Hostname of the target operator for result display")
    source: str = Field(default="anchored_terminal", description="Source of the command")
