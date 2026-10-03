# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Keep Gateway state primitives out of the ensemble cache client."""

import importlib.util

import pytest

from app.clients.kv_cache_client import KVCacheClient
from app.constants.kv_keys import KVKey
from app.db.kv_service import KVService

pytestmark = pytest.mark.unit


def test_replay_guard_is_owned_by_gateway():
    assert importlib.util.find_spec("app.security.request_timestamp") is None
    assert not hasattr(KVKey, "nonce")


@pytest.mark.parametrize(
    "method",
    [
        "hset",
        "hget",
        "hgetall",
        "hdel",
        "rpush",
        "lpush",
        "lrange",
        "llen",
        "ltrim",
        "incr",
        "decr",
        "expire",
        "ttl",
        "exists",
        "setex",
        "ping",
    ],
)
def test_cache_client_does_not_emulate_gateway_state_primitives(method):
    assert not hasattr(KVCacheClient, method)
    assert not hasattr(KVService, method)
