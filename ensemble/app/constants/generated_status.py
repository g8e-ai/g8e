# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Generated status constants and enums.

Enums and constants are re-exported from the g8e protocol package
(protocol/python/g8e/), which is the SSOT. The ensemble must not hand-roll
duplicates that drift from the protocol.
"""

from enum import IntEnum, StrEnum

from g8e.constants import ComponentName
from g8e.enums import (
    ActionType,
    AISource,
    AITaskId,
    AuditEventSource,
    AuditEventType,
    AuditorReason,
    AuditSseEventType,
    AuthAuditEventType,
    AuthAuditResult,
    AuthProvider,
    CaseStatus,
    CitationLayout,
    CommandCategory,
    CommandErrorType,
    CommandStatus,
    ComponentStatus,
    ConnectionState,
    ConsensusAuditMode,
    ConsensusAuditStatus,
    ConsensusMember,
    DownloadAuditEventType,
    Environment,
    EventType,
    G8eAvailability,
    GatewayMode,
    HeartbeatType,
    HistoryActor,
    InvestigationStatus,
    LoginAuditEventType,
    OperatorHistoryEventType,
    OperatorRole,
    OperatorStatus,
    OperatorType,
    Platform,
    Priority,
    ReasoningAgent,
    RiskLevel,
    RiskThreshold,
    SentinelStatus,
    SessionEndReason,
    SessionEventType,
    SessionKeyPrefix,
    SessionSuspiciousReason,
    SessionType,
    Severity,
    SlashTier,
    StreamStatus,
    SystemHealth,
    TaskStatus,
    ThinkingPhase,
    TieBreakReason,
    ToolCallDefaults,
    ToolScope,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
    UserRole,
    UserStatus,
    VaultMode,
    VersionStability,
    WorkflowType,
)
from g8e.enums import (
    LlmModels as LLMs,
)
from g8e.enums import (
    OperatorToolName as _ProtocolOperatorToolName,
)


class ScrubberPriority(IntEnum):
    """Priority levels for Sentinel scrubber patterns. Lower values = higher priority."""

    EXACT_CREDENTIAL = 1
    URL_OR_CONNECTION = 2
    CONTEXTUAL_CREDENTIAL = 3
    GENERIC_PII = 4


# OperatorToolName: re-export the protocol enum, extended with ensemble-specific
# tools that are not yet in the Go protocol SSOT. The 19 protocol members are
# sourced from g8e.enums.OperatorToolName; the 2 ensemble-specific members
# (GRANT_INTENT, REVOKE_INTENT) are for the in-progress AWS cloud operator
# feature and will move to the protocol when that feature is fleshed out.
_ensemble_tool_members: dict[str, str] = {
    "GRANT_INTENT": "grant_intent_permission",
    "REVOKE_INTENT": "revoke_intent_permission",
}

OperatorToolName = StrEnum(  # type: ignore[misc]
    "OperatorToolName",
    {
        **{m.name: m.value for m in _ProtocolOperatorToolName},
        **_ensemble_tool_members,
    },
)

__all__ = [
    "AISource",
    "AITaskId",
    "ActionType",
    "AuditEventSource",
    "AuditEventType",
    "AuditSseEventType",
    "AuditorReason",
    "AuthAuditEventType",
    "AuthAuditResult",
    "AuthProvider",
    "CaseStatus",
    "CitationLayout",
    "CommandCategory",
    "CommandErrorType",
    "CommandStatus",
    "ComponentName",
    "ComponentStatus",
    "ConnectionState",
    "ConsensusAuditMode",
    "ConsensusAuditStatus",
    "ConsensusMember",
    "DownloadAuditEventType",
    "Environment",
    "EventType",
    "G8eAvailability",
    "GatewayMode",
    "HeartbeatType",
    "HistoryActor",
    "InvestigationStatus",
    "LLMs",
    "LoginAuditEventType",
    "OperatorHistoryEventType",
    "OperatorRole",
    "OperatorStatus",
    "OperatorToolName",
    "OperatorType",
    "Platform",
    "Priority",
    "ReasoningAgent",
    "RiskLevel",
    "RiskThreshold",
    "ScrubberPriority",
    "SentinelStatus",
    "SessionEndReason",
    "SessionEventType",
    "SessionKeyPrefix",
    "SessionSuspiciousReason",
    "SessionType",
    "Severity",
    "SlashTier",
    "StreamStatus",
    "SystemHealth",
    "TaskStatus",
    "ThinkingPhase",
    "TieBreakReason",
    "ToolCallDefaults",
    "ToolScope",
    "TriageComplexityClassification",
    "TriageConfidence",
    "TriageIntentClassification",
    "TriageRequestPosture",
    "UserRole",
    "UserStatus",
    "VaultMode",
    "VersionStability",
    "WorkflowType",
]
