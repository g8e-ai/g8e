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

# Exported constants - all loaded from JSON (single source of truth)
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
ERRORS = _load_protocol_json("errors.json")

from enum import StrEnum

# Helper to get Component names (formerly ComponentName enum)
class ComponentName(StrEnum):
    CLIENT = "client"
    G8EE = "g8ee"
    G8EO = "g8eo"
    OPERATOR = "g8eo" # Alias


# Error enums - generated from protocol/constants/errors.json
def _create_enum_from_json(enum_name: str, json_data: dict[str, Any]) -> StrEnum:
    """Create a StrEnum class from JSON data."""
    enum_dict = json_data.get(enum_name, {})
    return StrEnum(enum_name, enum_dict)


def _flatten_nested_dict(nested: dict[str, Any], prefix: str = "", separator: str = ".") -> dict[str, str]:
    """Flatten a nested dictionary into dot-separated keys."""
    result = {}
    for key, value in nested.items():
        new_key = f"{prefix}{separator}{key}" if prefix else key
        if isinstance(value, dict):
            result.update(_flatten_nested_dict(value, new_key, separator))
        else:
            result[new_key] = value
    return result


ErrorCategory = _create_enum_from_json("ErrorCategory", ERRORS)
ErrorSeverity = _create_enum_from_json("ErrorSeverity", ERRORS)
ErrorCode = _create_enum_from_json("ErrorCode", ERRORS)

# Prompt enums - generated from protocol/constants/prompts.json
# AgentMode maps from the JSON keys
_agent_mode_data = PROMPTS.get("agent.mode", {})
AgentMode = StrEnum("AgentMode", {k.upper().replace(".", "_"): k for k in _agent_mode_data.keys()})

# PromptSection maps from the JSON keys
_prompt_section_data = PROMPTS.get("prompt.section", {})
PromptSection = StrEnum("PromptSection", {k.upper().replace(".", "_"): k for k in _prompt_section_data.keys()})

# PromptFile needs to be flattened from nested structure
_prompt_file_nested = PROMPTS.get("prompt.file", {})
_prompt_file_flat = _flatten_nested_dict(_prompt_file_nested, separator="_")
# Convert keys to uppercase enum names matching g8ee conventions
# Replace dots with underscores in keys to avoid invalid enum names
PromptFile = StrEnum("PromptFile", {k.upper().replace(".", "_"): v for k, v in _prompt_file_flat.items()})

# HTTP headers - loaded from JSON (protocol/constants/headers.json)
# Use HEADERS dict directly or access via these convenience constants
HTTP_ACCEL_BUFFERING_HEADER = HEADERS.get("http.x-accel-buffering", "X-Accel-Buffering")
HTTP_ACCEPT_HEADER = HEADERS.get("http.accept", "Accept")
HTTP_ACCEPT_LANGUAGE_HEADER = HEADERS.get("http.accept-language", "Accept-Language")
HTTP_ACCESS_CONTROL_ALLOW_CREDENTIALS = HEADERS.get("http.access-control-allow-creds", "Access-Control-Allow-Credentials")
HTTP_ACCESS_CONTROL_ALLOW_ORIGIN = HEADERS.get("http.access-control-allow-origin", "Access-Control-Allow-Origin")
HTTP_ACCESS_CONTROL_REQUEST_HEADERS = HEADERS.get("http.access-control-req-headers", "Access-Control-Request-Headers")
HTTP_ACCESS_CONTROL_REQUEST_METHOD = HEADERS.get("http.access-control-req-method", "Access-Control-Request-Method")
HTTP_API_KEY_HEADER = HEADERS.get("http.api-key", "X-API-Key")
HTTP_AUTHORIZATION_HEADER = HEADERS.get("http.authorization", "Authorization")
HTTP_BEARER_PREFIX = "Bearer"
HTTP_CACHE_CONTROL_HEADER = HEADERS.get("http.cache-control", "Cache-Control")
HTTP_CONTENT_LANGUAGE_HEADER = HEADERS.get("http.content-language", "Content-Language")
HTTP_CONTENT_TYPE_HEADER = HEADERS.get("http.content-type", "Content-Type")
HTTP_COOKIE_HEADER = HEADERS.get("http.cookie", "Cookie")
HTTP_FORWARDED_FOR_HEADER = HEADERS.get("http.x-forwarded-for", "X-Forwarded-For")
HTTP_LAST_EVENT_ID_HEADER = HEADERS.get("http.last-event-id", "Last-Event-ID")
HTTP_PRAGMA_HEADER = HEADERS.get("http.pragma", "Pragma")
HTTP_REQUESTED_WITH_HEADER = HEADERS.get("http.requested-with", "X-Requested-With")
HTTP_SET_COOKIE_HEADER = HEADERS.get("http.set-cookie", "Set-Cookie")
HTTP_USER_AGENT_HEADER = HEADERS.get("http.user-agent", "User-Agent")
HTTP_DEVICE_TOKEN_HEADER = HEADERS.get("x-g8e.device-token", "X-G8E-Device-Token")

# Session Headers - derived from headers.json
HTTP_WEB_SESSION_ID_HEADER = HEADERS.get("x-g8e.web-session-id", "X-G8E-Web-Session-ID")
HTTP_CLI_SESSION_ID_HEADER = HEADERS.get("http.x-session-id", "X-G8E-CLI-Session-ID")
HTTP_OPERATOR_SESSION_ID_HEADER = HEADERS.get("x-g8e.operator-session-id", "X-G8E-Operator-Session-ID")
HTTP_OPERATOR_ID_HEADER = HEADERS.get("x-g8e.operator-id", "X-G8E-Operator-ID")
HTTP_OPERATOR_API_KEY_HEADER = HEADERS.get("x-g8e.operator-api-key", "X-G8E-Operator-API-Key")
HTTP_SYSTEM_FINGERPRINT_HEADER = HEADERS.get("x-g8e.system-fingerprint", "X-G8E-System-Fingerprint")

# Context Headers - derived from headers.json
HTTP_PROXY_ORGANIZATION_ID_HEADER = HEADERS.get("http.x-proxy-organization-id", "X-Proxy-Organization-Id")
HTTP_PROXY_USER_EMAIL_HEADER = HEADERS.get("http.x-proxy-user-email", "X-Proxy-User-Email")
HTTP_PROXY_USER_ID_HEADER = HEADERS.get("http.x-proxy-user-id", "X-Proxy-User-Id")
HTTP_CASE_ID_HEADER = HEADERS.get("x-g8e.case-id", "X-G8E-Case-ID")
HTTP_USER_ID_HEADER = HEADERS.get("x-g8e.user-id", "X-G8E-User-ID")
HTTP_ORGANIZATION_ID_HEADER = HEADERS.get("x-g8e.organization-id", "X-G8E-Organization-ID")
HTTP_INVESTIGATION_ID_HEADER = HEADERS.get("x-g8e.investigation-id", "X-G8E-Investigation-ID")
HTTP_TASK_ID_HEADER = HEADERS.get("x-g8e.task-id", "X-G8E-Task-ID")
HTTP_BOUND_OPERATORS_HEADER = HEADERS.get("x-g8e.bound-operators", "X-G8E-Bound-Operators")
HTTP_EXECUTION_ID_HEADER = HEADERS.get("x-g8e.request-id", "X-G8E-Request-ID")
HTTP_COMPONENT_NAME_HEADER = HEADERS.get("x-g8e.source-component", "X-G8E-Source-Component")
