# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for observed-tier binding validation.

Verifies that a candidate tier is observed on every task declared to
exercise it. A task that declares it exercises a tier must have
matching provider-boundary telemetry for that tier. Missing observations
for declared tiers are rejected. Observations for non-declared tiers are
ignored. Non-invoked roles are preserved as explicit non-observations.
"""

from __future__ import annotations

import pytest

from g8e_evals.index import ModelRole, TaskTierDeclaration, TierObservationRecord, validate_observed_tier_binding


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _declaration(task_id: str, *tiers: ModelRole) -> TaskTierDeclaration:
    return TaskTierDeclaration(task_id=task_id, declared_tiers=list(tiers))


def _make_tier_observation(
    *,
    task_id: str = "task-1",
    tier: str = "primary",
    observed: bool = True,
    model_variant_id: str = "qwen3-8b-q4_0",
) -> TierObservationRecord:
    return TierObservationRecord(
        task_id=task_id,
        tier=tier,
        observed=observed,
        model_variant_id=model_variant_id,
        evidence_hash=_VALID_HASH,
    )


class TestObservedTierBinding:
    def test_all_declared_tiers_observed_passes(self):
        """Every task that declares a tier has matching observations."""
        task_tier_declarations = [
            _declaration("task-1", ModelRole.PRIMARY),
            _declaration("task-2", ModelRole.PRIMARY, ModelRole.ASSISTANT),
        ]
        observations = [
            _make_tier_observation(task_id="task-1", tier="primary"),
            _make_tier_observation(task_id="task-2", tier="primary"),
            _make_tier_observation(task_id="task-2", tier="assistant"),
        ]
        validate_observed_tier_binding(task_tier_declarations, observations)

    def test_missing_observation_for_declared_tier_rejected(self):
        """A task declaring a tier without a matching observation is rejected."""
        task_tier_declarations = [_declaration("task-1", ModelRole.PRIMARY)]
        observations = []  # No observations at all
        with pytest.raises(ValueError, match=r"missing.*tier.*observation"):
            validate_observed_tier_binding(task_tier_declarations, observations)

    def test_observation_for_non_declared_tier_ignored(self):
        """An observation for a tier not declared by the task is ignored."""
        task_tier_declarations = [_declaration("task-1", ModelRole.PRIMARY)]
        observations = [
            _make_tier_observation(task_id="task-1", tier="primary"),
            _make_tier_observation(task_id="task-1", tier="lite"),  # Not declared
        ]
        validate_observed_tier_binding(task_tier_declarations, observations)

    def test_non_invoked_role_preserved_as_non_observation(self):
        """A non-invoked role is preserved as an explicit non-observation."""
        task_tier_declarations = [_declaration("task-1", ModelRole.PRIMARY)]
        observations = [
            _make_tier_observation(task_id="task-1", tier="primary", observed=True),
            _make_tier_observation(task_id="task-1", tier="assistant", observed=False),
        ]
        validate_observed_tier_binding(task_tier_declarations, observations)

    def test_observed_false_for_declared_tier_rejected(self):
        """An explicit non-observation (observed=False) for a declared tier is rejected."""
        task_tier_declarations = [_declaration("task-1", ModelRole.PRIMARY)]
        observations = [
            _make_tier_observation(task_id="task-1", tier="primary", observed=False),
        ]
        with pytest.raises(ValueError, match=r"declared.*not observed"):
            validate_observed_tier_binding(task_tier_declarations, observations)

    def test_empty_declarations_passes(self):
        """No tier declarations means no observations needed."""
        task_tier_declarations = []
        observations = []
        validate_observed_tier_binding(task_tier_declarations, observations)

    def test_multiple_tasks_all_tiers_observed_passes(self):
        """Multiple tasks with multiple tiers all observed passes."""
        task_tier_declarations = [
            _declaration("task-1", ModelRole.PRIMARY, ModelRole.ASSISTANT, ModelRole.LITE),
            _declaration("task-2", ModelRole.PRIMARY, ModelRole.ASSISTANT, ModelRole.LITE),
        ]
        observations = [
            _make_tier_observation(task_id="task-1", tier="primary"),
            _make_tier_observation(task_id="task-1", tier="assistant"),
            _make_tier_observation(task_id="task-1", tier="lite"),
            _make_tier_observation(task_id="task-2", tier="primary"),
            _make_tier_observation(task_id="task-2", tier="assistant"),
            _make_tier_observation(task_id="task-2", tier="lite"),
        ]
        validate_observed_tier_binding(task_tier_declarations, observations)
