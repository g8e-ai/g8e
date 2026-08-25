# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests verifying .env auto-loading behavior (D.7)."""

import os
import tempfile
from pathlib import Path
from dotenv import load_dotenv


def test_dotenv_loading_does_not_override_existing_env_vars():
    """Verify load_dotenv(override=False) preserves existing environment variables."""
    test_key = "G8E_TEST_EXISTING_VAR_D7"
    os.environ[test_key] = "docker_provided_value"

    try:
        with tempfile.NamedTemporaryFile("w", delete=False) as f:
            f.write(f"{test_key}=env_file_value\n")
            dotenv_path = f.name

        try:
            load_dotenv(dotenv_path=dotenv_path, override=False)
            assert os.environ[test_key] == "docker_provided_value"
        finally:
            Path(dotenv_path).unlink(missing_ok=True)
    finally:
        os.environ.pop(test_key, None)


def test_dotenv_loading_loads_unset_vars():
    """Verify load_dotenv(override=False) loads variables that are unset in environment."""
    test_key = "G8E_TEST_UNSET_VAR_D7"
    os.environ.pop(test_key, None)

    try:
        with tempfile.NamedTemporaryFile("w", delete=False) as f:
            f.write(f"{test_key}=loaded_from_dotenv\n")
            dotenv_path = f.name

        try:
            load_dotenv(dotenv_path=dotenv_path, override=False)
            assert os.environ[test_key] == "loaded_from_dotenv"
        finally:
            Path(dotenv_path).unlink(missing_ok=True)
    finally:
        os.environ.pop(test_key, None)
