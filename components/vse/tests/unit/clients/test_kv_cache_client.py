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

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock

import aiohttp
import pytest

from app.clients.kv_cache_client import KVCacheClient, _encode_key
from app.constants import ComponentName

pytestmark = pytest.mark.unit

@pytest.fixture
def disconnected_client():
    client = KVCacheClient(
        http_url="https://vsodb:9000",
        component_name=ComponentName.VSE,
    )
    return client

class TestKVCacheClientInit:
    def test_explicit_urls_override_defaults(self):
        client = KVCacheClient(
            http_url="https://custom-host",
        )
        assert client.http_url == "https://custom-host"

    def test_trailing_slash_stripped_from_urls(self):
        client = KVCacheClient(
            http_url="https://vsodb:9000/",
        )
        assert not client.http_url.endswith("/")

    def test_component_name_default_is_vse(self):
        client = KVCacheClient()
        assert client.component_name is ComponentName.VSE

    def test_is_healthy_false_on_init(self):
        client = KVCacheClient()
        assert client.is_healthy() is False

@pytest.mark.asyncio
class TestKVCacheClientUnhealthyGuards:
    async def test_get_returns_none_when_unhealthy(self, disconnected_client):
        result = await disconnected_client.get("some:key")
        assert result is None

    async def test_set_returns_false_when_unhealthy(self, disconnected_client):
        result = await disconnected_client.set("some:key", "value")
        assert result is False

    async def test_incr_returns_zero_when_unhealthy(self, disconnected_client):
        result = await disconnected_client.incr("counter:key")
        assert result == 0

class TestEncodeKey:
    def test_plain_key_unchanged(self):
        assert _encode_key("simple") == "simple"

    def test_colon_separator_encoded(self):
        encoded = _encode_key("operators:op-123:status")
        assert ":" not in encoded

    def test_slash_encoded(self):
        encoded = _encode_key("path/with/slashes")
        assert "/" not in encoded

    def test_dot_encoded(self):
        encoded = _encode_key("cache.doc")
        assert encoded == "cache.doc"

    def test_asterisk_encoded(self):
        encoded = _encode_key("key*wildcard")
        assert "*" not in encoded

    def test_plus_encoded(self):
        encoded = _encode_key("user+id")
        assert "+" not in encoded

    def test_question_mark_encoded(self):
        encoded = _encode_key("key?query")
        assert "?" not in encoded

    def test_open_bracket_encoded(self):
        encoded = _encode_key("array[0]")
        assert "[" not in encoded

    def test_close_bracket_encoded(self):
        encoded = _encode_key("array[0]")
        assert "]" not in encoded

    def test_dollar_sign_encoded(self):
        encoded = _encode_key("$variable")
        assert "$" not in encoded

    def test_caret_encoded(self):
        encoded = _encode_key("key^anchor")
        assert "^" not in encoded

    def test_open_paren_encoded(self):
        encoded = _encode_key("func(arg)")
        assert "(" not in encoded

    def test_close_paren_encoded(self):
        encoded = _encode_key("func(arg)")
        assert ")" not in encoded

    def test_pipe_encoded(self):
        encoded = _encode_key("a|b")
        assert "|" not in encoded

    def test_backslash_encoded(self):
        encoded = _encode_key("path\\to\\file")
        assert "\\" not in encoded

    def test_space_encoded(self):
        encoded = _encode_key("key with spaces")
        assert " " not in encoded

    def test_hash_encoded(self):
        encoded = _encode_key("key#fragment")
        assert "#" not in encoded

    def test_percent_encoded(self):
        encoded = _encode_key("100%complete")
        assert encoded == "100%25complete"

    def test_ampersand_encoded(self):
        encoded = _encode_key("a&b")
        assert "&" not in encoded

    def test_equals_encoded(self):
        encoded = _encode_key("key=value")
        assert "=" not in encoded

    def test_at_sign_encoded(self):
        encoded = _encode_key("user@host")
        assert "@" not in encoded
