# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local ledger-consistency simulator for the synthetic ledger-consistency eval suite.

The simulator is a production-shaped system under test that maintains a
deterministic ledger head and entry count.  It does not process governance
actions or produce signed receipts; it tracks the ledger state so observers
can prove that the independently observed ledger state matches the expected
consistency, entry count, and head hash at the declared collection boundary.

The simulator is deterministic: the same input ledger entries always
produce the same head hash and entry count.  An optional ``inject_inconsistency``
parameter allows the simulator to produce an inconsistent ledger state,
simulating a ledger that has been tampered with or has diverged from the
expected state.
"""

from __future__ import annotations

import hashlib
from dataclasses import dataclass


@dataclass(frozen=True)
class LedgerEntry:
    """One ledger entry with its sequence number and payload hash."""

    sequence: int
    payload_hash: str


@dataclass(frozen=True)
class LedgerConsistencyResult:
    """The result of observing the ledger state through the simulator.

    The ``head_sha256`` is the SHA-256 digest of the ledger head (the
    concatenation of all entry hashes).  The ``entry_count`` is the total
    number of entries in the ledger.  The ``consistent`` flag indicates
    whether the ledger is internally consistent (no gaps in sequence
    numbers and no duplicate entries).
    """

    head_sha256: str
    entry_count: int
    consistent: bool


class LocalLedgerConsistencySimulator:
    """A local ledger-consistency simulator that maintains a deterministic ledger.

    The simulator maintains a list of ledger entries and produces a
    content-addressed head hash.  When ``inject_inconsistency`` is set,
    the simulator produces an inconsistent ledger state (a gap in sequence
    numbers or a duplicated entry), simulating a ledger that has been
    tampered with.
    """

    def __init__(self) -> None:
        self._entries: list[LedgerEntry] = []

    def append_entry(self, payload: str) -> None:
        """Append a new entry to the ledger with the next sequence number."""
        sequence = len(self._entries) + 1
        payload_hash = hashlib.sha256(f"{sequence}:{payload}".encode()).hexdigest()
        self._entries.append(LedgerEntry(sequence=sequence, payload_hash=payload_hash))

    def append_entries(self, payloads: list[str]) -> None:
        """Append multiple entries to the ledger in order."""
        for payload in payloads:
            self.append_entry(payload)

    def inject_inconsistency(self) -> None:
        """Inject an inconsistency into the ledger by duplicating an entry.

        This simulates a ledger that has been tampered with.  The duplicate
        entry creates an inconsistent state that the observer should detect.
        """
        if not self._entries:
            return
        duplicate = self._entries[-1]
        self._entries.append(LedgerEntry(sequence=duplicate.sequence, payload_hash=duplicate.payload_hash))

    def inject_sequence_gap(self) -> None:
        """Inject a sequence gap into the ledger by appending a non-sequential entry.

        This simulates a ledger where entries are missing.  The gap creates
        an inconsistent state that the observer should detect.
        """
        next_sequence = (len(self._entries) + 1) * 2
        payload_hash = hashlib.sha256(f"{next_sequence}:gap".encode()).hexdigest()
        self._entries.append(LedgerEntry(sequence=next_sequence, payload_hash=payload_hash))

    def finish(self) -> LedgerConsistencyResult:
        """Produce the final ledger-consistency result with a content-addressed head hash."""
        if not self._entries:
            return LedgerConsistencyResult(
                head_sha256=hashlib.sha256(b"").hexdigest(),
                entry_count=0,
                consistent=True,
            )
        content = "|".join(f"{e.sequence}:{e.payload_hash}" for e in self._entries)
        head_sha256 = hashlib.sha256(content.encode()).hexdigest()
        sequences = [e.sequence for e in self._entries]
        expected_sequences = list(range(1, len(self._entries) + 1))
        consistent = sequences == expected_sequences
        return LedgerConsistencyResult(
            head_sha256=head_sha256,
            entry_count=len(self._entries),
            consistent=consistent,
        )

    @property
    def entries(self) -> list[LedgerEntry]:
        return list(self._entries)

    @property
    def entry_count(self) -> int:
        return len(self._entries)
