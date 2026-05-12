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
 * Device Link Service
 *
 * Multi-use device link for deploying the Operator binary to one or many systems.
 *
 * Flow:
 * 1. User creates a device link (max_uses, expiry)
 * 2. User distributes the command to N systems
 * 3. Each system runs the command, registers device info
 * 4. Service finds an available Operator slot, delegates to DeviceRegistrationService
 * 5. Operator binary starts with credentials
 *
 * This service handles: token generation, usage tracking, fingerprint deduplication.
 * DeviceRegistrationService handles: operator claiming, session creation, activation, SSE.
 */

import crypto from 'crypto';
import { sessionIdTag } from '../../utils/session_log.js';
import { now, addSeconds, toISOString } from '../../models/base.js';
import { DeviceLinkData, DeviceLinkClaim } from '../../models/auth_models.js';
import { logger } from '../../utils/logger.js';
import { OperatorStatus, OperatorType } from '../../constants/operator.js';
import { DeviceLinkStatus } from '../../constants/auth.js';
import { KVKey } from '../../constants/kv_keys.js';
import { TokenFormat, DeviceLinkError, DEVICE_LINK_TTL_SECONDS, DEVICE_LINK_TTL_MIN_SECONDS, DEVICE_LINK_TTL_MAX_SECONDS, DEVICE_LINK_MAX_USES, LOCK_TTL_MS, LOCK_RETRY_DELAY_MS, LOCK_MAX_RETRIES } from '../../constants/auth.js';
import { G8eHttpContext } from '../../models/request_models.js';

export function isValidTokenFormat(token) {
    if (!token || typeof token !== 'string') return false;
    return TokenFormat.DEVICE_LINK.test(token);
}

export function isValidDownloadTokenFormat(token) {
    if (!token || typeof token !== 'string') return false;
    return TokenFormat.DOWNLOAD_TOKEN.test(token);
}

function sanitizeString(input, maxLength = 255) {
    if (!input || typeof input !== 'string') return '';
    // Strip control characters and trim.
    // NOTE: Do NOT HTML-escape here. Escaping is a render-time concern.
    return input
        .slice(0, maxLength)
        .replace(/[\x00-\x1F\x7F]/g, '')
        .trim();
}

function sanitizeFingerprint(input) {
    if (!input || typeof input !== 'string') return '';
    return input.replace(/[^0-9a-fA-F]/g, '').slice(0, 128).toLowerCase();
}

class DeviceLinkService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService        - CacheAsideService instance
     * @param {Object} options.operatorService          - OperatorDataService instance
     * @param {Object} options.webSessionService        - WebSessionService instance
     * @param {Object} options.deviceRegistrationService - DeviceRegistrationService instance
     * @param {Object} options.internalHttpClient       - InternalHttpClient instance
     */
    constructor({ cacheAsideService, operatorService, webSessionService, deviceRegistrationService, internalHttpClient } = {}) {
        if (!cacheAsideService)         throw new Error('cacheAsideService is required');
        if (!operatorService)           throw new Error('operatorService is required');
        if (!webSessionService)         throw new Error('webSessionService is required');
        if (!deviceRegistrationService) throw new Error('deviceRegistrationService is required');
        if (!internalHttpClient)       throw new Error('internalHttpClient is required');

        this._cache_aside                   = cacheAsideService;
        this._operatorService         = operatorService;
        this._webSessionService       = webSessionService;
        this._deviceRegistration      = deviceRegistrationService;
        this._internalHttpClient      = internalHttpClient;
    }

    async generateLink({ user_id, organization_id, operator_id, web_session_id }) {
        const operator = await this._operatorService.getOperator(operator_id);
        if (!operator) {
            return { success: false, error: DeviceLinkError.OPERATOR_NOT_FOUND };
        }

        if (operator.user_id !== user_id) {
            return { success: false, error: DeviceLinkError.OPERATOR_WRONG_USER };
        }

        if (operator.status === OperatorStatus.TERMINATED) {
            return { success: false, error: DeviceLinkError.OPERATOR_TERMINATED };
        }

        const token = `dlk_${crypto.randomBytes(24).toString('base64url')}`;
        const createdAt = now();
        const expiresAt = addSeconds(createdAt, DEVICE_LINK_TTL_SECONDS);

        const linkData = DeviceLinkData.parse({
            token,
            user_id,
            organization_id,
            operator_id,
            web_session_id,
            status: DeviceLinkStatus.PENDING,
            created_at: createdAt,
            expires_at: expiresAt,
        });

        await this._cache_aside.kvSetJson(KVKey.deviceLink(token), linkData.forKV(), DEVICE_LINK_TTL_SECONDS);
        await this._cache_aside.kvSadd(KVKey.deviceLinkList(user_id), token);

        const operatorCommand = `g8e.operator --device-token ${token}`;

        logger.info('[DEVICE-LINK] Single-operator link generated', {
            token_prefix: token.substring(0, 20) + '...',
            user_id,
            operator_id
        });

        return {
            success: true,
            token,
            operator_command: operatorCommand,
            expires_at: toISOString(linkData.expires_at)
        };
    }

    async cancelLink(token, user_id) {
        const stored = await this._cache_aside.kvGetJson(KVKey.deviceLink(token));
        if (!stored) {
            return { success: true };
        }

        const linkData = DeviceLinkData.fromKV(stored);

        if (linkData.user_id !== user_id) {
            return { success: false, error: DeviceLinkError.UNAUTHORIZED };
        }

        await this._cache_aside.kvDel(KVKey.deviceLink(token));
        await this._cache_aside.kvSrem(KVKey.deviceLinkList(user_id), token);

        logger.info('[DEVICE-LINK] Link cancelled', {
            token_prefix: token.substring(0, 20) + '...'
        });

        return { success: true };
    }

    async createLink({ user_id, organization_id, name, max_uses, ttl_seconds = DEVICE_LINK_TTL_SECONDS, webSessionId = null }) {
        if (max_uses === undefined || max_uses === null) {
            return { success: false, error: DeviceLinkError.MAX_USES_INVALID };
        }
        if (max_uses < 1 || max_uses > DEVICE_LINK_MAX_USES) {
            return { success: false, error: DeviceLinkError.MAX_USES_INVALID };
        }

        if (ttl_seconds < DEVICE_LINK_TTL_MIN_SECONDS || ttl_seconds > DEVICE_LINK_TTL_MAX_SECONDS) {
            return { success: false, error: DeviceLinkError.TTL_INVALID };
        }

        try {
            // Call substrate-owned create route
            const response = await this._internalHttpClient.createDeviceLink({
                user_id,
                organization_id,
                name: name ? sanitizeString(name, 100) : null,
                max_uses,
                ttl_seconds,
                web_session_id: webSessionId
            });

            if (!response.success) {
                throw new Error(response.error || 'Failed to create device link via substrate');
            }

            logger.info('[DEVICE-LINK] Link created via substrate', {
                token_prefix: response.token.substring(0, 20) + '...',
                user_id,
                max_uses,
                ttl_seconds
            });

            return response;

        } catch (err) {
            logger.error('[DEVICE-LINK] Failed to create link via substrate', {
                user_id,
                error: err.message,
            });
            return { success: false, error: err.message };
        }
    }

    async getLink(token) {
        if (!isValidTokenFormat(token)) {
            return { success: false, error: DeviceLinkError.INVALID_TOKEN_FORMAT };
        }

        const stored = await this._cache_aside.kvGetJson(KVKey.deviceLink(token));
        if (!stored) {
            return { success: false, error: DeviceLinkError.LINK_NOT_FOUND };
        }

        const linkData = DeviceLinkData.fromKV(stored);
        if (linkData.expires_at < now()) {
            return { success: false, error: DeviceLinkError.LINK_EXPIRED };
        }

        return { success: true, data: linkData };
    }

    async _registerSingleOperatorLink(token, linkData, deviceInfo, sanitizedFingerprint) {
        const g8eContext = G8eHttpContext.parse({
            web_session_id: linkData.web_session_id,
            user_id: linkData.user_id,
            organization_id: linkData.organization_id,
        });

        const result = await this._deviceRegistration.registerDevice({
            operator_id: linkData.operator_id,
            deviceInfo,
            operator_type: OperatorType.CLOUD,
            g8eContext,
        });

        if (!result.success) {
            return result;
        }

        linkData.status = DeviceLinkStatus.USED;
        linkData.used_at = now();
        linkData.device_info = {
            system_fingerprint: sanitizedFingerprint,
            hostname: sanitizeString(deviceInfo.hostname, 255),
            os: sanitizeString(deviceInfo.os, 32),
            arch: sanitizeString(deviceInfo.arch, 32),
            username: sanitizeString(deviceInfo.username, 255),
        };

        const linkTtl = await this._cache_aside.kvTtl(KVKey.deviceLink(token));
        if (linkTtl === -2) {
            return { success: false, error: DeviceLinkError.LINK_NOT_FOUND };
        }
        const remainingTtl = linkTtl > 0 ? linkTtl : DEVICE_LINK_TTL_MIN_SECONDS;
        await this._cache_aside.kvSetJson(KVKey.deviceLink(token), linkData.forKV(), remainingTtl);

        logger.info('[DEVICE-LINK] Single-operator device registered', {
            token_prefix: token.substring(0, 20) + '...',
            operator_id: result.operator_id,
            operator_session_id_tag: sessionIdTag(result.operator_session_id)
        });

        return {
            success: true,
            operator_session_id: result.operator_session_id,
            operator_id: result.operator_id,
            api_key: result.api_key,
            operator_cert: result.operator_cert,
            operator_cert_key: result.operator_cert_key,
            session: result.session,
            config: result.config
        };
    }

    async registerDevice(token, deviceInfo) {
        if (!isValidTokenFormat(token)) {
            return { success: false, error: DeviceLinkError.INVALID_TOKEN_FORMAT };
        }

        if (!deviceInfo.system_fingerprint) {
            return { success: false, error: DeviceLinkError.MISSING_FINGERPRINT };
        }

        const sanitizedFingerprint = sanitizeFingerprint(deviceInfo.system_fingerprint);
        if (!sanitizedFingerprint) {
            return { success: false, error: DeviceLinkError.INVALID_FINGERPRINT };
        }

        const linkResult = await this.getLink(token);
        if (!linkResult.success) {
            return linkResult;
        }
        const linkData = linkResult.data;

        if (linkData.status === DeviceLinkStatus.REVOKED) {
            return { success: false, error: DeviceLinkError.LINK_REVOKED };
        }

        if (linkData.status === DeviceLinkStatus.USED) {
            return { success: false, error: DeviceLinkError.LINK_ALREADY_USED };
        }

        if (linkData.status === DeviceLinkStatus.EXHAUSTED) {
            return { success: false, error: DeviceLinkError.LINK_EXHAUSTED };
        }

        if (linkData.status === DeviceLinkStatus.PENDING) {
            return this._registerSingleOperatorLink(token, linkData, deviceInfo, sanitizedFingerprint);
        }

        if (linkData.status !== DeviceLinkStatus.ACTIVE) {
            return { success: false, error: `Device link is ${linkData.status}` };
        }

        // 1. Check if this device has already claimed a slot VIA THIS LINK
        // If it has, we bypass usage checks and reuse the same slot.
        const existingClaim = linkData.claims.find(c => c.system_fingerprint === sanitizedFingerprint);
        
        if (existingClaim) {
            const webSessionId = await this._webSessionService.getUserActiveSession(linkData.user_id);
            const result = await this._deviceRegistration.registerDevice({
                operator_id: existingClaim.operator_id,
                deviceInfo,
                g8eContext: G8eHttpContext.parse({
                    web_session_id: webSessionId,
                    user_id: linkData.user_id,
                    organization_id: linkData.organization_id,
                }),
            });
            return result;
        }

        // 2. g8ee is authoritative for slot management - always pass null operator_id
        // g8ee will find existing slot by fingerprint or create new slot on-demand
        const targetOperatorId = null;

        // 3. Acquire distributed lock for fingerprint deduplication
        const fingerprintKey = KVKey.deviceLinkFingerprints(token);
        const lockKey = KVKey.deviceLinkRegistrationLock(token);
        const lockValue = `${token}:${sanitizedFingerprint}:${Date.now()}`;
        let lockAcquired = false;

        // Dedup check (fingerprint only — same physical device must not register twice on this link).
        // Using SADD is atomic and prevents multiple registrations from the same device.
        const fingerprintAdded = await this._cache_aside.kvSadd(fingerprintKey, sanitizedFingerprint);
        
        if (fingerprintAdded === 0) {
            // Already registered or registration in progress.
            // Poll for the claim to appear in linkData (handles race where SADD finished but claim not yet written).
            for (let i = 0; i < 10; i++) {
                const freshLink = await this.getLink(token);
                if (freshLink.success) {
                    const claim = freshLink.data.claims.find(c => c.system_fingerprint === sanitizedFingerprint);
                    if (claim) {
                        const webSessionId = await this._webSessionService.getUserActiveSession(linkData.user_id);
                        return await this._deviceRegistration.registerDevice({
                            operator_id: claim.operator_id,
                            deviceInfo,
                            g8eContext: G8eHttpContext.parse({
                                web_session_id: webSessionId,
                                user_id: linkData.user_id,
                                organization_id: linkData.organization_id,
                            }),
                            device_link_token: token,
                        });
                    }
                }
                await new Promise(resolve => setTimeout(resolve, 500));
            }
            logger.error('[DEVICE-LINK] Device already registered - claim not found after polling', {
                token_prefix: token.substring(0, 20) + '...',
                fingerprint_prefix: sanitizedFingerprint.substring(0, 16),
                poll_attempts: 10
            });
            return { success: false, error: DeviceLinkError.DEVICE_ALREADY_REGISTERED };
        }

        // Atomic usage check before calling g8ee
        const currentUsage = await this._cache_aside.kvScard(fingerprintKey);
        logger.info('[DEVICE-LINK] Usage check', {
            token_prefix: token.substring(0, 20) + '...',
            fingerprint_prefix: sanitizedFingerprint.substring(0, 16),
            current_usage: currentUsage,
            max_uses: linkData.max_uses
        });
        if (currentUsage > linkData.max_uses) {
            await this._cache_aside.kvSrem(fingerprintKey, sanitizedFingerprint);
            logger.error('[DEVICE-LINK] Link exhausted', {
                token_prefix: token.substring(0, 20) + '...',
                fingerprint_prefix: sanitizedFingerprint.substring(0, 16),
                current_usage: currentUsage,
                max_uses: linkData.max_uses
            });
            return { success: false, error: DeviceLinkError.LINK_EXHAUSTED };
        }

        // 4. g8ee is authoritative for slot management.
        // We call g8ee OUTSIDE the lock to allow high-concurrency parallel registration.
        // The lock is only used to protect the final linkData JSON update.
        const webSessionId = await this._webSessionService.getUserActiveSession(linkData.user_id);
        const g8eContext = G8eHttpContext.parse({
            web_session_id: webSessionId,
            user_id: linkData.user_id,
            organization_id: linkData.organization_id,
        });

        const result = await this._deviceRegistration.registerDevice({
            operator_id: targetOperatorId,
            deviceInfo,
            g8eContext,
            device_link_token: token,
        });

        if (!result.success) {
            await this._cache_aside.kvSrem(fingerprintKey, sanitizedFingerprint);
            return result;
        }

        // 5. Acquire lock to update linkData JSON blob
        // Adaptive retry: scale max retries based on device link width (max_uses)
        const concurrencyWidth = linkData.max_uses;
        const adaptiveMaxRetries = Math.min(
            LOCK_MAX_RETRIES + Math.ceil(concurrencyWidth * 2),
            LOCK_MAX_RETRIES * 3
        );

        for (let attempt = 0; attempt < adaptiveMaxRetries; attempt++) {
            const acquired = await this._cache_aside.kvSet(lockKey, lockValue, 'PX', LOCK_TTL_MS, 'NX');
            if (acquired) {
                lockAcquired = true;
                break;
            }
            const backoffMs = Math.min(LOCK_RETRY_DELAY_MS * Math.pow(1.5, Math.floor(attempt / 5)), LOCK_RETRY_DELAY_MS * 4);
            const jitter = Math.random() * 50;
            await new Promise(resolve => setTimeout(resolve, backoffMs + jitter));
        }

        if (!lockAcquired) {
            logger.error('[DEVICE-LINK] Failed to acquire registration lock for update', {
                token_prefix: token.substring(0, 20) + '...',
                user_id: linkData.user_id
            });
            // We successfully registered on g8ee, so we MUST try to persist the claim even if the lock failed
            // but for minimal changes, we'll return busy and let the device retry (it will find the claim via SADD check next time)
            return { success: false, error: DeviceLinkError.REGISTRATION_BUSY };
        }

        // Fetch TTL after lock is acquired
        const linkTtl = await this._cache_aside.kvTtl(KVKey.deviceLink(token));

        try {
            if (linkTtl > 0) {
                await this._cache_aside.kvExpire(fingerprintKey, linkTtl);
            }

            const freshLinkResult = await this.getLink(token);
            const freshLinkData = freshLinkResult.success ? freshLinkResult.data : linkData;

            // Check if claim was already added by a concurrent request
            const existingClaim = freshLinkData.claims.find(c => c.system_fingerprint === sanitizedFingerprint);
            if (existingClaim) {
                // Claim already exists, nothing to update
                return result;
            }

            // Add claim with actual operator_id from successful registration
            freshLinkData.claims.push(DeviceLinkClaim.parse({
                system_fingerprint: sanitizedFingerprint,
                hostname: sanitizeString(deviceInfo.hostname, 255),
                operator_id: result.operator_id,
                claimed_at: now()
            }));

            freshLinkData.uses = freshLinkData.claims.length;

            if (freshLinkData.claims.length >= freshLinkData.max_uses) {
                freshLinkData.status = DeviceLinkStatus.EXHAUSTED;
            }

            const remainingTtl = linkTtl > 0 ? linkTtl : DEVICE_LINK_TTL_MIN_SECONDS;
            await this._cache_aside.kvSetJson(KVKey.deviceLink(token), freshLinkData.forKV(), remainingTtl);

            logger.info('[DEVICE-LINK] Device registered and link updated', {
                token_prefix: token.substring(0, 20) + '...',
                operator_id: result.operator_id,
                uses: `${freshLinkData.uses}/${freshLinkData.max_uses}`
            });

            return {
                success: true,
                operator_session_id: result.operator_session_id,
                operator_id: result.operator_id,
                api_key: result.api_key,
                operator_cert: result.operator_cert,
                operator_cert_key: result.operator_cert_key,
                session: result.session,
                config: result.config
            };
        } finally {
            const currentValue = await this._cache_aside.kvGet(lockKey);
            if (currentValue === lockValue) {
                await this._cache_aside.kvDel(lockKey);
            }
        }
    }

    async listLinks(user_id) {
        try {
            // Call substrate-owned list route
            const response = await this._internalHttpClient.listDeviceLinks(user_id);
            
            if (!response.success) {
                throw new Error(response.error || 'Failed to list device links via substrate');
            }

            return response;

        } catch (err) {
            logger.error('[DEVICE-LINK] Failed to list links via substrate', {
                user_id,
                error: err.message,
            });
            return { success: false, error: err.message, links: [] };
        }
    }

    async deleteLink(token, user_id) {
        const stored = await this._cache_aside.kvGetJson(KVKey.deviceLink(token));
        if (!stored) {
            await this._cache_aside.kvSrem(KVKey.deviceLinkList(user_id), token);
            return { success: true };
        }

        const linkData = DeviceLinkData.fromKV(stored);

        if (linkData.user_id !== user_id) {
            return { success: false, error: DeviceLinkError.UNAUTHORIZED };
        }

        if (linkData.status === DeviceLinkStatus.ACTIVE && linkData.expires_at >= now()) {
            return { success: false, error: DeviceLinkError.CANNOT_DELETE_ACTIVE };
        }

        await this._cache_aside.kvDel(KVKey.deviceLink(token));
        await this._cache_aside.kvSrem(KVKey.deviceLinkList(user_id), token);

        logger.info('[DEVICE-LINK] Link deleted', {
            token_prefix: token.substring(0, 20) + '...',
            previous_status: linkData.status,
            uses: `${linkData.uses}/${linkData.max_uses}`
        });

        return { success: true };
    }

    async revokeLink(token, user_id) {
        try {
            // Call substrate-owned delete route (revoke behavior is default for active links in g8eo)
            const response = await this._internalHttpClient.deleteDeviceLink(token, user_id);
            
            if (!response.success) {
                throw new Error(response.error || 'Failed to revoke device link via substrate');
            }

            logger.info('[DEVICE-LINK] Link revoked via substrate', {
                token_prefix: token.substring(0, 20) + '...',
            });

            return { success: true };

        } catch (err) {
            logger.error('[DEVICE-LINK] Failed to revoke link via substrate', {
                token,
                user_id,
                error: err.message,
            });
            return { success: false, error: err.message };
        }
    }
}

export { DeviceLinkService };
export default DeviceLinkService;
