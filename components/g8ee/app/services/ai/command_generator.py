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
import json
import logging
import shlex
from collections import defaultdict
from typing import Any, List, NoReturn, Optional

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
    TieBreakReason,
    EventType,
    VerifierReason,
)
from app.llm.prompts import (
    build_command_constraints_message,
    build_tribunal_generator_prompt,
    build_tribunal_verifier_prompt,
    build_tribunal_verifier_context,
    build_tribunal_prompt_fields,
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
    TribunalConsensusFailedError,
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
    TribunalConsensusFailedPayload,
    TribunalDissentRecordedPayload,
    TribunalSessionCompletedPayload,
    VoteBreakdown,
)
from app.models.events import SessionEvent
from app.models.model_configs import get_model_config
from app.services.infra.g8ed_event_service import EventService
from app.utils.agent_persona_loader import get_agent_persona, get_tribunal_member
from app.utils.json_utils import extract_json_from_text
from app.utils.whitelist_validator import validate_command_against_whitelist
from app.utils.blacklist_validator import validate_command_against_blacklist


logger = logging.getLogger(__name__)

_MAX_TOKENS_GENERATION = 4096
_MAX_TOKENS_VERIFIER = 1024

_TERMINAL_TRIBUNAL_EVENTS = {
    EventType.TRIBUNAL_SESSION_STARTED,
    EventType.TRIBUNAL_SESSION_COMPLETED,
    EventType.TRIBUNAL_SESSION_DISABLED,
    EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED,
    EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
    EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
    EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
    EventType.TRIBUNAL_SESSION_VERIFIER_FAILED,
}


class TribunalEmitter:
    """Handles emission of Tribunal SSE events via EventService.

    Distinguishes between terminal events (which raise on publish failure)
    and progress events (which swallow failures to avoid blocking the pipeline).
    """

    def __init__(
        self,
        event_service: EventService,
        g8e_context: G8eHttpContext,
    ):
        self.event_service = event_service
        self.g8e_context = g8e_context

    async def emit(self, event_type: EventType, payload: Any) -> None:
        """Emit an SSE event. Re-raises if event_type is terminal."""
        try:
            if self.event_service is None or self.g8e_context is None:
                return
            event = SessionEvent(
                event_type=event_type,
                payload=payload,
                web_session_id=self.g8e_context.web_session_id,
                user_id=self.g8e_context.user_id,
                case_id=self.g8e_context.case_id,
                investigation_id=self.g8e_context.investigation_id,
                source_component=ComponentName.G8EE,
            )
            await self.event_service.publish(event)
        except Exception as exc:
            if event_type in _TERMINAL_TRIBUNAL_EVENTS:
                logger.error("[TRIBUNAL-EMIT] Terminal event %s failed: %s", event_type, exc)
                raise
            logger.warning("[TRIBUNAL-EMIT] Progress event %s failed (swallowed): %s", event_type, exc)

TRIBUNAL_MIN_CONSENSUS = 2


def _is_system_error(error_message: str) -> bool:
    """Classify an error message as a system error vs. a model error.

    System errors are infrastructure failures (auth, network, config) that
    should trigger TribunalSystemError. Model errors are LLM output problems
    that should trigger TribunalGenerationFailedError.
    """
    error_lower = error_message.lower()
    # Safety validation failures are model errors, not system errors
    if "safety validation failed" in error_lower:
        return False
    system_indicators = [
        "401", "403", "unauthorized", "forbidden",
        "authentication", "api key",
        "connection refused", "connectionerror", "timeout",
        "dns", "ssl", "econnrefused",
        "unsupported llm provider",
    ]
    return any(indicator in error_lower for indicator in system_indicators)


def _normalise_command(raw: str) -> str:
    """Normalise a command string by stripping cosmetic differences.

    Normalization steps (applied in order):
    1. Strip markdown code fences
    2. Strip common prefixes
    3. Strip comment lines (# prefix)
    4. Strip shebang lines (#!/bin/bash, etc.)
    5. Strip trailing semicolons
    6. Collapse multiple spaces to single spaces (outside quoted strings)
    7. Strip trailing newlines

    Returns the first line if multi-line with unbalanced quotes, or empty string
    if the command is invalid.
    """
    if not raw:
        return ""

    # Strip markdown code fences
    for fence in ["```bash", "```sh", "```"]:
        if raw.startswith(fence):
            raw = raw[len(fence):].strip()
            if raw.endswith("```"):
                raw = raw[:-3].strip()
            break

    # Strip common prefixes
    prefixes = ["Command:", "The command is:", "Final command:"]
    for prefix in prefixes:
        if raw.startswith(prefix):
            raw = raw[len(prefix):].strip()

    # Strip comment lines (lines starting with #)
    lines = raw.split("\n")
    lines = [line for line in lines if not line.strip().startswith("#")]
    raw = "\n".join(lines).strip()

    # Strip shebang lines
    if raw.startswith("#!"):
        lines = raw.split("\n")
        if len(lines) > 1:
            raw = "\n".join(lines[1:]).strip()
        else:
            raw = ""

    # Strip trailing semicolons
    raw = raw.rstrip(";").strip()

    # Collapse multiple spaces to single spaces (simple version - outside quoted strings)
    # This is a conservative approach; full quoted-string-aware parsing would be more complex
    parts = raw.split()
    raw = " ".join(parts)

    # Strip trailing newlines
    raw = raw.rstrip("\n")

    # Handle multi-line commands (heredocs, etc.)
    lines = raw.split("\n")
    if len(lines) > 1:
        # Check for heredocs first - they must be preserved as multi-line
        if "<<" in raw:
            return raw.strip()
        # Check if the first line has valid shell syntax
        first_line = lines[0].strip()
        try:
            shlex.split(first_line)
            return first_line
        except ValueError:
            # First line invalid, return empty
            return ""

    # Validate shell syntax
    try:
        shlex.split(raw)
        return raw.strip()
    except ValueError:
        return ""


def _validate_command_safety(
    command: str,
    whitelisting_enabled: bool,
    blacklisting_enabled: bool,
    operator_context: OperatorContext | None,
) -> tuple[bool, str | None]:
    """Validate a command against forbidden patterns, whitelist, and blacklist.

    Returns (is_safe, error_message).
    """
    if not command:
        return False, "Empty command"

    # Check forbidden patterns (sudo, etc.)
    for pattern in FORBIDDEN_COMMAND_PATTERNS:
        if pattern in command:
            return False, f"Command contains forbidden pattern: {pattern}"

    # Check blacklist
    if blacklisting_enabled:
        blacklist_result = validate_command_against_blacklist(command)
        is_allowed = blacklist_result.is_allowed
        if not is_allowed:
            return False, f"Command blocked by blacklist: {blacklist_result.reason if blacklist_result else 'Unknown reason'}"

    # Check whitelist
    if whitelisting_enabled:
        whitelist_result = validate_command_against_whitelist(
            command, operator_context.os
        )
        is_valid = whitelist_result.is_valid
        if not is_valid:
            return False, f"Command not whitelisted: {whitelist_result.reason if whitelist_result else 'Unknown reason'}"

    return True, None


async def _fail_verifier(
    emitter: TribunalEmitter,
    request: str,
    reason: VerifierReason,
    error_msg: str,
    candidate_command: str,
) -> NoReturn:
    """Emit a verifier failure event and raise TribunalVerifierFailedError."""
    await emitter.emit(
        EventType.TRIBUNAL_SESSION_VERIFIER_FAILED,
        TribunalSessionVerifierFailedPayload(
            request=request,
            reason=reason,
            error=error_msg,
            candidate_command=candidate_command,
        ),
    )
    raise TribunalVerifierFailedError(
        reason=reason,
        request=request,
        error=error_msg,
        candidate_command=candidate_command,
    )


def _member_for_pass(pass_index: int) -> TribunalMember:
    """Map a pass index to a Tribunal member.

    Uses round-robin through the five members: axiom, concord, variance,
    pragma, nemesis. Nemesis is always included (pass 4 or mod 5).
    """
    members = [
        TribunalMember.AXIOM,
        TribunalMember.CONCORD,
        TribunalMember.VARIANCE,
        TribunalMember.PRAGMA,
        TribunalMember.NEMESIS,
    ]
    return members[pass_index % len(members)]


def _resolve_model(llm_settings: LLMSettings, request: str = "") -> str:
    """Resolve the concrete model string from settings.

    Priority: assistant_model > primary_model. Raises TribunalModelNotConfiguredError
    if neither is set.
    """
    if llm_settings.assistant_model:
        return llm_settings.assistant_model

    if llm_settings.primary_model:
        return llm_settings.primary_model

    provider = llm_settings.primary_provider or llm_settings.assistant_provider
    raise TribunalModelNotConfiguredError(
        provider=provider.value if provider else "unknown",
        request=request,
    )


def _weighted_vote(candidates: list[CandidateCommand], total_members: int) -> tuple[str | None, float, VoteBreakdown, list[CandidateCommand] | None]:
    """Compute uniform-weighted majority vote with deterministic tie-breaking.

    Each member contributes exactly 1 vote per candidate (no position decay).
    Tie-breaker ladder:
      1. Longest command wins (compositional pressure)
      2. Non-Nemesis cluster wins over Nemesis-including cluster
      3. Alphabetical (deterministic fallback)

    consensus_strength is calculated as max_votes / total_members, where total_members
    is the count of members who were asked to produce (not the count of unique candidates).

    Returns (vote_winner, vote_score, vote_breakdown, tied_candidates).
    """
    if not candidates:
        return None, 0.0, VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={},
            winner=None,
            winner_supporters=[],
            dissenters_by_command={},
            consensus_strength=0.0,
            tie_broken=False,
            tie_break_reason=None,
        ), None

    # Group candidates by command (uniform voting - each member = 1 vote)
    candidates_by_command: dict[str, list[str]] = {}
    candidates_by_member: dict[str, str] = {}
    for c in candidates:
        candidates_by_member[c.member.value] = c.command
        if c.command not in candidates_by_command:
            candidates_by_command[c.command] = []
        candidates_by_command[c.command].append(c.member.value)

    # Calculate vote counts (uniform - each member = 1 vote)
    vote_counts = {cmd: len(members) for cmd, members in candidates_by_command.items()}
    max_votes = max(vote_counts.values()) if vote_counts else 0

    # Check consensus threshold
    if max_votes < TRIBUNAL_MIN_CONSENSUS:
        return None, 0.0, VoteBreakdown(
            candidates_by_member=candidates_by_member,
            candidates_by_command=candidates_by_command,
            winner=None,
            winner_supporters=[],
            dissenters_by_command=candidates_by_command,
            consensus_strength=max_votes / total_members if total_members > 0 else 0.0,
            tie_broken=False,
            tie_break_reason=None,
        ), None

    # Find candidates with max votes (potential tie)
    top_candidates = [cmd for cmd, count in vote_counts.items() if count == max_votes]

    if len(top_candidates) == 1:
        winner = top_candidates[0]
        dissenters = {cmd: members for cmd, members in candidates_by_command.items() if cmd != winner}
        return winner, max_votes / total_members, VoteBreakdown(
            candidates_by_member=candidates_by_member,
            candidates_by_command=candidates_by_command,
            winner=winner,
            winner_supporters=candidates_by_command[winner],
            dissenters_by_command=dissenters,
            consensus_strength=max_votes / total_members,
            tie_broken=False,
            tie_break_reason=None,
        ), None

    # Tie detected - apply tie-breaker ladder
    # 1. Shortest command wins (maximizes signal density for approval fatigue reduction)
    shortest_cmd = min(top_candidates, key=len)
    if len(set(len(c) for c in top_candidates)) > 1:
        # Multiple lengths, pick shortest
        winner = shortest_cmd
        dissenters = {cmd: members for cmd, members in candidates_by_command.items() if cmd != winner}
        tied_candidates = [CandidateCommand(command=cmd, pass_index=0, member=TribunalMember.AXIOM) for cmd in top_candidates]
        return winner, max_votes / total_members, VoteBreakdown(
            candidates_by_member=candidates_by_member,
            candidates_by_command=candidates_by_command,
            winner=winner,
            winner_supporters=candidates_by_command[winner],
            dissenters_by_command=dissenters,
            consensus_strength=max_votes / total_members,
            tie_broken=True,
            tie_break_reason=TieBreakReason.SHORTEST,
        ), tied_candidates

    # 2. Non-Nemesis cluster wins over Nemesis-including cluster
    nemesis_members = [TribunalMember.NEMESIS.value]
    non_nemesis_candidates = [
        cmd for cmd in top_candidates
        if not any(m in nemesis_members for m in candidates_by_command[cmd])
    ]
    nemesis_including_candidates = [
        cmd for cmd in top_candidates
        if any(m in nemesis_members for m in candidates_by_command[cmd])
    ]
    if non_nemesis_candidates and nemesis_including_candidates:
        winner = non_nemesis_candidates[0]
        dissenters = {cmd: members for cmd, members in candidates_by_command.items() if cmd != winner}
        tied_candidates = [CandidateCommand(command=cmd, pass_index=0, member=TribunalMember.AXIOM) for cmd in top_candidates]
        return winner, max_votes / total_members, VoteBreakdown(
            candidates_by_member=candidates_by_member,
            candidates_by_command=candidates_by_command,
            winner=winner,
            winner_supporters=candidates_by_command[winner],
            dissenters_by_command=dissenters,
            consensus_strength=max_votes / total_members,
            tie_broken=True,
            tie_break_reason=TieBreakReason.EXCLUDED_NEMESIS,
        ), tied_candidates

    # 3. Alphabetical fallback
    winner = sorted(top_candidates)[0]
    dissenters = {cmd: members for cmd, members in candidates_by_command.items() if cmd != winner}
    tied_candidates = [CandidateCommand(command=cmd, pass_index=0, member=TribunalMember.AXIOM) for cmd in top_candidates]
    return winner, max_votes / total_members, VoteBreakdown(
        candidates_by_member=candidates_by_member,
        candidates_by_command=candidates_by_command,
        winner=winner,
        winner_supporters=candidates_by_command[winner],
        dissenters_by_command=dissenters,
        consensus_strength=max_votes / total_members,
        tie_broken=True,
        tie_break_reason=TieBreakReason.ALPHABETICAL,
    ), tied_candidates


class TribunalResponse(G8eBaseModel):
    """Structured response for Tribunal command generation."""
    command: str


class TribunalVerifierResponse(G8eBaseModel):
    """Structured response for Tribunal verification."""
    status: str  # "ok", "revised", or "swap"
    revised_command: Optional[str] = None
    swap_to_cluster: Optional[str] = None


class VerifierClusterInfo(G8eBaseModel):
    """Internal model for passing cluster info to the verifier prompt."""
    cluster_id: str
    command: str
    support_count: int


class VerifierInput(G8eBaseModel):
    """Internal model for the verifier prompt context."""
    winner: str | None
    mode: str  # "unanimous", "majority", "tied"
    clusters: list[VerifierClusterInfo]


def _parse_verifier_response(
    raw_text: str, 
    mode: str, 
    cluster_ids: list[str]
) -> tuple[str, str | None, str | None]:
    """Parse and validate verifier JSON response with mode-specific rules."""
    data = extract_json_from_text(raw_text)
    if not data:
        raise ValueError("Verifier returned invalid JSON format")

    try:
        status = data.get("status", "").lower()
        revised_raw = data.get("revised_command")
        swap_to_cluster = data.get("swap_to_cluster")
        
        # Mode-specific validation
        if mode == "unanimous":
            if status not in ("ok", "revised"):
                raise ValueError(f"invalid status {status!r} for mode {mode!r}")
            if swap_to_cluster:
                raise ValueError(f"Verifier returned swap_to_cluster in {mode!r} mode")
        elif mode == "tied":
            if status == "ok":
                raise ValueError(f"invalid status {status!r} for mode {mode!r} (must disambiguate)")
            if status not in ("swap", "revised"):
                raise ValueError(f"invalid status {status!r} for mode {mode!r}")
        elif mode == "majority":
            if status not in ("ok", "swap", "revised"):
                raise ValueError(f"invalid status {status!r} for mode {mode!r}")

        if status == "swap":
            if not swap_to_cluster:
                raise ValueError("Verifier returned status='swap' but no swap_to_cluster")
            if swap_to_cluster not in cluster_ids:
                raise ValueError(f"invalid swap_to_cluster {swap_to_cluster!r}")
        
        if status == "revised" and not revised_raw:
            raise ValueError("Verifier returned status='revised' but no revised_command")
            
        return status, revised_raw, swap_to_cluster
    except (AttributeError) as exc:
        raise ValueError(f"Verifier returned malformed JSON structure: {str(exc)}") from exc


async def _run_verifier(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    mode: str,
    vote_winner: str | None,
    vote_breakdown: VoteBreakdown,
    tied_candidates: list[CandidateCommand] | None,
    operator_context: OperatorContext | None,
    emitter: TribunalEmitter,
    command_constraints_message: str,
    verifier_persona: TribunalMember,
) -> tuple[bool, str | None, str | None, VerifierReason, str | None, str | None]:
    """Run the dissent-aware Verifier."""
    # Prepare cluster info and mapping
    clusters: list[VerifierClusterInfo] = []
    cluster_to_cmd: dict[str, str] = {}
    cluster_to_members: dict[str, list[str]] = {}
    
    # 1. Add winner as cluster_a
    if not vote_winner:
        raise ValueError("vote_winner is required - no fallback allowed")
    target_cmd = vote_winner

    cluster_to_cmd["cluster_a"] = target_cmd
    cluster_to_members["cluster_a"] = vote_breakdown.candidates_by_command[target_cmd]
    clusters.append(VerifierClusterInfo(
        cluster_id="cluster_a",
        command=target_cmd,
        support_count=len(cluster_to_members["cluster_a"])
    ))

    # 2. Add other clusters
    idx = 1
    for cmd, members in vote_breakdown.candidates_by_command.items():
        if cmd == target_cmd:
            continue
        c_id = f"cluster_{chr(ord('a') + idx)}"
        cluster_to_cmd[c_id] = cmd
        cluster_to_members[c_id] = members
        clusters.append(VerifierClusterInfo(
            cluster_id=c_id,
            command=cmd,
            support_count=len(members)
        ))
        idx += 1

    verifier_input = VerifierInput(winner=target_cmd, mode=mode, clusters=clusters)
    
    await emitter.emit(
        EventType.TRIBUNAL_VOTING_REVIEW_STARTED,
        TribunalVerifierStartedPayload(candidate_command=target_cmd),
    )

    fields = build_tribunal_prompt_fields(
        operator_context,
        request=request,
        guidelines=guidelines,
        default_os=DEFAULT_OS_NAME,
        default_shell=DEFAULT_SHELL,
        default_working_directory=DEFAULT_WORKING_DIRECTORY,
    )
    
    # Prepare clusters for the prompt context function
    clusters_data = [
        {"cluster_id": c.cluster_id, "command": c.command, "support_count": c.support_count}
        for c in clusters
    ]
    
    verifier_context = build_tribunal_verifier_context(mode, target_cmd, clusters_data)
    prompt = build_tribunal_verifier_prompt(
        request=request,
        guidelines=guidelines,
        forbidden_patterns_message=fields["forbidden_patterns_message"],
        command_constraints_message=command_constraints_message,
        os=fields["os"],
        user_context=fields["user_context"],
        operator_context_str=fields["operator_context"],
        verifier_context=verifier_context,
    )

    logger.info("[TRIBUNAL-VERIFIER] mode=%s winner=%r clusters=%d", mode, target_cmd, len(clusters))
    model_config = get_model_config(model)
    
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
        system_instructions=verifier_persona.get_system_prompt(),
        response_format=response_format,
    )

    max_attempts = 2
    last_error = None
    raw_text = None

    for attempt in range(max_attempts):
        current_prompt = prompt
        if attempt > 0:
            current_prompt += "\n\nIMPORTANT: Your previous response was not valid JSON. Respond with ONLY a valid JSON object. No Markdown fences, no prose, no preamble."
            logger.info("[TRIBUNAL-VERIFIER] Retry attempt %d for mode=%s", attempt + 1, mode)

        try:
            response = await provider.generate_content_lite(
                model=model,
                contents=[Content(role=Role.USER, parts=[Part.from_text(current_prompt)])],
                lite_llm_settings=settings,
            )
            raw_text = (response.text or "").strip()
            if not raw_text:
                raise OllamaEmptyResponseError(
                    "Provider returned empty response",
                    model=model,
                    channel="lite",
                    done_reason=None,
                    prompt_eval_count=None,
                    eval_count=None,
                    num_ctx=0,
                    num_predict=0,
                    thinking_len=0,
                    tool_calls_count=0,
                    ctx_overflow_suspected=False,
                )
            
            status, revised_raw, swap_to_cluster = _parse_verifier_response(
                raw_text, mode, list(cluster_to_cmd.keys())
            )

            if status == "ok":
                await emitter.emit(
                    EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
                    TribunalVerifierCompletedPayload(passed=True, reason=VerifierReason.OK),
                )
                return True, target_cmd, None, VerifierReason.OK, None, None

            if status == "swap":
                final_cmd = cluster_to_cmd[swap_to_cluster]
                swap_to_member = cluster_to_members[swap_to_cluster][0] # Pick first member for telemetry
                
                # RE-VALIDATE SWAP TARGET SAFETY
                is_safe, safety_err = _validate_command_safety(final_cmd, True, True, operator_context)
                if not is_safe:
                    await _fail_verifier(emitter, request, VerifierReason.NO_VALID_REVISION, f"Swap target unsafe: {safety_err}", target_cmd)

                await emitter.emit(
                    EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
                    TribunalVerifierCompletedPayload(
                        passed=True, 
                        reason=VerifierReason.SWAPPED_TO_DISSENTER,
                        swap_to_cluster=swap_to_cluster,
                        swap_to_member=swap_to_member
                    ),
                )
                return True, final_cmd, None, VerifierReason.SWAPPED_TO_DISSENTER, swap_to_cluster, swap_to_member

            # Handle revised
            revised = _normalise_command(revised_raw)
            if not revised:
                await _fail_verifier(emitter, request, VerifierReason.NO_VALID_REVISION, "Empty revision", target_cmd)

            is_safe, safety_err = _validate_command_safety(revised, True, True, operator_context)
            if not is_safe:
                await _fail_verifier(emitter, request, VerifierReason.NO_VALID_REVISION, f"Revision unsafe: {safety_err}", target_cmd)

            reason = VerifierReason.REVISED_FROM_DISSENT if mode in ("majority", "tied") else VerifierReason.REVISED
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED,
                TribunalVerifierCompletedPayload(passed=False, revision=revised, reason=reason),
            )
            return False, revised, revised, reason, None, None

        except (ValueError, OllamaEmptyResponseError) as exc:
            last_error = exc
            logger.warning("[TRIBUNAL-VERIFIER] Attempt %d failed: %s (raw_text=%r)", attempt + 1, exc, raw_text[:200])
            if attempt == max_attempts - 1:
                # Final attempt failed
                if isinstance(exc, OllamaEmptyResponseError):
                    await _fail_verifier(emitter, request, VerifierReason.EMPTY_RESPONSE, str(exc), target_cmd)
                else:
                    await _fail_verifier(emitter, request, VerifierReason.NO_VALID_REVISION, f"Failed to parse verifier response after {max_attempts} attempts: {str(exc)}", target_cmd)
            continue
        except TribunalVerifierFailedError:
            raise
        except Exception as exc:
            logger.error("[TRIBUNAL-VERIFIER] Unexpected error: %s (raw_text=%r)", exc, raw_text[:200] if raw_text else "None", exc_info=True)
            await _fail_verifier(emitter, request, VerifierReason.VERIFIER_ERROR, str(exc), target_cmd)

    # Fallback (should be covered by final attempt check above)
    raise TribunalVerifierFailedError(
        reason=VerifierReason.VERIFIER_ERROR,
        request=request,
        error=f"Exhausted {max_attempts} attempts without success. Last error: {last_error}",
        candidate_command=target_cmd,
    )


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
    """Run a single Tribunal generation pass.

    Returns the normalised command string or None if the pass fails.
    Appends error messages to pass_errors list on failure.
    """
    member = _member_for_pass(pass_index)
    member_persona = get_tribunal_member(member)
    fields = build_tribunal_prompt_fields(
        operator_context,
        request=request,
        guidelines=guidelines,
        default_os=DEFAULT_OS_NAME,
        default_shell=DEFAULT_SHELL,
        default_working_directory=DEFAULT_WORKING_DIRECTORY,
    )

    prompt = build_tribunal_generator_prompt(
        request=request,
        guidelines=guidelines,
        forbidden_patterns_message=fields["forbidden_patterns_message"],
        command_constraints_message=command_constraints_message,
        os=fields["os"],
        shell=fields["shell"],
        user_context=fields["user_context"],
        working_directory=fields["working_directory"],
        operator_context_str=fields["operator_context"],
    )

    logger.info(
        "[TRIBUNAL-PASS] pass=%d member=%s model=%s request_len=%d",
        pass_index, member.value, model, len(request),
    )

    model_config = get_model_config(model)
    
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
        system_instructions=member_persona.get_system_prompt(),
        response_format=response_format,
    )

    try:
        response = await provider.generate_content_lite(
            model=model,
            contents=[Content(role=Role.USER, parts=[Part.from_text(prompt)])],
            lite_llm_settings=settings,
        )
        
        if not response.text or not response.text.strip():
            error_msg = f"Pass {pass_index} ({member.value}): empty response"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s", error_msg)
            return None

        raw_command = response.text.strip()
        
        # If structured output, extract command from JSON
        if model_config.supports_structured_output:
            parsed = extract_json_from_text(raw_command)
            if parsed and isinstance(parsed, dict) and "command" in parsed:
                raw_command = parsed["command"]

        normalised = _normalise_command(raw_command)
        
        if not normalised:
            error_msg = f"Pass {pass_index} ({member.value}): normalisation failed"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s (raw=%r)", error_msg, raw_command[:100])
            return None

        # Validate command safety (forbidden patterns like sudo)
        is_safe, safety_err = _validate_command_safety(normalised, False, False, operator_context)
        if not is_safe:
            error_msg = f"Pass {pass_index} ({member.value}): safety validation failed: {safety_err}"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s", error_msg)
            return None

        logger.info(
            "[TRIBUNAL-PASS] pass=%d member=%s success: cmd=%r",
            pass_index, member.value, normalised[:80],
        )

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(
                pass_index=pass_index,
                member=member,
                candidate=normalised,
                success=True,
            ),
        )

        return normalised

    except OllamaEmptyResponseError as exc:
        error_msg = f"Pass {pass_index} ({member.value}): {str(exc)}"
        pass_errors.append(error_msg)
        logger.error("[TRIBUNAL-PASS] %s", error_msg)
        return None
    except Exception as exc:
        error_msg = f"Pass {pass_index} ({member.value}): {str(exc)}"
        pass_errors.append(error_msg)
        logger.error("[TRIBUNAL-PASS] %s", error_msg, exc_info=True)
        return None


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
    total_members: int,
) -> tuple[str | None, float, VoteBreakdown, list[CandidateCommand] | None]:
    """Stage 2: compute weighted majority vote and emit consensus event.

    Returns (vote_winner, vote_score, vote_breakdown, tied_candidates).
    """
    vote_winner, vote_score, vote_breakdown, tied_candidates = _weighted_vote(candidates, total_members)

    if vote_winner is None:
        # Check if we have a winner but consensus strength is too low
        if vote_breakdown.consensus_strength > 0:
            # consensus_failed due to low strength
            logger.warning("[TRIBUNAL] Consensus strength too low: %.2f < %d members", vote_breakdown.consensus_strength, TRIBUNAL_MIN_CONSENSUS)
            # Log side-by-side candidate comparison for telemetry
            logger.info("[TRIBUNAL-TELEMETRY] Candidate breakdown for consensus failure:")
            for member, cmd in vote_breakdown.candidates_by_member.items():
                logger.info("[TRIBUNAL-TELEMETRY]   %s: %s", member, cmd[:200] + "..." if len(cmd) > 200 else cmd)
        elif vote_breakdown.tie_break_reason == TieBreakReason.VERIFIER_DISAMBIGUATION:
            logger.info("[TRIBUNAL] Voting tied; verifier disambiguation required")
        else:
            logger.warning("[TRIBUNAL] Consensus failed: no agreement among members")
            # Log side-by-side candidate comparison for telemetry
            logger.info("[TRIBUNAL-TELEMETRY] Candidate breakdown for consensus failure:")
            for member, cmd in vote_breakdown.candidates_by_member.items():
                logger.info("[TRIBUNAL-TELEMETRY]   %s: %s", member, cmd[:200] + "..." if len(cmd) > 200 else cmd)
            
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_CONSENSUS_FAILED,
            TribunalConsensusFailedPayload(
                request=request,
                vote_breakdown=vote_breakdown,
            ),
        )
    else:
        await emitter.emit(
            EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED,
            TribunalVotingCompletedPayload(
                vote_winner=vote_winner,
                vote_score=vote_score,
                num_candidates=len(candidates),
                request=request,
                vote_breakdown=vote_breakdown,
            ),
        )
        
        # Emit dissent events for audit
        for cmd, members in vote_breakdown.dissenters_by_command.items():
            await emitter.emit(
                EventType.TRIBUNAL_VOTING_DISSENT_RECORDED,
                TribunalDissentRecordedPayload(
                    request=request,
                    losing_command=cmd,
                    dissenting_member_ids=members,
                    winner=vote_winner,
                    vote_breakdown=vote_breakdown,
                )
            )

    return vote_winner, vote_score, vote_breakdown, tied_candidates




async def _run_verification_stage(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    vote_winner: str | None,
    vote_breakdown: VoteBreakdown,
    operator_context: OperatorContext | None,
    verifier_enabled: bool,
    emitter: TribunalEmitter,
    command_constraints_message: str,
    tied_candidates: list[CandidateCommand] | None = None,
) -> tuple[str | None, CommandGenerationOutcome, bool, str | None, VerifierReason]:
    """Stage 3: optionally verify the vote winner and determine outcome.

    Returns (final_command, outcome, verifier_passed, verifier_revision, verifier_reason).
    """
    if not verifier_enabled:
        return vote_winner, CommandGenerationOutcome.CONSENSUS, True, None, VerifierReason.OK

    # Determine mode based on consensus strength and tie-break reason
    if vote_breakdown.consensus_strength == 1.0:
        mode = "unanimous"
    elif vote_breakdown.tie_break_reason == TieBreakReason.VERIFIER_DISAMBIGUATION:
        mode = "tied"
    else:
        mode = "majority"

    verifier_passed, final_command, verifier_revision, verifier_reason, swap_to_cluster, swap_to_member = await _run_verifier(
        provider=provider,
        model=model,
        request=request,
        guidelines=guidelines,
        mode=mode,
        vote_winner=vote_winner,
        vote_breakdown=vote_breakdown,
        tied_candidates=tied_candidates,
        operator_context=operator_context,
        emitter=emitter,
        command_constraints_message=command_constraints_message,
        verifier_persona=get_agent_persona("auditor"),
    )

    outcome = CommandGenerationOutcome.VERIFIED if verifier_passed else CommandGenerationOutcome.VERIFICATION_FAILED
    
    # If it was a revision (not a swap or OK), use the legacy REVISED outcome if appropriate
    if not verifier_passed and verifier_reason == VerifierReason.REVISED:
        outcome = CommandGenerationOutcome.VERIFICATION_FAILED

    return final_command, outcome, verifier_passed, verifier_revision, verifier_reason


async def _build_and_emit_result(
    request: str,
    guidelines: str,
    final_command: str | None,
    outcome: CommandGenerationOutcome,
    candidates: list[CandidateCommand],
    vote_winner: str | None,
    vote_score: float | None,
    vote_breakdown: VoteBreakdown | None,
    verifier_passed: bool | None,
    verifier_revision: str | None,
    verifier_reason: VerifierReason | None,
    emitter: TribunalEmitter,
    whitelisting_enabled: bool = False,
    blacklisting_enabled: bool = False,
    whitelisted_commands: list[str] | None = None,
    blacklisted_commands: list[dict[str, str]] | None = None,
    operator_context: OperatorContext | None = None,
) -> CommandGenerationResult:
    """Stage 4: assemble the result model and emit the session-completed event."""
    # Final safety validation check
    is_safe = True
    safety_error = None
    if final_command:
        is_safe, safety_error = _validate_command_safety(
            final_command,
            whitelisting_enabled=whitelisting_enabled,
            blacklisting_enabled=blacklisting_enabled,
            operator_context=operator_context,
        )

    if not is_safe:
        logger.error("[TRIBUNAL] Final command safety validation failed: %s", safety_error)
        outcome = CommandGenerationOutcome.CONSENSUS_FAILED
        final_command = None
        # We don't raise here, we return a CONSENSUS_FAILED result so the UI can show the breakdown
        # and the AI knows the command was rejected for safety.

    result = CommandGenerationResult(
        request=request,
        guidelines=guidelines,
        final_command=final_command,
        outcome=outcome,
        candidates=candidates,
        vote_winner=vote_winner,
        vote_score=vote_score,
        vote_breakdown=vote_breakdown,
        verifier_passed=verifier_passed,
        verifier_revision=verifier_revision,
        verifier_reason=verifier_reason,
    )

    await emitter.emit(
        EventType.TRIBUNAL_SESSION_COMPLETED,
        TribunalSessionCompletedPayload(
            request=request,
            final_command=final_command or "",
            outcome=outcome,
            vote_score=vote_score or 0.0,
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
    fields = build_tribunal_prompt_fields(
        operator_context,
        request=request,
        guidelines=guidelines,
        default_os=DEFAULT_OS_NAME,
        default_shell=DEFAULT_SHELL,
        default_working_directory=DEFAULT_WORKING_DIRECTORY,
    )

    logger.info(
        "[TRIBUNAL-ENTRY] generate_command called: request_len=%d guidelines_len=%d os=%s shell=%s user=%s hostname=%s arch=%s",
        len(request), len(guidelines), fields["os"], fields["shell"], fields["user_context"],
        operator_context.hostname if operator_context else None,
        operator_context.architecture if operator_context else None,
    )

    command_constraints_message = build_command_constraints_message(
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
        provider_str = str(settings.llm.assistant_provider) if settings.llm.assistant_provider else "None"
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
            TribunalSessionProviderUnavailablePayload(
                request=request,
                provider=provider_str,
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

    vote_winner, vote_score, vote_breakdown, tied_candidates = await _run_voting_stage(
        candidates=candidates, request=request, emitter=emitter, total_members=num_passes,
    )

    if vote_winner is None and vote_breakdown.tie_break_reason != TieBreakReason.VERIFIER_DISAMBIGUATION:
        # Consensus failed completely
        await _build_and_emit_result(
            request=request,
            guidelines=guidelines,
            final_command=None,
            outcome=CommandGenerationOutcome.CONSENSUS_FAILED,
            candidates=candidates,
            vote_winner=None,
            vote_score=0.0,
            vote_breakdown=vote_breakdown,
            verifier_passed=None,
            verifier_revision=None,
            verifier_reason=None,
            emitter=emitter,
            whitelisting_enabled=whitelisting_enabled,
            blacklisting_enabled=blacklisting_enabled,
            whitelisted_commands=whitelisted_commands,
            blacklisted_commands=blacklisted_commands,
            operator_context=operator_context,
        )
        raise TribunalConsensusFailedError(request=request, vote_breakdown=vote_breakdown)

    final_command, outcome, verifier_passed, verifier_revision, verifier_reason = await _run_verification_stage(
        provider=provider,
        model=model,
        request=request,
        guidelines=guidelines,
        vote_winner=vote_winner,
        vote_breakdown=vote_breakdown,
        operator_context=operator_context,
        verifier_enabled=settings.llm.llm_command_gen_verifier,
        emitter=emitter,
        command_constraints_message=command_constraints_message,
        tied_candidates=tied_candidates,
    )

    return await _build_and_emit_result(
        request=request,
        guidelines=guidelines,
        final_command=final_command,
        outcome=outcome,
        candidates=candidates,
        vote_winner=vote_winner,
        vote_score=vote_score,
        vote_breakdown=vote_breakdown,
        verifier_passed=verifier_passed,
        verifier_revision=verifier_revision,
        verifier_reason=verifier_reason,
        emitter=emitter,
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
        whitelisted_commands=whitelisted_commands,
        blacklisted_commands=blacklisted_commands,
        operator_context=operator_context,
    )
