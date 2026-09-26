// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockServiceClient } from '@test/mocks/mock-browser-env.js';

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
            injected.setResponse('gateway', '/api/v1/operators/bind', makeOkResponse());
            operatorPanelService.setClient(injected);

            await operatorPanelService.bindOperator('op-di');

            expect(injected.getRequestLog()).toHaveLength(1);
        });

        it('falls back to window.serviceClient when no client is injected', async () => {
            const windowClient = new MockServiceClient();
            windowClient.setResponse('gateway', '/api/v1/operators/bind', makeOkResponse());
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
            it('POSTs to /api/v1/operators/bind with operator_id', async () => {
                client.setResponse('gateway', '/api/v1/operators/bind', makeOkResponse({ success: true, operator: {} }));

                const resp = await operatorPanelService.bindOperator('op-123');

                expect(resp.ok).toBe(true);
                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', service: 'gateway', path: '/api/v1/operators/bind', body: { operator_id: 'op-123' } });
            });

            it('returns the raw Response so callers can inspect ok/status', async () => {
                client.setResponse('gateway', '/api/v1/operators/bind', makeErrorResponse(403, { error: 'Slot limit reached' }));

                const resp = await operatorPanelService.bindOperator('op-full');

                expect(resp.ok).toBe(false);
                expect(resp.status).toBe(403);
            });
        });

        describe('unbindOperator', () => {
            it('POSTs to /api/v1/operators/unbind with empty body by default', async () => {
                client.setResponse('gateway', '/api/v1/operators/unbind', makeOkResponse());

                await operatorPanelService.unbindOperator();

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/v1/operators/unbind', body: {} });
            });

            it('forwards the body when operator_id is provided for force-unbind', async () => {
                client.setResponse('gateway', '/api/v1/operators/unbind', makeOkResponse());

                await operatorPanelService.unbindOperator({ operator_id: 'op-456' });

                const [req] = client.getRequestLog();
                expect(req.body).toEqual({ operator_id: 'op-456' });
            });
        });

        describe('bindAllOperators', () => {
            it('POSTs to /api/v1/operators/bind with operator_ids array', async () => {
                client.setResponse('gateway', '/api/v1/operators/bind', makeOkResponse());

                await operatorPanelService.bindAllOperators(['op-1', 'op-2']);

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/v1/operators/bind', body: { operator_ids: ['op-1', 'op-2'] } });
            });
        });

        describe('unbindAllOperators', () => {
            it('POSTs to /api/v1/operators/unbind with operator_ids array', async () => {
                client.setResponse('gateway', '/api/v1/operators/unbind', makeOkResponse());

                await operatorPanelService.unbindAllOperators(['op-1', 'op-2']);

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/v1/operators/unbind', body: { operator_ids: ['op-1', 'op-2'] } });
            });
        });

        describe('stopOperator', () => {
            it('POSTs to /api/v1/operators/:id/stop', async () => {
                client.setResponse('gateway', '/api/v1/operators/op-789/stop', makeOkResponse());

                await operatorPanelService.stopOperator('op-789');

                const [req] = client.getRequestLog();
                expect(req).toMatchObject({ method: 'POST', path: '/api/v1/operators/op-789/stop' });
            });

            it('builds path correctly for different operator IDs', async () => {
                await operatorPanelService.stopOperator('abc-def-ghi');

                const [req] = client.getRequestLog();
                expect(req.path).toBe('/api/v1/operators/abc-def-ghi/stop');
            });
        });
    });

    describe('operator details', () => {
        let client;

        beforeEach(() => {
            client = new MockServiceClient();
            operatorPanelService.setClient(client);
        });

        it('GETs /api/v1/operators/:id', async () => {
            client.setResponse('gateway', '/api/v1/operators/op-abc', makeOkResponse({ id: 'op-abc' }));

            const resp = await operatorPanelService.getOperatorDetails('op-abc');

            expect(resp.ok).toBe(true);
            const [req] = client.getRequestLog();
            expect(req).toMatchObject({ method: 'GET', service: 'gateway', path: '/api/v1/operators/op-abc' });
        });
    });

    describe('listOperators', () => {
        it('normalizes gateway operator documents for the panel', async () => {
            const client = new MockServiceClient();
            client.setResponse('gateway', '/api/v1/operators', makeOkResponse({
                success: true,
                operators: [{ id: 'op-1', status: 'active', bound_web_session_id: 'sess-1' }],
            }));
            operatorPanelService.setClient(client);

            const data = await operatorPanelService.listOperators();

            expect(data.operators[0].operator_id).toBe('op-1');
            expect(data.operators[0].web_session_id).toBe('sess-1');
        });
    });

    describe('removed browser surfaces', () => {
        it('rejects API key and device-link calls', async () => {
            await expect(operatorPanelService.getOperatorApiKey('op-1')).rejects.toThrow(/not available/i);
            await expect(operatorPanelService.generateDeviceLink('op-1')).rejects.toThrow(/not available/i);
        });
    });
});
