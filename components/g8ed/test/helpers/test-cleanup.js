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
 * Comprehensive Test Cleanup Utilities
 * 
 * Provides automatic cleanup for g8es KV and g8es document store resources
 * Use this in all integration tests to ensure proper isolation
 */

import { v4 as uuidv4 } from 'uuid';
import { Collections } from '../../constants/collections.js';
import { KVKey } from '../../constants/kv_keys.js';

export class TestCleanupHelper {
    constructor(cacheAsideOrKvClient, maybeCacheAside, options = {}) {
        // Handle multiple calling patterns:
        // new TestCleanupHelper(cacheAside, options)
        // new TestCleanupHelper(kvClient, cacheAside, options)
        if (maybeCacheAside && typeof maybeCacheAside.getDocument === 'function') {
            // Second pattern: kvClient, cacheAside, options
            this.kvClient = cacheAsideOrKvClient;
            this._cache_aside = maybeCacheAside;
            this.options = options;
        } else {
            // First pattern: cacheAside, options
            this.kvClient = null;
            this._cache_aside = cacheAsideOrKvClient;
            this.options = maybeCacheAside || {};
        }
        
        this.kvKeys = new Set();
        this.dbDocuments = new Map(); // collection -> Set of docIds
        this.operatorsCollection = this.options.operatorsCollection || Collections.OPERATORS;
        this.usersCollection = this.options.usersCollection || Collections.USERS;
    }

    /**
     * Generate UUID-based Operator ID and track for cleanup
     * 
     * Returns explicit session IDs with proper prefixes:
     * - operatorSessionId: For Operator daemon sessions (SessionType.OPERATOR)
     * - webSessionId: For browser web sessions (SessionType.WEB)
     * 
     * NOTE: This only generates IDs and tracks them for cleanup.
     * To create actual sessions, use webSessionService.createWebSession() or operatorSessionService.createOperatorSession()
     * followed by cleanup.trackWebSession() or cleanup.trackOperatorSession().
     */
    generateOperatorId(userId) {
        const opId = uuidv4();
        // Generate session IDs with proper prefix format matching production
        const operatorSessionId = `operator_session_${Date.now()}_${uuidv4()}`;
        const webSessionId = `web_session_${Date.now() + 1}_${uuidv4()}`;
        
        // Track Operator document and its API key (by operator_id, not doc ID)
        this.trackDBDoc(this.operatorsCollection, opId);
        this.trackApiKeyByOperatorId(opId);
        
        // Track all related g8es KV keys
        this.trackG8esKey(KVKey.doc(this.operatorsCollection, opId));
        this.trackG8esKey(KVKey.operatorFirstDeployed(opId));
        this.trackG8esKey(KVKey.operatorTrackedStatus(opId));
        this.trackG8esKey(KVKey.userOperators(userId));
        
        // Track session keys with unified format
        this.trackG8esKey(KVKey.operatorSessionKey(operatorSessionId));
        this.trackG8esKey(KVKey.webSessionKey(webSessionId));
        this.trackG8esKey(KVKey.userWebSessions(userId));
        
        return { opId, operatorSessionId, webSessionId };
    }
    
    /**
     * Create a real web session using the application SessionService and track for cleanup.
     * Use this in all integration tests instead of new SessionService().
     *
     * @param {object} sessionData - { user_id, user_data, api_key?, organization_id? }
     * @param {object} [requestContext] - { ip?, userAgent? }
     * @returns {Promise<object>} The created session document
     */
    async createTrackedWebSession(sessionData, requestContext = {}) {
        const { getTestServices } = await import('./test-services.js');
        const { webSessionService } = await getTestServices();
        const session = await webSessionService.createWebSession(sessionData, requestContext);
        this.trackWebSession(session.id, sessionData.user_id);
        return session;
    }

    /**
     * Create a real operator session using the application SessionService and track for cleanup.
     * Use this in all integration tests instead of new SessionService().
     *
     * @param {object} sessionData - { user_id, operator_id, user_data, api_key?, organization_id? }
     * @param {object} [requestContext] - { ip?, userAgent? }
     * @returns {Promise<object>} The created session document
     */
    async createTrackedOperatorSession(sessionData, requestContext = {}) {
        const { getTestServices } = await import('./test-services.js');
        const { operatorSessionService } = await getTestServices();
        const session = await operatorSessionService.createOperatorSession(sessionData, requestContext);
        this.trackOperatorSession(session.id, sessionData.user_id);
        return session;
    }
    
    /**
     * Track a g8es KV key for cleanup
     */
    trackG8esKey(key) {
        this.kvKeys.add(key);
    }

    /**
     * Track a DB document for cleanup
     */
    trackDBDoc(collection, docId) {
        if (!this.dbDocuments.has(collection)) {
            this.dbDocuments.set(collection, new Set());
        }
        this.dbDocuments.get(collection).add(docId);
    }

    /**
     * Track an Operator that was created outside of generateOperatorId
     */
    trackOperator(opId, userId) {
        this.trackDBDoc(this.operatorsCollection, opId);
        // Track API key by operator_id pattern (api_keys uses hashed key as doc ID, not operator_id)
        this.trackApiKeyByOperatorId(opId);
        this.trackG8esKey(KVKey.doc(this.operatorsCollection, opId));
        this.trackG8esKey(KVKey.operatorFirstDeployed(opId));
        this.trackG8esKey(KVKey.operatorTrackedStatus(opId));
        if (userId) {
            this.trackG8esKey(KVKey.userOperators(userId));
        }
    }

    /**
     * Track API key by operator_id for cleanup (API keys use hashed key as doc ID, not operator_id)
     * During cleanup, we query by operator_id to find and delete the actual document
     */
    trackApiKeyByOperatorId(operatorId) {
        if (!this.dbDocuments.has('api_keys-by-operator')) {
            this.dbDocuments.set('api_keys-by-operator', new Set());
        }
        this.dbDocuments.get('api_keys-by-operator').add(operatorId);
    }

    /**
     * Track operator_usage documents for cleanup
     * Operator usage documents are keyed by userId_period (e.g., "userId_2026-01")
     * 
     * @param {string} userId - The user ID
     * @param {string} period - The period (e.g., "2026-01")
     */
    trackOperatorUsage(userId, period) {
        const docId = `${userId}_${period}`;
        this.trackDBDoc(Collections.OPERATOR_USAGE, docId);
    }

    /**
     * Track console audit logs for cleanup by user_id
     * Console audit logs are created when admin/superadmin users access console endpoints
     * @param {string} userId - The user ID to clean up audit logs for
     */
    trackConsoleAudit(userId) {
        if (!this.dbDocuments.has('console_audit-pattern')) {
            this.dbDocuments.set('console_audit-pattern', new Set());
        }
        this.dbDocuments.get('console_audit-pattern').add(userId);
    }

    /**
     * Track an organization for cleanup
     * @param {string} orgId - The organization ID (document ID)
     */
    trackOrganization(orgId) {
        this.trackDBDoc(Collections.ORGANIZATIONS, orgId);
    }

    /**
     * Track a user and all related resources for cleanup
     * Use this after auth flow creates a user to ensure complete cleanup
     * 
     * This will clean up:
     * - User document in g8es document store
     * - All operators for this user (g8es document store + g8es KV)
     * - All API keys for this user (g8es document store + g8es KV)
     * - User's download API key (g8es KV)
     * - WebSession audit logs for this user
     * - Operator usage documents for this user
     * - User cache and session tracking keys (g8es KV)
     * - Login audit logs (if email provided)
     * 
     * @param {string} userId - The user ID
     * @param {string} email - The user's email (for login audit cleanup)
     */
    trackUser(userId, email = null) {
        // Track user sessions g8es KV key
        this.trackG8esKey(KVKey.userWebSessions(userId));
        
        // Track user operators g8es KV key
        this.trackG8esKey(KVKey.userOperators(userId));
        
        // Track user doc g8es KV key
        this.trackG8esKey(KVKey.doc(this.usersCollection, userId));
        
        // Track login audit logs if email provided
        if (email) {
            this.trackLoginAudit(email);
        }
        
        // Track user-pattern for querying and deleting all related resources
        // This handles: user doc, operators, api_keys
        if (!this.dbDocuments.has('user-pattern')) {
            this.dbDocuments.set('user-pattern', new Set());
        }
        this.dbDocuments.get('user-pattern').add(userId);
    }

    /**
     * Track a web session and its DB documents for cleanup.
     * Use this after calling webSessionService.createWebSession().
     */
    trackWebSession(sessionId, userId = null) {
        this.trackG8esKey(KVKey.webSessionKey(sessionId));
        if (userId) {
            this.trackG8esKey(KVKey.userWebSessions(userId));
        }
        this.trackDBDoc(Collections.WEB_SESSIONS, sessionId);
    }

    /**
     * Track an operator session and its DB documents for cleanup.
     * Use this after calling operatorSessionService.createOperatorSession().
     */
    trackOperatorSession(sessionId, userId = null) {
        this.trackG8esKey(KVKey.operatorSessionKey(sessionId));
        if (userId) {
            this.trackG8esKey(KVKey.userOperators(userId));
        }
        this.trackDBDoc(Collections.OPERATOR_SESSIONS, sessionId);
    }

    /**
     * Track session audit logs in DB using pattern matching
     */
    trackSessionAuditPattern(sessionId) {
        if (!this.dbDocuments.has('sessions-pattern')) {
            this.dbDocuments.set('sessions-pattern', new Set());
        }
        this.dbDocuments.get('sessions-pattern').add(sessionId);
    }

    /**
     * Track login audit logs for a specific identifier (email)
     * Use this when testing login security features that create audit logs
     */
    trackLoginAudit(identifier) {
        if (!this.dbDocuments.has('login_audit-pattern')) {
            this.dbDocuments.set('login_audit-pattern', new Set());
        }
        this.dbDocuments.get('login_audit-pattern').add(identifier);
        
        // Also track account_locks for this identifier (login security creates both)
        this.trackAccountLock(identifier);

        // Track KV login security keys — these cause stale 429s if not cleaned
        this.trackG8esKey(KVKey.loginFailed(identifier));
        this.trackG8esKey(KVKey.loginLock(identifier));
    }

    /**
     * Track account lock documents for a specific identifier
     * Account locks are stored with a hashed document ID based on the identifier
     * Use this when testing login security features that create account locks
     */
    trackAccountLock(identifier) {
        if (!this.dbDocuments.has('account_locks-pattern')) {
            this.dbDocuments.set('account_locks-pattern', new Set());
        }
        this.dbDocuments.get('account_locks-pattern').add(identifier);
    }

    /**
     * Track multiple g8es KV keys matching a pattern
     */
    async trackKVPattern(pattern) {
        const keys = await this._scanKVKeys(pattern);
        keys.forEach(key => this.kvKeys.add(key));
    }

    /**
     * Clean up all sessions for a specific user (call at start of test for isolation)
     * This ensures no stale sessions from previous test runs interfere
     */
    async cleanupUserSessions(userId) {
        const userSessionsKey = KVKey.userWebSessions(userId);
        const sessionIds = await this._cache_aside.kvZrange(userSessionsKey, 0, -1);
        
        const keysToDelete = [userSessionsKey];
        for (const sessionId of sessionIds) {
            keysToDelete.push(KVKey.webSessionKey(sessionId));
            keysToDelete.push(KVKey.operatorSessionKey(sessionId));
        }
        
        if (keysToDelete.length > 0) {
            await this._cache_aside.kvDel(...keysToDelete);
        }
    }

    /**
     * Clean up all tracked resources
     */
    async cleanup() {
        await Promise.all([
            this._cleanupKV(),
            this._cleanupDB()
        ]);
    }

    /**
     * Check if g8es KVCacheClient connection is available for operations
     */
    _isKVAvailable() {
        return !!this._cache_aside;
    }

    /**
     * Safely delete g8es KV key(s), ignoring connection closed errors
     */
    async _safeKVDel(...keys) {
        if (!this._isKVAvailable()) {
            return;
        }
        try {
            await this._cache_aside.kvDel(...keys);
        } catch (err) {
            // Ignore errors
        }
    }

    /**
     * Clean up g8es KV keys
     */
    async _cleanupKV() {
        if (this.kvKeys.size === 0) {
            return;
        }

        // Skip cleanup if client is terminated
        if (!this._isKVAvailable()) {
            this.kvKeys.clear();
            return;
        }

        const keys = Array.from(this.kvKeys);
        
        // Delete in batches of 100
        for (let i = 0; i < keys.length; i += 100) {
            const batch = keys.slice(i, i + 100);
            if (batch.length > 0) {
                try {
                    await this._cache_aside.kvDel(...batch);
                } catch (err) {
                    // Ignore errors
                }
            }
        }

        this.kvKeys.clear();
    }

    /**
     * Clean up DB documents
     */
    async _cleanupDB() {
        if (!this._cache_aside) {
            this.dbDocuments.clear();
            return;
        }

        const deletePromises = [];

        for (const [collection, docIds] of this.dbDocuments.entries()) {
            if (collection === 'sessions-pattern') {
                for (const sessionId of docIds) {
                    deletePromises.push(
                        this._cache_aside.deleteDocument(Collections.WEB_SESSIONS, sessionId),
                        this._cache_aside.deleteDocument(Collections.OPERATOR_SESSIONS, sessionId)
                    );
                }
            } else if (collection === 'login_audit-pattern') {
                for (const identifier of docIds) {
                    try {
                        const docs = await this._cache_aside.queryDocuments(Collections.LOGIN_AUDIT, [
                            { field: 'identifier', operator: '==', value: identifier }
                        ]);
                        for (const doc of docs) {
                            deletePromises.push(this._cache_aside.deleteDocument(Collections.LOGIN_AUDIT, doc.id));
                        }
                    } catch (e) { /* ignore */ }
                }
            } else if (collection === 'account_locks-pattern') {
                const crypto = await import('crypto');
                for (const identifier of docIds) {
                    const docId = crypto.createHash('sha256').update(identifier).digest('hex').substring(0, 32);
                    deletePromises.push(this._cache_aside.deleteDocument(Collections.ACCOUNT_LOCKS, docId));
                }
            } else if (collection === 'console_audit-pattern') {
                for (const userId of docIds) {
                    try {
                        const docs = await this._cache_aside.queryDocuments(Collections.CONSOLE_AUDIT, [
                            { field: 'user_id', operator: '==', value: userId }
                        ]);
                        for (const doc of docs) {
                            deletePromises.push(this._cache_aside.deleteDocument(Collections.CONSOLE_AUDIT, doc.id));
                        }
                    } catch (e) { /* ignore */ }
                }
            } else if (collection === 'api_keys-by-operator') {
                for (const operatorId of docIds) {
                    try {
                        const docs = await this._cache_aside.queryDocuments(Collections.API_KEYS, [
                            { field: 'operator_id', operator: '==', value: operatorId }
                        ]);
                        for (const doc of docs) {
                            deletePromises.push(this._cache_aside.deleteDocument(Collections.API_KEYS, doc.id));
                        }
                    } catch (e) { /* ignore */ }
                }
            } else if (collection === 'user-pattern') {
                for (const userId of docIds) {
                    try {
                        const userDoc = await this._cache_aside.getDocument(this.usersCollection, userId);
                        if (userDoc) {
                            if (userDoc.g8e_key) {
                                deletePromises.push(
                                    this._safeKVDel(KVKey.doc('api_keys', userDoc.g8e_key))
                                );
                            }
                            deletePromises.push(this._cache_aside.deleteDocument(this.usersCollection, userId));
                        }
                    } catch (e) { /* ignore */ }

                    try {
                        const ops = await this._cache_aside.queryDocuments(this.operatorsCollection, [
                            { field: 'user_id', operator: '==', value: userId }
                        ]);
                        for (const op of ops) {
                            deletePromises.push(
                                this._cache_aside.deleteDocument(this.operatorsCollection, op.operator_id)
                            );
                            if (op.operator_api_key) {
                                deletePromises.push(
                                    this._safeKVDel(KVKey.doc('api_keys', op.operator_api_key))
                                );
                            }
                            deletePromises.push(
                                this._safeKVDel(KVKey.doc('operators', op.operator_id)),
                                this._safeKVDel(KVKey.operatorFirstDeployed(op.operator_id)),
                                this._safeKVDel(KVKey.operatorTrackedStatus(op.operator_id))
                            );
                        }
                    } catch (e) { /* ignore */ }

                    try {
                        const apiKeys = await this._cache_aside.queryDocuments(Collections.API_KEYS, [
                            { field: 'user_id', operator: '==', value: userId }
                        ]);
                        for (const doc of apiKeys) {
                            deletePromises.push(this._cache_aside.deleteDocument(Collections.API_KEYS, doc.id));
                            if (doc.key) {
                                deletePromises.push(this._safeKVDel(KVKey.doc('api_keys', doc.key)));
                            }
                        }
                    } catch (e) { /* ignore */ }

                    try {
                        const ouDocs = await this._cache_aside.queryDocuments(Collections.OPERATOR_USAGE, [
                            { field: 'user_id', operator: '==', value: userId }
                        ]);
                        for (const doc of ouDocs) {
                            deletePromises.push(this._cache_aside.deleteDocument(Collections.OPERATOR_USAGE, doc.id));
                        }
                    } catch (e) { /* ignore */ }

                    try {
                        const orgDoc = await this._cache_aside.getDocument(Collections.ORGANIZATIONS, userId);
                        if (orgDoc) {
                            deletePromises.push(this._cache_aside.deleteDocument(Collections.ORGANIZATIONS, userId));
                        }
                    } catch (e) { /* ignore */ }

                    try {
                        const caDocs = await this._cache_aside.queryDocuments(Collections.CONSOLE_AUDIT, [
                            { field: 'user_id', operator: '==', value: userId }
                        ]);
                        for (const doc of caDocs) {
                            deletePromises.push(this._cache_aside.deleteDocument(Collections.CONSOLE_AUDIT, doc.id));
                        }
                    } catch (e) { /* ignore */ }
                }
            } else {
                for (const docId of docIds) {
                    deletePromises.push(this._cache_aside.deleteDocument(collection, docId));
                }
            }
        }

        await Promise.all(deletePromises);
        this.dbDocuments.clear();
    }

    /**
     * Scan g8es KV keys matching a pattern
     */
    async _scanKVKeys(pattern) {
        if (this.kvClient && typeof this.kvClient.keys === 'function') {
            return this.kvClient.keys(pattern);
        }
        if (!this._cache_aside) {
            return [];
        }
        return this._cache_aside.kvKeys(pattern);
    }
}
