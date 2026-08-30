import express from 'express';
import request from 'supertest';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../../../utils/logger.js', () => ({
    logger: { error: vi.fn(), warn: vi.fn() },
}));
vi.mock('../../../constants/service_config.js', () => ({
    DEFAULT_DOCS_DIR: '/unused',
}));

import { DocsPaths } from '../../../constants/api_paths.js';
import { createDocsRouter } from '../../../routes/platform/docs_routes.js';

describe('docs route rate limiting', () => {
    it.each([DocsPaths.TREE, DocsPaths.FILE, '/future-route'])('applies the API rate limiter to %s', async (routePath) => {
        const apiRateLimiter = vi.fn((_req, res) => res.status(429).json({ error: 'rate limited' }));
        const optionalAuth = vi.fn((_req, _res, next) => next());
        const router = createDocsRouter({
            config: {},
            authMiddleware: { optionalAuth },
            rateLimiters: { apiRateLimiter },
        });
        const app = express().use(router);

        await request(app).get(routePath).query({ path: 'README.md' }).expect(429);

        expect(apiRateLimiter).toHaveBeenCalledOnce();
        expect(optionalAuth).not.toHaveBeenCalled();
    });
});
