# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local final-state simulator for the synthetic final-state eval suite.

The simulator is a production-shaped system under test that processes
actions and produces signed ``ActionReceipt`` objects with deterministic
state-root-before and state-root-after values.  It does not process
governance actions through L1/L2/L3/L4 stages; it produces a simple
signed receipt so the ``FinalStateAssertionGrader`` can verify that the
observed state-root transition matches the expected predicate (changed
or unchanged) with a verified source receipt.

The simulator uses Ed25519 signing keys (``nacl.signing``) to produce real
cryptographic signatures on receipts, so the receipt verification path is
exercised end-to-end.  The state-root-before and state-root-after values
are deterministic: the caller declares them explicitly, and the simulator
signs the receipt binding them.
"""

from __future__ import annotations

import hashlib
from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, datetime

import nacl.signing
from g8e.operator.v1.operator_pb2 import (
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
    EXECUTION_STATUS_COMPLETED,
    L2_STATUS_NOT_REQUIRED,
    L3_STATUS_NOT_REQUIRED,
    ActionReceipt,
)
from g8e.receipts import canonicalize_action_receipt

_SIGNER_KEY_ID = "final-state-sim-signer"


@dataclass(frozen=True)
class FinalStateResult:
    """The result of processing a final-state action through the simulator.

    The ``receipt`` is a signed ``ActionReceipt`` with the declared
    state-root-before and state-root-after values.  The ``state_root_before``
    and ``state_root_after`` fields are the deterministic state roots that
    the ``FinalStateAssertionGrader`` evaluates against the expected
    predicate.
    """

    receipt: ActionReceipt
    state_root_before: str
    state_root_after: str
    transaction_id: str


class LocalFinalStateSimulator:
    """A local final-state simulator that produces signed receipts with deterministic state roots.

    The simulator produces a signed ``ActionReceipt`` for each processed
    action.  The state-root-before and state-root-after values are declared
    by the caller, so the simulator can produce both changed and unchanged
    state-root transitions.  The receipt is signed with an Ed25519 key so
    the receipt verification path is exercised.
    """

    def __init__(
        self,
        *,
        now: Callable[[], datetime] | None = None,
        signing_key: nacl.signing.SigningKey | None = None,
    ) -> None:
        self._now = now or (lambda: datetime.now(UTC))
        self._signing_key = signing_key or nacl.signing.SigningKey.generate()
        self._tx_counter = 0

    @property
    def signer_key_id(self) -> str:
        return _SIGNER_KEY_ID

    @property
    def public_key_hex(self) -> str:
        return self._signing_key.verify_key.encode().hex()

    def _next_transaction_id(self) -> str:
        self._tx_counter += 1
        return f"final-state-sim-tx-{self._tx_counter}"

    def process_action(
        self,
        action_type: str,
        state_root_before: str,
        state_root_after: str,
    ) -> FinalStateResult:
        """Process an action and produce a signed receipt with the declared state roots.

        The receipt is signed with the simulator's Ed25519 key.  The
        state-root-before and state-root-after values are set exactly as
        declared, so the caller controls whether the predicate is
        ``state_root_changed`` or ``state_root_unchanged``.
        """
        tx_id = self._next_transaction_id()
        tx_hash = hashlib.sha256(f"{tx_id}:{action_type}".encode()).hexdigest()
        receipt = ActionReceipt(
            transaction_id=tx_id,
            transaction_hash=tx_hash,
            status=EXECUTION_STATUS_COMPLETED,
            result_summary="completed",
            state_root_before=state_root_before,
            state_root_after=state_root_after,
            executed_at_unix_ms=int(self._now().timestamp() * 1000),
            signer_key_id=_SIGNER_KEY_ID,
            l2_status=L2_STATUS_NOT_REQUIRED,
            l3_status=L3_STATUS_NOT_REQUIRED,
        )
        receipt.deterministic_stage_evidence.add(
            kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
            outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
            action_type=action_type,
            transaction_id=tx_id,
            transaction_hash=tx_hash,
        )
        receipt.signature = self._signing_key.sign(
            canonicalize_action_receipt(receipt)
        ).signature.hex()
        return FinalStateResult(
            receipt=receipt,
            state_root_before=state_root_before,
            state_root_after=state_root_after,
            transaction_id=tx_id,
        )
