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
 * BaseSessionService - Shared infrastructure for WebSessionService and OperatorSessionService
 *
 * Provides encryption, decryption, KV key helpers, integrity validation,
 * cache-aside access, and session event logging. Not instantiated directly.
 */

import { createCipheriv, createDecipheriv, randomBytes, randomUUID } from 'crypto';
import { logger } from '../../utils/logger.js';
import { now } from '../../models/base.js';
import { SessionDocument, WebSessionDocument, OperatorSessionDocument } from '../../models/auth_models.js';
import { SessionType, SessionEventType, SessionKeyPrefix } from '../../constants/session.js';
import { Environment } from '../../constants/ai.js';
import { SESSION_TTL_SECONDS } from '../../constants/session.js';

/**
 * Build a canonical session ID of the form `{prefix}_{ms}_{uuid}` where the
 * prefix is the shared wire constant in `status.json::session.key.prefix`.
 *
 * Single source of truth for session-id format; used by every session service
 * and by test helpers so no caller can drift from the production shape.
 *
 * @param {string} sessionType One of `SessionType.WEB|OPERATOR|CLI`.
 * @returns {string} Canonical session id.
 */
export function generateSessionId(sessionType) {
    const prefix = SessionKeyPrefix[sessionType];
    if (!prefix) {
        throw new Error(`generateSessionId: unknown session type ${JSON.stringify(sessionType)}`);
    }
    return `${prefix}_${Date.now()}_${randomUUID()}`;
}

export class BaseSessionService {
    constructor(options = {}) {
        if (!options.cacheAsideService) {
            throw new Error('cacheAsideService is required');
        }

        const settings = options.settings || {};
        this._cache_aside = options.cacheAsideService;
        this.bootstrapService = options.bootstrapService;
        this.sessionTTL = options.sessionTTL || (parseInt(settings.session_ttl) || SESSION_TTL_SECONDS);

        logger.info('[SESSION-SERVICE] Initializing with bootstrap service', {
            hasBootstrapService: !!this.bootstrapService,
            settings: Object.keys(settings)
        });

        this.encryptionKey = null;
        this.healthy = true;
    }

    /**
     * Get the session encryption key from bootstrap service.
     * Loads on-demand to eliminate config object dependency.
     * @returns {string}
     */
    _getEncryptionKey() {
        if (this.encryptionKey) {
            return this.encryptionKey;
        }

        if (!this.bootstrapService) {
            throw new Error('[SESSION-SERVICE] BootstrapService is required for session encryption');
        }

        this.encryptionKey = this.bootstrapService.loadSessionEncryptionKey();
        if (!this.encryptionKey) {
            throw new Error('[SESSION-SERVICE] SESSION_ENCRYPTION_KEY must be set');
        }

        logger.info('[SESSION-SERVICE] Session encryption key loaded from bootstrap service', {
            keyLength: this.encryptionKey.length
        });

        return this.encryptionKey;
    }

    _generateSessionId() {
        return `session_${Date.now()}_${randomUUID()}`;
    }

    /**
     * Encrypt sensitive field using AES-256-GCM.
     * Returns null for undefined/null values.
     */
    _encryptField(value) {
        if (!value) {
            return null;
        }
        
        const encryptionKey = this._getEncryptionKey();
        if (!encryptionKey) {
            throw new Error('[SESSION-SERVICE] Cannot encrypt field: SESSION_ENCRYPTION_KEY is not set');
        }

        const iv = randomBytes(16);
        const cipher = createCipheriv('aes-256-gcm', Buffer.from(encryptionKey, 'hex'), iv);
        let encrypted = cipher.update(JSON.stringify(value), 'utf8', 'hex');
        encrypted += cipher.final('hex');
        const authTag = cipher.getAuthTag();

        return {
            encrypted: true,
            data: encrypted,
            iv: iv.toString('hex'),
            authTag: authTag.toString('hex')
        };
    }

    /**
     * Decrypt sensitive field using AES-256-GCM.
     */
    _decryptField(encryptedData) {
        if (!encryptedData || !encryptedData.encrypted) {
            return encryptedData;
        }

        try {
            const encryptionKey = this._getEncryptionKey();
            if (!encryptionKey) {
                return encryptedData;
            }

            const decipher = createDecipheriv(
                'aes-256-gcm',
                Buffer.from(encryptionKey, 'hex'),
                Buffer.from(encryptedData.iv, 'hex')
            );
            decipher.setAuthTag(Buffer.from(encryptedData.authTag, 'hex'));

            let decrypted = decipher.update(encryptedData.data, 'hex', 'utf8');
            decrypted += decipher.final('utf8');

            return JSON.parse(decrypted);
        } catch (error) {
            logger.error('[SESSION-SERVICE] Decryption failed', { error: error.message });
            return null;
        }
    }

    _decryptSessionFields(session) {
        if (!session) return null;

        return {
            ...session,
            api_key: this._decryptField(session.api_key)
        };
    }

    _toSessionDocument(sessionData) {
        if (sessionData instanceof SessionDocument) return sessionData;
        const type = sessionData?.session_type;
        if (type === SessionType.OPERATOR) return OperatorSessionDocument.parse(sessionData);
        if (type === SessionType.WEB) return WebSessionDocument.parse(sessionData);
        return SessionDocument.parse(sessionData);
    }

    /**
     * Log session event to g8es audit trail (async, non-blocking).
     * Events are stored as an array within the session document itself.
     */
    async _logSessionEvent(eventType, session, metadata = {}) {
        try {
            const event = {
                event_type: eventType,
                timestamp: now(),
                client_ip: session.client_ip,
                user_agent: session.user_agent,
                metadata: { operator_id: session.operator_id || null, ...metadata }
            };
            const existing = await this._cache_aside.getDocument(this.sessionsCollection, session.id);
            const events = Array.isArray(existing?.events) ? [...existing.events, event] : [event];
            await this._cache_aside.updateDocument(this.sessionsCollection, session.id, { events });
        } catch (err) {
            logger.error('[SESSION-SERVICE] Failed to write session event', { error: err.message, eventType });
        }
    }

    /**
     * SECURITY: Validate session integrity — ensures all required fields exist.
     * Prevents tampered, malformed, or incomplete sessions from being accepted.
     *
     * @param {Object} session
     * @param {string} sessionId
     * @returns {{ valid: boolean, reason?: string }}
     */
    _validateSessionIntegrity(session, sessionId) {
        const baseRequiredFields = [
            'id',
            'session_type',
            'is_active',
            'created_at',
            'absolute_expires_at'
        ];

        for (const field of baseRequiredFields) {
            if (session[field] === undefined || session[field] === null) {
                return { valid: false, reason: `missing_required_field:${field}` };
            }
        }

        if (session.id !== sessionId) {
            return { valid: false, reason: 'session_id_mismatch' };
        }

        if (session.is_active !== true) {
            return { valid: false, reason: 'session_inactive' };
        }

        const validSessionTypes = [SessionType.WEB, SessionType.OPERATOR];
        if (!validSessionTypes.includes(session.session_type)) {
            return { valid: false, reason: `invalid_session_type:${session.session_type}` };
        }

        if (session.session_type === SessionType.WEB) {
            if (!session.user_data) {
                return { valid: false, reason: 'web_session_missing:user_data' };
            }
            if (!session.user_data?.email) {
                return { valid: false, reason: 'web_session_missing:user_data.email' };
            }
            if (session.user_id === undefined || session.user_id === null) {
                return { valid: false, reason: 'web_session_missing:user_id' };
            }
            if (typeof session.user_id !== 'string' || session.user_id.trim() === '') {
                return { valid: false, reason: 'invalid_user_id' };
            }
        }
        
        return { valid: true };
    }

    isHealthy() {
        return this.healthy;
    }

    async waitForReady() {
        logger.info('[SESSION-SERVICE] Session service ready (G8es-backed)');
        return true;
    }

    async close() {
        // KVCacheClient lifecycle managed externally
    }
}
