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
import { createApiKeyMiddleware } from '@vsod/middleware/api_key_auth.js';
import { ApiKeyError, BEARER_PREFIX } from '@vsod/constants/auth.js';

describe('ApiKeyAuth Middleware', () => {
    let apiKeyService;
    let userService;
    let middleware;
    let req;
    let res;
    let next;

    beforeEach(() => {
        apiKeyService = {
            validateApiKey: vi.fn(),
            updateLastUsed: vi.fn().mockResolvedValue()
        };
        userService = {
            getUser: vi.fn()
        };
        middleware = createApiKeyMiddleware({ apiKeyService, userService });
        
        req = {
            headers: {},
            path: '/api/test',
            method: 'GET',
            ip: '127.0.0.1'
        };
        res = {
            status: vi.fn().mockReturnThis(),
            json: vi.fn().mockReturnThis()
        };
        next = vi.fn();
    });

    describe('requireApiKey', () => {
        it('should return 401 if Authorization header is missing', async () => {
            await middleware.requireApiKey(req, res, next);

            expect(res.status).toHaveBeenCalledWith(401);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: ApiKeyError.REQUIRED
            }));
            expect(next).not.toHaveBeenCalled();
        });

        it('should return 401 if Authorization header does not start with Bearer', async () => {
            req.headers.authorization = 'Basic dGVzdDp0ZXN0';
            await middleware.requireApiKey(req, res, next);

            expect(res.status).toHaveBeenCalledWith(401);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: ApiKeyError.INVALID_FORMAT
            }));
            expect(next).not.toHaveBeenCalled();
        });

        it('should return 401 if API key is empty after Bearer prefix', async () => {
            req.headers.authorization = BEARER_PREFIX;
            await middleware.requireApiKey(req, res, next);

            expect(res.status).toHaveBeenCalledWith(401);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: ApiKeyError.REQUIRED
            }));
        });

        it('should return 401 if API key validation fails', async () => {
            req.headers.authorization = `${BEARER_PREFIX}invalid-key`;
            apiKeyService.validateApiKey.mockResolvedValue({ success: false, error: 'Invalid' });

            await middleware.requireApiKey(req, res, next);

            expect(res.status).toHaveBeenCalledWith(401);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: ApiKeyError.INVALID
            }));
        });

        it('should return 401 if API key data is missing user_id', async () => {
            req.headers.authorization = `${BEARER_PREFIX}valid-key`;
            apiKeyService.validateApiKey.mockResolvedValue({
                success: true,
                data: { organization_id: 'org-1' }
            });

            await middleware.requireApiKey(req, res, next);

            expect(res.status).toHaveBeenCalledWith(401);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: ApiKeyError.INVALID
            }));
        });

        it('should return 401 if user is not found', async () => {
            req.headers.authorization = `${BEARER_PREFIX}valid-key`;
            apiKeyService.validateApiKey.mockResolvedValue({
                success: true,
                data: { user_id: 'user-1' }
            });
            userService.getUser.mockResolvedValue(null);

            await middleware.requireApiKey(req, res, next);

            expect(res.status).toHaveBeenCalledWith(401);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: ApiKeyError.USER_NOT_FOUND
            }));
        });

        it('should attach user and key data and call next on success', async () => {
            const keyData = { user_id: 'user-1', organization_id: 'org-1' };
            const user = { id: 'user-1', organization_id: 'org-1' };
            req.headers.authorization = `${BEARER_PREFIX}valid-key`;
            apiKeyService.validateApiKey.mockResolvedValue({ success: true, data: keyData });
            userService.getUser.mockResolvedValue(user);

            await middleware.requireApiKey(req, res, next);

            expect(req.apiKey).toBe('valid-key');
            expect(req.userId).toBe('user-1');
            expect(req.user).toBe(user);
            expect(req.apiKeyData).toBe(keyData);
            expect(apiKeyService.updateLastUsed).toHaveBeenCalledWith('valid-key');
            expect(next).toHaveBeenCalled();
        });

        it('should return 500 on unexpected errors', async () => {
            req.headers.authorization = `${BEARER_PREFIX}valid-key`;
            apiKeyService.validateApiKey.mockRejectedValue(new Error('Unexpected'));

            await middleware.requireApiKey(req, res, next);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: ApiKeyError.INTERNAL_ERROR
            }));
        });
    });

    describe('optionalApiKey', () => {
        it('should call next if Authorization header is missing', async () => {
            await middleware.optionalApiKey(req, res, next);
            expect(next).toHaveBeenCalled();
        });

        it('should validate if Authorization header is present', async () => {
            req.headers.authorization = `${BEARER_PREFIX}valid-key`;
            apiKeyService.validateApiKey.mockResolvedValue({ success: false });

            await middleware.optionalApiKey(req, res, next);

            expect(res.status).toHaveBeenCalledWith(401);
        });
    });
});
