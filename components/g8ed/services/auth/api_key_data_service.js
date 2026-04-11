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
import { logger } from '../../utils/logger.js';
import { Collections } from '../../constants/collections.js';
import { ApiKeyDocument } from '../../models/auth_models.js';
import { API_KEY_HASH_ALGORITHM, API_KEY_HASH_LENGTH } from '../../constants/auth.js';

/**
 * ApiKeyDataService - Low-level CRUD for API Key documents.
 * Adheres to Data Layer (CRUD) principles from developer.md.
 */
class ApiKeyDataService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     */
    constructor({ cacheAsideService }) {
        if (!cacheAsideService) throw new Error('ApiKeyDataService requires cacheAsideService');
        this._cache_aside = cacheAsideService;
        this.collectionName = Collections.API_KEYS;
    }

    /**
     * Generate a deterministic document ID from a raw API key.
     * @param {string} apiKey
     * @returns {string}
     */
    makeDocId(apiKey) {
        return crypto
            .createHash(API_KEY_HASH_ALGORITHM)
            .update(apiKey)
            .digest('hex')
            .substring(0, API_KEY_HASH_LENGTH);
    }

    /**
     * Create an API key document.
     * @param {string} docId
     * @param {ApiKeyDocument} apiKeyDoc
     * @returns {Promise<void>}
     */
    async createKey(docId, apiKeyDoc) {
        const result = await this._cache_aside.createDocument(this.collectionName, docId, apiKeyDoc);
        if (!result.success) {
            throw new Error(result.error || 'Failed to create API key document');
        }
    }

    /**
     * Get an API key document by its deterministic ID.
     * @param {string} docId
     * @returns {Promise<ApiKeyDocument|null>}
     */
    async getKey(docId) {
        const data = await this._cache_aside.getDocument(this.collectionName, docId);
        return data ? ApiKeyDocument.parse(data) : null;
    }

    /**
     * Update an API key document.
     * @param {string} docId
     * @param {Object} updates
     * @returns {Promise<void>}
     */
    async updateKey(docId, updates) {
        const result = await this._cache_aside.updateDocument(this.collectionName, docId, updates);
        if (!result.success) {
            throw new Error(result.error || 'Failed to update API key document');
        }
    }

    /**
     * Delete an API key document.
     * @param {string} docId
     * @returns {Promise<void>}
     */
    async deleteKey(docId) {
        const result = await this._cache_aside.deleteDocument(this.collectionName, docId);
        if (!result.success && !result.notFound) {
            throw new Error(result.error || 'Failed to delete API key document');
        }
    }
}

export { ApiKeyDataService };
