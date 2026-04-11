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

import { ErrorResponse } from '../models/response_models.js';
import { logger } from '../utils/logger.js';
import { BEARER_PREFIX, API_KEY_LOG_PREFIX_LENGTH, ApiKeyError } from '../constants/auth.js';

export function createApiKeyMiddleware({ apiKeyService, userService }) {
    /**
     * Require valid API key in Authorization header
     */
    const requireApiKey = async (req, res, next) => {
        try {
            const authHeader = req.headers.authorization;
            
            if (!authHeader) {
                logger.warn('[API-KEY-AUTH] Missing Authorization header', {
                    path: req.path,
                    method: req.method,
                    ip: req.ip
                });
                return res.status(401).json(new ErrorResponse({
                    error: ApiKeyError.REQUIRED
                }).forWire());
            }

            if (!authHeader.startsWith(BEARER_PREFIX)) {
                logger.warn('[API-KEY-AUTH] Invalid Authorization header format', {
                    path: req.path,
                    method: req.method,
                    ip: req.ip
                });
                return res.status(401).json(new ErrorResponse({
                    error: ApiKeyError.INVALID_FORMAT
                }).forWire());
            }

            const apiKey = authHeader.substring(BEARER_PREFIX.length);

            if (!apiKey) {
                return res.status(401).json(new ErrorResponse({
                    error: ApiKeyError.REQUIRED
                }).forWire());
            }

            const validation = await apiKeyService.validateApiKey(apiKey);

            if (!validation.success) {
                logger.warn('[API-KEY-AUTH] API key validation failed', {
                    path: req.path,
                    method: req.method,
                    ip: req.ip,
                    error: validation.error,
                    api_key_prefix: apiKey.substring(0, API_KEY_LOG_PREFIX_LENGTH) + '...'
                });
                return res.status(401).json(new ErrorResponse({
                    error: ApiKeyError.INVALID
                }).forWire());
            }

            const keyData = validation.data;
            const userId = keyData.user_id;

            if (!userId) {
                logger.error('[API-KEY-AUTH] API key missing user_id', {
                    api_key_prefix: apiKey.substring(0, API_KEY_LOG_PREFIX_LENGTH) + '...'
                });
                return res.status(401).json(new ErrorResponse({
                    error: ApiKeyError.INVALID
                }).forWire());
            }

            const user = await userService.getUser(userId);

            if (!user) {
                logger.error('[API-KEY-AUTH] User not found for API key', {
                    user_id: userId,
                    api_key_prefix: apiKey.substring(0, API_KEY_LOG_PREFIX_LENGTH) + '...'
                });
                return res.status(401).json(new ErrorResponse({
                    error: ApiKeyError.USER_NOT_FOUND
                }).forWire());
            }

            req.apiKey = apiKey;
            req.userId = userId;
            req.organizationId = user.organization_id || keyData.organization_id;
            req.apiKeyData = keyData;
            req.user = user;

            logger.info('[API-KEY-AUTH] API key authenticated successfully', {
                user_id: userId,
                path: req.path,
                method: req.method,
                api_key_prefix: apiKey.substring(0, API_KEY_LOG_PREFIX_LENGTH) + '...'
            });

            apiKeyService.updateLastUsed(apiKey).catch(err => {
                logger.warn('[API-KEY-AUTH] Failed to update last_used_at', { error: err.message });
            });

            next();
        } catch (error) {
            logger.error('[API-KEY-AUTH] Unexpected error during API key authentication', {
                error: error.message,
                stack: error.stack,
                path: req.path
            });
            return res.status(500).json(new ErrorResponse({
                error: ApiKeyError.INTERNAL_ERROR
            }).forWire());
        }
    };

    /**
     * Optional authentication - attaches session if present but doesn't require it
     */
    const optionalApiKey = async (req, res, next) => {
        const authHeader = req.headers.authorization;
        
        if (!authHeader || !authHeader.startsWith(BEARER_PREFIX)) {
            return next();
        }

        return requireApiKey(req, res, next);
    };

    return {
        requireApiKey,
        optionalApiKey
    };
}
