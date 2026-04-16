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
from enum import Enum
from app.constants.paths import PATHS

_SHARED_DIR = PATHS["infra"]["shared_constants_dir"]

def _load(filename: str) -> dict:
    path = _SHARED_DIR + "/" + filename
    try:
        with open(path) as f:
            return json.load(f)
    except FileNotFoundError as e:
        raise RuntimeError(f"Shared constants file not found: {path}") from e
    except json.JSONDecodeError as e:
        raise RuntimeError(f"Invalid JSON in shared constants file {path}: {e}") from e

_STATUS = _load("status.json")

class OperatorStatus(str, Enum):
    AVAILABLE = "available"
    UNAVAILABLE = "unavailable"
    OFFLINE = "offline"
    BOUND = "bound"
    STALE = "stale"
    ACTIVE = "active"
    STOPPED = "stopped"
    TERMINATED = "terminated"

class ExecutionStatus(str, Enum):
    PENDING = "pending"
    EXECUTING = "executing"
    COMPLETED = "completed"
    FAILED = "failed"
    TIMEOUT = "timeout"
    CANCELLED = "cancelled"
    CANCEL_REQUESTED = "cancel_requested"
    DENIED = "denied"
    FEEDBACK = "feedback"

class ComponentName(str, Enum):
    G8EE = "g8ee"
    G8EO = "g8eo"
    G8ED = "g8ed"

class ComponentStatus(str, Enum):
    ACTIVE = "active"
    INACTIVE = "inactive"
    MAINTENANCE = "maintenance"
    ERROR = "error"

class AITaskId(str, Enum):
    COMMAND = "ai.command"
    DIRECT_COMMAND = "ai.direct.command"
    FILE_EDIT = "ai.file.edit"
    FS_LIST = "ai.fs.list"
    FS_READ = "ai.fs.read"
    PORT_CHECK = "ai.port.check"
    FETCH_LOGS = "ai.fetch.logs"
    FETCH_HISTORY = "ai.fetch.history"
    FETCH_FILE_HISTORY = "ai.fetch.file.history"
    RESTORE_FILE = "ai.restore.file"
    FETCH_FILE_DIFF = "ai.fetch.file.diff"
    INTENT_GRANT = "ai.intent.grant"
    INTENT_REVOKE = "ai.intent.revoke"

class ApiKeyStatus(str, Enum):
    ACTIVE = "active"
    REVOKED = "revoked"
    EXPIRED = "expired"
    SUSPENDED = "suspended"

class ApprovalType(str, Enum):
    COMMAND = "command"
    FILE_EDIT = "file.edit"
    INTENT = "intent"

class CaseStatus(str, Enum):
    NEW = "New"
    TRIAGE = "Triage"
    ESCALATED = "Escalated"
    WAITING_FOR_CUSTOMER = "WaitingForCustomer"
    INVESTIGATE = "Investigate"
    HUMAN_REVIEW = "HumanReview"
    RESOLVED = "Resolved"
    CLOSED = "Closed"

class CloudSubtype(str, Enum):
    AWS = _STATUS["cloud.subtype"]["aws"]
    AZURE = _STATUS["cloud.subtype"]["azure"]
    GCP = _STATUS["cloud.subtype"]["gcp"]
    G8E_POD = _STATUS["cloud.subtype"]["g8ep"]

class CommandErrorType(str, Enum):
    VALIDATION_ERROR = "validation.error"
    SECURITY_ERROR = "security.error"
    SECURITY_VIOLATION = "security.violation"
    BINDING_VIOLATION = "binding.violation"
    NO_OPERATORS_AVAILABLE = "no.operators.available"
    OPERATOR_RESOLUTION_ERROR = "g8e.resolution.error"
    CLOUD_OPERATOR_REQUIRED = "cloud.operator.required"
    BLACKLIST_VIOLATION = "blacklist.violation"
    WHITELIST_VIOLATION = "whitelist.violation"
    EXECUTION_FAILED = "execution.failed"
    EXECUTION_ERROR = "execution.error"
    USER_DENIED = "user.denied"
    USER_FEEDBACK = "user.feedback"
    PERMISSION_DENIED = "permission.denied"
    COMMAND_TIMEOUT = "command.timeout"
    COMMAND_EXECUTION_FAILED = "command.execution.failed"
    PUBSUB_SUBSCRIPTION_NOT_READY = "pubsub.subscription.not.ready"
    UNKNOWN_TOOL = "unknown.tool"
    FS_LIST_FAILED = "fs.list.failed"
    FS_READ_FAILED = "fs.read.failed"
    USER_CANCELLED = "user.cancelled"
    RISK_ANALYSIS_BLOCKED = "risk.analysis.blocked"
    APPROVAL_DENIED = "approval.denied"
    OPERATION_TIMEOUT = "operation.timeout"
    INVALID_INTENT = "invalid.intent"
    MISSING_OPERATOR_ID = "missing.operator.id"
    PARTIAL_IAM_UPDATE_FAILED = "partial.iam.update.failed"
    PARTIAL_IAM_DETACH_FAILED = "partial.iam.detach.failed"
    RESTORE_FILE_FAILED = "restore.file.failed"
    FETCH_FILE_DIFF_FAILED = "fetch.file.diff.failed"
    FETCH_LOGS_FAILED = "fetch.logs.failed"
    FETCH_HISTORY_FAILED = "fetch.history.failed"
    FETCH_FILE_HISTORY_FAILED = "fetch.file.history.failed"
    PORT_CHECK_FAILED = "port.check.failed"
    APPROVAL_TIMEOUT = "approval.timeout"
    PERMISSION_ERROR = "permission.error"

class ConversationStatus(str, Enum):
    ACTIVE = "active"
    INACTIVE = "inactive"
    COMPLETED = "completed"

class EscalationRisk(str, Enum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"

class FileOperation(str, Enum):
    READ = "read"
    CREATE = "create"
    WRITE = "write"
    UPDATE = "update"
    REPLACE = "replace"
    INSERT = "insert"
    DELETE = "delete"
    PATCH = "patch"

class HealthStatus(str, Enum):
    HEALTHY = "healthy"
    UNHEALTHY = "unhealthy"

class HeartbeatType(str, Enum):
    AUTOMATIC = "automatic"
    BOOTSTRAP = "bootstrap"
    REQUESTED = "requested"

class InfrastructureStatus(str, Enum):
    UNKNOWN = "unknown"
    HEALTHY = "healthy"
    STABLE = "stable"
    DEGRADED = "degraded"
    CRITICAL = "critical"

class InvestigationStatus(str, Enum):
    OPEN = "Open"
    CLOSED = "Closed"
    ESCALATED = "Escalated"
    RESOLVED = "Resolved"

class NetworkProtocol(str, Enum):
    TCP = "tcp"
    UDP = "udp"

class OperatorToolName(str, Enum):
    RUN_COMMANDS = "run_commands_with_operator"
    FILE_CREATE = "file_create_on_operator"
    FILE_WRITE = "file_write_on_operator"
    FILE_READ = "file_read_on_operator"
    FILE_UPDATE = "file_update_on_operator"
    CHECK_PORT = "check_port_status"
    LIST_FILES = "list_files_and_directories_with_detailed_metadata"
    READ_FILE_CONTENT = "read_file_content"
    GRANT_INTENT = "grant_intent_permission"
    REVOKE_INTENT = "revoke_intent_permission"
    FETCH_EXECUTION_OUTPUT = "fetch_execution_output"
    FETCH_SESSION_HISTORY = "fetch_session_history"
    FETCH_FILE_HISTORY = "fetch_file_history"
    RESTORE_FILE = "restore_file"
    FETCH_FILE_DIFF = "fetch_file_diff"
    G8E_SEARCH_WEB = "g8e_web_search"
    QUERY_INVESTIGATION_CONTEXT = "query_investigation_context"
    GET_COMMAND_CONSTRAINTS = "get_command_constraints"

OPERATOR_TOOLS = frozenset({
    OperatorToolName.RUN_COMMANDS.value,
    OperatorToolName.FILE_CREATE.value,
    OperatorToolName.FILE_WRITE.value,
    OperatorToolName.FILE_READ.value,
    OperatorToolName.FILE_UPDATE.value,
    OperatorToolName.CHECK_PORT.value,
    OperatorToolName.LIST_FILES.value,
    OperatorToolName.READ_FILE_CONTENT.value,
    OperatorToolName.GRANT_INTENT.value,
    OperatorToolName.REVOKE_INTENT.value,
    OperatorToolName.FETCH_EXECUTION_OUTPUT.value,
    OperatorToolName.FETCH_SESSION_HISTORY.value,
    OperatorToolName.FETCH_FILE_HISTORY.value,
    OperatorToolName.RESTORE_FILE.value,
    OperatorToolName.FETCH_FILE_DIFF.value,
})

class OperatorType(str, Enum):
    SYSTEM = _STATUS["g8e.type"]["system"]
    CLOUD = _STATUS["g8e.type"]["cloud"]

class Platform(str, Enum):
    LINUX = "linux"
    WINDOWS = "windows"
    DARWIN = "darwin"

class Priority(int, Enum):
    LOW = 1
    MEDIUM = 2
    HIGH = 3
    CRITICAL = 4

class RiskLevel(str, Enum):
    LOW = "LOW"
    MEDIUM = "MEDIUM"
    HIGH = "HIGH"

class RiskThreshold(str, Enum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"

class SessionType(str, Enum):
    WEB = "web"
    OPERATOR = "operator"

class Severity(int, Enum):
    LOW = 1
    MEDIUM = 2
    HIGH = 3
    CRITICAL = 4

class TaskStatus(str, Enum):
    PENDING = "pending"
    IN_PROGRESS = "executing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"

class VaultMode(str, Enum):
    RAW = "raw"
    SCRUBBED = "scrubbed"

class VersionStability(str, Enum):
    STABLE = "stable"
    BETA = "beta"
    DEV = "dev"

