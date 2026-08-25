# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import pytest

from app.utils.security import (
    DEFAULT_KEY_DERIVATION_ITERATIONS,
    DEFAULT_KEY_DERIVATION_SALT,
    derive_key_identifier,
    hash_password,
    is_shell_required,
    validate_safe_path,
    verify_password,
)


class TestPasswordHashing:
    """Test password hashing and verification."""

    def test_hash_and_verify_success(self) -> None:
        raw_pw = "SuperSecurePassword123!"
        hashed = hash_password(raw_pw, iterations=1000)

        assert hashed.startswith("$pbkdf2-sha256$")
        assert verify_password(raw_pw, hashed) is True

    def test_verify_wrong_password_fails(self) -> None:
        raw_pw = "SuperSecurePassword123!"
        wrong_pw = "WrongPassword123!"
        hashed = hash_password(raw_pw, iterations=1000)

        assert verify_password(wrong_pw, hashed) is False

    def test_verify_empty_inputs(self) -> None:
        assert verify_password("", "$pbkdf2-sha256$i=1000$abcd$1234") is False
        assert verify_password("somepass", "") is False

    def test_verify_malformed_hashes(self) -> None:
        assert verify_password("pass", "not-a-valid-hash") is False
        assert verify_password("pass", "$pbkdf2-sha256$invalid") is False
        assert verify_password("pass", "$pbkdf2-sha256$i=notanumber$abcd$1234") is False
        assert verify_password("pass", "$pbkdf2-sha256$i=-10$abcd$1234") is False
        assert verify_password("pass", "$pbkdf2-sha256$i=1000$nothex$1234") is False

    def test_hash_empty_password_raises(self) -> None:
        with pytest.raises(ValueError, match="Password cannot be empty"):
            hash_password("")

    def test_hash_invalid_iterations_raises(self) -> None:
        with pytest.raises(ValueError, match="Iterations must be positive"):
            hash_password("validpassword", iterations=0)

    def test_custom_salt_deterministic(self) -> None:
        raw_pw = "DeterministicPass123"
        salt = b"0123456789abcdef"
        hash1 = hash_password(raw_pw, salt=salt, iterations=1000)
        hash2 = hash_password(raw_pw, salt=salt, iterations=1000)

        assert hash1 == hash2
        assert verify_password(raw_pw, hash1) is True


class TestKeyDerivation:
    """Test deterministic key identifier derivation."""

    def test_derive_key_identifier_deterministic(self) -> None:
        key = "g8e_test_secret_token_123456789"
        doc_id1 = derive_key_identifier(key, iterations=1000, length=32)
        doc_id2 = derive_key_identifier(key, iterations=1000, length=32)

        assert doc_id1 == doc_id2
        assert len(doc_id1) == 32

    def test_derive_key_different_inputs_different_outputs(self) -> None:
        key1 = "g8e_test_key_1"
        key2 = "g8e_test_key_2"
        id1 = derive_key_identifier(key1, iterations=1000)
        id2 = derive_key_identifier(key2, iterations=1000)

        assert id1 != id2

    def test_derive_key_custom_salt_and_length(self) -> None:
        key = "g8e_operator_key"
        custom_salt = b"custom_salt_bytes"
        derived_16 = derive_key_identifier(key, salt=custom_salt, iterations=1000, length=16)
        derived_64 = derive_key_identifier(key, salt=custom_salt, iterations=1000, length=64)

        assert len(derived_16) == 16
        assert len(derived_64) == 64
        assert derived_64.startswith(derived_16)

    def test_derive_key_empty_raises(self) -> None:
        with pytest.raises(ValueError, match="Raw material cannot be empty"):
            derive_key_identifier("")

    def test_derive_key_invalid_length_raises(self) -> None:
        with pytest.raises(ValueError, match="Length must be positive"):
            derive_key_identifier("some_key", length=0)
