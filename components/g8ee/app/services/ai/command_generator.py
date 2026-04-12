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

"""
Tribunal Command Generator

Implements the three-member heterogeneous AI panel (The Tribunal) for
syntactically precise command generation.

Pipeline (fires only for run_commands_with_operator):

  1. Generation  — N independent passes produce candidate command strings for
                   the same intent + context. Higher temperature encourages
                   diverse candidates.

  2. Voting      — Candidates are normalised (strip trailing whitespace / newlines)
                   and grouped by exact value. Each unique string receives a weight
                   equal to the sum of position-decay weights of its occurrences:
                   weight[i] = 1 / (pass_index + 1), so earlier passes carry more
                   weight when the Tribunal agrees quickly. The string with the
                   highest aggregate weight becomes the vote winner.

  3. Verification — A separate fast Tribunal call (the verifier) evaluates the
                    winner strictly against the original intent and reports either
                    "ok" or a short revised command. If the verifier signals a
                    problem it returns the revised string; we use that instead and
                    record VERIFICATION_FAILED.

  4. Fallback     — If the Tribunal produces no consensus (all candidates unique,
                    or total weight tie, or any unrecoverable error) the original
                    Large LLM command is used unchanged and FALLBACK is recorded.

The Large LLM is never involved in this pipeline: it already proposed the
command through the normal ReAct loop. The Tribunal only refines the syntactic
accuracy of that proposal.

Configuration:

  llm_command_gen_passes    — Number of Tribunal generation passes (default: 3)
  llm_command_gen_verifier  — "true"/"false" to enable/disable verifier (default: true)
  LLM_COMMAND_GEN_ENABLED   — "true"/"false" master switch (default: true)

Temperatures are fixed per Tribunal member and are sourced from shared/constants/agents.json:

  Axiom    (pass 0, cycles) — 0.0  (fully deterministic, statistical probability and resource efficiency)
  Concord  (pass 1, cycles) — 0.4  (moderate determinism with ethical flexibility)
  Variance (pass 2, cycles) — 0.8  (high creativity and intentional unpredictability)

  Verifier (arbitrator)      — 0.0  (deterministic evaluation)
    """

import asyncio
import logging
from collections import defaultdict

from app.models.settings import LLMSettings, G8eeUserSettings
from app.models.base import G8eBaseModel
from app.models.http_context import G8eHttpContext
from app.constants import (
    CommandGenerationOutcome,
    ComponentName,
    LLMProvider as LLMProviderEnum,
    TribunalFallbackReason,
    TribunalMember,
    ThinkingLevel,
    TRIBUNAL_MEMBER_TEMPERATURES,
    EventType,
    VerifierReason,
    OPENAI_DEFAULT_MODEL,
    OLLAMA_DEFAULT_MODEL,
    ANTHROPIC_DEFAULT_MODEL,
    GEMINI_DEFAULT_MODEL,
)
from app.llm.factory import get_llm_provider
from app.llm.llm_types import Content, GenerateContentConfig, Part, Role, ThinkingConfig, LiteLLMSettings
from app.llm.provider import LLMProvider
from app.models.agents.tribunal import (
    CandidateCommand,
    CommandGenerationResult,
    TribunalMemberResult,
    TribunalSystemError,
    TribunalProviderUnavailableError,
    TribunalGenerationFailedError,
    TribunalVerifierFailedError,
    TribunalPassCompletedPayload,
    TribunalVerifierStartedPayload,
    TribunalVerifierCompletedPayload,
    TribunalSessionStartedPayload,
    TribunalFallbackPayload,
    TribunalVotingCompletedPayload,
    TribunalSessionCompletedPayload,
)
from app.models.model_configs import get_lowest_thinking_level
from app.models.events import SessionEvent
from app.services.infra.g8ed_event_service import EventService

logger = logging.getLogger(__name__)

_GENERATION_PROMPT_TEMPLATE = """\
You are a shell command specialist. Given the user intent and system context below, \
output ONLY the exact shell command string to run — no explanation, no markdown fences, \
no extra text. The command must be syntactically correct and immediately executable.

<example>
Intent: list all running processes sorted by memory usage
OS: linux
Shell: bash
Working directory: /home/user
Original command: ps aux --sort=-%mem
Output only the command string: ps aux --sort=-%mem | head -20
</example>

<intent>
{intent}
</intent>

<system_context>
OS: {os}
Shell: {shell}
Working directory: {working_directory}
</system_context>

<original_command>
{original_command}
</original_command>

Output only the command string:"""

_VERIFIER_PROMPT_TEMPLATE = """\
You are a strict shell command syntax verifier. Evaluate the candidate command \
against the stated intent and operating system.

Rules:
- If the command is syntactically correct and fulfils the intent, respond with exactly: ok
- If there is a syntax error, wrong quoting, wrong escaping, or a missing flag that \
  would cause the command to fail, respond with the corrected command string only — \
  no explanation, no markdown fences.

<intent>
{intent}
</intent>

<os>
{os}
</os>

<candidate_command>
{candidate_command}
</candidate_command>

Respond with exactly "ok" or the corrected command:"""

_MAX_TOKENS_GENERATION = 256
_MAX_TOKENS_VERIFIER = 256


_PROVIDER_DEFAULT_MODELS: dict[LLMProviderEnum, str] = {
    LLMProviderEnum.OLLAMA: OLLAMA_DEFAULT_MODEL,
    LLMProviderEnum.OPENAI: OPENAI_DEFAULT_MODEL,
    LLMProviderEnum.ANTHROPIC: ANTHROPIC_DEFAULT_MODEL,
    LLMProviderEnum.GEMINI: GEMINI_DEFAULT_MODEL,
}


_SYSTEM_ERROR_PATTERNS: tuple[str, ...] = (
    "401",
    "403",
    "authentication",
    "unauthorized",
    "forbidden",
    "api key",
    "api_key",
    "invalid key",
    "connection refused",
    "connection error",
    "connectionerror",
    "timeout",
    "timed out",
    "name resolution",
    "dns",
    "ssl",
    "certificate",
    "unsupported llm provider",
    "no such host",
    "network",
    "econnrefused",
    "enotfound",
)


def _is_system_error(error_message: str) -> bool:
    """Return True if the error message indicates a system-level failure.

    System errors are infrastructure problems (auth, network, config) that
    affect all passes equally. Distinguishing these from legitimate model
    disagreement prevents the Tribunal from silently falling back to the
    original command when the LLM is misconfigured.
    """
    lower = error_message.lower()
    return any(pattern in lower for pattern in _SYSTEM_ERROR_PATTERNS)


def _resolve_model(llm: LLMSettings) -> str:
    """Resolve a concrete model string for the Tribunal pipeline.

    Fallback chain: assistant_model -> primary_model -> provider default.
    """
    if llm.assistant_model:
        return llm.assistant_model
    if llm.primary_model:
        return llm.primary_model
    return _PROVIDER_DEFAULT_MODELS.get(llm.provider, OLLAMA_DEFAULT_MODEL)


_TRIBUNAL_MEMBERS: tuple[TribunalMember, ...] = (
    TribunalMember.AXIOM,
    TribunalMember.CONCORD,
    TribunalMember.VARIANCE,
)


def _member_for_pass(pass_index: int) -> TribunalMember:
    """Return the Tribunal member assigned to a given pass index (cycles every 3)."""
    return _TRIBUNAL_MEMBERS[pass_index % len(_TRIBUNAL_MEMBERS)]


def _temperature_for_pass(pass_index: int) -> float:
    """Return the canonical temperature for the member assigned to a given pass."""
    return TRIBUNAL_MEMBER_TEMPERATURES[_member_for_pass(pass_index)]


def _build_thinking_config(model_name: str) -> ThinkingConfig:
    """Build a ThinkingConfig appropriate for the given model.

    Models that support thinking use their lowest supported level to minimise
    latency for this fast-path pipeline. Models without thinking support get a
    disabled config that providers are expected to ignore.
    """
    level = get_lowest_thinking_level(model_name)
    return ThinkingConfig(
        thinking_level=level,
        include_thoughts=False,
    )


def _normalise_command(raw: str) -> str:
    """Strip surrounding whitespace, trailing newlines, and markdown fences."""
    cmd = raw.strip()
    for fence in ("```bash", "```shell", "```sh", "```"):
        if cmd.startswith(fence):
            cmd = cmd[len(fence):]
            break
    if cmd.endswith("```"):
        cmd = cmd[:-3]
    return cmd.strip()


def _weighted_vote(candidates: list[CandidateCommand]) -> tuple[str | None, float]:
    """
    Compute weighted majority vote over candidate commands.

    Each pass i (0-indexed) contributes weight 1/(i+1) to its candidate.
    Returns (winner_command, normalised_score) or (None, 0.0) if no candidates.
    """
    if not candidates:
        return None, 0.0

    weights: dict[str, float] = defaultdict(float)
    for c in candidates:
        weights[c.command] += 1.0 / (c.pass_index + 1)

    if not weights:
        return None, 0.0

    total_weight = sum(weights.values())
    winner = max(weights, key=lambda k: weights[k])
    normalised = weights[winner] / total_weight if total_weight > 0 else 0.0
    return winner, normalised


class TribunalEmitter:
    """Helper for emitting Tribunal SSE events with consistent context."""

    def __init__(
        self,
        event_service: EventService | None,
        g8e_context: G8eHttpContext | None,
    ):
        self.svc = event_service
        self.ctx = g8e_context

    async def emit(self, event_type: EventType, payload: G8eBaseModel) -> None:
        """Fire-and-forget SSE event. Swallows errors to prevent pipeline stalls."""
        if not self.svc or not self.ctx or not self.ctx.web_session_id or not self.ctx.user_id:
            return
        try:
            await self.svc.publish(
                SessionEvent(
                    event_type=event_type,
                    payload=payload,
                    web_session_id=self.ctx.web_session_id,
                    user_id=self.ctx.user_id,
                    case_id=self.ctx.case_id,
                    investigation_id=self.ctx.investigation_id,
                )
            )
        except Exception as exc:
            logger.error("[TRIBUNAL_EMIT] Failed to emit %s: %s", event_type, exc)


async def _run_generation_pass(
    provider: LLMProvider,
    model: str,
    intent: str,
    original_command: str,
    os_name: str,
    shell: str,
    working_directory: str,
    pass_index: int,
    emitter: TribunalEmitter,
    pass_errors: list[str],
) -> str | None:
    """Run one Tribunal generation pass and return the normalised candidate command.

    Failed passes append their error string to pass_errors so the caller can
    classify the failure mode (system vs. legitimate).
    """
    member = _member_for_pass(pass_index)
    temperature = _temperature_for_pass(pass_index)
    prompt = _GENERATION_PROMPT_TEMPLATE.format(
        intent=intent,
        os=os_name,
        shell=shell,
        working_directory=working_directory,
        original_command=original_command,
    )
    settings = LiteLLMSettings(
        temperature=temperature,
        max_output_tokens=None,
        system_instruction="",
    )
    try:
        response = await provider.generate_content_lite(
            model=model,
            contents=[Content(role=Role.USER, parts=[Part.from_text(prompt)])],
            lite_llm_settings=settings,
        )
        if not response or not response.text:
            error_msg = f"Pass {pass_index} ({member.value}) returned empty response"
            pass_errors.append(error_msg)
            logger.warning("[TRIBUNAL] %s", error_msg)
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
                TribunalPassCompletedPayload(pass_index=pass_index, member=member, candidate=None, success=False),
            )
            return None
        candidate = _normalise_command(response.text)
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(pass_index=pass_index, member=member, candidate=candidate, success=True),
        )
        return candidate
    except Exception as exc:
        error_msg = str(exc)
        pass_errors.append(error_msg)
        if _is_system_error(error_msg):
            logger.error("[TRIBUNAL] Pass %d (%s) system error: %s", pass_index, member, exc)
        else:
            logger.warning("[TRIBUNAL] Pass %d (%s) failed: %s", pass_index, member, exc)
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(pass_index=pass_index, member=member, candidate=None, success=False, error=error_msg),
        )
        return None


async def _run_verifier(
    provider: LLMProvider,
    model: str,
    intent: str,
    candidate_command: str,
    os_name: str,
    emitter: TribunalEmitter,
) -> tuple[bool, str | None]:
    """Run the Tribunal verifier against the vote winner."""
    await emitter.emit(
        EventType.TRIBUNAL_VOTING_REVIEW_STARTED,
        TribunalVerifierStartedPayload(candidate_command=candidate_command),
    )

    prompt = _VERIFIER_PROMPT_TEMPLATE.format(
        intent=intent,
        os=os_name,
        candidate_command=candidate_command,
    )
    settings = LiteLLMSettings(
        temperature=None,
        max_output_tokens=None,
        system_instruction="",
    )
    try:
        response = await provider.generate_content_lite(
            model=model,
            contents=[Content(role=Role.USER, parts=[Part.from_text(prompt)])],
            lite_llm_settings=settings,
        )
        if not response or not response.text:
            logger.error("[TRIBUNAL] Verifier returned empty response; cannot verify candidate")
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
                TribunalVerifierCompletedPayload(passed=False, reason=VerifierReason.EMPTY_RESPONSE),
            )
            raise TribunalVerifierFailedError(
                reason="empty_response",
                error="Verifier returned empty response",
                original_command=candidate_command,
            )

        answer = response.text.strip()
        if answer.lower() == "ok":
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
                TribunalVerifierCompletedPayload(passed=True, reason=VerifierReason.OK),
            )
            return True, None

        revised = _normalise_command(answer)
        if revised and revised != candidate_command:
            logger.info("[TRIBUNAL] Verifier revised command: original=%r revised=%r", candidate_command, revised)
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
                TribunalVerifierCompletedPayload(passed=False, revision=revised, reason=VerifierReason.REVISED),
            )
            return False, revised

        logger.error("[TRIBUNAL] Verifier returned non-ok but no valid revision; cannot verify candidate")
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
            TribunalVerifierCompletedPayload(passed=False, reason=VerifierReason.NO_VALID_REVISION),
        )
        raise TribunalVerifierFailedError(
            reason="no_valid_revision",
            error=f"Verifier returned non-ok answer without valid revision: {answer[:100]}",
            original_command=candidate_command,
        )

    except TribunalVerifierFailedError:
        raise
    except Exception as exc:
        logger.error("[TRIBUNAL] Verifier failed with exception; cannot verify candidate: %s", exc)
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
            TribunalVerifierCompletedPayload(passed=False, reason=VerifierReason.VERIFIER_ERROR, error=str(exc)),
        )
        raise TribunalVerifierFailedError(
            reason="exception",
            error=str(exc),
            original_command=candidate_command,
        )


async def generate_command(
    original_command: str,
    intent: str,
    os_name: str,
    shell: str,
    working_directory: str,
    g8ed_event_service: EventService,
    web_session_id: str,
    user_id: str,
    case_id: str,
    investigation_id: str,
    settings: G8eeUserSettings,
) -> CommandGenerationResult:
    """Run the Tribunal pipeline to refine a command string."""
    g8e_context = G8eHttpContext(
        web_session_id=web_session_id,
        user_id=user_id,
        case_id=case_id,
        investigation_id=investigation_id,
        source_component=ComponentName.G8EE,
    )
    emitter = TribunalEmitter(g8ed_event_service, g8e_context)
    resolved_settings = settings

    if not resolved_settings.llm.llm_command_gen_enabled:
        logger.info("[TRIBUNAL] Disabled via LLM_COMMAND_GEN_ENABLED; using original command")
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_FALLBACK_TRIGGERED,
            TribunalFallbackPayload(reason=TribunalFallbackReason.DISABLED, original_command=original_command, final_command=original_command),
        )
        return CommandGenerationResult(
            original_command=original_command,
            final_command=original_command,
            outcome=CommandGenerationOutcome.DISABLED,
        )

    model = _resolve_model(resolved_settings.llm)
    num_passes = max(1, resolved_settings.llm.llm_command_gen_passes)
    members = [_member_for_pass(i) for i in range(num_passes)]

    logger.info(
        "[TRIBUNAL] Starting session: provider=%s model=%s passes=%d members=%s original_command=%r intent_chars=%d",
        resolved_settings.llm.provider, model, num_passes, [m.value for m in members], original_command[:80], len(intent),
    )

    await emitter.emit(
        EventType.TRIBUNAL_SESSION_STARTED,
        TribunalSessionStartedPayload(
            original_command=original_command,
            model=model,
            num_passes=num_passes,
            members=members,
            os_name=os_name,
            shell=shell,
        ),
    )

    async with get_llm_provider(resolved_settings.llm) as provider:
        # Stage 1: Generation
        pass_errors: list[str] = []
        pass_tasks = [
            _run_generation_pass(
                provider=provider, model=model, intent=intent, original_command=original_command,
                os_name=os_name, shell=shell, working_directory=working_directory,
                pass_index=i, emitter=emitter, pass_errors=pass_errors,
            )
            for i in range(num_passes)
        ]
        raw_results = await asyncio.gather(*pass_tasks, return_exceptions=False)
        candidates = [
            CandidateCommand(command=res, pass_index=i, member=_member_for_pass(i))
            for i, res in enumerate(raw_results) if res
        ]

        if not candidates:
            all_system = pass_errors and all(_is_system_error(e) for e in pass_errors)
            if all_system:
                logger.error(
                    "[TRIBUNAL] All %d generation passes failed due to system errors: %s",
                    num_passes, pass_errors,
                )
                await emitter.emit(
                    EventType.TRIBUNAL_SESSION_FALLBACK_TRIGGERED,
                    TribunalFallbackPayload(
                        reason=TribunalFallbackReason.SYSTEM_ERROR,
                        original_command=original_command,
                        final_command=original_command,
                        pass_errors=pass_errors,
                    ),
                )
                raise TribunalSystemError(
                    pass_errors=pass_errors,
                    original_command=original_command,
                )

            logger.error("[TRIBUNAL] All generation passes failed for non-system reasons; halting execution")
            await emitter.emit(
                EventType.TRIBUNAL_SESSION_FALLBACK_TRIGGERED,
                TribunalFallbackPayload(
                    reason=TribunalFallbackReason.ALL_PASSES_FAILED,
                    original_command=original_command,
                    final_command=original_command,
                    pass_errors=pass_errors if pass_errors else None,
                ),
            )
            raise TribunalGenerationFailedError(
                pass_errors=pass_errors if pass_errors else ["No candidates produced"],
                original_command=original_command,
            )

        # Stage 2: Voting
        vote_winner, vote_score = _weighted_vote(candidates)
        assert vote_winner is not None, "vote_winner should not be None after empty candidates check"

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED,
            TribunalVotingCompletedPayload(vote_winner=vote_winner, vote_score=vote_score, num_candidates=len(candidates), original_command=original_command),
        )

        # Stage 3: Verification
        final_command = vote_winner
        outcome = CommandGenerationOutcome.CONSENSUS
        verifier_passed = True
        verifier_revision = None

        if resolved_settings.llm.llm_command_gen_verifier:
            verifier_passed, verifier_revision = await _run_verifier(
                provider=provider, model=model, intent=intent,
                candidate_command=vote_winner, os_name=os_name, emitter=emitter,
            )
            if verifier_passed:
                logger.info("[TRIBUNAL] Verifier approved: %r", vote_winner)
                outcome = CommandGenerationOutcome.VERIFIED
            elif verifier_revision:
                logger.info("[TRIBUNAL] Verifier revised: %r -> %r", vote_winner, verifier_revision)
                final_command = verifier_revision
                outcome = CommandGenerationOutcome.VERIFICATION_FAILED
            else:
                outcome = CommandGenerationOutcome.VERIFIED
                verifier_passed = True

        # Stage 4: Result & Final Event
        result = CommandGenerationResult(
            original_command=original_command,
            final_command=final_command,
            outcome=outcome,
            candidates=candidates,
            vote_winner=vote_winner,
            vote_score=vote_score,
            verifier_passed=verifier_passed,
            verifier_revision=verifier_revision,
        )

        await emitter.emit(
            EventType.TRIBUNAL_SESSION_COMPLETED,
            TribunalSessionCompletedPayload(
                original_command=original_command,
                final_command=final_command,
                outcome=outcome,
                vote_score=vote_score,
                refined=final_command != original_command,
            ),
        )
        return result
