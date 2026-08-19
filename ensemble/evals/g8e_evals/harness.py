# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import Enum
from typing import Any, Dict, List, Optional, Literal, Union

from g8e.generated_paths import PathConstants, PortConstants
from g8e_evals.models import ActionReceipt, ScoreDetails, TaskMetadata

# Forward reference for ChatEvaluationReceipt to avoid circular import
from typing import TYPE_CHECKING
if TYPE_CHECKING:
    from g8e_evals.sut.g8ee_chat import ChatEvaluationReceipt


class BindingType(str, Enum):
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
    primary: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    assistant: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    lite: LLMRoleConfig = field(default_factory=LLMRoleConfig)

    web_search_project: str | None = None
    web_search_app: str | None = None
    web_search_api_key: str | None = None

    operator_url: str = f"https://localhost:{PortConstants.PORT_OPERATOR_HTTPS}"
    operator_session_id: str | None = None
    state_root: str = "test-state-root-v1"

    l2_private_key: str | None = None
    l2_key_id: str | None = None
    mode: Literal["receipt", "baseline"] = "receipt"

@dataclass
class Task:
    id: str
    prompt: str
    metadata: TaskMetadata = field(default_factory=TaskMetadata)


@dataclass
class Response:
    answer: str
    model: str
    transaction_id: str | None = None
    # receipt can be either ChatEvaluationReceipt (from chat SUT) or ActionReceipt (from Gateway)
    receipt: Union[ChatEvaluationReceipt, ActionReceipt] | None = None
    receipt_signature: str | None = None
    receipt_verified: bool = False
    binding: BindingType = BindingType.UNBOUND
    unbound_reason: str | None = None


@dataclass
class Score:
    task_id: str
    passed: bool
    details: ScoreDetails = field(default_factory=ScoreDetails)


@dataclass
class RowResult:
    task: Task
    response: Response
    score: Score
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
