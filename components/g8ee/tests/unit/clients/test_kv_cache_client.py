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


from unittest.mock import AsyncMock, MagicMock, patch

import aiohttp
import pytest

from app.clients.kv_cache_client import KVCacheClient, _encode_key
from app.constants import ComponentName
from app.errors import NetworkError
from app.models.settings import ListenSettings

pytestmark = pytest.mark.unit


class MockResponse:
    def __init__(self, status=200, text="{}", json_data=None):
        self.status = status
        self._text = text
        self._json_data = json_data

    async def text(self):
        return self._text

    async def json(self):
        return self._json_data or {}

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        pass


@pytest.fixture
def mock_session():
    session = MagicMock(spec=aiohttp.ClientSession)
    session.request = MagicMock()
    session.closed = False
    session.close = AsyncMock()
    return session


@pytest.fixture
def client(mock_session):
    client = KVCacheClient(
        http_url="https://g8es:9000",
        component_name=ComponentName.G8EE,
    )
    client._session = mock_session
    client._healthy = True
    return client


@pytest.fixture
def disconnected_client():
    client = KVCacheClient(
        http_url="https://g8es:9000",
        component_name=ComponentName.G8EE,
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
            http_url="https://g8es:9000/",
        )
        assert not client.http_url.endswith("/")

    def test_component_name_default_is_g8ee(self):
        client = KVCacheClient()
        assert client.component_name is ComponentName.G8EE

    def test_default_init(self):
        with patch("app.clients.kv_cache_client.SettingsService"), \
             patch("app.clients.kv_cache_client.ListenSettings") as mock_listen:
            mock_listen.from_bootstrap.return_value = MagicMock(http_url="https://default-g8es:9000")
            client = KVCacheClient()
            assert client.http_url == "https://default-g8es:9000"

    def test_init_with_listen_settings(self):
        mock_listen = MagicMock(spec=ListenSettings)
        mock_listen.http_url = "https://explicit-g8es:9000"
        client = KVCacheClient(listen_settings=mock_listen)
        assert client.http_url == "https://explicit-g8es:9000"

    def test_is_healthy_false_on_init(self):
        client = KVCacheClient()
        assert client.is_healthy() is False

@pytest.mark.asyncio
class TestKVCacheClientRequest:
    async def test_request_success_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        result = await client._request("GET", "/health")
        assert result == {"status": "ok"}

    async def test_request_success_text(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text="plain text")
        result = await client._request("GET", "/some-path")
        assert result == "plain text"

    async def test_request_error_400_with_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=400, text='{"error": "bad request"}')
        with pytest.raises(NetworkError) as exc:
            await client._request("GET", "/fail")
        assert "bad request" in str(exc.value)
        assert exc.value.error_detail.details["http_status"] == 400

    async def test_request_error_500_plain_text(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=500, text="Internal Server Error")
        with pytest.raises(NetworkError) as exc:
            await client._request("GET", "/fail")
        assert "HTTP 500" in str(exc.value)
        assert "Internal Server Error" in str(exc.value)

    async def test_request_exception(self, client, mock_session):
        mock_session.request.side_effect = Exception("Connection refused")
        with pytest.raises(NetworkError) as exc:
            await client._request("GET", "/fail")
        assert "HTTP request failed" in str(exc.value)
        assert "Connection refused" in str(exc.value)


@pytest.mark.asyncio
class TestKVCacheClientLifecycle:
    async def test_connect_success(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        # We need to ensure _get_http_session returns our mock
        with patch("app.clients.kv_cache_client.new_kv_http_session", return_value=mock_session):
            connected = await client.connect()
            assert connected is True
            assert client.is_healthy() is True

    async def test_connect_failure(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "error"}')
        with patch("app.clients.kv_cache_client.new_kv_http_session", return_value=mock_session):
            connected = await client.connect()
            assert connected is False
            assert client.is_healthy() is False

    async def test_connect_exception(self, client, mock_session):
        mock_session.request.side_effect = Exception("Down")
        with patch("app.clients.kv_cache_client.new_kv_http_session", return_value=mock_session):
            connected = await client.connect()
            assert connected is False
            assert client.is_healthy() is False

    async def test_health_check_success(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        healthy = await client.health_check()
        assert healthy is True

    async def test_health_check_failure(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=500, text="Error")
        healthy = await client.health_check()
        assert healthy is False

    async def test_close(self, client, mock_session):
        await client.close()
        mock_session.close.assert_called_once()
        assert client.is_healthy() is False

    async def test_close_already_closed(self, client, mock_session):
        mock_session.closed = True
        await client.close()
        mock_session.close.assert_not_called()
        assert client.is_healthy() is False

    async def test_close_no_session(self, client):
        client._session = None
        await client.close()
        assert client.is_healthy() is False

    async def test_ping(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        assert await client.ping() is True


@pytest.mark.asyncio
class TestKVCacheClientUnhealthyGuards:
    async def test_get_returns_none_when_unhealthy(self, disconnected_client):
        assert await disconnected_client.get("some:key") is None

    async def test_set_returns_false_when_unhealthy(self, disconnected_client):
        assert await disconnected_client.set("some:key", "value") is False

    async def test_incr_returns_zero_when_unhealthy(self, disconnected_client):
        assert await disconnected_client.incr("counter:key") == 0

    async def test_decr_returns_zero_when_unhealthy(self, disconnected_client):
        assert await disconnected_client.decr("counter:key") == 0
    async def test_get_success(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "bar"}')
        assert await client.get("foo") == "bar"

    async def test_get_failure(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.get("foo") is None

    async def test_set_success(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        assert await client.set("foo", "bar") is True

    async def test_set_with_ex(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        await client.set("foo", "bar", ex=60)
        args, kwargs = mock_session.request.call_args
        assert kwargs["json"] == {"value": "bar", "ttl": 60}

    async def test_set_with_px(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        await client.set("foo", "bar", px=2500)
        args, kwargs = mock_session.request.call_args
        assert kwargs["json"] == {"value": "bar", "ttl": 2}

    async def test_set_failure(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=500, text="Error")
        assert await client.set("foo", "bar") is False

    async def test_setex(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        assert await client.setex("foo", 10, "bar") is True

    async def test_delete(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        assert await client.delete("k1", "k2") == 2
        assert mock_session.request.call_count == 2

    async def test_exists(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=200, text='{"value": "v1"}'),
            MockResponse(status=404, text="Not found"),
        ]
        assert await client.exists("k1", "k2") == 1

    async def test_expire(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        assert await client.expire("foo", 30) is True

    async def test_ttl(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"ttl": 45}')
        assert await client.ttl("foo") == 45

    async def test_ttl_not_found(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.ttl("foo") == -2

    async def test_keys(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"keys": ["k1", "k2"]}')
        assert await client.keys("prefix:*") == ["k1", "k2"]

    async def test_delete_pattern(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"deleted": 5}')
        assert await client.delete_pattern("prefix:*") == 5


    async def test_delete_exception(self, client, mock_session):
        mock_session.request.side_effect = Exception("error")
        assert await client.delete("k1") == 0

    async def test_expire_exception(self, client, mock_session):
        mock_session.request.side_effect = Exception("error")
        assert await client.expire("foo", 30) is False

    async def test_keys_exception(self, client, mock_session):
        mock_session.request.side_effect = Exception("error")
        assert await client.keys() == []

    async def test_delete_pattern_exception(self, client, mock_session):
        mock_session.request.side_effect = Exception("error")
        assert await client.delete_pattern("prefix:*") == 0


@pytest.mark.asyncio
class TestKVCacheClientJSONOperations:
    async def test_get_json_not_found(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.get_json("foo") is None

    async def test_get_json_success(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "{\\"a\\": 1}"}')
        assert await client.get_json("foo") == {"a": 1}

    async def test_get_json_invalid(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "not json"}')
        assert await client.get_json("foo") == "not json"

    async def test_set_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"status": "ok"}')
        assert await client.set_json("foo", {"a": 1}) is True
        args, kwargs = mock_session.request.call_args
        assert kwargs["json"]["value"] == '{"a": 1}'


@pytest.mark.asyncio
class TestKVCacheClientHashOperations:
    async def test_hset_new(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=404, text="Not found"),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.hset("hkey", "f1", "v1") == 1
        args, kwargs = mock_session.request.call_args
        assert kwargs["json"]["value"] == '{"f1": "v1"}'

    async def test_hset_existing(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=200, text='{"value": "{\\"f1\\": \\"v1\\"}"}'),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.hset("hkey", "f2", "v2") == 1
        args, kwargs = mock_session.request.call_args
        assert "f1" in kwargs["json"]["value"]
        assert "f2" in kwargs["json"]["value"]

    async def test_hget(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "{\\"f1\\": \\"v1\\"}"}')
        assert await client.hget("hkey", "f1") == "v1"
        assert await client.hget("hkey", "f2") is None

    async def test_hgetall(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "{\\"f1\\": \\"v1\\"}"}')
        assert await client.hgetall("hkey") == {"f1": "v1"}

    async def test_hdel(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=200, text='{"value": "{\\"f1\\": \\"v1\\", \\"f2\\": \\"v2\\"}"}'),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.hdel("hkey", "f1") == 1
        args, kwargs = mock_session.request.call_args
        assert "f1" not in kwargs["json"]["value"]
        assert "f2" in kwargs["json"]["value"]


    async def test_hget_not_found(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.hget("hkey", "f1") is None

    async def test_hget_invalid_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "not json"}')
        assert await client.hget("hkey", "f1") is None

    async def test_hgetall_not_found(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.hgetall("hkey") is None

    async def test_hgetall_invalid_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "not json"}')
        assert await client.hgetall("hkey") is None

    async def test_hdel_not_found(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.hdel("hkey", "f1") == 0

    async def test_hdel_field_not_present(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "{\\"f1\\": \\"v1\\"}"}')
        assert await client.hdel("hkey", "f2") == 0

    async def test_hdel_invalid_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "not json"}')
        assert await client.hdel("hkey", "f1") == 0


@pytest.mark.asyncio
class TestKVCacheClientListOperations:
    async def test_rpush_new(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=404, text="Not found"),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.rpush("lkey", "a") == 1

    async def test_lpush_new(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=404, text="Not found"),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.lpush("lkey", "a") == 1

    async def test_lrange_not_found(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.lrange("lkey", 0, -1) == []

    async def test_lrange_invalid_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "not json"}')
        assert await client.lrange("lkey", 0, -1) == []

    async def test_llen_not_found(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.llen("lkey") == 0

    async def test_llen_invalid_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "not json"}')
        assert await client.llen("lkey") == 0

    async def test_ltrim_not_found(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=404, text="Not found")
        assert await client.ltrim("lkey", 0, -1) is True

    async def test_ltrim_invalid_json(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "not json"}')
        assert await client.ltrim("lkey", 0, -1) is True
    async def test_rpush(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=200, text='{"value": "[\\"a\\"]"}'),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.rpush("lkey", "b", "c") == 3
        args, kwargs = mock_session.request.call_args
        assert kwargs["json"]["value"] == '["a", "b", "c"]'

    async def test_lpush(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=200, text='{"value": "[\\"a\\"]"}'),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.lpush("lkey", "b", "c") == 3
        args, kwargs = mock_session.request.call_args
        assert kwargs["json"]["value"] == '["c", "b", "a"]'

    async def test_lrange(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "[1, 2, 3, 4, 5]"}')
        assert await client.lrange("lkey", 1, 3) == [2, 3, 4]
        assert await client.lrange("lkey", 2, -1) == [3, 4, 5]

    async def test_llen(self, client, mock_session):
        mock_session.request.return_value = MockResponse(status=200, text='{"value": "[1, 2, 3]"}')
        assert await client.llen("lkey") == 3

    async def test_ltrim(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=200, text='{"value": "[1, 2, 3, 4, 5]"}'),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.ltrim("lkey", 1, 3) is True
        args, kwargs = mock_session.request.call_args
        assert kwargs["json"]["value"] == "[2, 3, 4]"


@pytest.mark.asyncio
class TestKVCacheClientAtomicOperations:
    async def test_incr(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=200, text='{"value": "10"}'),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.incr("counter") == 11

    async def test_decr(self, client, mock_session):
        mock_session.request.side_effect = [
            MockResponse(status=200, text='{"value": "10"}'),  # get
            MockResponse(status=200, text='{"status": "ok"}'),  # set
        ]
        assert await client.decr("counter") == 9

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
