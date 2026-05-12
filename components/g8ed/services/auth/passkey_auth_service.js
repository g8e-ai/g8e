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
 * Challenge storage: `passkey_challenges` document collection. Challenges are
 *   single-use nonces consumed on read during verification (see
 *   `_consumeChallenge`), so rows do not leak on failed or abandoned flows.
 *   The PASSKEY_CHALLENGE_TTL_SECONDS value is applied to the cache-aside
 *   entry; the underlying document is deleted explicitly.
 * Credential storage: user document passkey_credentials array (write-through cache).
 */

import { logger } from '../../utils/logger.js';
import { AuthProvider } from '../../constants/auth.js';
import { Collections } from '../../constants/collections.js';

export class PasskeyAuthService {
    constructor({ userService, cacheAsideService, settingsService, internalHttpClient }) {
        if (!userService)        throw new Error('PasskeyAuthService requires userService');
        if (!cacheAsideService)  throw new Error('PasskeyAuthService requires cacheAsideService');
        if (!settingsService)    throw new Error('PasskeyAuthService requires settingsService');
        if (!internalHttpClient) throw new Error('PasskeyAuthService requires internalHttpClient');
        this._userService     = userService;
        this._cache_aside           = cacheAsideService;
        this._settingsService = settingsService;
        this._internalHttpClient = internalHttpClient;
    }

    // -------------------------------------------------------------------------
    // Registration — challenge
    // -------------------------------------------------------------------------

    async generateRegistrationChallenge(req, user) {
        try {
            // Call substrate-owned challenge generation
            const response = await this._internalHttpClient.passkeyRegisterChallenge({
                user_id: user.id,
                email: user.email,
                user_name: user.name || user.email
            });

            if (!response.success) {
                throw new Error(response.error || 'Failed to generate registration challenge via substrate');
            }

            logger.info('[PASSKEY] Registration challenge generated via substrate', { userId: user.id });
            return response;

        } catch (err) {
            logger.error('[PASSKEY] Failed to generate registration challenge via substrate', {
                userId: user.id,
                error: err.message,
            });
            throw err;
        }
    }

    // -------------------------------------------------------------------------
    // Registration — verify
    // -------------------------------------------------------------------------

    async verifyRegistration(req, user, attestationResponse) {
        try {
            // Call substrate-owned verification
            const response = await this._internalHttpClient.passkeyRegisterVerify({
                user_id: user.id,
                attestation_response: {
                    id: attestationResponse.id,
                    raw_id: attestationResponse.rawId,
                    client_data_json: attestationResponse.response.clientDataJSON,
                    attestation_object: attestationResponse.response.attestationObject,
                    transports: attestationResponse.response.transports || []
                }
            });

            if (!response.success) {
                logger.warn('[PASSKEY] Registration not verified via substrate', { userId: user.id, error: response.error });
                return { verified: false, error: response.error || 'Registration not verified.' };
            }

            // Sync local user document (credentials moved to substrate)
            await this._cache_aside.evictDocument(Collections.USERS, user.id);

            logger.info('[PASSKEY] Credential registered via substrate', {
                userId: user.id,
                credentialId: response.credential?.id.substring(0, 12) + '...',
            });

            return { verified: true };

        } catch (err) {
            logger.error('[PASSKEY] Failed to verify registration via substrate', {
                userId: user.id,
                error: err.message,
            });
            return { verified: false, error: 'Registration verification failed.' };
        }
    }

    // -------------------------------------------------------------------------
    // Authentication — challenge
    // -------------------------------------------------------------------------

    async generateAuthenticationChallenge(req, user) {
        try {
            // Call substrate-owned challenge generation
            const response = await this._internalHttpClient.passkeyAuthChallenge({
                user_id: user.id,
                email: user.email
            });

            if (!response.success) {
                if (response.needs_setup) {
                    logger.warn('[PASSKEY] Auth challenge: user has no registered passkeys (substrate)', { userId: user.id });
                    return null;
                }
                throw new Error(response.error || 'Failed to generate auth challenge via substrate');
            }

            logger.info('[PASSKEY] Authentication challenge generated via substrate', { userId: user.id });
            return response;

        } catch (err) {
            logger.error('[PASSKEY] Failed to generate auth challenge via substrate', {
                userId: user.id,
                error: err.message,
            });
            throw err;
        }
    }

    // -------------------------------------------------------------------------
    // Credential management
    // -------------------------------------------------------------------------

    async listCredentials(userId) {
        try {
            const response = await this._internalHttpClient.passkeyCredentials(userId);
            if (!response.success) return null;
            return response.credentials || [];
        } catch (err) {
            logger.error('[PASSKEY] Failed to list credentials via substrate', { userId, error: err.message });
            return null;
        }
    }

    async revokeCredential(userId, credentialId) {
        try {
            const response = await this._internalHttpClient.passkeyRevokeCredential(credentialId, userId);
            
            if (response.success) {
                // Sync local user cache
                await this._cache_aside.evictDocument(Collections.USERS, userId);
            }

            return { 
                found: response.found, 
                userExists: response.success || response.found, 
                remaining: response.remaining 
            };
        } catch (err) {
            logger.error('[PASSKEY] Failed to revoke credential via substrate', { userId, error: err.message });
            return { found: false, userExists: true };
        }
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
        try {
            // Call substrate-owned verification
            const response = await this._internalHttpClient.passkeyAuthVerify({
                user_id: user.id,
                email: user.email,
                assertion_response: {
                    id: assertionResponse.id,
                    raw_id: assertionResponse.rawId,
                    client_data_json: assertionResponse.response.clientDataJSON,
                    authenticator_data: assertionResponse.response.authenticatorData,
                    signature: assertionResponse.response.signature,
                    user_handle: assertionResponse.response.userHandle
                }
            });

            if (!response.success) {
                logger.warn('[PASSKEY] Authentication not verified via substrate', { userId: user.id, error: response.error });
                return { verified: false, error: response.error || 'Authentication not verified.' };
            }

            // Sync local user cache (last_login updated on substrate)
            await this._cache_aside.evictDocument(Collections.USERS, user.id);

            logger.info('[PASSKEY] Authentication verified via substrate', {
                userId: user.id,
                credentialId: assertionResponse.id.substring(0, 12) + '...',
            });

            return { verified: true, session_id: response.session_id };

        } catch (err) {
            logger.error('[PASSKEY] Failed to verify authentication via substrate', {
                userId: user.id,
                error: err.message,
            });
            return { verified: false, error: 'Authentication verification failed.' };
        }
    }
}

