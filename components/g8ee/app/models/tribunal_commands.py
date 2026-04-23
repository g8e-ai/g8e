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

from app.constants import CommandGenerationOutcome, TribunalMember, TieBreakReason, AuditorReason
from app.models.agents.tribunal import CandidateCommand, VoteBreakdown

from .base import Field as BaseField, G8eIdentifiableModel, UTCDatetime


class TribunalCommandRequestContext(G8eIdentifiableModel.model.__base__):
    request: str = Field(..., description="Caller's natural-language request that seeded the Tribunal")
    guidelines: str = Field(default="", description="Caller's optional guidelines on command shape passed to the Tribunal")
    model: str | None = Field(default=None, description="LLM model used for Tribunal generation")
    num_passes: int | None = Field(default=None, description="Number of generation passes performed (default 5, one per Tribunal member)")


class TribunalCommandGenerationResult(G8eIdentifiableModel.model.__base__):
    final_command: str | None = Field(default=None, description="Command string produced by the Tribunal pipeline; null when outcome=consensus_failed")
    outcome: CommandGenerationOutcome = Field(..., description="Tribunal pipeline outcome")
    vote_winner: str | None = Field(default=None, description="Command string that won the uniform majority vote; null when outcome=consensus_failed")
    vote_score: float | None = Field(default=None, description="Fraction of members who voted for the winner (winner_count / total_members); null when outcome=consensus_failed")


class TribunalCommandAuditor(G8eIdentifiableModel.model.__base__):
    auditor_passed: bool | None = Field(default=None, description="True if the auditor approved the vote winner (or an equivalent dissenter cluster)")
    auditor_revision: str | None = Field(default=None, description="Revised command produced by the auditor when auditor_passed=false")
    auditor_reason: AuditorReason | None = Field(default=None, description="The auditor's stated reason")
    swap_to_cluster: str | None = Field(default=None, description="Opaque cluster id the Auditor swapped to (set only on reason=swapped_to_dissenter)")
    swap_to_member: str | None = Field(default=None, description="TribunalMember id resolved from swap_to_cluster")


class TribunalCommandPipelineMetadata(G8eIdentifiableModel.model.__base__):
    consensus_confidence: str | None = Field(default=None, description="Qualitative consensus confidence level derived from quantitative metrics")
    execution_duration_ms: int | None = Field(default=None, description="Total Tribunal pipeline execution duration in milliseconds")
    stage_1_duration_ms: int | None = Field(default=None, description="Stage 1 (generation) duration in milliseconds")
    stage_2_duration_ms: int | None = Field(default=None, description="Stage 2 (voting) duration in milliseconds")
    stage_3_duration_ms: int | None = Field(default=None, description="Stage 3 (verification) duration in milliseconds")


class TribunalCommandErrorContext(G8eIdentifiableModel.model.__base__):
    error_type: str | None = Field(default=None, description="Type of Tribunal error that occurred")
    error_message: str | None = Field(default=None, description="Human-readable error message")
    pass_errors: list[str] = Field(default_factory=list, description="Per-pass error messages when multiple passes fail")


class TribunalCommand(G8eIdentifiableModel):
    """Full Tribunal command generation record with investigation reference.

    Captures all Tribunal pipeline metadata including candidate commands,
    voting breakdown, auditor decisions, and final outcome. Links to an
    investigation via investigation_id for audit and analytics.
    """
    investigation_id: str = Field(..., description="Associated investigation ID. Links this Tribunal command to the investigation that triggered it")
    case_id: str = Field(..., description="Associated case ID. Denormalized for query efficiency without investigation lookup")
    created_at: UTCDatetime = Field(..., description="When the Tribunal command generation was initiated")
    updated_at: UTCDatetime | None = Field(default=None, description="When the record was last updated")
    request_context: TribunalCommandRequestContext = Field(..., description="Request context that seeded the Tribunal")
    generation_result: TribunalCommandGenerationResult = Field(..., description="Generation result from the Tribunal pipeline")
    candidates: list[CandidateCommand] = Field(default_factory=list, description="All candidate commands generated by Tribunal members")
    vote_breakdown: VoteBreakdown | None = Field(default=None, description="Full per-member vote attribution, dissent clusters, and tie-break record")
    auditor: TribunalCommandAuditor | None = Field(default=None, description="Auditor pass results when enabled")
    pipeline_metadata: TribunalCommandPipelineMetadata | None = Field(default=None, description="Additional pipeline execution metadata")
    error_context: TribunalCommandErrorContext | None = Field(default=None, description="Error information when Tribunal pipeline fails")


__all__ = [
    "TribunalCommand",
    "TribunalCommandRequestContext",
    "TribunalCommandGenerationResult",
    "TribunalCommandAuditor",
    "TribunalCommandPipelineMetadata",
    "TribunalCommandErrorContext",
]
