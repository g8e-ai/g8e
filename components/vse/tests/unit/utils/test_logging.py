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

"""
Tests for app/logging.py.

Covers: redact_email, redact_value, redact_pii, redact_message,
        ComponentFormatter, register_component_logger, setup_logging.
"""

import logging
import sys

import pytest

from app.models.settings import VSEPlatformSettings
from app.constants import LogLevel
from app.logging import (
    PII_FIELDS,
    ComponentFormatter,
    _logger_component_registry,
    redact_email,
    redact_message,
    redact_pii,
    redact_value,
    register_component_logger,
    setup_logging,
)

pytestmark = pytest.mark.unit


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_settings(log_level: LogLevel = LogLevel.INFO, enable_logging: bool = True) -> VSEPlatformSettings:
    settings = VSEPlatformSettings(port=443)
    settings.log_level = log_level
    settings.enable_logging = enable_logging
    return settings


def _make_record(
    name: str = "vse.services.something",
    msg: str = "test message",
    pathname: str = "/app/components/vse/services/something.py",
    module: str = "something",
    levelno: int = logging.INFO,
) -> logging.LogRecord:
    record = logging.LogRecord(
        name=name,
        level=levelno,
        pathname=pathname,
        lineno=42,
        msg=msg,
        args=(),
        exc_info=None,
    )
    record.module = module
    return record


# ---------------------------------------------------------------------------
# redact_email
# ---------------------------------------------------------------------------

@pytest.mark.unit
class TestRedactEmail:

    def test_typical_email_redacted(self):
        result = redact_email("alice@example.com")
        assert "@" in result
        assert "alice" not in result
        assert "example" not in result

    def test_local_part_first_last_preserved(self):
        result = redact_email("alice@example.com")
        local, domain = result.split("@")
        assert local[0] == "a"
        assert local[-1] == "e"
        assert all(c == "*" for c in local[1:-1])

    def test_short_local_part_all_stars(self):
        result = redact_email("ab@example.com")
        local, _ = result.split("@")
        assert all(c == "*" for c in local)

    def test_single_char_local(self):
        result = redact_email("a@example.com")
        local, _ = result.split("@")
        assert local == "*"

    def test_domain_part_redacted(self):
        result = redact_email("user@longdomain.com")
        _, domain = result.split("@")
        assert domain[0] == "l"
        assert domain[-1] == "m"
        assert all(c == "*" for c in domain[1:-1])

    def test_short_domain_all_stars(self):
        result = redact_email("user@ab")
        _, domain = result.split("@")
        assert all(c == "*" for c in domain)

    def test_no_at_sign_returned_unchanged(self):
        assert redact_email("notanemail") == "notanemail"

    def test_empty_string_returned_unchanged(self):
        assert redact_email("") == ""


# ---------------------------------------------------------------------------
# redact_value
# ---------------------------------------------------------------------------

@pytest.mark.unit
class TestRedactValue:

    def test_none_returned_as_none(self):
        assert redact_value("email", None) is None

    def test_pii_email_field_redacted(self):
        result = redact_value("email", "alice@example.com")
        assert "@" in result
        assert "alice" not in result

    def test_pii_name_field_first_char_preserved(self):
        result = redact_value("name", "Alice Smith")
        assert result[0] == "A"
        assert result[1:] == "*" * (len("Alice Smith") - 1)

    def test_pii_field_non_string_returns_stars(self):
        result = redact_value("email", 12345)
        assert result == "***"

    def test_non_pii_field_with_email_in_value_redacted(self):
        result = redact_value("message", "contact alice@example.com for help")
        assert "alice@example.com" not in result
        assert "@" in result

    def test_non_pii_field_no_email_unchanged(self):
        result = redact_value("status", "active")
        assert result == "active"

    def test_pii_field_names_case_insensitive(self):
        result = redact_value("EMAIL", "admin@example.com")
        assert "admin" not in result

    @pytest.mark.parametrize("field", PII_FIELDS)
    def test_all_pii_fields_are_redacted(self, field):
        result = redact_value(field, "SomeValue")
        assert result != "SomeValue"

    def test_empty_string_pii_value(self):
        result = redact_value("name", "")
        assert result == "***"

    def test_non_pii_integer_value_unchanged(self):
        assert redact_value("count", 42) == 42


# ---------------------------------------------------------------------------
# redact_pii
# ---------------------------------------------------------------------------

@pytest.mark.unit
class TestRedactPii:

    def test_none_returned_as_none(self):
        assert redact_pii(None) is None

    def test_scalar_returned_unchanged(self):
        assert redact_pii("hello") == "hello"
        assert redact_pii(123) == 123

    def test_dict_pii_field_redacted(self):
        result = redact_pii({"email": "alice@example.com", "status": "active"})
        assert "alice@example.com" not in result["email"]
        assert result["status"] == "active"

    def test_list_of_dicts_redacted(self):
        data = [{"email": "alice@example.com"}, {"email": "admin@example.com"}]
        result = redact_pii(data)
        assert "alice@example.com" not in result[0]["email"]
        assert "admin@example.com" not in result[1]["email"]

    def test_nested_dict_redacted(self):
        data = {"user": {"email": "alice@example.com", "name": "Alice"}}
        result = redact_pii(data)
        assert "alice@example.com" not in result["user"]["email"]
        assert "Alice" not in result["user"]["name"]

    def test_depth_limit_returns_obj_at_max_depth(self):
        obj = {"email": "x@y.com"}
        result = redact_pii(obj, depth=11)
        assert result is obj

    def test_empty_dict(self):
        assert redact_pii({}) == {}

    def test_empty_list(self):
        assert redact_pii([]) == []

    def test_non_pii_fields_in_dict_unchanged(self):
        data = {"status": "active", "count": 5}
        result = redact_pii(data)
        assert result["status"] == "active"
        assert result["count"] == 5


# ---------------------------------------------------------------------------
# redact_message
# ---------------------------------------------------------------------------

@pytest.mark.unit
class TestRedactMessage:

    def test_email_in_message_redacted(self):
        msg = "Please contact alice@example.com for support"
        result = redact_message(msg)
        assert "alice@example.com" not in result

    def test_no_email_returned_unchanged(self):
        msg = "No email here"
        assert redact_message(msg) == msg

    def test_non_string_returned_unchanged(self):
        assert redact_message(42) == 42
        assert redact_message(None) is None

    def test_multiple_emails_all_redacted(self):
        msg = "From: a@b.com To: c@d.com"
        result = redact_message(msg)
        assert "a@b.com" not in result
        assert "c@d.com" not in result

    def test_string_without_at_returned_unchanged(self):
        msg = "no at sign in this string"
        assert redact_message(msg) == msg


# ---------------------------------------------------------------------------
# register_component_logger
# ---------------------------------------------------------------------------

@pytest.mark.unit
class TestRegisterComponentLogger:

    def test_registers_mapping(self):
        register_component_logger("test.logger.reg", "mycomponent")
        assert _logger_component_registry.get("test.logger.reg") == "mycomponent"

    def test_overwrites_existing_mapping(self):
        register_component_logger("test.logger.overwrite", "comp_a")
        register_component_logger("test.logger.overwrite", "comp_b")
        assert _logger_component_registry["test.logger.overwrite"] == "comp_b"


# ---------------------------------------------------------------------------
# ComponentFormatter
# ---------------------------------------------------------------------------

@pytest.mark.unit
class TestComponentFormatter:

    @pytest.fixture
    def formatter_with_name(self):
        return ComponentFormatter(
            "%(asctime)s - %(levelname)s - [%(component)s] - %(message)s",
            component_name="vse",
        )

    @pytest.fixture
    def formatter_no_name(self):
        return ComponentFormatter(
            "%(asctime)s - %(levelname)s - [%(component)s] - %(message)s",
        )

    def test_explicit_component_name_used(self, formatter_with_name):
        record = _make_record(msg="hello")
        formatter_with_name.format(record)
        assert record.component == "vse"

    def test_component_resolved_from_registry(self, formatter_no_name):
        register_component_logger("vse.services.mymod", "vse")
        record = _make_record(name="vse.services.mymod")
        formatter_no_name.format(record)
        assert record.component == "vse"

    def test_component_resolved_from_pathname(self, formatter_no_name):
        record = _make_record(
            name="unregistered.logger",
            pathname="/app/components/vsa/something.py",
            module="something",
        )
        formatter_no_name.format(record)
        assert record.component == "vsa"

    def test_component_resolved_from_module(self, formatter_no_name):
        record = _make_record(
            name="unregistered.logger2",
            pathname="/irrelevant/path.py",
            module="something",
        )
        record.module = "vse.utils"
        formatter_no_name.format(record)
        assert record.component == "vse"

    def test_pii_in_message_redacted(self, formatter_with_name):
        record = _make_record(msg="user email is alice@example.com")
        formatter_with_name.format(record)
        assert "alice@example.com" not in record.msg

    def test_extra_fields_appended(self, formatter_with_name):
        record = _make_record(msg="msg with extra")
        record.custom_field = "custom_value"
        output = formatter_with_name.format(record)
        assert "custom_field" in output
        assert "custom_value" in output

    def test_extra_pii_fields_redacted(self, formatter_with_name):
        record = _make_record(msg="test")
        record.email = "admin@example.com"
        output = formatter_with_name.format(record)
        assert "admin@example.com" not in output

    def test_standard_attributes_not_in_extra(self, formatter_with_name):
        record = _make_record(msg="test")
        output = formatter_with_name.format(record)
        assert "extra=" not in output or '"name"' not in output

    def test_unserializable_extra_falls_back_to_repr(self, formatter_with_name):
        record = _make_record(msg="test")

        class Unserializable:
            def __repr__(self):
                return "<unserializable-obj>"

        record.weird_field = Unserializable()
        output = formatter_with_name.format(record)
        assert "weird_field" in output


# ---------------------------------------------------------------------------
# setup_logging — logging enabled
# ---------------------------------------------------------------------------

@pytest.mark.unit
class TestSetupLoggingEnabled:

    def _isolated_setup(self, log_level=LogLevel.DEBUG, component_name=None):
        root = logging.getLogger()
        for h in root.handlers[:]:
            root.removeHandler(h)
        settings = _make_settings(log_level=log_level, enable_logging=True)
        setup_logging(settings, component_name=component_name)
        return root

    def test_handler_added_to_root_logger(self):
        root = self._isolated_setup()
        assert any(isinstance(h, logging.StreamHandler) for h in root.handlers)

    def test_root_level_set_to_debug(self):
        root = self._isolated_setup(log_level=LogLevel.DEBUG)
        assert root.level == logging.DEBUG

    def test_root_level_set_to_warning(self):
        root = self._isolated_setup(log_level=LogLevel.WARNING)
        assert root.level == logging.WARNING

    def test_all_log_levels_resolve_correctly(self):
        for level in LogLevel:
            root = self._isolated_setup(log_level=level)
            assert root.level == getattr(logging, level)

    def test_component_name_registered_in_registry(self):
        self._isolated_setup(component_name="vse")
        assert _logger_component_registry.get("vse") == "vse"

    def test_formatter_is_component_formatter(self):
        root = self._isolated_setup(component_name="vse")
        stream_handlers = [h for h in root.handlers if isinstance(h, logging.StreamHandler)]
        assert stream_handlers
        assert isinstance(stream_handlers[0].formatter, ComponentFormatter)

    def test_existing_handlers_replaced(self):
        root = logging.getLogger()
        dummy = logging.StreamHandler(sys.stdout)
        root.addHandler(dummy)
        self._isolated_setup()
        assert dummy not in root.handlers

    def test_third_party_loggers_level_set(self):
        self._isolated_setup()
        assert logging.getLogger("urllib3").level == logging.INFO
        assert logging.getLogger("aiohttp").level == logging.WARNING
        assert logging.getLogger("openai").level == logging.WARNING

    def test_handler_writes_to_stdout(self):
        root = self._isolated_setup()
        stream_handlers = [h for h in root.handlers if isinstance(h, logging.StreamHandler)]
        assert any(h.stream is sys.stdout for h in stream_handlers)


# ---------------------------------------------------------------------------
# setup_logging — logging disabled
# ---------------------------------------------------------------------------

@pytest.mark.unit
class TestSetupLoggingDisabled:

    def _isolated_disable(self):
        root = logging.getLogger()
        for h in root.handlers[:]:
            root.removeHandler(h)
        settings = _make_settings(enable_logging=False)
        setup_logging(settings, component_name="vse")
        return root

    def test_null_handler_added(self):
        root = self._isolated_disable()
        assert any(isinstance(h, logging.NullHandler) for h in root.handlers)

    def test_level_set_above_critical(self):
        root = self._isolated_disable()
        assert root.level > logging.CRITICAL

    def test_existing_handlers_removed(self):
        root = logging.getLogger()
        dummy = logging.StreamHandler(sys.stdout)
        root.addHandler(dummy)
        root = self._isolated_disable()
        assert dummy not in root.handlers

    def test_null_handler_not_duplicated_on_repeat_call(self):
        root = self._isolated_disable()
        count_before = sum(1 for h in root.handlers if isinstance(h, logging.NullHandler))
        settings = _make_settings(enable_logging=False)
        setup_logging(settings, component_name="vse")
        count_after = sum(1 for h in root.handlers if isinstance(h, logging.NullHandler))
        assert count_after == count_before
