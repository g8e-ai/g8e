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
import { Collections } from '../../constants/collections.js';

/**
 * InvestigationService (Data Layer)
 * Pure CRUD operations for the Investigations collection via CacheAside.
 */
export class InvestigationService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     */
    constructor({ cacheAsideService }) {
        if (!cacheAsideService) throw new Error('cacheAsideService is required');
        this._cacheAside = cacheAsideService;
        this.collectionName = Collections.INVESTIGATIONS;
    }

    /**
     * Get a single investigation by ID.
     * @param {string} investigationId 
     * @returns {Promise<Object|null>}
     */
    async getInvestigation(investigationId) {
        return await this._cacheAside.getDocument(this.collectionName, investigationId);
    }

    /**
     * Query investigations based on filters.
     * @param {Array} filters 
     * @param {number} limit 
     * @returns {Promise<Array>}
     */
    async queryInvestigations(filters = [], limit = 20) {
        const results = await this._cacheAside.queryDocuments(this.collectionName, filters, limit);
        return Array.isArray(results) ? results : [];
    }

    /**
     * Get investigations for a specific user.
     * @param {string} userId 
     * @param {number} limit 
     * @returns {Promise<Array>}
     */
    async getInvestigationsByUserId(userId, limit = 20) {
        const filters = [{ field: 'user_id', operator: '==', value: userId }];
        return await this.queryInvestigations(filters, limit);
    }
}
