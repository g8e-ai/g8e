"""Typed models for evaluation harness components.

These Pydantic models replace Dict[str, Any] usage throughout the evals codebase,
providing schema validation and making the harness robust against schema changes
in the Engine and protocol definitions.
"""

from __future__ import annotations

from enum import Enum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class ExecutionStatus(str, Enum):
    """Execution status enum matching operator.proto ExecutionStatus."""
    UNSPECIFIED = "EXECUTION_STATUS_UNSPECIFIED"
    EXECUTING = "EXECUTION_STATUS_EXECUTING"
    COMPLETED = "EXECUTION_STATUS_COMPLETED"
    FAILED = "EXECUTION_STATUS_FAILED"
    CANCELLED = "EXECUTION_STATUS_CANCELLED"
    TIMEOUT = "EXECUTION_STATUS_TIMEOUT"


class ActionReceipt(BaseModel):
    """Typed ActionReceipt model matching operator.proto ActionReceipt message.
    
    This is the signed proof of a completed or failed mutation, emitted by the
    Warden after execution.
    """
    model_config = ConfigDict(extra="ignore")

    transaction_id: str
    transaction_hash: str
    status: ExecutionStatus
    result_summary: str = ""
    state_root_before: str = ""
    state_root_after: str = ""
    executed_at_unix_ms: int = 0
    signer_key_id: str = ""
    signature: str = ""

    @classmethod
    def from_proto_dict(cls, data: dict[str, Any]) -> "ActionReceipt":
        """Create from protojson dict, handling enum conversion."""
        # Proto may return status as string enum name or int
        status_value = data.get("status", 0)
        if isinstance(status_value, int):
            # Map int to enum
            status_map = {
                0: ExecutionStatus.UNSPECIFIED,
                1: ExecutionStatus.EXECUTING,
                2: ExecutionStatus.COMPLETED,
                3: ExecutionStatus.FAILED,
                4: ExecutionStatus.CANCELLED,
                5: ExecutionStatus.TIMEOUT,
            }
            status = status_map.get(status_value, ExecutionStatus.UNSPECIFIED)
        elif isinstance(status_value, str):
            # Already a string, try to match enum
            try:
                status = ExecutionStatus(status_value)
            except ValueError:
                # Fallback for common aliases
                status_map = {
                    "COMPLETED": ExecutionStatus.COMPLETED,
                    "FAILED": ExecutionStatus.FAILED,
                }
                status = status_map.get(status_value, ExecutionStatus.UNSPECIFIED)
        else:
            status = ExecutionStatus.UNSPECIFIED

        return cls(
            transaction_id=data.get("transaction_id", ""),
            transaction_hash=data.get("transaction_hash", ""),
            status=status,
            result_summary=data.get("result_summary", ""),
            state_root_before=data.get("state_root_before", ""),
            state_root_after=data.get("state_root_after", ""),
            executed_at_unix_ms=int(data.get("executed_at_unix_ms", 0)),
            signer_key_id=data.get("signer_key_id", ""),
            signature=data.get("signature", ""),
        )


class ScoreDetails(BaseModel):
    """Typed details for Score evaluation results."""
    model_config = ConfigDict(extra="ignore")

    # Common evaluation metrics
    error_message: str = ""
    error_type: str = ""
    validation_errors: list[str] = Field(default_factory=list)
    
    # Benchmark-specific details can be added as extra fields
    benchmark_specific: dict[str, Any] = Field(default_factory=dict)


class TaskMetadata(BaseModel):
    """Typed metadata for Task objects."""
    model_config = ConfigDict(extra="ignore")

    benchmark: str = ""
    category: str = ""
    difficulty: str = ""
    tags: list[str] = Field(default_factory=list)
    # IFEval-specific fields
    instruction_id_list: list[str] = Field(default_factory=list)
    kwargs: list[dict[str, Any]] = Field(default_factory=list)
    # Other benchmark-specific data
    benchmark_specific: dict[str, Any] = Field(default_factory=dict)


class AggregateMetadata(BaseModel):
    """Typed metadata for Aggregate results."""
    model_config = ConfigDict(extra="ignore")

    suite_version: str = ""
    operator_version: str = ""
    test_timestamp: str = ""
    environment_info: dict[str, Any] = Field(default_factory=dict)
