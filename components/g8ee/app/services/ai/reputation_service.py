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

"""Phase 3 reputation writer (GDD §14.5).

Pure-function classifier and EMA helper plus an async dispatcher that:

1. Maps a tribunal verdict (and its execution result, when known) into a
   per-agent ``StakeOutcome`` table keyed by persona id (axiom, concord,
   variance, pragma, nemesis, sage, auditor).
2. EMA-updates each affected ``ReputationState`` row.
3. Writes one ``StakeResolution`` per affected agent for replayability.

The slashing classifier is intentionally side-effect-free so it can be
tested exhaustively against the §14.5 table. The writer is the only side
effect carrier and is gated on the env flag ``REPUTATION_RESOLUTION_ENABLED``
at the call site (Phase 3 Slice B will add the env-flag wiring; the dispatcher
itself is always safe to invoke).

Vortex (GDD §3) is preserved: this module reads `reputation_state` (sole
post-execution writer) but is not visible to any persona prompt builder.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from datetime import datetime, UTC

from app.constants import (
    AuditorReason,
    CommandGenerationOutcome,
    RiskLevel,
    TribunalMember,
)
from app.models.agents.tribunal import CommandGenerationResult
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
halve). GDD §14.10 suggests this as the start point; ops can override via the
env var ``REPUTATION_EMA_HALF_LIFE`` at the call site."""

BOOTSTRAP_SCALAR: float = 0.5
"""Neutral starting scalar for any agent that has no prior `reputation_state`
row. Mirrors the value seeded by ``scripts/data/seed-reputation-state.py``."""

TRIBUNAL_HONEST_FOUR: tuple[str, ...] = (
    str(TribunalMember.AXIOM),
    str(TribunalMember.CONCORD),
    str(TribunalMember.VARIANCE),
    str(TribunalMember.PRAGMA),
)
"""Persona ids for the honest four. Nemesis is excluded — its stake follows a
proper-scoring rule (GDD §5)."""

NEMESIS_ID: str = str(TribunalMember.NEMESIS)
SAGE_ID: str = "sage"
AUDITOR_ID: str = "auditor"

# Slash-tier scalar adjustments. The classifier returns a slash tier; these
# multipliers approximate the GDD §6 stake-loss bands and are applied AFTER
# the EMA update so the slash bites the post-update scalar. The tier is also
# preserved on the StakeResolution record so peer auditors can replay.
_SLASH_TIER_RETENTION: dict[SlashTier, float] = {
    # Tier 1: 50-100% — pick mid-band conservative retention so a single
    # catastrophic event halves the agent's standing.
    SlashTier.TIER_1: 0.25,
    # Tier 2: 5-20% — modest hit per fault.
    SlashTier.TIER_2: 0.85,
    # Tier 3: 0.1-1% — barely above noise; the EMA itself does most of the
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
    warden_risk: RiskLevel | None = None
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
    warden_risk: RiskLevel | None,
) -> bool:
    """True if the executed command was high-risk per warden AND failed.

    Mirrors the GDD §14.5 Tier 1 trigger: ``warden_command_risk = HIGH`` plus a
    non-zero damaging exit. We treat any failure (not just a damaging exit
    code, which is unknowable from the platform) as the worst-case proxy.
    """
    if warden_risk != RiskLevel.HIGH:
        return False
    return _execution_failed(execution_result)


def classify_stakes(inputs: ClassifierInputs) -> list[StakeOutcome]:
    """Compute per-agent ``StakeOutcome`` rows from a verdict.

    Mirrors GDD §14.5. Returns one outcome per affected agent — the four
    honest tribunal members (always), Nemesis (always), Sage (always), and
    Auditor (when an Auditor verdict was emitted, i.e. ``auditor_reason`` is
    not None). ``inputs.extra_agents`` lets callers add Triage when Phase 4
    starts emitting clarification telemetry; the classifier returns a
    neutral 0.5 for any extra agent it has no rule for, which keeps the
    EMA at its bootstrap value until a real signal exists.
    """

    gen = inputs.gen_result
    breakdown = gen.vote_breakdown
    outcome = gen.outcome
    auditor_reason = gen.auditor_reason
    auditor_passed = gen.auditor_passed

    supporters = _winner_supporters(gen)
    exec_failed = _execution_failed(inputs.execution_result)
    destructive = _execution_destructive(inputs.execution_result, inputs.warden_risk)

    rows: list[StakeOutcome] = []

    # ------------------------------------------------------------------
    # Tribunal honest four
    # ------------------------------------------------------------------
    for member_id in TRIBUNAL_HONEST_FOUR:
        supported_winner = member_id in supporters
        candidate = breakdown.candidates_by_member.get(member_id) if breakdown else None
        missed = candidate is None

        if missed:
            # Tier 3: missed pass / liveness fault.
            rows.append(StakeOutcome(
                agent_id=member_id,
                outcome_score=0.1,
                rationale="missed_pass",
                slash_tier=SlashTier.TIER_3,
            ))
            continue

        if outcome == CommandGenerationOutcome.CONSENSUS_FAILED:
            # No winner emerged. Honest four take a flat hit; the EMA absorbs.
            rows.append(StakeOutcome(
                agent_id=member_id,
                outcome_score=0.3,
                rationale="consensus_failed",
            ))
            continue

        if supported_winner and (auditor_passed or outcome == CommandGenerationOutcome.CONSENSUS):
            rows.append(StakeOutcome(
                agent_id=member_id,
                outcome_score=1.0,
                rationale="winner_supporter_verified",
            ))
        elif supported_winner and auditor_reason in (
            AuditorReason.REVISED,
            AuditorReason.REVISED_FROM_DISSENT,
            AuditorReason.SWAPPED_TO_DISSENTER,
        ):
            # Voted for the winner but auditor intervened.
            rows.append(StakeOutcome(
                agent_id=member_id,
                outcome_score=0.55,
                rationale="winner_supporter_revised",
            ))
        elif supported_winner and auditor_reason == AuditorReason.WHITELIST_VIOLATION:
            # Tier 2: winning candidate was demonstrably non-compliant.
            rows.append(StakeOutcome(
                agent_id=member_id,
                outcome_score=0.1,
                rationale="winner_supporter_whitelist_violation",
                slash_tier=SlashTier.TIER_2,
            ))
        elif supported_winner:
            rows.append(StakeOutcome(
                agent_id=member_id,
                outcome_score=0.4,
                rationale="winner_supporter_unverified",
            ))
        else:
            # Honest dissent — calibrated lower than supporters, still positive
            # baseline because diversity is a feature.
            rows.append(StakeOutcome(
                agent_id=member_id,
                outcome_score=0.45,
                rationale="dissenter",
            ))

    # ------------------------------------------------------------------
    # Nemesis (proper scoring rule, GDD §5)
    # ------------------------------------------------------------------
    nemesis_candidate = breakdown.candidates_by_member.get(NEMESIS_ID) if breakdown else None
    nemesis_attacked = nemesis_candidate not in (None, "") and (NEMESIS_ID not in supporters)
    nemesis_abstained = nemesis_candidate in (None, "")

    auditor_intervened = auditor_reason in (
        AuditorReason.REVISED,
        AuditorReason.REVISED_FROM_DISSENT,
        AuditorReason.SWAPPED_TO_DISSENTER,
        AuditorReason.WHITELIST_VIOLATION,
    )

    if outcome == CommandGenerationOutcome.CONSENSUS_FAILED:
        # No clean consensus formed. Nemesis cannot be wrong here; small gain.
        rows.append(StakeOutcome(
            agent_id=NEMESIS_ID,
            outcome_score=0.6,
            rationale="nemesis_no_consensus",
        ))
    elif auditor_intervened and nemesis_attacked:
        # Confirmed-flawed consensus + nemesis attacked = large gain.
        rows.append(StakeOutcome(
            agent_id=NEMESIS_ID,
            outcome_score=1.0,
            rationale="nemesis_attack_confirmed",
        ))
    elif auditor_intervened and nemesis_abstained:
        # Missed a real flaw — large loss (Tier 3 liveness).
        rows.append(StakeOutcome(
            agent_id=NEMESIS_ID,
            outcome_score=0.1,
            rationale="nemesis_abstain_miss",
            slash_tier=SlashTier.TIER_3,
        ))
    elif (auditor_passed or outcome == CommandGenerationOutcome.CONSENSUS) and nemesis_abstained:
        # Clean consensus + abstained = small gain (calibration reward).
        rows.append(StakeOutcome(
            agent_id=NEMESIS_ID,
            outcome_score=0.7,
            rationale="nemesis_abstain_clean",
        ))
    elif (auditor_passed or outcome == CommandGenerationOutcome.CONSENSUS) and nemesis_attacked:
        # Clean consensus + nemesis attacked = false alarm, large loss.
        rows.append(StakeOutcome(
            agent_id=NEMESIS_ID,
            outcome_score=0.05,
            rationale="nemesis_attack_false_alarm",
            slash_tier=SlashTier.TIER_2,
        ))
    else:
        # Fallback (shouldn't usually fire). Keep neutral.
        rows.append(StakeOutcome(
            agent_id=NEMESIS_ID,
            outcome_score=0.5,
            rationale="nemesis_uncalibrated",
        ))

    # ------------------------------------------------------------------
    # Sage (one-shot sufficiency, GDD §5)
    # ------------------------------------------------------------------
    if outcome == CommandGenerationOutcome.CONSENSUS_FAILED:
        rows.append(StakeOutcome(
            agent_id=SAGE_ID,
            outcome_score=0.1,
            rationale="sage_consensus_failed",
            slash_tier=SlashTier.TIER_3,
        ))
    elif outcome == CommandGenerationOutcome.CONSENSUS or (auditor_passed and not auditor_intervened):
        rows.append(StakeOutcome(
            agent_id=SAGE_ID,
            outcome_score=1.0,
            rationale="sage_one_shot",
        ))
    elif outcome == CommandGenerationOutcome.VERIFIED:
        rows.append(StakeOutcome(
            agent_id=SAGE_ID,
            outcome_score=0.85,
            rationale="sage_verified",
        ))
    elif auditor_reason in (AuditorReason.REVISED, AuditorReason.REVISED_FROM_DISSENT):
        rows.append(StakeOutcome(
            agent_id=SAGE_ID,
            outcome_score=0.55,
            rationale="sage_revised",
        ))
    elif auditor_reason == AuditorReason.SWAPPED_TO_DISSENTER:
        rows.append(StakeOutcome(
            agent_id=SAGE_ID,
            outcome_score=0.4,
            rationale="sage_swapped",
        ))
    else:
        rows.append(StakeOutcome(
            agent_id=SAGE_ID,
            outcome_score=0.4,
            rationale="sage_unverified",
        ))

    # ------------------------------------------------------------------
    # Auditor (downstream truth, GDD §5)
    # ------------------------------------------------------------------
    if auditor_reason is not None:
        if destructive:
            # Tier 1: catastrophic — auditor approved a HIGH-risk command that
            # then failed during execution.
            rows.append(StakeOutcome(
                agent_id=AUDITOR_ID,
                outcome_score=0.0,
                rationale="auditor_destructive_failure",
                slash_tier=SlashTier.TIER_1,
            ))
        elif auditor_reason == AuditorReason.AUDITOR_ERROR:
            rows.append(StakeOutcome(
                agent_id=AUDITOR_ID,
                outcome_score=0.1,
                rationale="auditor_error",
                slash_tier=SlashTier.TIER_2,
            ))
        elif auditor_passed and not exec_failed:
            rows.append(StakeOutcome(
                agent_id=AUDITOR_ID,
                outcome_score=1.0,
                rationale="auditor_verdict_held",
            ))
        elif auditor_passed and exec_failed:
            rows.append(StakeOutcome(
                agent_id=AUDITOR_ID,
                outcome_score=0.35,
                rationale="auditor_verdict_failed_execution",
            ))
        elif auditor_intervened:
            # Auditor caught something. Reward proportional to whether the
            # intervention then held up at execution.
            rows.append(StakeOutcome(
                agent_id=AUDITOR_ID,
                outcome_score=0.7 if not exec_failed else 0.4,
                rationale="auditor_intervention",
            ))
        else:
            rows.append(StakeOutcome(
                agent_id=AUDITOR_ID,
                outcome_score=0.5,
                rationale="auditor_neutral",
            ))

    # ------------------------------------------------------------------
    # Extra agents (Phase 4 hook — Triage clarifications)
    # ------------------------------------------------------------------
    for extra in inputs.extra_agents:
        if any(row.agent_id == extra for row in rows):
            continue
        rows.append(StakeOutcome(
            agent_id=extra,
            outcome_score=0.5,
            rationale="no_signal",
        ))

    return rows


# ---------------------------------------------------------------------------
# Async dispatcher
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ResolveStakesResult:
    """Return value of ``resolve_stakes`` — one row per affected agent."""

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
        warden_risk: RiskLevel | None = None,
        extra_agents: tuple[str, ...] = (),
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

        outcomes = classify_stakes(ClassifierInputs(
            gen_result=gen_result,
            execution_result=execution_result,
            warden_risk=warden_risk,
            extra_agents=extra_agents,
        ))

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
                last_slash_tier=int(outcome.slash_tier) if outcome.slash_tier is not None else (current_state.last_slash_tier if current_state else None),
                updated_at=now,
            )
            await self.reputation_data_service.upsert_state(updated_state)

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
            await self.stake_resolution_data_service.create(resolution)
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
                    "slash_tier": int(outcome.slash_tier) if outcome.slash_tier is not None else None,
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
