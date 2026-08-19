# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 8 — KV key patterns sourced from g8e.constants.KV."""

import pytest

from g8e.constants import KV, kv_key as _g8e_kv_key

from app.constants.kv_keys import CACHE_PREFIX, KVKey, KVKeyPrefix

pytestmark = pytest.mark.unit


class TestCachePrefix:
    """Verify CACHE_PREFIX is sourced from g8e constants."""

    def test_cache_prefix_matches_g8e(self):
        assert CACHE_PREFIX == KV["kv_keys"]["CachePrefix"]["value"]

    def test_cache_prefix_value(self):
        assert CACHE_PREFIX == "g8e"


class TestKVKeyMethodsFromG8e:
    """Verify KVKey methods produce keys matching g8e KV templates."""

    def test_doc(self):
        result = KVKey.doc("users", "user-123")
        expected = _g8e_kv_key("CacheDoc", **{"collection": "users", "id": "user-123"})
        assert result == expected
        assert result == "g8e:cache:doc:users:user-123"

    def test_query(self):
        result = KVKey.query("users", "abc123")
        expected = _g8e_kv_key("CacheQuery", **{"collection": "users", "hash": "abc123"})
        assert result == expected
        assert result == "g8e:cache:query:users:abc123"

    def test_web_session(self):
        result = KVKey.web_session("sess-123")
        expected = _g8e_kv_key("SessionWeb", **{"session.type": "web", "session.id": "sess-123"})
        assert result == expected
        assert result == "g8e:sessions:web:sess-123"

    def test_operator_session(self):
        result = KVKey.operator_session("op-sess-456")
        expected = _g8e_kv_key(
            "SessionOperator", **{"operator.session.id": "op-sess-456"}
        )
        assert result == expected
        assert result == "g8e:sessions:operator:op-sess-456"

    def test_session_operator_bind(self):
        result = KVKey.session_operator_bind("op-sess-456")
        expected = _g8e_kv_key("SessionOperatorBind", **{"operator.session.id": "op-sess-456"})
        assert result == expected
        assert result == "g8e:sessions:operator:op-sess-456:bind"

    def test_session_web_bind(self):
        result = KVKey.session_web_bind("web-sess-789")
        expected = _g8e_kv_key("SessionWebBind", **{"web.session.id": "web-sess-789"})
        assert result == expected
        assert result == "g8e:sessions:web:web-sess-789:bind"

    def test_operator_first_deployed(self):
        result = KVKey.operator_first_deployed("op-001")
        expected = _g8e_kv_key("OperatorFirstDeployed", **{"operator.id": "op-001"})
        assert result == expected
        assert result == "g8e:operator:op-001:first.deployed"

    def test_operator_tracked_status(self):
        result = KVKey.operator_tracked_status("op-001")
        expected = _g8e_kv_key("OperatorTrackedStatus", **{"operator.id": "op-001"})
        assert result == expected
        assert result == "g8e:operator:op-001:tracked.status"

    def test_user_operators(self):
        result = KVKey.user_operators("user-123")
        expected = _g8e_kv_key("UserOperators", **{"user.id": "user-123"})
        assert result == expected
        assert result == "g8e:user:user-123:operators"

    def test_user_web_sessions(self):
        result = KVKey.user_web_sessions("user-123")
        expected = _g8e_kv_key("UserWebSessions", **{"user.id": "user-123"})
        assert result == expected
        assert result == "g8e:user:user-123:web_sessions"

    def test_user_memories(self):
        result = KVKey.user_memories("user-123")
        expected = _g8e_kv_key("UserMemories", **{"user.id": "user-123"})
        assert result == expected
        assert result == "g8e:user:user-123:memories"

    def test_attachment(self):
        result = KVKey.attachment("inv-001", "att-002")
        expected = _g8e_kv_key(
            "InvestigationAttachment",
            **{"investigation.id": "inv-001", "attachment.id": "att-002"},
        )
        assert result == expected
        assert result == "g8e:investigation:inv-001:attachment:att-002"

    def test_attachment_index(self):
        result = KVKey.attachment_index("inv-001")
        expected = _g8e_kv_key(
            "InvestigationAttachmentIndex", **{"investigation.id": "inv-001"}
        )
        assert result == expected
        assert result == "g8e:investigation:inv-001:attachment.index"

    def test_nonce(self):
        result = KVKey.nonce("nonce-abc")
        expected = _g8e_kv_key("AuthNonce", **{"nonce": "nonce-abc"})
        assert result == expected
        assert result == "g8e:auth:nonce:nonce-abc"

    def test_download_token(self):
        result = KVKey.download_token("tok-123")
        expected = _g8e_kv_key("AuthTokenDownload", **{"token": "tok-123"})
        assert result == expected
        assert result == "g8e:auth:token:download:tok-123"

    def test_device_link(self):
        result = KVKey.device_link("tok-123")
        expected = _g8e_kv_key("AuthTokenDevice", **{"token": "tok-123"})
        assert result == expected
        assert result == "g8e:auth:token:device:tok-123"

    def test_device_link_uses(self):
        result = KVKey.device_link_uses("tok-123")
        expected = _g8e_kv_key("AuthTokenDeviceUses", **{"token": "tok-123"})
        assert result == expected
        assert result == "g8e:auth:token:device:tok-123:uses"

    def test_device_link_fingerprints(self):
        result = KVKey.device_link_fingerprints("tok-123")
        expected = _g8e_kv_key("AuthTokenDeviceFingerprints", **{"token": "tok-123"})
        assert result == expected
        assert result == "g8e:auth:token:device:tok-123:fingerprints"

    def test_device_link_registration_lock(self):
        result = KVKey.device_link_registration_lock("tok-123")
        expected = _g8e_kv_key("AuthTokenDeviceRegLock", **{"token": "tok-123"})
        assert result == expected
        assert result == "g8e:auth:token:device:tok-123:reg.lock"

    def test_device_link_list(self):
        result = KVKey.device_link_list("user-123")
        expected = _g8e_kv_key("AuthDeviceList", **{"user.id": "user-123"})
        assert result == expected
        assert result == "g8e:auth:device.list:user-123"

    def test_login_failed(self):
        result = KVKey.login_failed("user@example.com")
        expected = _g8e_kv_key("AuthLoginFailed", **{"identifier": "user@example.com"})
        assert result == expected
        assert result == "g8e:auth:login:user@example.com:failed"

    def test_login_lock(self):
        result = KVKey.login_lock("user@example.com")
        expected = _g8e_kv_key("AuthLoginLock", **{"identifier": "user@example.com"})
        assert result == expected
        assert result == "g8e:auth:login:user@example.com:lock"

    def test_login_ip_accounts(self):
        result = KVKey.login_ip_accounts("10.0.0.1")
        expected = _g8e_kv_key("AuthLoginIPAccounts", **{"ip": "10.0.0.1"})
        assert result == expected
        assert result == "g8e:auth:login:ip:10.0.0.1:accounts"

    def test_pending_cmd(self):
        result = KVKey.pending_cmd("exec-001")
        expected = _g8e_kv_key("ExecutionPendingCmd", **{"execution.id": "exec-001"})
        assert result == expected
        assert result == "g8e:execution:exec-001:pending.cmd"


class TestSessionsPluralConvention:
    """Verify all session-related keys use 'sessions' (plural) per g8e protocol."""

    @pytest.mark.parametrize(
        "key",
        [
            KVKey.web_session("sess-123"),
            KVKey.operator_session("op-sess-456"),
            KVKey.session_operator_bind("op-sess-456"),
            KVKey.session_web_bind("web-sess-789"),
        ],
    )
    def test_uses_sessions_plural(self, key: str):
        assert ":sessions:" in key
        assert ":session:" not in key


class TestG8eeSpecificKeys:
    """Verify g8ee-specific keys (not in g8e protocol) are local strings."""

    def test_cli_session(self):
        result = KVKey.cli_session("cli-sess-001")
        assert result == "g8e:sessions:cli:cli-sess-001"
        assert ":sessions:" in result

    def test_operator_slot_counter(self):
        result = KVKey.operator_slot_counter("user-123")
        assert result == "g8e:user:user-123:operator.slot.counter"


class TestKVKeyPrefix:
    """Verify KVKeyPrefix values are derived from g8e templates."""

    def test_cache_doc_prefix(self):
        assert KVKeyPrefix.CACHE_DOC == "g8e:cache:doc:"

    def test_cache_query_prefix(self):
        assert KVKeyPrefix.CACHE_QUERY == "g8e:cache:query:"

    def test_cache_doc_prefix_matches_g8e_template(self):
        template = KV["kv_keys"]["CacheDoc"]["value"]
        assert KVKeyPrefix.CACHE_DOC == template.split("{")[0]

    def test_cache_query_prefix_matches_g8e_template(self):
        template = KV["kv_keys"]["CacheQuery"]["value"]
        assert KVKeyPrefix.CACHE_QUERY == template.split("{")[0]
