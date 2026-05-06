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

from __future__ import annotations

import hashlib
import hmac
import logging
import time
from typing import TYPE_CHECKING, NoReturn

from app.errors import OllamaEmptyResponseError
from app.models.base import G8eBaseModel
from app.models.agent import OperatorContext
from app.models.reputation import GENESIS_PREV_ROOT, ReputationCommitment
from app.services.data.reputation_data_service import ReputationDataService
from app.utils.merkle import leaf_bytes, merkle_root
from app.constants import (
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    EventType,
    AuditorReason,
)
from app.constants.status import CommandErrorType
from app.llm.prompts import (
    build_tribunal_auditor_prompt,
    build_tribunal_auditor_context,
    build_tribunal_prompt_fields,
)
from app.llm.llm_types import Content, Part, Role, ResponseFormat
from app.llm.provider import LLMProvider
from app.models.agents.tribunal import (
    CandidateCommand,
    AuditorClusterInfo,
    TribunalAuditorFailedError,
    TribunalAuditorStartedPayload,
    TribunalAuditorCompletedPayload,
    TribunalAuditorFailedPayload,
    VoteBreakdown,
)
from app.utils.agent_persona_loader import AgentPersona
from app.models.model_configs import get_model_config

if TYPE_CHECKING:
    from app.services.ai.generator import TribunalEmitter
from app.utils.json_utils import extract_json_from_text
from app.utils.safety import validate_command_safety

# Internal import for normalisation
from app.utils.command import normalise_command

logger = logging.getLogger(__name__)

class TribunalAuditorResponse(G8eBaseModel):
    """Structured response for Tribunal audit."""
    status: str  # "ok", "revised", or "swap"
    revised_command: str | None = None
    swap_to_cluster: str | None = None

# AuditorClusterInfo moved to app.models.agents.tribunal

class AuditorInput(G8eBaseModel):
    """Internal model for the auditor prompt context."""
    winner: str | None
    mode: str  # "unanimous", "majority", "tied"
    clusters: list[AuditorClusterInfo]

async def fail_auditor(
    emitter: TribunalEmitter,
    request: str,
    reason: AuditorReason,
    error_msg: str,
    candidate_command: str,
) -> NoReturn:
    """Emit an auditor failure event and raise TribunalAuditorFailedError."""
    await emitter.emit(
        EventType.TRIBUNAL_SESSION_AUDITOR_FAILED,
        TribunalAuditorFailedPayload(
            request=request,
            reason=reason,
            error=error_msg,
            candidate_command=candidate_command,
        ),
    )
    raise TribunalAuditorFailedError(
        reason=reason,
        request=request,
        error=error_msg,
        candidate_command=candidate_command,
    )

def parse_auditor_response(
    raw_text: str,
    mode: str,
    cluster_ids: list[str]
) -> tuple[str, str | None, str | None]:
    """Parse and validate auditor JSON response with mode-specific rules."""
    data = extract_json_from_text(raw_text)
    if not isinstance(data, dict):
        raise ValueError("Auditor returned invalid JSON format (expected dictionary)")

    try:
        status_raw = data.get("status")
        status = str(status_raw).lower() if status_raw is not None else ""
        revised_raw = data.get("revised_command")
        swap_to_cluster = data.get("swap_to_cluster")

        # Convert revised_raw and swap_to_cluster to str | None explicitly if needed, 
        # but pydantic/type-hints will handle it if they are already strings or None.
        revised_cmd = str(revised_raw) if revised_raw is not None else None
        swap_id = str(swap_to_cluster) if swap_to_cluster is not None else None

        # Mode-specific validation
        if mode == "unanimous":
            if status not in ("ok", "revised"):
                raise ValueError(f"invalid status {status!r} for mode {mode!r}")
            if swap_to_cluster:
                raise ValueError(f"Auditor returned swap_to_cluster in {mode!r} mode")
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
                raise ValueError("Auditor returned status='swap' but no swap_to_cluster")
            if swap_to_cluster not in cluster_ids:
                raise ValueError(f"invalid swap_to_cluster {swap_to_cluster!r}")

        if status == "revised" and not revised_cmd:
            raise ValueError("Auditor returned status='revised' but no revised_command")

        return status, revised_cmd, swap_id
    except (AttributeError, TypeError) as exc:
        raise ValueError(f"Auditor returned malformed JSON structure: {exc!s}") from exc

def build_auditor_prompt(
    request: str,
    guidelines: str,
    mode: str,
    target_cmd: str,
    clusters: list[AuditorClusterInfo],
    operator_context: OperatorContext | None,
    command_constraints_message: str,
) -> str:
    """Build the prompt for the Auditor."""
    fields = build_tribunal_prompt_fields(
        operator_context,
        request=request,
        guidelines=guidelines,
        default_os=DEFAULT_OS_NAME,
        default_shell=DEFAULT_SHELL,
        default_working_directory=DEFAULT_WORKING_DIRECTORY,
    )

    clusters_data = [
        {"cluster_id": c.cluster_id, "command": c.command, "support_count": c.support_count}
        for c in clusters
    ]

    auditor_context = build_tribunal_auditor_context(mode, target_cmd, clusters_data)
    return build_tribunal_auditor_prompt(
        request=request,
        guidelines=guidelines,
        forbidden_patterns_message=fields["forbidden_patterns_message"],
        command_constraints_message=command_constraints_message,
        os=fields["os"],
        user_context=fields["user_context"],
        operator_context_str=fields["operator_context"],
        auditor_context=auditor_context,
    )

async def call_auditor_llm(
    provider: LLMProvider,
    model: str,
    prompt: str,
    auditor_persona: AgentPersona,
    attempt: int = 0,
) -> str:
    """Execute the LLM call for the Auditor."""
    model_config = get_model_config(model)

    response_format = None
    if model_config.supports_structured_output:
        response_format = ResponseFormat.from_pydantic_schema(
            TribunalAuditorResponse.model_json_schema(),
            name="TribunalAuditorResponse"
        )

    settings = model_config.to_litellm_settings(
        system_instructions=auditor_persona.get_system_prompt() or "",
        response_format=response_format,
    )

    if settings is None:
        raise ValueError(f"Failed to generate LiteLLM settings for model {model}")

    current_prompt = prompt
    if attempt > 0:
        current_prompt += "\n\nIMPORTANT: Your previous response was not valid JSON. Respond with ONLY a valid JSON object. No Markdown fences, no prose, no preamble."

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
    return raw_text

async def run_auditor(
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
    auditor_persona: AgentPersona,
    whitelisting_enabled: bool = False,
    blacklisting_enabled: bool = False,
) -> tuple[bool, str | None, str | None, AuditorReason, str | None, str | None]:
    """Run the dissent-aware Auditor (Deprecated: Use stage orchestrator instead)."""
    # Prepare cluster info and mapping
    clusters: list[AuditorClusterInfo] = []
    cluster_to_cmd: dict[str, str] = {}
    cluster_to_members: dict[str, list[str]] = {}

    # 1. Add winner as cluster_a
    if not vote_winner:
        raise ValueError("vote_winner is required")
    target_cmd = vote_winner

    cluster_to_cmd["cluster_a"] = target_cmd
    cluster_to_members["cluster_a"] = vote_breakdown.candidates_by_command[target_cmd]
    clusters.append(AuditorClusterInfo(
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
        clusters.append(AuditorClusterInfo(
            cluster_id=c_id,
            command=cmd,
            support_count=len(members)
        ))
        idx += 1

    correlation_id = getattr(emitter, "correlation_id", None)

    await emitter.emit(
        EventType.TRIBUNAL_VOTING_AUDIT_STARTED,
        TribunalAuditorStartedPayload(candidate_command=target_cmd),
        correlation_id=correlation_id,
    )

    auditor_start_time = time.time()
    prompt = build_auditor_prompt(
        request=request,
        guidelines=guidelines,
        mode=mode,
        target_cmd=target_cmd,
        clusters=clusters,
        operator_context=operator_context,
        command_constraints_message=command_constraints_message,
    )

    max_attempts = 2
    last_error = None
    raw_text = None

    for attempt in range(max_attempts):
        try:
            raw_text = await call_auditor_llm(provider, model, prompt, auditor_persona, attempt)
            status, revised_raw, swap_to_cluster = parse_auditor_response(
                raw_text, mode, list(cluster_to_cmd.keys())
            )

            if status == "ok":
                total_duration_ms = (time.time() - auditor_start_time) * 1000
                logger.info("[TRIBUNAL-AUDITOR] Completed with status=ok total_duration_ms=%.2f", total_duration_ms)
                await emitter.emit(
                    EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED,
                    TribunalAuditorCompletedPayload(passed=True, reason=AuditorReason.OK),
                    correlation_id=correlation_id,
                )
                return True, target_cmd, None, AuditorReason.OK, None, None

            if status == "swap" and swap_to_cluster:
                final_cmd = cluster_to_cmd[swap_to_cluster]
                swap_to_member = cluster_to_members[swap_to_cluster][0] # Pick first member for telemetry

                # RE-VALIDATE SWAP TARGET SAFETY (L1 Technical Bedrock)
                safety_result = validate_command_safety(final_cmd, whitelisting_enabled, blacklisting_enabled, operator_context)
                if not safety_result.is_safe:
                    reason = AuditorReason.WHITELIST_VIOLATION if safety_result.error_type == CommandErrorType.WHITELIST_VIOLATION else AuditorReason.NO_VALID_REVISION
                    await fail_auditor(emitter, request, reason, f"Swap target technical safety failure: {safety_result.error_message}", target_cmd)

                total_duration_ms = (time.time() - auditor_start_time) * 1000
                logger.info("[TRIBUNAL-AUDITOR] Completed with status=swap total_duration_ms=%.2f", total_duration_ms)
                await emitter.emit(
                    EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED,
                    TribunalAuditorCompletedPayload(
                        passed=True,
                        reason=AuditorReason.SWAPPED_TO_DISSENTER,
                        swap_to_cluster=swap_to_cluster,
                        swap_to_member=swap_to_member
                    ),
                    correlation_id=correlation_id,
                )
                return True, final_cmd, None, AuditorReason.SWAPPED_TO_DISSENTER, swap_to_cluster, swap_to_member

            # Handle revised
            if status == "revised" and revised_raw:
                revised_str = str(revised_raw)
                revised = normalise_command(revised_str)
                if not revised:
                    await fail_auditor(emitter, request, AuditorReason.NO_VALID_REVISION, "Empty revision", target_cmd)

            # RE-VALIDATE REVISION SAFETY (L1 Technical Bedrock)
            # revised is defined if status == "revised" and normalise_command succeeded
            if status == "revised":
                # Ensure revised is bound for safety, though normalise_command check above handles it
                revised_final = locals().get('revised')
                if not revised_final:
                     await fail_auditor(emitter, request, AuditorReason.NO_VALID_REVISION, "Missing revision variable", target_cmd)
                
                safety_result = validate_command_safety(revised_final, whitelisting_enabled, blacklisting_enabled, operator_context)
                if not safety_result.is_safe:
                    reason = AuditorReason.WHITELIST_VIOLATION if safety_result.error_type == CommandErrorType.WHITELIST_VIOLATION else AuditorReason.NO_VALID_REVISION
                    await fail_auditor(emitter, request, reason, f"Revision technical safety failure: {safety_result.error_message}", target_cmd)

                reason = AuditorReason.REVISED_FROM_DISSENT if mode in ("majority", "tied") else AuditorReason.REVISED
                total_duration_ms = (time.time() - auditor_start_time) * 1000
                logger.info("[TRIBUNAL-AUDITOR] Completed with status=revised total_duration_ms=%.2f", total_duration_ms)
                await emitter.emit(
                    EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED,
                    TribunalAuditorCompletedPayload(passed=False, revision=revised_final, reason=reason),
                    correlation_id=correlation_id,
                )
                return False, revised_final, revised_final, reason, None, None

        except (ValueError, OllamaEmptyResponseError) as exc:
            last_error = exc
            logger.warning("[TRIBUNAL-AUDITOR] Attempt %d failed: %s (raw_text=%r)", attempt + 1, exc, raw_text[:200] if raw_text else "None")
            if attempt == max_attempts - 1:
                # Final attempt failed
                total_duration_ms = (time.time() - auditor_start_time) * 1000
                logger.info("[TRIBUNAL-AUDITOR] Failed after %d attempts total_duration_ms=%.2f", max_attempts, total_duration_ms)
                if isinstance(exc, OllamaEmptyResponseError):
                    await fail_auditor(emitter, request, AuditorReason.EMPTY_RESPONSE, str(exc), target_cmd)
                else:
                    await fail_auditor(emitter, request, AuditorReason.NO_VALID_REVISION, f"Failed to parse auditor response after {max_attempts} attempts: {exc!s}", target_cmd)
            continue
        except TribunalAuditorFailedError:
            raise
        except Exception as exc:
            total_duration_ms = (time.time() - auditor_start_time) * 1000
            logger.info("[TRIBUNAL-AUDITOR] Unexpected error total_duration_ms=%.2f", total_duration_ms)
            logger.error("[TRIBUNAL-AUDITOR] Unexpected error: %s (raw_text=%r)", exc, raw_text[:200] if raw_text else "None", exc_info=True)
            await fail_auditor(emitter, request, AuditorReason.AUDITOR_ERROR, str(exc), target_cmd)

    # Fallback (should be covered by final attempt check above)
    raise TribunalAuditorFailedError(
        reason=AuditorReason.AUDITOR_ERROR,
        request=request,
        error=f"Exhausted {max_attempts} attempts without success. Last error: {last_error}",
        candidate_command=target_cmd,
    )


async def commit_reputation(
    reputation_data_service: ReputationDataService,
    tribunal_command_id: str,
    investigation_id: str,
    hmac_key: str,
) -> ReputationCommitment:
    """Compute, sign, and persist a Merkle commitment over the reputation scoreboard.

    GDD §14.4 Artifact B: the Auditor, during its verdict step, binds the
    verdict to a snapshot of `reputation_state` by writing a signed Merkle
    commitment. The commitment chains across verdicts via ``prev_root``
    (deployment-scoped) so any party with the hash-chained history can
    verify the Auditor's claims without re-executing the verdict path.

    This function is single-responsibility by design: it does not emit
    events or populate downstream models. Callers own side-effects.
    """
    if not tribunal_command_id:
        raise ValueError("tribunal_command_id is required")
    if not investigation_id:
        raise ValueError("investigation_id is required")
    if not hmac_key:
        raise ValueError("hmac_key is required")

    states = await reputation_data_service.list_states()
    leaves = [leaf_bytes(s.agent_id, s.scalar) for s in states]
    root = merkle_root(leaves)

    latest = await reputation_data_service.get_latest_commitment()
    prev_root = latest.merkle_root if latest is not None else GENESIS_PREV_ROOT

    signature = hmac.new(
        hmac_key.encode("utf-8"),
        (root + prev_root + tribunal_command_id).encode("utf-8"),
        hashlib.sha256,
    ).hexdigest()

    commitment = ReputationCommitment(
        investigation_id=investigation_id,
        tribunal_command_id=tribunal_command_id,
        merkle_root=root,
        prev_root=prev_root,
        leaves_count=len(states),
        signature=signature,
    )
    return await reputation_data_service.create_commitment(commitment)
