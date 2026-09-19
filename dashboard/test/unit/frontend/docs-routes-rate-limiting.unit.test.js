// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import request from 'supertest';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../../../utils/logger.js', () => ({
    logger: { error: vi.fn(), warn: vi.fn() },
}));
vi.mock('../../../constants/service_config.js', () => ({
    DEFAULT_DOCS_DIR: '/unused',
}));

const { docsRateLimiterMock } = vi.hoisted(() => ({
    docsRateLimiterMock: vi.fn((_req, res) => res.status(429).json({ error: 'rate limited' })),
}));

vi.mock('../../../middleware/rate-limit.js', () => ({
    buildApiRateLimiter: () => docsRateLimiterMock,
}));

import { DocsPaths } from '../../../constants/api_paths.js';
import { createDocsRouter } from '../../../routes/platform/docs_routes.js';

describe('docs route rate limiting', () => {
    it.each([DocsPaths.TREE, DocsPaths.FILE])('applies the docs rate limiter to %s', async (routePath) => {
        docsRateLimiterMock.mockClear();
        const optionalAuth = vi.fn((_req, _res, next) => next());
        const router = createDocsRouter({
            config: {},
            authMiddleware: { optionalAuth },
        });
        const app = express().use(router);

        await request(app).get(routePath).query({ path: 'README.md' }).expect(429);

        expect(docsRateLimiterMock).toHaveBeenCalledOnce();
        expect(optionalAuth).not.toHaveBeenCalled();
    });
});
