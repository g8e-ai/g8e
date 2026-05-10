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

import { describe, it, expect, vi } from 'vitest';
import { 
    G8eError, 
    ValidationError, 
    AuthenticationError, 
    AuthorizationError, 
    ResourceNotFoundError, 
    BusinessLogicError, 
    DatabaseError, 
    InternalServerError 
} from '@g8ed/services/error_service.js';
import { ErrorCode, ErrorCategory, ErrorSeverity } from '@g8ed/constants/errors.js';

describe('ErrorService', () => {
    describe('G8eError', () => {
        it('should initialize with default values', () => {
            const message = 'Test error message';
            const error = new G8eError(message);

            expect(error.message).toBe(message);
            expect(error.name).toBe('G8eError');
            expect(error.code).toBe(ErrorCode.GENERIC_ERROR);
            expect(error.category).toBe(ErrorCategory.INTERNAL);
            expect(error.severity).toBe(ErrorSeverity.MEDIUM);
            expect(error.source).toBe('g8ed');
            expect(error.component).toBe('g8ed');
            expect(error.details).toEqual({});
            expect(error.traceId).toBeNull();
            expect(error.requestId).toBeNull();
            expect(error.retrySuggested).toBe(false);
            expect(error.remediationSteps).toEqual([]);
            expect(error.timestamp).toBeDefined();
            expect(new Date(error.timestamp).getTime()).not.toBeNaN();
        });

        it('should initialize with provided options', () => {
            const options = {
                code: 'CUSTOM_CODE',
                category: ErrorCategory.VALIDATION,
                severity: ErrorSeverity.HIGH,
                source: 'custom-source',
                component: 'custom-component',
                details: { field: 'value' },
                traceId: 'trace-123',
                requestId: 'request-456',
                retrySuggested: true,
                remediationSteps: ['Step 1', 'Step 2']
            };
            const error = new G8eError('Message', options);

            expect(error.code).toBe(options.code);
            expect(error.category).toBe(options.category);
            expect(error.severity).toBe(options.severity);
            expect(error.source).toBe(options.source);
            expect(error.component).toBe(options.component);
            expect(error.details).toEqual(options.details);
            expect(error.traceId).toBe(options.traceId);
            expect(error.requestId).toBe(options.requestId);
            expect(error.retrySuggested).toBe(options.retrySuggested);
            expect(error.remediationSteps).toEqual(options.remediationSteps);
        });

        it('should handle cause option correctly', () => {
            const cause = new Error('Original error');
            cause.stack = 'Original stack trace';
            const error = new G8eError('New error', { cause });

            expect(error.cause).toBe(cause);
            expect(error.causeMessage).toBe(cause.message);
            expect(error.causeStackTrace).toEqual(['Original stack trace']);
        });

        it('should convert to standardized wire format via toG8eErrorResponse', () => {
            const options = {
                code: ErrorCode.VALIDATION_ERROR,
                category: ErrorCategory.VALIDATION,
                traceId: 'trace-id',
                requestId: 'request-id',
                cause: new Error('cause')
            };
            const error = new G8eError('Test message', options);
            const response = error.toG8eErrorResponse();

            expect(response.error.code).toBe(error.code);
            expect(response.error.message).toBe(error.message);
            expect(response.error.category).toBe(error.category);
            expect(response.error.timestamp).toBe(error.timestamp);
            expect(response.error.cause.cause_message).toBe(error.causeMessage);
            expect(response.trace_id).toBe(options.traceId);
            expect(response.execution_id).toBe(options.requestId);
        });

        it('should return correct HTTP status codes', () => {
            const testCases = [
                { category: ErrorCategory.VALIDATION, expected: 400 },
                { category: ErrorCategory.AUTH, expected: 401 },
                { category: ErrorCategory.PERMISSION, expected: 403 },
                { category: ErrorCategory.RESOURCE_NOT_FOUND, expected: 404 },
                { category: ErrorCategory.CONFLICT, expected: 409 },
                { category: ErrorCategory.RATE_LIMIT, expected: 429 },
                { category: ErrorCategory.SERVICE_UNAVAILABLE, expected: 503 },
                { category: ErrorCategory.EXTERNAL_SERVICE, expected: 502 },
                { category: ErrorCategory.TIMEOUT, expected: 504 },
                { category: 'UNKNOWN', expected: 500 }
            ];

            testCases.forEach(({ category, expected }) => {
                const error = new G8eError('msg', { category });
                expect(error.getHttpStatus()).toBe(expected);
            });
        });
    });

    describe('Specialized Error Classes', () => {
        it('ValidationError should have correct defaults', () => {
            const error = new ValidationError('Invalid');
            expect(error.code).toBe(ErrorCode.VALIDATION_ERROR);
            expect(error.category).toBe(ErrorCategory.VALIDATION);
            expect(error.getHttpStatus()).toBe(400);
        });

        it('AuthenticationError should have correct defaults', () => {
            const error = new AuthenticationError('Unauthenticated');
            expect(error.code).toBe(ErrorCode.AUTH_ERROR);
            expect(error.category).toBe(ErrorCategory.AUTH);
            expect(error.getHttpStatus()).toBe(401);
        });

        it('AuthorizationError should have correct defaults', () => {
            const error = new AuthorizationError('Unauthorized');
            expect(error.code).toBe(ErrorCode.INSUFFICIENT_PERMISSIONS);
            expect(error.category).toBe(ErrorCategory.PERMISSION);
            expect(error.getHttpStatus()).toBe(403);
        });

        it('ResourceNotFoundError should have correct defaults', () => {
            const error = new ResourceNotFoundError('Not Found');
            expect(error.code).toBe(ErrorCode.GENERIC_NOT_FOUND);
            expect(error.category).toBe(ErrorCategory.RESOURCE_NOT_FOUND);
            expect(error.getHttpStatus()).toBe(404);
        });

        it('InternalServerError should have correct defaults', () => {
            const error = new InternalServerError('Server Error');
            expect(error.code).toBe(ErrorCode.UNEXPECTED_ERROR);
            expect(error.category).toBe(ErrorCategory.INTERNAL);
            expect(error.severity).toBe(ErrorSeverity.HIGH);
            expect(error.getHttpStatus()).toBe(500);
        });
    });
});
