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

import crypto from 'crypto';
import { now } from '../../models/base.js';
import { ApiKeyDocument } from '../../models/auth_models.js';
import { logger } from '../../utils/logger.js';
import { ApiKeyStatus, ApiKeyError, API_KEY_LOG_PREFIX_LENGTH } from '../../constants/auth.js';
import { API_KEY_PREFIX } from '../../constants/operator_defaults.js';

/**
 * ApiKeyService - Domain Layer (Orchestration) for API Keys.
 * Handles validation, generation, and orchestration of API key lifecycle.
 * Adheres to Domain Layer principles from developer.md.
 */
class ApiKeyService {
    /**
     * @param {Object} options
     * @param {Object} options.apiKeyDataService - ApiKeyDataService instance (Data Layer)
     */
    constructor({ apiKeyDataService }) {
        if (!apiKeyDataService) throw new Error('ApiKeyService requires apiKeyDataService');
        this._data = apiKeyDataService;
        logger.info('[API-KEY-SERVICE] Domain service initialized');
    }

    /**
     * Generate a new raw API key.
     * @returns {string}
     */
    generateRawKey() {
        return `${API_KEY_PREFIX}${crypto.randomBytes(32).toString('hex')}`;
    }

    /**
     * Validate a raw API key.
     * @param {string} apiKey
     * @returns {Promise<{success: boolean, data?: ApiKeyDocument, error?: string}>}
     */
    async validateKey(apiKey) {
        try {
            if (!apiKey) {
                return { success: false, error: 'API key is required' };
            }
            if (!apiKey.startsWith(API_KEY_PREFIX)) {
                return { success: false, error: ApiKeyError.INVALID_KEY_FORMAT };
            }

            const docId = this._data.makeDocId(apiKey);
            const doc = await this._data.getKey(docId);

            if (!doc) {
                logger.warn('[API-KEY-SERVICE] API key not found', { 
                    api_key_prefix: this._getLogPrefix(apiKey) 
                });
                return { success: false, error: 'API key not found' };
            }

            if (doc.status !== ApiKeyStatus.ACTIVE) {
                logger.warn('[API-KEY-SERVICE] API key is not active', {
                    api_key_prefix: this._getLogPrefix(apiKey),
                    status: doc.status
                });
                return { success: false, error: `API key is ${doc.status}` };
            }

            if (doc.expires_at && doc.expires_at < now()) {
                logger.warn('[API-KEY-SERVICE] API key has expired', {
                    api_key_prefix: this._getLogPrefix(apiKey),
                    expires_at: doc.expires_at
                });
                return { success: false, error: 'API key has expired' };
            }

            return { success: true, data: doc };
        } catch (error) {
            logger.error('[API-KEY-SERVICE] Validation failed', { error: error.message });
            return { success: false, error: 'Internal validation error' };
        }
    }

    /**
     * Issue (create and store) a new API key.
     * @param {string} apiKey
     * @param {Object} keyParams
     * @returns {Promise<{success: boolean, error?: string}>}
     */
    async issueKey(apiKey, keyParams) {
        try {
            const docId = this._data.makeDocId(apiKey);
            const doc = ApiKeyDocument.parse({
                ...keyParams,
                created_at: keyParams.created_at || now(),
                status: keyParams.status || ApiKeyStatus.ACTIVE
            });

            await this._data.createKey(docId, doc);

            logger.info('[API-KEY-SERVICE] API key issued', {
                api_key_prefix: this._getLogPrefix(apiKey),
                user_id: doc.user_id
            });

            return { success: true };
        } catch (error) {
            logger.error('[API-KEY-SERVICE] Failed to issue API key', { error: error.message });
            return { success: false, error: error.message };
        }
    }

    /**
     * Update the last used timestamp of a key.
     * @param {string} apiKey
     */
    async recordUsage(apiKey) {
        try {
            const docId = this._data.makeDocId(apiKey);
            await this._data.updateKey(docId, { last_used_at: now() });
        } catch (error) {
            logger.warn('[API-KEY-SERVICE] Failed to record usage', { error: error.message });
        }
    }

    /**
     * Revoke (delete) an API key.
     * @param {string} apiKey
     * @returns {Promise<{success: boolean}>}
     */
    async revokeKey(apiKey) {
        try {
            const docId = this._data.makeDocId(apiKey);
            await this._data.deleteKey(docId);
            return { success: true };
        } catch (error) {
            logger.error('[API-KEY-SERVICE] Failed to revoke key', { error: error.message });
            return { success: false };
        }
    }

    /**
     * Legacy compatibility for storeApiKey (matches previous signature).
     * @deprecated Use issueKey instead.
     */
    async storeApiKey(apiKey, keyData) {
        return this.issueKey(apiKey, keyData);
    }

    _getLogPrefix(apiKey) {
        return apiKey.substring(0, API_KEY_LOG_PREFIX_LENGTH) + '...';
    }
}

export { ApiKeyService };
