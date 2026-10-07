# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 8 — KV key patterns sourced from g8e.constants.KV."""

import pytest
from g8e.constants import KV
from g8e.constants import kv_key as _g8e_kv_key

from app.constants.kv_keys import CACHE_PREFIX, KVKey, KVKeyPrefix

pytestmark = pytest.mark.unit


class TestCachePrefix:
    """Verify CACHE_PREFIX is sourced from g8e constants."""

    def test_cache_prefix_matches_g8e(self):
        assert KV["kv_keys"]["CachePrefix"]["value"] == CACHE_PREFIX

    def test_cache_prefix_value(self):
        assert CACHE_PREFIX == "g8e"


class TestKVKeyMethodsFromG8e:
    """Verify KVKey methods produce keys matching g8e KV templates."""

    def test_doc(self):
        result = KVKey.doc("users", "user-123")
        expected = _g8e_kv_key("CacheDoc", collection="users", id="user-123")
        assert result == expected
        assert result == "g8e:cache:doc:users:user-123"

    def test_query(self):
        result = KVKey.query("users", "abc123")
        expected = _g8e_kv_key("CacheQuery", collection="users", hash="abc123")
        assert result == expected
        assert result == "g8e:cache:query:users:abc123"


class TestSessionsPluralConvention:
    """Verify all session-related keys use 'sessions' (plural) per g8e protocol."""


class TestG8eeSpecificKeys:
    """Verify g8ee-specific keys (not in g8e protocol) are local strings."""


class TestKVKeyPrefix:
    """Verify KVKeyPrefix values are derived from g8e templates."""

    def test_cache_doc_prefix(self):
        assert KVKeyPrefix.CACHE_DOC == "g8e:cache:doc:"

    def test_cache_query_prefix(self):
        assert KVKeyPrefix.CACHE_QUERY == "g8e:cache:query:"

    def test_cache_doc_prefix_matches_g8e_template(self):
        template = KV["kv_keys"]["CacheDoc"]["value"]
        assert template.split("{")[0] == KVKeyPrefix.CACHE_DOC

    def test_cache_query_prefix_matches_g8e_template(self):
        template = KV["kv_keys"]["CacheQuery"]["value"]
        assert template.split("{")[0] == KVKeyPrefix.CACHE_QUERY
