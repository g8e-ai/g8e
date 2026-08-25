// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect } from 'vitest';
import request from 'supertest';
import { createApp } from '../../../server.js';

const TEST_GATEWAY_ORIGIN = 'https://test-gateway:8443';

describe('createApp gateway origin injection', () => {
    it('serves /g8e-config.js with the gateway origin as a JS assignment', async () => {
        const app = createApp({ gatewayOrigin: TEST_GATEWAY_ORIGIN });
        const res = await request(app)
            .get('/g8e-config.js')
            .expect(200);

        expect(res.headers['content-type']).toMatch(/application\/javascript/);
        expect(res.text).toContain(`window.G8E_GATEWAY_URL = "${TEST_GATEWAY_ORIGIN}"`);
    });

    it('sets no-store cache control on /g8e-config.js', async () => {
        const app = createApp({ gatewayOrigin: TEST_GATEWAY_ORIGIN });
        const res = await request(app).get('/g8e-config.js');
        expect(res.headers['cache-control']).toContain('no-store');
    });

    it('sets the CSP header allowing the configured gateway origin', async () => {
        const app = createApp({ gatewayOrigin: TEST_GATEWAY_ORIGIN });
        const res = await request(app).get('/').set('Accept', 'text/html');
        const csp = res.headers['content-security-policy'] ?? '';
        expect(csp).toContain(`connect-src 'self' ${TEST_GATEWAY_ORIGIN}`);
    });
});

describe('createApp SPA fallback', () => {
    const app = createApp({ gatewayOrigin: TEST_GATEWAY_ORIGIN });

    it('serves index.html for unknown HTML-accepting GET routes', async () => {
        const res = await request(app)
            .get('/some/unknown/client/route')
            .set('Accept', 'text/html')
            .expect(200);

        expect(res.headers['content-type']).toMatch(/text\/html/);
        expect(res.text).toContain('<!DOCTYPE html>');
        expect(res.text).toContain('/g8e-config.js');
    });

    it('falls through (404) for non-HTML GET requests to unknown routes', async () => {
        await request(app)
            .get('/some/unknown/api/route')
            .set('Accept', 'application/json')
            .expect(404);
    });

    it('serves index.html for the root path with HTML accept', async () => {
        const res = await request(app)
            .get('/')
            .set('Accept', 'text/html')
            .expect(200);

        expect(res.text).toContain('<!DOCTYPE html>');
    });
});

describe('createApp request logging', () => {
    it('logs method, path, status, and duration on response finish', async () => {
        const logs = [];
        const origLog = console.log;
        console.log = (...args) => logs.push(args.join(' '));
        try {
            const app = createApp({ gatewayOrigin: TEST_GATEWAY_ORIGIN });
            await request(app).get('/g8e-config.js').expect(200);
        } finally {
            console.log = origLog;
        }
        const reqLog = logs.find((l) => l.startsWith('GET /g8e-config.js '));
        expect(reqLog).toBeDefined();
        expect(reqLog).toMatch(/GET \/g8e-config\.js 200 \d+\.\dms$/);
    });
});

describe('createApp fail-closed on missing gateway origin', () => {
    it('throws when gatewayOrigin is undefined', () => {
        expect(() => createApp({})).toThrow(/G8E_GATEWAY_URL is required/);
    });

    it('throws when gatewayOrigin is empty string', () => {
        expect(() => createApp({ gatewayOrigin: '' })).toThrow(/G8E_GATEWAY_URL is required/);
    });

    it('throws when gatewayOrigin is null', () => {
        expect(() => createApp({ gatewayOrigin: null })).toThrow(/G8E_GATEWAY_URL is required/);
    });
});
