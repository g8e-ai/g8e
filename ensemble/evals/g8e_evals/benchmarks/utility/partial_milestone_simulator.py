# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local partial-milestone simulator for the synthetic partial-milestone eval suite.

The simulator is a production-shaped system under test that records the
intermediate milestones reached by a model during a long-horizon task.  It
does not process governance actions or produce signed receipts; it stores
the reached milestones so observers can prove that each declared milestone
was reached at the declared order index.

The simulator is deterministic: the same input milestones always produce
the same observed milestones.  An optional ``inject_milestones`` parameter
allows the simulator to override the recorded milestones, simulating a model
that skips, reorders, or misses milestones.
"""

from __future__ import annotations

import hashlib
from dataclasses import dataclass


@dataclass(frozen=True)
class MilestoneRecord:
    """One reached milestone with its observed label and order index."""

    label: str
    order: int


@dataclass(frozen=True)
class PartialMilestoneResult:
    """The result of recording milestones through the simulator.

    The ``milestones`` list is the ordered list of reached milestones.  The
    ``milestone_hash`` is a SHA-256 digest of the milestone records,
    providing a content-addressed reference for evidence binding.
    """

    milestones: tuple[MilestoneRecord, ...]
    milestone_hash: str


class LocalPartialMilestoneSimulator:
    """A local partial-milestone simulator that records reached milestones.

    The simulator records the milestones reached by the model and produces a
    content-addressed hash of the milestone records.  When
    ``inject_milestones`` is set, the simulator overrides the recorded
    milestones with the injected values, simulating a model that skips,
    reorders, or misses milestones.
    """

    def __init__(self) -> None:
        self._milestones: list[MilestoneRecord] = []

    def set_milestones(self, milestones: list[MilestoneRecord]) -> None:
        """Record the milestones reached by the model."""
        self._milestones = list(milestones)

    def inject_milestones(self, milestones: list[MilestoneRecord]) -> None:
        """Override the recorded milestones with injected values.

        This simulates a model that skips, reorders, or misses milestones.
        The injected milestones replace any previously recorded milestones.
        """
        self._milestones = list(milestones)

    def add_milestone(self, label: str, order: int) -> None:
        """Append a single reached milestone to the recorded list."""
        self._milestones.append(MilestoneRecord(label=label, order=order))

    def finish(self) -> PartialMilestoneResult:
        """Produce the final partial-milestone result with a content-addressed hash."""
        content = "|".join(f"{m.label}:{m.order}" for m in self._milestones)
        milestone_hash = hashlib.sha256(content.encode()).hexdigest()
        return PartialMilestoneResult(
            milestones=tuple(self._milestones),
            milestone_hash=milestone_hash,
        )

    @property
    def milestones(self) -> list[MilestoneRecord]:
        return list(self._milestones)

    @property
    def milestone_labels(self) -> list[str]:
        return [m.label for m in self._milestones]

    @property
    def milestone_orders(self) -> list[int]:
        return [m.order for m in self._milestones]
