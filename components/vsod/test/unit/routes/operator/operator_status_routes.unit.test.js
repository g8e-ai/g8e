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
import { createOperatorStatusRouter } from '@vsod/routes/operator/operator_status_routes.js';
import { OperatorPaths } from '@vsod/constants/api_paths.js';
import { OperatorStatus } from '@vsod/constants/operator.js';
import { globalContextMiddleware } from '@vsod/middleware/context.js';

describe('OperatorStatusRoutes Unit Tests', () => {
    let app;
    let mockOperatorService;
    let mockG8ENodeOperatorService;
    let mockAuthMiddleware;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockOperatorService = {
            relayStopCommandToG8ee: vi.fn()
        };
        mockG8ENodeOperatorService = {
            relaunchG8ENodeOperatorForUser: vi.fn()
        };
        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => {
                req.userId = 'test-user-id';
                req.webSessionId = 'test-web-session-id';
                next();
            })
        };
        mockAuthorizationMiddleware = {
            requireOperatorOwnership: vi.fn((req, res, next) => {
                req.operator = {
                    operator_id: 'test-op-id',
                    operator_session_id: 'test-op-session-id',
                    status: OperatorStatus.ACTIVE,
                    forClient: () => ({ operator_id: 'test-op-id' })
                };
                next();
            })
        };

        const router = createOperatorStatusRouter({
            services: {
                operatorService: mockOperatorService,
                g8eNodeOperatorService: mockG8ENodeOperatorService,
                internalHttpClient: vi.fn() // Add mock internalHttpClient if needed
            },
            authMiddleware: mockAuthMiddleware,
            authorizationMiddleware: mockAuthorizationMiddleware
        });

        app = express();
        app.use(express.json());
        app.use(globalContextMiddleware);
        app.use('/api/operator', router);
    });

    describe(`POST ${OperatorPaths.G8E_GATEWAY_REAUTH}`, () => {
        it('successfully initiates g8e-pod reauth', async () => {
            mockG8ENodeOperatorService.relaunchG8ENodeOperatorForUser.mockResolvedValue({
                success: true,
                operator_id: 'new-g8e-pod-op-id'
            });

            const res = await request(app).post('/api/operator/g8e-pod/reauth');

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(res.body.operator_id).toBe('new-g8e-pod-op-id');
            expect(mockG8ENodeOperatorService.relaunchG8ENodeOperatorForUser).toHaveBeenCalledWith('test-user-id');
        });

        it('returns 404 if g8e-pod reauth fails', async () => {
            mockG8ENodeOperatorService.relaunchG8ENodeOperatorForUser.mockResolvedValue({
                success: false,
                error: 'g8e-pod not found'
            });

            const res = await request(app).post('/api/operator/g8e-pod/reauth');

            expect(res.status).toBe(404);
            expect(res.body.error).toBe('g8e-pod not found');
        });
    });

    describe(`GET ${OperatorPaths.DETAILS}`, () => {
        it('returns operator details', async () => {
            const res = await request(app).get('/api/operator/test-op-id/details');

            expect(res.status).toBe(200);
            expect(res.body.operator_id).toBe('test-op-id');
            expect(res.body.status_display).toBe(OperatorStatus.ACTIVE);
        });
    });

    describe(`POST ${OperatorPaths.STOP}`, () => {
        it('successfully relays stop command', async () => {
            mockOperatorService.relayStopCommandToG8ee.mockResolvedValue({ success: true });

            const res = await request(app).post('/api/operator/test-op-id/stop');

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(mockOperatorService.relayStopCommandToG8ee).toHaveBeenCalled();
        });

        it('returns 400 if operator has no session', async () => {
            mockAuthorizationMiddleware.requireOperatorOwnership.mockImplementation((req, res, next) => {
                req.operator = { operator_id: 'test-op-id' }; // no operator_session_id
                next();
            });

            const res = await request(app).post('/api/operator/test-op-id/stop');

            expect(res.status).toBe(400);
            expect(res.body.error).toBe('Operator has no active session');
        });

        it('returns 500 if relay to g8ee fails', async () => {
            mockOperatorService.relayStopCommandToG8ee.mockRejectedValue(new Error('g8ee Unreachable'));

            const res = await request(app).post('/api/operator/test-op-id/stop');

            expect(res.status).toBe(500);
            expect(res.body.error).toBe('Failed to process stop request');
        });
    });
});
