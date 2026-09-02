# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for the evals CLI ``verify-receipts`` command.

These cover the multi-signer fix: unified-stack runs produce receipts signed
by two distinct actuators (gateway and operator), so the verifier must load
every ``*Actuator_pub.pem`` file in the PKI directory and match each receipt
to its ``signer_key_id``. The prior implementation hardcoded a single
``warden_pub.pem`` and used it for all receipts, which failed for any receipt
signed by a different actuator.
"""
from __future__ import annotations

import base64
import binascii
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
    canonicalize_action_receipt,
    canonicalize_receipt_persistence_attestation,
    _signature_digest,
)
from g8e_evals.cli import main
from g8e_evals.schema import ReceiptObservation

pytestmark = pytest.mark.unit


def _generate_keypair() -> tuple[str, str, str]:
    """Generate an Ed25519 keypair and return (key_id_hex, pem, signing_key_hex)."""
    signing_key = SigningKey.generate()
    verify_key = signing_key.verify_key
    raw_pub = bytes(verify_key)
    key_id = binascii.hexlify(raw_pub).decode()
    der = ED25519_SPKI_PREFIX + raw_pub
    pem_b64 = base64.b64encode(der).decode()
    pem = f"-----BEGIN PUBLIC KEY-----\n{pem_b64}\n-----END PUBLIC KEY-----\n"
    return key_id, pem, binascii.hexlify(bytes(signing_key)).decode()


def _write_pem(tmp_path: Path, name: str, pem: str) -> Path:
    pki_dir = tmp_path / "pki"
    pki_dir.mkdir(exist_ok=True)
    path = pki_dir / name
    path.write_text(pem)
    return pki_dir


def _sign_receipt(receipt: ActionReceipt, signing_key_hex: str) -> None:
    """Sign the receipt and its persistence attestation in place."""
    signing_key = SigningKey(binascii.unhexlify(signing_key_hex))
    canonical = canonicalize_action_receipt(receipt)
    receipt.signature = binascii.hexlify(signing_key.sign(canonical).signature).decode()

    attestation = receipt.final_persistence_attestation
    attestation.receipt_signature_digest = _signature_digest([receipt.signature])
    attestation.signer_key_id = receipt.signer_key_id
    canonical_att = canonicalize_receipt_persistence_attestation(attestation)
    attestation.signature = binascii.hexlify(signing_key.sign(canonical_att).signature).decode()


def _make_receipt(
    tx_id: str,
    signer_key_id: str,
    signing_key_hex: str,
    action_type: str = "FILE_EDIT",
) -> ActionReceipt:
    receipt = ActionReceipt(
        transaction_id=tx_id,
        transaction_hash=f"sha256:{tx_id}",
        status="EXECUTION_STATUS_COMPLETED",
        result_summary="completed",
        state_root_before="sha256:before",
        state_root_after="sha256:after",
        executed_at_unix_ms=1777777777123,
        signer_key_id=signer_key_id,
        l2_status="L2_STATUS_REQUIRED_VALID",
        l3_status="L3_STATUS_NOT_REQUIRED",
        deterministic_stage_evidence=[
            DeterministicStageEvidence(
                stage_id=f"stage-{tx_id}",
                action_type=action_type,
                transaction_id=tx_id,
            ),
        ],
        final_persistence_attestation=ReceiptPersistenceAttestation(
            transaction_id=tx_id,
            persisted_at_unix_ms=1777777777456,
            audit_record_id=tx_id,
        ),
    )
    _sign_receipt(receipt, signing_key_hex)
    return receipt


def _write_receipts(report_dir: Path, receipts: list[ActionReceipt]) -> Path:
    report_dir.mkdir(parents=True, exist_ok=True)
    path = report_dir / "receipts.jsonl"

    with open(path, "w") as f:
        for i, receipt in enumerate(receipts):
            obs = ReceiptObservation(
                schema_version="1.0",
                receipt_id=f"receipt-{i}",
                attempt_id=f"attempt-{i}",
                run_id="run-1",
                transaction_id=receipt.transaction_id,
                action_type="FILE_EDIT",
                primary=True,
                verified=False,
                action_receipt=receipt,
            )
            f.write(obs.model_dump_json() + "\n")
    return path


def test_verify_receipts_multi_signer_both_keys_loaded(tmp_path):
    """Two receipts signed by two different actuators both verify when both
    PEM files are present in the PKI directory."""
    gw_key_id, gw_pem, gw_priv = _generate_keypair()
    op_key_id, op_pem, op_priv = _generate_keypair()

    pki_dir = _write_pem(tmp_path, "gateway-Actuator_pub.pem", gw_pem)
    _write_pem(tmp_path, "operator-Actuator_pub.pem", op_pem)

    receipt_gw = _make_receipt("tx-gw-001", gw_key_id, gw_priv)
    receipt_op = _make_receipt("tx-op-001", op_key_id, op_priv)

    report_dir = tmp_path / "report"
    _write_receipts(report_dir, [receipt_gw, receipt_op])

    runner = CliRunner()
    result = runner.invoke(main, ["verify-receipts", str(report_dir), "--pki-dir", str(pki_dir)])

    assert result.exit_code == 0, result.output
    assert "Verified: 2" in result.output
    assert "Failed: 0" in result.output
    assert "Keys loaded: 2" in result.output
    assert "No-key receipts: 0" in result.output


def test_verify_receipts_multi_signer_missing_key_fails(tmp_path):
    """A receipt whose signer_key_id has no matching PEM fails verification."""
    gw_key_id, gw_pem, gw_priv = _generate_keypair()
    op_key_id, _, op_priv = _generate_keypair()

    pki_dir = _write_pem(tmp_path, "gateway-Actuator_pub.pem", gw_pem)
    # Operator PEM intentionally omitted.

    receipt_gw = _make_receipt("tx-gw-001", gw_key_id, gw_priv)
    receipt_op = _make_receipt("tx-op-001", op_key_id, op_priv)

    report_dir = tmp_path / "report"
    _write_receipts(report_dir, [receipt_gw, receipt_op])

    runner = CliRunner()
    result = runner.invoke(main, ["verify-receipts", str(report_dir), "--pki-dir", str(pki_dir)])

    assert result.exit_code == 1, result.output
    assert "Failed: 1" in result.output
    assert "No-key receipts: 1" in result.output
    assert "Verified: 1" in result.output


def test_verify_receipts_no_pem_files_errors(tmp_path):
    """An empty PKI directory produces a clear error, not a silent pass."""
    pki_dir = tmp_path / "pki"
    pki_dir.mkdir()
    report_dir = tmp_path / "report"
    report_dir.mkdir()
    (report_dir / "receipts.jsonl").write_text("")

    runner = CliRunner()
    result = runner.invoke(main, ["verify-receipts", str(report_dir), "--pki-dir", str(pki_dir)])

    assert result.exit_code == 1, result.output
    assert "No actuator public keys" in result.output


def test_verify_receipts_single_signer_still_works(tmp_path):
    """A single-actuator run with one PEM file verifies correctly."""
    key_id, pem, priv = _generate_keypair()
    pki_dir = _write_pem(tmp_path, "gateway-Actuator_pub.pem", pem)

    receipt = _make_receipt("tx-single-001", key_id, priv)
    report_dir = tmp_path / "report"
    _write_receipts(report_dir, [receipt])

    runner = CliRunner()
    result = runner.invoke(main, ["verify-receipts", str(report_dir), "--pki-dir", str(pki_dir)])

    assert result.exit_code == 0, result.output
    assert "Verified: 1" in result.output
    assert "Keys loaded: 1" in result.output
