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
# Flatten status structure for easier access
if "status" in STATUS:
    STATUS = STATUS["status"]
MSG = _load_protocol_json("senders.json")
COLLECTIONS = _load_protocol_json("collections.json")
KV = _load_protocol_json("kv_keys.json")
CHANNELS = _load_protocol_json("channels.json")
PUBSUB = _load_protocol_json("pubsub.json")
INTENTS = _load_protocol_json("intents.json")
PROMPTS = _load_protocol_json("prompts.json")
TIMESTAMP = _load_protocol_json("timestamp.json")
API_PATHS = _load_protocol_json("api_paths.json")
_raw_headers = _load_protocol_json("headers.json")
HEADERS = {k: v["value"] if isinstance(v, dict) and "value" in v else v for k, v in _raw_headers.items()}
DOCUMENT_IDS = _load_protocol_json("document_ids.json")
PLATFORM = _load_protocol_json("platform.json")
AGENTS = _load_protocol_json("agents.json")
# Error constants are hardcoded below, not loaded from JSON

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
    IDENTITY = "identity"
    SAFETY = "safety"
    LOYALTY = "loyalty"
    DISSENT = "dissent"
    CAPABILITIES = "capabilities"
    EXECUTION = "execution"
    TOOLS = "tools"
    CONSTRAINTS = "constraints"
    DOCS = "docs"
    SYSTEM_CONTEXT = "system_context"
    SENTINEL_MODE = "sentinel_mode"
    TRIAGE_CONTEXT = "triage_context"
    INVESTIGATION_CONTEXT = "investigation_context"
    RESPONSE_CONSTRAINTS = "response_constraints"
    LEARNED_CONTEXT = "learned_context"
    AGENT_PERSONA = "agent_persona"


class PromptFile(StrEnum):
    MODES_OPERATOR_BOUND_CAPABILITIES = "modes/operator_bound/capabilities.txt"
    MODES_OPERATOR_BOUND_EXECUTION = "modes/operator_bound/execution.txt"
    MODES_OPERATOR_BOUND_TOOLS = "modes/operator_bound/tools.txt"
    MODES_OPERATOR_NOT_BOUND_CAPABILITIES = "modes/operator_not_bound/capabilities.txt"
    MODES_OPERATOR_NOT_BOUND_EXECUTION = "modes/operator_not_bound/execution.txt"
    MODES_OPERATOR_NOT_BOUND_TOOLS = "modes/operator_not_bound/tools.txt"
    MODES_OPERATOR_NOT_BOUND_CAPABILITIES_NO_SEARCH = "modes/operator_not_bound/capabilities_no_search.txt"
    MODES_OPERATOR_NOT_BOUND_EXECUTION_NO_SEARCH = "modes/operator_not_bound/execution_no_search.txt"
    MODES_CLOUD_OPERATOR_BOUND_CAPABILITIES = "modes/cloud_operator_bound/capabilities.txt"
    MODES_CLOUD_OPERATOR_BOUND_EXECUTION = "modes/cloud_operator_bound/execution.txt"
    MODES_CLOUD_OPERATOR_BOUND_TOOLS = "modes/cloud_operator_bound/tools.txt"
    CORE_IDENTITY = "core/identity.txt"
    CORE_SAFETY = "core/safety.txt"
    CORE_LOYALTY = "core/loyalty.txt"
    CORE_DISSENT = "core/dissent.txt"
    SYSTEM_RESPONSE_CONSTRAINTS = "system/response_constraints.txt"
    SYSTEM_SENTINEL_MODE = "system/sentinel_mode.txt"
    TRIBUNAL_GENERATOR = "tribunal/generator.txt"
    TRIBUNAL_GENERATOR_ROUND_2 = "tribunal/generator_round_2.txt"
    TRIBUNAL_ROUND_2_AXIOM = "tribunal/round_2/axiom.txt"
    TRIBUNAL_ROUND_2_CONCORD = "tribunal/round_2/concord.txt"
    TRIBUNAL_ROUND_2_VARIANCE = "tribunal/round_2/variance.txt"
    TRIBUNAL_ROUND_2_PRAGMA = "tribunal/round_2/pragma.txt"
    TRIBUNAL_ROUND_2_NEMESIS = "tribunal/round_2/nemesis.txt"
    TRIBUNAL_AUDITOR = "tribunal/auditor.txt"
    TOOLS_RUN_COMMANDS = "tools/run_commands_with_operator.txt"
    TOOLS_FILE_CREATE = "tools/file_create_on_operator.txt"
    TOOLS_FILE_WRITE = "tools/file_write_on_operator.txt"
    TOOLS_FILE_READ = "tools/file_read_on_operator.txt"
    TOOLS_FILE_UPDATE = "tools/file_update_on_operator.txt"
    TOOLS_G8E_WEB_SEARCH = "tools/g8e_web_search.txt"
    TOOLS_CHECK_PORT = "tools/check_port_status.txt"
    TOOLS_LIST_FILES = "tools/list_files_and_directories_with_detailed_metadata.txt"
    TOOLS_RECURSIVE_GREP = "tools/recursive_grep_search.txt"
    TOOLS_GRANT_INTENT = "tools/grant_intent_permission.txt"
    TOOLS_REVOKE_INTENT = "tools/revoke_intent_permission.txt"
    TOOLS_FETCH_FILE_HISTORY = "tools/fetch_file_history.txt"
    TOOLS_FETCH_FILE_DIFF = "tools/fetch_file_diff.txt"
    TOOLS_QUERY_INVESTIGATION_CONTEXT = "tools/query_investigation_context.txt"
    TOOLS_GET_COMMAND_CONSTRAINTS = "tools/get_command_constraints.txt"
    TOOLS_SSH_INVENTORY = "tools/list_ssh_inventory.txt"
    TOOLS_STREAM_OPERATOR = "tools/stream_operator_to_ssh_fleet.txt"


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

# HTTP headers are available via g8e_protocol.headers (generated from headers.json)
# Import from g8e_protocol.headers for canonical header constants
