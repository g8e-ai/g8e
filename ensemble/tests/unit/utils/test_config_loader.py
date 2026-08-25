# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for config_loader module."""

import json
import tempfile
from pathlib import Path

import pytest

from app.errors import ConfigurationError
from app.utils.config_loader import load_json_config


class TestLoadJsonConfig:
    """Tests for load_json_config function."""

    def test_load_valid_json(self):
        """Successfully load a valid JSON file."""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump({"key": "value", "number": 42}, f)
            temp_path = Path(f.name)

        try:
            result = load_json_config(temp_path, config_name="test config")
            assert result == {"key": "value", "number": 42}
        finally:
            temp_path.unlink()

    def test_file_not_found_raises_configuration_error(self):
        """Raise ConfigurationError when file does not exist."""
        non_existent_path = Path("/tmp/does_not_exist_12345.json")
        with pytest.raises(ConfigurationError) as exc_info:
            load_json_config(non_existent_path, config_name="test config")
        assert "test config not found at" in str(exc_info.value)
        assert str(non_existent_path) in str(exc_info.value)

    def test_invalid_json_raises_configuration_error(self):
        """Raise ConfigurationError when file contains invalid JSON."""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            f.write("{ invalid json }")
            temp_path = Path(f.name)

        try:
            with pytest.raises(ConfigurationError) as exc_info:
                load_json_config(temp_path, config_name="test config")
            assert "Invalid JSON in test config at" in str(exc_info.value)
            assert str(temp_path) in str(exc_info.value)
            assert exc_info.value.__cause__ is not None
        finally:
            temp_path.unlink()

    def test_empty_json_returns_empty_dict(self):
        """Successfully load an empty JSON object."""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump({}, f)
            temp_path = Path(f.name)

        try:
            result = load_json_config(temp_path, config_name="test config")
            assert result == {}
        finally:
            temp_path.unlink()

    def test_nested_json_structure(self):
        """Successfully load a nested JSON structure."""
        data = {"level1": {"level2": {"level3": "deep value"}}, "array": [1, 2, 3]}
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump(data, f)
            temp_path = Path(f.name)

        try:
            result = load_json_config(temp_path, config_name="test config")
            assert result == data
        finally:
            temp_path.unlink()
