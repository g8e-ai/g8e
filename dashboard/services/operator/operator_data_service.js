// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { logger } from '../../utils/logger.js';
import { Collections } from '../../constants/collections.js';
import { OperatorDocument } from '../../models/operator_model.js';

/**
 * OperatorDataService (Data Layer)
 * Pure CRUD operations for the Operators collection.
 * No business logic, no orchestration, no side effects.
 */
export class OperatorDataService {
    constructor({ cacheAside }) {
        if (!cacheAside) throw new Error('cacheAside is required');
        this.cacheAside = cacheAside;
        this.collectionName = Collections.OPERATORS;
    }

    async getOperator(operatorId) {
        const data = await this.cacheAside.getDocument(this.collectionName, operatorId);
        return data ? OperatorDocument.fromDB(data) : null;
    }

    async getOperatorFresh(operatorId) {
        await this.cacheAside.evictDocument(this.collectionName, operatorId);
        return await this.getOperator(operatorId);
    }

    async queryOperators(filters) {
        const data = await this.cacheAside.queryDocuments(this.collectionName, filters);
        return data || [];
    }

    async createOperator(operatorId, operatorDoc) {
        return await this.cacheAside.createDocument(this.collectionName, operatorId, operatorDoc);
    }

    async updateOperator(operatorId, updateData) {
        return await this.cacheAside.updateDocument(this.collectionName, operatorId, updateData);
    }

    async deleteOperator(operatorId) {
        return await this.cacheAside.deleteDocument(this.collectionName, operatorId);
    }
}
