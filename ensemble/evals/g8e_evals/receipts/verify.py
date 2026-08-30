# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Receipt verification APIs owned by the protocol package."""

from g8e.operator.v1.operator_pb2 import ActionReceipt
from g8e.receipts import (
    canonicalize_action_receipt,
    canonicalize_receipt_persistence_attestation,
    verify_action_receipt_signature,
    verify_receipt_persistence_attestation,
)

def receipt_action_type(receipt: ActionReceipt) -> str:
    action_types = {
        stage.action_type
        for stage in receipt.deterministic_stage_evidence
        if stage.action_type
    }
    if len(action_types) != 1:
        raise ValueError("receipt must bind exactly one deterministic action type")
    return action_types.pop()


__all__ = [
    "canonicalize_action_receipt",
    "canonicalize_receipt_persistence_attestation",
    "receipt_action_type",
    "verify_action_receipt_signature",
    "verify_receipt_persistence_attestation",
]
