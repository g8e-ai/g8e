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
import { now, addSeconds, toISOString } from '../../models/base.js';
import { DeviceLinkData, DeviceLinkClaim } from '../../models/auth_models.js';
import { logger } from '../../utils/logger.js';
import { OperatorStatus, OperatorType } from '../../constants/operator.js';
import { DeviceLinkStatus } from '../../constants/auth.js';
import { KVKey } from '../../constants/kv_keys.js';
import { TokenFormat, DeviceLinkError, DEFAULT_DEVICE_LINK_MAX_USES, DEVICE_LINK_MAX_USES_MIN, DEVICE_LINK_MAX_USES_MAX } from '../../constants/auth.js';
import { DEVICE_LINK_TTL_SECONDS, DEVICE_LINK_TTL_MIN_SECONDS, DEVICE_LINK_TTL_MAX_SECONDS, LOCK_TTL_MS, LOCK_RETRY_DELAY_MS, LOCK_MAX_RETRIES } from '../../constants/auth.js';

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
     */
    constructor({ cacheAsideService, operatorService, webSessionService, deviceRegistrationService } = {}) {
        if (!cacheAsideService)         throw new Error('cacheAsideService is required');
        if (!operatorService)           throw new Error('operatorService is required');
        if (!webSessionService)         throw new Error('webSessionService is required');
        if (!deviceRegistrationService) throw new Error('deviceRegistrationService is required');

        this._cache_aside                   = cacheAsideService;
        this._operatorService         = operatorService;
        this._webSessionService       = webSessionService;
        this._deviceRegistration      = deviceRegistrationService;
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

    async createLink({ user_id, organization_id, name, max_uses = DEFAULT_DEVICE_LINK_MAX_USES, ttl_seconds = DEVICE_LINK_TTL_SECONDS }) {
        if (max_uses < DEVICE_LINK_MAX_USES_MIN || max_uses > DEVICE_LINK_MAX_USES_MAX) {
            return { success: false, error: DeviceLinkError.MAX_USES_INVALID };
        }

        if (ttl_seconds < DEVICE_LINK_TTL_MIN_SECONDS || ttl_seconds > DEVICE_LINK_TTL_MAX_SECONDS) {
            return { success: false, error: DeviceLinkError.TTL_INVALID };
        }

        const allOperators = await this._operatorService.queryOperators([{ field: 'user_id', operator: '==', value: user_id }]);
        const currentSlotCount = allOperators.filter(op => op.status !== OperatorStatus.TERMINATED).length;

        if (max_uses > currentSlotCount) {
            const slotsToCreate = max_uses - currentSlotCount;
            const createdSlotIds = [];

            for (let i = 0; i < slotsToCreate; i++) {
                const slotNumber = currentSlotCount + i + 1;
                const creationResponse = await this._operatorService.createOperatorSlot({
                    userId: user_id,
                    organizationId: organization_id,
                    slotNumber: slotNumber,
                    operatorType: OperatorType.SYSTEM,
                    cloudSubtype: null,
                    namePrefix: 'operator',
                    isG8eNode: false,
                });

                if (!creationResponse.success || !creationResponse.operator_id) {
                    for (const id of createdSlotIds) {
                        await this._operatorService.terminateOperator(id).catch(() => {});
                    }
                    return { success: false, error: DeviceLinkError.SLOT_CREATE_FAILED };
                }
                createdSlotIds.push(creationResponse.operator_id);
            }

            logger.info('[DEVICE-LINK] Generated operator slots for device link', {
                user_id,
                slots_created: slotsToCreate,
                total_slots: max_uses
            });
        }

        const token = `dlk_${crypto.randomBytes(24).toString('base64url')}`;
        const createdAt = now();
        const expiresAt = addSeconds(createdAt, ttl_seconds);

        const linkData = DeviceLinkData.parse({
            token,
            user_id,
            organization_id,
            name: name ? sanitizeString(name, 100) : null,
            max_uses,
            uses: 0,
            created_at: createdAt,
            expires_at: expiresAt,
            status: DeviceLinkStatus.ACTIVE,
            claims: []
        });

        await this._cache_aside.kvSetJson(KVKey.deviceLink(token), linkData.forKV(), ttl_seconds);
        await this._cache_aside.kvSadd(KVKey.deviceLinkList(user_id), token);

        const operatorCommand = `g8e.operator --device-token ${token}`;

        logger.info('[DEVICE-LINK] Link created', {
            token_prefix: token.substring(0, 20) + '...',
            user_id,
            max_uses,
            ttl_seconds
        });

        return {
            success: true,
            token,
            operator_command: operatorCommand,
            name: linkData.name,
            max_uses,
            expires_at: toISOString(linkData.expires_at)
        };
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
        const g8eContext = {
            web_session_id: linkData.web_session_id,
            user_id: linkData.user_id,
            organization_id: linkData.organization_id,
        };

        const result = await this._deviceRegistration.registerDevice({
            operator_id: linkData.operator_id,
            deviceInfo,
            operator_type: OperatorType.SYSTEM,
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
            operator_session_id: result.operator_session_id.substring(0, 12) + '...'
        });

        return {
            success: true,
            operator_session_id: result.operator_session_id,
            operator_id: result.operator_id
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
                g8eContext: {
                    web_session_id: webSessionId,
                    user_id: linkData.user_id,
                    organization_id: linkData.organization_id,
                },
            });
            return result;
        }

        // 2. New registration for THIS LINK.
        // Check if device already has a slot from a previous link or manual registration.
        const allOperators = await this._operatorService.queryOperators([{ field: 'user_id', operator: '==', value: linkData.user_id }]);
        const existingOp = allOperators.find(op => 
            op.system_fingerprint === sanitizedFingerprint && 
            op.status !== OperatorStatus.TERMINATED
        );

        // 3. Acquire distributed lock to prevent race condition where multiple
        // concurrent claims select the same available operator slot
        const lockKey = KVKey.deviceLinkRegistrationLock(token);
        const lockValue = `${token}:${sanitizedFingerprint}:${Date.now()}`;
        let lockAcquired = false;

        for (let attempt = 0; attempt < LOCK_MAX_RETRIES; attempt++) {
            const acquired = await this._cache_aside.kvSet(lockKey, lockValue, 'PX', LOCK_TTL_MS, 'NX');
            if (acquired) {
                lockAcquired = true;
                break;
            }
            await new Promise(resolve => setTimeout(resolve, LOCK_RETRY_DELAY_MS));
        }

        if (!lockAcquired) {
            logger.error('[DEVICE-LINK] Failed to acquire registration lock', {
                token_prefix: token.substring(0, 20) + '...',
                user_id: linkData.user_id
            });
            return { success: false, error: DeviceLinkError.REGISTRATION_BUSY };
        }

        // Fetch TTL after lock is acquired so it reflects the current remaining time.
        const linkTtl = await this._cache_aside.kvTtl(KVKey.deviceLink(token));

        try {
            let targetOperatorId = existingOp?.id;

            if (!targetOperatorId) {
                // Re-query operators to ensure we have latest state after lock
                const freshOperators = await this._operatorService.queryOperators([{ field: 'user_id', operator: '==', value: linkData.user_id }]);

                // Double check fingerprint match after lock
                const opAfterLock = freshOperators.find(op => 
                    op.system_fingerprint === sanitizedFingerprint && 
                    op.status !== OperatorStatus.TERMINATED
                );

                if (opAfterLock) {
                    targetOperatorId = opAfterLock.id;
                } else {
                    // Re-use an available operator if one exists
                    const availableOperator = freshOperators.find(op => op.status === OperatorStatus.AVAILABLE);
                    
                    if (availableOperator) {
                        targetOperatorId = availableOperator.id;
                    } else {
                        const creationResponse = await this._operatorService.createOperatorSlot({
                            userId: linkData.user_id,
                            organizationId: linkData.organization_id,
                            slotNumber: freshOperators.filter(op => op.status !== OperatorStatus.TERMINATED).length + 1,
                            operatorType: OperatorType.SYSTEM,
                            cloudSubtype: null,
                            namePrefix: 'operator',
                            isG8eNode: false,
                        });

                        if (!creationResponse.success || !creationResponse.operator_id) {
                            return { success: false, error: DeviceLinkError.SLOT_CREATE_FAILED };
                        }
                        targetOperatorId = creationResponse.operator_id;
                    }
                }
            }

            // Dedup check (fingerprint only — same physical device must not register twice on this link).
            // This MUST happen after lock acquisition and before use counter increment to prevent
            // race conditions where concurrent requests from the same device consume the quota.
            const fingerprintKey = KVKey.deviceLinkFingerprints(token);
            const fingerprintAdded = await this._cache_aside.kvSadd(fingerprintKey, sanitizedFingerprint);
            if (fingerprintAdded === 0) {
                return { success: false, error: DeviceLinkError.DEVICE_ALREADY_REGISTERED };
            }

            if (linkTtl > 0) {
                await this._cache_aside.kvExpire(fingerprintKey, linkTtl);
            }

            // Persist claim BEFORE incrementing use counter so concurrent requests
            // will see the existing claim and reuse the slot
            const freshLinkResult = await this.getLink(token);
            const freshLinkData = freshLinkResult.success ? freshLinkResult.data : linkData;

            // Check if claim was already added by a concurrent request
            const existingClaim = freshLinkData.claims.find(c => c.system_fingerprint === sanitizedFingerprint);
            if (existingClaim) {
                await this._cache_aside.kvSrem(fingerprintKey, sanitizedFingerprint);
                const webSessionId = await this._webSessionService.getUserActiveSession(linkData.user_id);
                const result = await this._deviceRegistration.registerDevice({
                    operator_id: existingClaim.operator_id,
                    deviceInfo,
                    g8eContext: {
                        web_session_id: webSessionId,
                        user_id: linkData.user_id,
                        organization_id: linkData.organization_id,
                    },
                });
                return result;
            }

            freshLinkData.claims.push(DeviceLinkClaim.parse({
                system_fingerprint: sanitizedFingerprint,
                hostname: sanitizeString(deviceInfo.hostname, 255),
                operator_id: targetOperatorId,
                claimed_at: now()
            }));

            freshLinkData.uses = freshLinkData.claims.length;

            if (freshLinkData.claims.length >= freshLinkData.max_uses) {
                freshLinkData.status = DeviceLinkStatus.EXHAUSTED;
            }

            const remainingTtl = linkTtl > 0 ? linkTtl : DEVICE_LINK_TTL_MIN_SECONDS;
            await this._cache_aside.kvSetJson(KVKey.deviceLink(token), freshLinkData.forKV(), remainingTtl);

            // Now increment use counter AFTER claim is persisted
            const usesKey = KVKey.deviceLinkUses(token);
            const newUsesCount = await this._cache_aside.kvIncr(usesKey);

            if (linkTtl > 0) {
                await this._cache_aside.kvExpire(usesKey, linkTtl);
            }

            const webSessionId = await this._webSessionService.getUserActiveSession(linkData.user_id);

            const g8eContext = {
                web_session_id: webSessionId,
                user_id: linkData.user_id,
                organization_id: linkData.organization_id,
            };

            const result = await this._deviceRegistration.registerDevice({
                operator_id: targetOperatorId,
                deviceInfo,
                g8eContext,
            });

            if (!result.success) {
                await this._cache_aside.kvDecr(usesKey);
                await this._cache_aside.kvSrem(fingerprintKey, sanitizedFingerprint);
                // Remove the claim we added
                freshLinkData.claims = freshLinkData.claims.filter(c => c.system_fingerprint !== sanitizedFingerprint);
                freshLinkData.uses = freshLinkData.claims.length;
                if (freshLinkData.claims.length < freshLinkData.max_uses) {
                    freshLinkData.status = DeviceLinkStatus.ACTIVE;
                }
                await this._cache_aside.kvSetJson(KVKey.deviceLink(token), freshLinkData.forKV(), remainingTtl);
                return result;
            }

            logger.info('[DEVICE-LINK] Device registered', {
                token_prefix: token.substring(0, 20) + '...',
                operator_id: result.operator_id,
                uses: `${newUsesCount}/${freshLinkData.max_uses}`
            });

            return {
                success: true,
                operator_session_id: result.operator_session_id,
                operator_id: result.operator_id
            };
        } finally {
            const currentValue = await this._cache_aside.kvGet(lockKey);
            if (currentValue === lockValue) {
                await this._cache_aside.kvDel(lockKey);
            }
        }
    }

    async listLinks(user_id) {
        const tokens = await this._cache_aside.kvSmembers(KVKey.deviceLinkList(user_id));
        const links = [];
        const expiredTokens = [];

        for (const token of tokens) {
            const stored = await this._cache_aside.kvGetJson(KVKey.deviceLink(token));
            if (!stored) {
                expiredTokens.push(token);
                continue;
            }

            const linkData = DeviceLinkData.fromKV(stored);
            const isExpired = linkData.expires_at < now();

            const serialized = linkData.forWire();
            links.push({
                token: serialized.token,
                name: serialized.name,
                max_uses: serialized.max_uses,
                uses: serialized.uses,
                status: isExpired ? DeviceLinkStatus.EXPIRED : serialized.status,
                created_at: serialized.created_at,
                expires_at: serialized.expires_at
            });
        }

        if (expiredTokens.length > 0) {
            await this._cache_aside.kvSrem(KVKey.deviceLinkList(user_id), ...expiredTokens);
        }

        links.sort((a, b) => (a.created_at < b.created_at ? 1 : -1));

        return { success: true, links };
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
        const linkResult = await this.getLink(token);
        if (!linkResult.success) {
            return linkResult;
        }

        const linkData = linkResult.data;

        if (linkData.user_id !== user_id) {
            return { success: false, error: DeviceLinkError.UNAUTHORIZED };
        }

        linkData.status = DeviceLinkStatus.REVOKED;
        linkData.revoked_at = now();

        await this._cache_aside.kvSetJson(KVKey.deviceLink(token), linkData.forKV(), DEVICE_LINK_TTL_MIN_SECONDS);
        await this._cache_aside.kvSrem(KVKey.deviceLinkList(user_id), token);

        logger.info('[DEVICE-LINK] Link revoked', {
            token_prefix: token.substring(0, 20) + '...',
            uses: `${linkData.uses}/${linkData.max_uses}`
        });

        return { success: true };
    }
}

export { DeviceLinkService };
export default DeviceLinkService;
