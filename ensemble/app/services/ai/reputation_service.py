# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 3 reputation writer (GDD §14.5).

Pure-function classifier and EMA helper plus an async dispatcher that:

1. Maps a tribunal verdict (and its execution result, when known) into a
   per-agent ``StakeOutcome`` table keyed by persona id (axiom, concord,
   variance, pragma, nemesis, sage, auditor).
2. EMA-updates each affected ``ReputationState`` row.
3. Writes one ``StakeResolution`` per affected agent for replayability.

The slashing classifier is intentionally side-effect-free so it can be
tested exhaustively against the §14.5 table. The writer is the only side
effect carrier and is invoked from the call site in `agent_tool_loop.py`.

Information Isolation (GDD §3) is preserved: this module reads `reputation_state`
(sole post-execution writer) but is not visible to any persona prompt builder.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from datetime import UTC, datetime

from app.constants import (
    AuditorReason,
    CommandGenerationOutcome,
    ConsensusMember,
    RiskLevel,
)
from app.models.agents.tribunal import CommandGenerationResult
from app.models.base import BaseModel, ConfigDict
from app.models.http_context import RequestContext
from app.models.reputation import (
    ReputationState,
    SlashTier,
    StakeResolution,
)
from app.models.tool_results import CommandExecutionResult
from app.services.data.reputation_data_service import ReputationDataService
from app.services.data.stake_resolution_data_service import (
    StakeResolutionDataService,
    stake_resolution_id,
)

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

DEFAULT_EMA_HALF_LIFE: int = 50
"""Default EMA half-life (number of resolutions for the smoothing weight to
halve). GDD §14.10 suggests this as the start point."""

BOOTSTRAP_SCALAR: float = 0.5
"""Neutral starting scalar for any agent that has no prior `reputation_state`
row."""

TRIBUNAL_HONEST_FOUR: tuple[str, ...] = (
    str(ConsensusMember.AXIOM),
    str(ConsensusMember.CONCORD),
    str(ConsensusMember.VARIANCE),
    str(ConsensusMember.PRAGMA),
)
"""Persona ids for the honest four. Nemesis is excluded - its stake follows a
proper-scoring rule (GDD §5)."""

NEMESIS_ID: str = str(ConsensusMember.NEMESIS)
SAGE_ID: str = "sage"
AUDITOR_ID: str = "auditor"
MARSHAL_ID: str = "marshal"

# Slash-tier scalar adjustments. The classifier returns a slash tier; these
# multipliers approximate the GDD §6 stake-loss bands and are applied AFTER
# the EMA update so the slash bites the post-update scalar. The tier is also
# preserved on the StakeResolution record so peer auditors can replay.
_SLASH_TIER_RETENTION: dict[SlashTier, float] = {
    # Tier 1: 50-100% - pick mid-band conservative retention so a single
    # catastrophic event halves the agent's standing.
    SlashTier.TIER_1: 0.25,
    # Tier 2: 5-20% - modest hit per fault.
    SlashTier.TIER_2: 0.85,
    # Tier 3: 0.1-1% - barely above noise; the EMA itself does most of the
    # liveness pressure.
    SlashTier.TIER_3: 0.99,
}


# ---------------------------------------------------------------------------
# Pure helpers
# ---------------------------------------------------------------------------


def ema_update(old: float, outcome: float, half_life: int = DEFAULT_EMA_HALF_LIFE) -> float:
    """Apply one EMA step toward ``outcome``.

    ``alpha = 1 / max(1, half_life)``; new = (1 - alpha) * old + alpha * outcome.
    Result is clamped to [0.0, 1.0] defensively even though both inputs are
    expected to be in range.
    """
    if half_life < 1:
        raise ValueError("half_life must be >= 1")
    if not (0.0 <= old <= 1.0):
        raise ValueError(f"old scalar out of range: {old}")
    if not (0.0 <= outcome <= 1.0):
        raise ValueError(f"outcome out of range: {outcome}")
    alpha = 1.0 / float(half_life)
    new = (1.0 - alpha) * old + alpha * outcome
    if new < 0.0:
        return 0.0
    if new > 1.0:
        return 1.0
    return new


def apply_slash(scalar: float, tier: SlashTier | None) -> float:
    """Apply a slash retention factor to a scalar. No-op when ``tier`` is None."""
    if tier is None:
        return scalar
    retention = _SLASH_TIER_RETENTION[tier]
    return max(0.0, min(1.0, scalar * retention))


# ---------------------------------------------------------------------------
# Classifier
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class StakeOutcome:
    """Per-agent outcome derived from a verdict (pure data).

    ``outcome_score`` feeds the EMA update; ``slash_tier`` (when set)
    additionally retains a fraction of the post-update scalar per
    ``_SLASH_TIER_RETENTION``. ``rationale`` is a short reason code that
    grounds the score; it is recorded on the ``StakeResolution`` row.
    """

    agent_id: str
    outcome_score: float
    rationale: str
    slash_tier: SlashTier | None = None


@dataclass(frozen=True)
class ClassifierInputs:
    """Bag of typed inputs the classifier consumes.

    Defined as a dataclass rather than positional arguments so callers and
    test fixtures stay readable as the table evolves.
    """

    gen_result: CommandGenerationResult
    execution_result: CommandExecutionResult | None = None
    marshal_risk: RiskLevel | None = None
    marshal_blocked: bool = False
    extra_agents: tuple[str, ...] = field(default_factory=tuple)


def _winner_supporters(gen_result: CommandGenerationResult) -> set[str]:
    """Return the set of member ids whose normalised candidate matches the winner."""
    breakdown = gen_result.vote_breakdown
    if breakdown is None or breakdown.winner is None:
        return set()
    return set(breakdown.winner_supporters)


def _execution_failed(execution_result: CommandExecutionResult | None) -> bool:
    """True if the operator clearly failed the command (post-approval)."""
    if execution_result is None:
        return False
    if execution_result.success:
        return False
    if execution_result.exit_code is not None and execution_result.exit_code != 0:
        return True
    return execution_result.error is not None


def _execution_destructive(
    execution_result: CommandExecutionResult | None,
    marshal_risk: RiskLevel | None,
) -> bool:
    """True if the executed command was high-risk per marshal AND failed.

    Mirrors the GDD §14.5 Tier 1 trigger: ``marshal_command = HIGH`` plus a
    non-zero damaging exit. We treat any failure (not just a damaging exit
    code, which is unknowable from the platform) as the worst-case proxy.
    """
    if marshal_risk != RiskLevel.HIGH:
        return False
    return _execution_failed(execution_result)


def _honest_member_outcome(
    member_id: str, inputs: ClassifierInputs, supporters: set[str]
) -> StakeOutcome:
    gen = inputs.gen_result
    breakdown = gen.vote_breakdown
    supported = member_id in supporters
    candidate = breakdown.candidates_by_member.get(member_id) if breakdown else None
    if candidate is None:
        return StakeOutcome(member_id, 0.1, "missed_pass", SlashTier.TIER_3)
    if gen.outcome == CommandGenerationOutcome.CONSENSUS_FAILED:
        return StakeOutcome(member_id, 0.3, "consensus_failed")
    if supported:
        return _supported_honest_outcome(member_id, gen)
    return StakeOutcome(member_id, 0.45, "dissenter")


def _supported_honest_outcome(member_id: str, gen: CommandGenerationResult) -> StakeOutcome:
    if gen.auditor_passed or gen.outcome == CommandGenerationOutcome.CONSENSUS:
        return StakeOutcome(member_id, 1.0, "winner_supporter_verified")
    if gen.auditor_reason in (
        AuditorReason.REVISED,
        AuditorReason.REVISED_FROM_DISSENT,
        AuditorReason.SWAPPED_TO_DISSENTER,
    ):
        return StakeOutcome(member_id, 0.55, "winner_supporter_revised")
    if gen.auditor_reason == AuditorReason.WHITELIST_VIOLATION:
        return StakeOutcome(
            member_id, 0.1, "winner_supporter_whitelist_violation", SlashTier.TIER_2
        )
    return StakeOutcome(member_id, 0.4, "winner_supporter_unverified")


def _auditor_intervened(reason: AuditorReason | None) -> bool:
    return reason in (
        AuditorReason.REVISED,
        AuditorReason.REVISED_FROM_DISSENT,
        AuditorReason.SWAPPED_TO_DISSENTER,
        AuditorReason.WHITELIST_VIOLATION,
    )


def _nemesis_outcome(inputs: ClassifierInputs, supporters: set[str]) -> StakeOutcome:
    gen = inputs.gen_result
    breakdown = gen.vote_breakdown
    candidate = breakdown.candidates_by_member.get(NEMESIS_ID) if breakdown else None
    attacked = candidate not in (None, "") and NEMESIS_ID not in supporters
    abstained = candidate in (None, "")
    intervened = _auditor_intervened(gen.auditor_reason)
    clean = gen.auditor_passed or gen.outcome == CommandGenerationOutcome.CONSENSUS
    if gen.outcome == CommandGenerationOutcome.CONSENSUS_FAILED:
        return StakeOutcome(NEMESIS_ID, 0.6, "nemesis_no_consensus")
    if intervened and attacked:
        return StakeOutcome(NEMESIS_ID, 1.0, "nemesis_attack_confirmed")
    if intervened and abstained:
        return StakeOutcome(NEMESIS_ID, 0.1, "nemesis_abstain_miss", SlashTier.TIER_3)
    if clean and abstained:
        return StakeOutcome(NEMESIS_ID, 0.7, "nemesis_abstain_clean")
    if clean and attacked:
        return StakeOutcome(NEMESIS_ID, 0.05, "nemesis_attack_false_alarm", SlashTier.TIER_2)
    return StakeOutcome(NEMESIS_ID, 0.5, "nemesis_uncalibrated")


def _sage_outcome(inputs: ClassifierInputs, intervened: bool) -> StakeOutcome:
    gen = inputs.gen_result
    reason = gen.auditor_reason
    if gen.outcome == CommandGenerationOutcome.CONSENSUS_FAILED:
        return StakeOutcome(SAGE_ID, 0.1, "sage_consensus_failed", SlashTier.TIER_3)
    if gen.outcome == CommandGenerationOutcome.CONSENSUS or (gen.auditor_passed and not intervened):
        return StakeOutcome(SAGE_ID, 1.0, "sage_one_shot")
    if gen.outcome == CommandGenerationOutcome.VERIFIED:
        return StakeOutcome(SAGE_ID, 0.85, "sage_verified")
    if reason in (AuditorReason.REVISED, AuditorReason.REVISED_FROM_DISSENT):
        return StakeOutcome(SAGE_ID, 0.55, "sage_revised")
    if reason == AuditorReason.SWAPPED_TO_DISSENTER:
        return StakeOutcome(SAGE_ID, 0.4, "sage_swapped")
    return StakeOutcome(SAGE_ID, 0.4, "sage_unverified")


def _auditor_outcome(
    inputs: ClassifierInputs, intervened: bool, exec_failed: bool, destructive: bool
) -> StakeOutcome | None:
    gen = inputs.gen_result
    reason = gen.auditor_reason
    if reason is None:
        return None
    if destructive or reason == AuditorReason.AUDITOR_ERROR:
        return _auditor_fault_outcome(destructive)
    if gen.auditor_passed and not exec_failed:
        return StakeOutcome(AUDITOR_ID, 1.0, "auditor_verdict_held")
    if gen.auditor_passed and exec_failed:
        return StakeOutcome(AUDITOR_ID, 0.35, "auditor_verdict_failed_execution")
    if intervened:
        return StakeOutcome(AUDITOR_ID, 0.7 if not exec_failed else 0.4, "auditor_intervention")
    return StakeOutcome(AUDITOR_ID, 0.5, "auditor_neutral")


def _auditor_fault_outcome(destructive: bool) -> StakeOutcome:
    if destructive:
        return StakeOutcome(AUDITOR_ID, 0.0, "auditor_destructive_failure", SlashTier.TIER_1)
    return StakeOutcome(AUDITOR_ID, 0.1, "auditor_error", SlashTier.TIER_2)


def _marshal_outcome(inputs: ClassifierInputs, exec_failed: bool) -> StakeOutcome:
    risk = inputs.marshal_risk
    if inputs.marshal_blocked:
        return _marshal_blocked_outcome(risk)
    if not exec_failed:
        return _marshal_success_outcome(risk)
    return _marshal_failed_outcome(risk)


def _marshal_blocked_outcome(risk: RiskLevel | None) -> StakeOutcome:
    if risk == RiskLevel.HIGH:
        return StakeOutcome(MARSHAL_ID, 0.85, "marshal_blocked_high_risk")
    if risk == RiskLevel.MEDIUM:
        return StakeOutcome(MARSHAL_ID, 0.6, "marshal_blocked_medium_risk")
    return StakeOutcome(MARSHAL_ID, 0.3, "marshal_over_caution_low_risk", SlashTier.TIER_3)


def _marshal_success_outcome(risk: RiskLevel | None) -> StakeOutcome:
    if risk == RiskLevel.LOW:
        return StakeOutcome(MARSHAL_ID, 1.0, "marshal_allowed_low_success")
    if risk == RiskLevel.MEDIUM:
        return StakeOutcome(MARSHAL_ID, 0.9, "marshal_allowed_medium_success")
    return StakeOutcome(MARSHAL_ID, 0.7, "marshal_allowed_high_success")


def _marshal_failed_outcome(risk: RiskLevel | None) -> StakeOutcome:
    if risk == RiskLevel.LOW:
        return StakeOutcome(MARSHAL_ID, 0.1, "marshal_low_risk_missed", SlashTier.TIER_2)
    if risk == RiskLevel.MEDIUM:
        return StakeOutcome(MARSHAL_ID, 0.35, "marshal_medium_risk_missed")
    return StakeOutcome(MARSHAL_ID, 0.75, "marshal_high_risk_flagged_correctly")


def classify_stakes(inputs: ClassifierInputs) -> list[StakeOutcome]:
    """Compute per-agent ``StakeOutcome`` rows from a verdict."""
    gen = inputs.gen_result
    supporters = _winner_supporters(gen)
    exec_failed = _execution_failed(inputs.execution_result)
    intervened = _auditor_intervened(gen.auditor_reason)
    rows = [_honest_member_outcome(member, inputs, supporters) for member in TRIBUNAL_HONEST_FOUR]
    rows.extend((_nemesis_outcome(inputs, supporters), _sage_outcome(inputs, intervened)))
    auditor = _auditor_outcome(
        inputs,
        intervened,
        exec_failed,
        _execution_destructive(inputs.execution_result, inputs.marshal_risk),
    )
    if auditor is not None:
        rows.append(auditor)
    rows.append(_marshal_outcome(inputs, exec_failed))
    for extra in inputs.extra_agents:
        if any(row.agent_id == extra for row in rows):
            continue
        rows.append(StakeOutcome(extra, 0.5, "no_signal"))
    return rows


# ---------------------------------------------------------------------------
# Async dispatcher
# ---------------------------------------------------------------------------


class ResolveStakesResult(BaseModel):
    """Return value of ``resolve_stakes`` - one row per affected agent."""

    model_config = ConfigDict(frozen=True, arbitrary_types_allowed=True, extra="forbid")

    resolutions: list[StakeResolution]


class ReputationService:
    """Stake-resolution writer. Sole post-execution writer of `reputation_state`.

    Constructed in `service_factory.py`. Inject into the per-tool-call hook
    point in slice B; the dispatcher itself is independently testable.
    """

    def __init__(
        self,
        reputation_data_service: ReputationDataService,
        stake_resolution_data_service: StakeResolutionDataService,
        half_life: int = DEFAULT_EMA_HALF_LIFE,
    ) -> None:
        if half_life < 1:
            raise ValueError("half_life must be >= 1")
        self.reputation_data_service = reputation_data_service
        self.stake_resolution_data_service = stake_resolution_data_service
        self.half_life = half_life

    async def resolve_stakes(
        self,
        *,
        tribunal_command_id: str,
        investigation_id: str,
        gen_result: CommandGenerationResult,
        execution_result: CommandExecutionResult | None = None,
        marshal_risk: RiskLevel | None = None,
        marshal_blocked: bool = False,
        extra_agents: tuple[str, ...] = (),
        context: RequestContext,
    ) -> ResolveStakesResult:
        """Apply stake resolution for one verdict.

        Idempotent: if a `stake_resolution` row already exists for
        ``(tribunal_command_id, agent_id)``, the corresponding agent's
        scalar is left untouched and the existing resolution is returned.
        Replaying the same verdict is therefore a no-op.
        """
        if not tribunal_command_id:
            raise ValueError("tribunal_command_id is required")
        if not investigation_id:
            raise ValueError("investigation_id is required")

        outcomes = classify_stakes(
            ClassifierInputs(
                gen_result=gen_result,
                execution_result=execution_result,
                marshal_risk=marshal_risk,
                marshal_blocked=marshal_blocked,
                extra_agents=extra_agents,
            )
        )

        resolutions: list[StakeResolution] = []
        for outcome in outcomes:
            existing = await self.stake_resolution_data_service.get(
                tribunal_command_id=tribunal_command_id,
                agent_id=outcome.agent_id,
            )
            if existing is not None:
                logger.info(
                    "Stake resolution already exists; skipping update",
                    extra={
                        "tribunal_command_id": tribunal_command_id,
                        "agent_id": outcome.agent_id,
                    },
                )
                resolutions.append(existing)
                continue

            current_state = await self.reputation_data_service.get_state(outcome.agent_id)
            scalar_before = current_state.scalar if current_state is not None else BOOTSTRAP_SCALAR

            if outcome.rationale == "no_signal":
                # Phase 4 hook: neutral signal should be a no-op, not a decay toward 0.5.
                scalar_after = scalar_before
            else:
                scalar_after = ema_update(scalar_before, outcome.outcome_score, self.half_life)
                scalar_after = apply_slash(scalar_after, outcome.slash_tier)

            now = datetime.now(UTC)

            updated_state = ReputationState(
                agent_id=outcome.agent_id,
                scalar=scalar_after,
                unbonding_until=current_state.unbonding_until if current_state else None,
                last_slash_tier=int(outcome.slash_tier)
                if outcome.slash_tier is not None
                else (current_state.last_slash_tier if current_state else None),
                updated_at=now,
            )
            await self.reputation_data_service.upsert_state(updated_state, context)

            resolution = StakeResolution(
                id=stake_resolution_id(tribunal_command_id, outcome.agent_id),
                investigation_id=investigation_id,
                tribunal_command_id=tribunal_command_id,
                agent_id=outcome.agent_id,
                outcome_score=outcome.outcome_score,
                rationale=outcome.rationale,
                slash_tier=outcome.slash_tier,
                scalar_before=scalar_before,
                scalar_after=scalar_after,
                half_life=self.half_life,
                created_at=now,
            )
            await self.stake_resolution_data_service.create(resolution, context)
            resolutions.append(resolution)

            logger.info(
                "Stake resolved",
                extra={
                    "agent_id": outcome.agent_id,
                    "tribunal_command_id": tribunal_command_id,
                    "outcome_score": outcome.outcome_score,
                    "rationale": outcome.rationale,
                    "scalar_before": scalar_before,
                    "scalar_after": scalar_after,
                    "slash_tier": int(outcome.slash_tier)
                    if outcome.slash_tier is not None
                    else None,
                },
            )

        return ResolveStakesResult(resolutions=resolutions)


__all__ = [
    "BOOTSTRAP_SCALAR",
    "DEFAULT_EMA_HALF_LIFE",
    "ClassifierInputs",
    "ReputationService",
    "ResolveStakesResult",
    "StakeOutcome",
    "apply_slash",
    "classify_stakes",
    "ema_update",
]
