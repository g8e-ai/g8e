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

import { logger } from '../../utils/logger.js';
import { BEARER_PREFIX, ApiKeyError, DownloadKeyType, ApiKeyPermission } from '../../constants/auth.js';
import { isValidTokenFormat as isValidDeviceLinkFormat } from './device_link_service.js';
import { DeviceLinkData } from '../../models/auth_models.js';
import { KVKey } from '../../constants/kv_keys.js';

class DownloadAuthService {
    /**
     * @param {object} options
     * @param {object} options.cacheAsideService - CacheAsideService instance
     * @param {object} options.userService        - UserService instance
     * @param {object} options.apiKeyService      - ApiKeyService instance
     */
    constructor({ cacheAsideService, userService, apiKeyService }) {
        if (!cacheAsideService) throw new Error('cacheAsideService is required');
        if (!userService)       throw new Error('userService is required');
        if (!apiKeyService)     throw new Error('apiKeyService is required');
        this._cache_aside = cacheAsideService;
        this.userService   = userService;
        this.apiKeyService = apiKeyService;
    }

    /**
     * Validate an Authorization Bearer token for operator binary download/checksum endpoints.
     *
     * Token types accepted, in order of prefix priority:
     *   g8e_ (download) — user's g8e_key stored on the user document
     *   dlk_               — device-link token (validated via KV, not consumed)
     *   g8e_ (operator) — operator API key with operator:download permission or operator_id
     *
     * @param {import('express').Request} req
     * @param {object} [options]
     * @param {boolean} [options.allowDlt=true] - Whether to allow device-link tokens (dlk_*)
     * @returns {Promise<
     *   { success: true,  userId: string, organizationId: string|null, operatorId: string|null, keyType: string } |
     *   { success: false, status: number, error: string }
     * >}
     */
    async validate(req, options = {}) {
        const { allowDlt = true } = options;
        const authHeader = req.headers.authorization;

        if (!authHeader || !authHeader.startsWith(BEARER_PREFIX)) {
            return {
                success: false,
                status:  401,
                error:   'Authentication required. Provide token in Authorization header: Bearer <token>',
            };
        }

        const token = authHeader.substring(BEARER_PREFIX.length);

        if (token.startsWith('dlk_')) {
            if (!allowDlt) {
                logger.warn('[DOWNLOAD-AUTH] Device-link token rejected (allowDlt=false)', {
                    token_prefix: token.substring(0, 20) + '...',
                    ip:           req.ip,
                });
                return {
                    success: false,
                    status:  403,
                    error:   'Device-link tokens are not permitted for this resource.',
                };
            }
            return this._validateDeviceLinkToken(req, token);
        }

        return this._validateApiKey(req, token);
    }

    async _validateDeviceLinkToken(req, token) {
        if (!isValidDeviceLinkFormat(token)) {
            logger.warn('[DOWNLOAD-AUTH] Invalid device-link token format rejected', {
                token_prefix: token.substring(0, 20) + '...',
                ip:           req.ip,
            });
            return { success: false, status: 401, error: ApiKeyError.AUTH_FAILED };
        }

        const linkDataRaw = await this._cache_aside.kvGetJson(KVKey.deviceLink(token));

        if (!linkDataRaw) {
            logger.warn('[DOWNLOAD-AUTH] Device-link token not found', {
                token_prefix: token.substring(0, 20) + '...',
            });
            return { success: false, status: 401, error: ApiKeyError.AUTH_FAILED };
        }

        const linkData = DeviceLinkData.fromKV(linkDataRaw);

        logger.info('[DOWNLOAD-AUTH] Device-link token validated', {
            token_prefix: token.substring(0, 20) + '...',
            user_id:      linkData.user_id,
            operator_id:  linkData.operator_id,
        });

        return {
            success:        true,
            userId:         linkData.user_id,
            organizationId: linkData.organization_id,
            operatorId:     linkData.operator_id,
            keyType:        DownloadKeyType.DEVICE_LINK,
        };
    }

    async _validateApiKey(req, token) {
        const user = await this.userService.getUserByApiKey(token);

        if (user) {
            logger.info('[DOWNLOAD-AUTH] Download API key validated via user document', {
                token_prefix: token.substring(0, 20) + '...',
                user_id:      user.id,
            });
            return {
                success:        true,
                userId:         user.id,
                organizationId: user.organization_id,
                operatorId:     null,
                keyType:        DownloadKeyType.USER_DOWNLOAD,
            };
        }

        const keyValidation = await this.apiKeyService.validateKey(token);

        if (!keyValidation.success || !keyValidation.data) {
            logger.warn('[DOWNLOAD-AUTH] Invalid API key', {
                token_prefix: token.substring(0, 20) + '...',
            });
            return { success: false, status: 401, error: ApiKeyError.INVALID_OR_EXPIRED };
        }

        const keyData         = keyValidation.data;
        const hasDownloadPerm = keyData.permissions.includes(ApiKeyPermission.OPERATOR_DOWNLOAD);
        const isOperatorKey   = !!keyData.operator_id;

        if (!hasDownloadPerm && !isOperatorKey) {
            logger.warn('[DOWNLOAD-AUTH] API key lacks download permission', {
                token_prefix:    token.substring(0, 10) + '...',
                permissions:     keyData.permissions,
                has_operator_id: isOperatorKey,
            });
            return { success: false, status: 403, error: ApiKeyError.NO_DOWNLOAD_PERMISSION };
        }

        return {
            success:        true,
            userId:         keyData.user_id,
            organizationId: keyData.organization_id,
            operatorId:     keyData.operator_id,
            keyType:        isOperatorKey ? DownloadKeyType.OPERATOR_SPECIFIC : DownloadKeyType.USER_DOWNLOAD,
        };
    }
}

export { DownloadAuthService };
