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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { createInternalOperatorRouter } from '@vsod/routes/internal/internal_operator_routes.js';
import { OperatorStatus } from '@vsod/constants/operator.js';
import { DeviceLinkError } from '@vsod/constants/auth.js';
import { mockOperators } from '@test/fixtures/operators.fixture.js';

describe('Internal Operator Routes [UNIT]', () => {
    let router;
    let mockOperatorService;
    let mockG8ENodeOperatorService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockOperatorService = {
            refreshOperatorApiKey: vi.fn(),
            resetOperator: vi.fn(),
            getUserOperators: vi.fn(),
            initializeOperatorSlots: vi.fn(),
            getOperator: vi.fn(),
            getOperatorWithSessionContext: vi.fn()
        };
        mockG8ENodeOperatorService = {
            relaunchG8ENodeOperatorForUser: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        router = createInternalOperatorRouter({
            services: {
                operatorService: mockOperatorService,
                g8eNodeOperatorService: mockG8ENodeOperatorService
            },

            authorizationMiddleware: mockAuthorizationMiddleware
        });
    });

    const createMockReq = (overrides = {}) => ({
        params: {},
        body: {},
        ...overrides
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    describe('POST /:operatorId/refresh-key', () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/:operatorId/refresh-key');
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should refresh operator API key', async () => {
            const req = createMockReq({
                params: { operatorId: 'op_123' },
                body: { user_id: 'user_123' }
            });
            const res = createMockRes();

            mockOperatorService.refreshOperatorApiKey.mockResolvedValue({
                success: true,
                message: 'Key refreshed',
                new_operator_id: 'op_456'
            });

            await getRoute()(req, res);

            expect(mockOperatorService.refreshOperatorApiKey).toHaveBeenCalledWith('op_123', 'user_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                new_operator_id: 'op_456'
            }));
        });

        it('should return 400 if user_id is missing', async () => {
            const req = createMockReq({ params: { operatorId: 'op_123' } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'user_id is required in request body'
            }));
        });
    });

    describe('POST /:operatorId/reset-cache', () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/:operatorId/reset-cache');
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should reset operator cache', async () => {
            const req = createMockReq({ params: { operatorId: 'op_123' } });
            const res = createMockRes();

            mockOperatorService.resetOperator.mockResolvedValue({
                success: true,
                operator: { status: OperatorStatus.AVAILABLE }
            });

            await getRoute()(req, res);

            expect(mockOperatorService.resetOperator).toHaveBeenCalledWith('op_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                status: OperatorStatus.AVAILABLE
            }));
        });
    });

    describe('GET /user/:userId', () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/user/:userId' && s.route?.methods?.get);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should list operators for a user', async () => {
            const req = createMockReq({ params: { userId: 'user_123' } });
            const res = createMockRes();

            mockOperatorService.getUserOperators.mockResolvedValue({
                operators: [{ id: 'op_1', status: OperatorStatus.AVAILABLE }],
                totalCount: 1,
                activeCount: 1
            });

            await getRoute()(req, res);

            expect(mockOperatorService.getUserOperators).toHaveBeenCalledWith('user_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                total_count: 1
            }));
        });
    });

    describe('POST /user/:userId/reauth', () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/user/:userId/reauth' && s.route?.methods?.post);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should relaunch g8e-pod operator for user', async () => {
            const req = createMockReq({ params: { userId: 'user_123' } });
            const res = createMockRes();

            mockG8ENodeOperatorService.relaunchG8ENodeOperatorForUser.mockResolvedValue({
                success: true,
                operator_id: 'op_dp_123'
            });

            await getRoute()(req, res);

            expect(mockG8ENodeOperatorService.relaunchG8ENodeOperatorForUser).toHaveBeenCalledWith('user_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                operator_id: 'op_dp_123'
            }));
        });

        it('should return 404 if relaunch fails', async () => {
            const req = createMockReq({ params: { userId: 'user_123' } });
            const res = createMockRes();

            mockG8ENodeOperatorService.relaunchG8ENodeOperatorForUser.mockResolvedValue({
                success: false,
                error: 'Launch failed'
            });

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(404);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Launch failed'
            }));
        });

        it('should return 500 on unexpected error', async () => {
            const req = createMockReq({ params: { userId: 'user_123' } });
            const res = createMockRes();

            mockG8ENodeOperatorService.relaunchG8ENodeOperatorForUser.mockRejectedValue(new Error('Boom'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Boom'
            }));
        });
    });

    describe('POST /user/:userId/initialize-slots', () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/user/:userId/initialize-slots' && s.route?.methods?.post);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should initialize operator slots', async () => {
            const req = createMockReq({ 
                params: { userId: 'user_123' },
                body: { organization_id: 'org_123' }
            });
            const res = createMockRes();

            mockOperatorService.initializeOperatorSlots.mockResolvedValue(['op_1', 'op_2']);

            await getRoute()(req, res);

            expect(mockOperatorService.initializeOperatorSlots).toHaveBeenCalledWith('user_123', 'org_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                operator_ids: ['op_1', 'op_2'],
                count: 2
            }));
        });

        it('should use userId as organizationId if not provided', async () => {
            const req = createMockReq({ params: { userId: 'user_123' } });
            const res = createMockRes();

            mockOperatorService.initializeOperatorSlots.mockResolvedValue(['op_1']);

            await getRoute()(req, res);

            expect(mockOperatorService.initializeOperatorSlots).toHaveBeenCalledWith('user_123', 'user_123');
        });

        it('should return 500 on error', async () => {
            const req = createMockReq({ params: { userId: 'user_123' } });
            const res = createMockRes();

            mockOperatorService.initializeOperatorSlots.mockRejectedValue(new Error('Initialization failed'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Initialization failed'
            }));
        });
    });

    describe('GET /:operatorId/status', () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/:operatorId/status' && s.route?.methods?.get);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return operator status details', async () => {
            const operator = mockOperators.activeOperator;
            const req = createMockReq({ params: { operatorId: operator.operator_id } });
            const res = createMockRes();

            mockOperatorService.getOperator.mockResolvedValue(operator);

            await getRoute()(req, res);

            expect(mockOperatorService.getOperator).toHaveBeenCalledWith(operator.operator_id);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                operator_id: operator.operator_id
            }));
        });

        it('should return 404 if operator not found', async () => {
            const req = createMockReq({ params: { operatorId: 'op_missing' } });
            const res = createMockRes();

            mockOperatorService.getOperator.mockResolvedValue(null);

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(404);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: DeviceLinkError.OPERATOR_NOT_FOUND
            }));
        });
    });

    describe('GET /:operatorId/with-session-context', () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/:operatorId/with-session-context' && s.route?.methods?.get);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return operator with session context', async () => {
            const req = createMockReq({ params: { operatorId: 'op_123' } });
            const res = createMockRes();

            const mockContext = { 
                operator: { operator_id: 'op_123' },
                operator_session: { id: 'os_1' },
                web_session: { id: 'ws_1' },
                sessions_linked: true,
                forWire: function() {
                    return {
                        operator: this.operator,
                        operator_session: this.operator_session,
                        web_session: this.web_session,
                        sessions_linked: this.sessions_linked
                    };
                }
            };
            mockOperatorService.getOperatorWithSessionContext.mockResolvedValue(mockContext);

            await getRoute()(req, res);

            expect(mockOperatorService.getOperatorWithSessionContext).toHaveBeenCalledWith('op_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                operator: expect.objectContaining({ operator_id: 'op_123' }),
                operator_session: expect.objectContaining({ id: 'os_1' })
            }));
        });

        it('should return 404 if context not found', async () => {
            const req = createMockReq({ params: { operatorId: 'op_missing' } });
            const res = createMockRes();

            mockOperatorService.getOperatorWithSessionContext.mockResolvedValue(null);

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(404);
        });
    });

});
