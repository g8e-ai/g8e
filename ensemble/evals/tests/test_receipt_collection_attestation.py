# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest

from g8e.operator.v1.operator_pb2 import ActionReceipt, ReceiptPersistenceAttestation
from g8e.receipts import (
    action_receipt_to_dict,
    verify_action_receipt_signature,
    verify_receipt_persistence_attestation,
)
from g8e_evals.receipts.collector import ReceiptCollector

pytestmark = pytest.mark.unit

PUBLIC_KEY_HEX = "03a107bff3ce10be1d70dd18e74bc09967e4d6309ba50d5f1ddc8664125531b8"
RECEIPT_SIGNATURE = "01d60a4fc5d5b2f33ab0208b14efde29fb37f8e8dc5db0a99b4962c3b28676c432b887b9ce3d1d94a31a345c3d5773dd37dd1e2da5a6cb9e89e2d00a029ef603"
ATTESTATION_SIGNATURE = "8fee4aa18c1beb8fc52b7ee371ff82a727f75430c672e694cf3d4cafc0f5dfb33a4a28ba79d405c7b3a4b6c61876f0a05a90f9a197b96039fd34d47c69ba9c03"


@pytest.mark.asyncio
async def test_collector_preserves_signature_valid_final_persistence_attestation():
    receipt = ActionReceipt(
        transaction_id="tx-golden-001",
        transaction_hash="sha256:8d969eef6ecad3c29a3a629280e686cff8ca6a85acebf4c7f2c5c8d5e6f7a8b9",
        status="EXECUTION_STATUS_COMPLETED",
        result_summary="completed <safely> & exactly",
        state_root_before="sha256:before",
        state_root_after="sha256:after",
        executed_at_unix_ms=1777777777123,
        signer_key_id="warden-golden-key",
        signature=RECEIPT_SIGNATURE,
        l2_status="L2_STATUS_REQUIRED_VALID",
        l3_status="L3_STATUS_REQUIRED_FAILED",
        final_persistence_attestation=ReceiptPersistenceAttestation(
            transaction_id="tx-golden-001",
            receipt_signature_digest="1545e4ab2c67f481d133c2f8a9e1fe3538e666e07285b1a7ba3e1bbcbe29acf9",
            persisted_at_unix_ms=1777777777456,
            audit_record_id="tx-golden-001",
            signer_key_id="warden-golden-key",
            signature=ATTESTATION_SIGNATURE,
        ),
    )
    response = MagicMock(status_code=200)
    response.json.return_value = action_receipt_to_dict(receipt)
    client = MagicMock()
    client.get = AsyncMock(return_value=response)
    client_context = MagicMock()
    client_context.__aenter__ = AsyncMock(return_value=client)
    client_context.__aexit__ = AsyncMock(return_value=None)
    auth = MagicMock()
    auth.auth_headers.return_value = {}
    auth.make_async_client.return_value = client_context

    collected = await ReceiptCollector("https://gateway:8443", auth=auth).collect_receipt(receipt.transaction_id)

    assert collected is not None
    assert collected.HasField("final_persistence_attestation")
    assert verify_action_receipt_signature(collected, PUBLIC_KEY_HEX)
    assert verify_receipt_persistence_attestation(collected, PUBLIC_KEY_HEX)


@pytest.mark.asyncio
async def test_collector_resolves_receipt_by_authoritative_investigation_correlation():
    receipt = ActionReceipt(
        transaction_id="tx-investigation-001",
        transaction_hash="hash-investigation-001",
    )
    response = MagicMock(status_code=200)
    response.json.return_value = action_receipt_to_dict(receipt)
    client = MagicMock()
    client.get = AsyncMock(return_value=response)
    client_context = MagicMock()
    client_context.__aenter__ = AsyncMock(return_value=client)
    client_context.__aexit__ = AsyncMock(return_value=None)
    auth = MagicMock()
    auth.auth_headers.return_value = {}
    auth.make_async_client.return_value = client_context

    collected = await ReceiptCollector(
        "https://gateway:8443", auth=auth
    ).collect_receipt_for_investigation("investigation-001")

    assert collected is not None
    assert collected.transaction_id == receipt.transaction_id
    client.get.assert_awaited_with(
        "https://gateway:8443/api/v1/audit/receipts",
        params={"investigation_id": "investigation-001"},
        headers={},
    )
