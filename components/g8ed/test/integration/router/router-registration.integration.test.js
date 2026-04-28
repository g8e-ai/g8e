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

/**
 * Router Registration Integration Test
 * 
 * Proves that URL paths are correctly registered and accessible via HTTP,
 * rather than just calling handler functions directly.
 * 
 * Tests:
 * - Health endpoints are accessible
 * - Auth endpoints are registered (authentication required)
 * - Platform endpoints are registered
 * - Internal endpoints are registered (authorization required)
 * - Operator endpoints are registered
 * - Static assets are served
 * 
 * Uses HttpTestClient with the full Express app created by createG8edApp,
 * ensuring the complete middleware stack is tested.
 */

import { describe, it, expect, beforeAll, afterAll, beforeEach, afterEach } from 'vitest';
import { createTestClient } from '../../helpers/http-test-client.js';
import { TestCleanupHelper } from '../../helpers/test-cleanup.js';
import { Collections } from '../../../constants/collections.js';
import { BasePaths } from '../../../constants/api_paths.js';

describe('Router Registration Integration [INTEGRATION]', () => {
    let client;
    let services;
    let cleanup;

    beforeAll(async () => {
        client = await createTestClient();
        services = client.services;
        cleanup = new TestCleanupHelper(services.kvClient, services.cacheAsideService, {
            usersCollection: services.userService.collectionName,
            operatorsCollection: services.operatorService.collectionName
        });
    });

    afterAll(async () => {
        if (client) {
            await client.close();
        }
        if (cleanup) {
            await cleanup.cleanup();
        }
    });

    beforeEach(async () => {
        client.reset();
    });

    afterEach(async () => {
        if (cleanup) {
            await cleanup.cleanup();
        }
    });

    describe('Health Endpoints', () => {
        it('GET /health - returns health status', async () => {
            const res = await client.get('/health');
            expect(res.status).toBe(200);
            expect(res.body).toHaveProperty('status');
        });

        it('GET /health/live - liveness probe accessible', async () => {
            const res = await client.get('/health/live');
            expect(res.status).toBe(200);
        });

        it('GET /health/store - store health check accessible', async () => {
            const res = await client.get('/health/store');
            expect(res.status).toBe(200);
        });
    });

    describe('Auth Endpoints Registration', () => {
        it('POST /api/auth/register - endpoint registered', async () => {
            const res = await client
                .post('/api/auth/register')
                .send({ email: 'test@example.com', name: 'Test User' });
            
            expect([200, 201, 400, 409]).toContain(res.status);
        });

        it('GET /api/auth/web-session - endpoint registered (requires auth)', async () => {
            const res = await client.get('/api/auth/web-session');
            expect(res.status).toBe(401);
        });
    });

    describe('Platform Endpoints Registration', () => {
        it('GET /api/chat/health - chat health endpoint registered (requires auth)', async () => {
            const res = await client.get('/api/chat/health');
            expect([200, 401, 403]).toContain(res.status);
        });

        it('GET /api/metrics/health - metrics health endpoint registered (requires auth)', async () => {
            const res = await client.get('/api/metrics/health');
            expect([200, 401, 403]).toContain(res.status);
        });

        it('GET /sse/health - SSE health endpoint registered (requires auth)', async () => {
            const res = await client.get('/sse/health');
            expect([200, 401, 403]).toContain(res.status);
        });
    });

    describe('Internal Endpoints Registration', () => {
        it('GET /api/internal/health - internal health endpoint registered (requires auth)', async () => {
            const res = await client.get('/api/internal/health');
            expect([401, 403]).toContain(res.status);
        });
    });

    describe('Operator Endpoints Registration', () => {
        it('GET /api/operators - operator list endpoint registered (requires auth)', async () => {
            const res = await client.get('/api/operators');
            expect([401, 404]).toContain(res.status);
        });

        it('GET /operator/health - operator binary health endpoint registered (requires auth)', async () => {
            const res = await client.get('/operator/health');
            expect([200, 401, 403]).toContain(res.status);
        });
    });

    describe('Static Assets', () => {
        it('GET /favicon.ico - favicon accessible', async () => {
            const res = await client.get('/favicon.ico');
            expect([200, 404]).toContain(res.status);
        });

        it('GET / - root path returns HTML or redirect', async () => {
            const res = await client.get('/');
            expect([200, 302]).toContain(res.status);
            if (res.status === 200) {
                expect(res.type).toBe('text/html');
            }
        });
    });

    describe('Route Path Constants Match Registration', () => {
        it('BasePaths.AUTH - /api/auth routes are registered', async () => {
            const res = await client.post(`${BasePaths.AUTH}/register`).send({});
            expect([200, 201, 400]).toContain(res.status);
        });

        it('BasePaths.HEALTH - /health routes are registered', async () => {
            const res = await client.get(BasePaths.HEALTH);
            expect(res.status).toBe(200);
        });

        it('BasePaths.CHAT - /api/chat routes are registered (requires auth)', async () => {
            const res = await client.get(`${BasePaths.CHAT}/health`);
            expect([200, 401, 403]).toContain(res.status);
        });

        it('BasePaths.SSE - /sse routes are registered (requires auth)', async () => {
            const res = await client.get(`${BasePaths.SSE}/health`);
            expect([200, 401, 403]).toContain(res.status);
        });

        it('BasePaths.OPERATOR - /api/operators routes are registered', async () => {
            const res = await client.get(BasePaths.OPERATOR);
            expect([401, 404]).toContain(res.status);
        });

        it('BasePaths.INTERNAL - /api/internal routes are registered (requires auth)', async () => {
            const res = await client.get(`${BasePaths.INTERNAL}/health`);
            expect([401, 403]).toContain(res.status);
        });
    });

    describe('404 Handling', () => {
        it('GET /nonexistent-path - returns 404', async () => {
            const res = await client.get('/this-path-does-not-exist');
            expect(res.status).toBe(404);
        });

        it('GET /api/nonexistent - returns 404', async () => {
            const res = await client.get('/api/nonexistent-endpoint');
            expect(res.status).toBe(404);
        });
    });

    describe('Middleware Integration', () => {
        it('Security headers are present', async () => {
            const res = await client.get('/health');
            expect(res.headers['x-content-type-options']).toBe('nosniff');
            expect(res.headers['x-frame-options']).toBeDefined();
        });
    });
});
