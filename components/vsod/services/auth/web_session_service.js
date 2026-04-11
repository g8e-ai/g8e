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
 * WebSessionService - Cache-Aside Web Session Manager for VSOD
 *
 * Manages authenticated browser (web) sessions exclusively.
 *
 * WRITE Flow (cache-aside):
 * 1. Write to VSODB document store (authoritative)
 * 2. Update VSODB KV cache
 * 3. Return to client
 *
 * READ Flow (cache-aside):
 * 1. Check VSODB KV cache (~1-5ms)
 * 2. On miss, read from VSODB document store -> populate KV cache
 * 3. Return data
 *
 * DELETE Flow (cache-aside):
 * 1. Delete from VSODB document store (authoritative)
 * 2. Delete from VSODB KV cache
 */

import { randomUUID } from 'crypto';
import { logger } from '../../utils/logger.js';
import { now, addSeconds, secondsBetween } from '../../models/base.js';
import { WebSessionDocument } from '../../models/auth_models.js';
import { AuthProvider } from '../../constants/auth.js';
import { SessionType, SessionEventType, SessionEndReason, SessionSuspiciousReason } from '../../constants/session.js';
import { KVKey, KVScanPattern } from '../../constants/kv_keys.js';
import { ABSOLUTE_SESSION_TIMEOUT_SECONDS } from '../../constants/session.js';
import { Collections } from '../../constants/collections.js';
import { BaseSessionService } from './base_session_service.js';

export class WebSessionService extends BaseSessionService {
    constructor(options = {}) {
        super(options);
        const config = options.config || {};
        this.absoluteSessionTimeout = parseInt(config.absolute_session_timeout) || ABSOLUTE_SESSION_TIMEOUT_SECONDS;
        this.sessionsCollection = Collections.WEB_SESSIONS;
    }

    _generateSessionId() {
        return `web_session_${Date.now()}_${randomUUID()}`;
    }

    /**
     * Safely read user's web session IDs from sorted set.
     * Handles WRONGTYPE errors from corrupt keys by deleting and returning empty.
     */
    async _safeGetUserWebSessionIds(userId, reverse = false) {
        const userSessionsKey = KVKey.userWebSessions(userId);
        try {
            return reverse
                ? await this._cache_aside.kvZrevrange(userSessionsKey, 0, -1)
                : await this._cache_aside.kvZrange(userSessionsKey, 0, -1);
        } catch (error) {
            if (error.message?.includes('WRONGTYPE')) {
                logger.warn('[WEB-SESSION-SERVICE] Corrupt web_sessions key (wrong type), deleting', {
                    userId,
                    error: error.message
                });
                await this._cache_aside.kvDel(userSessionsKey);
                return [];
            }
            throw error;
        }
    }

    /**
     * Safely add a session to user's web sessions sorted set.
     * Handles WRONGTYPE errors from corrupt keys by deleting, recreating, and retrying.
     */
    async _safeAddToWebSessions(userId, score, sessionId) {
        const userSessionsKey = KVKey.userWebSessions(userId);
        try {
            await this._cache_aside.kvZadd(userSessionsKey, score, sessionId);
        } catch (error) {
            if (error.message?.includes('WRONGTYPE')) {
                logger.warn('[WEB-SESSION-SERVICE] Corrupt web_sessions key during write, resetting', {
                    userId,
                    error: error.message
                });
                await this._cache_aside.kvDel(userSessionsKey);
                await this._cache_aside.kvZadd(userSessionsKey, score, sessionId);
            } else {
                throw error;
            }
        }
    }

    /**
     * Create a new WEB session for an authenticated browser user.
     *
     * Required: user_id
     * Optional: user_data, organization_id, metadata
     *
     * @param {Object} sessionData
     * @param {string}  sessionData.user_id            - Authenticated user ID (required)
     * @param {Object}  [sessionData.user_data]        - User profile payload
     * @param {string}  [sessionData.organization_id]  - Organization override
     * @param {Object}  [sessionData.metadata]         - Arbitrary metadata (device, etc.)
     * @param {Object}  [requestContext]               - { ip, userAgent, loginMethod }
     * @param {Object}  [options]
     * @param {number}  [options.ttlSeconds]           - Custom TTL override
     * @returns {Promise<Object>} Persisted session document (sensitive fields decrypted)
     */
    async createWebSession(sessionData, requestContext = {}, options = {}) {
        if (!sessionData?.user_id) {
            throw new Error('createWebSession requires user_id');
        }
        const sessionId = this._generateSessionId();
        const ts = now();
        const customTtl = options.ttlSeconds;
        const absoluteExpiresAt = addSeconds(ts, customTtl ?? this.absoluteSessionTimeout);
        const idleExpiresAt = addSeconds(ts, customTtl ?? this.sessionTTL);

        const session = WebSessionDocument.parse({
            id: sessionId,
            session_type: SessionType.WEB,
            user_id: sessionData.user_id,
            organization_id: sessionData.organization_id || sessionData.user_data?.organization_id || null,
            user_data: sessionData.user_data ?? null,
            api_key: this._encryptField(sessionData.api_key),
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
            operator_status: null,
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
            throw new Error(`Web session persistence failed: ${result.error}`);
        }

        await this._safeAddToWebSessions(sessionData.user_id, Date.now(), sessionId);
        await this._cache_aside.kvExpire(KVKey.userWebSessions(sessionData.user_id), this.sessionTTL);

        logger.info('[WEB-SESSION-SERVICE] Web session created', {
            sessionId: sessionId.substring(0, 25) + '...',
            ttl,
            userEmail: sessionData.user_data?.email,
            userId: sessionData.user_id,
            clientIp: session.client_ip,
        });

        await this._logSessionEvent(SessionEventType.SESSION_CREATED, session, {
            login_method: session.login_method,
            session_type: SessionType.WEB,
        });

        return this._decryptSessionFields(session.forDB());
    }

    /**
     * Validate a web session and check for expiry and integrity.
     *
     * @param {string} webSessionId
     * @param {Object} [requestContext] - { ip, userAgent }
     * @returns {Promise<Object|null>}
     */
    async validateSession(webSessionId, requestContext = {}) {
        if (!webSessionId) {
            return null;
        }

        const data = await this._cache_aside.getDocument(
            this.sessionsCollection,
            webSessionId
        );

        if (!data || data.session_type !== SessionType.WEB) {
            logger.info('[WEB-SESSION-SERVICE] Web session not found or wrong type', {
                webSessionId: webSessionId.substring(0, 12) + '...'
            });
            return null;
        }

        // Canonical parse: ensures Date objects and type safety
        const session = WebSessionDocument.parse(data);

        const integrityCheck = this._validateSessionIntegrity(session, webSessionId);
        if (!integrityCheck.valid) {
            logger.error('[WEB-SESSION-SERVICE] Web session integrity check failed', {
                webSessionId: webSessionId.substring(0, 12) + '...',
                reason: integrityCheck.reason
            });
            await this.endSession(webSessionId, SessionEndReason.INTEGRITY_FAILURE);
            return null;
        }

        const checkTime = now();

        if (session.absolute_expires_at) {
            const absoluteExpiry = session.absolute_expires_at instanceof Date ? session.absolute_expires_at : new Date(session.absolute_expires_at);
            if (checkTime > absoluteExpiry) {
                logger.warn('[WEB-SESSION-SERVICE] Web session exceeded absolute timeout', {
                    webSessionId: webSessionId.substring(0, 12) + '...',
                    absoluteExpiresAt: session.absolute_expires_at
                });
                await this.endSession(webSessionId);
                await this._logSessionEvent(SessionEventType.SESSION_TIMEOUT_ABSOLUTE, session);
                return null;
            }
        }

        if (session.idle_expires_at) {
            const idleExpiry = session.idle_expires_at instanceof Date ? session.idle_expires_at : new Date(session.idle_expires_at);
            if (checkTime > idleExpiry) {
                logger.warn('[WEB-SESSION-SERVICE] Web session exceeded idle timeout', {
                    webSessionId: webSessionId.substring(0, 12) + '...',
                    idleExpiresAt: session.idle_expires_at
                });
                await this.endSession(webSessionId);
                await this._logSessionEvent(SessionEventType.SESSION_TIMEOUT_IDLE, session);
                return null;
            }
        }

        if (requestContext.ip && session.client_ip && requestContext.ip !== session.client_ip) {
            logger.warn('[WEB-SESSION-SERVICE] IP mismatch detected - potential session hijacking', {
                webSessionId: webSessionId.substring(0, 12) + '...',
                originalIp: session.client_ip,
                currentIp: requestContext.ip
            });

            session.ip_changes = (session.ip_changes || 0) + 1;
            session.last_ip = requestContext.ip;

            if (session.ip_changes > 3) {
                session.suspicious_activity = true;
                await this._logSessionEvent(SessionEventType.SESSION_SUSPICIOUS_ACTIVITY, session, {
                    reason: SessionSuspiciousReason.EXCESSIVE_IP_CHANGES,
                    ip_changes: session.ip_changes
                });
            }

            const patch = { ip_changes: session.ip_changes, last_ip: session.last_ip };
            if (session.suspicious_activity) patch.suspicious_activity = true;
            await this._cache_aside.updateDocument(this.sessionsCollection, webSessionId, patch);
        }

        return this._decryptSessionFields(session);
    }

    /**
     * Validate session and update last activity timestamp.
     * Use for authenticated requests that should bump activity.
     */
    async validateAndUpdateActivity(webSessionId) {
        const session = await this.validateSession(webSessionId);
        if (!session) {
            return null;
        }

        await this._cache_aside.updateDocument(
            this.sessionsCollection,
            webSessionId,
            { last_activity: now() }
        );

        return session;
    }

    /**
     * Refresh session TTL (respects absolute timeout).
     * CACHE-ASIDE PATTERN: Update VSODB document store (source of truth), then update VSODB KV cache.
     */
    async refreshSession(webSessionId, session = null) {
        if (!session) {
            const data = await this._cache_aside.getDocument(
                this.sessionsCollection,
                webSessionId
            );
            if (!data || data.session_type !== SessionType.WEB) {
                return false;
            }
            session = WebSessionDocument.parse(data);
        }

        const checkTime = now();

        if (session.absolute_expires_at) {
            const absoluteExpiry = new Date(session.absolute_expires_at);
            if (checkTime > absoluteExpiry) {
                logger.warn('[WEB-SESSION-SERVICE] Cannot refresh - absolute timeout exceeded', {
                    webSessionId: webSessionId.substring(0, 12) + '...'
                });
                await this.endSession(webSessionId);
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
            webSessionId,
            { last_activity: checkTime, idle_expires_at: newIdleExpiry }
        );

        logger.info('[WEB-SESSION-SERVICE] Web session refreshed', {
            webSessionId: webSessionId.substring(0, 12) + '...',
            ttl,
            timeUntilAbsoluteExpiry
        });

        return true;
    }

    /**
     * Bind an operator to a web session.
     * Updates the operator_ids array in the web session document.
     * CACHE-ASIDE PATTERN: Updates VSODB document store then invalidates KV.
     */
    async bindOperatorToWebSession(webSessionId, operatorId) {
        const session = await this._cache_aside.getDocument(this.sessionsCollection, webSessionId);
        if (!session || session.session_type !== SessionType.WEB) {
            logger.warn('[WEB-SESSION-SERVICE] Cannot bind operator to non-existent web session', {
                webSessionId: webSessionId.substring(0, 12) + '...'
            });
            return false;
        }

        const operatorIds = Array.isArray(session.operator_ids) ? session.operator_ids : [];
        if (operatorIds.includes(operatorId)) {
            return true;
        }

        const updatedOperatorIds = [...operatorIds, operatorId];
        await this._cache_aside.updateDocument(
            this.sessionsCollection,
            webSessionId,
            { operator_ids: updatedOperatorIds, last_activity: now() }
        );

        logger.info('[WEB-SESSION-SERVICE] Operator bound to web session document', {
            webSessionId: webSessionId.substring(0, 12) + '...',
            operatorId
        });

        return true;
    }

    /**
     * Unbind an operator from a web session.
     * Updates the operator_ids array in the web session document.
     * CACHE-ASIDE PATTERN: Updates VSODB document store then invalidates KV.
     */
    async unbindOperatorFromWebSession(webSessionId, operatorId) {
        const session = await this._cache_aside.getDocument(this.sessionsCollection, webSessionId);
        if (!session || session.session_type !== SessionType.WEB) {
            logger.warn('[WEB-SESSION-SERVICE] Cannot unbind operator from non-existent web session', {
                webSessionId: webSessionId.substring(0, 12) + '...'
            });
            return false;
        }

        const operatorIds = Array.isArray(session.operator_ids) ? session.operator_ids : [];
        if (!operatorIds.includes(operatorId)) {
            return true;
        }

        const updatedOperatorIds = operatorIds.filter(id => id !== operatorId);
        await this._cache_aside.updateDocument(
            this.sessionsCollection,
            webSessionId,
            { operator_ids: updatedOperatorIds, last_activity: now() }
        );

        logger.info('[WEB-SESSION-SERVICE] Operator unbound from web session document', {
            webSessionId: webSessionId.substring(0, 12) + '...',
            operatorId
        });

        return true;
    }

    /**
     * Update session data with deep merge for user_data.
     * CACHE-ASIDE PATTERN: Update VSODB document store (source of truth), then update VSODB KV cache.
     * @returns {Promise<WebSessionDocument|null>}
     */
    async updateSession(webSessionId, updates) {
        const session = await this._cache_aside.getDocument(
            this.sessionsCollection,
            webSessionId
        );
        if (!session || session.session_type !== SessionType.WEB) {
            logger.warn('[WEB-SESSION-SERVICE] Cannot update non-existent web session', {
                webSessionId: webSessionId.substring(0, 12) + '...'
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

        const sessionDoc = WebSessionDocument.parse(persistedSession);

        await this._cache_aside.updateDocument(
            this.sessionsCollection,
            webSessionId,
            sessionDoc
        );

        logger.info('[WEB-SESSION-SERVICE] Web session updated', {
            webSessionId: webSessionId.substring(0, 12) + '...',
            updatedFields: Object.keys(updates)
        });

        return this._decryptSessionFields(sessionDoc.forDB());
    }

    /**
     * Extend session TTL to full duration.
     * CACHE-ASIDE PATTERN: Update VSODB document store (source of truth), then update VSODB KV cache.
     */
    async extendSession(webSessionId) {
        const session = await this._cache_aside.getDocument(
            this.sessionsCollection,
            webSessionId
        );
        if (!session || session.session_type !== SessionType.WEB) {
            logger.warn('[WEB-SESSION-SERVICE] Cannot extend non-existent web session', {
                webSessionId: webSessionId.substring(0, 12) + '...'
            });
            return false;
        }

        const extendTime = now();

        await this._cache_aside.updateDocument(
            this.sessionsCollection,
            webSessionId,
            WebSessionDocument.parse({
                ...session,
                absolute_expires_at: addSeconds(extendTime, this.absoluteSessionTimeout),
                idle_expires_at: addSeconds(extendTime, this.sessionTTL),
                last_activity: extendTime
            })
        );

        logger.info('[WEB-SESSION-SERVICE] Web session TTL extended to full duration', {
            webSessionId: webSessionId.substring(0, 12) + '...',
            newTTL: this.sessionTTL
        });

        return true;
    }

    /**
     * End a web session (logout).
     * CACHE-ASIDE PATTERN: Delete from VSODB document store (source of truth), then invalidate VSODB KV cache.
     */
    async endSession(webSessionId, reason = SessionEndReason.USER_LOGOUT) {
        const session = await this._cache_aside.getDocument(
            this.sessionsCollection,
            webSessionId
        );

        if (!session) {
            logger.info('[WEB-SESSION-SERVICE] Web session not found for end', {
                webSessionId: webSessionId.substring(0, 12) + '...',
                reason
            });
            return false;
        }

        const userId = session.user_id;

        await this._cache_aside.deleteDocument(
            this.sessionsCollection,
            webSessionId
        );

        if (userId) {
            await this._cache_aside.kvZrem(KVKey.userWebSessions(userId), webSessionId);
        }

        await this._logSessionEvent(SessionEventType.SESSION_ENDED, session, { reason });

        logger.info('[WEB-SESSION-SERVICE] Web session ended', {
            webSessionId: webSessionId.substring(0, 12) + '...',
            cleanedUserTracking: !!userId,
            reason
        });

        return true;
    }

    /**
     * Regenerate a web session ID (prevents session fixation).
     * Creates a new web session with the same data but a different ID.
     */
    async regenerateWebSession(oldSessionId, requestContext = {}) {
        const raw = await this._cache_aside.getDocument(this.sessionsCollection, oldSessionId);
        if (raw && raw.session_type !== SessionType.WEB) {
            throw new Error(`[WEB-SESSION-SERVICE] Cannot regenerate operator session as web session: ${oldSessionId.substring(0, 12)}...`);
        }

        const session = await this.validateSession(oldSessionId, requestContext);
        if (!session) {
            logger.warn('[WEB-SESSION-SERVICE] Cannot regenerate non-existent web session', {
                oldSessionId: oldSessionId.substring(0, 12) + '...'
            });
            return null;
        }

        const newSession = await this.createWebSession({
            user_id: session.user_id,
            user_data: session.user_data,
            organization_id: session.organization_id,
            metadata: session.metadata,
        }, requestContext);

        await this.endSession(oldSessionId, SessionEndReason.SESSION_REGENERATION);

        logger.info('[WEB-SESSION-SERVICE] Web session regenerated', {
            oldSessionId: oldSessionId.substring(0, 12) + '...',
            newSessionId: newSession.id.substring(0, 12) + '...'
        });

        await this._logSessionEvent(SessionEventType.SESSION_REGENERATED, newSession, {
            old_session_id: oldSessionId
        });

        return newSession;
    }

    /**
     * Update all web sessions for a user (e.g., after role change).
     */
    async updateAllUserSessions(userId, updates) {
        const sessionIds = await this._safeGetUserWebSessionIds(userId);

        let updatedCount = 0;
        for (const sessionId of sessionIds) {
            try {
                await this.updateSession(sessionId, updates);
                updatedCount++;
            } catch (error) {
                logger.warn('[WEB-SESSION-SERVICE] Failed to update session', {
                    sessionId: sessionId.substring(0, 12) + '...',
                    error: error.message
                });
            }
        }

        logger.info('[WEB-SESSION-SERVICE] All user sessions updated', {
            userId,
            updatedCount,
            totalSessions: sessionIds.length,
            updatedFields: Object.keys(updates)
        });

        return updatedCount;
    }

    /**
     * Invalidate all web sessions for a user (e.g., security breach).
     * Uses endSession per session to ensure full cache-aside delete.
     */
    async invalidateAllUserSessions(userId) {
        const sessionIds = await this._safeGetUserWebSessionIds(userId);
        let deletedCount = 0;
        for (const sessionId of sessionIds) {
            const deleted = await this.endSession(sessionId, SessionEndReason.INVALIDATE_ALL);
            if (deleted) deletedCount++;
        }
        await this._cache_aside.kvDel(KVKey.userWebSessions(userId));
        logger.info('[WEB-SESSION-SERVICE] All user sessions invalidated', { userId, deletedCount, totalSessions: sessionIds.length });
        return deletedCount;
    }

    /**
     * Get user's most recent active web session.
     *
     * @param {string} userId
     * @returns {Promise<string|null>} Most recent web session ID or null
     */
    async getUserActiveSession(userId) {
        if (!userId) {
            return null;
        }

        const userSessionsKey = KVKey.userWebSessions(userId);
        const sessionIds = await this._safeGetUserWebSessionIds(userId, true);

        for (const sessionId of sessionIds) {
            const session = await this._cache_aside.getDocument(this.sessionsCollection, sessionId);
            if (session && session.is_active) {
                return sessionId;
            } else {
                await this._cache_aside.kvZrem(userSessionsKey, sessionId);
            }
        }

        return null;
    }

    /**
     * Get all active web sessions for a user.
     *
     * @param {string} userId
     * @returns {Promise<string[]>} Array of web session IDs
     */
    async getUserActiveSessions(userId) {
        if (!userId) {
            return [];
        }

        const userSessionsKey = KVKey.userWebSessions(userId);
        const sessionIds = await this._safeGetUserWebSessionIds(userId);

        if (sessionIds.length === 0) {
            return [];
        }

        const validSessions = [];
        for (const sessionId of sessionIds) {
            const session = await this._cache_aside.getDocument(this.sessionsCollection, sessionId);
            if (session && session.is_active) {
                validSessions.push(sessionId);
            } else {
                await this._cache_aside.kvZrem(userSessionsKey, sessionId);
            }
        }

        return validSessions;
    }

    /**
     * Get total session count across all web session keys (for monitoring).
     * Uses SCAN instead of KEYS to avoid blocking VSODB KV in production.
     */
    async getSessionCount() {
        let count = 0;
        let cursor = '0';
        do {
            const result = await this._cache_aside.kvScan(cursor, 'MATCH', KVScanPattern.scanWebSessions(), 'COUNT', 100);
            cursor = result[0];
            count += result[1].length;
        } while (cursor !== '0');
        return count;
    }
}
