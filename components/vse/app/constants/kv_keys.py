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

import json
from app.constants.paths import PATHS

_SHARED_DIR = PATHS["infra"]["shared_constants_dir"]

def _load(filename: str) -> dict[str, object]:
    path = _SHARED_DIR + "/" + filename
    try:
        with open(path) as f:
            return json.load(f)
    except FileNotFoundError as e:
        raise RuntimeError(f"Shared constants file not found: {path}") from e
    except json.JSONDecodeError as e:
        raise RuntimeError(f"Invalid JSON in shared constants file {path}: {e}") from e

_KV: dict[str, object] = _load("kv_keys.json")

CACHE_PREFIX = _KV["cache.prefix"]

class KVKey:
    """Canonical KV store keys. All keys use the version prefix from shared constants."""

    @classmethod
    def doc(cls, collection: str, document_id: str) -> str:
        """g8e:cache:doc:{collection}:{id}"""
        return f"{CACHE_PREFIX}:cache:doc:{collection}:{document_id}"

    @classmethod
    def query(cls, collection: str, query_hash: str) -> str:
        """g8e:cache:query:{collection}:{hash}"""
        return f"{CACHE_PREFIX}:cache:query:{collection}:{query_hash}"

    @classmethod
    def session(cls, session_type: str, session_id: str) -> str:
        """g8e:session:{session.type}:{session.id}"""
        return f"{CACHE_PREFIX}:session:{session_type}:{session_id}"

    @classmethod
    def session_operator_bind(cls, operator_session_id: str) -> str:
        """g8e:session:operator:{g8e.session.id}:bind"""
        return f"{CACHE_PREFIX}:session:operator:{operator_session_id}:bind"

    @classmethod
    def session_web_bind(cls, web_session_id: str) -> str:
        """g8e:session:web:{web.session.id}:bind"""
        return f"{CACHE_PREFIX}:session:web:{web_session_id}:bind"

    @classmethod
    def operator_first_deployed(cls, operator_id: str) -> str:
        """g8e:operator:{g8e.id}:first.deployed"""
        return f"{CACHE_PREFIX}:operator:{operator_id}:first.deployed"

    @classmethod
    def operator_tracked_status(cls, operator_id: str) -> str:
        """g8e:operator:{g8e.id}:tracked.status"""
        return f"{CACHE_PREFIX}:operator:{operator_id}:tracked.status"

    @classmethod
    def user_operators(cls, user_id: str) -> str:
        """g8e:user:{user.id}:operators"""
        return f"{CACHE_PREFIX}:user:{user_id}:operators"

    @classmethod
    def user_web_sessions(cls, user_id: str) -> str:
        """g8e:user:{user.id}:web_sessions"""
        return f"{CACHE_PREFIX}:user:{user_id}:web_sessions"

    @classmethod
    def user_memories(cls, user_id: str) -> str:
        """g8e:user:{user.id}:memories"""
        return f"{CACHE_PREFIX}:user:{user_id}:memories"

    @classmethod
    def attachment(cls, investigation_id: str, attachment_id: str) -> str:
        """g8e:investigation:{investigation.id}:attachment:{attachment.id}"""
        return f"{CACHE_PREFIX}:investigation:{investigation_id}:attachment:{attachment_id}"

    @classmethod
    def attachment_index(cls, investigation_id: str) -> str:
        """g8e:investigation:{investigation.id}:attachment.index"""
        return f"{CACHE_PREFIX}:investigation:{investigation_id}:attachment.index"

    @classmethod
    def nonce(cls, nonce: str) -> str:
        """g8e:auth:nonce:{nonce}"""
        return f"{CACHE_PREFIX}:auth:nonce:{nonce}"

    @classmethod
    def download_token(cls, token: str) -> str:
        """g8e:auth:token:download:{token}"""
        return f"{CACHE_PREFIX}:auth:token:download:{token}"

    @classmethod
    def device_link(cls, token: str) -> str:
        """g8e:auth:token:device:{token}"""
        return f"{CACHE_PREFIX}:auth:token:device:{token}"

    @classmethod
    def device_link_uses(cls, token: str) -> str:
        """g8e:auth:token:device:{token}:uses"""
        return f"{CACHE_PREFIX}:auth:token:device:{token}:uses"

    @classmethod
    def device_link_fingerprints(cls, token: str) -> str:
        """g8e:auth:token:device:{token}:fingerprints"""
        return f"{CACHE_PREFIX}:auth:token:device:{token}:fingerprints"

    @classmethod
    def device_link_registration_lock(cls, token: str) -> str:
        """g8e:auth:token:device:{token}:reg.lock"""
        return f"{CACHE_PREFIX}:auth:token:device:{token}:reg.lock"

    @classmethod
    def device_link_list(cls, user_id: str) -> str:
        """g8e:auth:device.list:{user.id}"""
        return f"{CACHE_PREFIX}:auth:device.list:{user_id}"

    @classmethod
    def login_failed(cls, identifier: str) -> str:
        """g8e:auth:login:{identifier}:failed"""
        return f"{CACHE_PREFIX}:auth:login:{identifier}:failed"

    @classmethod
    def login_lock(cls, identifier: str) -> str:
        """g8e:auth:login:{identifier}:lock"""
        return f"{CACHE_PREFIX}:auth:login:{identifier}:lock"

    @classmethod
    def login_ip_accounts(cls, ip: str) -> str:
        """g8e:auth:login:ip:{ip}:accounts"""
        return f"{CACHE_PREFIX}:auth:login:ip:{ip}:accounts"

    @classmethod
    def pending_cmd(cls, execution_id: str) -> str:
        """g8e:execution:{execution.id}:pending.cmd"""
        return f"{CACHE_PREFIX}:execution:{execution_id}:pending.cmd"

class KVKeyPrefix:
    """Canonical KV store key prefixes. All prefixes use the version prefix."""
    CACHE_DOC = f"{CACHE_PREFIX}:cache:doc:"
    CACHE_QUERY = f"{CACHE_PREFIX}:cache:query:"
