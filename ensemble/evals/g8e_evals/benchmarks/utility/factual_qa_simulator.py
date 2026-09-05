# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local factual-QA simulator for the synthetic factual/domain-QA eval suite.

The simulator is a production-shaped system under test that records the
answer text produced by a model for a factual or domain-QA task.  It does
not process governance actions or produce signed receipts; it stores the
answer string so observers can prove that the observed answer satisfies the
declared match type against the expected answer.

The simulator is deterministic: the same input answer always produces the
same observed answer.  An optional ``inject_answer`` parameter allows the
simulator to override the recorded answer, simulating a model that produces
a wrong or differently formatted answer.
"""

from __future__ import annotations

import hashlib
from dataclasses import dataclass


@dataclass(frozen=True)
class FactualQAResult:
    """The result of recording a factual-QA answer through the simulator.

    The ``observed_answer`` is the answer text that was produced.  The
    ``answer_hash`` is a SHA-256 digest of the answer text, providing a
    content-addressed reference for evidence binding.
    """

    observed_answer: str
    answer_hash: str


class LocalFactualQASimulator:
    """A local factual-QA simulator that records answer text.

    The simulator records the answer text produced by the model and produces
    a content-addressed hash of the answer.  When ``inject_answer`` is set,
    the simulator overrides the recorded answer with the injected value,
    simulating a model that produces a wrong or differently formatted answer.
    """

    def __init__(self) -> None:
        self._answer: str = ""

    def set_answer(self, answer: str) -> None:
        """Record the answer text produced by the model."""
        self._answer = answer

    def inject_answer(self, answer: str) -> None:
        """Override the recorded answer with an injected value.

        This simulates a model that produces a wrong or differently
        formatted answer.  The injected answer replaces any previously
        recorded answer.
        """
        self._answer = answer

    def finish(self) -> FactualQAResult:
        """Produce the final factual-QA result with a content-addressed hash."""
        answer_hash = hashlib.sha256(self._answer.encode()).hexdigest()
        return FactualQAResult(
            observed_answer=self._answer,
            answer_hash=answer_hash,
        )

    @property
    def observed_answer(self) -> str:
        return self._answer
