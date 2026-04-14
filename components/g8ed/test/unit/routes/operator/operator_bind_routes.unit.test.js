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
import { createBindOperatorsRouter } from '@g8ed/routes/operator/operator_bind_routes.js';
import { OperatorPaths } from '@g8ed/constants/api_paths.js';

describe('BindOperatorsRoutes Unit Tests', () => {
    let app;
    let mockBindOperatorsService;
    let mockAuthMiddleware;

    beforeEach(() => {
        mockBindOperatorsService = {
            bindOperator: vi.fn(),
            bindOperators: vi.fn(),
            unbindOperator: vi.fn(),
            unbindOperators: vi.fn()
        };

        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => {
                req.userId = 'test-user-id';
                req.webSessionId = 'test-web-session-id';
                next();
            })
        };

        const router = createBindOperatorsRouter({
            services: {
                bindOperatorsService: mockBindOperatorsService
            },

            authMiddleware: mockAuthMiddleware
        });

        app = express();
        app.use(express.json());
        app.use('/api/operator', router);
    });

    describe(`POST ${OperatorPaths.BIND}`, () => {
        it('successfully binds an operator', async () => {
            mockBindOperatorsService.bindOperators.mockResolvedValue({
                success: true,
                statusCode: 200,
                bound_count: 1,
                failed_count: 0,
                bound_operator_ids: ['test-op-id'],
                failed_operator_ids: [],
                errors: []
            });

            const res = await request(app)
                .post('/api/operator/bind')
                .send({ operator_ids: ['test-op-id'] });

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(res.body.bound_count).toBe(1);
            expect(mockBindOperatorsService.bindOperators).toHaveBeenCalledWith(
                expect.objectContaining({
                    operator_ids: ['test-op-id'],
                    web_session_id: 'test-web-session-id',
                    user_id: 'test-user-id'
                })
            );
        });

        it('returns multi-status 207 when partial binding fails', async () => {
            mockBindOperatorsService.bindOperators.mockResolvedValue({
                success: true,
                statusCode: 207,
                bound_count: 1,
                failed_count: 1,
                bound_operator_ids: ['test-op-1'],
                failed_operator_ids: ['test-op-2'],
                errors: [{ operator_id: 'test-op-2', error: 'Not found' }]
            });

            const res = await request(app)
                .post('/api/operator/bind')
                .send({ operator_ids: ['test-op-1', 'test-op-2'] });

            expect(res.status).toBe(207);
            expect(res.body.success).toBe(true);
            expect(res.body.failed_count).toBe(1);
        });

        it('returns error when all bindings fail', async () => {
            mockBindOperatorsService.bindOperators.mockResolvedValue({
                success: false,
                statusCode: 400,
                bound_count: 0,
                failed_count: 1,
                bound_operator_ids: [],
                failed_operator_ids: ['test-op-id'],
                errors: [{ operator_id: 'test-op-id', error: 'Limit reached' }]
            });

            const res = await request(app)
                .post('/api/operator/bind')
                .send({ operator_ids: ['test-op-id'] });

            expect(res.status).toBe(400);
            expect(res.body.success).toBe(false);
            expect(res.body.errors[0].error).toBe('Limit reached');
        });
    });

    describe(`POST ${OperatorPaths.BIND_ALL}`, () => {
        it('successfully binds multiple operators', async () => {
            mockBindOperatorsService.bindOperators.mockResolvedValue({
                success: true,
                statusCode: 200,
                bound_count: 2,
                failed_count: 0,
                bound_operator_ids: ['op1', 'op2'],
                failed_operator_ids: [],
                errors: []
            });

            const res = await request(app)
                .post('/api/operator/bind-all')
                .send({ operator_ids: ['op1', 'op2'] });

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(res.body.bound_count).toBe(2);
        });
    });

    describe(`POST ${OperatorPaths.UNBIND}`, () => {
        it('successfully unbinds an operator', async () => {
            mockBindOperatorsService.unbindOperators.mockResolvedValue({
                success: true,
                statusCode: 200,
                unbound_count: 1,
                failed_count: 0,
                unbound_operator_ids: ['test-op-id'],
                failed_operator_ids: [],
                errors: []
            });

            const res = await request(app)
                .post('/api/operator/unbind')
                .send({ operator_id: 'test-op-id' });

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(mockBindOperatorsService.unbindOperators).toHaveBeenCalled();
        });
    });

    describe(`POST ${OperatorPaths.UNBIND_ALL}`, () => {
        it('successfully unbinds multiple operators', async () => {
            mockBindOperatorsService.unbindOperators.mockResolvedValue({
                success: true,
                statusCode: 200,
                unbound_count: 2,
                failed_count: 0,
                unbound_operator_ids: ['op1', 'op2'],
                failed_operator_ids: [],
                errors: []
            });

            const res = await request(app)
                .post('/api/operator/unbind-all')
                .send({ operator_ids: ['op1', 'op2'] });

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(res.body.unbound_count).toBe(2);
        });
    });
});
