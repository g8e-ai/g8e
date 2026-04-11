// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function loadSharedErrors() {
    const sharedPath = path.resolve(__dirname, '../../../shared/constants/errors.json');
    try {
        const data = fs.readFileSync(sharedPath, 'utf8');
        return JSON.parse(data);
    } catch (error) {
        return {};
    }
}

const shared = loadSharedErrors();

export const ErrorCategory = Object.freeze({
    NETWORK:             shared.ErrorCategory?.NETWORK || 'network',
    DATABASE:            shared.ErrorCategory?.DATABASE || 'database',
    PUBSUB:              shared.ErrorCategory?.PUBSUB || 'pubsub',
    STORAGE:             shared.ErrorCategory?.STORAGE || 'storage',
    AUTH:                shared.ErrorCategory?.AUTH || 'auth',
    VALIDATION:          shared.ErrorCategory?.VALIDATION || 'validation',
    BUSINESS_LOGIC:      shared.ErrorCategory?.BUSINESS_LOGIC || 'business_logic',
    RESOURCE_NOT_FOUND:  shared.ErrorCategory?.RESOURCE_NOT_FOUND || 'resource_not_found',
    PERMISSION:          shared.ErrorCategory?.PERMISSION || 'permission',
    INTERNAL:            shared.ErrorCategory?.INTERNAL || 'internal',
    CONFIGURATION:       shared.ErrorCategory?.CONFIGURATION || 'configuration',
    DEPENDENCY:          shared.ErrorCategory?.DEPENDENCY || 'dependency',
    CONFLICT:            shared.ErrorCategory?.CONFLICT || 'conflict',
    RATE_LIMIT:          shared.ErrorCategory?.RATE_LIMIT || 'rate_limit',
    SERVICE_UNAVAILABLE: shared.ErrorCategory?.SERVICE_UNAVAILABLE || 'service_unavailable',
    EXTERNAL_SERVICE:    shared.ErrorCategory?.EXTERNAL_SERVICE || 'external_service',
    TIMEOUT:             shared.ErrorCategory?.TIMEOUT || 'timeout'
});

export const ErrorSeverity = Object.freeze({
    CRITICAL: shared.ErrorSeverity?.CRITICAL || 'critical',
    HIGH:     shared.ErrorSeverity?.HIGH || 'high',
    MEDIUM:   shared.ErrorSeverity?.MEDIUM || 'medium',
    LOW:      shared.ErrorSeverity?.LOW || 'low',
    INFO:     shared.ErrorSeverity?.INFO || 'info'
});

export const ErrorCode = Object.freeze({
    GENERIC_ERROR:            shared.ErrorCode?.GENERIC_ERROR || 'VSO-1000',
    UNEXPECTED_ERROR:         shared.ErrorCode?.UNEXPECTED_ERROR || 'VSO-1001',
    NOT_IMPLEMENTED:          shared.ErrorCode?.NOT_IMPLEMENTED || 'VSO-1002',
    CONFIG_ERROR:             shared.ErrorCode?.CONFIG_ERROR || 'VSO-1100',
    MISSING_ENV_VAR:          shared.ErrorCode?.MISSING_ENV_VAR || 'VSO-1101',
    INVALID_SETTINGS:         shared.ErrorCode?.INVALID_SETTINGS || 'VSO-1102',
    SERVICE_INIT_ERROR:       shared.ErrorCode?.SERVICE_INIT_ERROR || 'VSO-1103',
    AUTH_ERROR:               shared.ErrorCode?.AUTH_ERROR || 'VSO-1200',
    TOKEN_EXPIRED:            shared.ErrorCode?.TOKEN_EXPIRED || 'VSO-1201',
    INVALID_TOKEN:            shared.ErrorCode?.INVALID_TOKEN || 'VSO-1202',
    INSUFFICIENT_PERMISSIONS: shared.ErrorCode?.INSUFFICIENT_PERMISSIONS || 'VSO-1203',
    DB_CONNECTION_ERROR:      shared.ErrorCode?.DB_CONNECTION_ERROR || 'VSO-1300',
    DB_QUERY_ERROR:           shared.ErrorCode?.DB_QUERY_ERROR || 'VSO-1301',
    DB_DOCUMENT_NOT_FOUND:    shared.ErrorCode?.DB_DOCUMENT_NOT_FOUND || 'VSO-1302',
    DB_WRITE_ERROR:           shared.ErrorCode?.DB_WRITE_ERROR || 'VSO-1303',
    DB_TRANSACTION_ERROR:     shared.ErrorCode?.DB_TRANSACTION_ERROR || 'VSO-1304',
    PUBSUB_CONNECTION_ERROR:  shared.ErrorCode?.PUBSUB_CONNECTION_ERROR || 'VSO-1400',
    PUBSUB_PUBLISH_ERROR:     shared.ErrorCode?.PUBSUB_PUBLISH_ERROR || 'VSO-1401',
    PUBSUB_SUBSCRIBE_ERROR:   shared.ErrorCode?.PUBSUB_SUBSCRIBE_ERROR || 'VSO-1402',
    PUBSUB_TOPIC_ERROR:       shared.ErrorCode?.PUBSUB_TOPIC_ERROR || 'VSO-1403',
    STORAGE_CONNECTION_ERROR: shared.ErrorCode?.STORAGE_CONNECTION_ERROR || 'VSO-1500',
    STORAGE_READ_ERROR:       shared.ErrorCode?.STORAGE_READ_ERROR || 'VSO-1501',
    STORAGE_WRITE_ERROR:      shared.ErrorCode?.STORAGE_WRITE_ERROR || 'VSO-1502',
    STORAGE_DELETE_ERROR:     shared.ErrorCode?.STORAGE_DELETE_ERROR || 'VSO-1503',
    API_CONNECTION_ERROR:     shared.ErrorCode?.API_CONNECTION_ERROR || 'VSO-1600',
    API_TIMEOUT_ERROR:        shared.ErrorCode?.API_TIMEOUT_ERROR || 'VSO-1601',
    API_RESPONSE_ERROR:       shared.ErrorCode?.API_RESPONSE_ERROR || 'VSO-1602',
    API_REQUEST_ERROR:        shared.ErrorCode?.API_REQUEST_ERROR || 'VSO-1603',
    API_RATE_LIMIT_ERROR:     shared.ErrorCode?.API_RATE_LIMIT_ERROR || 'VSO-1604',
    GENERIC_NOT_FOUND:        shared.ErrorCode?.GENERIC_NOT_FOUND || 'VSO-1605',
    EXTERNAL_SERVICE_ERROR:   shared.ErrorCode?.EXTERNAL_SERVICE_ERROR || 'VSO-1607',
    VALIDATION_ERROR:         shared.ErrorCode?.VALIDATION_ERROR || 'VSO-1700',
    MISSING_REQUIRED_FIELD:   shared.ErrorCode?.MISSING_REQUIRED_FIELD || 'VSO-1701',
    INVALID_FIELD_FORMAT:     shared.ErrorCode?.INVALID_FIELD_FORMAT || 'VSO-1702',
    INVALID_FIELD_VALUE:      shared.ErrorCode?.INVALID_FIELD_VALUE || 'VSO-1703',
    INVALID_FIELD_TYPE:       shared.ErrorCode?.INVALID_FIELD_TYPE || 'VSO-1704',
    SCHEMA_VALIDATION_FAILED: shared.ErrorCode?.SCHEMA_VALIDATION_FAILED || 'VSO-1705',
    SCHEMA_NOT_FOUND:         shared.ErrorCode?.SCHEMA_NOT_FOUND || 'VSO-1706',
    BUSINESS_LOGIC_ERROR:     shared.ErrorCode?.BUSINESS_LOGIC_ERROR || 'VSO-1800',
    WORKFLOW_ERROR:           shared.ErrorCode?.WORKFLOW_ERROR || 'VSO-1801',
    STATE_TRANSITION_ERROR:   shared.ErrorCode?.STATE_TRANSITION_ERROR || 'VSO-1802',
    RESOURCE_CONFLICT:        shared.ErrorCode?.RESOURCE_CONFLICT || 'VSO-1803',
    TASK_CREATION_FAILED:     shared.ErrorCode?.TASK_CREATION_FAILED || 'VSO-1804',
    OPERATION_FAILED:         shared.ErrorCode?.OPERATION_FAILED || 'VSO-1805',
    SERVICE_UNAVAILABLE_ERROR: shared.ErrorCode?.SERVICE_UNAVAILABLE_ERROR || 'VSO-1900'
});

export const HttpStatusMessage = Object.freeze({
    INTERNAL_SERVER_ERROR: 'Internal Server Error',
    BAD_REQUEST:           'Bad Request',
    UNAUTHORIZED:          'Unauthorized',
    FORBIDDEN:             'Forbidden',
    NOT_FOUND:             'Not Found',
    PAYLOAD_TOO_LARGE:     'Payload Too Large',
    TOO_MANY_REQUESTS:     'Too Many Requests',
    REQUEST_FAILED:        'Request Failed'
});

