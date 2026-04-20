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

import { randomUUID } from 'crypto';
import { addSeconds, secondsBetween, now } from '../../models/base.js';
import { logger } from '../../utils/logger.js';
import { SessionType, SESSION_TTL_SECONDS, ABSOLUTE_SESSION_TIMEOUT_SECONDS, SessionEndReason, SessionEventType } from '../../constants/session.js';
import { AuthProvider } from '../../constants/auth.js';
import { BaseSessionService } from './base_session_service.js';
import { CliSessionDocument } from '../../models/auth_models.js';
import { Collections } from '../../constants/collections.js';

/**
 * CliSessionService - Manages CLI sessions for platform authentication
 * 
 * CLI sessions are used for CLI tool authentication without requiring operator slots.
 * They do not have operator_id and cannot run operators, but can access platform APIs.
 */
export class CliSessionService extends BaseSessionService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAside service instance
     * @param {Object} options.bootstrapService - Bootstrap service instance
     * @param {Object} options.auditService - Audit service instance
     */
    constructor({ cacheAsideService, bootstrapService, auditService }) {
        super({ cacheAsideService, bootstrapService });
        this._audit = auditService;
        this.sessionsCollection = Collections.CLI_SESSIONS;
        this.absoluteSessionTimeout = ABSOLUTE_SESSION_TIMEOUT_SECONDS;
    }

    _generateSessionId() {
        return `cli_session_${Date.now()}_${randomUUID()}`;
    }

    _decryptSessionFields(session) {
        const decrypted = { ...session };
        if (decrypted.api_key) {
            decrypted.api_key = this._decryptField(decrypted.api_key);
        }
        return decrypted;
    }

    async _logSessionEvent(eventType, session, metadata = {}) {
        try {
            await this._audit.logSessionEvent({
                session_id: session.id,
                user_id: session.user_id,
                event_type: eventType,
                session_type: session.session_type,
                metadata: {
                    ...metadata,
                    ip: session.client_ip,
                    user_agent: session.user_agent,
                },
            });
        } catch (err) {
            logger.warn('[CLI-SESSION-SERVICE] Failed to log session event', { error: err.message });
        }
    }

    /**
     * Create a CLI session.
     *
     * @param {Object} sessionData
     * @param {string} sessionData.user_id
     * @param {Object} sessionData.user_data
     * @param {string} sessionData.api_key
     * @param {string} [sessionData.organization_id]
     * @param {Object} [requestContext]               - { ip, userAgent, loginMethod }
     * @param {Object} [options]
     * @param {number} [options.ttlSeconds]           - Custom TTL override
     * @returns {Promise<Object>} Persisted session document (sensitive fields decrypted)
     */
    async createSession(sessionData, requestContext = {}, options = {}) {
        const sessionId = this._generateSessionId();
        const ts = now();
        const customTtl = options.ttlSeconds;
        const absoluteExpiresAt = addSeconds(ts, customTtl ?? this.absoluteSessionTimeout);
        const idleExpiresAt = addSeconds(ts, customTtl ?? this.sessionTTL);

        const session = CliSessionDocument.parse({
            id: sessionId,
            session_type: SessionType.CLI,
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
            throw new Error(`CLI session persistence failed: ${result.error}`);
        }

        logger.info('[CLI-SESSION-SERVICE] CLI session created', {
            sessionId: sessionId.substring(0, 12) + '...',
            ttl,
            userId: sessionData.user_id,
        });

        await this._logSessionEvent(SessionEventType.SESSION_CREATED, session, {
            login_method: session.login_method,
            session_type: SessionType.CLI,
        });

        return this._decryptSessionFields(session.forDB());
    }

    /**
     * Validate a CLI session and check for expiry and integrity.
     *
     * @param {string} cliSessionId
     * @param {Object} [requestContext] - { ip, userAgent }
     * @returns {Promise<Object|null>}
     */
    async validateSession(cliSessionId, requestContext = {}) {
        if (!cliSessionId) {
            return null;
        }

        const data = await this._cache_aside.getDocument(
            this.sessionsCollection,
            cliSessionId
        );

        if (!data || data.session_type !== SessionType.CLI) {
            logger.info('[CLI-SESSION-SERVICE] CLI session not found or wrong type', {
                cliSessionId: cliSessionId.substring(0, 12) + '...'
            });
            return null;
        }

        const session = CliSessionDocument.parse(data);

        const integrityCheck = this._validateSessionIntegrity(session, cliSessionId);
        if (!integrityCheck.valid) {
            logger.error('[CLI-SESSION-SERVICE] CLI session integrity check failed', {
                cliSessionId: cliSessionId.substring(0, 12) + '...',
                reason: integrityCheck.reason
            });
            await this.endSession(cliSessionId, SessionEndReason.INTEGRITY_FAILURE);
            return null;
        }

        const checkTime = now();
        const expiryCheck = this._checkSessionExpiry(session, checkTime);
        if (!expiryCheck.valid) {
            logger.info('[CLI-SESSION-SERVICE] CLI session expired', {
                cliSessionId: cliSessionId.substring(0, 12) + '...',
                reason: expiryCheck.reason
            });
            await this.endSession(cliSessionId, SessionEndReason.SESSION_REGENERATION);
            return null;
        }

        // Update last activity on successful validation
        await this.updateLastActivity(cliSessionId, checkTime, requestContext.ip);

        return this._decryptSessionFields(session.forDB());
    }

    _validateSessionIntegrity(session, sessionId) {
        if (!session.is_active) {
            return { valid: false, reason: 'Session not active' };
        }
        if (!session.id || session.id !== sessionId) {
            return { valid: false, reason: 'Session ID mismatch' };
        }
        return { valid: true };
    }

    _checkSessionExpiry(session, checkTime) {
        if (checkTime > session.absolute_expires_at) {
            return { valid: false, reason: 'Absolute timeout' };
        }
        if (checkTime > session.idle_expires_at) {
            return { valid: false, reason: 'Idle timeout' };
        }
        return { valid: true };
    }

    async updateLastActivity(sessionId, timestamp, ip) {
        try {
            const current = await this._cache_aside.getDocument(
                this.sessionsCollection,
                sessionId
            );
            if (!current) return;

            const updated = {
                ...current,
                last_activity: timestamp,
                last_ip: ip || current.last_ip,
            };

            const ttl = Math.min(
                this.sessionTTL,
                secondsBetween(timestamp, current.absolute_expires_at)
            );

            await this._cache_aside.updateDocument(
                this.sessionsCollection,
                sessionId,
                updated,
                ttl
            );
        } catch (err) {
            logger.warn('[CLI-SESSION-SERVICE] Failed to update last activity', {
                sessionId: sessionId.substring(0, 12) + '...',
                error: err.message
            });
        }
    }

    /**
     * End a CLI session.
     *
     * @param {string} sessionId
     * @param {string} reason
     * @returns {Promise<boolean>}
     */
    async endSession(sessionId, reason = SessionEndReason.USER_LOGOUT) {
        try {
            const session = await this._cache_aside.getDocument(
                this.sessionsCollection,
                sessionId
            );
            if (!session) return false;

            await this._logSessionEvent(SessionEventType.SESSION_ENDED, session, {
                end_reason: reason,
            });

            const result = await this._cache_aside.deleteDocument(
                this.sessionsCollection,
                sessionId
            );

            logger.info('[CLI-SESSION-SERVICE] CLI session ended', {
                sessionId: sessionId.substring(0, 12) + '...',
                reason,
                success: result.success,
            });

            return result.success;
        } catch (err) {
            logger.error('[CLI-SESSION-SERVICE] Failed to end session', {
                sessionId: sessionId.substring(0, 12) + '...',
                error: err.message
            });
            return false;
        }
    }
}
