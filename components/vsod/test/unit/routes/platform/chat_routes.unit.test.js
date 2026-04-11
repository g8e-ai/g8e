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
import { createChatRouter } from '@vsod/routes/platform/chat_routes.js';
import { ChatPaths } from '@vsod/constants/api_paths.js';
import { SystemHealth } from '@vsod/constants/ai.js';
import { 
    ChatMessageRequest, 
    VSOHttpContext, 
    StopAIRequest,
    InvestigationQueryRequest
} from '@vsod/models/request_models.js';
import { 
    ChatMessageResponse, 
    InvestigationListResponse,
    ChatActionResponse,
    ChatHealthResponse,
    ErrorResponse
} from '@vsod/models/response_models.js';

describe('Chat Routes [UNIT]', () => {
    let router;
    let mockInternalHttpClient;
    let mockBindingService;
    let mockAuthMiddleware;
    let mockAuthorizationMiddleware;
    let mockRateLimiters;

    beforeEach(() => {
        mockInternalHttpClient = {
            sendChatMessage: vi.fn(),
            queryInvestigations: vi.fn(),
            getInvestigation: vi.fn(),
            stopAIProcessing: vi.fn(),
            deleteCase: vi.fn(),
            healthCheck: vi.fn()
        };
        mockBindingService = {
            resolveBoundOperators: vi.fn().mockResolvedValue([])
        };
        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => next()),
            requireOperatorBinding: vi.fn(() => (req, res, next) => {
                req.boundOperators = [];
                next();
            })
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };
        mockRateLimiters = {
            chatRateLimiter: vi.fn((req, res, next) => next()),
            apiRateLimiter: vi.fn((req, res, next) => next())
        };

        router = createChatRouter({
            services: {
                internalHttpClient: mockInternalHttpClient,
                bindingService: mockBindingService
            },
            authMiddleware: mockAuthMiddleware,
            authorizationMiddleware: mockAuthorizationMiddleware,
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
            body: {},
            query: {},
            params: {},
            webSessionId: 'ws_123',
            userId: 'user_123',
            boundOperators: [],
            ...overrides
        };

        // Add lazy-evaluated vsoContext like the real middleware
        Object.defineProperty(req, 'vsoContext', {
            get() {
                if (!this._vsoContext) {
                    this._vsoContext = VSOHttpContext.parse({
                        web_session_id: this.webSessionId,
                        user_id: this.userId,
                        organization_id: this.session?.organization_id || this.session?.user_data?.organization_id || null,
                        case_id: this.body?.case_id || this.query?.case_id || this.params?.caseId || null,
                        investigation_id: this.body?.investigation_id || this.query?.investigation_id || this.params?.investigationId || null,
                        task_id: this.body?.task_id || this.query?.task_id || this.params?.taskId || null,
                        bound_operators: this.boundOperators || [],
                        execution_id: `req_test_${Date.now()}`
                    });
                }
                return this._vsoContext;
            }
        });

        return req;
    };

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        res.send = vi.fn().mockReturnValue(res);
        return res;
    };

    describe(`POST ${ChatPaths.SEND}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ChatPaths.SEND);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should send chat message and return response using typed models', async () => {
            const chatBody = {
                message: 'Hello AI',
                case_id: 'case_123',
                investigation_id: 'inv_123'
            };
            const req = createMockReq({ body: chatBody });
            const res = createMockRes();
            
            mockInternalHttpClient.sendChatMessage.mockResolvedValue({
                success: true,
                data: { reply: 'Hi there' }
            });

            await getRoute()(req, res);

            // Verify internal call used typed context
            expect(mockInternalHttpClient.sendChatMessage).toHaveBeenCalledWith(
                expect.any(Object), // result of ChatMessageRequest.forWire()
                expect.objectContaining({
                    web_session_id: 'ws_123',
                    user_id: 'user_123',
                    organization_id: 'org_123'
                })
            );

            // Verify response is a typed ChatMessageResponse
            expect(res.json).toHaveBeenCalledWith(expect.any(Object));
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.success).toBe(true);
        });

        it('should handle send failures with typed error response', async () => {
            const req = createMockReq({
                body: { message: 'fail', case_id: 'c', investigation_id: 'i' }
            });
            const res = createMockRes();
            mockInternalHttpClient.sendChatMessage.mockRejectedValue(new Error('VSE timeout'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.success).toBe(false);
            expect(responseData.error).toBe('VSE timeout');
        });
    });

    describe(`GET ${ChatPaths.INVESTIGATIONS}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ChatPaths.INVESTIGATIONS);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should query investigations using typed context', async () => {
            const req = createMockReq({ query: { case_id: 'case_123' } });
            const res = createMockRes();
            mockInternalHttpClient.queryInvestigations.mockResolvedValue([{ id: 'inv_1' }]);

            await getRoute()(req, res);

            expect(mockInternalHttpClient.queryInvestigations).toHaveBeenCalledWith(
                expect.any(URLSearchParams),
                expect.objectContaining({
                    web_session_id: 'ws_123',
                    user_id: 'user_123'
                })
            );
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.investigations).toHaveLength(1);
        });

        it('should handle query failures with typed error response', async () => {
            const req = createMockReq({ query: { case_id: 'case_123' } });
            const res = createMockRes();
            mockInternalHttpClient.queryInvestigations.mockRejectedValue(new Error('VSE query failed'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.success).toBe(false);
            expect(responseData.error).toBe('VSE query failed');
        });

        it('should handle empty investigation array response', async () => {
            const req = createMockReq({ query: { case_id: 'case_123' } });
            const res = createMockRes();
            mockInternalHttpClient.queryInvestigations.mockResolvedValue(null);

            await getRoute()(req, res);

            const responseData = res.json.mock.calls[0][0];
            expect(responseData.investigations).toEqual([]);
            expect(responseData.count).toBe(0);
        });

        it('should filter null/undefined query parameters', async () => {
            const req = createMockReq({ query: { case_id: 'case_123', investigation_id: null, foo: undefined } });
            const res = createMockRes();
            mockInternalHttpClient.queryInvestigations.mockResolvedValue([]);

            await getRoute()(req, res);

            const queryParams = mockInternalHttpClient.queryInvestigations.mock.calls[0][0];
            expect(queryParams.get('case_id')).toBe('case_123');
            expect(queryParams.get('investigation_id')).toBeNull();
            expect(queryParams.get('foo')).toBeNull();
        });
    });

    describe(`POST ${ChatPaths.STOP}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ChatPaths.STOP);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should stop AI processing with typed stop request', async () => {
            const req = createMockReq({
                body: { investigation_id: 'inv_123', reason: 'user_cancel' }
            });
            const res = createMockRes();
            mockInternalHttpClient.stopAIProcessing.mockResolvedValue({
                success: true,
                data: { message: 'Stopped', was_active: true }
            });

            await getRoute()(req, res);

            expect(mockInternalHttpClient.stopAIProcessing).toHaveBeenCalledWith(
                expect.any(Object), // StopAIRequest.forWire()
                expect.objectContaining({
                    web_session_id: 'ws_123',
                    user_id: 'user_123'
                })
            );
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.success).toBe(true);
        });

        it('should handle stop failures with typed ErrorResponse', async () => {
            const req = createMockReq({
                body: { investigation_id: 'inv_123', reason: 'user_cancel' }
            });
            const res = createMockRes();
            mockInternalHttpClient.stopAIProcessing.mockRejectedValue(new Error('VSE stop failed'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.error).toBe('VSE stop failed');
        });

        it('should include investigation_id and was_active in response data', async () => {
            const req = createMockReq({
                body: { investigation_id: 'inv_123', reason: 'user_cancel' }
            });
            const res = createMockRes();
            mockInternalHttpClient.stopAIProcessing.mockResolvedValue({
                success: true,
                data: { message: 'Stopped', was_active: false }
            });

            await getRoute()(req, res);

            const responseData = res.json.mock.calls[0][0];
            expect(responseData.data.investigation_id).toBe('inv_123');
            expect(responseData.data.was_active).toBe(false);
        });
    });

    describe(`DELETE ${ChatPaths.CASES}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ChatPaths.CASES);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should delete a case using typed context', async () => {
            const req = createMockReq({ params: { caseId: 'case_123' } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockInternalHttpClient.deleteCase).toHaveBeenCalledWith(
                'case_123',
                expect.objectContaining({
                    web_session_id: 'ws_123',
                    user_id: 'user_123'
                })
            );
            expect(res.status).toHaveBeenCalledWith(204);
        });

        it('should handle deletion failures with typed ErrorResponse', async () => {
            const req = createMockReq({ params: { caseId: 'case_123' } });
            const res = createMockRes();
            mockInternalHttpClient.deleteCase.mockRejectedValue(new Error('Case not found'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.error).toBe('Case not found');
        });
    });

    describe(`GET ${ChatPaths.INVESTIGATION}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ChatPaths.INVESTIGATION);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should get single investigation using typed context', async () => {
            const req = createMockReq({ 
                params: { investigationId: 'inv_123' },
                query: { case_id: 'case_123' }
            });
            const res = createMockRes();
            const mockInvestigation = { id: 'inv_123', title: 'Test Investigation' };
            mockInternalHttpClient.getInvestigation.mockResolvedValue(mockInvestigation);

            await getRoute()(req, res);

            expect(mockInternalHttpClient.getInvestigation).toHaveBeenCalledWith(
                'inv_123',
                expect.objectContaining({
                    web_session_id: 'ws_123',
                    user_id: 'user_123',
                    investigation_id: 'inv_123',
                    case_id: 'case_123'
                })
            );
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.success).toBe(true);
            expect(responseData.data).toEqual(mockInvestigation);
        });

        it('should handle investigation get failures with typed error response', async () => {
            const req = createMockReq({ params: { investigationId: 'inv_123' } });
            const res = createMockRes();
            mockInternalHttpClient.getInvestigation.mockRejectedValue(new Error('Investigation not found'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.success).toBe(false);
            expect(responseData.error).toBe('Investigation not found');
        });

        it('should handle missing case_id query parameter', async () => {
            const req = createMockReq({ params: { investigationId: 'inv_123' } });
            const res = createMockRes();
            mockInternalHttpClient.getInvestigation.mockResolvedValue({ id: 'inv_123' });

            await getRoute()(req, res);

            expect(mockInternalHttpClient.getInvestigation).toHaveBeenCalledWith(
                'inv_123',
                expect.objectContaining({
                    case_id: null
                })
            );
        });
    });

    describe(`GET ${ChatPaths.HEALTH}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ChatPaths.HEALTH);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return health status with typed ChatHealthResponse', async () => {
            const req = createMockReq();
            const res = createMockRes();
            const mockHealthStatus = { vse: 'healthy', vsodb: 'healthy' };
            mockInternalHttpClient.healthCheck.mockResolvedValue(mockHealthStatus);

            await getRoute()(req, res);

            expect(mockInternalHttpClient.healthCheck).toHaveBeenCalled();
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.service).toBe('vsod-http-routes');
            expect(responseData.status).toBe(SystemHealth.HEALTHY);
            expect(responseData.internal_services).toEqual(mockHealthStatus);
            expect(responseData.timestamp).toBeDefined();
        });

        it('should handle health check failures with unhealthy status', async () => {
            const req = createMockReq();
            const res = createMockRes();
            mockInternalHttpClient.healthCheck.mockRejectedValue(new Error('Health check failed'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.service).toBe('vsod-http-routes');
            expect(responseData.status).toBe(SystemHealth.UNHEALTHY);
            expect(responseData.error).toBe('Health check failed');
            expect(responseData.timestamp).toBeDefined();
        });

        it('should require internal origin middleware', async () => {
            const layer = router.stack.find(s => s.route?.path === ChatPaths.HEALTH);
            expect(layer.route.stack[0].handle).toBe(mockAuthorizationMiddleware.requireInternalOrigin);
        });
    });
});
