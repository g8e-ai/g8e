# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import base64
import binascii
import json
from pathlib import Path

import pytest
from click.testing import CliRunner
from nacl.signing import SigningKey

from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DeterministicStageEvidence,
    ReceiptPersistenceAttestation,
)
from g8e.receipts import (
    ED25519_SPKI_PREFIX,
    _signature_digest,
    canonicalize_action_receipt,
    canonicalize_receipt_persistence_attestation,
)
from g8e_evals.cli import main
from g8e_evals.schema import ReceiptObservation, RunManifest

pytestmark = pytest.mark.unit

_EXPECTED_FIELDS = {
    "schema_version",
    "run_id",
    "verified_at",
    "verifier_version",
    "scope",
    "total_receipts",
    "verified_signatures",
    "verified_persistence",
    "failed_signatures",
    "failed_persistence",
    "missing_keys",
    "distinct_signer_key_ids",
    "receipt_bound_eligible_attempts",
    "sample_receipt_fingerprints",
}


def _generate_keypair() -> tuple[str, str, SigningKey]:
    signing_key = SigningKey.generate()
    raw_public_key = bytes(signing_key.verify_key)
    key_id = binascii.hexlify(raw_public_key).decode()
    pem = (
        "-----BEGIN PUBLIC KEY-----\n"
        f"{base64.b64encode(ED25519_SPKI_PREFIX + raw_public_key).decode()}\n"
        "-----END PUBLIC KEY-----\n"
    )
    return key_id, pem, signing_key


def _write_key(tmp_path: Path, pem: str) -> Path:
    pki_dir = tmp_path / "pki"
    pki_dir.mkdir(exist_ok=True)
    (pki_dir / "gateway-Actuator_pub.pem").write_text(pem)
    return pki_dir


def _make_receipt(transaction_id: str, key_id: str, signing_key: SigningKey) -> ActionReceipt:
    receipt = ActionReceipt(
        transaction_id=transaction_id,
        transaction_hash=f"sha256:{transaction_id}",
        status="EXECUTION_STATUS_COMPLETED",
        result_summary="completed",
        state_root_before="sha256:before",
        state_root_after="sha256:after",
        executed_at_unix_ms=1777777777123,
        signer_key_id=key_id,
        l2_status="L2_STATUS_REQUIRED_VALID",
        l3_status="L3_STATUS_NOT_REQUIRED",
        deterministic_stage_evidence=[
            DeterministicStageEvidence(
                stage_id=f"stage-{transaction_id}",
                action_type="FILE_EDIT",
                transaction_id=transaction_id,
            ),
        ],
        final_persistence_attestation=ReceiptPersistenceAttestation(
            transaction_id=transaction_id,
            persisted_at_unix_ms=1777777777456,
            audit_record_id=transaction_id,
        ),
    )
    receipt.signature = binascii.hexlify(
        signing_key.sign(canonicalize_action_receipt(receipt)).signature
    ).decode()
    _sign_persistence(receipt, signing_key)
    return receipt


def _sign_persistence(receipt: ActionReceipt, signing_key: SigningKey) -> None:
    attestation = receipt.final_persistence_attestation
    attestation.receipt_signature_digest = _signature_digest([receipt.signature])
    attestation.signer_key_id = receipt.signer_key_id
    attestation.signature = binascii.hexlify(
        signing_key.sign(canonicalize_receipt_persistence_attestation(attestation)).signature
    ).decode()


def _write_report(report_dir: Path, receipts: list[ActionReceipt]) -> None:
    report_dir.mkdir()
    manifest = RunManifest(run_id="run-json-1", suite_id="test-suite", suite_version="1.0.0")
    (report_dir / "manifest.json").write_text(manifest.model_dump_json())
    with (report_dir / "receipts.jsonl").open("w") as output:
        for index, receipt in enumerate(receipts):
            observation = ReceiptObservation(
                receipt_id=f"receipt-{index}",
                attempt_id=f"attempt-{index}",
                run_id=manifest.run_id,
                transaction_id=receipt.transaction_id,
                action_type="FILE_EDIT",
                primary=True,
                verified=False,
                action_receipt=receipt,
            )
            output.write(observation.model_dump_json() + "\n")


def _invoke_json(report_dir: Path, pki_dir: Path):
    return CliRunner().invoke(
        main,
        ["verify-receipts", str(report_dir), "--pki-dir", str(pki_dir), "--json"],
    )


def test_verify_receipts_json_emits_typed_result_without_rich_output(tmp_path):
    key_id, pem, signing_key = _generate_keypair()
    pki_dir = _write_key(tmp_path, pem)
    report_dir = tmp_path / "report"
    _write_report(report_dir, [_make_receipt("tx-1", key_id, signing_key)])

    result = _invoke_json(report_dir, pki_dir)

    assert result.exit_code == 0, result.output
    payload = json.loads(result.output)
    assert set(payload) == _EXPECTED_FIELDS
    assert payload["run_id"] == "run-json-1"
    assert payload["total_receipts"] == 1
    assert payload["verified_signatures"] == 1
    assert payload["verified_persistence"] == 1
    assert payload["failed_signatures"] == 0
    assert payload["failed_persistence"] == 0
    assert payload["missing_keys"] == 0
    assert payload["distinct_signer_key_ids"] == [key_id]
    assert payload["receipt_bound_eligible_attempts"] == 1
    assert payload["sample_receipt_fingerprints"][0]["receipt_id"] == "receipt-0"
    assert "loaded key" not in result.output
    assert "Verifying receipts" not in result.output
    assert "[bold" not in result.output


def test_verify_receipts_json_reports_zero_receipts(tmp_path):
    _, pem, _ = _generate_keypair()
    pki_dir = _write_key(tmp_path, pem)
    report_dir = tmp_path / "report"
    _write_report(report_dir, [])

    result = _invoke_json(report_dir, pki_dir)

    assert result.exit_code == 0, result.output
    payload = json.loads(result.output)
    assert payload["total_receipts"] == 0
    assert payload["verified_signatures"] == 0
    assert payload["verified_persistence"] == 0
    assert payload["receipt_bound_eligible_attempts"] == 0
    assert payload["sample_receipt_fingerprints"] == []


def test_verify_receipts_json_reports_missing_signer_key(tmp_path):
    _, pem, _ = _generate_keypair()
    missing_key_id, _, missing_signing_key = _generate_keypair()
    pki_dir = _write_key(tmp_path, pem)
    report_dir = tmp_path / "report"
    _write_report(report_dir, [_make_receipt("tx-missing", missing_key_id, missing_signing_key)])

    result = _invoke_json(report_dir, pki_dir)

    assert result.exit_code == 1, result.output
    payload = json.loads(result.output)
    assert payload["total_receipts"] == 1
    assert payload["missing_keys"] == 1
    assert payload["verified_signatures"] == 0
    assert payload["verified_persistence"] == 0


def test_verify_receipts_json_reports_signature_failure_independently(tmp_path):
    key_id, pem, signing_key = _generate_keypair()
    pki_dir = _write_key(tmp_path, pem)
    receipt = _make_receipt("tx-invalid", key_id, signing_key)
    receipt.signature = "00"
    _sign_persistence(receipt, signing_key)
    report_dir = tmp_path / "report"
    _write_report(report_dir, [receipt])

    result = _invoke_json(report_dir, pki_dir)

    assert result.exit_code == 1, result.output
    payload = json.loads(result.output)
    assert payload["failed_signatures"] == 1
    assert payload["verified_signatures"] == 0
    assert payload["failed_persistence"] == 0
    assert payload["verified_persistence"] == 1
