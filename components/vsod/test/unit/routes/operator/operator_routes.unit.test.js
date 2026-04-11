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
import { createOperatorRouter } from '@vsod/routes/operator/operator_routes.js';
import { OperatorPaths } from '@vsod/constants/api_paths.js';
import { SystemHealth } from '@vsod/constants/ai.js';
import { OperatorRouteError } from '@vsod/constants/service_config.js';

describe('OperatorRoutes Unit Tests', () => {
    let app;
    let mockSettings;
    let mockOperatorDownloadService;
    let mockDownloadAuthService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockSettings = {};
        mockOperatorDownloadService = {
            getPlatformAvailability: vi.fn(),
            getBinary: vi.fn()
        };
        mockDownloadAuthService = {
            validate: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        const router = createOperatorRouter({
            services: {
                operatorDownloadService: mockOperatorDownloadService,
                downloadAuthService: mockDownloadAuthService
            },

            authorizationMiddleware: mockAuthorizationMiddleware
        });

        app = express();
        app.use(express.json());
        app.use('/api/operator', router);
    });

    describe(`GET ${OperatorPaths.HEALTH}`, () => {
        it('returns healthy status when all platforms are available', async () => {
            mockOperatorDownloadService.getPlatformAvailability.mockResolvedValue({
                'linux/amd64': { available: true, size: 100 },
                'linux/arm64': { available: true, size: 100 }
            });

            const res = await request(app).get('/api/operator/health');

            expect(res.status).toBe(200);
            expect(res.body.status).toBe(SystemHealth.HEALTHY);
            expect(res.body.platforms).toHaveLength(2);
        });

        it('returns degraded status when some platforms are missing', async () => {
            mockOperatorDownloadService.getPlatformAvailability.mockResolvedValue({
                'linux/amd64': { available: true, size: 100 },
                'linux/arm64': { available: false, size: 0 }
            });

            const res = await request(app).get('/api/operator/health');

            expect(res.status).toBe(200);
            expect(res.body.status).toBe(SystemHealth.DEGRADED);
        });
    });

    describe(`GET ${OperatorPaths.DOWNLOAD}`, () => {
        it('successfully downloads a binary', async () => {
            mockDownloadAuthService.validate.mockResolvedValue({
                success: true,
                userId: 'user-1',
                organizationId: 'org-1',
                keyType: 'OPERATOR'
            });
            mockOperatorDownloadService.getBinary.mockResolvedValue(Buffer.from('binary-content'));

            const res = await request(app).get('/api/operator/download/linux/amd64');

            expect(res.status).toBe(200);
            expect(res.header['content-type']).toBe('application/octet-stream');
            expect(res.body.toString()).toBe('binary-content');
        });

        it('returns 400 for unsupported OS', async () => {
            const res = await request(app).get('/api/operator/download/windows/amd64');

            expect(res.status).toBe(400);
            expect(res.body.error).toBe(OperatorRouteError.UNSUPPORTED_OS);
        });

        it('returns 401 when authentication fails', async () => {
            mockDownloadAuthService.validate.mockResolvedValue({
                success: false,
                status: 401,
                error: 'Invalid token'
            });

            const res = await request(app).get('/api/operator/download/linux/amd64');

            expect(res.status).toBe(401);
            expect(res.body.error).toBe('Invalid token');
        });

        it('returns 503 if binary is not available', async () => {
            mockDownloadAuthService.validate.mockResolvedValue({ success: true });
            mockOperatorDownloadService.getBinary.mockRejectedValue(new Error('Operator binary not available for linux/amd64'));

            const res = await request(app).get('/api/operator/download/linux/amd64');

            expect(res.status).toBe(503);
            expect(res.body.error).toBe(OperatorRouteError.BINARY_NOT_AVAILABLE);
        });
    });

    describe(`GET ${OperatorPaths.DOWNLOAD_SHA256}`, () => {
        it('successfully returns sha256 checksum', async () => {
            mockDownloadAuthService.validate.mockResolvedValue({ success: true });
            mockOperatorDownloadService.getBinary.mockResolvedValue(Buffer.from('binary-content'));

            const res = await request(app).get('/api/operator/download/linux/amd64/sha256');

            expect(res.status).toBe(200);
            expect(res.header['content-type']).toContain('text/plain');
            // Content should contain a hash and the filename
            expect(res.text).toMatch(/^[a-f0-9]{64}\s+g8e.operator\n$/);
        });
    });
});
