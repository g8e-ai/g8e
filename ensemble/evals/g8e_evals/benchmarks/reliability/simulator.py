# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local reliability simulator for the synthetic reliability eval suite.

The simulator is a production-shaped system under test that simulates
adverse conditions (provider throttling, malformed structured output,
interrupted streams, etc.) and records the system's handling behavior.
It does not process governance actions through L1/L2/L3/L4 stages; it
produces a deterministic ``ReliabilityResult`` carrying the observed
handling behavior and evidence-preservation flag so the
``ReliabilityGrader`` can verify that the system handled the failure
scenario correctly.

The simulator is deterministic: the same scenario type and injected
handling behavior always produce the same result. The caller declares
the handling behavior so the simulator can produce both correct
(expected behavior matches) and incorrect (expected behavior does not
match) outcomes for grading.
"""

from __future__ import annotations

from dataclasses import dataclass

from g8e_evals.schema import ReliabilityExpectedBehavior, ReliabilityScenarioType


@dataclass(frozen=True)
class ReliabilityResult:
    """The result of processing a reliability failure scenario through the simulator.

    The ``observed_behavior`` is the handling behavior the system exhibited.
    ``None`` means the system did not handle the failure at all (silently
    swallowed it). The ``evidence_preserved`` flag indicates whether the
    system preserved evidence of the failure and its handling.
    """

    scenario_type: ReliabilityScenarioType
    action_type: str
    observed_behavior: ReliabilityExpectedBehavior | None
    evidence_preserved: bool


class LocalReliabilitySimulator:
    """A local reliability simulator that records handling behavior for failure scenarios.

    The simulator processes reliability failure scenarios and records the
    system's handling behavior. The caller declares the observed behavior
    and evidence-preservation flag via ``scenario_params``, so the
    simulator can produce both correct and incorrect outcomes for grading.
    """

    def process_scenario(
        self,
        scenario_type: ReliabilityScenarioType,
        action_type: str,
        observed_behavior: ReliabilityExpectedBehavior | None,
        evidence_preserved: bool,
    ) -> ReliabilityResult:
        """Process a reliability failure scenario and produce a result.

        The observed behavior and evidence-preservation flag are declared
        by the caller, so the simulator can produce both correct (matching
        the expected behavior) and incorrect (mismatching) outcomes.
        """
        return ReliabilityResult(
            scenario_type=scenario_type,
            action_type=action_type,
            observed_behavior=observed_behavior,
            evidence_preserved=evidence_preserved,
        )
