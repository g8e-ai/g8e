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
from app.models.http_context import RequestContext


class ResourceCreationRequest(G8eBaseModel):
    """Typed request to create new case and investigation resources.

    Replaces the in-band NEW_CASE_ID sentinel string with structured intent
    in the request body. When create_case is True, the chat endpoint will
    create a new case and investigation inline before processing the message.
    """
    create_case: bool = Field(default=False, description="When True, create new case and investigation")
    case_title: str | None = Field(default=None, description="Optional case title override")


class SettingsGetRequest(G8eBaseModel):
    """Request model for GET /settings/user."""
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")


class ChatMessageRequest(G8eBaseModel):
    """Request model for chat messages.

    Identity and business context (case_id, investigation_id, web_session_id,
    user_id) come from the context field in the request body.
    The request body carries user-controlled content plus context.

    To create a new case+investigation, set resource_creation.create_case to True.
    """
    context: RequestContext = Field(..., description="Request context with session/case/investigation identity")
    message: str = Field(..., description="Chat message content")
    attachments: list[AttachmentMetadata] | None = Field(default_factory=list, description="File attachments")
    sentinel_mode: bool = Field(default=True, description="Sentinel mode - when True, data is scrubbed before storage and AI sees redacted data")
    resource_creation: ResourceCreationRequest | None = Field(default=None, description="Resource creation configuration - when set with create_case=True, creates new case and investigation")
    llm_primary_provider: str | None = Field(default=None, description="Primary LLM provider override for complex tasks - null uses server default")
    llm_assistant_provider: str | None = Field(default=None, description="Assistant LLM provider override for simple tasks - null uses server default")
    llm_lite_provider: str | None = Field(default=None, description="Lite LLM provider override for quick tasks - null uses server default")
    llm_primary_model: str | None = Field(default=None, description="Primary LLM model override for complex tasks - null uses server default")
    llm_assistant_model: str | None = Field(default=None, description="Assistant LLM model override for simple tasks - null uses server default")
    llm_lite_model: str | None = Field(default=None, description="Lite LLM model override for quick tasks - null uses server default")
    llm_primary_api_key: str | None = Field(default=None, description="Primary LLM API key override")
    llm_primary_endpoint: str | None = Field(default=None, description="Primary LLM endpoint override")
    llm_assistant_api_key: str | None = Field(default=None, description="Assistant LLM API key override")
    llm_assistant_endpoint: str | None = Field(default=None, description="Assistant LLM endpoint override")
    llm_lite_api_key: str | None = Field(default=None, description="Lite LLM API key override")
    llm_lite_endpoint: str | None = Field(default=None, description="Lite LLM endpoint override")


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
    the context field in the request body. Only approval-specific fields are in the body.
    The router enriches this with operator_session_id / operator_id from
    the bound operator before passing it to the approval service.
    """
    context: RequestContext = Field(..., description="Request context with session/case/investigation identity")
    approval_id: str = Field(..., description="Approval ID")
    approved: bool = Field(..., description="Whether the command was approved")
    reason: str = Field(default="Approval denied by user", description="Reason for denial if not approved")
    operator_session_id: str | None = Field(default=None, description="Operator session ID (set by router from bound operator)")
    operator_id: str | None = Field(default=None, description="Operator ID (set by router from bound operator)")


class OperatorSlotCreationRequest(G8eBaseModel):
    """Request model for operator slot creation.

    Called by client during user initialization and device link creation.
    g8ee handles the actual write to the operator document.

    Identity and business context (user_id, organization_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    slot_number: int = Field(..., description="Slot number")
    operator_type: str = Field(..., description="Operator type (CLOUD, SYSTEM)")
    cloud_subtype: str | None = Field(default=None, description="Cloud operator subtype")
    name_prefix: str = Field(default="operator", description="Name prefix")


class OperatorSlotCreationResponse(G8eBaseModel):
    """Response for operator slot creation."""
    success: bool
    operator_id: str | None = None
    api_key: str | None = None
    error: str | None = None


class OperatorSlotClaimRequest(G8eBaseModel):
    """Request model for operator slot claiming.

    Called by client during device registration.
    g8ee handles the actual write to the operator document.

    Identity and business context (user_id, organization_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
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

    Called by client to issue API keys for existing slots that were created without keys.
    g8ee handles the actual write to the operator document.

    Identity and business context (user_id, organization_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    operator_id: str = Field(..., description="Operator ID")
    api_key: str = Field(..., description="New API key")


class OperatorUpdateApiKeyResponse(G8eBaseModel):
    """Response for updating an operator's API key."""
    success: bool
    error: str | None = None


class OperatorBindRequest(G8eBaseModel):
    """Request model for operator binding.

    Called by client during operator bind operations.
    g8ee handles the actual write to the operator document.

    Identity and business context (user_id, organization_id, web_session_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    operator_ids: list[str] = Field(..., description="Operator IDs to bind")


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

    Called by client during operator unbinding operations.
    g8ee handles the actual write to the operator document.

    Identity and business context (user_id, organization_id, web_session_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    operator_ids: list[str] = Field(..., description="Operator IDs to unbind")


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
    """Request model for operator authentication via API key (Bearer) relayed through client.

    Internal g8ee-client API contract for operator authentication.

    Identity and business context (web_session_id, user_id, organization_id) come from the
    context field in the request body.
    """
    model_config = ConfigDict(extra="forbid")

    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    authorization: str = Field(..., description="The Bearer token (API key) for the operator")
    operator_session_id: str = Field(..., description="g8eo substrate operator session UUID — used as the g8ee session document ID so the CLI Bearer token resolves directly")
    runtime_config: dict | None = Field(default=None)


class OperatorAuthenticateResponse(G8eBaseModel):
    """Response model for operator authentication."""
    success: bool
    operator_session_id: str | None = None
    operator_id: str | None = None
    user_id: str | None = None
    api_key: str | None = None
    config: dict | None = None
    operator_session: dict | None = None
    operator_cert: str | None = None
    operator_cert_key: str | None = None
    error: str | None = None


class OperatorDeviceLinkRegisterRequest(G8eBaseModel):
    """Request model for device-link operator registration.

    Called by client after device-link token consumption.
    Trust model: caller is client via internal mTLS. No authorization header.

    Identity and business context (user_id, organization_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    operator_id: str | None = Field(default=None, description="Operator ID (optional if creating on-demand)")
    operator_type: str = Field(default="SYSTEM", description="Operator type")
    device_link_token: str | None = Field(default=None, description="Device link token for on-demand slot creation")
    operator_session_id: str | None = Field(default=None, description="g8eo substrate operator session ID — used as the g8ee session document ID so the CLI Bearer token resolves directly")


class OperatorDeviceLinkRegisterResponse(G8eBaseModel):
    """Response model for device-link operator registration."""
    success: bool
    operator_id: str | None = None
    operator_session_id: str | None = None
    user_id: str | None = None
    api_key: str | None = None
    operator_cert: str | None = None
    operator_cert_key: str | None = None
    operator_session: dict | None = None
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
    operator_session: dict | None = None
    error: str | None = None


class PendingApprovalsResponse(G8eBaseModel):
    """Response for GET /operator/approval/pending.

    Returns all pending approvals currently waiting for user response.
    """
    pending_approvals: dict[str, PendingApproval] = Field(default_factory=dict, description="Pending approvals keyed by approval_id")


class StopAIRequest(G8eBaseModel):
    """Request model for stopping active AI processing.

    Identity and business context (case_id, investigation_id, web_session_id,
    user_id) come from the context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/case/investigation identity")
    reason: str = Field(default="User requested stop", description="Reason for stopping")


class StopOperatorRequest(G8eBaseModel):
    """Request model for stopping an operator via pub/sub shutdown command.

    Identity and business context (user_id, organization_id, web_session_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    operator_id: str = Field(..., description="Operator ID")
    operator_session_id: str = Field(..., description="Operator session ID")


class OperatorSessionRegistrationRequest(G8eBaseModel):
    """Request model for registering or deregistering an operator session heartbeat subscription.

    Called by client when an operator authenticates (register) or goes offline/stops (deregister).
    Triggers g8ee to subscribe or unsubscribe from the heartbeat pub/sub channel for this session.

    Identity and business context (user_id, organization_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    operator_id: str = Field(..., description="Operator ID")
    operator_session_id: str = Field(..., description="Operator session ID")


class OperatorTerminateRequest(G8eBaseModel):
    """Request model for operator termination.

    Identity and business context (user_id, organization_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
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


class OperatorListenSessionAuthRequest(G8eBaseModel):
    """Request model for starting a session auth listener.

    Called by client during device registration bootstrap.

    Identity and business context (user_id, organization_id) come from the
    context field in the request body.
    """
    context: RequestContext = Field(..., description="Request context with session/user/organization identity")
    operator_session_id: str = Field(..., description="Operator session ID")
    operator_id: str = Field(..., description="Operator ID")


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

    Body carries command-specific payload plus context. All identity and context
    (user_id, web_session_id, operator_id, operator_session_id, case_id,
    investigation_id) come from the context field.
    """
    context: RequestContext = Field(..., description="Request context with session/case/investigation identity")
    command: str = Field(..., description="Command to execute on operator")
    execution_id: str = Field(..., description="Execution ID for tracking")
    hostname: str | None = Field(default=None, description="Hostname of the target operator for result display")
    source: str = Field(default="anchored_terminal", description="Source of the command")


class UserSettingsUpdateResponse(G8eBaseModel):
    """Response model for user settings update sync."""
    success: bool
    error: str | None = None


# Client API response models for InternalHttpClient
class SSEPushResponse(G8eBaseModel):
    """Response for SSE event push to client."""
    success: bool
    listeners: int = 0
    error: str | None = None


class GrantIntentResponse(G8eBaseModel):
    """Response for intent grant request."""
    success: bool
    error: str | None = None


class IntentOperationResult(G8eBaseModel):
    """Result of intent operation."""
    success: bool
    error: str | None = None


class IntentRequestPayload(G8eBaseModel):
    """Payload for intent request."""
    context: RequestContext = Field(..., description="Request context with session/case/investigation identity")
    operator_id: str
    intent: str


class RevokeIntentResponse(G8eBaseModel):
    """Response for intent revoke request."""
    success: bool
    error: str | None = None


class OperatorLinkResponse(G8eBaseModel):
    """Response for operator device link generation."""
    success: bool
    link_token: str | None = None
    error: str | None = None


class OperatorLinkRequestPayload(G8eBaseModel):
    """Payload for operator device link request."""
    context: RequestContext = Field(..., description="Request context with session/case/investigation identity")
    operator_id: str
    user_id: str
