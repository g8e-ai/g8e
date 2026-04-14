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
import { createOperatorApprovalRouter } from '@g8ed/routes/operator/operator_approval_routes.js';
import { OperatorApprovalPaths } from '@g8ed/constants/api_paths.js';
import { OperatorRelayService } from '@g8ed/services/operator/operator_relay_service.js';
import { globalContextMiddleware } from '@g8ed/middleware/context.js';

vi.mock('@g8ed/services/operator/operator_relay_service.js', () => ({
    OperatorRelayService: vi.fn(function () {
        Object.assign(this, {
            relayApprovalResponseToG8ee: vi.fn(),
            relayDirectCommandToG8ee: vi.fn(),
            relayPendingApprovalsFromG8ee: vi.fn()
        });
    })
}));

describe('OperatorApprovalRoutes Unit Tests', () => {
    let app;
    let mockBindingService;
    let mockOperatorSessionService;
    let mockAuthMiddleware;
    let mockRateLimiters;
    let mockInternalHttpClient;
    let mockRelay;

    beforeEach(() => {
        OperatorRelayService.mockClear();

        mockBindingService = {
            getBoundOperatorSessionIds: vi.fn(),
            resolveBoundOperators: vi.fn()
        };
        mockOperatorSessionService = {
            validateSession: vi.fn()
        };
        mockInternalHttpClient = {};
        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => {
                req.userId = 'test-user-id';
                req.webSessionId = 'test-web-session-id';
                req.session = { organization_id: 'test-org-id' };
                next();
            }),
            requireOperatorBinding: vi.fn(async (req, res, next) => {
                req.boundOperators = (await mockBindingService.resolveBoundOperators(req.webSessionId)) || [];
                next();
            }),
            requireAtLeastOneOperator: vi.fn((req, res, next) => {
                if (!req.boundOperators || req.boundOperators.length === 0) {
                    return res.status(400).json({ error: 'No active Operator session found' });
                }
                next();
            }),
        };
        mockRateLimiters = {
            apiRateLimiter: vi.fn((req, res, next) => next())
        };

        const router = createOperatorApprovalRouter({
            services: {
                bindingService: mockBindingService,
                operatorSessionService: mockOperatorSessionService,
                internalHttpClient: mockInternalHttpClient
            },
            authMiddleware: mockAuthMiddleware,
            rateLimiters: mockRateLimiters
        });

        // Capture the relay instance created by the router
        mockRelay = OperatorRelayService.mock.results[0].value;

        app = express();
        app.use(express.json());
        app.use(globalContextMiddleware);
        app.use('/api/operator/approval', router);
    });

    describe(`POST ${OperatorApprovalPaths.RESPOND}`, () => {
        const validBody = {
            approval_id: 'test-approval-id',
            approved: true,
            case_id: 'test-case-id',
            investigation_id: 'test-inv-id',
            task_id: 'test-task-id'
        };

        const mockBoundOperators = [{
            operator_id: 'test-op-id',
            operator_session_id: 'test-op-session-id',
            status: 'bound',
            operator_type: 'system'
        }];

        it('successfully relays approval response via OperatorRelayService', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            mockRelay.relayApprovalResponseToG8ee.mockResolvedValue({ success: true });

            const res = await request(app)
                .post('/api/operator/approval/respond')
                .send(validBody);

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(mockRelay.relayApprovalResponseToG8ee).toHaveBeenCalled();
        });

        it('OperatorRelayService is constructed with internalHttpClient', () => {
            expect(OperatorRelayService).toHaveBeenCalledWith({ internalHttpClient: mockInternalHttpClient });
        });

        it('relay body matches g8ee contract (approval_id, approved, reason only)', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            mockRelay.relayApprovalResponseToG8ee.mockResolvedValue({ success: true });

            await request(app)
                .post('/api/operator/approval/respond')
                .send({ ...validBody, reason: 'User approved' });

            const [relayedBody] = mockRelay.relayApprovalResponseToG8ee.mock.calls[0];
            expect(Object.keys(relayedBody).sort()).toEqual(['approval_id', 'approved', 'reason']);
            expect(relayedBody.approval_id).toBe('test-approval-id');
            expect(relayedBody.approved).toBe(true);
            expect(relayedBody.reason).toBe('User approved');
        });

        it('context fields travel via G8eHttpContext with bound_operators', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            mockRelay.relayApprovalResponseToG8ee.mockResolvedValue({ success: true });

            await request(app)
                .post('/api/operator/approval/respond')
                .send(validBody);

            const [, g8eContext] = mockRelay.relayApprovalResponseToG8ee.mock.calls[0];
            expect(g8eContext.web_session_id).toBe('test-web-session-id');
            expect(g8eContext.user_id).toBe('test-user-id');
            expect(g8eContext.case_id).toBe('test-case-id');
            expect(g8eContext.investigation_id).toBe('test-inv-id');
            expect(g8eContext.task_id).toBe('test-task-id');
            expect(g8eContext.bound_operators).toEqual(mockBoundOperators);
        });

        it('does not depend on req.services (regression)', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            mockRelay.relayApprovalResponseToG8ee.mockResolvedValue({ success: true });

            const res = await request(app)
                .post('/api/operator/approval/respond')
                .send(validBody);

            expect(res.status).toBe(200);
        });

        it('returns 400 if context fields are missing', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            const res = await request(app)
                .post('/api/operator/approval/respond')
                .send({
                    approval_id: 'test-id',
                    approved: true,
                });

            expect(res.status).toBe(400);
            expect(res.body.error).toBe('case_id, investigation_id, and task_id are required');
        });

        it('returns 400 if no bound operators resolved', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue([]);

            const res = await request(app)
                .post('/api/operator/approval/respond')
                .send(validBody);

            expect(res.status).toBe(400);
            expect(res.body.error).toBe('No active Operator session found');
        });
    });

    describe(`POST ${OperatorApprovalPaths.DIRECT_COMMAND}`, () => {
        it('successfully relays direct command via OperatorRelayService', async () => {
            mockBindingService.getBoundOperatorSessionIds.mockResolvedValue(['test-op-session-id']);
            mockOperatorSessionService.validateSession.mockResolvedValue({ operator_id: 'test-op-id' });
            mockBindingService.resolveBoundOperators.mockResolvedValue([{ operator_id: 'test-op-id' }]);
            mockRelay.relayDirectCommandToG8ee.mockResolvedValue({ success: true });

            const res = await request(app)
                .post('/api/operator/approval/direct-command')
                .send({
                    execution_id: 'test-exec-id',
                    command: 'ls -la'
                });

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(mockRelay.relayDirectCommandToG8ee).toHaveBeenCalled();
        });

        it('returns 400 if no bound operator session', async () => {
            mockBindingService.getBoundOperatorSessionIds.mockResolvedValue([]);

            const res = await request(app)
                .post('/api/operator/approval/direct-command')
                .send({ execution_id: 'test-id', command: 'ls' });

            expect(res.status).toBe(400);
            expect(res.body.error).toContain('No active Operator session found');
        });
    });

    describe(`GET ${OperatorApprovalPaths.PENDING}`, () => {
        const mockBoundOperators = [{
            operator_id: 'test-op-id',
            operator_session_id: 'test-op-session-id',
            status: 'bound',
            operator_type: 'system'
        }];

        it('successfully fetches pending approvals via OperatorRelayService', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            mockRelay.relayPendingApprovalsFromG8ee.mockResolvedValue({
                success: true,
                pending_approvals: {
                    'approval-1': { approval_id: 'approval-1', approved: false, command: 'ls' },
                    'approval-2': { approval_id: 'approval-2', approved: false, command: 'pwd' }
                }
            });

            const res = await request(app)
                .get('/api/operator/approval/pending')
                .query({ case_id: 'test-case-id', investigation_id: 'test-inv-id' });

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(res.body.pending_approvals).toBeDefined();
            expect(mockRelay.relayPendingApprovalsFromG8ee).toHaveBeenCalled();
        });

        it('passes query params to G8eHttpContext', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            mockRelay.relayPendingApprovalsFromG8ee.mockResolvedValue({ success: true, pending_approvals: {} });

            const res = await request(app)
                .get('/api/operator/approval/pending')
                .query({ case_id: 'test-case-id', investigation_id: 'test-inv-id' });

            expect(res.status).toBe(200);
            expect(mockRelay.relayPendingApprovalsFromG8ee).toHaveBeenCalled();
            const [g8eContext] = mockRelay.relayPendingApprovalsFromG8ee.mock.calls[0];
            expect(g8eContext.case_id).toBe('test-case-id');
            expect(g8eContext.investigation_id).toBe('test-inv-id');
        });

        it('handles missing query params gracefully (nulls)', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            mockRelay.relayPendingApprovalsFromG8ee.mockResolvedValue({ success: true, pending_approvals: {} });

            const res = await request(app)
                .get('/api/operator/approval/pending');

            expect(res.status).toBe(200);
            expect(mockRelay.relayPendingApprovalsFromG8ee).toHaveBeenCalled();
            const [g8eContext] = mockRelay.relayPendingApprovalsFromG8ee.mock.calls[0];
            expect(g8eContext.case_id).toBeNull();
            expect(g8eContext.investigation_id).toBeNull();
        });

        it('returns 400 if no bound operators resolved', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue([]);

            const res = await request(app)
                .get('/api/operator/approval/pending');

            expect(res.status).toBe(400);
            expect(res.body.error).toBe('No active Operator session found');
        });

        it('context includes bound_operators array', async () => {
            mockBindingService.resolveBoundOperators.mockResolvedValue(mockBoundOperators);
            mockRelay.relayPendingApprovalsFromG8ee.mockResolvedValue({ success: true, pending_approvals: {} });

            const res = await request(app)
                .get('/api/operator/approval/pending');

            expect(res.status).toBe(200);
            expect(mockRelay.relayPendingApprovalsFromG8ee).toHaveBeenCalled();
            const calls = mockRelay.relayPendingApprovalsFromG8ee.mock.calls;
            expect(calls.length).toBeGreaterThan(0);
            const [g8eContext] = calls[0];
            expect(g8eContext.bound_operators).toEqual(mockBoundOperators);
        });
    });
});
