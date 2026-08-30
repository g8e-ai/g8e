# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import StrEnum
from typing import TYPE_CHECKING, Any, Protocol

from app.models.model_telemetry import ModelCallTelemetry
from g8e.constants import PORTS
from g8e.operator.v1.operator_pb2 import ActionReceipt
from g8e_evals.arms import Arm, ArmDefinition, get_arm_definition
from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.models import ScoreDetails, TaskMetadata

# Forward reference for ChatEvaluationReceipt to avoid circular import
if TYPE_CHECKING:
    pass


class EvidenceLike(Protocol):
    """Structural protocol for chat evidence attached to a Response.

    Both ``ChatEvaluationReceipt`` (g8ee arms) and ``_DirectEvidenceWrapper``
    (direct arm) satisfy this protocol. The CLI and report writer consume
    these attributes without forcing every arm into the SSE evidence schema.
    """

    @property
    def terminal_event(self) -> str | None: ...

    @property
    def event_count(self) -> int: ...

    def model_dump(self) -> dict[str, Any]: ...


class BindingType(StrEnum):
    RECEIPT_BOUND = "RECEIPT_BOUND"
    UNBOUND = "UNBOUND"


@dataclass
class LLMRoleConfig:
    provider: str | None = None
    model: str | None = None
    api_key: str | None = None
    endpoint: str | None = None

@dataclass
class SUTConfig:
    g8ee_url: str
    primary: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    assistant: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    lite: LLMRoleConfig = field(default_factory=LLMRoleConfig)

    web_search_project: str | None = None
    web_search_app: str | None = None
    web_search_api_key: str | None = None

    operator_url: str = f"https://localhost:{PORTS['ports']['OperatorHttps']['value']}"
    operator_session_id: str | None = None
    auth_context: CLIAuthContext | None = None
    state_root: str = "test-state-root-v1"

    l2_private_key: str | None = None
    l2_key_id: str | None = None
    arm: Arm = Arm.ENSEMBLE_UNGOVERNED

    @property
    def arm_definition(self) -> ArmDefinition:
        """Return the static arm definition for this config's arm."""
        return get_arm_definition(self.arm)

@dataclass
class Task:
    id: str
    prompt: str
    metadata: TaskMetadata = field(default_factory=TaskMetadata)


@dataclass
class Response:
    answer: str
    model: str
    arm: Arm = Arm.ENSEMBLE_UNGOVERNED
    transaction_id: str | None = None
    chat_evidence: EvidenceLike | None = None
    action_receipt: ActionReceipt | None = None
    receipt_verified: bool = False
    binding: BindingType = BindingType.UNBOUND
    unbound_reason: str | None = None


@dataclass
class Score:
    task_id: str
    passed: bool
    details: ScoreDetails = field(default_factory=ScoreDetails)
    model_calls: list[ModelCallTelemetry] = field(default_factory=list)


@dataclass
class RowResult:
    task: Task
    response: Response
    score: Score
    arm: Arm = Arm.ENSEMBLE_UNGOVERNED
    timestamp: datetime = field(default_factory=lambda: datetime.now(UTC))


@dataclass
class Aggregate:
    suite: str
    pass_rate: float
    total_tasks: int
    passed_tasks: int
    receipt_coverage_pct: float
    receipt_verification_pct: float
    metadata: dict[str, Any] = field(default_factory=dict)
