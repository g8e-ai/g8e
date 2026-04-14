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
import { createMCPRouter } from '@g8ed/routes/platform/mcp_routes.js';
import { MCPPaths } from '@g8ed/constants/api_paths.js';
import { G8eHttpContext } from '@g8ed/models/request_models.js';

describe('MCP Routes [UNIT]', () => {
    let router;
    let mockInternalHttpClient;
    let mockBindingService;
    let mockAuthMiddleware;
    let mockRateLimiters;
    let mockApiKeyService;
    let mockUserService;

    beforeEach(() => {
        mockInternalHttpClient = {
            mcpToolsList: vi.fn(),
            mcpToolsCall: vi.fn(),
        };
        mockBindingService = {
            resolveBoundOperators: vi.fn().mockResolvedValue([]),
            resolveBoundOperatorsForUser: vi.fn().mockResolvedValue([]),
        };
        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => next()),
            requireOperatorBinding: vi.fn((req, res, next) => {
                req.boundOperators = ['op1', 'op2'];
                next();
            }),
        };
        mockRateLimiters = {
            apiRateLimiter: vi.fn((req, res, next) => next()),
        };
        mockApiKeyService = {
            validateKey: vi.fn().mockResolvedValue({ success: false, error: 'Not configured' }),
            recordUsage: vi.fn().mockResolvedValue(undefined),
        };
        mockUserService = {
            getUser: vi.fn(),
        };

        router = createMCPRouter({
            services: {
                internalHttpClient: mockInternalHttpClient,
                apiKeyService: mockApiKeyService,
                userService: mockUserService,
                bindingService: mockBindingService
            },
            authMiddleware: mockAuthMiddleware,
            rateLimiters: mockRateLimiters,
        });
    });

    const createMockReq = (body = {}) => {
        const req = {
            headers: {},
            query: {},
            session: {
                id: 'sess_123',
                user_id: 'user_123',
                organization_id: 'org_123',
            },
            body,
            webSessionId: 'ws_123',
            userId: 'user_123',
            boundOperators: ['op1', 'op2'],
        };

        // Add lazy-evaluated g8eContext like the real middleware
        Object.defineProperty(req, 'g8eContext', {
            get() {
                if (!this._g8eContext) {
                    this._g8eContext = G8eHttpContext.parse({
                        web_session_id: this.webSessionId,
                        user_id: this.userId,
                        organization_id: this.organizationId || this.session?.organization_id || this.session?.user_data?.organization_id || null,
                        case_id: this.body?.case_id || this.query?.case_id || this.params?.caseId || null,
                        investigation_id: this.body?.investigation_id || this.query?.investigation_id || this.params?.investigationId || null,
                        task_id: this.body?.task_id || this.query?.task_id || this.params?.taskId || null,
                        bound_operators: this.boundOperators || [],
                        execution_id: `req_test_mcp_${Date.now()}`
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
        res.end = vi.fn().mockReturnValue(res);
        return res;
    };

    const getRoute = () => {
        const layer = router.stack.find(s => s.route?.path === MCPPaths.ROOT);
        return layer.route.stack[layer.route.stack.length - 1].handle;
    };

    describe('initialize', () => {
        it('should return server info and capabilities', async () => {
            const req = createMockReq({
                jsonrpc: '2.0',
                id: '1',
                method: 'initialize',
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith(
                expect.objectContaining({
                    jsonrpc: '2.0',
                    id: '1',
                    result: expect.objectContaining({
                        protocolVersion: '2025-03-26',
                        serverInfo: expect.objectContaining({ name: 'g8e' }),
                        capabilities: expect.objectContaining({ tools: {} }),
                    }),
                })
            );
        });
    });

    describe('notifications/initialized', () => {
        it('should return 204 with no body', async () => {
            const req = createMockReq({
                jsonrpc: '2.0',
                id: '2',
                method: 'notifications/initialized',
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(204);
            expect(res.end).toHaveBeenCalled();
        });
    });

    describe('ping', () => {
        it('should return empty result', async () => {
            const req = createMockReq({
                jsonrpc: '2.0',
                id: '3',
                method: 'ping',
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith({
                jsonrpc: '2.0',
                id: '3',
                result: {},
            });
        });
    });

    describe('tools/list', () => {
        it('should relay to g8ee and return tool list', async () => {
            const tools = [
                { name: 'run_commands_with_operator', description: 'Run a command', inputSchema: {} },
            ];
            mockInternalHttpClient.mcpToolsList.mockResolvedValue({ tools });

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '4',
                method: 'tools/list',
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockInternalHttpClient.mcpToolsList).toHaveBeenCalledWith(
                expect.objectContaining({
                    web_session_id: 'ws_123',
                    user_id: 'user_123',
                })
            );

            const response = res.json.mock.calls[0][0];
            expect(response.jsonrpc).toBe('2.0');
            expect(response.id).toBe('4');
            expect(response.result.tools).toEqual(tools);
        });

        it('should return error on internal failure', async () => {
            mockInternalHttpClient.mcpToolsList.mockRejectedValue(new Error('g8ee unavailable'));

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '5',
                method: 'tools/list',
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            const response = res.json.mock.calls[0][0];
            expect(response.error.code).toBe(-32603);
            expect(response.error.message).toContain('g8ee unavailable');
        });
    });

    describe('tools/call', () => {
        it('should relay tool call to g8ee and return result', async () => {
            mockInternalHttpClient.mcpToolsCall.mockResolvedValue({
                result: {
                    content: [{ type: 'text', text: 'command output' }],
                    isError: false,
                },
            });

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '6',
                method: 'tools/call',
                params: {
                    name: 'run_commands_with_operator',
                    arguments: { command: 'ls' },
                },
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockInternalHttpClient.mcpToolsCall).toHaveBeenCalledWith(
                expect.objectContaining({
                    tool_name: 'run_commands_with_operator',
                    arguments: { command: 'ls' },
                    request_id: '6',
                }),
                expect.objectContaining({ web_session_id: 'ws_123' })
            );

            const response = res.json.mock.calls[0][0];
            expect(response.jsonrpc).toBe('2.0');
            expect(response.result.content[0].text).toBe('command output');
        });

        it('should return JSON-RPC error when g8ee returns error field', async () => {
            mockInternalHttpClient.mcpToolsCall.mockResolvedValue({
                error: { code: -32603, message: 'No operators available' },
            });

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '7',
                method: 'tools/call',
                params: { name: 'run_commands_with_operator', arguments: {} },
            });
            const res = createMockRes();

            await getRoute()(req, res);

            const response = res.json.mock.calls[0][0];
            expect(response.error.code).toBe(-32603);
        });

        it('should return -32602 when tool name is missing', async () => {
            const req = createMockReq({
                jsonrpc: '2.0',
                id: '8',
                method: 'tools/call',
                params: {},
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            const response = res.json.mock.calls[0][0];
            expect(response.error.code).toBe(-32602);
        });
    });

    describe('invalid requests', () => {
        it('should return -32600 for missing jsonrpc field', async () => {
            const req = createMockReq({ method: 'initialize' });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            const response = res.json.mock.calls[0][0];
            expect(response.error.code).toBe(-32600);
        });

        it('should return -32600 for missing method field', async () => {
            const req = createMockReq({ jsonrpc: '2.0', id: '9' });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            const response = res.json.mock.calls[0][0];
            expect(response.error.code).toBe(-32600);
        });

        it('should return -32601 for unknown method', async () => {
            const req = createMockReq({
                jsonrpc: '2.0',
                id: '10',
                method: 'resources/list',
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            const response = res.json.mock.calls[0][0];
            expect(response.error.code).toBe(-32601);
        });
    });

    describe('auth middleware', () => {
        it('should use requireAuth middleware when OAuth Client ID not provided', async () => {
            const req = createMockReq({
                jsonrpc: '2.0',
                id: '1',
                method: 'initialize',
            });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockAuthMiddleware.requireAuth).toHaveBeenCalled();
        });
    });

    describe('OAuth Client ID authentication', () => {
        it('should authenticate via OAuth Client ID when provided in header', async () => {
            mockApiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: 'user_123', organization_id: 'org_123' },
            });
            mockUserService.getUser.mockResolvedValue({
                id: 'user_123',
                organization_id: 'org_123',
            });
            mockBindingService.resolveBoundOperatorsForUser.mockResolvedValue([]);

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '1',
                method: 'initialize',
            });
            req.headers['x-oauth-client-id'] = 'g8e_test_key';
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockApiKeyService.validateKey).toHaveBeenCalledWith('g8e_test_key');
            expect(mockUserService.getUser).toHaveBeenCalledWith('user_123');
            expect(mockApiKeyService.recordUsage).toHaveBeenCalledWith('g8e_test_key');
            expect(res.json).toHaveBeenCalledWith(
                expect.objectContaining({
                    jsonrpc: '2.0',
                    id: '1',
                    result: expect.objectContaining({
                        protocolVersion: '2025-03-26',
                    }),
                })
            );
        });

        it('should authenticate via OAuth Client ID when provided in query param', async () => {
            mockApiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: 'user_123', organization_id: 'org_123' },
            });
            mockUserService.getUser.mockResolvedValue({
                id: 'user_123',
                organization_id: 'org_123',
            });
            mockBindingService.resolveBoundOperatorsForUser.mockResolvedValue([]);

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '1',
                method: 'initialize',
            });
            req.query.oauth_client_id = 'g8e_test_key';
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockApiKeyService.validateKey).toHaveBeenCalledWith('g8e_test_key');
            expect(res.json).toHaveBeenCalledWith(
                expect.objectContaining({
                    jsonrpc: '2.0',
                    id: '1',
                })
            );
        });

        it('should return 401 when OAuth Client ID validation fails', async () => {
            mockApiKeyService.validateKey.mockResolvedValue({
                success: false,
                error: 'Invalid API key',
            });

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '1',
                method: 'initialize',
            });
            req.headers['x-oauth-client-id'] = 'invalid_key';
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(401);
            const response = res.json.mock.calls[0][0];
            expect(response.error.code).toBe(-32000);
            expect(response.error.message).toContain('OAuth Client ID authentication failed');
        });

        it('should return 401 when OAuth Client ID is missing user_id', async () => {
            mockApiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: null, organization_id: 'org_123' },
            });

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '1',
                method: 'initialize',
            });
            req.headers['x-oauth-client-id'] = 'g8e_test_key';
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(401);
        });

        it('should return 401 when user not found for OAuth Client ID', async () => {
            mockApiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: 'user_123', organization_id: 'org_123' },
            });
            mockUserService.getUser.mockResolvedValue(null);

            const req = createMockReq({
                jsonrpc: '2.0',
                id: '1',
                method: 'initialize',
            });
            req.headers['x-oauth-client-id'] = 'g8e_test_key';
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(401);
        });

    });
});
