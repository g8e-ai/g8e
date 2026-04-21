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
import { createOperatorApiKeyRouter } from '@g8ed/routes/operator/operator_api_key_routes.js';
import { OperatorPaths } from '@g8ed/constants/api_paths.js';

describe('OperatorApiKeyRoutes Unit Tests', () => {
    let app;
    let mockOperatorService;
    let mockAuthMiddleware;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockOperatorService = {
            refreshOperatorApiKey: vi.fn()
        };

        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => {
                req.userId = 'test-user-id';
                next();
            })
        };

        mockAuthorizationMiddleware = {
            requireOperatorOwnership: vi.fn((req, res, next) => {
                req.operator = {
                    id: 'test-op-id',
                    api_key: 'test-api-key'
                };
                next();
            })
        };

        const router = createOperatorApiKeyRouter({
            services: {
                operatorService: mockOperatorService
            },

            authMiddleware: mockAuthMiddleware,
            authorizationMiddleware: mockAuthorizationMiddleware
        });

        app = express();
        app.use(express.json());
        app.use('/api/operator', router);
    });

    describe(`GET ${OperatorPaths.API_KEY}`, () => {
        it('returns API key when found', async () => {
            const res = await request(app).get('/api/operator/test-op-id/api-key');
            
            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(res.body.api_key).toBe('test-api-key');
            expect(res.body.id).toBe('test-op-id');
        });

        it('returns 404 if API key is missing', async () => {
            mockAuthorizationMiddleware.requireOperatorOwnership.mockImplementation((req, res, next) => {
                req.operator = { id: 'test-op-id' }; // no api_key
                next();
            });

            const res = await request(app).get('/api/operator/test-op-id/api-key');
            
            expect(res.status).toBe(404);
            expect(res.body.error).toBe('No API key found for this operator');
        });
    });

    describe(`POST ${OperatorPaths.REFRESH_API_KEY}`, () => {
        it('successfully refreshes API key', async () => {
            mockOperatorService.refreshOperatorApiKey.mockResolvedValue({
                success: true,
                message: 'Refreshed',
                new_operator_id: 'new-op-id',
                slot_number: 1,
                new_api_key: 'new-api-key'
            });

            const res = await request(app).post('/api/operator/test-op-id/refresh-api-key');
            
            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(res.body.new_api_key).toBe('new-api-key');
            expect(mockOperatorService.refreshOperatorApiKey).toHaveBeenCalledWith('test-op-id', 'test-user-id');
        });

        it('returns 403 if refresh is unauthorized', async () => {
            mockOperatorService.refreshOperatorApiKey.mockResolvedValue({
                success: false,
                message: 'Unauthorized refresh'
            });

            const res = await request(app).post('/api/operator/test-op-id/refresh-api-key');
            
            expect(res.status).toBe(403);
            expect(res.body.error).toBe('Unauthorized refresh');
        });

        it('returns 400 if refresh fails with other message', async () => {
            mockOperatorService.refreshOperatorApiKey.mockResolvedValue({
                success: false,
                message: 'Invalid slot'
            });

            const res = await request(app).post('/api/operator/test-op-id/refresh-api-key');
            
            expect(res.status).toBe(400);
            expect(res.body.error).toBe('Invalid slot');
        });
    });
});
