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

/**
 * BoundSessionsService
 *
 * Owns the full lifecycle of operator↔web session bindings.
 *
 * Binding state is stored bidirectionally in g8es KV (fast lookup path):
 *   sessionBindOperators(operatorSessionId) → webSessionId  (STRING)
 *   sessionWebBind(webSessionId)            → {operatorSessionId, ...}  (SET)
 *
 * A BoundSessionsDocument is persisted to g8es document store (bound_sessions
 * collection) for durability and audit. The document id equals the web_session_id.
 *
 * All persistence and KV operations flow through CacheAsideService.
 */

import { logger } from '../../utils/logger.js';
import { now } from '../../models/base.js';
import { BoundSessionsDocument } from '../../models/session_binding_model.js';
import { BoundOperatorContext } from '../../models/request_models.js';
import { KVKey } from '../../constants/kv_keys.js';
import { Collections } from '../../constants/collections.js';

export class BoundSessionsService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     * @param {Object} options.operatorService - OperatorService instance
     */
    constructor({ cacheAsideService, operatorService } = {}) {
        if (!cacheAsideService) throw new Error('cacheAsideService is required');
        if (!operatorService) throw new Error('operatorService is required');

        this._cache_aside = cacheAsideService;
        this.operatorService = operatorService;
        this.collection = Collections.BOUND_SESSIONS;
    }

    /**
     * Bind an operator session to a web session.
     *
     * Writes bidirectional KV entries:
     *   sessionBindOperators(operatorSessionId) → webSessionId  (STRING)
     *   sessionWebBind(webSessionId)            → operatorSessionId  (SET member)
     *
     * Creates or updates the BoundSessionsDocument in g8es document store.
     *
     * @param {string} operatorSessionId
     * @param {string} webSessionId
     * @param {string} userId
     * @param {string} operatorId - The durable operator ID
     * @returns {Promise<void>}
     */
    async bind(operatorSessionId, webSessionId, userId, operatorId) {
        const BindOperatorsKey = KVKey.sessionBindOperators(operatorSessionId);
        const webBindKey = KVKey.sessionWebBind(webSessionId);

        await this._cache_aside.kvSet(BindOperatorsKey, webSessionId);
        await this._cache_aside.kvSadd(webBindKey, operatorSessionId);

        const existing = await this._cache_aside.getDocument(this.collection, webSessionId);
        if (existing) {
            await this._updateBindingDocument(webSessionId, operatorSessionId, operatorId, 'add', existing);
        } else {
            await this._createBindingDocument(webSessionId, userId, operatorSessionId, operatorId);
        }

        logger.info('[BOUND-SESSIONS] Operator bound to web session durability layer', {
            operatorSessionId: operatorSessionId.substring(0, 12) + '...',
            webSessionId: webSessionId.substring(0, 12) + '...',
            operatorId
        });
    }

    /**
     * Unbind an operator session from a web session.
     *
     * Deletes:
     *   sessionBindOperators(operatorSessionId)
     *   sessionWebBind(webSessionId) member: operatorSessionId
     *   (deletes SET key entirely when it becomes empty)
     *
     * Updates the BoundSessionsDocument in g8es document store.
     *
     * @param {string} operatorSessionId
     * @param {string} webSessionId
     * @param {string} operatorId - The durable operator ID
     * @returns {Promise<void>}
     */
    async unbind(operatorSessionId, webSessionId, operatorId) {
        const BindOperatorsKey = KVKey.sessionBindOperators(operatorSessionId);
        const webBindKey = KVKey.sessionWebBind(webSessionId);

        await this._cache_aside.kvDel(BindOperatorsKey);
        await this._cache_aside.kvSrem(webBindKey, operatorSessionId);

        const remaining = await this._cache_aside.kvScard(webBindKey);
        if (remaining === 0) {
            await this._cache_aside.kvDel(webBindKey);
        }

        const existing = await this._cache_aside.getDocument(this.collection, webSessionId);
        if (existing) {
            await this._updateBindingDocument(webSessionId, operatorSessionId, operatorId, 'remove', existing);
        }

        logger.info('[BOUND-SESSIONS] Operator unbound from web session durability layer', {
            operatorSessionId: operatorSessionId.substring(0, 12) + '...',
            webSessionId: webSessionId.substring(0, 12) + '...',
            operatorId
        });
    }

    /**
     * Get all operator session IDs bound to a web session.
     * Returns raw bind table members — liveness checks are the caller's responsibility.
     *
     * @param {string} webSessionId
     * @returns {Promise<string[]>}
     */
    async getBoundOperatorSessionIds(webSessionId) {
        const webBindKey = KVKey.sessionWebBind(webSessionId);
        const members = await this._cache_aside.kvSmembers(webBindKey);
        return members || [];
    }

    /**
     * Get the web session ID that an operator session is bound to.
     *
     * @param {string} operatorSessionId
     * @returns {Promise<string|null>}
     */
    async getWebSessionForOperator(operatorSessionId) {
        const BindOperatorsKey = KVKey.sessionBindOperators(operatorSessionId);
        const webSessionId = await this._cache_aside.kvGet(BindOperatorsKey);
        return webSessionId || null;
    }

    /**
     * Resolve all live bound operators for a web session.
     * Reads the binding document (one read) and fetches operator docs in parallel.
     *
     * @param {string} webSessionId
     * @returns {Promise<BoundOperatorContext[]>}
     */
    async resolveBoundOperators(webSessionId) {
        const bindingDoc = await this._cache_aside.getDocument(this.collection, webSessionId);

        if (!bindingDoc || bindingDoc.status !== 'active') {
            return [];
        }

        const operatorIds = bindingDoc.operator_ids || [];
        const operatorSessionIds = bindingDoc.operator_session_ids || [];

        if (operatorIds.length === 0) {
            return [];
        }

        const operators = await Promise.all(
            operatorIds.map(id => this.operatorService.getOperator(id))
        );

        const boundOperators = [];

        for (let i = 0; i < operatorIds.length; i++) {
            const operator = operators[i];
            if (!operator) {
                continue;
            }

            boundOperators.push(BoundOperatorContext.parse({
                operator_id: operatorIds[i],
                operator_session_id: operatorSessionIds[i],
                status: operator.status,
            }).forWire());
        }

        if (boundOperators.length > 0) {
            logger.info('[BOUND-SESSIONS] Resolved bound operators for web session', {
                webSessionId: webSessionId.substring(0, 12) + '...',
                count: boundOperators.length,
                operatorIds: boundOperators.map(op => op.operator_id)
            });
        }

        return boundOperators;
    }

    /**
     * Resolve all live bound operators for a user (for OAuth Client ID auth).
     * Scans all bound sessions for the user and aggregates bound operators.
     *
     * @param {string} userId
     * @returns {Promise<BoundOperatorContext[]>}
     */
    async resolveBoundOperatorsForUser(userId) {
        // Scan bound sessions collection for this user
        const pattern = `${this.collection}:*`;
        const keys = await this._cache_aside.kvKeys(pattern);
        
        const boundOperators = [];
        
        for (const key of keys) {
            const webSessionId = key.replace(`${this.collection}:`, '');
            const bindingDoc = await this._cache_aside.getDocument(this.collection, webSessionId);
            
            if (!bindingDoc || bindingDoc.status !== 'active' || bindingDoc.user_id !== userId) {
                continue;
            }

            const operatorIds = bindingDoc.operator_ids || [];
            const operatorSessionIds = bindingDoc.operator_session_ids || [];

            if (operatorIds.length === 0) {
                continue;
            }

            const operators = await Promise.all(
                operatorIds.map(id => this.operatorService.getOperator(id))
            );

            for (let i = 0; i < operatorIds.length; i++) {
                const operator = operators[i];
                if (!operator) {
                    continue;
                }

                boundOperators.push(BoundOperatorContext.parse({
                    operator_id: operatorIds[i],
                    operator_session_id: operatorSessionIds[i],
                    status: operator.status,
                }).forWire());
            }
        }

        if (boundOperators.length > 0) {
            logger.info('[BOUND-SESSIONS] Resolved bound operators for user (OAuth auth)', {
                userId,
                count: boundOperators.length,
                operatorIds: boundOperators.map(op => op.operator_id)
            });
        }

        return boundOperators;
    }

    async _createBindingDocument(webSessionId, userId, operatorSessionId, operatorId) {
        try {
            const doc = BoundSessionsDocument.parse({
                id: webSessionId,
                web_session_id: webSessionId,
                user_id: userId,
                operator_session_ids: [operatorSessionId],
                operator_ids: [operatorId],
                bound_at: now(),
                last_updated_at: now(),
                status: 'active'
            });
            await this._cache_aside.createDocument(this.collection, webSessionId, doc);
        } catch (err) {
            logger.error('[BOUND-SESSIONS] Failed to create binding document', {
                error: err.message,
                webSessionId: webSessionId.substring(0, 12) + '...',
            });
        }
    }

    async _updateBindingDocument(webSessionId, operatorSessionId, operatorId, operation, existing) {
        try {
            const currentSessionIds = Array.isArray(existing.operator_session_ids)
                ? existing.operator_session_ids
                : [];
            
            const currentOperatorIds = Array.isArray(existing.operator_ids)
                ? existing.operator_ids
                : [];

            let updatedSessionIds;
            let updatedOperatorIds;

            if (operation === 'add') {
                updatedSessionIds = [...new Set([...currentSessionIds, operatorSessionId])];
                updatedOperatorIds = [...new Set([...currentOperatorIds, operatorId])];
            } else {
                updatedSessionIds = currentSessionIds.filter(id => id !== operatorSessionId);
                updatedOperatorIds = currentOperatorIds.filter(id => id !== operatorId);
            }

            const status = updatedSessionIds.length > 0 ? 'active' : 'ended';

            await this._cache_aside.updateDocument(this.collection, webSessionId, {
                operator_session_ids: updatedSessionIds,
                operator_ids: updatedOperatorIds,
                last_updated_at: now(),
                status
            });
        } catch (err) {
            logger.error('[BOUND-SESSIONS] Failed to update binding document', {
                error: err.message,
                webSessionId: webSessionId.substring(0, 12) + '...',
                operation,
            });
        }
    }
}
