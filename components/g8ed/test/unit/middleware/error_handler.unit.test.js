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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createErrorHandlerMiddleware } from '@g8ed/middleware/error_handler.js';
import { G8eError, InternalServerError, AuthenticationError } from '@g8ed/services/error_service.js';
import { HttpStatusMessage, ErrorCategory } from '@g8ed/constants/errors.js';

describe('ErrorHandler Middleware', () => {
    let config;
    let middleware;
    let req;
    let res;
    let next;

    beforeEach(() => {
        config = { environment: 'development' };
        middleware = createErrorHandlerMiddleware({ config });
        
        req = {
            url: '/api/test',
            method: 'GET',
            headers: {
                'x-request-id': 'req-123',
                'x-trace-id': 'trace-456'
            }
        };
        res = {
            status: vi.fn().mockReturnThis(),
            json: vi.fn().mockReturnThis()
        };
        next = vi.fn();
    });

    it('should handle G8eError instances', () => {
        const err = new AuthenticationError('Auth failed');
        middleware(err, req, res, next);

        expect(res.status).toHaveBeenCalledWith(401);
        expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
            error: expect.objectContaining({
                message: 'Auth failed',
                category: ErrorCategory.AUTH
            })
        }));
    });

    it('should wrap generic Error in InternalServerError', () => {
        const err = new Error('Generic failure');
        middleware(err, req, res, next);

        expect(res.status).toHaveBeenCalledWith(500);
        expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
            error: expect.objectContaining({
                category: ErrorCategory.INTERNAL,
                message: 'Generic failure'
            })
        }));
    });

    it('should redact internal details in production', () => {
        config.environment = 'production';
        middleware = createErrorHandlerMiddleware({ config });
        
        const err = new InternalServerError('Sensitive DB error');
        middleware(err, req, res, next);

        const responseBody = res.json.mock.calls[0][0];
        expect(responseBody.error.message).toBe(HttpStatusMessage.INTERNAL_SERVER_ERROR);
        expect(responseBody.error.cause).toBeNull();
    });

    it('should not redact non-internal G8eErrors in production', () => {
        config.environment = 'production';
        middleware = createErrorHandlerMiddleware({ config });
        
        const err = new AuthenticationError('Invalid credentials');
        middleware(err, req, res, next);

        expect(res.status).toHaveBeenCalledWith(401);
        expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
            error: expect.objectContaining({
                message: 'Invalid credentials'
            })
        }));
    });
});
