# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Production observer implementations for the synthetic tool-sequence eval suite.

These observers interact with a ``LocalToolUseSimulator`` to produce typed
``ToolSequenceObservation`` records that the deterministic tool-sequence
grader consumes.  The observer implements the ``ToolSequenceObserver``
protocol from ``g8e_evals.harness`` and produces verified observations
bound to source evidence.
"""

from __future__ import annotations

from datetime import UTC, datetime

from g8e_evals.benchmarks.utility.factual_qa_simulator import LocalFactualQASimulator
from g8e_evals.benchmarks.utility.tool_use_simulator import LocalToolUseSimulator
from g8e_evals.schema import (
    AttemptRecord,
    FactualQAObservation,
    TaskDefinition,
    ToolSequenceObservation,
    VerificationStatus,
)


class ToolSequenceObserverImpl:
    """Observes tool-use sequences and produces typed observations.

    The observer records the tool invocation sequence from the simulator
    and produces a ``ToolSequenceObservation`` for each assertion on the
    task.  The observation captures the ordered list of tool names at
    the declared collection boundary, bound to source evidence.
    """

    def __init__(
        self,
        simulator: LocalToolUseSimulator,
        evidence_sha: str,
        evidence_ref: str,
    ) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[ToolSequenceObservation]:
        result = self._simulator.finish()
        observations: list[ToolSequenceObservation] = []

        for assertion in task.tool_sequence_assertions:
            observations.append(ToolSequenceObservation(
                observation_id=f"{attempt.attempt_id}:tool-sequence:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                observed_sequence=result.observed_sequence,
                collection_boundary=assertion.collection_boundary,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return observations


class FactualQAObserverImpl:
    """Observes factual-QA answers and produces typed observations.

    The observer records the answer text from the simulator and produces a
    ``FactualQAObservation`` for each assertion on the task.  The observation
    captures the answer string at the declared collection boundary, bound to
    source evidence.
    """

    def __init__(
        self,
        simulator: LocalFactualQASimulator,
        evidence_sha: str,
        evidence_ref: str,
    ) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[FactualQAObservation]:
        result = self._simulator.finish()
        observations: list[FactualQAObservation] = []

        for assertion in task.factual_qa_assertions:
            observations.append(FactualQAObservation(
                observation_id=f"{attempt.attempt_id}:factual-qa:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                observed_answer=result.observed_answer,
                collection_boundary=assertion.collection_boundary,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return observations
