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

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { createDocsRouter } from '@vsod/routes/platform/docs_routes.js';
import { DocsPaths } from '@vsod/constants/api_paths.js';
import fs from 'fs';
import path from 'path';

vi.mock('fs');

describe('Docs Routes [UNIT]', () => {
    let router;
    let mockConfig;
    let mockAuthMiddleware;

    beforeEach(() => {
        mockConfig = {
            docs_dir: '/tmp/docs',
            readme_path: '/tmp/README.md'
        };
        mockAuthMiddleware = {
            optionalAuth: vi.fn((req, res, next) => next())
        };

        router = createDocsRouter({
            services: {
                settingsService: mockConfig
            },

            authMiddleware: mockAuthMiddleware
        });

        vi.clearAllMocks();
    });

    const createMockReq = (overrides = {}) => ({
        query: {},
        params: {},
        userId: 'user_123',
        ...overrides
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    describe(`GET ${DocsPaths.TREE}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === DocsPaths.TREE);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return docs tree when directory exists', () => {
            vi.spyOn(fs, 'existsSync').mockReturnValue(true);
            vi.spyOn(fs, 'readdirSync').mockReturnValue([
                { name: 'file1.md', isFile: () => true, isDirectory: () => false },
                { name: 'subdir', isFile: () => false, isDirectory: () => true }
            ]);
            // Mock nested readdirSync for buildTree recursion
            fs.readdirSync.mockReturnValueOnce([
                { name: 'file1.md', isFile: () => true, isDirectory: () => false },
                { name: 'subdir', isFile: () => false, isDirectory: () => true }
            ]).mockReturnValueOnce([
                { name: 'subfile.md', isFile: () => true, isDirectory: () => false }
            ]);

            const req = createMockReq();
            const res = createMockRes();

            getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                tree: expect.any(Array)
            }));
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.tree[0].name).toBe('README.md'); // unshifted
            expect(responseData.tree[1].name).toBe('subdir');
        });

        it('should handle missing docs directory', () => {
            vi.spyOn(fs, 'existsSync').mockReturnValue(false);

            const req = createMockReq();
            const res = createMockRes();

            getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(503);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Docs directory not available'
            }));
        });
    });

    describe(`GET ${DocsPaths.FILE}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === DocsPaths.FILE);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return file content for valid markdown path', () => {
            vi.spyOn(fs, 'existsSync').mockReturnValue(true);
            vi.spyOn(fs, 'readFileSync').mockReturnValue('# Content');
            
            const req = createMockReq({ query: { path: 'file.md' } });
            const res = createMockRes();

            getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                content: '# Content'
            }));
        });

        it('should block path traversal', () => {
            const req = createMockReq({ query: { path: '../secret.txt' } });
            const res = createMockRes();

            getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(403);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Access denied'
            }));
        });

        it('should return 404 for non-existent file', () => {
            vi.spyOn(fs, 'existsSync').mockReturnValue(false);

            const req = createMockReq({ query: { path: 'missing.md' } });
            const res = createMockRes();

            getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(404);
        });
    });
});
