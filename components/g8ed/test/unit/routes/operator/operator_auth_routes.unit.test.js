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
import express from 'express';
import request from 'supertest';

// Pass-through the module-level operator auth limiters so this suite's
// request volume does not exhaust the real limiter windows.
vi.mock('@g8ed/middleware/rate-limit.js', async () => {
    const actual = await vi.importActual('@g8ed/middleware/rate-limit.js');
    return {
        ...actual,
        operatorAuthRateLimiter: (req, res, next) => next(),
        operatorAuthIpBackstopLimiter: (req, res, next) => next()
    };
});

import { createOperatorAuthRouter } from '@g8ed/routes/operator/operator_auth_routes.js';
import { AuthPaths } from '@g8ed/constants/api_paths.js';
import { OperatorAuthError, AuthError, ApiKeyError } from '@g8ed/constants/auth.js';

describe('OperatorAuthRoutes Unit Tests', () => {
    let app;
    let mockOperatorAuthService;
    let mockOperatorSessionService;
    let mockCliSessionService;
    let mockRateLimiters;
    let mockRequestTimestampMiddleware;

    beforeEach(() => {
        mockOperatorAuthService = {
            authenticateOperator: vi.fn()
        };
        mockOperatorSessionService = {
            validateSession: vi.fn(),
            refreshSession: vi.fn()
        };
        mockCliSessionService = {
            validateSession: vi.fn()
        };
        mockRateLimiters = {
            operatorRefreshRateLimiter: vi.fn((req, res, next) => next())
        };
        mockRequestTimestampMiddleware = {
            requireRequestTimestamp: vi.fn(() => (req, res, next) => next())
        };

        const router = createOperatorAuthRouter({
            services: {
                operatorAuthService: mockOperatorAuthService,
                operatorSessionService: mockOperatorSessionService,
                cliSessionService: mockCliSessionService
            },

            rateLimiters: mockRateLimiters,
            requestTimestampMiddleware: mockRequestTimestampMiddleware
        });

        app = express();
        app.use(express.json());
        app.use('/api/auth', router);
    });

    describe(`POST ${AuthPaths.OPERATOR_AUTH}`, () => {
        it('successfully authenticates operator', async () => {
            mockOperatorAuthService.authenticateOperator.mockResolvedValue({
                success: true,
                response: {
                    forClient: () => ({ success: true, operator_id: 'test-op-id' })
                }
            });

            const res = await request(app)
                .post('/api/auth/operator')
                .set('Authorization', 'Bearer test-token')
                .send({ system_info: { os: 'linux' } });

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(mockOperatorAuthService.authenticateOperator).toHaveBeenCalledWith({
                authorizationHeader: 'Bearer test-token',
                body: { system_info: { os: 'linux' } }
            });
        });

        it('returns error when authentication fails', async () => {
            mockOperatorAuthService.authenticateOperator.mockResolvedValue({
                success: false,
                statusCode: 401,
                error: 'Invalid key',
                code: 'INVALID_KEY'
            });

            const res = await request(app).post('/api/auth/operator');

            expect(res.status).toBe(401);
            expect(res.body.error).toBe('Invalid key');
        });

        it('returns 500 on unexpected error', async () => {
            mockOperatorAuthService.authenticateOperator.mockRejectedValue(new Error('Fatal'));

            const res = await request(app).post('/api/auth/operator');

            expect(res.status).toBe(500);
            expect(res.body.error).toBe(ApiKeyError.INTERNAL_ERROR);
        });
    });

    describe(`POST ${AuthPaths.OPERATOR_REFRESH}`, () => {
        it('successfully refreshes operator session', async () => {
            const mockSession = { operator_id: 'test-op-id', expires_at: 12345, operator_status: 'ACTIVE' };
            mockOperatorSessionService.validateSession.mockResolvedValue(mockSession);
            mockOperatorSessionService.refreshSession.mockResolvedValue(true);

            const res = await request(app)
                .post('/api/auth/operator/refresh')
                .send({ operator_session_id: 'test-session-id' });

            expect(res.status).toBe(200);
            expect(res.body.session.operator_id).toBe('test-op-id');
            expect(mockOperatorSessionService.refreshSession).toHaveBeenCalledWith('test-session-id', mockSession);
        });

        it('returns 400 if operator_session_id is missing', async () => {
            const res = await request(app).post('/api/auth/operator/refresh').send({});

            expect(res.status).toBe(400);
            expect(res.body.error).toBe(OperatorAuthError.MISSING_OPERATOR_SESSION_ID);
        });

        it('returns 401 if session is invalid', async () => {
            mockOperatorSessionService.validateSession.mockResolvedValue(null);

            const res = await request(app)
                .post('/api/auth/operator/refresh')
                .send({ operator_session_id: 'invalid-id' });

            expect(res.status).toBe(401);
            expect(res.body.error).toBe(AuthError.INVALID_OR_EXPIRED_SESSION);
        });
    });

    describe(`POST ${AuthPaths.OPERATOR_VALIDATE}`, () => {
        it('returns valid:true for a live operator session', async () => {
            mockOperatorSessionService.validateSession.mockResolvedValue({
                user_id: 'u-1',
                operator_id: 'op-1',
            });

            const res = await request(app)
                .post('/api/auth/operator/validate')
                .send({ operator_session_id: 'operator_session_abc' });

            expect(res.status).toBe(200);
            expect(res.body.valid).toBe(true);
            expect(res.body.success).toBe(true);
            expect(res.body.session_type).toBe('OPERATOR');
            expect(res.body.user_id).toBe('u-1');
            expect(res.body.operator_id).toBe('op-1');
            expect(mockCliSessionService.validateSession).not.toHaveBeenCalled();
        });

        it('routes CLI-prefixed session ids to cliSessionService', async () => {
            mockCliSessionService.validateSession.mockResolvedValue({
                user_id: 'u-2',
                operator_id: null,
            });

            const res = await request(app)
                .post('/api/auth/operator/validate')
                .send({ operator_session_id: 'cli_session_xyz' });

            expect(res.status).toBe(200);
            expect(res.body.valid).toBe(true);
            expect(res.body.session_type).toBe('CLI');
            expect(res.body.user_id).toBe('u-2');
            expect(res.body.operator_id).toBeNull();
            expect(mockOperatorSessionService.validateSession).not.toHaveBeenCalled();
        });

        it('returns 401 with valid:false when session is invalid', async () => {
            mockOperatorSessionService.validateSession.mockResolvedValue(null);

            const res = await request(app)
                .post('/api/auth/operator/validate')
                .send({ operator_session_id: 'operator_session_gone' });

            expect(res.status).toBe(401);
            expect(res.body.valid).toBe(false);
            expect(res.body.success).toBe(false);
        });

        it('returns 400 with valid:false when session id missing', async () => {
            const res = await request(app)
                .post('/api/auth/operator/validate')
                .send({});

            expect(res.status).toBe(400);
            expect(res.body.valid).toBe(false);
        });
    });
});
