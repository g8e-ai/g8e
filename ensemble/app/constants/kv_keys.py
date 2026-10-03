# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Gateway document and query cache keys derived from protocol templates."""

from g8e.constants import KV, kv_key as _g8e_kv_key

CACHE_PREFIX: str = KV["kv_keys"]["CachePrefix"]["value"]


class KVKey:
    """Canonical KV store keys. All keys use the version prefix from protocol constants."""

    @classmethod
    def doc(cls, collection: str, document_id: str) -> str:
        """g8e:cache:doc:{collection}:{id}"""
        return _g8e_kv_key("CacheDoc", collection=collection, id=document_id)

    @classmethod
    def query(cls, collection: str, query_hash: str) -> str:
        """g8e:cache:query:{collection}:{hash}"""
        return _g8e_kv_key("CacheQuery", collection=collection, hash=query_hash)


def _derive_prefix(key_name: str) -> str:
    """Extract the static prefix from a g8e KV key template (everything before the first placeholder)."""
    template = KV["kv_keys"][key_name]["value"]
    return template.split("{")[0]


class KVKeyPrefix:
    """Canonical KV store key prefixes. All prefixes use the version prefix."""

    CACHE_DOC = _derive_prefix("CacheDoc")
    CACHE_QUERY = _derive_prefix("CacheQuery")
