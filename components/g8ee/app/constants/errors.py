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

# Use absolute path for shared models in container
_SHARED_DIR = "/app/shared/models"

# Note: We are using the shared/models/errors.py directly but we might want to 
# eventually move these to a more central constants location if they are purely enums.
# For now, we mirror the structure to break the circular dependency.

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

class ErrorCode(str, Enum):
    # Base Errors
    GENERIC_ERROR = "VSO-1000"
    UNEXPECTED_ERROR = "VSO-1001"
    NOT_IMPLEMENTED = "VSO-1002"

    # Configuration Errors
    CONFIG_ERROR = "VSO-1100"
    MISSING_ENV_VAR = "VSO-1101"
    INVALID_SETTINGS = "VSO-1102"
    SERVICE_INIT_ERROR = "VSO-1103"

    # Auth & Permissions
    AUTH_ERROR = "VSO-1200"
    TOKEN_EXPIRED = "VSO-1201"
    INVALID_TOKEN = "VSO-1202"
    INSUFFICIENT_PERMISSIONS = "VSO-1203"

    # Database Errors
    DB_CONNECTION_ERROR = "VSO-1300"
    DB_QUERY_ERROR = "VSO-1301"
    DB_DOCUMENT_NOT_FOUND = "VSO-1302"
    DB_WRITE_ERROR = "VSO-1303"
    DB_TRANSACTION_ERROR = "VSO-1304"

    # PubSub Errors
    PUBSUB_CONNECTION_ERROR = "VSO-1400"
    PUBSUB_PUBLISH_ERROR = "VSO-1401"
    PUBSUB_SUBSCRIBE_ERROR = "VSO-1402"
    PUBSUB_TOPIC_ERROR = "VSO-1403"

    # Storage Errors
    STORAGE_CONNECTION_ERROR = "VSO-1500"
    STORAGE_READ_ERROR = "VSO-1501"
    STORAGE_WRITE_ERROR = "VSO-1502"
    STORAGE_DELETE_ERROR = "VSO-1503"

    # Network & API Errors
    API_CONNECTION_ERROR = "VSO-1600"
    API_TIMEOUT_ERROR = "VSO-1601"
    API_RESPONSE_ERROR = "VSO-1602"
    API_REQUEST_ERROR = "VSO-1603"
    API_RATE_LIMIT_ERROR = "VSO-1604"
    GENERIC_NOT_FOUND = "VSO-1605"
    EXTERNAL_SERVICE_ERROR = "VSO-1607"

    # Validation Errors
    VALIDATION_ERROR = "VSO-1700"
    MISSING_REQUIRED_FIELD = "VSO-1701"
    INVALID_FIELD_FORMAT = "VSO-1702"
    INVALID_FIELD_VALUE = "VSO-1703"
    INVALID_FIELD_TYPE = "VSO-1704"
    SCHEMA_VALIDATION_FAILED = "VSO-1705"
    SCHEMA_NOT_FOUND = "VSO-1706"

    # Business Logic Errors
    BUSINESS_LOGIC_ERROR = "VSO-1800"
    WORKFLOW_ERROR = "VSO-1801"
    WORKFLOW_FAILED = "VSO-1801" # Alias for backward compatibility if needed
    STATE_TRANSITION_ERROR = "VSO-1802"
    RESOURCE_CONFLICT = "VSO-1803"
    TASK_CREATION_FAILED = "VSO-1804"
    OPERATION_FAILED = "VSO-1805"

    # Infrastructure Errors
    SERVICE_UNAVAILABLE_ERROR = "VSO-1900"
