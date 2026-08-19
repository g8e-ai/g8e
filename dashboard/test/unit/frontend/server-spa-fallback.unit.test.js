// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect } from 'vitest';
import request from 'supertest';
import { app } from '../../../server.js';

describe('server SPA fallback', () => {
    it('serves index.html for unknown HTML-accepting GET routes', async () => {
        const res = await request(app)
            .get('/some/unknown/client/route')
            .set('Accept', 'text/html')
            .expect(200);

        expect(res.headers['content-type']).toMatch(/text\/html/);
        expect(res.text).toContain('<!DOCTYPE html>');
        expect(res.text).toContain('window.G8E_GATEWAY_URL');
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

    it('sets the CSP header allowing the gateway origin', async () => {
        const res = await request(app).get('/').set('Accept', 'text/html');
        const csp = res.headers['content-security-policy'] ?? '';
        expect(csp).toContain("connect-src 'self' https://localhost:8443");
    });
});
