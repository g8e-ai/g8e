# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Canonical KV store keys.

Key patterns are sourced from g8e.constants.KV to stay in sync with the
protocol. The KVKey class methods wrap g8e's kv_key() formatter, translating
g8ee-style parameter names to the protocol's dotted placeholder names.

g8ee-specific keys not in the protocol (cli_session, operator_slot_counter)
are defined locally using the protocol's ``sessions`` (plural) convention.
"""

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

    @classmethod
    def operator_session(cls, operator_session_id: str) -> str:
        """g8e:sessions:operator:{operator.session.id}"""
        return _g8e_kv_key("SessionOperator", **{"operator.session.id": operator_session_id})

    @classmethod
    def web_session(cls, web_session_id: str) -> str:
        """g8e:sessions:web:{session.id}"""
        return _g8e_kv_key("SessionWeb", **{"session.type": "web", "session.id": web_session_id})

    @classmethod
    def cli_session(cls, cli_session_id: str) -> str:
        """g8e:sessions:cli:{session.id} (g8ee-specific, not in protocol)"""
        return f"{CACHE_PREFIX}:sessions:cli:{cli_session_id}"

    @classmethod
    def session_operator_bind(cls, operator_session_id: str) -> str:
        """g8e:sessions:operator:{operator.session.id}:bind"""
        return _g8e_kv_key("SessionOperatorBind", **{"operator.session.id": operator_session_id})

    @classmethod
    def session_web_bind(cls, web_session_id: str) -> str:
        """g8e:sessions:web:{web.session.id}:bind"""
        return _g8e_kv_key("SessionWebBind", **{"web.session.id": web_session_id})

    @classmethod
    def operator_first_deployed(cls, operator_id: str) -> str:
        """g8e:operator:{operator.id}:first.deployed"""
        return _g8e_kv_key("OperatorFirstDeployed", **{"operator.id": operator_id})

    @classmethod
    def operator_tracked_status(cls, operator_id: str) -> str:
        """g8e:operator:{operator.id}:tracked.status"""
        return _g8e_kv_key("OperatorTrackedStatus", **{"operator.id": operator_id})

    @classmethod
    def user_operators(cls, user_id: str) -> str:
        """g8e:user:{user.id}:operators"""
        return _g8e_kv_key("UserOperators", **{"user.id": user_id})

    @classmethod
    def operator_slot_counter(cls, user_id: str) -> str:
        """g8e:user:{user.id}:operator.slot.counter (g8ee-specific, not in protocol)"""
        return f"{CACHE_PREFIX}:user:{user_id}:operator.slot.counter"

    @classmethod
    def user_web_sessions(cls, user_id: str) -> str:
        """g8e:user:{user.id}:web_sessions"""
        return _g8e_kv_key("UserWebSessions", **{"user.id": user_id})

    @classmethod
    def user_memories(cls, user_id: str) -> str:
        """g8e:user:{user.id}:memories"""
        return _g8e_kv_key("UserMemories", **{"user.id": user_id})

    @classmethod
    def attachment(cls, investigation_id: str, attachment_id: str) -> str:
        """g8e:investigation:{investigation.id}:attachment:{attachment.id}"""
        return _g8e_kv_key(
            "InvestigationAttachment",
            **{"investigation.id": investigation_id, "attachment.id": attachment_id},
        )

    @classmethod
    def attachment_index(cls, investigation_id: str) -> str:
        """g8e:investigation:{investigation.id}:attachment.index"""
        return _g8e_kv_key("InvestigationAttachmentIndex", **{"investigation.id": investigation_id})

    @classmethod
    def nonce(cls, nonce: str) -> str:
        """g8e:auth:nonce:{nonce}"""
        return _g8e_kv_key("AuthNonce", nonce=nonce)

    @classmethod
    def download_token(cls, token: str) -> str:
        """g8e:auth:token:download:{token}"""
        return _g8e_kv_key("AuthTokenDownload", token=token)

    @classmethod
    def device_link(cls, token: str) -> str:
        """g8e:auth:token:device:{token}"""
        return _g8e_kv_key("AuthTokenDevice", token=token)

    @classmethod
    def device_link_uses(cls, token: str) -> str:
        """g8e:auth:token:device:{token}:uses"""
        return _g8e_kv_key("AuthTokenDeviceUses", token=token)

    @classmethod
    def device_link_fingerprints(cls, token: str) -> str:
        """g8e:auth:token:device:{token}:fingerprints"""
        return _g8e_kv_key("AuthTokenDeviceFingerprints", token=token)

    @classmethod
    def device_link_registration_lock(cls, token: str) -> str:
        """g8e:auth:token:device:{token}:reg.lock"""
        return _g8e_kv_key("AuthTokenDeviceRegLock", token=token)

    @classmethod
    def device_link_list(cls, user_id: str) -> str:
        """g8e:auth:device.list:{user.id}"""
        return _g8e_kv_key("AuthDeviceList", **{"user.id": user_id})

    @classmethod
    def login_failed(cls, identifier: str) -> str:
        """g8e:auth:login:{identifier}:failed"""
        return _g8e_kv_key("AuthLoginFailed", identifier=identifier)

    @classmethod
    def login_lock(cls, identifier: str) -> str:
        """g8e:auth:login:{identifier}:lock"""
        return _g8e_kv_key("AuthLoginLock", identifier=identifier)

    @classmethod
    def login_ip_accounts(cls, ip: str) -> str:
        """g8e:auth:login:ip:{ip}:accounts"""
        return _g8e_kv_key("AuthLoginIPAccounts", ip=ip)

    @classmethod
    def pending_cmd(cls, execution_id: str) -> str:
        """g8e:execution:{execution.id}:pending.cmd"""
        return _g8e_kv_key("ExecutionPendingCmd", **{"execution.id": execution_id})


def _derive_prefix(key_name: str) -> str:
    """Extract the static prefix from a g8e KV key template (everything before the first placeholder)."""
    template = KV["kv_keys"][key_name]["value"]
    return template.split("{")[0]


class KVKeyPrefix:
    """Canonical KV store key prefixes. All prefixes use the version prefix."""

    CACHE_DOC = _derive_prefix("CacheDoc")
    CACHE_QUERY = _derive_prefix("CacheQuery")
