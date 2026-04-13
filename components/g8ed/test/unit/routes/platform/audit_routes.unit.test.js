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
import { createAuditRouter } from '@g8ed/routes/platform/audit_routes.js';
import { AuditPaths } from '@g8ed/constants/api_paths.js';
import { G8eHttpContext } from '@g8ed/models/request_models.js';
import { AuditEventResponse, ErrorResponse } from '@g8ed/models/response_models.js';

describe('Audit Routes [UNIT]', () => {
    let router;
    let mockAuditService;
    let mockBindingService;
    let mockInternalHttpClient;
    let mockInvestigationService;
    let mockAuthMiddleware;
    let mockRateLimiters;

    beforeEach(() => {
        mockAuditService = {
            flattenInvestigationEvents: vi.fn(),
            buildCsvFromEvents: vi.fn()
        };
        mockBindingService = {
            resolveBoundOperators: vi.fn().mockResolvedValue([])
        };
        mockInternalHttpClient = {
            queryInvestigations: vi.fn()
        };
        mockInvestigationService = {
            queryInvestigations: vi.fn()
        };
        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => next()),
            requireOperatorBinding: vi.fn((req, res, next) => {
                req.boundOperators = ['op1', 'op2'];
                next();
            }),
        };
        mockRateLimiters = {
            auditRateLimiter: vi.fn((req, res, next) => next())
        };

        router = createAuditRouter({
            services: {
                auditService: mockAuditService,
                internalHttpClient: mockInternalHttpClient,
                investigationService: mockInvestigationService,
                bindingService: mockBindingService
            },
            authMiddleware: mockAuthMiddleware,
            rateLimiters: mockRateLimiters
        });
    });

    const createMockReq = (overrides = {}) => {
        const req = {
            session: {
                id: 'sess_123',
                user_id: 'user_123',
                organization_id: 'org_123'
            },
            query: {},
            body: {},
            params: {},
            webSessionId: 'ws_123',
            userId: 'user_123',
            boundOperators: ['op1', 'op2'],
            ...overrides
        };

        // Add lazy-evaluated g8eContext like the real middleware
        Object.defineProperty(req, 'g8eContext', {
            get() {
                if (!this._g8eContext) {
                    this._g8eContext = G8eHttpContext.parse({
                        web_session_id: this.webSessionId,
                        user_id: this.userId,
                        organization_id: this.session?.organization_id || this.session?.user_data?.organization_id || null,
                        case_id: this.body?.case_id || this.query?.case_id || this.params?.caseId || null,
                        investigation_id: this.body?.investigation_id || this.query?.investigation_id || this.params?.investigationId || null,
                        task_id: this.body?.task_id || this.query?.task_id || this.params?.taskId || null,
                        bound_operators: this.boundOperators || [],
                        execution_id: `req_test_audit_${Date.now()}`
                    });
                }
                return this._g8eContext;
            }
        });

        return req;
    };

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        res.setHeader = vi.fn().mockReturnValue(res);
        res.send = vi.fn().mockReturnValue(res);
        return res;
    };

    describe(`GET ${AuditPaths.EVENTS}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === AuditPaths.EVENTS);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should fetch and return audited events', async () => {
            const mockInvestigations = [{ id: 'inv_1' }];
            const mockEvents = [{ id: 'evt_1', timestamp: 123456789 }];
            
            mockInvestigationService.queryInvestigations.mockResolvedValue(mockInvestigations);
            mockAuditService.flattenInvestigationEvents.mockReturnValue(mockEvents);

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockInvestigationService.queryInvestigations).toHaveBeenCalledWith(
                expect.arrayContaining([
                    { field: 'user_id', operator: '==', value: 'user_123' }
                ]),
                100
            );
            expect(res.json).toHaveBeenCalledWith(expect.any(Object));
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.events).toEqual(mockEvents);
            expect(responseData.count).toBe(1);
            expect(responseData.total_investigations).toBe(1);
        });

        it('should handle errors from internal services', async () => {
            mockInvestigationService.queryInvestigations.mockRejectedValue(new Error('Cache aside error'));

            const req = createMockReq();
            const res = createMockRes();
            const next = vi.fn();

            await getRoute()(req, res, next);

            expect(next).toHaveBeenCalledWith(expect.any(Error));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe('Cache aside error');
        });
    });

    describe(`GET ${AuditPaths.DOWNLOAD}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === AuditPaths.DOWNLOAD);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return JSON download by default', async () => {
            const mockEvents = [{ id: 'evt_1' }];
            mockInvestigationService.queryInvestigations.mockResolvedValue([]);
            mockAuditService.flattenInvestigationEvents.mockReturnValue(mockEvents);

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockInvestigationService.queryInvestigations).toHaveBeenCalledWith(
                expect.arrayContaining([
                    { field: 'user_id', operator: '==', value: 'user_123' }
                ]),
                100
            );
            expect(res.setHeader).toHaveBeenCalledWith('Content-Type', 'application/json');
            expect(res.setHeader).toHaveBeenCalledWith('Content-Disposition', expect.stringContaining('.json"'));
            expect(res.json).toHaveBeenCalledWith(expect.any(Object));
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.events).toEqual(mockEvents);
            expect(responseData.total_events).toBe(1);
        });

        it('should return CSV download when requested', async () => {
            const mockEvents = [{ id: 'evt_1' }];
            const mockCsv = 'id,timestamp\nevt_1,123456789';
            
            mockInvestigationService.queryInvestigations.mockResolvedValue([]);
            mockAuditService.flattenInvestigationEvents.mockReturnValue(mockEvents);
            mockAuditService.buildCsvFromEvents.mockReturnValue(mockCsv);

            const req = createMockReq({ query: { format: 'csv' } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockInvestigationService.queryInvestigations).toHaveBeenCalled();
            expect(res.setHeader).toHaveBeenCalledWith('Content-Type', 'text/csv');
            expect(res.setHeader).toHaveBeenCalledWith('Content-Disposition', expect.stringContaining('.csv"'));
            expect(res.send).toHaveBeenCalledWith(mockCsv);
        });

        it('should handle export errors', async () => {
            mockInvestigationService.queryInvestigations.mockRejectedValue(new Error('Export failed'));

            const req = createMockReq();
            const res = createMockRes();
            const next = vi.fn();

            await getRoute()(req, res, next);

            expect(next).toHaveBeenCalledWith(expect.any(Error));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe('Export failed');
        });
    });
});
