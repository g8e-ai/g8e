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

from enum import Enum

class ErrorCategory(str, Enum):
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

class ErrorSeverity(str, Enum):
    CRITICAL = "critical"
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"
    INFO = "info"

class CommandCategory(str, Enum):
    CSV_WHITELIST = "csv_whitelist"
    NETWORK_DIAGNOSTICS = "network_diagnostics"
    SYSTEM_DIAGNOSTICS = "system_diagnostics"

class ErrorCode(str, Enum):
    # Base Errors
    GENERIC_ERROR = "G8E-1000"
    UNEXPECTED_ERROR = "G8E-1001"
    NOT_IMPLEMENTED = "G8E-1002"

    # Configuration Errors
    CONFIG_ERROR = "G8E-1100"
    MISSING_ENV_VAR = "G8E-1101"
    INVALID_SETTINGS = "G8E-1102"
    SERVICE_INIT_ERROR = "G8E-1103"

    # Auth & Permissions
    AUTH_ERROR = "G8E-1200"
    TOKEN_EXPIRED = "G8E-1201"
    INVALID_TOKEN = "G8E-1202"
    INSUFFICIENT_PERMISSIONS = "G8E-1203"

    # Database Errors
    DB_CONNECTION_ERROR = "G8E-1300"
    DB_QUERY_ERROR = "G8E-1301"
    DB_DOCUMENT_NOT_FOUND = "G8E-1302"
    DB_WRITE_ERROR = "G8E-1303"
    DB_TRANSACTION_ERROR = "G8E-1304"

    # PubSub Errors
    PUBSUB_CONNECTION_ERROR = "G8E-1400"
    PUBSUB_PUBLISH_ERROR = "G8E-1401"
    PUBSUB_SUBSCRIBE_ERROR = "G8E-1402"
    PUBSUB_TOPIC_ERROR = "G8E-1403"

    # Storage Errors
    STORAGE_CONNECTION_ERROR = "G8E-1500"
    STORAGE_READ_ERROR = "G8E-1501"
    STORAGE_WRITE_ERROR = "G8E-1502"
    STORAGE_DELETE_ERROR = "G8E-1503"

    # Network & API Errors
    API_CONNECTION_ERROR = "G8E-1600"
    API_TIMEOUT_ERROR = "G8E-1601"
    API_RESPONSE_ERROR = "G8E-1602"
    API_REQUEST_ERROR = "G8E-1603"
    API_RATE_LIMIT_ERROR = "G8E-1604"
    GENERIC_NOT_FOUND = "G8E-1605"
    EXTERNAL_SERVICE_ERROR = "G8E-1607"

    # Validation Errors
    VALIDATION_ERROR = "G8E-1700"
    MISSING_REQUIRED_FIELD = "G8E-1701"
    INVALID_FIELD_FORMAT = "G8E-1702"
    INVALID_FIELD_VALUE = "G8E-1703"
    INVALID_FIELD_TYPE = "G8E-1704"
    SCHEMA_VALIDATION_FAILED = "G8E-1705"
    SCHEMA_NOT_FOUND = "G8E-1706"

    # Business Logic Errors
    BUSINESS_LOGIC_ERROR = "G8E-1800"
    WORKFLOW_ERROR = "G8E-1801"
    STATE_TRANSITION_ERROR = "G8E-1802"
    RESOURCE_CONFLICT = "G8E-1803"
    TASK_CREATION_FAILED = "G8E-1804"
    OPERATION_FAILED = "G8E-1805"

    # Infrastructure Errors
    SERVICE_UNAVAILABLE_ERROR = "G8E-1900"
