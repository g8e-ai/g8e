# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Production observer implementations for the synthetic utility eval suites.

These observers interact with a ``LocalToolUseSimulator``,
``LocalFactualQASimulator``, or ``LocalCitationBackedSimulator`` to
produce typed observation records that the deterministic graders consume.
Each observer implements the corresponding observer protocol from
``g8e_evals.harness`` and produces verified observations bound to source
evidence.
"""

from __future__ import annotations

from datetime import UTC, datetime

from g8e_evals.benchmarks.utility.citation_backed_simulator import LocalCitationBackedSimulator
from g8e_evals.benchmarks.utility.factual_qa_simulator import LocalFactualQASimulator
from g8e_evals.benchmarks.utility.partial_milestone_simulator import LocalPartialMilestoneSimulator
from g8e_evals.benchmarks.utility.tool_use_simulator import LocalToolUseSimulator
from g8e_evals.schema import (
    AttemptRecord,
    CitationBackedObservation,
    FactualQAObservation,
    PartialMilestoneObservation,
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


class CitationBackedObserverImpl:
    """Observes citation-backed answers and produces typed observations.

    The observer records the citation text from the simulator and produces
    a ``CitationBackedObservation`` for each assertion on the task.  The
    observation captures the citation string at the declared collection
    boundary, bound to source evidence.
    """

    def __init__(
        self,
        simulator: LocalCitationBackedSimulator,
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
    ) -> list[CitationBackedObservation]:
        result = self._simulator.finish()
        observations: list[CitationBackedObservation] = []

        for assertion in task.citation_backed_assertions:
            observations.append(CitationBackedObservation(
                observation_id=f"{attempt.attempt_id}:citation-backed:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                observed_citation=result.observed_citation,
                collection_boundary=assertion.collection_boundary,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return observations


class PartialMilestoneObserverImpl:
    """Observes reached milestones and produces typed observations.

    The observer records the reached milestones from the simulator and
    produces a ``PartialMilestoneObservation`` for each assertion on the
    task.  The observation captures whether the declared milestone was
    reached and at what order index, bound to source evidence.
    """

    def __init__(
        self,
        simulator: LocalPartialMilestoneSimulator,
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
    ) -> list[PartialMilestoneObservation]:
        result = self._simulator.finish()
        reached_by_order = {m.order: m for m in result.milestones}
        observations: list[PartialMilestoneObservation] = []

        for assertion in task.partial_milestone_assertions:
            reached_milestone = reached_by_order.get(assertion.expected_order)
            milestone_reached = reached_milestone is not None
            observed_label = reached_milestone.label if reached_milestone else ""
            observed_order = reached_milestone.order if reached_milestone else None
            observations.append(PartialMilestoneObservation(
                observation_id=f"{attempt.attempt_id}:partial-milestone:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                milestone_reached=milestone_reached,
                observed_label=observed_label,
                observed_order=observed_order,
                collection_boundary=assertion.collection_boundary,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))

        return observations
