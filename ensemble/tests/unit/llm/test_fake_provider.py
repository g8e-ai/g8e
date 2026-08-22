# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for the FakeProvider (deterministic CI LLM provider)."""

from __future__ import annotations

import pytest

from app.llm.llm_dataclasses import Content, Part
from app.llm.llm_types import PrimaryLLMSettings
from app.llm.providers.fake import FakeProvider


@pytest.fixture
def provider() -> FakeProvider:
    return FakeProvider()


@pytest.fixture
def settings() -> PrimaryLLMSettings:
    return PrimaryLLMSettings()


def _user_content(message: str) -> list[Content]:
    return [Content(role="user", parts=[Part(text=message)])]


class TestFakeProviderFileCreate:
    """Verify the fake provider generates file_create tool calls."""

    @pytest.mark.asyncio
    async def test_stream_primary_yields_file_create_tool_call(self, provider, settings):
        message = "Create a new file at /tmp/g8e-smoke-test.txt with the content: hello world"
        chunks = []
        async for chunk in provider.generate_content_stream_primary("fake-model", _user_content(message), settings):
            chunks.append(chunk)

        assert len(chunks) == 1
        chunk = chunks[0]
        assert len(chunk.tool_calls) == 1
        tool_call = chunk.tool_calls[0]
        assert tool_call.name == "file_create_on_operator"
        assert tool_call.args["file_path"] == "/tmp/g8e-smoke-test.txt"
        assert tool_call.args["content"] == "hello world"
        assert tool_call.args["target_operators"] == ["all"]
        assert tool_call.args["justification"] is not None
        assert chunk.finish_reason == "STOP"

    @pytest.mark.asyncio
    async def test_generate_primary_returns_file_create_tool_call(self, provider, settings):
        message = "Create a new file at /tmp/test.txt with the content: test content"
        response = await provider.generate_content_primary("fake-model", _user_content(message), settings)

        assert len(response.candidates) == 1
        tool_calls = response.tool_calls
        assert len(tool_calls) == 1
        assert tool_calls[0].name == "file_create_on_operator"
        assert tool_calls[0].args["file_path"] == "/tmp/test.txt"
        assert tool_calls[0].args["content"] == "test content"


class TestFakeProviderFileWrite:
    """Verify the fake provider generates file_write tool calls."""

    @pytest.mark.asyncio
    async def test_stream_primary_yields_file_write_tool_call(self, provider, settings):
        message = "Write the following content to the file at /tmp/output.txt: new data"
        chunks = []
        async for chunk in provider.generate_content_stream_primary("fake-model", _user_content(message), settings):
            chunks.append(chunk)

        assert len(chunks) == 1
        chunk = chunks[0]
        assert len(chunk.tool_calls) == 1
        assert chunk.tool_calls[0].name == "file_write_on_operator"
        assert chunk.tool_calls[0].args["file_path"] == "/tmp/output.txt"
        assert chunk.tool_calls[0].args["content"] == "new data"


class TestFakeProviderNonToolCallResponses:
    """Verify the fake provider returns text responses for non-file operations."""

    @pytest.mark.asyncio
    async def test_delete_instruction_returns_text(self, provider, settings):
        message = "Delete the investigation note created in the previous run."
        chunks = []
        async for chunk in provider.generate_content_stream_primary("fake-model", _user_content(message), settings):
            chunks.append(chunk)

        assert len(chunks) == 1
        assert len(chunks[0].tool_calls) == 0
        assert chunks[0].text is not None
        assert chunks[0].finish_reason == "STOP"

    @pytest.mark.asyncio
    async def test_investigation_note_returns_text(self, provider, settings):
        message = "Create a new investigation note documenting this smoke test run."
        chunks = []
        async for chunk in provider.generate_content_stream_primary("fake-model", _user_content(message), settings):
            chunks.append(chunk)

        assert len(chunks) == 1
        assert len(chunks[0].tool_calls) == 0
        assert chunks[0].text is not None


class TestFakeProviderDefaults:
    """Verify the fake provider falls back to defaults when path/content are absent."""

    @pytest.mark.asyncio
    async def test_create_without_explicit_path_uses_default(self, provider, settings):
        message = "Create a file for the smoke test"
        chunks = []
        async for chunk in provider.generate_content_stream_primary("fake-model", _user_content(message), settings):
            chunks.append(chunk)

        assert len(chunks[0].tool_calls) == 1
        assert chunks[0].tool_calls[0].name == "file_create_on_operator"
        # Falls back to the default path
        assert chunks[0].tool_calls[0].args["file_path"] == "/tmp/g8e-fake-provider.txt"


class TestFakeProviderCallLog:
    """Verify the fake provider records calls in its call_log."""

    @pytest.mark.asyncio
    async def test_call_log_records_stream_primary(self, provider, settings):
        message = "Create a new file at /tmp/test.txt with the content: data"
        async for _ in provider.generate_content_stream_primary("fake-model", _user_content(message), settings):
            pass

        assert len(provider.call_log) == 1
        entry = provider.call_log[0]
        assert entry["method"] == "generate_content_stream_primary"
        assert entry["model"] == "fake-model"


class TestFakeProviderValidation:
    """Verify the fake provider validation always succeeds."""

    def test_validate_config_returns_empty_list(self):
        errors = FakeProvider.validate_config(api_key=None, endpoint=None)
        assert errors == []

    def test_validate_config_with_values_returns_empty_list(self):
        errors = FakeProvider.validate_config(api_key="some-key", endpoint="http://localhost:9999")
        assert errors == []


class TestFakeProviderFactoryWiring:
    """Verify the LLM provider factory returns a FakeProvider for LLMProvider.FAKE."""

    def test_factory_returns_fake_provider(self):
        from app.constants import LLMProvider
        from app.llm.factory import get_llm_provider, clear_provider_cache
        from app.models.settings import LLMSettings

        settings = LLMSettings(
            primary_provider=LLMProvider.FAKE,
            primary_model="fake-model",
            primary_endpoint="",
        )
        provider = get_llm_provider(settings)
        assert isinstance(provider, FakeProvider)

    def test_llm_provider_enum_has_fake_value(self):
        from app.constants import LLMProvider

        assert LLMProvider.FAKE.value == "fake"
        assert LLMProvider("fake") == LLMProvider.FAKE
