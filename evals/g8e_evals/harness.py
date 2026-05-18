from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any, Dict, List, Optional, Literal, Union

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
    provider: Optional[str] = None
    model: Optional[str] = None
    api_key: Optional[str] = None
    endpoint: Optional[str] = None

@dataclass
class SUTConfig:
    primary: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    assistant: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    lite: LLMRoleConfig = field(default_factory=LLMRoleConfig)
    
    operator_url: str = "https://localhost:9000"
    operator_id: Optional[str] = None
    operator_session_id: Optional[str] = None
    state_root: str = "test-state-root-v1"
    
    l2_private_key: Optional[str] = None
    l2_key_id: Optional[str] = None
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
    transaction_id: Optional[str] = None
    # receipt can be either ChatEvaluationReceipt (from chat SUT) or ActionReceipt (from substrate)
    receipt: Optional[Union["ChatEvaluationReceipt", ActionReceipt]] = None
    receipt_signature: Optional[str] = None
    receipt_verified: bool = False
    binding: BindingType = BindingType.UNBOUND
    unbound_reason: Optional[str] = None


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
    timestamp: datetime = field(default_factory=datetime.utcnow)


@dataclass
class Aggregate:
    suite: str
    pass_rate: float
    total_tasks: int
    passed_tasks: int
    receipt_coverage_pct: float
    receipt_verification_pct: float
    metadata: Dict[str, Any] = field(default_factory=dict)
