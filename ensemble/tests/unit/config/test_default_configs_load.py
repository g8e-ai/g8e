# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests that default JSON config files load correctly and contain minimum expected entries.

This ensures that typos or gutted config files are caught by CI before deploy.
"""

from app.utils.auto_approved_validator import (
    get_auto_approved_validator,
    register_auto_approved_validator,
)
from app.utils.blacklist_validator import get_blacklist_validator, register_blacklist_validator
from app.utils.whitelist_validator import get_whitelist_validator, register_whitelist_validator


class TestDefaultWhitelistConfigLoads:
    """Test that the default whitelist.json loads without error."""

    def test_default_whitelist_json_loads_without_error(self):
        """Assert that the default whitelist.json loads without raising ConfigurationError.

        This catches typos and malformed JSON in the config file before deploy.
        """
        import app.utils.whitelist_validator as wv_module

        wv_module._validator_instance = None
        validator = get_whitelist_validator(whitelist_path=None)
        assert validator is not None
        register_whitelist_validator(validator)


class TestDefaultBlacklistConfigLoads:
    """Test that the default blacklist.json loads without error."""

    def test_default_blacklist_json_loads_without_error(self):
        """Assert that the default blacklist.json loads without raising ConfigurationError.

        This catches typos and malformed JSON in the config file before deploy.
        """
        import app.utils.blacklist_validator as bv_module

        bv_module._validator = None
        validator = get_blacklist_validator(blacklist_path=None)
        assert validator is not None
        register_blacklist_validator(validator)


class TestDefaultAutoApprovedConfigLoads:
    """Test that the default auto_approved.json loads without error."""

    def test_default_auto_approved_json_loads_without_error(self):
        """Assert that the default auto_approved.json loads without raising ConfigurationError.

        This catches typos and malformed JSON in the config file before deploy.
        """
        import app.utils.auto_approved_validator as av_module

        av_module._validator = None
        validator = get_auto_approved_validator(auto_approved_path=None)
        assert validator is not None
        register_auto_approved_validator(validator)

    def test_default_auto_approved_contains_uptime(self):
        """Assert that the default auto_approved.json contains 'uptime' in auto_approved_commands.

        Since auto_approved.json is enabled by default, we verify it contains expected entries
        to catch gutted files.
        """
        import app.utils.auto_approved_validator as av_module

        av_module._validator = None
        validator = get_auto_approved_validator(auto_approved_path=None)
        command_names = validator.get_auto_approved_command_names()
        assert "uptime" in command_names, "Default auto_approved.json should contain 'uptime'"
        register_auto_approved_validator(validator)
