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
 * OperatorSessionService - Cache-Aside Operator Session Manager for g8ed
 *
 * Manages operator daemon sessions exclusively.
 *
 * WRITE Flow (cache-aside):
 * 1. Write to g8es document store (authoritative)
 * 2. Update g8es KV cache
 * 3. Return to client
 *
 * READ Flow (cache-aside):
 * 1. Check g8es KV cache (~1-5ms)
 * 2. On miss, read from g8es document store -> populate KV cache
 * 3. Return data
 *
 * DELETE Flow (cache-aside):
 * 1. Delete from g8es document store (authoritative)
 * 2. Delete from g8es KV cache
 */

import { randomUUID } from 'crypto';
import { logger } from '../../utils/logger.js';
import { now, addSeconds, secondsBetween } from '../../models/base.js';
import { OperatorSessionDocument } from '../../models/auth_models.js';
import { AuthProvider } from '../../constants/auth.js';
import { KVScanPattern } from '../../constants/kv_keys.js';
import { SessionType, SessionEventType, SessionEndReason } from '../../constants/session.js';
import { ABSOLUTE_SESSION_TIMEOUT_SECONDS } from '../../constants/session.js';
import { Collections } from '../../constants/collections.js';
import { BaseSessionService } from './base_session_service.js';

export class OperatorSessionService extends BaseSessionService {
    constructor(options = {}) {
        super(options);
        const config = options.config || {};
        this.absoluteSessionTimeout = parseInt(config.absolute_session_timeout) || ABSOLUTE_SESSION_TIMEOUT_SECONDS;
        this.sessionsCollection = Collections.OPERATOR_SESSIONS;
    }

    _generateSessionId() {
        return `operator_session_${Date.now()}_${randomUUID()}`;
    }

    /**
     * Create a new OPERATOR session for a connected operator process.
     *
     * Required: operator_id
     * Optional: user_id, user_data, api_key, operator_status, metadata
     *
     * @param {Object} sessionData
     * @param {string}  sessionData.operator_id        - Operator instance ID (required)
     * @param {string}  [sessionData.user_id]          - Owning user ID
     * @param {Object}  [sessionData.user_data]        - User profile payload
     * @param {string}  [sessionData.api_key]          - API key
     * @param {string}  [sessionData.operator_status]  - Initial operator status
     * @param {Object}  [sessionData.metadata]         - Arbitrary metadata (hostname, etc.)
     * @param {Object}  [requestContext]               - { ip, userAgent, loginMethod }
     * @param {Object}  [options]
     * @param {number}  [options.ttlSeconds]           - Custom TTL override
     * @returns {Promise<Object>} Persisted session document (sensitive fields decrypted)
     */
    async createOperatorSession(sessionData, requestContext = {}, options = {}) {
        const sessionId = this._generateSessionId();
        const ts = now();
        const customTtl = options.ttlSeconds;
        const absoluteExpiresAt = addSeconds(ts, customTtl ?? this.absoluteSessionTimeout);
        const idleExpiresAt = addSeconds(ts, customTtl ?? this.sessionTTL);

        const session = OperatorSessionDocument.parse({
            id: sessionId,
            session_type: SessionType.OPERATOR,
            user_id: sessionData.user_id,
            organization_id: sessionData.organization_id || sessionData.user_data?.organization_id || null,
            user_data: sessionData.user_data ?? null,
            api_key: this._encryptField(sessionData.api_key),
            operator_id: sessionData.operator_id,
            client_ip: requestContext.ip || null,
            user_agent: requestContext.userAgent || null,
            login_method: requestContext.loginMethod || AuthProvider.LOCAL,
            created_at: ts,
            absolute_expires_at: absoluteExpiresAt,
            idle_expires_at: idleExpiresAt,
            last_activity: ts,
            last_ip: requestContext.ip || null,
            ip_changes: 0,
            suspicious_activity: false,
            is_active: true,
            operator_status: sessionData.operator_status ?? null,
            metadata: sessionData.metadata ?? null,
        });

        const ttl = Math.min(this.sessionTTL, secondsBetween(ts, absoluteExpiresAt));
        const result = await this._cache_aside.createDocument(
            this.sessionsCollection,
            sessionId,
            session,
            ttl
        );
        if (!result.success) {
            throw new Error(`Operator session persistence failed: ${result.error}`);
        }

        logger.info('[OPERATOR-SESSION-SERVICE] Operator session created', {
            sessionId: sessionId.substring(0, 12) + '...',
            ttl,
            operatorId: sessionData.operator_id,
            userId: sessionData.user_id,
        });

        await this._logSessionEvent(SessionEventType.SESSION_CREATED, session, {
            login_method: session.login_method,
            session_type: SessionType.OPERATOR,
        });

        return this._decryptSessionFields(session.forDB());
    }

    /**
     * Validate an operator session and check for expiry and integrity.
     *
     * @param {string} operatorSessionId
     * @param {Object} [requestContext] - { ip, userAgent }
     * @returns {Promise<Object|null>}
     */
    async validateSession(operatorSessionId, requestContext = {}) {
        if (!operatorSessionId) {
            return null;
        }

        const data = await this._cache_aside.getDocument(
            this.sessionsCollection,
            operatorSessionId
        );

        if (!data || data.session_type !== SessionType.OPERATOR) {
            logger.info('[OPERATOR-SESSION-SERVICE] Operator session not found or wrong type', {
                operatorSessionId: operatorSessionId.substring(0, 12) + '...'
            });
            return null;
        }

        // Canonical parse: ensures Date objects and type safety
        const session = OperatorSessionDocument.parse(data);

        const integrityCheck = this._validateSessionIntegrity(session, operatorSessionId);
        if (!integrityCheck.valid) {
            logger.error('[OPERATOR-SESSION-SERVICE] Operator session integrity check failed', {
                operatorSessionId: operatorSessionId.substring(0, 12) + '...',
                reason: integrityCheck.reason
            });
            await this.endSession(operatorSessionId, SessionEndReason.INTEGRITY_FAILURE);
            return null;
        }

        const checkTime = now();

        if (session.absolute_expires_at) {
            const absoluteExpiry = session.absolute_expires_at instanceof Date ? session.absolute_expires_at : new Date(session.absolute_expires_at);
            if (checkTime > absoluteExpiry) {
                logger.warn('[OPERATOR-SESSION-SERVICE] Operator session exceeded absolute timeout', {
                    operatorSessionId: operatorSessionId.substring(0, 12) + '...',
                    absoluteExpiresAt: session.absolute_expires_at
                });
                await this.endSession(operatorSessionId);
                await this._logSessionEvent(SessionEventType.SESSION_TIMEOUT_ABSOLUTE, session);
                return null;
            }
        }

        if (session.idle_expires_at) {
            const idleExpiry = session.idle_expires_at instanceof Date ? session.idle_expires_at : new Date(session.idle_expires_at);
            if (checkTime > idleExpiry) {
                logger.warn('[OPERATOR-SESSION-SERVICE] Operator session exceeded idle timeout', {
                    operatorSessionId: operatorSessionId.substring(0, 12) + '...',
                    idleExpiresAt: session.idle_expires_at
                });
                await this.endSession(operatorSessionId);
                await this._logSessionEvent(SessionEventType.SESSION_TIMEOUT_IDLE, session);
                return null;
            }
        }

        return this._decryptSessionFields(session);
    }

    /**
     * Refresh operator session TTL (respects absolute timeout).
     * CACHE-ASIDE PATTERN: Update g8es document store (source of truth), then update g8es KV cache.
     */
    async refreshSession(operatorSessionId, session = null) {
        if (!session) {
            const data = await this._cache_aside.getDocument(
                this.sessionsCollection,
                operatorSessionId
            );
            if (!data || data.session_type !== SessionType.OPERATOR) {
                return false;
            }
            session = OperatorSessionDocument.parse(data);
        }

        const checkTime = now();

        if (session.absolute_expires_at) {
            const absoluteExpiry = new Date(session.absolute_expires_at);
            if (checkTime > absoluteExpiry) {
                logger.warn('[OPERATOR-SESSION-SERVICE] Cannot refresh - absolute timeout exceeded', {
                    operatorSessionId: operatorSessionId.substring(0, 12) + '...'
                });
                await this.endSession(operatorSessionId);
                return false;
            }
        }

        const newIdleExpiry = addSeconds(checkTime, this.sessionTTL);
        const absoluteExpiry = session.absolute_expires_at instanceof Date ? session.absolute_expires_at : new Date(session.absolute_expires_at);
        const timeUntilAbsoluteExpiry = session.absolute_expires_at
            ? secondsBetween(checkTime, absoluteExpiry)
            : this.sessionTTL;
        const ttl = Math.min(this.sessionTTL, timeUntilAbsoluteExpiry);

        await this._cache_aside.updateDocument(
            this.sessionsCollection,
            operatorSessionId,
            { last_activity: checkTime, idle_expires_at: newIdleExpiry }
        );

        logger.info('[OPERATOR-SESSION-SERVICE] Operator session refreshed', {
            operatorSessionId: operatorSessionId.substring(0, 12) + '...',
            ttl,
            timeUntilAbsoluteExpiry
        });

        return true;
    }

    /**
     * Update operator session data with deep merge for user_data.
     * CACHE-ASIDE PATTERN: Update g8es document store (source of truth), then update g8es KV cache.
     */
    async updateSession(operatorSessionId, updates) {
        const session = await this._cache_aside.getDocument(
            this.sessionsCollection,
            operatorSessionId
        );
        if (!session || session.session_type !== SessionType.OPERATOR) {
            logger.warn('[OPERATOR-SESSION-SERVICE] Cannot update non-existent operator session', {
                operatorSessionId: operatorSessionId.substring(0, 12) + '...'
            });
            return null;
        }

        const mergedSession = {
            ...session,
            ...updates,
            user_data: updates.user_data ? {
                ...session.user_data,
                ...updates.user_data
            } : session.user_data,
            last_activity: now()
        };

        const encryptedUpdates = { ...updates };
        if (updates.api_key !== undefined) {
            encryptedUpdates.api_key = this._encryptField(updates.api_key);
        }
        const persistedSession = {
            ...session,
            ...encryptedUpdates,
            user_data: mergedSession.user_data,
            last_activity: mergedSession.last_activity
        };

        await this._cache_aside.updateDocument(
            this.sessionsCollection,
            operatorSessionId,
            OperatorSessionDocument.parse(persistedSession)
        );

        logger.info('[OPERATOR-SESSION-SERVICE] Operator session updated', {
            operatorSessionId: operatorSessionId.substring(0, 12) + '...',
            updatedFields: Object.keys(updates)
        });

        return mergedSession;
    }

    /**
     * Extend operator session TTL to full duration.
     * CACHE-ASIDE PATTERN: Update g8es document store (source of truth), then update g8es KV cache.
     */
    async extendSession(operatorSessionId) {
        const session = await this._cache_aside.getDocument(
            this.sessionsCollection,
            operatorSessionId
        );
        if (!session || session.session_type !== SessionType.OPERATOR) {
            logger.warn('[OPERATOR-SESSION-SERVICE] Cannot extend non-existent operator session', {
                operatorSessionId: operatorSessionId.substring(0, 12) + '...'
            });
            return false;
        }

        const extendTime = now();

        await this._cache_aside.updateDocument(
            this.sessionsCollection,
            operatorSessionId,
            OperatorSessionDocument.parse({
                ...session,
                absolute_expires_at: addSeconds(extendTime, this.absoluteSessionTimeout),
                idle_expires_at: addSeconds(extendTime, this.sessionTTL),
                last_activity: extendTime
            })
        );

        logger.info('[OPERATOR-SESSION-SERVICE] Operator session TTL extended to full duration', {
            operatorSessionId: operatorSessionId.substring(0, 12) + '...',
            newTTL: this.sessionTTL
        });

        return true;
    }

    /**
     * End an operator session.
     * CACHE-ASIDE PATTERN: Delete from g8es document store (source of truth), then invalidate g8es KV cache.
     */
    async endSession(operatorSessionId, reason = SessionEndReason.USER_LOGOUT) {
        const session = await this._cache_aside.getDocument(
            this.sessionsCollection,
            operatorSessionId
        );

        if (!session) {
            logger.info('[OPERATOR-SESSION-SERVICE] Operator session not found for end', {
                operatorSessionId: operatorSessionId.substring(0, 12) + '...',
                reason
            });
            return false;
        }

        await this._cache_aside.deleteDocument(
            this.sessionsCollection,
            operatorSessionId
        );

        await this._logSessionEvent(SessionEventType.SESSION_ENDED, session, { reason });

        logger.info('[OPERATOR-SESSION-SERVICE] Operator session ended', {
            operatorSessionId: operatorSessionId.substring(0, 12) + '...',
            reason
        });

        return true;
    }

    /**
     * Regenerate an operator session ID (prevents session fixation).
     * Creates a new operator session with the same data but a different ID.
     */
    async regenerateOperatorSession(oldSessionId, requestContext = {}) {
        const raw = await this._cache_aside.getDocument(this.sessionsCollection, oldSessionId);
        if (raw && raw.session_type !== SessionType.OPERATOR) {
            throw new Error(`[OPERATOR-SESSION-SERVICE] Cannot regenerate web session as operator session: ${oldSessionId.substring(0, 12)}...`);
        }

        const session = await this.validateSession(oldSessionId, requestContext);
        if (!session) {
            logger.warn('[OPERATOR-SESSION-SERVICE] Cannot regenerate non-existent operator session', {
                oldSessionId: oldSessionId.substring(0, 12) + '...'
            });
            return null;
        }

        const newSession = await this.createOperatorSession({
            user_id: session.user_id,
            user_data: session.user_data,
            api_key: session.api_key,
            operator_id: session.operator_id,
            operator_status: session.operator_status,
            metadata: session.metadata,
        }, requestContext);

        await this.endSession(oldSessionId, SessionEndReason.SESSION_REGENERATION);

        logger.info('[OPERATOR-SESSION-SERVICE] Operator session regenerated', {
            oldSessionId: oldSessionId.substring(0, 12) + '...',
            newSessionId: newSession.id.substring(0, 12) + '...'
        });

        await this._logSessionEvent(SessionEventType.SESSION_REGENERATED, newSession, {
            old_session_id: oldSessionId
        });

        return newSession;
    }

    /**
     * Get total session count across all operator session keys (for monitoring).
     * Uses SCAN instead of KEYS to avoid blocking g8es KV in production.
     */
    async getSessionCount() {
        let count = 0;
        let cursor = '0';
        do {
            const result = await this._cache_aside.kvScan(cursor, 'MATCH', KVScanPattern.scanOperatorSessions(), 'COUNT', 100);
            cursor = result[0];
            count += result[1].length;
        } while (cursor !== '0');
        return count;
    }
}
