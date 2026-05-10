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

import { ErrorCode, ErrorCategory, ErrorSeverity } from '../constants/errors.js';

/**
 * Base g8e Error class for g8ed
 */
export class G8eError extends Error {
    constructor(message, options = {}) {
        super(message);
        this.name = this.constructor.name;
        
        this.code = options.code || ErrorCode.GENERIC_ERROR;
        this.category = options.category || ErrorCategory.INTERNAL;
        this.severity = options.severity || ErrorSeverity.MEDIUM;
        this.source = options.source || 'g8ed';
        this.component = options.component || 'g8ed';
        this.details = options.details || {};
        this.traceId = options.traceId || null;
        this.requestId = options.requestId || null;
        this.retrySuggested = options.retrySuggested || false;
        this.remediationSteps = options.remediationSteps || [];
        this.timestamp = new Date().toISOString();
        
        if (options.cause) {
            this.cause = options.cause;
            this.causeMessage = options.cause.message;
            this.causeStackTrace = options.cause.stack ? options.cause.stack.split('\n') : [];
        }

        Error.captureStackTrace(this, this.constructor);
    }

    /**
     * Converts error to standardized wire format
     */
    toG8eErrorResponse() {
        return {
            error: {
                code: this.code,
                message: this.message,
                category: this.category,
                severity: this.severity,
                timestamp: this.timestamp,
                component: this.component,
                source: this.source,
                details: this.details,
                trace_id: this.traceId,
                execution_id: this.requestId,
                retry_suggested: this.retrySuggested,
                remediation_steps: this.remediationSteps,
                cause: this.cause ? {
                    cause_message: this.causeMessage,
                    cause_stack_trace: this.causeStackTrace
                } : null
            },
            trace_id: this.traceId,
            execution_id: this.requestId
        };
    }

    getHttpStatus() {
        const categoryToStatus = {
            [ErrorCategory.VALIDATION]: 400,
            [ErrorCategory.BUSINESS_LOGIC]: 400,
            [ErrorCategory.AUTH]: 401,
            [ErrorCategory.PERMISSION]: 403,
            [ErrorCategory.RESOURCE_NOT_FOUND]: 404,
            [ErrorCategory.CONFLICT]: 409,
            [ErrorCategory.RATE_LIMIT]: 429,
            [ErrorCategory.SERVICE_UNAVAILABLE]: 503,
            [ErrorCategory.EXTERNAL_SERVICE]: 502,
            [ErrorCategory.TIMEOUT]: 504,
        };
        return categoryToStatus[this.category] || 500;
    }
}

export class ValidationError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.VALIDATION_ERROR,
            category: ErrorCategory.VALIDATION,
            ...options
        });
    }
}

export class AuthenticationError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.AUTH_ERROR,
            category: ErrorCategory.AUTH,
            ...options
        });
    }
}

export class AuthorizationError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.INSUFFICIENT_PERMISSIONS,
            category: ErrorCategory.PERMISSION,
            ...options
        });
    }
}

export class ResourceNotFoundError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.GENERIC_NOT_FOUND,
            category: ErrorCategory.RESOURCE_NOT_FOUND,
            ...options
        });
    }
}

export class BusinessLogicError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.BUSINESS_LOGIC_ERROR,
            category: ErrorCategory.BUSINESS_LOGIC,
            ...options
        });
    }
}

export class DatabaseError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.DB_CONNECTION_ERROR,
            category: ErrorCategory.DATABASE,
            ...options
        });
    }
}

export class NetworkError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.API_CONNECTION_ERROR,
            category: ErrorCategory.NETWORK,
            severity: ErrorSeverity.MEDIUM,
            ...options
        });
    }
}

export class RateLimitError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.API_RATE_LIMIT_ERROR,
            category: ErrorCategory.RATE_LIMIT,
            retrySuggested: true,
            ...options
        });
    }
}

export class TimeoutError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.API_TIMEOUT_ERROR,
            category: ErrorCategory.TIMEOUT,
            retrySuggested: true,
            ...options
        });
    }
}

export class ExternalServiceError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.EXTERNAL_SERVICE_ERROR,
            category: ErrorCategory.EXTERNAL_SERVICE,
            ...options
        });
    }
}

export class ConfigurationError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.CONFIG_ERROR,
            category: ErrorCategory.CONFIGURATION,
            ...options
        });
    }
}

export class StorageError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.STORAGE_CONNECTION_ERROR,
            category: ErrorCategory.STORAGE,
            ...options
        });
    }
}

export class ServiceUnavailableError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.SERVICE_UNAVAILABLE_ERROR,
            category: ErrorCategory.SERVICE_UNAVAILABLE,
            retrySuggested: true,
            ...options
        });
    }
}

export class InternalServerError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.UNEXPECTED_ERROR,
            category: ErrorCategory.INTERNAL,
            severity: ErrorSeverity.HIGH,
            ...options
        });
    }
}

export class G8eKeyError extends G8eError {
    constructor(message, options = {}) {
        super(message, {
            code: ErrorCode.BUSINESS_LOGIC_ERROR,
            category: ErrorCategory.BUSINESS_LOGIC,
            ...options
        });
    }
}

