# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import hmac
from pathlib import Path
import secrets

# Standard password hashing parameters (PBKDF2-HMAC-SHA256)
DEFAULT_PASSWORD_HASH_ITERATIONS = 600_000
DEFAULT_KEY_DERIVATION_ITERATIONS = 100_000
DEFAULT_KEY_DERIVATION_SALT = b"g8e_api_key_v1"
PASSWORD_HASH_PREFIX = "$pbkdf2-sha256$"


def validate_safe_path(path: str | Path, root: str | Path) -> Path:
    """
    Ensures a path is safe and stays within the specified root directory.

    Args:
        path: The path to validate (absolute or relative)
        root: The allowed root directory

    Returns:
        The resolved absolute Path object

    Raises:
        ValueError: If the path is invalid or attempts traversal outside the root
    """
    if not path:
        raise ValueError("Empty path provided")

    root_path = Path(root).resolve()

    # Clean and resolve the target path
    # Path.resolve() handles '..' segments and redundant slashes
    try:
        if Path(path).is_absolute():
            target_path = Path(path).resolve()
        else:
            target_path = (root_path / path).resolve()
    except Exception as e:
        raise ValueError(f"Invalid path format: {e}")

    # Security check: Ensure target_path is within root_path
    try:
        # relative_to raises ValueError if target_path is not under root_path
        target_path.relative_to(root_path)
    except ValueError:
        raise ValueError(f"Path traversal detected: {path} is outside of {root}")

    return target_path


def is_shell_required(command: str) -> bool:
    """
    Checks if a command string contains shell metacharacters.
    """
    # Common shell metacharacters
    metachars = set("|&><$();`\\*?[]~")
    return any(char in metachars for char in command)


def hash_password(
    password: str,
    *,
    salt: bytes | None = None,
    iterations: int = DEFAULT_PASSWORD_HASH_ITERATIONS,
) -> str:
    """Hash a password using PBKDF2-HMAC-SHA256 with a secure work factor.

    Returns a standard formatted string: $pbkdf2-sha256$i=<iterations>$<salt_hex>$<hash_hex>
    """
    if not password:
        raise ValueError("Password cannot be empty")
    if iterations < 1:
        raise ValueError("Iterations must be positive")

    if salt is None:
        salt = secrets.token_bytes(16)

    derived = hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt, iterations, dklen=32)
    return f"{PASSWORD_HASH_PREFIX}i={iterations}${salt.hex()}${derived.hex()}"


def verify_password(password: str, hashed_password: str) -> bool:
    """Verify a password against a PBKDF2-HMAC-SHA256 formatted hash using constant-time comparison."""
    if not password or not hashed_password:
        return False

    if not hashed_password.startswith(PASSWORD_HASH_PREFIX):
        return False

    try:
        parts = hashed_password[len(PASSWORD_HASH_PREFIX) :].split("$")
        if len(parts) != 3:
            return False

        iter_part, salt_hex, hash_hex = parts
        if not iter_part.startswith("i="):
            return False

        iterations = int(iter_part[2:])
        if iterations < 1:
            return False

        salt = bytes.fromhex(salt_hex)
        expected_hash = bytes.fromhex(hash_hex)

        derived = hashlib.pbkdf2_hmac(
            "sha256", password.encode("utf-8"), salt, iterations, dklen=len(expected_hash)
        )
        return hmac.compare_digest(derived, expected_hash)
    except (ValueError, TypeError):
        return False


def derive_key_identifier(
    raw_material: str,
    *,
    salt: bytes = DEFAULT_KEY_DERIVATION_SALT,
    iterations: int = DEFAULT_KEY_DERIVATION_ITERATIONS,
    length: int = 32,
) -> str:
    """Derive a deterministic identifier/hash from sensitive key material using PBKDF2-HMAC-SHA256."""
    if not raw_material:
        raise ValueError("Raw material cannot be empty")
    if length < 1:
        raise ValueError("Length must be positive")

    dklen = (length + 1) // 2
    derived = hashlib.pbkdf2_hmac(
        "sha256", raw_material.encode("utf-8"), salt, iterations, dklen=dklen
    )
    return derived.hex()[:length]

