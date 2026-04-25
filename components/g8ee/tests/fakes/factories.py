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

"""Consolidated data builders (factories) for g8ee tests."""

import uuid
from datetime import datetime
from app.constants import (
    AuthMethod,
    CaseStatus,
    ComponentName,
    ComponentStatus,
    EscalationRisk,
    EventType,
    HeartbeatType,
    InvestigationStatus,
    NEW_CASE_ID,
    OperatorStatus,
    OperatorType,
    Priority,
    Severity,
    TribunalMember,
)
from app.models.auth import AuthenticatedUser
from app.models.agents.tribunal import (
    CandidateCommand,
)
from app.models.http_context import BoundOperator, G8eHttpContext
from app.models.cases import CaseModel
from app.models.investigations import (
    ConversationHistoryMessage,
    ConversationMessageMetadata,
    EnrichedInvestigationContext,
    InvestigationCreateRequest,
    InvestigationCurrentState,
    InvestigationModel,
)
from app.models.memory import InvestigationMemory
from app.models.operators import (
    HeartbeatPerformanceMetrics,
    HeartbeatSystemIdentity,
    HeartbeatUptimeInfo,
    OperatorDocument,
    OperatorHeartbeat,
    OperatorSystemInfo,
    SystemInfoEnvironment,
    SystemInfoUserDetails,
)
from app.utils.timestamp import now


# ---------------------------------------------------------------------------
# Investigation Factories
# ---------------------------------------------------------------------------

def create_investigation_request(
    case_id: str = "test-case-id",
    case_title: str = "test-title",
    case_description: str = "test-description",
    priority: Priority = Priority.MEDIUM,
    user_id: str = "test-user-id",
    user_email: str = "test@example.com",
    sentinel_mode: bool = True,
) -> InvestigationCreateRequest:
    """Create an InvestigationCreateRequest with sensible defaults."""
    return InvestigationCreateRequest(
        case_id=case_id,
        case_title=case_title,
        case_description=case_description,
        priority=priority,
        user_id=user_id,
        user_email=user_email,
        sentinel_mode=sentinel_mode,
    )


def create_investigation_data(
    investigation_id: str | None = None,
    status: InvestigationStatus = InvestigationStatus.OPEN,
    sentinel_mode: bool = True,
    case_id: str | None = None,
    case_title: str = "test-title",
    case_description: str = "test-description",
    priority: Priority = Priority.MEDIUM,
    user_id: str | None = None,
    user_email: str = "test@example.com",
    created_at: datetime | None = None,
) -> InvestigationModel:
    """Create a fully populated InvestigationModel for unit tests."""
    # Generate unique IDs if not provided
    if investigation_id is None or case_id is None or user_id is None:
        unique_suffix = str(uuid.uuid4())[:8]
        investigation_id = investigation_id or f"test-inv-{unique_suffix}"
        case_id = case_id or f"test-case-{unique_suffix}"
        user_id = user_id or f"test-user-{unique_suffix}"
    
    return InvestigationModel(
        id=investigation_id,
        status=status,
        case_id=case_id,
        case_title=case_title,
        case_description=case_description,
        priority=priority,
        user_id=user_id,
        user_email=user_email,
        sentinel_mode=sentinel_mode,
        created_at=created_at or now(),
        current_state=InvestigationCurrentState(
            active_attempt=1,
            escalation_risk=EscalationRisk.LOW,
            collaboration_status={ComponentName.G8EE: ComponentStatus.ACTIVE},
        ),
    )


def build_enriched_context(
    investigation_id: str = "test-inv-id",
    case_id: str = "test-case-id",
    case_title: str = "test-title",
    case_description: str = "test-description",
    user_id: str = "test-user-id",
    status: InvestigationStatus = InvestigationStatus.OPEN,
    sentinel_mode: bool = True,
    conversation_history: list | None = None,
    operator_documents: list[OperatorDocument] | None = None,
    **kwargs,
) -> EnrichedInvestigationContext:
    """Create an EnrichedInvestigationContext for unit tests."""
    return EnrichedInvestigationContext(
        id=investigation_id,
        case_id=case_id,
        case_title=case_title,
        case_description=case_description,
        user_id=user_id,
        status=status,
        sentinel_mode=sentinel_mode,
        conversation_history=conversation_history or [],
        operator_documents=operator_documents or [],
        **kwargs,
    )


# Alias for backward compatibility and clearer naming
build_enriched_investigation = build_enriched_context


def build_investigation_with_operators(
    inv_id: str = "test-inv-id",
    case_id: str = "test-case-id",
    operators_bound: bool = True,
    operator_documents: list[OperatorDocument] | None = None,
    sentinel_mode: bool = True,
    web_session_id: str = "test-web-session",
) -> EnrichedInvestigationContext:
    """Build an EnrichedInvestigationContext with operator_documents."""
    total = len(operator_documents) if operator_documents else (1 if operators_bound else 0)
    return build_enriched_context(
        investigation_id=inv_id,
        case_id=case_id,
        web_session_id=web_session_id,
        sentinel_mode=sentinel_mode,
        operator_documents=operator_documents or [],
    )


def create_conversation_message(
    sender: EventType,
    content: str = "",
    message_id: str | None = None,
    timestamp: datetime | None = None,
    metadata: ConversationMessageMetadata | None = None,
    prev_hash: str = "0" * 64,
    entry_hash: str = "0" * 64,
) -> ConversationHistoryMessage:
    """Create a ConversationHistoryMessage for testing."""
    return ConversationHistoryMessage(
        id=message_id or str(uuid.uuid4()),
        sender=sender,
        content=content,
        timestamp=timestamp or now(),
        metadata=metadata or ConversationMessageMetadata(),
        prev_hash=prev_hash,
        entry_hash=entry_hash,
    )


# ---------------------------------------------------------------------------
# HTTP & Auth Factories
# ---------------------------------------------------------------------------

def build_g8e_http_context(
    web_session_id: str = "test-web-session",
    user_id: str = "test-user-id",
    case_id: str | None = None,
    investigation_id: str | None = None,
    organization_id: str | None = None,
    bound_operators: list[BoundOperator] | None = None,
    source_component: ComponentName = ComponentName.G8ED,
    new_case: bool = False,
) -> G8eHttpContext:
    """Build a G8eHttpContext with fixed deterministic defaults for unit tests."""
    resolved_inv_id = investigation_id
    if resolved_inv_id is None:
        resolved_inv_id = NEW_CASE_ID if new_case else "test-investigation-id"

    return G8eHttpContext(
        web_session_id=web_session_id,
        user_id=user_id,
        case_id=case_id or ("test-case-id" if not new_case else NEW_CASE_ID),
        investigation_id=resolved_inv_id,
        organization_id=organization_id,
        bound_operators=bound_operators or [],
        source_component=source_component,
        new_case=new_case,
    )


def build_bound_operator(
    operator_id: str = "test-operator-id",
    operator_session_id: str = "test-operator-session-id",
    status: OperatorStatus = OperatorStatus.BOUND,
) -> BoundOperator:
    """Build a BoundOperator with fixed deterministic defaults for unit tests."""
    return BoundOperator(
        operator_id=operator_id,
        operator_session_id=operator_session_id,
        status=status,
    )


def build_minimal_operator_document(
    operator_id: str | None = None,
    user_id: str | None = None,
    status: OperatorStatus = OperatorStatus.BOUND,
    hostname: str = "test-host",
    operator_type: OperatorType = OperatorType.SYSTEM,
) -> OperatorDocument:
    """Build an OperatorDocument with minimal defaults for unit/safety tests.

    Uses a non-root user (``test-user``) with no init_system or user_details.
    For evaluation and benchmark accuracy tests that need a realistic
    production Operator context, use ``build_production_operator_document`` instead.
    """
    # Generate unique IDs if not provided
    if operator_id is None or user_id is None:
        unique_suffix = str(uuid.uuid4())[:8]
        operator_id = operator_id or f"test-operator-{unique_suffix}"
        user_id = user_id or f"test-user-{unique_suffix}"

    return OperatorDocument(
        id=operator_id,
        operator_session_id=f"sess-{operator_id}",
        user_id=user_id,
        status=status,
        current_hostname=hostname,
        operator_type=operator_type,
        system_info=OperatorSystemInfo(
            hostname=hostname,
            os="linux",
            architecture="x86_64",
            current_user="test-user",
            environment=SystemInfoEnvironment(pwd="/home/test-user"),
            interfaces=[],
        ),
    )


def build_production_operator_document(
    operator_id: str | None = None,
    hostname: str = "eval-node-01",
    operator_type: OperatorType = OperatorType.SYSTEM,
) -> OperatorDocument:
    """Build a production-like OperatorDocument for evaluation and benchmark tests.

    Reflects a realistic production Operator environment where the binary
    was started with ``sudo ./g8eo`` — root user, systemd init, bare-metal
    Linux host.  This ensures accuracy tests exercise the agent's reasoning
    without colliding with the security layer that blocks ``sudo``.
    """
    import uuid
    if operator_id is None:
        operator_id = f"test-op-{uuid.uuid4().hex[:8]}"
    operator_session_id = f"test-sess-{uuid.uuid4().hex[:8]}"
    user_id = f"test-user-{uuid.uuid4().hex[:8]}"
    return OperatorDocument(
        id=operator_id,
        operator_session_id=operator_session_id,
        user_id=user_id,
        status=OperatorStatus.BOUND,
        current_hostname=hostname,
        operator_type=operator_type,
        system_info=OperatorSystemInfo(
            hostname=hostname,
            os="linux",
            architecture="amd64",
            current_user="root",
            user_details=SystemInfoUserDetails(
                username="root",
                uid="0",
                gid="0",
                home="/root",
                shell="/bin/bash",
            ),
            environment=SystemInfoEnvironment(
                pwd="/root",
                is_container=False,
                init_system="systemd",
            ),
            interfaces=[],
        ),
    )


def build_case_model(
    case_id: str = "case-test-123",
    title: str = "Test Case",
    user_id: str = "user-test-123",
    web_session_id: str = "session-test-123",
    status: CaseStatus = CaseStatus.NEW,
    description: str = "Test description",
    source: ComponentName = ComponentName.G8ED,
    priority: Priority = Priority.MEDIUM,
    severity: Severity = Severity.LOW,
) -> CaseModel:
    """Build a CaseModel for unit tests."""
    return CaseModel(
        id=case_id,
        title=title,
        description=description,
        user_id=user_id,
        user_email="test@example.com",
        web_session_id=web_session_id,
        status=status,
        priority=priority,
        severity=severity,
        source=source,
        created_at=now(),
        updated_at=now(),
    )


def build_operator_heartbeat(
    operator_id: str = "test-operator-id",
    timestamp: datetime | None = None,
) -> OperatorHeartbeat:
    """Build a valid OperatorHeartbeat for testing."""
    return OperatorHeartbeat(
        timestamp=timestamp or now(),
        heartbeat_type=HeartbeatType.AUTOMATIC,
        system_identity=HeartbeatSystemIdentity(
            hostname="test-host",
            os="linux",
            architecture="x86_64",
            cpu_count=8,
            memory_mb=16384,
        ),
        performance=HeartbeatPerformanceMetrics(
            cpu_percent=10.0,
            memory_percent=25.0,
            disk_percent=50.0,
            network_latency=5.0,
        ),
        uptime=HeartbeatUptimeInfo(
            uptime_display="1 day, 2:30:15",
            uptime_seconds=95415,
        ),
    )


def create_mock_llm_provider(text: str):
    """Build a mock LLM provider for tests.
    
    If text is provided, generate_content returns a mock response with that text.
    """
    from unittest.mock import AsyncMock, MagicMock
    from app.llm.llm_types import GenerateContentResponse, Candidate, Content, Part
    
    provider = MagicMock()
    provider.generate_content_stream = AsyncMock()
    
    if text is not None:
        mock_response = GenerateContentResponse(
            candidates=[Candidate(content=Content(role="model", parts=[Part.from_text(text)]))]
        )
        provider.generate_content = AsyncMock(return_value=mock_response)
    else:
        provider.generate_content = AsyncMock()
        
    return provider


def make_candidate_command(
    command: str = "ls -la",
    pass_index: int = 0,
    member: TribunalMember = TribunalMember.AXIOM,
) -> CandidateCommand:
    """Build a CandidateCommand model for tests."""
    return CandidateCommand(
        command=command,
        pass_index=pass_index,
        member=member,
    )


def build_authenticated_user(
    uid: str,
    user_id: str,
    email: str,
    organization_id: str | None,
    web_session_id: str,
    auth_method: AuthMethod,
) -> AuthenticatedUser:
    """Build an AuthenticatedUser with fixed deterministic defaults for unit tests."""
    return AuthenticatedUser(
        uid=uid,
        user_id=user_id,
        email=email,
        organization_id=organization_id,
        web_session_id=web_session_id,
        auth_method=auth_method,
    )


def create_investigation_memory(
    investigation_id: str | None = None,
    case_id: str | None = None,
    user_id: str | None = None,
    case_title: str = "test-title",
    investigation_summary: str = "Test investigation summary",
    communication_preferences: str = "Email updates preferred",
    technical_background: str = "Senior developer",
    response_style: str = "Concise technical explanations",
    problem_solving_approach: str = "Systematic debugging",
    interaction_style: str = "Autonomous with confirmations",
) -> InvestigationMemory:
    """Create an InvestigationMemory for unit tests."""
    # Generate unique IDs if not provided
    if investigation_id is None or case_id is None or user_id is None:
        unique_suffix = str(uuid.uuid4())[:8]
        investigation_id = investigation_id or f"test-inv-{unique_suffix}"
        case_id = case_id or f"test-case-{unique_suffix}"
        user_id = user_id or f"test-user-{unique_suffix}"
    
    return InvestigationMemory(
        investigation_id=investigation_id,
        case_id=case_id,
        user_id=user_id,
        status=InvestigationStatus.OPEN,
        case_title=case_title,
        investigation_summary=investigation_summary,
        communication_preferences=communication_preferences,
        technical_background=technical_background,
        response_style=response_style,
        problem_solving_approach=problem_solving_approach,
        interaction_style=interaction_style,
    )
