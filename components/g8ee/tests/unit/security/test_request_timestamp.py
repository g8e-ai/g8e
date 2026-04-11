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


from datetime import UTC, datetime, timedelta
from unittest.mock import AsyncMock

import pytest

from app.constants import KVKey, NonceErrorCode, TimestampErrorCode
from tests.fakes.fake_vsodb_clients import FakeKVClient
from tests.fakes.builder import create_mock_cache_aside_service
from app.security.request_timestamp import (
    NONCE_TTL_SECONDS,
    TIMESTAMP_WINDOW_SECONDS,
    NonceCheckResult,
    RequestTimestampValidator,
    RequestValidationResult,
    _nonce_cache,
    check_nonce_kv,
    check_nonce_memory,
    validate_message_timestamp,
    validate_request_timestamp,
    validate_timestamp,
)

pytestmark = [pytest.mark.unit]


def _utc_now() -> datetime:
    return datetime.now(UTC)


def _iso(dt: datetime) -> str:
    return dt.isoformat()


@pytest.fixture(autouse=True)
def clear_nonce_cache():
    _nonce_cache.clear()
    yield
    _nonce_cache.clear()


class TestValidateTimestamp:

    def test_valid_timestamp_within_window(self):
        ts = _iso(_utc_now())
        result = validate_timestamp(ts)
        assert result.is_valid is True
        assert result.error is None
        assert result.skew_seconds is not None
        assert result.skew_seconds < TIMESTAMP_WINDOW_SECONDS

    def test_valid_datetime_object(self):
        result = validate_timestamp(_utc_now())
        assert result.is_valid is True

    def test_valid_z_suffix_timestamp(self):
        ts = _utc_now().strftime("%Y-%m-%dT%H:%M:%SZ")
        result = validate_timestamp(ts)
        assert result.is_valid is True

    def test_missing_timestamp_required(self):
        result = validate_timestamp(None)
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.MISSING_TIMESTAMP

    def test_missing_timestamp_allowed(self):
        result = validate_timestamp(None, allow_missing=True)
        assert result.is_valid is True
        assert result.error is None

    def test_empty_string_treated_as_missing(self):
        result = validate_timestamp("")
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.MISSING_TIMESTAMP

    def test_invalid_format_returns_error(self):
        result = validate_timestamp("not-a-timestamp")
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.INVALID_FORMAT

    def test_timestamp_too_old(self):
        old = _utc_now() - timedelta(seconds=TIMESTAMP_WINDOW_SECONDS + 60)
        result = validate_timestamp(_iso(old))
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.OUTSIDE_WINDOW
        assert result.skew_seconds is not None
        assert result.skew_seconds > TIMESTAMP_WINDOW_SECONDS

    def test_timestamp_too_far_future(self):
        future = _utc_now() + timedelta(seconds=TIMESTAMP_WINDOW_SECONDS + 60)
        result = validate_timestamp(_iso(future))
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.OUTSIDE_WINDOW

    def test_timestamp_exactly_at_boundary_valid(self):
        at_boundary = _utc_now() - timedelta(seconds=TIMESTAMP_WINDOW_SECONDS - 1)
        result = validate_timestamp(_iso(at_boundary))
        assert result.is_valid is True

    def test_error_code_is_str_enum_no_value_needed(self):
        result = validate_timestamp("bad")
        assert result.error_code == TimestampErrorCode.INVALID_FORMAT
        assert isinstance(result.error_code, str)


class TestCheckNonceMemory:

    def test_first_use_not_replay(self):
        result = check_nonce_memory("unique-nonce-abc")
        assert result.success is True
        assert result.is_replay is False

    def test_second_use_is_replay(self):
        nonce = "replay-nonce-xyz"
        check_nonce_memory(nonce)
        result = check_nonce_memory(nonce)
        assert result.success is True
        assert result.is_replay is True

    def test_different_nonces_not_replay(self):
        check_nonce_memory("nonce-one")
        result = check_nonce_memory("nonce-two")
        assert result.is_replay is False

    def test_nonce_cache_cleanup_triggered(self):
        for i in range(10001):
            _nonce_cache[f"old-nonce-{i}"] = _utc_now() - timedelta(seconds=700)
        check_nonce_memory("trigger-cleanup")
        assert len(_nonce_cache) < 10001


class TestCheckNonceKV:
    pytestmark = pytest.mark.asyncio(loop_scope="session")

    async def test_new_nonce_not_replay(self):
        kv_mock = FakeKVClient()
        cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
        result = await check_nonce_kv("new-nonce", cache_aside_service)
        assert result.success is True
        assert result.is_replay is False
        kv_mock.set.assert_awaited_once()

    async def test_existing_nonce_is_replay(self):
        kv_mock = FakeKVClient()
        kv_mock.set = AsyncMock(return_value=None)
        cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
        result = await check_nonce_kv("seen-nonce", cache_aside_service)
        assert result.success is True
        assert result.is_replay is True

    async def test_kv_exception_returns_failure(self):
        kv_mock = FakeKVClient()
        kv_mock.set = AsyncMock(side_effect=Exception("connection refused"))
        cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
        result = await check_nonce_kv("any-nonce", cache_aside_service)
        assert result.success is False
        assert result.error_code == NonceErrorCode.CHECK_FAILED

    async def test_nonce_key_prefixed(self):
        kv_mock = FakeKVClient()
        cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
        await check_nonce_kv("abc123", cache_aside_service)
        call_args = kv_mock.set.call_args
        assert call_args[0][0] == KVKey.nonce("abc123")

    async def test_nonce_key_uses_ttl(self):
        kv_mock = FakeKVClient()
        cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
        await check_nonce_kv("ttl-check", cache_aside_service)
        call_args = kv_mock.set.call_args
        assert NONCE_TTL_SECONDS in call_args[0] or call_args[1].get("ex") == NONCE_TTL_SECONDS


class TestValidateRequestTimestamp:
    pytestmark = pytest.mark.asyncio(loop_scope="session")

    async def test_valid_timestamp_no_nonce(self):
        ts = _iso(_utc_now())
        result = await validate_request_timestamp(
            ts, nonce="", require_nonce=False, cache_aside_service=None, context={}
        )
        assert result.is_valid is True

    async def test_missing_timestamp_rejected(self):
        result = await validate_request_timestamp(
            None, nonce="", require_nonce=False, cache_aside_service=None, context={}
        )
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.MISSING_TIMESTAMP

    async def test_expired_timestamp_rejected(self):
        old = _iso(_utc_now() - timedelta(seconds=TIMESTAMP_WINDOW_SECONDS + 60))
        result = await validate_request_timestamp(
            old, nonce="", require_nonce=False, cache_aside_service=None, context={}
        )
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.OUTSIDE_WINDOW

    async def test_valid_with_nonce(self):
        ts = _iso(_utc_now())
        result = await validate_request_timestamp(
            ts, nonce="nonce-valid-1", require_nonce=False, cache_aside_service=None, context={}
        )
        assert result.is_valid is True

    async def test_replay_nonce_rejected(self):
        ts = _iso(_utc_now())
        nonce = "replay-check-nonce"
        await validate_request_timestamp(
            ts, nonce=nonce, require_nonce=False, cache_aside_service=None, context={}
        )
        result = await validate_request_timestamp(
            ts, nonce=nonce, require_nonce=False, cache_aside_service=None, context={}
        )
        assert result.is_valid is False
        assert result.error_code == NonceErrorCode.REPLAY_DETECTED

    async def test_missing_nonce_when_required(self):
        ts = _iso(_utc_now())
        result = await validate_request_timestamp(
            ts, nonce=None, require_nonce=True, cache_aside_service=None, context={}
        )
        assert result.is_valid is False
        assert result.error_code == NonceErrorCode.MISSING_REQUIRED

    async def test_missing_nonce_when_not_required(self):
        ts = _iso(_utc_now())
        result = await validate_request_timestamp(
            ts, nonce=None, require_nonce=False, cache_aside_service=None, context={}
        )
        assert result.is_valid is True

    async def test_kv_cache_client_used_when_provided(self):
        kv_mock = FakeKVClient()
        cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
        ts = _iso(_utc_now())
        result = await validate_request_timestamp(
            ts, nonce="kv-nonce", require_nonce=False, cache_aside_service=cache_aside_service, context={}
        )
        assert result.is_valid is True
        kv_mock.set.assert_awaited_once()

    async def test_kv_failure_falls_back_to_memory(self):
        kv_mock = FakeKVClient()
        kv_mock.set = AsyncMock(side_effect=Exception("kv down"))
        cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
        ts = _iso(_utc_now())
        result = await validate_request_timestamp(
            ts, nonce="fallback-nonce", require_nonce=False, cache_aside_service=cache_aside_service, context={}
        )
        assert result.is_valid is True

    async def test_error_code_is_str_value(self):
        result = await validate_request_timestamp(
            None, nonce="", require_nonce=False, cache_aside_service=None, context={}
        )
        assert isinstance(result.error_code, str)
        assert result.error_code == TimestampErrorCode.MISSING_TIMESTAMP

    async def test_context_dict_accepted(self):
        ts = _iso(_utc_now())
        result = await validate_request_timestamp(
            ts, nonce="", require_nonce=False, cache_aside_service=None, context={"operator_id": "op-123"}
        )
        assert result.is_valid is True


class TestValidateMessageTimestamp:

    def test_valid_message_with_timestamp(self):
        msg = {"timestamp": _iso(_utc_now())}
        result = validate_message_timestamp(msg)
        assert result.is_valid is True

    def test_timestamp_in_metadata(self):
        msg = {"metadata": {"timestamp": _iso(_utc_now())}}
        result = validate_message_timestamp(msg)
        assert result.is_valid is True

    def test_missing_timestamp_rejected(self):
        result = validate_message_timestamp({})
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.MISSING_TIMESTAMP

    def test_expired_timestamp_rejected(self):
        old = _iso(_utc_now() - timedelta(seconds=TIMESTAMP_WINDOW_SECONDS + 60))
        result = validate_message_timestamp({"timestamp": old})
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.OUTSIDE_WINDOW

    def test_replay_nonce_rejected(self):
        msg = {"timestamp": _iso(_utc_now()), "request_nonce": "msg-replay-nonce"}
        validate_message_timestamp(msg)
        result = validate_message_timestamp(msg)
        assert result.is_valid is False
        assert result.error_code == NonceErrorCode.REPLAY_DETECTED

    def test_nonce_in_metadata(self):
        nonce = "metadata-nonce-unique"
        msg = {
            "timestamp": _iso(_utc_now()),
            "metadata": {"request_nonce": nonce}
        }
        result = validate_message_timestamp(msg)
        assert result.is_valid is True

    def test_missing_nonce_when_required(self):
        msg = {"timestamp": _iso(_utc_now())}
        result = validate_message_timestamp(msg, require_nonce=True)
        assert result.is_valid is False
        assert result.error_code == NonceErrorCode.MISSING_REQUIRED

    def test_custom_field_names(self):
        msg = {"ts": _iso(_utc_now()), "rid": "custom-nonce-field"}
        result = validate_message_timestamp(msg, timestamp_field="ts", nonce_field="rid")
        assert result.is_valid is True


class TestRequestTimestampValidator:

    def test_validate_sync_valid(self):
        validator = RequestTimestampValidator(cache_aside_service=None)
        result = validator.validate_sync(_iso(_utc_now()), nonce="")
        assert result.is_valid is True

    def test_validate_sync_invalid_timestamp(self):
        validator = RequestTimestampValidator(cache_aside_service=None)
        result = validator.validate_sync("garbage", nonce="")
        assert result.is_valid is False
        assert result.error_code == TimestampErrorCode.INVALID_FORMAT

    def test_validate_sync_replay_detected(self):
        validator = RequestTimestampValidator(cache_aside_service=None)
        ts = _iso(_utc_now())
        nonce = "sync-replay-nonce"
        validator.validate_sync(ts, nonce=nonce)
        result = validator.validate_sync(ts, nonce=nonce)
        assert result.is_valid is False
        assert result.error_code == NonceErrorCode.REPLAY_DETECTED

    def test_validate_sync_require_nonce_missing(self):
        validator = RequestTimestampValidator(cache_aside_service=None, require_nonce=True)
        result = validator.validate_sync(_iso(_utc_now()), nonce="")
        assert result.is_valid is False
        assert result.error_code == NonceErrorCode.MISSING_REQUIRED

    def test_validate_sync_require_nonce_provided(self):
        validator = RequestTimestampValidator(cache_aside_service=None, require_nonce=True)
        result = validator.validate_sync(_iso(_utc_now()), nonce="required-nonce-ok")
        assert result.is_valid is True

    def test_validate_message_delegates(self):
        validator = RequestTimestampValidator(cache_aside_service=None)
        msg = {"timestamp": _iso(_utc_now())}
        result = validator.validate_message(msg)
        assert result.is_valid is True

    def test_validate_message_require_nonce_propagated(self):
        validator = RequestTimestampValidator(cache_aside_service=None, require_nonce=True)
        msg = {"timestamp": _iso(_utc_now())}
        result = validator.validate_message(msg)
        assert result.is_valid is False
        assert result.error_code == NonceErrorCode.MISSING_REQUIRED


class TestRequestTimestampValidatorAsync:
    pytestmark = pytest.mark.asyncio(loop_scope="session")

    async def test_validate_async_valid(self):
        validator = RequestTimestampValidator(cache_aside_service=None)
        result = await validator.validate_async(_iso(_utc_now()), nonce="", context={})
        assert result.is_valid is True

    async def test_validate_async_with_kv_cache_client(self):
        kv_mock = FakeKVClient()
        cache_aside_service = create_mock_cache_aside_service(kv_cache_client=kv_mock)
        validator = RequestTimestampValidator(cache_aside_service=cache_aside_service)
        result = await validator.validate_async(_iso(_utc_now()), nonce="async-kv-nonce", context={})
        assert result.is_valid is True


class TestResultModels:

    def test_nonce_check_result_has_error_code_field(self):
        result = NonceCheckResult(
            success=False,
            error="kv failure",
            error_code=NonceErrorCode.CHECK_FAILED
        )
        assert result.error_code == NonceErrorCode.CHECK_FAILED

    def test_request_validation_result_error_code_is_str_value(self):
        result = RequestValidationResult(
            is_valid=False,
            error="bad",
            error_code=TimestampErrorCode.MISSING_TIMESTAMP
        )
        assert isinstance(result.error_code, str)
        assert result.error_code == TimestampErrorCode.MISSING_TIMESTAMP

    def test_request_validation_result_accepts_nonce_error_code(self):
        result = RequestValidationResult(
            is_valid=False,
            error="replay",
            error_code=NonceErrorCode.REPLAY_DETECTED
        )
        assert result.error_code == NonceErrorCode.REPLAY_DETECTED
