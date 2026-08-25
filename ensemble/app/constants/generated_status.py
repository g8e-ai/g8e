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

from enum import StrEnum

from g8e.enums import OperatorToolName as _ProtocolOperatorToolName

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


# ComponentName is now imported from g8e.constants
from g8e.constants import ComponentName

# Enums sourced from g8e protocol STATUS constants via g8e.enums
from g8e.enums import (
    SessionEndReason,
    SessionEventType,
    SessionKeyPrefix,
    SessionSuspiciousReason,
    SentinelStatus,
    VaultMode,
    CommandStatus,
    ConnectionState,
    StreamStatus,
    SystemHealth,
    Environment,
    HistoryActor,
    ActionType,
    ActionStatus,
    AISource,
    AuditEventSource,
    AuditEventType,
    AuditSseEventType,
    AuthAuditEventType,
    AuthAuditResult,
    AuthProvider,
    CitationLayout,
    DownloadAuditEventType,
    G8eActionType,
    G8eAvailability,
    GatewayMode,
    LoginAuditEventType,
    ToolCallDefaults,
    UserRole,
    UserStatus,
    HeartbeatType,
    LlmModels as LLMs,
)


from enum import IntEnum


class ScrubberPriority(IntEnum):
    """Priority levels for Sentinel scrubber patterns. Lower values = higher priority."""

    EXACT_CREDENTIAL = 1
    URL_OR_CONNECTION = 2
    CONTEXTUAL_CREDENTIAL = 3
    GENERIC_PII = 4


from g8e.enums import CommandErrorType


from g8e.enums import ToolScope


from g8e.enums import AITaskId


from g8e.enums import OperatorStatus


from g8e.enums import OperatorType


from g8e.enums import Platform


from g8e.enums import Priority


from g8e.enums import RiskLevel


from g8e.enums import RiskThreshold


from g8e.enums import TaskStatus


from g8e.enums import VersionStability


from g8e.enums import ReasoningAgent


from g8e.enums import WorkflowType


from g8e.enums import ComponentStatus


from g8e.enums import EventType


from g8e.enums import InvestigationStatus


from g8e.enums import Severity


from g8e.enums import OperatorHistoryEventType


from g8e.enums import CommandCategory


from g8e.enums import TriageComplexityClassification


from g8e.enums import TriageConfidence


from g8e.enums import TriageIntentClassification


from g8e.enums import TriageRequestPosture


from g8e.enums import TieBreakReason


from g8e.enums import ConsensusMember


from g8e.enums import ConsensusAuditMode


from g8e.enums import ConsensusAuditStatus


from g8e.enums import AuditorReason


from g8e.enums import CaseStatus


from g8e.enums import SessionType


from g8e.enums import SlashTier
