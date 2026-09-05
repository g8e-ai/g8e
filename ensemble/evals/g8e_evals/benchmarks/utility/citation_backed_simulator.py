# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local citation-backed simulator for the synthetic citation-backed eval suite.

The simulator is a production-shaped system under test that records the
citation text produced by a model for a citation-backed answer task.  It
does not process governance actions or produce signed receipts; it stores
the citation string so observers can prove that the observed citation
satisfies the declared match type against the expected citation.

The simulator is deterministic: the same input citation always produces
the same observed citation.  An optional ``inject_citation`` parameter
allows the simulator to override the recorded citation, simulating a model
that produces a wrong or differently formatted citation.
"""

from __future__ import annotations

import hashlib
from dataclasses import dataclass


@dataclass(frozen=True)
class CitationBackedResult:
    """The result of recording a citation-backed answer through the simulator.

    The ``observed_citation`` is the citation text that was produced.  The
    ``citation_hash`` is a SHA-256 digest of the citation text, providing a
    content-addressed reference for evidence binding.
    """

    observed_citation: str
    citation_hash: str


class LocalCitationBackedSimulator:
    """A local citation-backed simulator that records citation text.

    The simulator records the citation text produced by the model and
    produces a content-addressed hash of the citation.  When
    ``inject_citation`` is set, the simulator overrides the recorded
    citation with the injected value, simulating a model that produces a
    wrong or differently formatted citation.
    """

    def __init__(self) -> None:
        self._citation: str = ""

    def set_citation(self, citation: str) -> None:
        """Record the citation text produced by the model."""
        self._citation = citation

    def inject_citation(self, citation: str) -> None:
        """Override the recorded citation with an injected value.

        This simulates a model that produces a wrong or differently
        formatted citation.  The injected citation replaces any
        previously recorded citation.
        """
        self._citation = citation

    def finish(self) -> CitationBackedResult:
        """Produce the final citation-backed result with a content-addressed hash."""
        citation_hash = hashlib.sha256(self._citation.encode()).hexdigest()
        return CitationBackedResult(
            observed_citation=self._citation,
            citation_hash=citation_hash,
        )

    @property
    def observed_citation(self) -> str:
        return self._citation
