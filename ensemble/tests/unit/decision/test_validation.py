# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for Jev lite-role coexistence validation."""

from __future__ import annotations

import logging

import pytest

from app.constants import LLMProvider
from app.decision.validation import (
    jev_generative_lite_limitations_message,
    log_jev_generative_lite_warning,
    validate_jev_lite_coexistence,
)
from app.models.settings import LLMSettings

pytestmark = pytest.mark.unit


class TestValidateJevLiteCoexistence:
    def test_returns_no_errors_for_generative_lite_provider(self):
        llm = LLMSettings(
            lite_provider=LLMProvider.OLLAMA,
            lite_model="qwen3:0.6b",
            llm_command_gen_enabled=True,
        )
        assert validate_jev_lite_coexistence(llm) == []

    def test_returns_no_errors_for_jev_when_tribunal_disabled(self):
        llm = LLMSettings(
            lite_provider=LLMProvider.JEV,
            lite_model="jev-latest",
            jev_api_key="ts_test_key",
            llm_command_gen_enabled=False,
        )
        assert validate_jev_lite_coexistence(llm) == []

    def test_rejects_jev_when_tribunal_enabled(self):
        llm = LLMSettings(
            lite_provider=LLMProvider.JEV,
            lite_model="jev-latest",
            jev_api_key="ts_test_key",
            llm_command_gen_enabled=True,
        )
        errors = validate_jev_lite_coexistence(llm)
        assert len(errors) == 1
        assert "Tribunal command generation" in errors[0]


class TestJevGenerativeLiteWarning:
    def test_limitations_message_lists_generative_call_sites(self):
        message = jev_generative_lite_limitations_message()
        assert "triage" in message
        assert "title generation" in message
        assert "memory extraction" in message

    def test_log_warning_emits_only_for_jev_lite(self, caplog):
        caplog.set_level(logging.WARNING)
        log_jev_generative_lite_warning(
            logging.getLogger("test"),
            LLMSettings(lite_provider=LLMProvider.OLLAMA, lite_model="qwen3:0.6b"),
        )
        assert caplog.records == []

        log_jev_generative_lite_warning(
            logging.getLogger("test"),
            LLMSettings(
                lite_provider=LLMProvider.JEV,
                lite_model="jev-latest",
                jev_api_key="ts_test_key",
            ),
        )
        assert len(caplog.records) == 1
        assert "title generation" in caplog.records[0].message
