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
import logging
import os
from pathlib import Path
from typing import Any

# Protocol Constants Loader for Python
# Provides a single entry point for protocol constants shared across components.

logger = logging.getLogger(__name__)

def _get_protocol_dir() -> Path:
    """Find the protocol directory."""
    # 1. Check environment variable
    if "G8E_PROTOCOL_DIR" in os.environ:
        return Path(os.environ["G8E_PROTOCOL_DIR"]) / "constants"
    
    # 2. Check relative to this file
    # protocol/python/g8e_protocol/constants.py -> protocol/constants
    rel_path = Path(__file__).parent.parent.parent / "constants"
    if rel_path.exists():
        return rel_path
    
    # 3. Fallback for containerized environments
    container_path = Path("/app/protocol/constants")
    if container_path.exists():
        return container_path
    
    return Path("./protocol/constants")

_PROTOCOL_CONSTANTS_DIR = _get_protocol_dir()

def _load_protocol_json(filename: str) -> dict[str, Any]:
    path = _PROTOCOL_CONSTANTS_DIR / filename
    if not path.exists():
        logger.warning("Protocol JSON %s not found at %s", filename, path)
        return {}

    with open(path) as f:
        return json.load(f)

# Exported constants
EVENTS = _load_protocol_json("events.json")
STATUS = _load_protocol_json("status.json")
MSG = _load_protocol_json("senders.json")
COLLECTIONS = _load_protocol_json("collections.json")
KV = _load_protocol_json("kv_keys.json")
CHANNELS = _load_protocol_json("channels.json")
PUBSUB = _load_protocol_json("pubsub.json")
INTENTS = _load_protocol_json("intents.json")
PROMPTS = _load_protocol_json("prompts.json")
TIMESTAMP = _load_protocol_json("timestamp.json")
HEADERS = _load_protocol_json("headers.json")
DOCUMENT_IDS = _load_protocol_json("document_ids.json")
PLATFORM = _load_protocol_json("platform.json")
AGENTS = _load_protocol_json("agents.json")

from enum import StrEnum

# Helper to get Component names (formerly ComponentName enum)
class ComponentName(StrEnum):
    CLIENT = "client"
    G8EE = "g8ee"
    G8EO = "g8eo"
    OPERATOR = "g8eo" # Alias

# Headers from g8ee/app/constants/headers.py
HTTP_ACCEL_BUFFERING_HEADER = "X-Accel-Buffering"
HTTP_ACCEPT_HEADER = "Accept"
HTTP_ACCEPT_LANGUAGE_HEADER = "Accept-Language"
HTTP_ACCESS_CONTROL_ALLOW_CREDENTIALS = "Access-Control-Allow-Credentials"
HTTP_ACCESS_CONTROL_ALLOW_ORIGIN = "Access-Control-Allow-Origin"
HTTP_ACCESS_CONTROL_REQUEST_HEADERS = "Access-Control-Request-Headers"
HTTP_ACCESS_CONTROL_REQUEST_METHOD = "Access-Control-Request-Method"
HTTP_API_KEY_HEADER = "X-API-Key"
HTTP_AUTHORIZATION_HEADER = "Authorization"
HTTP_BEARER_PREFIX = "Bearer"
HTTP_CACHE_CONTROL_HEADER = "Cache-Control"
HTTP_CONTENT_LANGUAGE_HEADER = "Content-Language"
HTTP_CONTENT_TYPE_HEADER = "Content-Type"
HTTP_COOKIE_HEADER = "Cookie"
HTTP_FORWARDED_FOR_HEADER = "X-Forwarded-For"
HTTP_LAST_EVENT_ID_HEADER = "Last-Event-ID"
HTTP_PRAGMA_HEADER = "Pragma"
HTTP_REQUESTED_WITH_HEADER = "X-Requested-With"
HTTP_SET_COOKIE_HEADER = "Set-Cookie"
HTTP_USER_AGENT_HEADER = "User-Agent"
HTTP_G8E_CLIENT_HEADER = "X-G8E-Client"
HTTP_G8E_OPERATOR_STATUS_HEADER = "X-G8E-Operator-Status"
HTTP_G8E_SYSTEM_FINGERPRINT_HEADER = "X-G8E-System-Fingerprint"
HTTP_G8E_SERVICE_HEADER = "X-G8E-Service"

# Session Headers
WEB_SESSION_ID_HEADER = "X-G8E-Web-Session-ID"
CLI_SESSION_ID_HEADER = "X-G8E-CLI-Session-ID"
OPERATOR_ID_HEADER = "X-G8E-Operator-ID"
OPERATOR_API_KEY_HEADER = "X-G8E-Operator-API-Key"

# Context Headers
PROXY_ORGANIZATION_ID_HEADER = "X-Proxy-Organization-Id"
PROXY_USER_EMAIL_HEADER = "X-Proxy-User-Email"
PROXY_USER_ID_HEADER = "X-Proxy-User-Id"
CASE_ID_HEADER = "X-G8E-Case-ID"
USER_ID_HEADER = "X-G8E-User-ID"
ORGANIZATION_ID_HEADER = "X-G8E-Organization-ID"
INVESTIGATION_ID_HEADER = "X-G8E-Investigation-ID"
TASK_ID_HEADER = "X-G8E-Task-ID"
BOUND_OPERATORS_HEADER = "X-G8E-Bound-Operators"
EXECUTION_ID_HEADER = "X-G8E-Request-ID"
COMPONENT_NAME_HEADER = "X-G8E-Source-Component"
