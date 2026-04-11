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
 * Passkey Authentication Service
 *
 * FIDO2/WebAuthn passwordless authentication — registration and assertion ceremonies.
 *
 * Registration flow:
 *   1. POST /api/auth/passkey/register-challenge  — generate and store challenge
 *   2. POST /api/auth/passkey/register-verify     — verify attestation, store credential
 *
 * Authentication flow:
 *   1. POST /api/auth/passkey/auth-challenge      — generate and store challenge (email lookup)
 *   2. POST /api/auth/passkey/auth-verify         — verify assertion, update counter, return user
 *
 * Challenge storage: VSODB KV with TTL (PASSKEY_CHALLENGE_TTL_SECONDS).
 * Credential storage: user document passkey_credentials array (write-through cache).
 */

import {
    generateRegistrationOptions,
    verifyRegistrationResponse,
    generateAuthenticationOptions,
    verifyAuthenticationResponse,
} from '@simplewebauthn/server';
import { isoBase64URL } from '@simplewebauthn/server/helpers';
import { randomUUID } from 'crypto';
import { logger } from '../../utils/logger.js';
import { now } from '../../models/base.js';
import { AuthProvider } from '../../constants/auth.js';
import { Collections } from '../../constants/collections.js';
import { CacheTTL } from '../../constants/service_config.js';
import { HTTP_X_FORWARDED_HOST_HEADER, HTTP_X_FORWARDED_PROTO_HEADER } from '../../constants/headers.js';

export const PASSKEY_CHALLENGE_TTL_SECONDS = CacheTTL.PASSKEY_CHALLENGE;

export class PasskeyAuthService {
    constructor({ userService, cacheAsideService, settingsService }) {
        if (!userService)        throw new Error('PasskeyAuthService requires userService');
        if (!cacheAsideService)  throw new Error('PasskeyAuthService requires cacheAsideService');
        if (!settingsService)    throw new Error('PasskeyAuthService requires settingsService');
        this._userService     = userService;
        this._cache_aside           = cacheAsideService;
        this._settingsService = settingsService;
    }

    async _getRpId(req) {
        const platformSettings = await this._settingsService.getPlatformSettings();
        const configRpId = platformSettings.passkey_rp_id || null;
        if (configRpId && configRpId !== 'localhost') {
            logger.info('[PASSKEY] getRpId: using settings passkey_rp_id', { rpId: configRpId });
            return configRpId;
        }
        const xForwardedHost = req?.get?.(HTTP_X_FORWARDED_HOST_HEADER);
        if (xForwardedHost) {
            const rpId = xForwardedHost.split(':')[0];
            logger.info('[PASSKEY] getRpId: using x-forwarded-host', { xForwardedHost, rpId });
            return rpId;
        }
        const rpId = req?.hostname || 'localhost';
        logger.info('[PASSKEY] getRpId: using req.hostname', { hostname: req?.hostname, rpId });
        return rpId;
    }

    async _getOrigin(req) {
        const platformSettings = await this._settingsService.getPlatformSettings();
        const configOrigin = platformSettings.passkey_origin || null;
        if (configOrigin && configOrigin !== 'https://localhost') {
            logger.info('[PASSKEY] getOrigin: using settings passkey_origin', { origin: configOrigin });
            return configOrigin;
        }
        const xForwardedProto = req?.get?.(HTTP_X_FORWARDED_PROTO_HEADER) || req?.protocol || 'https';
        const xForwardedHost  = req?.get?.(HTTP_X_FORWARDED_HOST_HEADER) || req?.get?.('host') || 'localhost';
        const origin = `${xForwardedProto}://${xForwardedHost}`;
        logger.info('[PASSKEY] getOrigin: derived from request', { xForwardedProto, xForwardedHost, origin });
        return origin;
    }

    async _getRpName() {
        const platformSettings = await this._settingsService.getPlatformSettings();
        return platformSettings.passkey_rp_name || 'g8e';
    }

    // -------------------------------------------------------------------------
    // Registration — challenge
    // -------------------------------------------------------------------------

    async generateRegistrationChallenge(req, user) {
        const rpId  = await this._getRpId(req);
        const existingCredentials = (user.passkey_credentials || []).map(c => ({
            id:         isoBase64URL.fromBuffer(Buffer.from(c.id, 'base64url')),
            type:       'public-key',
            transports: c.transports || [],
        }));

        const options = await generateRegistrationOptions({
            rpName:                    await this._getRpName(),
            rpID:                      rpId,
            userID:                    Buffer.from(user.id),
            userName:                  user.email,
            userDisplayName:           user.name || user.email,
            attestationType:           'none',
            excludeCredentials:        existingCredentials,
            authenticatorSelection: {
                residentKey:      'required',
                userVerification: 'required',
            },
            supportedAlgorithmIDs: [-7, -257],
        });

        await this._cache_aside.createDocument(
            Collections.PASSKEY_CHALLENGES,
            user.id,
            { challenge: options.challenge },
            PASSKEY_CHALLENGE_TTL_SECONDS
        );

        logger.info('[PASSKEY] Registration challenge generated', { userId: user.id });
        return options;
    }

    // -------------------------------------------------------------------------
    // Registration — verify
    // -------------------------------------------------------------------------

    async verifyRegistration(req, user, attestationResponse) {
        const rpId     = await this._getRpId(req);
        const origin   = await this._getOrigin(req);
        const doc = await this._cache_aside.getDocument(Collections.PASSKEY_CHALLENGES, user.id);
        const challenge = doc?.challenge;

        if (!challenge) {
            logger.warn('[PASSKEY] Registration verify: challenge expired or missing', { userId: user.id });
            return { verified: false, error: 'Challenge expired. Please try again.' };
        }

        let verification;
        try {
            verification = await verifyRegistrationResponse({
                response:             attestationResponse,
                expectedChallenge:    challenge,
                expectedOrigin:       origin,
                expectedRPID:         rpId,
                requireUserVerification: true,
            });
        } catch (error) {
            logger.warn('[PASSKEY] Registration verification failed', { userId: user.id, error: error.message });
            return { verified: false, error: 'Registration verification failed.' };
        }

        if (!verification.verified || !verification.registrationInfo) {
            logger.warn('[PASSKEY] Registration not verified', { userId: user.id });
            return { verified: false, error: 'Registration not verified.' };
        }

        await this._cache_aside.deleteDocument(Collections.PASSKEY_CHALLENGES, user.id);

        const { credential } = verification.registrationInfo;

        const newCredential = {
            id:           credential.id,
            public_key:   isoBase64URL.fromBuffer(credential.publicKey),
            counter:      credential.counter,
            transports:   attestationResponse.response?.transports || [],
            created_at:   now(),
            last_used_at: null,
        };

        const updatedCredentials = [...(user.passkey_credentials || []), newCredential];

        await this._userService.updateUser(user.id, {
            passkey_credentials: updatedCredentials,
            provider:            AuthProvider.PASSKEY,
        });

        logger.info('[PASSKEY] Credential registered', {
            userId:       user.id,
            credentialId: newCredential.id.substring(0, 12) + '...',
        });

        return { verified: true };
    }

    // -------------------------------------------------------------------------
    // Authentication — challenge
    // -------------------------------------------------------------------------

    async generateAuthenticationChallenge(req, user) {
        const rpId = await this._getRpId(req);
        const allowCredentials = (user.passkey_credentials || []).map(c => ({
            id:         isoBase64URL.fromBuffer(Buffer.from(c.id, 'base64url')),
            type:       'public-key',
            transports: c.transports || [],
        }));

        if (allowCredentials.length === 0) {
            logger.warn('[PASSKEY] Auth challenge: user has no registered passkeys', { userId: user.id });
            return null;
        }

        const options = await generateAuthenticationOptions({
            rpID:               rpId,
            userVerification:   'required',
            allowCredentials,
        });

        await this._cache_aside.createDocument(
            Collections.PASSKEY_CHALLENGES,
            user.id,
            { challenge: options.challenge },
            PASSKEY_CHALLENGE_TTL_SECONDS
        );

        logger.info('[PASSKEY] Authentication challenge generated', { userId: user.id });
        return options;
    }

    // -------------------------------------------------------------------------
    // Credential management
    // -------------------------------------------------------------------------

    async listCredentials(userId) {
        const user = await this._userService.getUser(userId);
        if (!user) return null;
        return (user.passkey_credentials || []).map(c => ({
            id:           c.id,
            transports:   c.transports || [],
            created_at:   c.created_at,
            last_used_at: c.last_used_at,
        }));
    }

    async revokeCredential(userId, credentialId) {
        const user = await this._userService.getUser(userId);
        if (!user) return { found: false, userExists: false };

        const existing = user.passkey_credentials || [];
        const target = existing.find(c => c.id === credentialId);
        if (!target) return { found: false, userExists: true };

        const updated = existing.filter(c => c.id !== credentialId);
        await this._userService.updateUser(userId, { passkey_credentials: updated });

        logger.info('[PASSKEY] Credential revoked', {
            userId,
            credentialId: credentialId.substring(0, 12) + '...',
        });

        return { found: true, userExists: true, remaining: updated.length };
    }

    async revokeAllCredentials(userId) {
        const user = await this._userService.getUser(userId);
        if (!user) return { userExists: false };

        const count = (user.passkey_credentials || []).length;
        await this._userService.updateUser(userId, { passkey_credentials: [] });

        logger.info('[PASSKEY] All credentials revoked', { userId, count });

        return { userExists: true, revoked: count };
    }

    // -------------------------------------------------------------------------
    // Authentication — verify
    // -------------------------------------------------------------------------

    async verifyAuthentication(req, user, assertionResponse) {
        if (!assertionResponse || !assertionResponse.id) {
            logger.warn('[PASSKEY] Auth verify: missing assertion response data', { userId: user.id });
            return { verified: false, error: 'Invalid authentication response.' };
        }

        const rpId     = await this._getRpId(req);
        const origin   = await this._getOrigin(req);
        const doc = await this._cache_aside.getDocument(Collections.PASSKEY_CHALLENGES, user.id);
        const challenge = doc?.challenge;

        if (!challenge) {
            logger.warn('[PASSKEY] Auth verify: challenge expired or missing', { userId: user.id });
            return { verified: false, error: 'Challenge expired. Please try again.' };
        }

        const credentialId = assertionResponse.id;
        const storedCredential = (user.passkey_credentials || []).find(c => c.id === credentialId);

        if (!storedCredential) {
            logger.warn('[PASSKEY] Auth verify: credential not found on user', {
                userId: user.id,
                credentialId: credentialId?.substring(0, 12) + '...',
            });
            return { verified: false, error: 'Passkey not recognized.' };
        }

        let verification;
        try {
            verification = await verifyAuthenticationResponse({
                response:             assertionResponse,
                expectedChallenge:    challenge,
                expectedOrigin:       origin,
                expectedRPID:         rpId,
                requireUserVerification: true,
                credential: {
                    id:         storedCredential.id,
                    publicKey:  Buffer.from(storedCredential.public_key, 'base64url'),
                    counter:    storedCredential.counter,
                    transports: storedCredential.transports || [],
                },
            });
        } catch (error) {
            logger.warn('[PASSKEY] Authentication verification failed', { userId: user.id, error: error.message });
            return { verified: false, error: 'Authentication verification failed.' };
        }

        if (!verification.verified) {
            logger.warn('[PASSKEY] Authentication not verified', { userId: user.id });
            return { verified: false, error: 'Authentication not verified.' };
        }

        await this._cache_aside.deleteDocument(Collections.PASSKEY_CHALLENGES, user.id);

        const { authenticationInfo } = verification;
        const updatedCredentials = (user.passkey_credentials || []).map(c => {
            if (c.id !== credentialId) return c;
            return { ...c, counter: authenticationInfo.newCounter, last_used_at: now() };
        });

        await this._userService.updateUser(user.id, {
            passkey_credentials: updatedCredentials,
        });

        this._userService.updateLastLogin(user.id).catch(err => {
            logger.warn('[PASSKEY] Failed to update last_login (non-critical)', { error: err.message });
        });

        logger.info('[PASSKEY] Authentication verified', {
            userId:       user.id,
            credentialId: credentialId.substring(0, 12) + '...',
        });

        return { verified: true };
    }
}

