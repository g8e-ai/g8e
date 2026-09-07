# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Production observer implementations for the synthetic economics and performance eval suite.

These observers interact with a ``LocalEconomicsPerformanceSimulator`` to
produce typed ``EconomicsPerformanceObservation`` records that the
deterministic graders consume. The observer implements the
``EconomicsPerformanceObserver`` protocol from ``g8e_evals.harness`` and
produces verified observations bound to source evidence.

The ``scenario_params`` mapping carries per-assertion observed values
keyed by assertion ID, so the CLI observer setup can configure both
correct (within tolerance) and incorrect (outside tolerance) measurement
outcomes from the scenario dataset without embedding them in typed
assertion fields.
"""

from __future__ import annotations

from datetime import UTC, datetime

from g8e_evals.benchmarks.economics.simulator import LocalEconomicsPerformanceSimulator
from g8e_evals.schema import (
    AttemptRecord,
    EconomicsPerformanceObservation,
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


class EconomicsPerformanceObserverImpl:
    """Observes economics and performance measurements and produces typed observations.

    The observer processes each economics-performance assertion through
    the ``LocalEconomicsPerformanceSimulator``, using per-assertion
    scenario parameters to declare the observed value. It produces an
    ``EconomicsPerformanceObservation`` for each assertion, bound to
    source evidence.
    """

    def __init__(
        self,
        simulator: LocalEconomicsPerformanceSimulator,
        scenario_params: dict[str, dict[str, float | str | None]],
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
    ) -> list[EconomicsPerformanceObservation]:
        observations: list[EconomicsPerformanceObservation] = []
        _sha, _status = _evidence_binding(self._evidence_sha)

        for assertion in task.economics_performance_assertions:
            params = self._scenario_params.get(assertion.assertion_id, {})
            observed_value_raw = params.get("observed_value")
            observed_value: float | None = (
                float(observed_value_raw) if observed_value_raw is not None else None
            )
            result = self._simulator.process_measurement(
                metric_kind=assertion.metric_kind,
                role=assertion.role,
                action_class=assertion.action_class,
                task_complexity=assertion.task_complexity,
                observed_value=observed_value,
                unit=assertion.unit,
            )
            observations.append(EconomicsPerformanceObservation(
                observation_id=f"{attempt.attempt_id}:economics_performance:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                metric_kind=result.metric_kind,
                role=result.role,
                action_class=result.action_class,
                task_complexity=result.task_complexity,
                observed_value=result.observed_value,
                unit=result.unit,
                collection_boundary=assertion.collection_boundary,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=_sha,
                verification_status=_status,
            ))

        return observations
