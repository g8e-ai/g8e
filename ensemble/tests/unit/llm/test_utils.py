# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest

from app.llm.utils import (
    ModelOverrideResolver,
    is_internal_endpoint,
    is_ollama_endpoint,
    resolve_model,
)


class TestModelOverrideResolver:
    def test_for_triage(self):
        resolver = ModelOverrideResolver(primary_model="p", assistant_model="a", lite_model="l")
        assert resolver.for_triage() == "l"

    def test_for_main_generation_complex(self):
        resolver = ModelOverrideResolver(primary_model="p", assistant_model="a", lite_model="l")
        assert resolver.for_main_generation(needs_primary=True) == "p"

    def test_for_main_generation_simple(self):
        resolver = ModelOverrideResolver(primary_model="p", assistant_model="a", lite_model="l")
        assert resolver.for_main_generation(needs_primary=False) == "a"

    def test_with_none_values(self):
        resolver = ModelOverrideResolver(primary_model=None, assistant_model=None, lite_model=None)
        assert resolver.for_triage() is None
        assert resolver.for_main_generation(needs_primary=True) is None
        assert resolver.for_main_generation(needs_primary=False) is None


class TestIsInternalEndpoint:
    @pytest.mark.parametrize(
        ("url", "expected"),
        [
            ("http://localhost:443", True),
            ("http://127.0.0.1:11434", True),
            ("http://[::1]:11434", True),
            ("http://operator:8000/v1", True),
            ("http://service.local:443", True),
            ("http://api.internal/v1", True),
            ("http://192.168.1.1:8000", True),
            ("http://10.0.0.1:8000", True),
            ("http://172.16.0.1:8000", True),
            ("https://google.com", False),
            ("https://api.openai.com/v1", False),
            (None, False),
            ("", False),
        ],
    )
    def test_is_internal_endpoint(self, url, expected):
        assert is_internal_endpoint(url) == expected

    def test_is_internal_endpoint_missing_hostname(self):
        # urlparse('http:///path') results in empty hostname
        assert is_internal_endpoint("http:///path") is False

    def test_is_internal_endpoint_public_ip(self):
        assert is_internal_endpoint("http://8.8.8.8") is False

    def test_is_internal_endpoint_empty_url(self):
        assert is_internal_endpoint("") is False
        assert is_internal_endpoint(None) is False

    def test_is_internal_endpoint_exception_fallback(self, monkeypatch):
        # Force an exception in urlparse or similar
        def mock_urlparse(url):
            raise Exception("Parsing failed")

        monkeypatch.setattr("app.llm.utils.urlparse", mock_urlparse)

        assert is_internal_endpoint("http://localhost:443") is True
        assert is_internal_endpoint("https://google.com") is False
        assert is_internal_endpoint(None) is False


class TestIsOllamaEndpoint:
    @pytest.mark.parametrize(
        ("url", "expected"),
        [
            ("http://localhost:11434", True),
            ("http://ollama:11434", True),
            ("http://my-ollama-server:8000", True),
            ("http://localhost:8000/api/ollama/generate", True),
            ("http://localhost:8000/v1", False),
            (None, False),
            ("", False),
        ],
    )
    def test_is_ollama_endpoint(self, url, expected):
        assert is_ollama_endpoint(url) == expected

    def test_is_ollama_endpoint_exception_fallback(self, monkeypatch):
        def mock_urlparse(url):
            raise Exception("Parsing failed")

        monkeypatch.setattr("app.llm.utils.urlparse", mock_urlparse)

        assert is_ollama_endpoint("http://localhost:11434") is True
        assert is_ollama_endpoint("http://ollama-host") is True
        assert is_ollama_endpoint("http://normal-host") is False


class TestResolveModel:
    def test_resolve_primary(self):
        assert resolve_model("primary", "over", None, None, "def", "a", "l") == "over"
        assert resolve_model("primary", None, None, None, "def", "a", "l") == "def"

    def test_resolve_assistant(self):
        assert resolve_model("assistant", None, "over", None, "p", "def", "l") == "over"
        assert resolve_model("assistant", None, None, None, "p", "def", "l") == "def"

    def test_resolve_lite(self):
        assert resolve_model("lite", None, None, "over", "p", "a", "def") == "over"
        assert resolve_model("lite", None, None, None, "p", "a", "def") == "def"

    def test_resolve_invalid_tier(self):
        with pytest.raises(ValueError, match="Invalid model tier"):
            resolve_model("invalid", None, None, None, "p", "a", "l")
