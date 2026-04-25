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

"""Unit tests for the Phase 3 ReputationService (GDD §14.5).

Pure-function table coverage and dispatcher idempotency. The dispatcher
uses ``AsyncMock`` data services — no cache, no IO. The classifier is
exercised exhaustively against the §14.5 outcome table.
"""

from __future__ import annotations

from datetime import UTC, datetime
from unittest.mock import AsyncMock

import pytest

from app.constants import (
    AuditorReason,
    CommandGenerationOutcome,
    RiskLevel,
    TribunalMember,
)
from app.models.agents.tribunal import (
    CommandGenerationResult,
    VoteBreakdown,
)
from app.models.reputation import ReputationState, SlashTier, StakeResolution
from app.models.tool_results import CommandExecutionResult
from app.services.ai.reputation_service import (
    AUDITOR_ID,
    BOOTSTRAP_SCALAR,
    DEFAULT_EMA_HALF_LIFE,
    NEMESIS_ID,
    SAGE_ID,
    TRIBUNAL_HONEST_FOUR,
    ClassifierInputs,
    ReputationService,
    apply_slash,
    classify_stakes,
    ema_update,
)
from app.services.data.stake_resolution_data_service import stake_resolution_id


pytestmark = [pytest.mark.unit]


# ---------------------------------------------------------------------------
# Builders
# ---------------------------------------------------------------------------


AXIOM = str(TribunalMember.AXIOM)
CONCORD = str(TribunalMember.CONCORD)
VARIANCE = str(TribunalMember.VARIANCE)
PRAGMA = str(TribunalMember.PRAGMA)
NEMESIS = str(TribunalMember.NEMESIS)


def _full_breakdown(
    *,
    winner: str | None = "ls -la",
    winner_supporters: tuple[str, ...] = (AXIOM, CONCORD, VARIANCE),
    candidates: dict[str, str] | None = None,
) -> VoteBreakdown:
    if candidates is None:
        candidates = {
            AXIOM: "ls -la",
            CONCORD: "ls -la",
            VARIANCE: "ls -la",
            PRAGMA: "ls",
            NEMESIS: "rm -rf /",
        }
    return VoteBreakdown(
        candidates_by_member=candidates,
        winner=winner,
        winner_supporters=list(winner_supporters),
        consensus_strength=len(winner_supporters) / max(1, len(candidates)),
    )


def _result(
    *,
    outcome: CommandGenerationOutcome = CommandGenerationOutcome.VERIFIED,
    auditor_passed: bool | None = True,
    auditor_reason: AuditorReason | None = AuditorReason.OK,
    breakdown: VoteBreakdown | None = None,
    final_command: str | None = "ls -la",
) -> CommandGenerationResult:
    return CommandGenerationResult(
        request="list files",
        final_command=final_command,
        outcome=outcome,
        vote_winner=breakdown.winner if breakdown else "ls -la",
        vote_score=breakdown.consensus_strength if breakdown else 0.6,
        vote_breakdown=breakdown if breakdown is not None else _full_breakdown(),
        auditor_passed=auditor_passed,
        auditor_reason=auditor_reason,
    )


def _outcomes_by_agent(rows) -> dict[str, "object"]:
    return {row.agent_id: row for row in rows}


# ---------------------------------------------------------------------------
# ema_update
# ---------------------------------------------------------------------------


class TestEmaUpdate:
    def test_half_life_one_collapses_to_outcome(self):
        # alpha = 1 -> new = outcome regardless of old
        assert ema_update(0.0, 1.0, half_life=1) == 1.0
        assert ema_update(1.0, 0.0, half_life=1) == 0.0
        assert ema_update(0.5, 0.25, half_life=1) == 0.25

    def test_convergence_from_below(self):
        x = 0.5
        for _ in range(2000):
            x = ema_update(x, 1.0, half_life=50)
        assert x > 0.99

    def test_convergence_from_above(self):
        x = 0.5
        for _ in range(2000):
            x = ema_update(x, 0.0, half_life=50)
        assert x < 0.01

    def test_clamps_inside_unit_interval(self):
        # Defensive clamping; should never escape [0,1] given valid inputs.
        x = ema_update(1.0, 1.0, half_life=50)
        assert 0.0 <= x <= 1.0
        x = ema_update(0.0, 0.0, half_life=50)
        assert 0.0 <= x <= 1.0

    def test_invalid_half_life_raises(self):
        with pytest.raises(ValueError):
            ema_update(0.5, 0.5, half_life=0)

    @pytest.mark.parametrize("bad", [-0.01, 1.01])
    def test_invalid_old_raises(self, bad):
        with pytest.raises(ValueError):
            ema_update(bad, 0.5)

    @pytest.mark.parametrize("bad", [-0.01, 1.01])
    def test_invalid_outcome_raises(self, bad):
        with pytest.raises(ValueError):
            ema_update(0.5, bad)


# ---------------------------------------------------------------------------
# apply_slash
# ---------------------------------------------------------------------------


class TestApplySlash:
    def test_none_is_noop(self):
        assert apply_slash(0.7, None) == 0.7

    def test_tier_factors_distinct_and_ordered(self):
        scalar = 0.8
        t1 = apply_slash(scalar, SlashTier.TIER_1)
        t2 = apply_slash(scalar, SlashTier.TIER_2)
        t3 = apply_slash(scalar, SlashTier.TIER_3)
        # Tier 1 is the harshest, Tier 3 barely moves the scalar.
        assert t1 < t2 < t3 < scalar
        # Tier 1 cuts substantially; Tier 3 is essentially identity.
        assert t1 < scalar * 0.5
        assert t3 > scalar * 0.95

    def test_clamps_at_zero(self):
        assert apply_slash(0.0, SlashTier.TIER_1) == 0.0


# ---------------------------------------------------------------------------
# classify_stakes — §14.5 table
# ---------------------------------------------------------------------------


class TestClassifyStakesHonestFour:
    def test_winner_supporters_score_top(self):
        rows = classify_stakes(ClassifierInputs(gen_result=_result()))
        by_agent = _outcomes_by_agent(rows)
        for member in (AXIOM, CONCORD, VARIANCE):
            assert by_agent[member].outcome_score == 1.0
            assert by_agent[member].rationale == "winner_supporter_verified"
            assert by_agent[member].slash_tier is None

    def test_dissenter_baseline(self):
        rows = classify_stakes(ClassifierInputs(gen_result=_result()))
        by_agent = _outcomes_by_agent(rows)
        # PRAGMA dissents from the winner -> calibrated mid-low; no slash.
        assert 0.4 <= by_agent[PRAGMA].outcome_score < 0.5
        assert by_agent[PRAGMA].rationale == "dissenter"
        assert by_agent[PRAGMA].slash_tier is None

    def test_winner_supporter_with_revision_lower_than_clean_pass(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.VERIFICATION_FAILED,
                auditor_passed=False,
                auditor_reason=AuditorReason.REVISED,
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[AXIOM].rationale == "winner_supporter_revised"
        assert by_agent[AXIOM].outcome_score < 1.0

    def test_winner_supporter_whitelist_violation_is_tier_2(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.VERIFICATION_FAILED,
                auditor_passed=False,
                auditor_reason=AuditorReason.WHITELIST_VIOLATION,
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[AXIOM].slash_tier == SlashTier.TIER_2

    def test_missed_pass_is_tier_3(self):
        candidates = {
            CONCORD: "ls -la",
            VARIANCE: "ls -la",
            PRAGMA: "ls",
            NEMESIS: "",
        }
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                breakdown=_full_breakdown(
                    winner_supporters=(CONCORD, VARIANCE),
                    candidates=candidates,
                ),
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert AXIOM in by_agent
        assert by_agent[AXIOM].rationale == "missed_pass"
        assert by_agent[AXIOM].slash_tier == SlashTier.TIER_3

    def test_consensus_failed_flat_hit(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.CONSENSUS_FAILED,
                auditor_passed=None,
                auditor_reason=None,
                breakdown=_full_breakdown(
                    winner=None,
                    winner_supporters=(),
                ),
                final_command=None,
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        for m in TRIBUNAL_HONEST_FOUR:
            assert by_agent[m].rationale == "consensus_failed"
            assert by_agent[m].slash_tier is None


class TestClassifyStakesNemesis:
    def test_clean_consensus_abstain_rewarded(self):
        candidates = {
            AXIOM: "ls",
            CONCORD: "ls",
            VARIANCE: "ls",
            PRAGMA: "ls",
            NEMESIS: "",  # abstain
        }
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.CONSENSUS,
                auditor_passed=True,
                auditor_reason=AuditorReason.OK,
                breakdown=_full_breakdown(
                    winner="ls",
                    winner_supporters=(AXIOM, CONCORD, VARIANCE, PRAGMA),
                    candidates=candidates,
                ),
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[NEMESIS].rationale == "nemesis_abstain_clean"
        assert by_agent[NEMESIS].slash_tier is None
        assert by_agent[NEMESIS].outcome_score >= 0.5

    def test_attack_confirmed_is_top_score(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.VERIFICATION_FAILED,
                auditor_passed=False,
                auditor_reason=AuditorReason.SWAPPED_TO_DISSENTER,
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[NEMESIS].outcome_score == 1.0
        assert by_agent[NEMESIS].rationale == "nemesis_attack_confirmed"

    def test_abstain_miss_is_tier_3(self):
        candidates = {
            AXIOM: "ls",
            CONCORD: "ls",
            VARIANCE: "ls",
            PRAGMA: "ls",
            NEMESIS: "",
        }
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.VERIFICATION_FAILED,
                auditor_passed=False,
                auditor_reason=AuditorReason.REVISED,
                breakdown=_full_breakdown(
                    winner="ls",
                    winner_supporters=(AXIOM, CONCORD, VARIANCE, PRAGMA),
                    candidates=candidates,
                ),
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[NEMESIS].rationale == "nemesis_abstain_miss"
        assert by_agent[NEMESIS].slash_tier == SlashTier.TIER_3

    def test_false_alarm_is_tier_2(self):
        # Clean consensus + Nemesis attacked -> false alarm -> tier 2.
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.CONSENSUS,
                auditor_passed=True,
                auditor_reason=AuditorReason.OK,
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        # Default breakdown has Nemesis voting for "rm -rf /", which is not
        # a winner support — counts as an attack.
        assert by_agent[NEMESIS].rationale == "nemesis_attack_false_alarm"
        assert by_agent[NEMESIS].slash_tier == SlashTier.TIER_2


class TestClassifyStakesSage:
    def test_one_shot_consensus_top_score(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(outcome=CommandGenerationOutcome.CONSENSUS),
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[SAGE_ID].outcome_score == 1.0
        assert by_agent[SAGE_ID].rationale == "sage_one_shot"

    def test_revised_lower_than_verified(self):
        rows_revised = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.VERIFICATION_FAILED,
                auditor_passed=False,
                auditor_reason=AuditorReason.REVISED,
            )
        ))
        rows_verified = classify_stakes(ClassifierInputs(
            gen_result=_result(outcome=CommandGenerationOutcome.VERIFIED),
        ))
        revised = _outcomes_by_agent(rows_revised)[SAGE_ID]
        verified = _outcomes_by_agent(rows_verified)[SAGE_ID]
        assert revised.outcome_score < verified.outcome_score

    def test_consensus_failed_tier_3(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.CONSENSUS_FAILED,
                auditor_passed=None,
                auditor_reason=None,
                breakdown=_full_breakdown(winner=None, winner_supporters=()),
                final_command=None,
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[SAGE_ID].slash_tier == SlashTier.TIER_3


class TestClassifyStakesAuditor:
    def test_verdict_held_top_score(self):
        rows = classify_stakes(ClassifierInputs(gen_result=_result()))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[AUDITOR_ID].rationale == "auditor_verdict_held"
        assert by_agent[AUDITOR_ID].outcome_score == 1.0

    def test_destructive_failure_is_tier_1(self):
        # Auditor passed a HIGH-risk command that subsequently failed at exec.
        exec_failed = CommandExecutionResult(success=False, exit_code=1, error="boom")
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(),
            execution_result=exec_failed,
            warden_risk=RiskLevel.HIGH,
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[AUDITOR_ID].slash_tier == SlashTier.TIER_1
        assert by_agent[AUDITOR_ID].outcome_score == 0.0

    def test_auditor_error_is_tier_2(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.VERIFICATION_FAILED,
                auditor_passed=False,
                auditor_reason=AuditorReason.AUDITOR_ERROR,
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent[AUDITOR_ID].slash_tier == SlashTier.TIER_2

    def test_no_verdict_skips_auditor_row(self):
        # CONSENSUS_FAILED -> no auditor verdict -> no auditor row.
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(
                outcome=CommandGenerationOutcome.CONSENSUS_FAILED,
                auditor_passed=None,
                auditor_reason=None,
                breakdown=_full_breakdown(winner=None, winner_supporters=()),
                final_command=None,
            )
        ))
        by_agent = _outcomes_by_agent(rows)
        assert AUDITOR_ID not in by_agent


class TestClassifyStakesExtraAgents:
    def test_extra_agent_neutral_score(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(),
            extra_agents=("triage",),
        ))
        by_agent = _outcomes_by_agent(rows)
        assert by_agent["triage"].outcome_score == 0.5
        assert by_agent["triage"].rationale == "no_signal"

    def test_extra_agent_does_not_overwrite_existing(self):
        rows = classify_stakes(ClassifierInputs(
            gen_result=_result(),
            extra_agents=(SAGE_ID,),
        ))
        sage_rows = [r for r in rows if r.agent_id == SAGE_ID]
        assert len(sage_rows) == 1
        # Sage row is the verdict-driven one, not the placeholder.
        assert sage_rows[0].rationale != "no_signal"


# ---------------------------------------------------------------------------
# ReputationService.resolve_stakes
# ---------------------------------------------------------------------------


class TestResolveStakes:
    pytestmark = [pytest.mark.asyncio(loop_scope="session")]

    @pytest.fixture
    def reputation_data(self) -> AsyncMock:
        m = AsyncMock()
        m.get_state.return_value = None  # bootstrap path
        m.upsert_state.return_value = None
        return m

    @pytest.fixture
    def stake_data(self) -> AsyncMock:
        m = AsyncMock()
        m.get.return_value = None
        m.create.side_effect = lambda r: r  # echo back
        return m

    @pytest.fixture
    def service(self, reputation_data, stake_data) -> ReputationService:
        return ReputationService(
            reputation_data_service=reputation_data,
            stake_resolution_data_service=stake_data,
        )

    async def test_bootstrap_uses_neutral_scalar(self, service, reputation_data, stake_data):
        result = await service.resolve_stakes(
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            gen_result=_result(),
        )
        # All rows should report scalar_before == BOOTSTRAP_SCALAR because
        # `get_state` returned None for every persona.
        assert result.resolutions, "expected at least one stake resolution"
        for row in result.resolutions:
            assert row.scalar_before == BOOTSTRAP_SCALAR
            assert row.half_life == DEFAULT_EMA_HALF_LIFE
            assert row.tribunal_command_id == "tc-1"
            assert row.investigation_id == "inv-1"
            assert row.id == stake_resolution_id("tc-1", row.agent_id)

    async def test_writes_one_state_and_one_resolution_per_agent(
        self, service, reputation_data, stake_data
    ):
        result = await service.resolve_stakes(
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            gen_result=_result(),
        )
        assert reputation_data.upsert_state.call_count == len(result.resolutions)
        assert stake_data.create.call_count == len(result.resolutions)

    async def test_idempotent_replay_skips_writes(
        self, service, reputation_data, stake_data
    ):
        # Pre-populate every agent so resolve_stakes treats the verdict as
        # already-resolved.
        async def _existing_get(tribunal_command_id, agent_id):
            return StakeResolution(
                id=stake_resolution_id(tribunal_command_id, agent_id),
                investigation_id="inv-1",
                tribunal_command_id=tribunal_command_id,
                agent_id=agent_id,
                outcome_score=1.0,
                rationale="winner_supporter_verified",
                slash_tier=None,
                scalar_before=0.5,
                scalar_after=0.51,
                half_life=DEFAULT_EMA_HALF_LIFE,
                created_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
            )

        stake_data.get.side_effect = _existing_get

        result = await service.resolve_stakes(
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            gen_result=_result(),
        )

        # Replay must be a no-op: no new state writes, no new resolution rows.
        reputation_data.upsert_state.assert_not_called()
        stake_data.create.assert_not_called()
        # We still surface the existing rows so callers can observe the prior
        # outcome.
        assert len(result.resolutions) > 0

    async def test_existing_state_used_as_scalar_before(
        self, service, reputation_data, stake_data
    ):
        async def _state(agent_id):
            return ReputationState(
                agent_id=agent_id,
                scalar=0.9,
                updated_at=datetime(2026, 4, 24, 12, 0, 0, tzinfo=UTC),
            )

        reputation_data.get_state.side_effect = _state

        result = await service.resolve_stakes(
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            gen_result=_result(),
        )

        for row in result.resolutions:
            assert row.scalar_before == 0.9

    async def test_destructive_tier_1_path_persists_slash_tier(
        self, service, reputation_data, stake_data
    ):
        result = await service.resolve_stakes(
            tribunal_command_id="tc-1",
            investigation_id="inv-1",
            gen_result=_result(),
            execution_result=CommandExecutionResult(success=False, exit_code=1, error="boom"),
            warden_risk=RiskLevel.HIGH,
        )
        auditor = next(r for r in result.resolutions if r.agent_id == AUDITOR_ID)
        assert auditor.slash_tier == SlashTier.TIER_1
        # scalar_after is bounded below by zero after slashing the EMA.
        assert auditor.scalar_after <= auditor.scalar_before

    async def test_invalid_command_id_raises(self, service):
        with pytest.raises(ValueError):
            await service.resolve_stakes(
                tribunal_command_id="",
                investigation_id="inv-1",
                gen_result=_result(),
            )

    async def test_invalid_investigation_id_raises(self, service):
        with pytest.raises(ValueError):
            await service.resolve_stakes(
                tribunal_command_id="tc-1",
                investigation_id="",
                gen_result=_result(),
            )

    async def test_invalid_half_life_in_constructor_raises(
        self, reputation_data, stake_data
    ):
        with pytest.raises(ValueError):
            ReputationService(
                reputation_data_service=reputation_data,
                stake_resolution_data_service=stake_data,
                half_life=0,
            )
