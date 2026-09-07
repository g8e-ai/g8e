# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Production observer implementations for the synthetic reliability eval suite.

These observers interact with a ``LocalReliabilitySimulator`` to produce
typed ``ReliabilityObservation`` records that the deterministic graders
consume. The observer implements the ``ReliabilityObserver`` protocol
from ``g8e_evals.harness`` and produces verified observations bound to
source evidence.

The ``scenario_params`` mapping carries per-assertion observed behavior
and evidence-preservation flags keyed by assertion ID, so the CLI observer
setup can configure both correct and incorrect handling outcomes from the
scenario dataset without embedding them in typed assertion fields.
"""

from __future__ import annotations

from datetime import UTC, datetime

from g8e_evals.benchmarks.reliability.simulator import LocalReliabilitySimulator
from g8e_evals.schema import (
    AttemptRecord,
    ReliabilityExpectedBehavior,
    ReliabilityObservation,
    TaskDefinition,
    VerificationStatus,
)


def _evidence_binding(evidence_sha: str) -> tuple[str | None, VerificationStatus]:
    """Return (source_evidence_sha256, verification_status) based on whether
    the evidence SHA is known at observation time.

    When the evidence SHA is not yet known (empty string), observations are
    created in PENDING state with no SHA. The caller updates them to VERIFIED
    after persisting the evidence artifact and computing its digest.
    """
    if evidence_sha:
        return evidence_sha, VerificationStatus.VERIFIED
    return None, VerificationStatus.PENDING


class ReliabilityObserverImpl:
    """Observes reliability failure-scenario handling and produces typed observations.

    The observer processes each reliability assertion through the
    ``LocalReliabilitySimulator``, using per-assertion scenario parameters
    to declare the observed handling behavior and evidence-preservation
    flag. It produces a ``ReliabilityObservation`` for each assertion,
    bound to source evidence.
    """

    def __init__(
        self,
        simulator: LocalReliabilitySimulator,
        scenario_params: dict[str, dict[str, str | bool | None]],
        evidence_sha: str,
        evidence_ref: str,
    ) -> None:
        self._simulator = simulator
        self._scenario_params = scenario_params
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[ReliabilityObservation]:
        observations: list[ReliabilityObservation] = []
        _sha, _status = _evidence_binding(self._evidence_sha)

        for assertion in task.reliability_assertions:
            params = self._scenario_params.get(assertion.assertion_id, {})
            observed_behavior_raw = params.get("observed_behavior")
            observed_behavior: ReliabilityExpectedBehavior | None = (
                ReliabilityExpectedBehavior(observed_behavior_raw)
                if observed_behavior_raw is not None
                else None
            )
            evidence_preserved = bool(params.get("evidence_preserved", False))
            result = self._simulator.process_scenario(
                scenario_type=assertion.scenario_type,
                action_type=assertion.action_type,
                observed_behavior=observed_behavior,
                evidence_preserved=evidence_preserved,
            )
            observations.append(ReliabilityObservation(
                observation_id=f"{attempt.attempt_id}:reliability:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                scenario_type=result.scenario_type,
                action_type=result.action_type,
                observed_behavior=result.observed_behavior,
                evidence_preserved=result.evidence_preserved,
                collection_boundary=assertion.collection_boundary,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=_sha,
                verification_status=_status,
            ))

        return observations
