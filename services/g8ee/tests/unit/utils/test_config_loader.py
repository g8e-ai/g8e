# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
        data = {
            "level1": {
                "level2": {
                    "level3": "deep value"
                }
            },
            "array": [1, 2, 3]
        }
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            json.dump(data, f)
            temp_path = Path(f.name)

        try:
            result = load_json_config(temp_path, config_name="test config")
            assert result == data
        finally:
            temp_path.unlink()
