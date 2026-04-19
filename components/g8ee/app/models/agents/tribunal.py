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
    TribunalFallbackReason,
    TribunalMember,
    VerifierReason,
)


class TribunalSystemError(Exception):
    """Raised when all Tribunal passes fail due to system errors.

    System errors include authentication failures, network errors, and
    configuration problems. These are distinct from legitimate model
    disagreement and must halt command execution. There is no fallback:
    Sage never proposes a command, so a failed Tribunal means there is
    no command to execute.
    """

    def __init__(self, pass_errors: list[str], request: str) -> None:
        self.pass_errors = pass_errors
        self.request = request
        super().__init__(
            f"All Tribunal passes failed due to system errors: {pass_errors}"
        )


class TribunalProviderUnavailableError(Exception):
    """Raised when the LLM provider cannot be initialized.

    This indicates a configuration problem (invalid provider, missing
    credentials, or unsupported provider) and must halt execution.
    """

    def __init__(self, provider: str, error: str, request: str) -> None:
        self.provider = provider
        self.error = error
        self.request = request
        super().__init__(
            f"Tribunal provider unavailable ({provider}): {error}"
        )


class TribunalGenerationFailedError(Exception):
    """Raised when all generation passes fail for non-system reasons.

    This indicates legitimate model failures (hallucinations, refusals,
    rate limits not classified as system errors) and must halt execution.
    """

    def __init__(self, pass_errors: list[str], request: str) -> None:
        self.pass_errors = pass_errors
        self.request = request
        super().__init__(
            f"All Tribunal generation passes failed: {pass_errors}"
        )


class TribunalVerifierFailedError(Exception):
    """Raised when the verifier fails and cannot validate the candidate.

    This includes empty responses, exceptions, or non-ok answers without
    valid revisions. The verifier's failure means we cannot trust the
    candidate and must halt execution.
    """

    def __init__(self, reason: str, error: str | None, candidate_command: str) -> None:
        self.reason = reason
        self.error = error
        self.candidate_command = candidate_command
        super().__init__(
            f"Tribunal verifier failed ({reason}): {error or 'no error details'}"
        )


class TribunalModelNotConfiguredError(Exception):
    """Raised when no model is configured for the Tribunal pipeline.

    This occurs when neither assistant_model nor primary_model is set
    in the LLM settings. The Tribunal must refuse to run rather than
    silently guessing a provider-specific default.
    """

    def __init__(self, provider: str, request: str = "") -> None:
        self.provider = provider
        self.request = request
        super().__init__(
            f"Tribunal model not configured for provider {provider}: "
            "set assistant_model or primary_model in LLM settings"
        )


class TribunalDisabledError(Exception):
    """Raised when the Tribunal pipeline is disabled but a command was requested.

    Sage never proposes commands directly; the Tribunal is the sole command
    authority. When the Tribunal is disabled, `run_commands_with_operator`
    cannot produce a command and must fail loudly rather than silently.
    """

    def __init__(self, request: str = "") -> None:
        self.request = request
        super().__init__(
            "Tribunal is disabled (llm_command_gen_enabled=False) but Sage requested a "
            "command. Enable the Tribunal or disable the run_commands_with_operator tool."
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


class CommandGenerationResult(G8eBaseModel):
    """Result of the Tribunal command generation pipeline for a single tool call."""
    request: str = Field(description="Sage's natural-language request that seeded the Tribunal")
    guidelines: str = Field(default="", description="Sage's optional creative guidelines passed to the Tribunal")
    final_command: str = Field(description="Command string produced by the Tribunal pipeline")
    outcome: CommandGenerationOutcome
    candidates: list[CandidateCommand] = Field(
        default_factory=list,
        description="All candidate strings generated by the Tribunal passes",
    )
    vote_winner: str | None = Field(
        default=None,
        description="Command string that won the weighted majority vote",
    )
    vote_score: float | None = Field(
        default=None,
        ge=0.0,
        le=1.0,
        description="Normalised vote weight of the winner (0.0-1.0)",
    )
    verifier_passed: bool | None = Field(
        default=None,
        description="True if the verifier approved the vote winner",
    )
    verifier_revision: str | None = Field(
        default=None,
        description="Revised command produced by the verifier when verifier_passed=False",
    )
    verifier_reason: str | None = Field(default=None, description="The verifier's stated reason.")


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


class TribunalSessionStartedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_STARTED events."""
    request: str
    guidelines: str = ""
    model: str
    num_passes: int = Field(ge=1)
    members: list[TribunalMember]


class TribunalFallbackPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_FALLBACK_TRIGGERED events.

    Emitted only for terminal Tribunal errors (provider unavailable, all
    passes failed, etc.). There is no fallback command — the tool fails.
    """
    reason: TribunalFallbackReason
    request: str
    error: str | None = None
    pass_errors: list[str] | None = None


class TribunalVotingCompletedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_VOTING_CONSENSUS_REACHED events."""
    vote_winner: str
    vote_score: float
    num_candidates: int = Field(ge=0)
    request: str


class TribunalSessionCompletedPayload(G8eBaseModel):
    """SSE payload for TRIBUNAL_SESSION_COMPLETED events."""
    request: str
    final_command: str
    outcome: CommandGenerationOutcome
    vote_score: float

