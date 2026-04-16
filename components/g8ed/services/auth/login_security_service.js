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
import { now, addSeconds } from '../../models/base.js';
import { AccountLockData, FailedAttemptsData, LoginAttemptEntry, IpTrackEntry, LoginAuditEntry, AuthAdminAuditEntry } from '../../models/auth_models.js';
import { LoginEventType } from '../../constants/auth.js';
import { Collections } from '../../constants/collections.js';
import { LoginSecurity } from '../../constants/rate_limits.js';
import { KVKey } from '../../constants/kv_keys.js';

const ACCOUNT_LOCKS_COLLECTION_BASE = Collections.ACCOUNT_LOCKS;
const MAX_FAILED_ATTEMPTS = LoginSecurity.MAX_FAILED_ATTEMPTS;
const PROGRESSIVE_DELAYS = LoginSecurity.PROGRESSIVE_DELAYS;
const FAILED_ATTEMPT_WINDOW = LoginSecurity.FAILED_ATTEMPT_WINDOW_SECONDS;
const CAPTCHA_THRESHOLD = LoginSecurity.CAPTCHA_THRESHOLD;
const ANOMALY_MULTI_ACCOUNT_THRESHOLD = LoginSecurity.ANOMALY_MULTI_ACCOUNT_THRESHOLD;
const ANOMALY_MULTI_ACCOUNT_WINDOW = LoginSecurity.ANOMALY_MULTI_ACCOUNT_WINDOW_SECONDS;

class LoginSecurityService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     * @param {Function} [options.geoipLookup] - GeoIP lookup function
     */
    constructor({ cacheAsideService, geoipLookup = null }) {
        if (!cacheAsideService) throw new Error('cacheAsideService is required');
        this._cache_aside = cacheAsideService;
        this.geoipLookup = geoipLookup;
    }

    generateDeviceFingerprint(req) {
        const components = [
            req.headers['user-agent'],
            req.headers['accept-language'],
            req.headers['accept-encoding'],
            req.headers['accept']
        ];
        
        const fingerprintString = components.join('|');
        return crypto.createHash('sha256').update(fingerprintString).digest('hex').substring(0, 32);
    }

    _makeLockDocId(identifier) {
        return crypto.createHash('sha256').update(identifier).digest('hex').substring(0, 32);
    }

    async isAccountLocked(identifier) {
        const dbId = this._makeLockDocId(identifier);

        const raw = await this._cache_aside.getDocument(ACCOUNT_LOCKS_COLLECTION_BASE, dbId);
        if (!raw) return { locked: false };

        const lockData = AccountLockData.parse(raw);
        logger.warn('[LOGIN-SECURITY] Account is locked', {
            identifier: this._redactIdentifier(identifier),
            locked_at: lockData.locked_at,
            failed_attempts: lockData.failed_attempts,
        });
        return {
            locked: true,
            reason: 'Account locked due to too many failed login attempts. Contact support to unlock.',
            locked_at: lockData.locked_at,
            failed_attempts: lockData.failed_attempts,
            last_attempt_ip: lockData.last_attempt_ip
        };
    }

    async getFailedAttemptStatus(identifier) {
        const failedKey = KVKey.loginFailed(identifier);
        const stored = await this._cache_aside.kvGetJson(failedKey);

        if (!stored) {
            return { attempts: 0, delay_ms: 0, requires_captcha: false };
        }

        const failedData = FailedAttemptsData.parse(stored);
        const attempts = failedData.count;
        const delay_ms = attempts < PROGRESSIVE_DELAYS.length
            ? PROGRESSIVE_DELAYS[attempts]
            : PROGRESSIVE_DELAYS[PROGRESSIVE_DELAYS.length - 1];
        const requires_captcha = attempts >= CAPTCHA_THRESHOLD;

        return { attempts, delay_ms, requires_captcha };
    }

    async recordFailedAttempt(identifier, requestContext = {}) {
        const failedKey = KVKey.loginFailed(identifier);
        const stored = await this._cache_aside.kvGetJson(failedKey);
        let attempts = 1;
        let attemptHistory = [];

        let existingData = null;
        if (stored) {
            existingData = FailedAttemptsData.parse(stored);
            attempts = existingData.count + 1;
            attemptHistory = existingData.history;
        }

        attemptHistory.push(LoginAttemptEntry.parse({
            timestamp: now(),
            ip: requestContext.ip,
            user_agent: requestContext.userAgent,
            device_fingerprint: requestContext.deviceFingerprint,
        }));

        if (attemptHistory.length > 10) {
            attemptHistory = attemptHistory.slice(-10);
        }

        if (attempts >= MAX_FAILED_ATTEMPTS) {
            const lockData = AccountLockData.parse({
                identifier,
                locked_at: now(),
                failed_attempts: attempts,
                last_attempt_ip: requestContext.ip,
                attempt_history: attemptHistory,
            });

            const dbId = this._makeLockDocId(identifier);
            await this._cache_aside.createDocument(ACCOUNT_LOCKS_COLLECTION_BASE, dbId, lockData);
            logger.info('[LOGIN-SECURITY] Account lock persisted to DB', {
                identifier: this._redactIdentifier(identifier)
            });

            const lockKey = KVKey.loginLock(identifier);
            await this._cache_aside.kvSetJson(lockKey, lockData.forKV());

            await this._cache_aside.kvDel(failedKey);

            logger.error('[LOGIN-SECURITY] Account LOCKED due to failed attempts', {
                identifier: this._redactIdentifier(identifier),
                attempts,
                ip: requestContext.ip
            });

            // Audit log the lockout
            await this._auditLoginEvent(LoginEventType.ACCOUNT_LOCKED, identifier, requestContext, {
                failed_attempts: attempts,
                attempt_history: attemptHistory
            });

            return { 
                locked: true, 
                attempts, 
                delay_ms: 0, 
                requires_captcha: false,
                message: 'Account locked due to too many failed login attempts. Contact support to unlock.'
            };
        }

        const failedData = FailedAttemptsData.parse({
            count: attempts,
            first_attempt: existingData ? existingData.first_attempt : now(),
            last_attempt: now(),
            history: attemptHistory,
        });
        await this._cache_aside.kvSetJson(failedKey, failedData.forKV(), FAILED_ATTEMPT_WINDOW);

        const delay_ms = attempts < PROGRESSIVE_DELAYS.length 
            ? PROGRESSIVE_DELAYS[attempts] 
            : PROGRESSIVE_DELAYS[PROGRESSIVE_DELAYS.length - 1];
        const requires_captcha = attempts >= CAPTCHA_THRESHOLD;

        logger.warn('[LOGIN-SECURITY] Failed login attempt recorded', {
            identifier: this._redactIdentifier(identifier),
            attempts,
            max_attempts: MAX_FAILED_ATTEMPTS,
            delay_ms,
            requires_captcha,
            ip: requestContext.ip
        });

        // Audit log the failed attempt
        await this._auditLoginEvent(LoginEventType.LOGIN_FAILED, identifier, requestContext, {
            attempt_number: attempts,
            remaining_attempts: MAX_FAILED_ATTEMPTS - attempts
        });

        return { locked: false, attempts, delay_ms, requires_captcha };
    }

    async clearFailedAttempts(identifier) {
        const failedKey = KVKey.loginFailed(identifier);
        await this._cache_aside.kvDel(failedKey);

        logger.info('[LOGIN-SECURITY] Failed attempts cleared on successful login', {
            identifier: this._redactIdentifier(identifier)
        });
    }

    async unlockAccount(identifier, adminUserId = null) {
        const dbId = this._makeLockDocId(identifier);

        const lockData = await this._cache_aside.getDocument(ACCOUNT_LOCKS_COLLECTION_BASE, dbId);

        if (!lockData) {
            return { success: false, error: 'Account is not locked' };
        }

        await this._cache_aside.deleteDocument(ACCOUNT_LOCKS_COLLECTION_BASE, dbId);
        await this._cache_aside.kvDel(KVKey.loginLock(identifier));

        logger.info('[LOGIN-SECURITY] Account manually unlocked from KV and DB', {
            identifier: this._redactIdentifier(identifier),
            unlocked_by: adminUserId
        });

        // Audit log the unlock
        await this._auditLoginEvent(LoginEventType.ACCOUNT_UNLOCKED, identifier, {}, {
            unlocked_by: adminUserId,
            previous_lock_data: lockData
        });

        return { success: true };
    }

    async trackIpAccount(ip, identifier) {
        const ipKey = KVKey.loginIpAccounts(ip);
        const trackTime = now();
        const cutoff = addSeconds(trackTime, -ANOMALY_MULTI_ACCOUNT_WINDOW);

        let entries = [];
        const rawIp = await this._cache_aside.kvGetJson(ipKey);
        if (rawIp) {
            try {
                entries = rawIp.map(e => IpTrackEntry.parse(e));
            } catch { /* ignore parse errors */ }
        }

        entries = entries.filter(e => e.ts > cutoff && e.id !== identifier);
        entries.push(IpTrackEntry.parse({ id: identifier, ts: trackTime }));

        await this._cache_aside.kvSetJson(
            ipKey,
            entries.map(e => e.forKV()),
            ANOMALY_MULTI_ACCOUNT_WINDOW
        );
    }

    async detectAnomalies(identifier, requestContext = {}, userHistory = null) {
        const anomalies = [];
        let risk_score = 0;
        const ip = requestContext.ip;

        if (ip) {
            const ipKey = KVKey.loginIpAccounts(ip);
            let accountCount = 0;
            const rawZset = await this._cache_aside.kvGetJson(ipKey);
            if (rawZset) {
                accountCount = Array.isArray(rawZset) ? rawZset.length : 0;
            }
            
            if (accountCount >= ANOMALY_MULTI_ACCOUNT_THRESHOLD) {
                anomalies.push(`multiple_accounts_same_ip:${accountCount}`);
                risk_score += 30;
                
                logger.warn('[LOGIN-SECURITY] Anomaly: Multiple accounts from same IP', {
                    ip,
                    account_count: accountCount,
                    threshold: ANOMALY_MULTI_ACCOUNT_THRESHOLD
                });
            }
        }

        if (userHistory && requestContext.deviceFingerprint) {
            const knownFingerprints = userHistory.known_device_fingerprints;
            if (knownFingerprints.length > 0 && !knownFingerprints.includes(requestContext.deviceFingerprint)) {
                anomalies.push('new_device');
                risk_score += 20;
                
                logger.info('[LOGIN-SECURITY] Anomaly: New device detected', {
                    identifier: this._redactIdentifier(identifier),
                    new_fingerprint: requestContext.deviceFingerprint.substring(0, 8) + '...'
                });
            }
        }

        if (userHistory && userHistory.typical_login_hours) {
            const currentHour = now().getUTCHours();
            const typicalHours = userHistory.typical_login_hours;
            
            if (!typicalHours.includes(currentHour)) {
                anomalies.push(`unusual_time:${currentHour}UTC`);
                risk_score += 10;
            }
        }

        if (this.geoipLookup && ip && userHistory && userHistory.typical_countries) {
            try {
                const geoData = await this.geoipLookup(ip);
                if (geoData && geoData.country && !userHistory.typical_countries.includes(geoData.country)) {
                    anomalies.push(`new_country:${geoData.country}`);
                    risk_score += 25;
                    
                    logger.warn('[LOGIN-SECURITY] Anomaly: Login from new country', {
                        identifier: this._redactIdentifier(identifier),
                        country: geoData.country,
                        typical_countries: userHistory.typical_countries
                    });
                }
            } catch (error) {
                logger.warn('[LOGIN-SECURITY] GeoIP lookup failed', { error: error.message });
            }
        }

        return { anomalies, risk_score };
    }

    async preLoginCheck(identifier, requestContext = {}) {
        // Check if account is locked
        const lockStatus = await this.isAccountLocked(identifier);
        if (lockStatus.locked) {
            return {
                allowed: false,
                delay_ms: 0,
                requires_captcha: false,
                error: lockStatus.reason,
                locked: true,
                locked_at: lockStatus.locked_at
            };
        }

        const attemptStatus = await this.getFailedAttemptStatus(identifier);
        await this.trackIpAccount(requestContext.ip, identifier);

        return {
            allowed: true,
            delay_ms: attemptStatus.delay_ms,
            requires_captcha: attemptStatus.requires_captcha,
            current_attempts: attemptStatus.attempts,
            max_attempts: MAX_FAILED_ATTEMPTS
        };
    }

    async postLoginSuccess(identifier, requestContext = {}) {
        await this.clearFailedAttempts(identifier);
        await this._auditLoginEvent(LoginEventType.LOGIN_SUCCESS, identifier, requestContext, {});
        const anomalyResult = await this.detectAnomalies(identifier, requestContext);
        
        if (anomalyResult.anomalies.length > 0) {
            logger.warn('[LOGIN-SECURITY] Login successful but anomalies detected', {
                identifier: this._redactIdentifier(identifier),
                anomalies: anomalyResult.anomalies,
                risk_score: anomalyResult.risk_score
            });

            await this._auditLoginEvent(LoginEventType.LOGIN_ANOMALY, identifier, requestContext, {
                anomalies: anomalyResult.anomalies,
                risk_score: anomalyResult.risk_score
            });
        }

        return { anomalies: anomalyResult.anomalies, risk_score: anomalyResult.risk_score };
    }

    async _auditLoginEvent(eventType, identifier, requestContext = {}, metadata = {}) {
        try {
            const auditId = `login_${eventType}_${Date.now()}_${crypto.randomBytes(4).toString('hex')}`;
            const auditLog = LoginAuditEntry.parse({
                event_type: eventType,
                identifier: identifier,
                identifier_redacted: this._redactIdentifier(identifier),
                timestamp: now(),
                ip: requestContext.ip || null,
                user_agent: requestContext.userAgent || null,
                device_fingerprint: requestContext.deviceFingerprint || null,
                metadata: metadata
            });

            try {
                const result = await this._cache_aside.createDocument(Collections.LOGIN_AUDIT, auditId, auditLog);
                if (!result.success) {
                    throw new Error(result.error);
                }
            } catch (err) {
                logger.error('[LOGIN-SECURITY] Failed to write audit log', {
                    error: err.message,
                    eventType,
                    auditId
                });
            }
        } catch (error) {
            logger.error('[LOGIN-SECURITY] Audit logging error', { error: error.message });
        }
    }

    _redactIdentifier(identifier) {
        if (!identifier) return 'None';
        
        const str = String(identifier);
        if (str.includes('@')) {
            const [local, domain] = str.split('@');
            return `${local.substring(0, 3)}***@${domain}`;
        }
        
        return `${str.substring(0, 6)}...`;
    }

    async getLockedAccounts() {
        const results = await this._cache_aside.queryDocuments(ACCOUNT_LOCKS_COLLECTION_BASE, []);
        return results.map(raw => AccountLockData.parse(raw));
    }

    isHealthy() {
        return !!this._cache_aside;
    }

    async auditAdminAccess(adminContext = {}) {
        try {
            const auditId = `admin_access_${Date.now()}_${crypto.randomBytes(4).toString('hex')}`;
            const auditLog = AuthAdminAuditEntry.parse({
                event_type: 'admin_access',
                action: adminContext.action || 'unknown',
                timestamp: now(),
                user_id: adminContext.userId || null,
                user_email: adminContext.userEmail || null,
                ip: adminContext.ip || null,
                user_agent: adminContext.userAgent || null,
                path: adminContext.path || null,
                method: adminContext.method || null,
                query_params: adminContext.queryParams || null,
                metadata: adminContext.metadata || {}
            });

            try {
                const result = await this._cache_aside.createDocument(Collections.AUTH_ADMIN_AUDIT, auditId, auditLog);
                if (!result.success) {
                    throw new Error(result.error);
                }
            } catch (err) {
                logger.error('[LOGIN-SECURITY] Failed to write admin audit log', {
                    error: err.message,
                    action: adminContext.action,
                    auditId
                });
            }
        } catch (error) {
            logger.error('[LOGIN-SECURITY] Admin audit logging error', { error: error.message });
        }
    }
}

export { LoginSecurityService };
