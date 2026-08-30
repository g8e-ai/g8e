# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import base64
import json

import pytest

from g8e_evals.evidence import load_evidence_encryption_key

pytestmark = pytest.mark.integration


def test_load_evidence_encryption_key_reads_typed_owner_only_key_file(tmp_path) -> None:
    key_path = tmp_path / "evidence-key.json"
    key_path.write_text(json.dumps({
        "version": 1,
        "key_id": "eval-key-2026-08",
        "key_b64": base64.b64encode(b"k" * 32).decode(),
    }))
    key_path.chmod(0o600)

    key = load_evidence_encryption_key(key_path)

    assert key.key_id == "eval-key-2026-08"
    assert key.key == b"k" * 32


def test_load_evidence_encryption_key_rejects_group_readable_file(tmp_path) -> None:
    key_path = tmp_path / "evidence-key.json"
    key_path.write_text(json.dumps({
        "version": 1,
        "key_id": "eval-key-2026-08",
        "key_b64": base64.b64encode(b"k" * 32).decode(),
    }))
    key_path.chmod(0o640)

    with pytest.raises(ValueError, match="owner-only"):
        load_evidence_encryption_key(key_path)


def test_load_evidence_encryption_key_rejects_unknown_fields(tmp_path) -> None:
    key_path = tmp_path / "evidence-key.json"
    key_path.write_text(json.dumps({
        "version": 1,
        "key_id": "eval-key-2026-08",
        "key_b64": base64.b64encode(b"k" * 32).decode(),
        "unexpected": "value",
    }))
    key_path.chmod(0o600)

    with pytest.raises(ValueError, match="invalid evidence encryption key file"):
        load_evidence_encryption_key(key_path)
