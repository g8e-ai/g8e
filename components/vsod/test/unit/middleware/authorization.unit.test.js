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
import { createAuthorizationMiddleware } from '@vsod/middleware/authorization.js';
import { AuthError, INTERNAL_AUTH_HEADER, DeviceLinkError } from '@vsod/constants/auth.js';
import { AuthenticationError, AuthorizationError } from '@vsod/services/error_service.js';

describe('Authorization Middleware', () => {
    let operatorService;
    let settingsService;
    let middleware;
    let req;
    let res;
    let next;

    beforeEach(() => {
        operatorService = {
            getOperator: vi.fn()
        };
        settingsService = {
            getInternalAuthToken: vi.fn().mockReturnValue('test-internal-token')
        };
        middleware = createAuthorizationMiddleware({ operatorService, settingsService });
        
        req = {
            headers: {},
            params: {},
            query: {},
            body: {},
            path: '/api/test',
            method: 'GET',
            ip: '127.0.0.1'
        };
        res = {
            status: vi.fn().mockReturnThis(),
            json: vi.fn().mockReturnThis()
        };
        next = vi.fn();
    });

    describe('requireOwnership', () => {
        it('should call next with AuthenticationError if no session userId present', async () => {
            await middleware.requireOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(AuthenticationError));
        });

        it('should call next if session userId matches param userId', async () => {
            req.userId = 'user-1';
            req.params.userId = 'user-1';
            await middleware.requireOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith();
            expect(req.authenticatedUserId).toBe('user-1');
        });

        it('should call next if session userId matches query user_id', async () => {
            req.userId = 'user-1';
            req.query.user_id = 'user-1';
            await middleware.requireOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith();
        });

        it('should call next if session userId matches body user_id', async () => {
            req.userId = 'user-1';
            req.body.user_id = 'user-1';
            await middleware.requireOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith();
        });

        it('should call next with AuthorizationError if session userId does not match request userId', async () => {
            req.userId = 'user-1';
            req.params.userId = 'user-2';
            await middleware.requireOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(AuthorizationError));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe(AuthError.FORBIDDEN_RESOURCE);
        });
    });

    describe('requireOperatorOwnership', () => {
        it('should call next with AuthenticationError if no session userId present', async () => {
            await middleware.requireOperatorOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(AuthenticationError));
        });

        it('should call next with AuthenticationError if operatorId is missing', async () => {
            req.userId = 'user-1';
            await middleware.requireOperatorOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(AuthenticationError));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe(AuthError.OPERATOR_ID_REQUIRED);
        });

        it('should call next with AuthenticationError if operator not found', async () => {
            req.userId = 'user-1';
            req.params.operatorId = 'op-1';
            operatorService.getOperator.mockResolvedValue(null);

            await middleware.requireOperatorOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(AuthenticationError));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe(DeviceLinkError.OPERATOR_NOT_FOUND);
        });

        it('should call next with AuthorizationError if operator belongs to different user', async () => {
            req.userId = 'user-1';
            req.params.operatorId = 'op-1';
            operatorService.getOperator.mockResolvedValue({ id: 'op-1', user_id: 'user-2' });

            await middleware.requireOperatorOwnership(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(AuthorizationError));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe(AuthError.FORBIDDEN_OPERATOR);
        });

        it('should call next and attach operator if ownership matches', async () => {
            req.userId = 'user-1';
            req.params.operatorId = 'op-1';
            const operator = { id: 'op-1', user_id: 'user-1' };
            operatorService.getOperator.mockResolvedValue(operator);

            await middleware.requireOperatorOwnership(req, res, next);

            expect(req.operator).toBe(operator);
            expect(req.authenticatedUserId).toBe('user-1');
            expect(next).toHaveBeenCalled();
        });
    });

    describe('requireInternalOrigin', () => {
        it('should call next if internal auth token matches', () => {
            req.headers[INTERNAL_AUTH_HEADER] = 'test-internal-token';
            req.originalUrl = '/api/internal/test';
            middleware.requireInternalOrigin(req, res, next);
            expect(next).toHaveBeenCalled();
        });

        it('should call next with AuthorizationError if internal auth token mismatches', () => {
            req.headers[INTERNAL_AUTH_HEADER] = 'wrong-token';
            req.originalUrl = '/api/internal/test';
            expect(() => middleware.requireInternalOrigin(req, res, next)).toThrow(AuthorizationError);
        });

        it('should call next for localhost health checks', () => {
            req.ip = '127.0.0.1';
            req.originalUrl = '/health/live';
            middleware.requireInternalOrigin(req, res, next);
            expect(next).toHaveBeenCalled();
        });

        it('should throw AuthorizationError for external health check attempts', () => {
            req.ip = '8.8.8.8';
            req.originalUrl = '/health/live';
            expect(() => middleware.requireInternalOrigin(req, res, next)).toThrow(AuthorizationError);
        });
    });
});
