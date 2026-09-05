# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local tool-use simulator for the synthetic tool-sequence eval suite.

The simulator is a production-shaped system under test that records the
ordered sequence of tool calls invoked by a model during a task.  It
does not process governance actions or produce signed receipts; it
tracks the tool invocation order so observers can prove that the
observed sequence matches or avoids the declared expected sequence.

The simulator is deterministic: the same input tool list always produces
the same observed sequence.  An optional ``inject_forbidden`` parameter
allows the simulator to inject a forbidden tool into the sequence,
simulating a model that violates an avoid constraint.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class ToolUseResult:
    """The result of recording a tool-use sequence through the simulator.

    The ``observed_sequence`` is the ordered list of tool names that were
    invoked.  The ``sequence_hash`` is a SHA-256 digest of the joined
    sequence, providing a content-addressed reference for evidence
    binding.
    """

    observed_sequence: list[str]
    sequence_hash: str


class LocalToolUseSimulator:
    """A local tool-use simulator that records ordered tool invocations.

    The simulator records tool calls in the order they are invoked and
    produces a content-addressed hash of the joined sequence.  When
    ``inject_forbidden`` is set, the simulator inserts the forbidden tool
    name at the declared position in the sequence, simulating a model
    that violates an avoid constraint.
    """

    def __init__(self) -> None:
        self._invoked_tools: list[str] = []

    def invoke(self, tool_name: str) -> None:
        """Record a single tool invocation in order."""
        self._invoked_tools.append(tool_name)

    def invoke_sequence(self, tool_names: list[str]) -> None:
        """Record a sequence of tool invocations in order."""
        self._invoked_tools.extend(tool_names)

    def inject_forbidden(self, forbidden_tool: str, position: int) -> None:
        """Insert a forbidden tool at the declared position in the sequence.

        This simulates a model that violates an avoid constraint by
        invoking a forbidden tool.  The position is clamped to the valid
        range of the current sequence.
        """
        clamped = max(0, min(position, len(self._invoked_tools)))
        self._invoked_tools.insert(clamped, forbidden_tool)

    def finish(self) -> ToolUseResult:
        """Produce the final tool-use result with a content-addressed hash."""
        import hashlib
        joined = ":".join(self._invoked_tools)
        sequence_hash = hashlib.sha256(joined.encode()).hexdigest()
        return ToolUseResult(
            observed_sequence=list(self._invoked_tools),
            sequence_hash=sequence_hash,
        )

    @property
    def observed_sequence(self) -> list[str]:
        return list(self._invoked_tools)
