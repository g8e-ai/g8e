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
from app.models.base import G8eBaseModel
from app.constants import (
    CommandGenerationOutcome,
    TieBreakReason,
    TribunalMember,
    VerifierReason,
)


class TribunalError(Exception):
    """Base class for all Tribunal terminal-state failures.

    Every subclass represents a distinct reason the Tribunal could not
    produce a trusted command. There is no fallback — Sage never proposes
    a command directly, so a failed Tribunal means the tool call must
    fail. Callers should catch this base class (rather than each concrete
    subclass) when mapping errors to user-facing tool-call failures, and
    inspect the concrete type only when richer context is required.

    Contract:
      - ``request`` is always set to Sage's original natural-language request
        so UI/log surfaces can show what the Tribunal was asked to do.
      - ``user_message`` is the concise, human-readable string that should
        be surfaced to the LLM and the UI as the tool-call error detail.
      - Subclasses set ``user_message`` via their ``__init__`` and pass it
        to ``super().__init__`` so ``str(exc)`` matches ``exc.user_message``.
    """

    def __init__(self, *, request: str, user_message: str) -> None:
        self.request = request
        self.user_message = user_message
        super().__init__(user_message)


class TribunalConsensusFailedError(TribunalError):
    """Raised when no two Tribunal members agree on a command.

    Emitted when the winning cluster's support is below
    TRIBUNAL_MIN_CONSENSUS. The error message includes a structured
    summary of the candidate set so Sage can decide how to proceed.
    """

    def __init__(self, request: str, vote_breakdown: "VoteBreakdown") -> None:
        self.vote_breakdown = vote_breakdown
        
        # Build a structured summary of candidates by member for the LLM
        candidates = [
            f"- {member}: {cmd}" 
            for member, cmd in vote_breakdown.candidates_by_member.items()
        ]
        summary = "\n".join(candidates)
        
        user_message = (
            "Tribunal consensus failed: no two members agreed on a command. "
            f"Candidates produced:\n{summary}\n\n"
            "Please rephrase your request, provide more explicit guidelines, "
            "or choose one of the candidates above if appropriate."
        )
        super().__init__(request=request, user_message=user_message)


class TribunalSystemError(TribunalError):
    """Raised when all Tribunal passes fail due to system errors.

    System errors include authentication failures, network errors, and
    configuration problems. These are distinct from legitimate model
    disagreement and must halt command execution.
    """

    def __init__(self, pass_errors: list[str], request: str) -> None:
        self.pass_errors = pass_errors
        super().__init__(
            request=request,
            user_message=f"Tribunal system error: {'; '.join(pass_errors)}",
        )


class TribunalProviderUnavailableError(TribunalError):
    """Raised when the LLM provider cannot be initialized.

    This indicates a configuration problem (invalid provider, missing
    credentials, or unsupported provider) and must halt execution.
    """

    def __init__(self, provider: str, error: str, request: str) -> None:
        self.provider = provider
        self.error = error
        super().__init__(
            request=request,
            user_message=f"Tribunal provider unavailable ({provider}): {error}",
        )


class TribunalGenerationFailedError(TribunalError):
    """Raised when all generation passes fail for non-system reasons.

    This indicates legitimate model failures (hallucinations, refusals,
    rate limits not classified as system errors) and must halt execution.
    """

    def __init__(self, pass_errors: list[str], request: str) -> None:
        self.pass_errors = pass_errors
        super().__init__(
            request=request,
            user_message=f"Tribunal generation failed: {'; '.join(pass_errors)}",
        )


class TribunalVerifierFailedError(TribunalError):
    """Raised when the verifier fails and cannot validate the candidate.

    This includes empty responses, exceptions, or non-ok answers without
    valid revisions. The verifier's failure means we cannot trust the
    candidate and must halt execution.
    """

    def __init__(
        self,
        reason: VerifierReason,
        error: str | None,
        candidate_command: str,
        request: str,
    ) -> None:
        self.reason = reason
        self.error = error
        self.candidate_command = candidate_command
        super().__init__(
            request=request,
            user_message=(
                f"Tribunal verifier failed ({reason.value}): "
                f"{error or 'no error details'}"
            ),
        )


class TribunalModelNotConfiguredError(TribunalError):
    """Raised when no model is configured for the Tribunal pipeline.

    This occurs when neither assistant_model nor primary_model is set
    in the LLM settings. The Tribunal must refuse to run rather than
    silently guessing a provider-specific default.
    """

    def __init__(self, provider: str, request: str) -> None:
        self.provider = provider
        super().__init__(
            request=request,
            user_message=(
                f"Tribunal model not configured for provider {provider}: "
                "set assistant_model or primary_model in LLM settings"
            ),
        )


class TribunalDisabledError(TribunalError):
    """Raised when the Tribunal pipeline is disabled but a command was requested.

    Sage never proposes commands directly; the Tribunal is the sole command
    authority. When the Tribunal is disabled, `run_commands_with_operator`
    cannot produce a command and must fail loudly rather than silently.
    """

    def __init__(self, request: str) -> None:
        super().__init__(
            request=request,
            user_message=(
                "Tribunal is disabled (llm_command_gen_enabled=False) but Sage "
                "requested a command. Enable the Tribunal or disable the "
                "run_commands_with_operator tool."
            ),
        )


class TribunalMemberResult(G8eBaseModel):
    """The structured output of a single Tribunal member."""
    reasoning: str = Field(description="The logical basis for the member's verdict.")
    command: str = Field(description="The proposed or refined command string.")
    confidence: float = Field(default=1.0, ge=0.0, le=1.0, description="Confidence in the proposal.")


class CandidateCommand(G8eBaseModel):
    """A single command candidate produced by one Tribunal generation pass."""
    command: str
    pass_index: int = Field(ge=0, description="Zero-based index of the generation pass that produced this candidate")
    member: TribunalMember = Field(description="Tribunal member that produced this candidate")
    reasoning: str | None = Field(default=None, description="The reasoning behind this candidate")


class VoteBreakdown(G8eBaseModel):
    """Full attribution of a uniform Tribunal vote.

    Every member contributes exactly one vote to the normalised command
    they produced. This model captures who agreed with whom, how strong
    the consensus was, and (when applicable) how a tie was resolved.
    Consumed by SSE events for audit and future analytics; never used
    for re-weighting.
    """
    candidates_by_member: dict[str, str] = Field(
        default_factory=dict,
        description="member_id -> normalised command string",
    )
    candidates_by_command: dict[str, list[str]] = Field(
        default_factory=dict,
        description="normalised command -> list of member_ids that produced it",
    )
    winner: str | None = Field(
        default=None,
        description="Normalised command that won the vote; None on consensus_failed",
    )
    winner_supporters: list[str] = Field(
        default_factory=list,
        description="member_ids that produced the winner",
    )
    dissenters_by_command: dict[str, list[str]] = Field(
        default_factory=dict,
        description="losing normalised command -> list of member_ids that produced it",
    )
    consensus_strength: float = Field(
        default=0.0,
        ge=0.0,
        le=1.0,
        description="winner_supporters / total_members; 0.0 on consensus_failed",
    )
    tie_broken: bool = Field(
        default=False,
        description="True when more than one cluster held the highest vote and a tie-break rule resolved it",
    )
    tie_break_reason: TieBreakReason | None = Field(
        default=None,
        description="Which tie-break rule resolved the tie (if any)",
    )


class CommandGenerationResult(G8eBaseModel):
    """Result of the Tribunal command generation pipeline for a single tool call.

    `final_command`, `vote_winner`, and `vote_score` are all nullable so
    the result model can faithfully represent a `CONSENSUS_FAILED` outcome
    (no two members agreed and no command was selected). Callers MUST
    inspect `outcome` before consuming the command fields.
    """
    request: str = Field(description="Caller's natural-language request that seeded the Tribunal")
    guidelines: str = Field(default="", description="Caller's optional guidelines on command shape passed to the Tribunal")
    final_command: str | None = Field(
        default=None,
        description="Command string produced by the Tribunal pipeline; None when outcome=CONSENSUS_FAILED",
    )
    outcome: CommandGenerationOutcome
    candidates: list[CandidateCommand] = Field(
        default_factory=list,
        description="All candidate strings generated by the Tribunal passes",
    )
    vote_winner: str | None = Field(
        default=None,
        description="Command string that won the uniform majority vote; None when outcome=CONSENSUS_FAILED",
    )
    vote_score: float | None = Field(
        default=None,
        ge=0.0,
        le=1.0,
        description="Fraction of members who voted for the winner (winner_count / total_members); None when outcome=CONSENSUS_FAILED",
    )
    vote_breakdown: VoteBreakdown | None = Field(
        default=None,
        description="Full per-member vote attribution, dissent clusters, and tie-break record",
    )
    verifier_passed: bool | None = Field(
        default=None,
        description="True if the verifier approved the vote winner (or an equivalent dissenter cluster)",
    )
    verifier_revision: str | None = Field(
        default=None,
        description="Revised command produced by the verifier when verifier_passed=False",
    )
    verifier_reason: VerifierReason | None = Field(default=None, description="The verifier's stated reason.")


class TribunalPassCompletedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_VOTING_PASS_COMPLETED events."""
    pass_index: int = Field(ge=0)
    member: TribunalMember
    candidate: str | None = None
    success: bool = False
    error: str | None = None


class TribunalVerifierStartedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_VOTING_REVIEW_STARTED events."""
    candidate_command: str


class TribunalVerifierCompletedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_VOTING_REVIEW_COMPLETED events."""
    passed: bool
    revision: str | None = None
    reason: VerifierReason
    error: str | None = None
    swap_to_cluster: str | None = Field(
        default=None,
        description="Opaque cluster id the Verifier swapped to (set only on reason=swapped_to_dissenter)",
    )
    swap_to_member: str | None = Field(
        default=None,
        description="TribunalMember id resolved from swap_to_cluster (internal mapping, safe to surface downstream)",
    )


class TribunalSessionStartedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_STARTED events."""
    request: str
    guidelines: str = ""
    model: str
    num_passes: int = Field(ge=1)
    members: list[TribunalMember]


class TribunalSessionDisabledPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_DISABLED.

    Emitted when the operator has disabled the Tribunal
    (llm_command_gen_enabled=False) but Sage requested a command.
    No work was attempted; the tool call fails with a refusal.
    """
    request: str


class TribunalSessionModelNotConfiguredPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED.

    Emitted when neither assistant_model nor primary_model is set for
    the Tribunal's provider. No work was attempted; configuration must
    be fixed before the tool can run.
    """
    request: str
    provider: str
    error: str


class TribunalSessionProviderUnavailablePayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE.

    Emitted when the LLM provider failed to initialize (invalid
    credentials, unsupported provider, etc.). Infrastructure-level
    failure distinct from per-pass generation errors.
    """
    request: str
    provider: str
    error: str


class TribunalSessionSystemErrorPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_SYSTEM_ERROR.

    Emitted when every generation pass failed with a system-class error
    (auth, network, configuration). Distinguishes infrastructure
    problems from legitimate model disagreement.
    """
    request: str
    pass_errors: list[str]


class TribunalSessionGenerationFailedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_GENERATION_FAILED.

    Emitted when every generation pass failed for non-system reasons
    (refusals, hallucinations, rate limits not classified as system
    errors). The model side is the problem, not the infrastructure.
    """
    request: str
    pass_errors: list[str]


class TribunalSessionVerifierFailedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_VERIFIER_FAILED.

    Emitted when voting produced a candidate but the verifier rejected
    it and produced no valid revision. The tool call fails because no
    trusted command was produced.
    """
    request: str
    reason: VerifierReason
    error: str | None = None
    candidate_command: str


class TribunalVotingCompletedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_VOTING_CONSENSUS_REACHED events.

    Backward-compatible fields (`vote_winner`, `vote_score`,
    `num_candidates`) remain alongside the new `vote_breakdown` so
    existing downstream consumers do not break.
    """
    vote_winner: str
    vote_score: float
    num_candidates: int = Field(ge=0)
    request: str
    vote_breakdown: VoteBreakdown


class TribunalConsensusFailedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_VOTING_CONSENSUS_FAILED events.

    Emitted when the winning cluster's support is below
    TRIBUNAL_MIN_CONSENSUS (no two members agreed). The verifier is
    skipped; Sage receives the full candidate set and decides whether
    to rephrase, ask the user for clarification, or abort.
    """
    request: str
    vote_breakdown: VoteBreakdown
    reason: str = Field(
        default="no_agreement",
        description="Short machine-readable reason (always 'no_agreement' today)",
    )


class TribunalDissentRecordedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_VOTING_DISSENT_RECORDED events.

    Fired once per losing cluster (distinct normalised command that
    lost the vote). Audit / analytics only; does not affect execution.
    Both `losing_command` and `dissenting_member_ids` are required
    and must be non-empty — these events are never emitted for empty
    clusters.
    """
    request: str
    losing_command: str = Field(
        min_length=1,
        description="The cluster's normalised command string",
    )
    dissenting_member_ids: list[str] = Field(
        min_length=1,
        description="Tribunal member ids that produced the losing command",
    )
    winner: str = Field(description="The winning command, for context")
    vote_breakdown: VoteBreakdown


class TribunalSessionCompletedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_COMPLETED events."""
    request: str
    final_command: str
    outcome: CommandGenerationOutcome
    vote_score: float

