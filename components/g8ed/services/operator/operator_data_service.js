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
import { OperatorStatus } from '../../constants/operator.js';
import { OperatorDocument } from '../../models/operator_model.js';

/**
 * OperatorDataService (Data Layer)
 * Pure CRUD operations for the Operators collection.
 * No business logic, no orchestration, no side effects.
 */
export class OperatorDataService {
    constructor({ cacheAsideService }) {
        if (!cacheAsideService) throw new Error('cacheAsideService is required');
        this._cache_aside = cacheAsideService;
        this.collectionName = Collections.OPERATORS;
    }

    async getOperator(operatorId) {
        const data = await this._cache_aside.getDocument(this.collectionName, operatorId);
        return data ? OperatorDocument.fromDB(data) : null;
    }

    async getOperatorFresh(operatorId) {
        await this._cache_aside.evictDocument(this.collectionName, operatorId);
        return await this.getOperator(operatorId);
    }

    async queryOperators(filters) {
        const data = await this._cache_aside.queryDocuments(this.collectionName, filters);
        return data || [];
    }

    async queryOperatorsFresh(filters) {
        const data = await this._cache_aside.queryDocuments(this.collectionName, filters, null, true);
        return data || [];
    }

    /**
     * Query operators and filter out TERMINATED ones.
     * Use this for all business logic that should only operate on "live" or "available" slots.
     */
    async queryListedOperators(filters = [], options = {}) {
        const operators = options.fresh 
            ? await this.queryOperatorsFresh(filters)
            : await this.queryOperators(filters);
            
        // Centralized TERMINATED filter
        // Status can be null if not yet initialized, but we only exclude explicit TERMINATED
        return operators.filter(op => op.status !== 'TERMINATED');
    }

    async createOperator(operatorId, operatorDoc) {
        return await this._cache_aside.createDocument(this.collectionName, operatorId, operatorDoc);
    }

    async updateOperator(operatorId, updateData) {
        return await this._cache_aside.updateDocument(this.collectionName, operatorId, updateData);
    }

    async deleteOperator(operatorId) {
        return await this._cache_aside.deleteDocument(this.collectionName, operatorId);
    }
}
