# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
import logging
import os
import re
from pathlib import Path
from typing import Any

# Protocol Constants Loader for Python
# Provides a single entry point for protocol constants shared across components.

logger = logging.getLogger(__name__)


class ProtocolConstantsError(RuntimeError):
    """Raised when a required protocol constant file is missing, empty, or malformed.

    Fail-closed sentinel for :func:`_load_protocol_json` and :func:`_get_protocol_dir`.
    Downstream ``KeyError``\\s from empty constant dicts (e.g. ``STATUS["status"]``)
    are prevented by raising this at import time with the offending path.
    """


def _get_protocol_dir() -> Path:
    """Resolve the protocol constants directory.

    Resolution order (first match wins):

    1. ``G8E_PROTOCOL_DIR`` env var — explicit override. An empty value is
       treated as unset so a stray ``G8E_PROTOCOL_DIR=`` line in ``.env``
       (loaded by ``load_dotenv``) cannot shadow the bundled ``_data/`` bundle.
    2. Bundled ``g8e/_data/`` inside site-packages — the production/container
       path used by ``pip install`` of this package.

    There are no dev-mode fallbacks. The previous source-tree and
    ``/app/protocol/constants`` probes were removed because they could silently
    win in a container with a stale or empty path, producing the E.1
    ``KeyError: 'status'`` crash. Developers running from a source checkout
    must set ``G8E_PROTOCOL_DIR`` explicitly or install the package so the
    bundled ``_data/`` is present.
    """
    # 1. Explicit env var override (empty string treated as unset).
    env_dir = os.environ.get("G8E_PROTOCOL_DIR", "").strip()
    if env_dir:
        return Path(env_dir) / "constants"

    # 2. Bundled _data/ inside the installed package — the only other path.
    pkg_path = Path(__file__).parent / "_data"
    return pkg_path


_PROTOCOL_CONSTANTS_DIR = _get_protocol_dir()


def _load_protocol_json(filename: str) -> dict[str, Any]:
    """Load a required protocol constant JSON file, fail closed on missing/empty.

    Raises :class:`ProtocolConstantsError` if the file is missing, empty, or
    contains empty JSON content (``{}`` or whitespace-only). A missing or empty
    constants file is a broken bundle, not a recoverable state — returning
    ``{}`` would let downstream code raise opaque ``KeyError``\\s far from the
    root cause (the E.1 ``KeyError: 'status'`` crash).
    """
    path = _PROTOCOL_CONSTANTS_DIR / filename
    if not path.exists():
        raise ProtocolConstantsError(
            f"Protocol constant file {filename!r} not found at {path}. "
            f"Set G8E_PROTOCOL_DIR to the protocol directory or reinstall the "
            f"g8e package to restore the bundled _data/ constants."
        )

    try:
        with open(path, encoding="utf-8") as f:
            data = json.load(f)
    except json.JSONDecodeError as exc:
        raise ProtocolConstantsError(
            f"Protocol constant file {filename!r} at {path} is malformed JSON: {exc}"
        ) from exc

    if not data:
        raise ProtocolConstantsError(
            f"Protocol constant file {filename!r} at {path} is empty. "
            f"The constants bundle is broken; reinstall the g8e package."
        )

    return data

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
NETWORK = _load_protocol_json("network.json")
API_PATHS = _load_protocol_json("api_paths.json")

try:
    from enum import StrEnum
except ImportError:
    from enum import Enum
    class StrEnum(str, Enum):
        pass


def collection(name: str) -> str:
    """Get the wire value for a collection by key. e.g. collection("cases") -> "cases" """
    return COLLECTIONS["collections"][name]["value"]


def channel(name: str) -> str:
    """Get the wire value for a channel by key."""
    return CHANNELS["channels"][name]["value"]


def document_id(name: str) -> str:
    """Get the wire value for a document ID by key. e.g. document_id("platform_settings") -> "platform_settings" """
    return DOCUMENT_IDS["document_ids"][name]["value"]


def intent(name: str) -> str:
    """Get the wire value for an intent by key."""
    return INTENTS["intents"][name]["value"]


def prompt(name: str) -> str:
    """Get the wire value for a prompt section by key."""
    return PROMPTS["prompts"][name]["value"]


def kv_key(name: str, **kwargs: str) -> str:
    """Get a formatted KV key. e.g. kv_key("SessionWeb", **{"session.type": "web", "session.id": "abc"}) -> "g8e:sessions:web:abc" """
    template = KV["kv_keys"][name]["value"]
    if not kwargs:
        return template
    return re.sub(r"\{([^}]+)\}", lambda m: kwargs[m.group(1)], template)


def kv_session_type(name: str) -> str:
    """Get the wire value for a session type."""
    return KV["session_types"][name]["value"]

# Component names — mirrors internal/constants/status.go ComponentName and
# protocol/constants/status.json component_name category.
class ComponentName(StrEnum):
    CLIENT = "client"
    G8EO = "g8eo"
    G8EO_GATEWAY = "g8eo-gateway"

# HTTP header constants — mirrors internal/constants/auth.go Header* and
# protocol/constants/headers.json. Values must match the Go SSOT exactly.
HTTP_ACCEL_BUFFERING_HEADER = "X-Accel-Buffering"
HTTP_ACCEPT_HEADER = "Accept"
HTTP_ACCEPT_LANGUAGE_HEADER = "Accept-Language"
HTTP_ACCESS_CONTROL_ALLOW_CREDENTIALS_HEADER = "Access-Control-Allow-Credentials"
HTTP_ACCESS_CONTROL_ALLOW_HEADERS_HEADER = "Access-Control-Allow-Headers"
HTTP_ACCESS_CONTROL_ALLOW_METHODS_HEADER = "Access-Control-Allow-Methods"
HTTP_ACCESS_CONTROL_ALLOW_ORIGIN_HEADER = "Access-Control-Allow-Origin"
HTTP_ACCESS_CONTROL_MAX_AGE_HEADER = "Access-Control-Max-Age"
HTTP_ACCESS_CONTROL_REQUEST_HEADERS_HEADER = "Access-Control-Request-Headers"
HTTP_ACCESS_CONTROL_REQUEST_METHOD_HEADER = "Access-Control-Request-Method"
HTTP_AUTHORIZATION_HEADER = "Authorization"
HTTP_BEARER_PREFIX = "Bearer "
HTTP_CACHE_CONTROL_HEADER = "Cache-Control"
HTTP_CONNECTION_HEADER = "Connection"
HTTP_CONTENT_DISPOSITION_HEADER = "Content-Disposition"
HTTP_CONTENT_LANGUAGE_HEADER = "Content-Language"
HTTP_CONTENT_LENGTH_HEADER = "Content-Length"
HTTP_CONTENT_SECURITY_POLICY_HEADER = "Content-Security-Policy"
HTTP_CONTENT_TYPE_HEADER = "Content-Type"
HTTP_COOKIE_HEADER = "Cookie"
HTTP_LAST_EVENT_ID_HEADER = "Last-Event-ID"
HTTP_PRAGMA_HEADER = "Pragma"
HTTP_REQUESTED_WITH_HEADER = "X-Requested-With"
HTTP_SET_COOKIE_HEADER = "Set-Cookie"
HTTP_USER_AGENT_HEADER = "User-Agent"
HTTP_VARY_HEADER = "Vary"
HTTP_X_CONTENT_TYPE_OPTIONS_HEADER = "X-Content-Type-Options"
HTTP_X_FORWARDED_FOR_HEADER = "X-Forwarded-For"
HTTP_X_FORWARDED_HOST_HEADER = "X-Forwarded-Host"
HTTP_X_FORWARDED_PROTO_HEADER = "X-Forwarded-Proto"
HTTP_X_FRAME_OPTIONS_HEADER = "X-Frame-Options"
HTTP_X_REQUEST_TIMESTAMP_HEADER = "X-Request-Timestamp"

# g8e-specific headers
HTTP_G8E_SYSTEM_FINGERPRINT_HEADER = "X-G8E-System-Fingerprint"

# Session headers
WEB_SESSION_ID_HEADER = "X-G8E-Web-Session-ID"
CLI_SESSION_ID_HEADER = "X-G8E-CLI-Session-ID"
OPERATOR_ID_HEADER = "X-G8E-Operator-ID"
OPERATOR_SESSION_ID_HEADER = "X-G8E-Operator-Session-ID"

# Context headers
PROXY_ORGANIZATION_ID_HEADER = "X-Proxy-Organization-Id"
PROXY_USER_ID_HEADER = "X-Proxy-User-Id"
CASE_ID_HEADER = "X-G8E-Case-ID"
USER_ID_HEADER = "X-G8E-User-ID"
ORGANIZATION_ID_HEADER = "X-G8E-Organization-ID"
INVESTIGATION_ID_HEADER = "X-G8E-Investigation-ID"
TASK_ID_HEADER = "X-G8E-Task-ID"
BOUND_OPERATORS_HEADER = "X-G8E-Bound-Operators"
EXECUTION_ID_HEADER = "X-G8E-Execution-ID"
REQUEST_ID_HEADER = "X-G8E-Request-ID"
COMPONENT_NAME_HEADER = "X-G8E-Source-Component"
