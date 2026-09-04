# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local governance simulator for synthetic governance-adversarial eval suites.

The simulator is a production-shaped system under test that processes
governance actions through deterministic stages (L1 doctrine, L2 consensus,
L3 notary, L4 verification), produces signed ``ActionReceipt`` objects with
deterministic stage evidence, and tracks terminal state so observers can
prove absence of duplicate-acceptance, tampered-value acceptance, and
expired-nonce acceptance.

The simulator uses Ed25519 signing keys (``nacl.signing``) to produce real
cryptographic signatures on receipts, so the receipt verification path is
exercised end-to-end.  When an action is rejected at a declared layer, the
stages before that layer succeed, the declared layer fails, and L4
verification fails.  When an action is allowed, all stages succeed and L4
verifies.
"""

from __future__ import annotations

import hashlib
from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, datetime

import nacl.signing
from g8e.operator.v1.operator_pb2 import (
    DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
    DETERMINISTIC_STAGE_KIND_L3_NOTARY,
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
    DETERMINISTIC_STAGE_OUTCOME_FAILED,
    DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
    EXECUTION_STATUS_COMPLETED,
    EXECUTION_STATUS_FAILED,
    L2_STATUS_NOT_REQUIRED,
    L2_STATUS_REQUIRED_FAILED,
    L2_STATUS_REQUIRED_VALID,
    L3_STATUS_NOT_REQUIRED,
    L3_STATUS_REQUIRED_FAILED,
    L3_STATUS_REQUIRED_VALID,
    ActionReceipt,
)
from g8e.receipts import canonicalize_action_receipt

from g8e_evals.schema import RejectionLayer


_SIGNER_KEY_ID = "governance-sim-signer"


@dataclass(frozen=True)
class GovernanceActionResult:
    """The result of processing a governance action through the simulator.

    The ``receipt`` is a signed ``ActionReceipt`` with deterministic stage
    evidence.  The ``bad_state_materialized`` flag indicates whether the
    prohibited terminal state (duplicate-acceptance, tampered-value
    acceptance, or expired-nonce acceptance) materialized despite the
    rejection.  In a correctly governed system the flag is always ``False``
    when the action is rejected; the simulator can be configured to
    simulate a governance bypass where the bad state materializes.
    """

    receipt: ActionReceipt
    bad_state_materialized: bool
    transaction_id: str
    transaction_hash: str


class LocalGovernanceSimulator:
    """A local governance simulator that produces signed receipts with deterministic stage evidence.

    The simulator processes actions through L1, L2, L3, and L4 stages.  When
    configured to reject at a declared layer, the stages before that layer
    succeed, the declared layer fails, and L4 verification fails.  When
    configured to allow, all stages succeed and L4 verifies.  The receipt is
    signed with an Ed25519 key so the receipt verification path is exercised.
    """

    def __init__(
        self,
        *,
        now: Callable[[], datetime] | None = None,
        signing_key: nacl.signing.SigningKey | None = None,
    ) -> None:
        self._now = now or (lambda: datetime.now(UTC))
        self._signing_key = signing_key or nacl.signing.SigningKey.generate()
        self._accepted_transaction_ids: set[str] = set()
        self._accepted_field_values: dict[str, str] = {}
        self._accepted_nonces: dict[str, datetime] = {}
        self._state_root = "root-0"
        self._tx_counter = 0

    @property
    def signer_key_id(self) -> str:
        return _SIGNER_KEY_ID

    @property
    def public_key_hex(self) -> str:
        return self._signing_key.verify_key.encode().hex()

    def _next_transaction_id(self) -> str:
        self._tx_counter += 1
        return f"gov-sim-tx-{self._tx_counter}"

    def _build_stages(
        self,
        action_type: str,
        rejection_layer: RejectionLayer | None,
        transaction_id: str,
        transaction_hash: str,
    ) -> list[tuple[int, int]]:
        """Build the deterministic stage evidence list for a governance action.

        Returns a list of ``(kind, outcome)`` pairs.  When ``rejection_layer``
        is ``None`` the action is allowed: all stages verify and L4 verifies.
        When ``rejection_layer`` is set, stages before that layer verify, the
        declared layer fails, and L4 fails.
        """
        _LAYER_ORDER = [
            RejectionLayer.L1_DOCTRINE,
            RejectionLayer.L2_CONSENSUS,
            RejectionLayer.L3_NOTARY,
            RejectionLayer.L4_VERIFICATION,
        ]
        _STAGE_FOR_LAYER = {
            RejectionLayer.L1_DOCTRINE: DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
            RejectionLayer.L2_CONSENSUS: DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
            RejectionLayer.L3_NOTARY: DETERMINISTIC_STAGE_KIND_L3_NOTARY,
            RejectionLayer.L4_VERIFICATION: DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
        }

        stages: list[tuple[int, int]] = []
        if rejection_layer is None:
            for layer in _LAYER_ORDER:
                stages.append((_STAGE_FOR_LAYER[layer], DETERMINISTIC_STAGE_OUTCOME_VERIFIED))
            stages.append((DETERMINISTIC_STAGE_KIND_L5_EXECUTION, DETERMINISTIC_STAGE_OUTCOME_COMPLETED))
            return stages

        for layer in _LAYER_ORDER:
            if layer == rejection_layer:
                stages.append((_STAGE_FOR_LAYER[layer], DETERMINISTIC_STAGE_OUTCOME_FAILED))
                if layer != RejectionLayer.L4_VERIFICATION:
                    stages.append((DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, DETERMINISTIC_STAGE_OUTCOME_FAILED))
                return stages
            stages.append((_STAGE_FOR_LAYER[layer], DETERMINISTIC_STAGE_OUTCOME_VERIFIED))

        stages.append((DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, DETERMINISTIC_STAGE_OUTCOME_FAILED))
        return stages

    def _make_receipt(
        self,
        action_type: str,
        rejection_layer: RejectionLayer | None,
        transaction_id: str,
        transaction_hash: str,
        state_root_before: str,
    ) -> ActionReceipt:
        stages = self._build_stages(action_type, rejection_layer, transaction_id, transaction_hash)
        rejected = rejection_layer is not None
        receipt = ActionReceipt(
            transaction_id=transaction_id,
            transaction_hash=transaction_hash,
            status=EXECUTION_STATUS_FAILED if rejected else EXECUTION_STATUS_COMPLETED,
            result_summary="rejected" if rejected else "completed",
            state_root_before=state_root_before,
            state_root_after=state_root_before if rejected else f"root-{self._tx_counter}",
            executed_at_unix_ms=int(self._now().timestamp() * 1000),
            signer_key_id=_SIGNER_KEY_ID,
            l2_status=(
                L2_STATUS_REQUIRED_FAILED
                if rejection_layer == RejectionLayer.L2_CONSENSUS
                else (L2_STATUS_NOT_REQUIRED if rejected else L2_STATUS_REQUIRED_VALID)
            ),
            l3_status=(
                L3_STATUS_REQUIRED_FAILED
                if rejection_layer == RejectionLayer.L3_NOTARY
                else (L3_STATUS_NOT_REQUIRED if rejected else L3_STATUS_REQUIRED_VALID)
            ),
        )
        for kind, outcome in stages:
            receipt.deterministic_stage_evidence.add(
                kind=kind,
                outcome=outcome,
                action_type=action_type,
                transaction_id=transaction_id,
                transaction_hash=transaction_hash,
            )
        receipt.signature = self._signing_key.sign(
            canonicalize_action_receipt(receipt)
        ).signature.hex()
        return receipt

    def process_replay(
        self,
        action_type: str,
        replayed_transaction_id: str,
        replayed_transaction_hash: str,
        rejection_layer: RejectionLayer,
        *,
        allow_bypass: bool = False,
    ) -> GovernanceActionResult:
        """Process a replay attempt through the governance simulator.

        When ``allow_bypass`` is ``False`` (the correct behavior), the replay
        is rejected at the declared layer and the replayed transaction is not
        double-accepted.  When ``allow_bypass`` is ``True``, the replay is
        still rejected but the replayed transaction is double-accepted,
        simulating a governance bypass.
        """
        tx_id = self._next_transaction_id()
        tx_hash = hashlib.sha256(f"{tx_id}:{replayed_transaction_id}".encode()).hexdigest()
        state_before = self._state_root
        receipt = self._make_receipt(action_type, rejection_layer, tx_id, tx_hash, state_before)

        bad_state = False
        if allow_bypass:
            self._accepted_transaction_ids.add(replayed_transaction_id)
            bad_state = True
        elif rejection_layer is None:
            self._accepted_transaction_ids.add(replayed_transaction_id)
            bad_state = True

        if not bad_state:
            self._state_root = f"root-{self._tx_counter}"

        return GovernanceActionResult(
            receipt=receipt,
            bad_state_materialized=bad_state,
            transaction_id=tx_id,
            transaction_hash=tx_hash,
        )

    def is_transaction_double_accepted(self, transaction_id: str) -> bool:
        """Check whether a replayed transaction ID has been double-accepted."""
        return transaction_id in self._accepted_transaction_ids

    def process_signed_field_tampering(
        self,
        action_type: str,
        tampered_field: str,
        tampered_value: str,
        rejection_layer: RejectionLayer,
        *,
        allow_bypass: bool = False,
    ) -> GovernanceActionResult:
        """Process a signed-field tampering attempt through the governance simulator.

        When ``allow_bypass`` is ``False``, the tampered action is rejected at
        the declared layer and the tampered field value is not accepted.  When
        ``allow_bypass`` is ``True``, the tampered value is accepted despite
        the rejection, simulating a governance bypass.
        """
        tx_id = self._next_transaction_id()
        tx_hash = hashlib.sha256(f"{tx_id}:{tampered_field}:{tampered_value}".encode()).hexdigest()
        state_before = self._state_root
        receipt = self._make_receipt(action_type, rejection_layer, tx_id, tx_hash, state_before)

        bad_state = False
        if allow_bypass:
            self._accepted_field_values[tampered_field] = tampered_value
            bad_state = True
        elif rejection_layer is None:
            self._accepted_field_values[tampered_field] = tampered_value
            bad_state = True

        if not bad_state:
            self._state_root = f"root-{self._tx_counter}"

        return GovernanceActionResult(
            receipt=receipt,
            bad_state_materialized=bad_state,
            transaction_id=tx_id,
            transaction_hash=tx_hash,
        )

    def is_tampered_field_accepted(self, tampered_field: str, tampered_value: str) -> bool:
        """Check whether a tampered field value has been accepted."""
        return self._accepted_field_values.get(tampered_field) == tampered_value

    def process_nonce_expiration(
        self,
        action_type: str,
        nonce_value: str,
        declared_expiry_timestamp: datetime,
        rejection_layer: RejectionLayer,
        *,
        allow_bypass: bool = False,
    ) -> GovernanceActionResult:
        """Process an expired-nonce reuse attempt through the governance simulator.

        When ``allow_bypass`` is ``False``, the expired-nonce action is
        rejected at the declared layer and the expired nonce is not accepted
        as valid.  When ``allow_bypass`` is ``True``, the expired nonce is
        accepted as valid despite the rejection, simulating a governance
        bypass.
        """
        tx_id = self._next_transaction_id()
        tx_hash = hashlib.sha256(f"{tx_id}:{nonce_value}".encode()).hexdigest()
        state_before = self._state_root
        receipt = self._make_receipt(action_type, rejection_layer, tx_id, tx_hash, state_before)

        bad_state = False
        if allow_bypass:
            self._accepted_nonces[nonce_value] = declared_expiry_timestamp
            bad_state = True
        elif rejection_layer is None:
            self._accepted_nonces[nonce_value] = declared_expiry_timestamp
            bad_state = True

        if not bad_state:
            self._state_root = f"root-{self._tx_counter}"

        return GovernanceActionResult(
            receipt=receipt,
            bad_state_materialized=bad_state,
            transaction_id=tx_id,
            transaction_hash=tx_hash,
        )

    def is_expired_nonce_accepted(self, nonce_value: str) -> bool:
        """Check whether an expired nonce has been accepted as valid."""
        return nonce_value in self._accepted_nonces
