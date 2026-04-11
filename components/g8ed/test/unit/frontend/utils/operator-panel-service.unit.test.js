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

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockServiceClient } from '@test/mocks/mock-browser-env.js';
import { ComponentName } from '@g8ed/public/js/models/investigation-models.js';

let operatorPanelService;

beforeEach(async () => {
    vi.resetModules();
    ({ operatorPanelService } = await import('@g8ed/public/js/utils/operator-panel-service.js'));
});

afterEach(() => {
    vi.restoreAllMocks();
});

function makeOkResponse(body = { success: true }) {
    return { ok: true, status: 200, json: async () => body };
}

function makeErrorResponse(status = 400, body = { error: 'Bad Request' }) {
    return { ok: false, status, json: async () => body };
}

describe('OperatorPanelService [UNIT - jsdom]', () => {

    describe('dependency injection', () => {
        it('uses the injected client instead of window.serviceClient', async () => {
            const injected = new MockServiceClient();
            injected.setResponse('g8ed', '/api/operators/bind', makeOkResponse());
            operatorPanelService.setClient(injected);

            await operatorPanelService.bindOperator('op-di');

            expect(injected.getRequestLog()).toHaveLength(1);
        });

        it('falls back to window.serviceClient when no client is injected', async () => {
            const windowClient = new MockServiceClient();
            windowClient.setResponse('g8ed', '/api/operators/bind', makeOkResponse());
            window.serviceClient = windowClient;

            await operatorPanelService.bindOperator('op-fallback');

            expect(windowClient.getRequestLog()).toHaveLength(1);
        });
    });

    describe('operator lifecycle', () => {
        let client;

        beforeEach(() => {
            client = new MockServiceClient();
            operatorPanelService.setClient(client);
        });

        describe('bindOperator', () => {
            it('POSTs to /api/operators/bind with operator_id', async () => {
                client.setResponse('g8ed', '/api/operators/bind', makeOkResponse({ success: true, operator: {} }));

                const resp = await operatorPanelService.bindOperator('op-123');

                expect(resp.ok).toBe(true);
                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', service: 'g8ed', path: '/api/operators/bind', body: { operator_id: 'op-123' } });
            });

            it('returns the raw Response so callers can inspect ok/status', async () => {
                client.setResponse('g8ed', '/api/operators/bind', makeErrorResponse(403, { error: 'Slot limit reached' }));

                const resp = await operatorPanelService.bindOperator('op-full');

                expect(resp.ok).toBe(false);
                expect(resp.status).toBe(403);
            });
        });

        describe('unbindOperator', () => {
            it('POSTs to /api/operators/unbind with empty body by default', async () => {
                client.setResponse('g8ed', '/api/operators/unbind', makeOkResponse());

                await operatorPanelService.unbindOperator();

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/operators/unbind', body: {} });
            });

            it('forwards the body when operator_id is provided for force-unbind', async () => {
                client.setResponse('g8ed', '/api/operators/unbind', makeOkResponse());

                await operatorPanelService.unbindOperator({ operator_id: 'op-456' });

                const [req] = client.getRequestLog();
                expect(req.body).toEqual({ operator_id: 'op-456' });
            });
        });

        describe('bindAllOperators', () => {
            it('POSTs to /api/operators/bind-all with operator_ids array', async () => {
                client.setResponse('g8ed', '/api/operators/bind-all', makeOkResponse());

                await operatorPanelService.bindAllOperators(['op-1', 'op-2']);

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/operators/bind-all', body: { operator_ids: ['op-1', 'op-2'] } });
            });
        });

        describe('unbindAllOperators', () => {
            it('POSTs to /api/operators/unbind-all with operator_ids array', async () => {
                client.setResponse('g8ed', '/api/operators/unbind-all', makeOkResponse());

                await operatorPanelService.unbindAllOperators(['op-1', 'op-2']);

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/operators/unbind-all', body: { operator_ids: ['op-1', 'op-2'] } });
            });
        });

        describe('stopOperator', () => {
            it('POSTs to /api/operators/:id/stop', async () => {
                client.setResponse('g8ed', '/api/operators/op-789/stop', makeOkResponse());

                await operatorPanelService.stopOperator('op-789');

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/operators/op-789/stop' });
            });

            it('builds path correctly for different operator IDs', async () => {
                await operatorPanelService.stopOperator('abc-def-ghi');

                const [req] = client.getRequestLog();
                expect(req.path).toBe('/api/operators/abc-def-ghi/stop');
            });
        });
    });

    describe('operator details & API keys', () => {
        let client;

        beforeEach(() => {
            client = new MockServiceClient();
            operatorPanelService.setClient(client);
        });

        describe('getOperatorDetails', () => {
            it('GETs /api/operators/:id/details', async () => {
                client.setResponse('g8ed', '/api/operators/op-abc/details', makeOkResponse({ data: { operator_id: 'op-abc' } }));

                const resp = await operatorPanelService.getOperatorDetails('op-abc');

                expect(resp.ok).toBe(true);
                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'GET', service: 'g8ed', path: '/api/operators/op-abc/details' });
            });

            it('returns 404 response when operator not found', async () => {
                const resp = await operatorPanelService.getOperatorDetails('op-missing');

                expect(resp.ok).toBe(false);
                expect(resp.status).toBe(404);
            });
        });

        describe('getOperatorApiKey', () => {
            it('GETs /api/operators/:id/api-key', async () => {
                client.setResponse('g8ed', '/api/operators/op-abc/api-key', makeOkResponse({ api_key: 'key-xyz' }));

                const resp = await operatorPanelService.getOperatorApiKey('op-abc');

                expect(resp.ok).toBe(true);
                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'GET', path: '/api/operators/op-abc/api-key' });
            });
        });

        describe('refreshOperatorApiKey', () => {
            it('POSTs to /api/operators/:id/refresh-api-key', async () => {
                client.setResponse('g8ed', '/api/operators/op-abc/refresh-api-key', makeOkResponse({ new_api_key: 'new-key', slot_number: 1 }));

                const resp = await operatorPanelService.refreshOperatorApiKey('op-abc');

                expect(resp.ok).toBe(true);
                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/operators/op-abc/refresh-api-key' });
            });
        });
    });

    describe('device links', () => {
        let client;

        beforeEach(() => {
            client = new MockServiceClient();
            operatorPanelService.setClient(client);
        });

        describe('generateDeviceLink', () => {
            it('POSTs to /api/auth/link/generate with operator_id', async () => {
                client.setResponse('g8ed', '/api/auth/link/generate', makeOkResponse({ token: 'tok-123', expires_at: '2026-01-01T00:00:00Z' }));

                await operatorPanelService.generateDeviceLink('op-abc');

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/auth/link/generate', body: { operator_id: 'op-abc' } });
            });
        });

        describe('createDeviceLink', () => {
            it('POSTs to /api/device-links with max_uses, expires_in_hours, and name', async () => {
                client.setResponse('g8ed', '/api/device-links', makeOkResponse({ success: true }));

                await operatorPanelService.createDeviceLink({ maxUses: 5, expiresInHours: 24, name: 'fleet-link' });

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({
                    method: 'POST',
                    path: '/api/device-links',
                    body: { max_uses: 5, expires_in_hours: 24, name: 'fleet-link' }
                });
            });

            it('omits name from body when name is not provided', async () => {
                client.setResponse('g8ed', '/api/device-links', makeOkResponse());

                await operatorPanelService.createDeviceLink({ maxUses: 1, expiresInHours: 1 });

                const [req] = client.getRequestLog();
                expect(req.body.name).toBeUndefined();
            });

            it('omits name from body when name is empty string', async () => {
                client.setResponse('g8ed', '/api/device-links', makeOkResponse());

                await operatorPanelService.createDeviceLink({ maxUses: 1, expiresInHours: 1, name: '' });

                const [req] = client.getRequestLog();
                expect(req.body.name).toBeUndefined();
            });
        });

        describe('listDeviceLinks', () => {
            it('GETs /api/device-links', async () => {
                client.setResponse('g8ed', '/api/device-links', makeOkResponse({ links: [] }));

                await operatorPanelService.listDeviceLinks();

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'GET', service: 'g8ed', path: '/api/device-links' });
            });
        });

        describe('revokeDeviceLink', () => {
            it('DELETEs /api/device-links/:tokenId without query string', async () => {
                client.setResponse('g8ed', '/api/device-links/token-abc', makeOkResponse());

                await operatorPanelService.revokeDeviceLink('token-abc');

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'DELETE', service: 'g8ed', path: '/api/device-links/token-abc' });
            });

            it('uses the exact tokenId in the path', async () => {
                await operatorPanelService.revokeDeviceLink('tok-xyz-999');

                const [req] = client.getRequestLog();
                expect(req.path).toBe('/api/device-links/tok-xyz-999');
            });
        });

        describe('deleteDeviceLink', () => {
            it('DELETEs /api/device-links/:tokenId?action=delete', async () => {
                client.setResponse('g8ed', '/api/device-links/token-abc?action=delete', makeOkResponse());

                await operatorPanelService.deleteDeviceLink('token-abc');

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'DELETE', service: 'g8ed', path: '/api/device-links/token-abc?action=delete' });
            });
        });
    });

    describe('device authorization', () => {
        let client;

        beforeEach(() => {
            client = new MockServiceClient();
            operatorPanelService.setClient(client);
        });

        describe('authorizeDevice', () => {
            it('POSTs to /api/auth/link/:token/authorize', async () => {
                client.setResponse('g8ed', '/api/auth/link/tok-xyz/authorize', makeOkResponse());

                await operatorPanelService.authorizeDevice('tok-xyz');

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', service: 'g8ed', path: '/api/auth/link/tok-xyz/authorize' });
            });

            it('returns error response for unknown token', async () => {
                client.setResponse('g8ed', '/api/auth/link/bad-tok/authorize', makeErrorResponse(404, { error: 'Token not found' }));

                const resp = await operatorPanelService.authorizeDevice('bad-tok');

                expect(resp.ok).toBe(false);
                expect(resp.status).toBe(404);
            });
        });

        describe('rejectDevice', () => {
            it('POSTs to /api/auth/link/:token/reject', async () => {
                client.setResponse('g8ed', '/api/auth/link/tok-xyz/reject', makeOkResponse());

                await operatorPanelService.rejectDevice('tok-xyz');

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', service: 'g8ed', path: '/api/auth/link/tok-xyz/reject' });
            });
        });
    });
});
