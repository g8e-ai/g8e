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

Implements the five-member heterogeneous AI panel (The Tribunal) as the sole
authority for shell command generation. Sage (the primary LLM) never proposes
commands directly; Sage sends the Tribunal a natural-language `request` and
optional `guidelines`, and the Tribunal produces the command.

Pipeline (fires only for run_commands_with_operator):

  1. Generation  — N independent passes each read Sage's request + guidelines
                   plus the operator context. Each pass uses a different Tribunal
                   member persona to surface ideologically distinct candidates.

  2. Voting      — Candidates are normalised (strip whitespace, drop markdown
                   fences) and grouped by exact value. Each unique string receives
                   a weight equal to the sum of position-decay weights of its
                   occurrences: weight[i] = 1 / (pass_index + 1). The string with
                   the highest aggregate weight becomes the vote winner.

  3. Verification — A separate fast Tribunal call (the Verifier) evaluates the
                    winner against Sage's request + guidelines. It responds with
                    either the literal string "ok" or a short revised command.
                    A non-ok response without a valid revision raises.

Failure modes (all raise — there is no fallback command because Sage never
proposed one):

  - TribunalDisabledError          — llm_command_gen_enabled=False
  - TribunalModelNotConfiguredError — neither assistant_model nor primary_model set
  - TribunalProviderUnavailableError — provider init failed
  - TribunalSystemError             — all passes failed with system errors
  - TribunalGenerationFailedError   — all passes failed for non-system reasons
  - TribunalVerifierFailedError     — verifier returned unusable output

Configuration:

  llm_command_gen_passes    — Number of Tribunal generation passes (default: 3)
  llm_command_gen_verifier  — "true"/"false" to enable/disable verifier (default: true)
  llm_command_gen_enabled   — master switch; when False the tool errors

Tribunal behavioral diversity comes from the ideological voice of each persona
(Axiom minimalist, Concord guardian, Variance exhaustive, Pragma conventional,
Nemesis adversary), not from numerical parameters.
    """

import asyncio
import logging
import shlex
from collections import defaultdict
from typing import List, NoReturn, Optional

from app.errors import OllamaEmptyResponseError
from app.models.settings import LLMSettings, G8eeUserSettings
from app.models.base import G8eBaseModel
from app.models.http_context import G8eHttpContext
from app.models.agent import OperatorContext
from app.constants import (
    CommandGenerationOutcome,
    ComponentName,
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    FORBIDDEN_COMMAND_PATTERNS,
    TribunalMember,
    EventType,
    VerifierReason,
)
from app.llm.factory import get_llm_provider
from app.llm.llm_types import Content, Part, Role, LiteLLMSettings, ResponseFormat
from app.llm.provider import LLMProvider
from app.models.agents.tribunal import (
    CandidateCommand,
    CommandGenerationResult,
    TribunalDisabledError,
    TribunalSystemError,
    TribunalProviderUnavailableError,
    TribunalGenerationFailedError,
    TribunalModelNotConfiguredError,
    TribunalVerifierFailedError,
    TribunalPassCompletedPayload,
    TribunalVerifierStartedPayload,
    TribunalVerifierCompletedPayload,
    TribunalSessionStartedPayload,
    TribunalSessionDisabledPayload,
    TribunalSessionModelNotConfiguredPayload,
    TribunalSessionProviderUnavailablePayload,
    TribunalSessionSystemErrorPayload,
    TribunalSessionGenerationFailedPayload,
    TribunalSessionVerifierFailedPayload,
    TribunalVotingCompletedPayload,
    TribunalSessionCompletedPayload,
)
from app.models.events import SessionEvent
from app.models.model_configs import get_model_config
from app.services.infra.g8ed_event_service import EventService
from app.utils.agent_persona_loader import get_agent_persona, get_tribunal_member
from app.utils.whitelist_validator import validate_command_against_whitelist
from app.utils.blacklist_validator import validate_command_against_blacklist

logger = logging.getLogger(__name__)


def _format_command_constraints_message(
    whitelisting_enabled: bool,
    blacklisting_enabled: bool,
    whitelisted_commands: List[str] | None,
    blacklisted_commands: List[dict[str, str]] | None,
) -> str:
    """Generate a message describing command constraints for Tribunal prompts."""
    parts: list[str] = []
    
    if whitelisting_enabled and whitelisted_commands:
        parts.append(
            f"COMMAND WHITELIST ACTIVE: Only these {len(whitelisted_commands)} commands are permitted. "
            f"Your proposed command MUST exactly match one of these whitelisted patterns. "
            f"Whitelisted commands: {', '.join(repr(c) for c in whitelisted_commands[:10])}"
            f"{'...' if len(whitelisted_commands) > 10 else ''}."
        )
    
    if blacklisting_enabled and blacklisted_commands:
        parts.append(
            f"COMMAND BLACKLIST ACTIVE: Commands matching these patterns are forbidden. "
            f"Your proposed command MUST NOT match any blacklisted pattern. "
            f"Blacklisted patterns: {', '.join(b.get('pattern', '') for b in blacklisted_commands[:10])}"
            f"{'...' if len(blacklisted_commands) > 10 else ''}."
        )
    
    if not whitelisting_enabled and not blacklisting_enabled:
        parts.append("No whitelist or blacklist constraints are active.")
    
    return " ".join(parts)


def _build_operator_context_string(operator_context: OperatorContext | None) -> str:
    """Build a formatted string of operator context for Tribunal prompts."""
    if not operator_context:
        return "No operator context available"

    parts: list[str] = []
    if operator_context.hostname:
        parts.append(f"Hostname: {operator_context.hostname}")
    if operator_context.os:
        parts.append(f"OS: {operator_context.os}")
    if operator_context.architecture:
        parts.append(f"Architecture: {operator_context.architecture}")
    if operator_context.username:
        uid_suffix = f" (uid={operator_context.uid})" if operator_context.uid is not None else ""
        parts.append(f"User: {operator_context.username}{uid_suffix}")
    if operator_context.shell:
        parts.append(f"Shell: {operator_context.shell}")
    if operator_context.working_directory:
        parts.append(f"Working Directory: {operator_context.working_directory}")
    if operator_context.operator_type:
        parts.append(f"Operator Type: {operator_context.operator_type}")
    if operator_context.is_cloud_operator:
        parts.append("Cloud Operator: Yes")
        if operator_context.cloud_subtype:
            parts.append(f"Cloud Subtype: {operator_context.cloud_subtype}")
        if operator_context.granted_intents:
            parts.append(f"Granted Intents: {operator_context.granted_intents}")
    if operator_context.is_container:
        parts.append("Container Environment: Yes")
        if operator_context.container_runtime:
            parts.append(f"Container Runtime: {operator_context.container_runtime}")
        if operator_context.init_system:
            parts.append(f"Init System: {operator_context.init_system}")
    elif operator_context.init_system:
        parts.append(f"Init System: {operator_context.init_system}")

    return "\n".join(parts) if parts else "No operator details available"


def _format_forbidden_patterns_message() -> str:
    """Generate a message listing all forbidden command patterns.

    Privilege escalation wrappers (sudo, su, pkexec, doas, etc.) are rejected
    unconditionally by `tool_service.execute_tool_call` regardless of uid. The
    platform's contract is that the Operator is launched with whatever privilege
    level it needs; in-command escalation is never permitted. The Tribunal
    must therefore never emit these patterns.
    """
    # Extract unique base patterns (e.g., "sudo" from "sudo", "su " from "su ")
    base_patterns = sorted(set(p.strip() for p in FORBIDDEN_COMMAND_PATTERNS))
    pattern_list = ", ".join(f'"{p}"' for p in base_patterns)
    return (
        f"CRITICAL: NEVER add {pattern_list}, or any privilege escalation wrapper. "
        f"The platform rejects any command containing these tokens regardless of the user's uid. "
        f"If a command requires root, the Operator itself must be launched with sufficient "
        f"privileges — in-command escalation is never accepted."
    )


def _prompt_fields(
    operator_context: OperatorContext | None,
    request: str,
    guidelines: str,
) -> dict[str, str]:
    """Build the common template kwargs used by every Tribunal persona prompt.

    Returns a dict with keys: os, shell, working_directory, user_context,
    operator_context, forbidden_patterns_message, request, guidelines.

    Centralising this avoids each stage re-deriving the same fields and
    guarantees that every persona sees consistent context.
    """
    os_name = (operator_context.os if operator_context else None) or DEFAULT_OS_NAME
    shell = (operator_context.shell if operator_context else None) or DEFAULT_SHELL
    working_directory = (
        operator_context.working_directory if operator_context else None
    ) or DEFAULT_WORKING_DIRECTORY
    username = operator_context.username if operator_context else None
    uid = operator_context.uid if operator_context else None
    if username and uid is not None:
        user_context = f"{username} (uid={uid})"
    else:
        user_context = username or "unknown"
    return {
        "os": os_name,
        "shell": shell,
        "working_directory": working_directory,
        "user_context": user_context,
        "operator_context": _build_operator_context_string(operator_context),
        "forbidden_patterns_message": _format_forbidden_patterns_message(),
        "request": request.strip() if request else "",
        "guidelines": guidelines.strip() if guidelines else "(none)",
    }


_MAX_TOKENS_GENERATION = 256
_MAX_TOKENS_VERIFIER = 256


TRIBUNAL_PROMPT_TEMPLATE = """\
{voice}

<constraints>
{forbidden_patterns_message}

{command_constraints_message}
</constraints>

<request>
{request}
</request>

<guidelines>
{guidelines}
</guidelines>

<system_context>
OS: {os}
Shell: {shell}
User: {user_context}
Working directory: {working_directory}
</system_context>

<operator_context>
{operator_context}
</operator_context>

Respond now with the exact command string and nothing else."""


TRIBUNAL_VERIFIER_TEMPLATE = """\
{voice}

<constraints>
{forbidden_patterns_message}

{command_constraints_message}
</constraints>

<request>
{request}
</request>

<guidelines>
{guidelines}
</guidelines>

<os>
{os}
</os>

<user>
{user_context}
</user>

<operator_context>
{operator_context}
</operator_context>

<candidate_command>
{candidate_command}
</candidate_command>

Respond now with exactly `ok` or the single corrected command, and nothing else."""


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


def _resolve_model(llm: LLMSettings, request: str = "") -> str:
    """Resolve a concrete model string for the Tribunal pipeline.

    Fallback chain: assistant_model -> primary_model.

    Raises TribunalModelNotConfiguredError if neither is set. ``request``
    is threaded through so the exception carries Sage's original ask at
    the raise site (no post-construction mutation by callers).
    """
    if llm.resolved_assistant_model:
        return llm.resolved_assistant_model
    if llm.primary_model:
        return llm.primary_model
    raise TribunalModelNotConfiguredError(
        provider=llm.primary_provider,
        request=request,
    )


_TRIBUNAL_MEMBERS: tuple[TribunalMember, ...] = (
    TribunalMember.AXIOM,
    TribunalMember.CONCORD,
    TribunalMember.VARIANCE,
    TribunalMember.PRAGMA,
    TribunalMember.NEMESIS,
)


def _member_for_pass(pass_index: int) -> TribunalMember:
    """Return the Tribunal member assigned to a given pass index (cycles every 5)."""
    return _TRIBUNAL_MEMBERS[pass_index % len(_TRIBUNAL_MEMBERS)]


def _normalise_command(raw: str) -> str:
    """Extract and normalise command from LLM response with robust parsing.

    Handles:
    1. Markdown code fences (```bash, ```shell, ```sh, ```).
    2. Common prefixes (e.g., "Command:", "The command is:").
    3. Explanatory text after the command.
    4. Preserves intentional multi-line structure (heredocs, pipelines).
    5. Validates shell syntax using shlex.

    Returns the extracted command string or an empty string if invalid.
    """
    if not raw:
        return ""

    cmd = raw.strip()

    # 1. Handle markdown fences
    for fence in ("```bash", "```shell", "```sh", "```"):
        if cmd.startswith(fence):
            cmd = cmd[len(fence):]
            # Strip trailing fence if it exists
            if cmd.endswith("```"):
                cmd = cmd[:-3]
            break
    
    # If no fence was matched but it ends with ```, still strip it
    if cmd.endswith("```"):
        cmd = cmd[:-3]

    cmd = cmd.strip()

    # 2. Handle common prefixes
    prefixes = (
        "command:",
        "the command is:",
        "proposed command:",
        "final command:",
        "revised command:",
        "exact command:",
    )
    lower_cmd = cmd.lower()
    for prefix in prefixes:
        if lower_cmd.startswith(prefix):
            cmd = cmd[len(prefix):].strip()
            break

    # 3. Handle trailing explanatory text
    # If there are multiple lines, and the first line looks like a command
    # but subsequent lines look like explanations, we might need to be careful.
    # However, for Tribunal, we usually expect a single logical command (which could be multi-line).
    
    # 4. Validate shell syntax
    try:
        # shlex.split handles quotes and escapes correctly.
        # It will raise ValueError for unbalanced quotes/escapes.
        shlex.split(cmd)
    except ValueError as exc:
        logger.warning("[TRIBUNAL-NORM] Shell syntax validation failed: %s", exc)
        # If it failed, it might be because of explanatory text after the command.
        # Let's try to take just the first line if it's a simple command.
        lines = cmd.splitlines()
        if len(lines) > 1:
            try:
                shlex.split(lines[0])
                logger.info("[TRIBUNAL-NORM] Falling back to first line for valid shell syntax")
                return lines[0].strip()
            except ValueError:
                pass
        return ""

    return cmd


class TribunalResponse(G8eBaseModel):
    """Structured response for Tribunal command generation."""
    command: str

class TribunalVerifierResponse(G8eBaseModel):
    """Structured response for Tribunal verification."""
    status: str  # "ok" or "revised"
    revised_command: Optional[str] = None

def _validate_command_safety(
    command: str,
    whitelisting_enabled: bool,
    blacklisting_enabled: bool,
    operator_context: OperatorContext | None,
) -> tuple[bool, str | None]:
    """Validate a command against all security constraints.
    
    Checks:
    1. FORBIDDEN_COMMAND_PATTERNS (sudo, su, etc.)
    2. Blacklist (if enabled)
    3. Whitelist (if enabled)
    
    Returns (is_safe, error_message).
    """
    if not command:
        return False, "Empty command"

    # 1. FORBIDDEN_COMMAND_PATTERNS
    for pattern in FORBIDDEN_COMMAND_PATTERNS:
        if pattern in command:
            return False, f"Command contains forbidden pattern: {pattern}"

    # 2. Blacklist
    if blacklisting_enabled:
        blacklist_result = validate_command_against_blacklist(command)
        if not blacklist_result.is_allowed:
            return False, f"Command blocked by blacklist: {blacklist_result.reason}"

    # 3. Whitelist
    if whitelisting_enabled:
        # We need the platform from operator_context
        from app.constants import Platform
        platform = Platform.LINUX
        if operator_context and operator_context.os:
            os_lower = operator_context.os.lower()
            if "windows" in os_lower:
                platform = Platform.WINDOWS
            elif "darwin" in os_lower or "macos" in os_lower:
                platform = Platform.MACOS

        whitelist_result = validate_command_against_whitelist(command, platform=platform)
        if not whitelist_result.is_valid:
            return False, f"Command not whitelisted: {whitelist_result.reason}"

    return True, None

def _weighted_vote(candidates: List[CandidateCommand]) -> tuple[str | None, float]:
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


_TERMINAL_TRIBUNAL_EVENTS: frozenset[EventType] = frozenset({
    EventType.TRIBUNAL_SESSION_STARTED,
    EventType.TRIBUNAL_SESSION_COMPLETED,
    EventType.TRIBUNAL_SESSION_DISABLED,
    EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED,
    EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
    EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
    EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
    EventType.TRIBUNAL_SESSION_VERIFIER_FAILED,
})


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
        """Fire-and-forget SSE event. Swallows errors to prevent pipeline stalls.

        Terminal events (TRIBUNAL_SESSION_*) are critical: they are the only signal
        the frontend receives that the Tribunal has ended. If publish() fails for
        a terminal event, log at CRITICAL and re-raise to ensure the caller is
        aware of the failure. For progress events, log at ERROR and swallow.
        """
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
            if event_type in _TERMINAL_TRIBUNAL_EVENTS:
                logger.critical(
                    "[TRIBUNAL_EMIT] CRITICAL: Failed to emit terminal event %s: %s. "
                    "Frontend will not receive Tribunal completion signal.",
                    event_type, exc
                )
                raise
            logger.error("[TRIBUNAL_EMIT] Failed to emit %s: %s", event_type, exc)


async def _run_generation_pass(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    pass_index: int,
    emitter: TribunalEmitter,
    pass_errors: List[str],
    command_constraints_message: str,
) -> str | None:
    """Run one Tribunal generation pass and return the normalised candidate command.

    Failed passes append their error string to pass_errors so the caller can
    classify the failure mode (system vs. legitimate).
    """
    member = _member_for_pass(pass_index)
    member_persona = get_tribunal_member(member.value)

    fields = _prompt_fields(operator_context, request=request, guidelines=guidelines)
    prompt = TRIBUNAL_PROMPT_TEMPLATE.format(
        voice=member_persona.get_system_prompt(),
        command_constraints_message=command_constraints_message,
        **fields,
    )
    logger.info(
        "[TRIBUNAL-PASS-%d] Member=%s prompt_len=%d user_context=%s os=%s shell=%s working_dir=%s",
        pass_index, member.value, len(prompt),
        fields["user_context"], fields["os"], fields["shell"], fields["working_directory"],
    )
    logger.debug("[TRIBUNAL-PASS-%d] Full prompt: %s", pass_index, prompt[:5000])
    model_config = get_model_config(model)
    
    # Use structured output if the model supports it
    response_format = None
    if model_config.supports_structured_output:
        response_format = ResponseFormat.from_pydantic_schema(
            TribunalResponse.model_json_schema(),
            name="TribunalResponse"
        )

    settings = LiteLLMSettings(
        max_output_tokens=_MAX_TOKENS_GENERATION,
        top_p_nucleus_sampling=model_config.top_p,
        top_k_filtering=model_config.top_k,
        stop_sequences=model_config.stop_sequences,
        system_instructions="",
        response_format=response_format,
    )
    try:
        response = await provider.generate_content_lite(
            model=model,
            contents=[Content(role=Role.USER, parts=[Part.from_text(prompt)])],
            lite_llm_settings=settings,
        )
        if not response.text or not response.text.strip():
            raise OllamaEmptyResponseError(
                f"Provider returned empty response text",
                model=model,
                channel="lite",
                done_reason="stop",
                prompt_eval_count=None,
                eval_count=None,
                num_ctx=0,
                num_predict=0,
                thinking_len=0,
                tool_calls_count=0,
                ctx_overflow_suspected=False,
            )
        
        raw_text = response.text
        if response_format:
            try:
                import json
                data = json.loads(raw_text)
                candidate_raw = data.get("command", "")
            except (json.JSONDecodeError, AttributeError):
                candidate_raw = raw_text
        else:
            candidate_raw = raw_text

        candidate = _normalise_command(candidate_raw)
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(pass_index=pass_index, member=member, candidate=candidate, success=True),
        )
        return candidate
    except OllamaEmptyResponseError as exc:
        error_msg = f"Pass {pass_index} ({member.value}) returned empty response: {exc}"
        pass_errors.append(error_msg)
        logger.warning("[TRIBUNAL] %s", error_msg)
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(pass_index=pass_index, member=member, candidate=None, success=False),
        )
        return None
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


async def _emit_verifier_failed_session(
    emitter: TribunalEmitter,
    request: str,
    reason: VerifierReason,
    error: str | None,
    candidate_command: str,
) -> None:
    """Emit TRIBUNAL_SESSION_VERIFIER_FAILED co-located with the raise site.

    Centralising emission next to the raise keeps all terminal-state events
    symmetric with the other session-failure events (disabled, model-not-
    configured, provider-unavailable, system-error, generation-failed) which
    all fire inside this module rather than in the orchestration caller.
    """
    await emitter.emit(
        EventType.TRIBUNAL_SESSION_VERIFIER_FAILED,
        TribunalSessionVerifierFailedPayload(
            request=request,
            reason=reason,
            error=error,
            candidate_command=candidate_command,
        ),
    )


async def _fail_verifier(
    emitter: TribunalEmitter,
    request: str,
    reason: VerifierReason,
    error_msg: str,
    candidate_command: str,
    error_detail: str | None = None,
) -> NoReturn:
    """Emit verifier failure event and raise TribunalVerifierFailedError.

    Emits TRIBUNAL_SESSION_VERIFIER_FAILED as the sole terminal signal for
    all verifier failure paths. TRIBUNAL_VOTING_REVIEW_COMPLETED is reserved
    for non-terminal outcomes (OK, REVISED) only.
    """
    await _emit_verifier_failed_session(
        emitter, request, reason, error_msg, candidate_command,
    )
    raise TribunalVerifierFailedError(
        reason=reason,
        error=error_msg,
        candidate_command=candidate_command,
        request=request,
    )


async def _run_verifier(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    candidate_command: str,
    operator_context: OperatorContext | None,
    emitter: TribunalEmitter,
    command_constraints_message: str,
) -> tuple[bool, str | None]:
    """Run the Tribunal verifier against the vote winner."""
    await emitter.emit(
        EventType.TRIBUNAL_VOTING_REVIEW_STARTED,
        TribunalVerifierStartedPayload(candidate_command=candidate_command),
    )

    verifier_persona = get_agent_persona("auditor")

    fields = _prompt_fields(operator_context, request=request, guidelines=guidelines)
    prompt = TRIBUNAL_VERIFIER_TEMPLATE.format(
        voice=verifier_persona.get_system_prompt(),
        command_constraints_message=command_constraints_message,
        candidate_command=candidate_command,
        **fields,
    )
    logger.info(
        "[TRIBUNAL-VERIFIER] prompt_len=%d user_context=%s os=%s candidate_command=%s",
        len(prompt), fields["user_context"], fields["os"], candidate_command,
    )
    logger.debug("[TRIBUNAL-VERIFIER] Full prompt: %s", prompt[:5000])
    model_config = get_model_config(model)
    
    # Use structured output if the model supports it
    response_format = None
    if model_config.supports_structured_output:
        response_format = ResponseFormat.from_pydantic_schema(
            TribunalVerifierResponse.model_json_schema(),
            name="TribunalVerifierResponse"
        )

    settings = LiteLLMSettings(
        max_output_tokens=_MAX_TOKENS_VERIFIER,
        top_p_nucleus_sampling=model_config.top_p,
        top_k_filtering=model_config.top_k,
        stop_sequences=model_config.stop_sequences,
        system_instructions="",
        response_format=response_format,
    )
    try:
        response = await provider.generate_content_lite(
            model=model,
            contents=[Content(role=Role.USER, parts=[Part.from_text(prompt)])],
            lite_llm_settings=settings,
        )
        if not response.text or not response.text.strip():
            raise OllamaEmptyResponseError(
                f"Verifier returned empty response text",
                model=model,
                channel="lite",
                done_reason="stop",
                prompt_eval_count=None,
                eval_count=None,
                num_ctx=0,
                num_predict=0,
                thinking_len=0,
                tool_calls_count=0,
                ctx_overflow_suspected=False,
            )
        
        raw_text = response.text.strip()
        status = ""
        revised_raw = ""

        if response_format:
            try:
                import json
                data = json.loads(raw_text)
                status = data.get("status", "").lower()
                revised_raw = data.get("revised_command", "")
            except (json.JSONDecodeError, AttributeError):
                # Fallback to text parsing
                status = raw_text.lower()
        else:
            status = raw_text.lower()

        if status == "ok" or "ok" in status.split():
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
                TribunalVerifierCompletedPayload(passed=True, reason=VerifierReason.OK),
            )
            return True, None

        # Handle revised command
        if not revised_raw and not response_format:
            # If not using structured output, the whole response might be the revised command
            revised_raw = raw_text

        revised = _normalise_command(revised_raw)
        if revised and revised != candidate_command:
            # VALIDATE REVISION SAFETY
            is_safe, safety_error = _validate_command_safety(
                revised,
                whitelisting_enabled=True, # Always validate safety for revisions
                blacklisting_enabled=True,
                operator_context=operator_context,
            )
            if not is_safe:
                logger.error("[TRIBUNAL] Verifier suggested unsafe revision: %s", safety_error)
                error_msg = f"Verifier suggested unsafe revision: {safety_error}"
                await _fail_verifier(
                    emitter, request, VerifierReason.NO_VALID_REVISION, error_msg, candidate_command,
                )

            logger.info("[TRIBUNAL] Verifier revised command: original=%r revised=%r", candidate_command, revised)
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
                TribunalVerifierCompletedPayload(passed=False, revision=revised, reason=VerifierReason.REVISED),
            )
            return False, revised

        logger.error("[TRIBUNAL] Verifier returned non-ok but no valid revision; cannot verify candidate")
        error_msg = f"Verifier returned non-ok answer without valid revision: {raw_text[:100]}"
        await _fail_verifier(
            emitter, request, VerifierReason.NO_VALID_REVISION, error_msg, candidate_command,
        )

    except TribunalVerifierFailedError:
        raise
    except Exception as exc:
        if isinstance(exc, OllamaEmptyResponseError):
            logger.error("[TRIBUNAL] Verifier returned empty response; cannot verify candidate: %s", exc)
            error_msg = f"Verifier returned empty response: {exc}"
            await _fail_verifier(
                emitter, request, VerifierReason.EMPTY_RESPONSE, error_msg, candidate_command,
            )
        logger.error("[TRIBUNAL] Verifier failed with exception; cannot verify candidate: %s", exc)
        await _fail_verifier(
            emitter, request, VerifierReason.VERIFIER_ERROR, str(exc), candidate_command, error_detail=str(exc),
        )


async def _run_generation_stage(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    num_passes: int,
    emitter: TribunalEmitter,
    command_constraints_message: str,
) -> List[CandidateCommand]:
    """Stage 1: run N parallel generation passes and return successful candidates.

    Raises TribunalSystemError when all passes fail due to system errors, or
    TribunalGenerationFailedError when all passes fail for non-system reasons.
    """
    pass_errors: List[str] = []
    pass_tasks = [
        _run_generation_pass(
            provider=provider, model=model, request=request, guidelines=guidelines,
            operator_context=operator_context, pass_index=i, emitter=emitter, pass_errors=pass_errors,
            command_constraints_message=command_constraints_message,
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
                EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
                TribunalSessionSystemErrorPayload(
                    request=request,
                    pass_errors=pass_errors,
                ),
            )
            raise TribunalSystemError(pass_errors=pass_errors, request=request)

        logger.error("[TRIBUNAL] All generation passes failed for non-system reasons; halting execution")
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
            TribunalSessionGenerationFailedPayload(
                request=request,
                pass_errors=pass_errors if pass_errors else ["No candidates produced"],
            ),
        )
        raise TribunalGenerationFailedError(
            pass_errors=pass_errors if pass_errors else ["No candidates produced"],
            request=request,
        )

    return candidates


async def _run_voting_stage(
    candidates: list[CandidateCommand],
    request: str,
    emitter: TribunalEmitter,
) -> tuple[str, float]:
    """Stage 2: compute weighted majority vote and emit consensus event.

    Returns (vote_winner, vote_score). The caller must guarantee candidates is
    non-empty; an assertion guards against programming errors.
    """
    vote_winner, vote_score = _weighted_vote(candidates)
    assert vote_winner is not None, "vote_winner should not be None after empty candidates check"

    await emitter.emit(
        EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED,
        TribunalVotingCompletedPayload(
            vote_winner=vote_winner,
            vote_score=vote_score,
            num_candidates=len(candidates),
            request=request,
        ),
    )
    return vote_winner, vote_score


async def _run_verification_stage(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    vote_winner: str,
    operator_context: OperatorContext | None,
    verifier_enabled: bool,
    emitter: TribunalEmitter,
    command_constraints_message: str,
) -> tuple[str | None, CommandGenerationOutcome, bool, str | None]:
    """Stage 3: optionally verify the vote winner and determine outcome.

    Returns (final_command, outcome, verifier_passed, verifier_revision).
    """
    if not verifier_enabled:
        return vote_winner, CommandGenerationOutcome.CONSENSUS, True, None

    verifier_passed, verifier_revision = await _run_verifier(
        provider=provider, model=model, request=request, guidelines=guidelines,
        candidate_command=vote_winner, operator_context=operator_context,
        emitter=emitter, command_constraints_message=command_constraints_message,
    )
    if verifier_passed:
        logger.info("[TRIBUNAL] Verifier approved: %r", vote_winner)
        return vote_winner, CommandGenerationOutcome.VERIFIED, True, None

    logger.info("[TRIBUNAL] Verifier revised: %r -> %r", vote_winner, verifier_revision)
    return verifier_revision or vote_winner, CommandGenerationOutcome.VERIFICATION_FAILED, False, verifier_revision


async def _build_and_emit_result(
    request: str,
    guidelines: str,
    final_command: str,
    outcome: CommandGenerationOutcome,
    candidates: list[CandidateCommand],
    vote_winner: str,
    vote_score: float,
    verifier_passed: bool,
    verifier_revision: str | None,
    emitter: TribunalEmitter,
) -> CommandGenerationResult:
    """Stage 4: assemble the result model and emit the session-completed event."""
    result = CommandGenerationResult(
        request=request,
        guidelines=guidelines,
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
            request=request,
            final_command=final_command,
            outcome=outcome,
            vote_score=vote_score,
        ),
    )
    return result


async def generate_command(
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    g8ed_event_service: EventService,
    web_session_id: str,
    user_id: str,
    case_id: str,
    investigation_id: str,
    settings: G8eeUserSettings,
    whitelisting_enabled: bool = False,
    blacklisting_enabled: bool = False,
    whitelisted_commands: list[str] | None = None,
    blacklisted_commands: list[dict[str, str]] | None = None,
) -> CommandGenerationResult:
    """Run the Tribunal pipeline to generate a command from Sage's request.

    The caller never proposes a command directly. `request` is the caller's
    natural-language articulation of what the Operator must accomplish;
    `guidelines` is optional constraints on the shape of the command (not its
    output). The Tribunal is the sole authority on the resulting command string.

    Raises on any failure mode. There is no fallback — Sage did not propose a
    command, so there is nothing to fall back to.
    """
    request = (request or "").strip()
    guidelines = (guidelines or "").strip()
    fields = _prompt_fields(operator_context, request=request, guidelines=guidelines)

    logger.info(
        "[TRIBUNAL-ENTRY] generate_command called: request_len=%d guidelines_len=%d os=%s shell=%s user=%s hostname=%s arch=%s",
        len(request), len(guidelines), fields["os"], fields["shell"], fields["user_context"],
        operator_context.hostname if operator_context else None,
        operator_context.architecture if operator_context else None,
    )

    command_constraints_message = _format_command_constraints_message(
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
        whitelisted_commands=whitelisted_commands,
        blacklisted_commands=blacklisted_commands,
    )
    logger.info(
        "[TRIBUNAL-ENTRY] Command constraints: whitelisting=%s blacklisting=%s whitelist_count=%d blacklist_count=%d",
        whitelisting_enabled, blacklisting_enabled,
        len(whitelisted_commands) if whitelisted_commands else 0,
        len(blacklisted_commands) if blacklisted_commands else 0,
    )
    logger.info(
        "[TRIBUNAL-ENTRY] Settings state: llm_command_gen_enabled=%s llm_command_gen_verifier=%s llm_command_gen_passes=%d assistant_provider=%s assistant_model=%s eval_judge_model=%s",
        settings.llm.llm_command_gen_enabled,
        settings.llm.llm_command_gen_verifier,
        settings.llm.llm_command_gen_passes,
        settings.llm.assistant_provider,
        settings.llm.assistant_model,
        settings.eval_judge.model,
    )

    g8e_context = G8eHttpContext(
        web_session_id=web_session_id,
        user_id=user_id,
        case_id=case_id,
        investigation_id=investigation_id,
        source_component=ComponentName.G8EE,
    )
    emitter = TribunalEmitter(g8ed_event_service, g8e_context)

    if not request:
        raise TribunalGenerationFailedError(
            pass_errors=["Sage submitted an empty request; cannot generate command"],
            request=request,
        )

    if not settings.llm.llm_command_gen_enabled:
        logger.error(
            "[TRIBUNAL] DISABLED via llm_command_gen_enabled=False; cannot produce a command "
            "because Sage never proposes one directly"
        )
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_DISABLED,
            TribunalSessionDisabledPayload(request=request),
        )
        raise TribunalDisabledError(request=request)

    try:
        model = _resolve_model(settings.llm, request=request)
        logger.info("[TRIBUNAL] Model resolved: %s", model)
    except TribunalModelNotConfiguredError as exc:
        logger.error("[TRIBUNAL] Model not configured: %s - assistant_model=%s primary_model=%s", exc, settings.llm.assistant_model, settings.llm.primary_model)
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED,
            TribunalSessionModelNotConfiguredPayload(
                request=request,
                provider=exc.provider,
                error=exc.user_message,
            ),
        )
        raise
    num_passes = max(1, settings.llm.llm_command_gen_passes)
    members = [_member_for_pass(i) for i in range(num_passes)]

    logger.info(
        "[TRIBUNAL] Starting session: provider=%s model=%s passes=%d members=%s request_chars=%d guidelines_chars=%d verifier_enabled=%s",
        settings.llm.assistant_provider, model, num_passes, [m.value for m in members],
        len(request), len(guidelines), settings.llm.llm_command_gen_verifier,
    )

    await emitter.emit(
        EventType.TRIBUNAL_SESSION_STARTED,
        TribunalSessionStartedPayload(
            request=request,
            guidelines=guidelines,
            model=model,
            num_passes=num_passes,
            members=members,
        ),
    )

    try:
        provider = get_llm_provider(settings.llm, is_assistant=True)
    except Exception as exc:
        logger.error("[TRIBUNAL] Provider initialization failed: %s", exc)
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
            TribunalSessionProviderUnavailablePayload(
                request=request,
                provider=settings.llm.assistant_provider,
                error=str(exc),
            ),
        )
        raise TribunalProviderUnavailableError(
            provider=settings.llm.assistant_provider,
            error=str(exc),
            request=request,
        ) from exc

    candidates = await _run_generation_stage(
        provider=provider, model=model, request=request, guidelines=guidelines,
        operator_context=operator_context, num_passes=num_passes, emitter=emitter,
        command_constraints_message=command_constraints_message,
    )

    vote_winner, vote_score = await _run_voting_stage(
        candidates=candidates, request=request, emitter=emitter,
    )

    final_command, outcome, verifier_passed, verifier_revision = await _run_verification_stage(
        provider=provider, model=model, request=request, guidelines=guidelines,
        vote_winner=vote_winner, operator_context=operator_context,
        verifier_enabled=settings.llm.llm_command_gen_verifier,
        emitter=emitter,
        command_constraints_message=command_constraints_message,
    )

    # Ensure final_command is never None — verifier contract guarantees a string,
    # but belt-and-braces against a future refactor leaving verifier_revision empty.
    final_command_str = final_command if final_command is not None else vote_winner

    # FINAL SAFETY VALIDATION
    is_safe, safety_error = _validate_command_safety(
        final_command_str,
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
        operator_context=operator_context,
    )
    if not is_safe:
        logger.error("[TRIBUNAL] Final command safety validation failed: %s", safety_error)
        error_msg = f"Final command safety validation failed: {safety_error}"
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
            TribunalSessionSystemErrorPayload(
                request=request,
                pass_errors=[error_msg],
            ),
        )
        raise TribunalGenerationFailedError(
            pass_errors=[error_msg],
            request=request,
        )

    return await _build_and_emit_result(
        request=request, guidelines=guidelines, final_command=final_command_str,
        outcome=outcome, candidates=candidates, vote_winner=vote_winner,
        vote_score=vote_score, verifier_passed=verifier_passed,
        verifier_revision=verifier_revision, emitter=emitter,
    )
