# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import base64
import json
from pathlib import Path

import pytest
from google.protobuf.json_format import ParseError

from g8e.operator.v1.operator_pb2 import (
    L2_STATUS_REQUIRED_VALID,
    L3_STATUS_REQUIRED_FAILED,
)
from g8e.receipts import (
    ED25519_SPKI_PREFIX,
    PUBLIC_KEY_PEM_BEGIN,
    PUBLIC_KEY_PEM_END,
    canonicalize_action_receipt,
    parse_action_receipt,
    verify_action_receipt_signature,
)

VECTORS_DIRECTORY_NAME = "vectors"
VECTOR_FILENAME = "action_receipt_canonicalization.json"
VECTOR_PATH = Path(__file__).resolve().parents[2] / VECTORS_DIRECTORY_NAME / VECTOR_FILENAME


@pytest.fixture
def vector() -> dict[str, object]:
    return json.loads(VECTOR_PATH.read_text())


def test_action_receipt_matches_cross_language_vector(vector):
    receipt = parse_action_receipt(vector["receipt"])
    assert receipt.l2_status == L2_STATUS_REQUIRED_VALID
    assert receipt.l3_status == L3_STATUS_REQUIRED_FAILED
    assert canonicalize_action_receipt(receipt).decode() == vector["canonical_utf8"]
    assert verify_action_receipt_signature(receipt, vector["public_key_hex"])


def test_action_receipt_verification_rejects_l2_tampering(vector):
    receipt = parse_action_receipt(vector["receipt"])
    receipt.l2_status = 1
    assert not verify_action_receipt_signature(receipt, vector["public_key_hex"])


def test_action_receipt_verification_accepts_ed25519_spki_pem(vector):
    receipt = parse_action_receipt(vector["receipt"])
    der_key = ED25519_SPKI_PREFIX + bytes.fromhex(vector["public_key_hex"])
    encoded_key = base64.b64encode(der_key).decode()
    pem_key = f"{PUBLIC_KEY_PEM_BEGIN}\n{encoded_key}\n{PUBLIC_KEY_PEM_END}\n"
    assert verify_action_receipt_signature(receipt, pem_key)


def test_action_receipt_parser_rejects_unknown_fields(vector):
    receipt_data = dict(vector["receipt"])
    receipt_data["unknown_proof"] = "must fail closed"
    with pytest.raises(ParseError):
        parse_action_receipt(receipt_data)
