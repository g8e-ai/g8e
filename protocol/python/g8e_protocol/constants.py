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


class ErrorCategory(StrEnum):
    NETWORK = "network"
    DATABASE = "database"
    PUBSUB = "pubsub"
    STORAGE = "storage"
    AUTH = "auth"
    VALIDATION = "validation"
    BUSINESS_LOGIC = "business_logic"
    RESOURCE_NOT_FOUND = "resource_not_found"
    PERMISSION = "permission"
    INTERNAL = "internal"
    CONFIGURATION = "configuration"
    DEPENDENCY = "dependency"
    CONFLICT = "conflict"
    RATE_LIMIT = "rate_limit"
    SERVICE_UNAVAILABLE = "service_unavailable"
    EXTERNAL_SERVICE = "external_service"
    TIMEOUT = "timeout"


class ErrorSeverity(StrEnum):
    CRITICAL = "critical"
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"
    INFO = "info"


class ErrorCode(StrEnum):
    GENERIC_ERROR = "G8E-1000"
    UNEXPECTED_ERROR = "G8E-1001"
    NOT_IMPLEMENTED = "G8E-1002"
    CONFIG_ERROR = "G8E-1100"
    MISSING_ENV_VAR = "G8E-1101"
    INVALID_SETTINGS = "G8E-1102"
    SERVICE_INIT_ERROR = "G8E-1103"
    AUTH_ERROR = "G8E-1200"
    TOKEN_EXPIRED = "G8E-1201"
    INVALID_TOKEN = "G8E-1202"
    INSUFFICIENT_PERMISSIONS = "G8E-1203"
    DB_CONNECTION_ERROR = "G8E-1300"
    DB_QUERY_ERROR = "G8E-1301"
    DB_DOCUMENT_NOT_FOUND = "G8E-1302"
    DB_WRITE_ERROR = "G8E-1303"
    DB_TRANSACTION_ERROR = "G8E-1304"
    PUBSUB_CONNECTION_ERROR = "G8E-1400"
    PUBSUB_PUBLISH_ERROR = "G8E-1401"
    PUBSUB_SUBSCRIBE_ERROR = "G8E-1402"
    PUBSUB_TOPIC_ERROR = "G8E-1403"
    STORAGE_CONNECTION_ERROR = "G8E-1500"
    STORAGE_READ_ERROR = "G8E-1501"
    STORAGE_WRITE_ERROR = "G8E-1502"
    STORAGE_DELETE_ERROR = "G8E-1503"
    API_CONNECTION_ERROR = "G8E-1600"
    API_TIMEOUT_ERROR = "G8E-1601"
    API_RESPONSE_ERROR = "G8E-1602"
    API_REQUEST_ERROR = "G8E-1603"
    API_RATE_LIMIT_ERROR = "G8E-1604"
    GENERIC_NOT_FOUND = "G8E-1605"
    EXTERNAL_SERVICE_ERROR = "G8E-1607"
    VALIDATION_ERROR = "G8E-1700"
    MISSING_REQUIRED_FIELD = "G8E-1701"
    INVALID_FIELD_FORMAT = "G8E-1702"
    INVALID_FIELD_VALUE = "G8E-1703"
    INVALID_FIELD_TYPE = "G8E-1704"
    SCHEMA_VALIDATION_FAILED = "G8E-1705"
    SCHEMA_NOT_FOUND = "G8E-1706"
    BUSINESS_LOGIC_ERROR = "G8E-1800"
    WORKFLOW_ERROR = "G8E-1801"
    STATE_TRANSITION_ERROR = "G8E-1802"
    RESOURCE_CONFLICT = "G8E-1803"
    TASK_CREATION_FAILED = "G8E-1804"
    OPERATION_FAILED = "G8E-1805"
    SERVICE_UNAVAILABLE_ERROR = "G8E-1900"


class AgentMode(StrEnum):
    OPERATOR_BOUND = "operator.bound"
    OPERATOR_NOT_BOUND = "operator.not.bound"
    CLOUD_OPERATOR_BOUND = "cloud.operator.bound"


class PromptSection(StrEnum):
    CAPABILITIES = "capabilities"
    EXECUTION = "execution"
    TOOLS = "tools"
    CONSTRAINTS = "constraints"


class PromptFile(StrEnum):
    MODES_OPERATOR_BOUND_CAPABILITIES = "modes/operator.bound/capabilities.md"
    MODES_OPERATOR_BOUND_EXECUTION = "modes/operator.bound/execution.md"
    MODES_OPERATOR_BOUND_TOOLS = "modes/operator.bound/tools.md"
    MODES_OPERATOR_NOT_BOUND_CAPABILITIES = "modes/operator.not.bound/capabilities.md"
    MODES_OPERATOR_NOT_BOUND_EXECUTION = "modes/operator.not.bound/execution.md"
    MODES_OPERATOR_NOT_BOUND_TOOLS = "modes/operator.not.bound/tools.md"
    MODES_CLOUD_OPERATOR_BOUND_CAPABILITIES = "modes/cloud.operator.bound/capabilities.md"
    MODES_CLOUD_OPERATOR_BOUND_EXECUTION = "modes/cloud.operator.bound/execution.md"
    MODES_CLOUD_OPERATOR_BOUND_TOOLS = "modes/cloud.operator.bound/tools.md"
    CORE_BASE_INSTRUCTIONS = "core/base_instructions.md"
    CORE_ERROR_RECOVERY = "core/error_recovery.md"
    CORE_GOVERNANCE_POSTURE = "core/governance_posture.md"
    CORE_OUTPUT_FORMATTING = "core/output_formatting.md"
    CORE_TOOL_USAGE_GUIDELINES = "core/tool_usage_guidelines.md"
    COMPONENTS_AUDITOR_VERDICT = "components/auditor/verdict.md"
    COMPONENTS_AUDITOR_REASONING = "components/auditor/reasoning.md"
    COMPONENTS_TRIAGE_INTENT = "components/triage/intent.md"
    COMPONENTS_TRIAGE_COMPLEXITY = "components/triage/complexity.md"
    COMPONENTS_TRIAGE_POSTURE = "components/triage/posture.md"


class ReasoningAgent(StrEnum):
    SAGE = "sage"
    DASH = "dash"


class TriageComplexityClassification(StrEnum):
    SIMPLE = "simple"
    COMPLEX = "complex"


class TriageConfidence(StrEnum):
    HIGH = "high"
    LOW = "low"


class TriageIntentClassification(StrEnum):
    INFORMATION = "information"
    ACTION = "action"
    UNKNOWN = "unknown"


class TriageRequestPosture(StrEnum):
    NORMAL = "normal"
    ESCALATED = "escalated"
    ADVERSARIAL = "adversarial"
    CONFUSED = "confused"


class TribunalMember(StrEnum):
    AXIOM = "axiom"
    CONCORD = "concord"
    VARIANCE = "variance"
    PRAGMA = "pragma"
    NEMESIS = "nemesis"


class AuditorReason(StrEnum):
    OK = "ok"
    REVISED = "revised"
    EMPTY_RESPONSE = "empty_response"
    NO_VALID_REVISION = "no_valid_revision"
    AUDITOR_ERROR = "auditor_error"
    SWAPPED_TO_DISSENTER = "swapped_to_dissenter"
    REVISED_FROM_DISSENT = "revised_from_dissent"
    WHITELIST_VIOLATION = "whitelist_violation"


class TieBreakReason(StrEnum):
    SHORTEST = "shortest"
    EXCLUDED_NEMESIS = "excluded_nemesis"
    ALPHABETICAL = "alphabetical"

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
