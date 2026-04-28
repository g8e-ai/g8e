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
 * User Service - g8ed Authoritative User Management
 *
 * g8ed owns all user/auth data. g8es document store is the source of truth.
 * g8es KV is a write-through read cache.
 *
 * WRITE Flow (cache-aside):
 * 1. Write to g8es document store (authoritative)
 * 2. Update g8es KV cache
 * 3. Return UserDocument to caller
 *
 * READ Flow (cache-aside):
 * 1. Check g8es KV cache (~1-5ms)
 * 2. On miss, read from g8es document store → populate KV cache
 * 3. Return UserDocument
 *
 * All public methods return UserDocument instances.
 * _writeUser accepts only UserDocument — the single serialization point.
 */

import { v4 as uuidv4 } from 'uuid';
import crypto from 'crypto';
import { logger } from '../../utils/logger.js';
import { now } from '../../models/base.js';
import { UserDocument } from '../../models/user_model.js';
import { UserRole, AuthProvider, ApiKeyClientName, ApiKeyStatus } from '../../constants/auth.js';
import { Collections } from '../../constants/collections.js';
import { API_KEY_PREFIX } from '../../constants/operator_defaults.js';
import { ApiKeyError } from '../../constants/auth.js';
import { cacheMetrics } from '../../utils/cache_metrics.js';
import { CacheType } from '../../constants/service_config.js';
import { G8eKeyError } from '../error_service.js';
import { BusinessLogicError } from '../error_service.js';

class UserService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     * @param {Object} [options.organizationService] - OrganizationModel instance
     * @param {Object} [options.apiKeyService] - ApiKeyService instance (Domain)
     */
    constructor({ cacheAsideService, organizationService, apiKeyService }) {
        if (!cacheAsideService) {
            throw new Error('UserService requires cacheAsideService');
        }
        this._cache_aside = cacheAsideService;
        this._organizationService = organizationService;
        this._apiKeyService = apiKeyService;
        this.collectionName = Collections.USERS;
        logger.info('[USER-SERVICE] Initialized with injected dependencies');
    }

    async _generateApiKey() {
        if (this._apiKeyService) {
            return await this._apiKeyService.generateRawKey();
        }
        return `${API_KEY_PREFIX}${crypto.randomBytes(32).toString('hex')}`;
    }

    /**
     * Write-through: g8es document store first (source of truth), then KV cache.
     * Accepts only a UserDocument — single serialization point.
     * @param {UserDocument} userDoc
     */
    async _writeUser(userDoc) {
        const result = await this._cache_aside.createDocument(this.collectionName, userDoc.id, userDoc);
        if (!result.success) {
            throw new Error(result.error || 'Failed to write user to DB');
        }
    }

    /**
     * Get user by ID. Returns UserDocument or null.
     * KV-first, falls back to g8es document store with cache warm on miss.
     * @param {string} userId
     * @returns {Promise<UserDocument|null>}
     */
    async getUser(userId) {
        try {
            const data = await this._cache_aside.getDocument(this.collectionName, userId);
            if (data) {
                logger.info('[USER-SERVICE] Cache HIT', { userId });
                cacheMetrics.recordHit(CacheType.USER);
                return UserDocument.parse(data);
            }
            cacheMetrics.recordMiss(CacheType.USER);
            return null;
        } catch (error) {
            cacheMetrics.recordError(CacheType.USER);
            logger.error('[USER-SERVICE] Failed to get user', { userId, error: error.message });
            return null;
        }
    }

    /**
     * Alias for getUser(userId) for consistency across components.
     */
    async getUserById(userId) {
        return this.getUser(userId);
    }

    /**
     * Create new user. Also creates a matching Organization document.
     * @param {Object} userData - { email, name, roles }
     * @returns {Promise<UserDocument>}
     */
    async createUser(userData) {
        const { email, name, roles, passkey_credentials } = userData;

        if (!email) {
            throw new Error('Email is required');
        }

        const sanitizedEmail = email.trim().toLowerCase();
        logger.info('[USER-SERVICE] Creating new user', { email: sanitizedEmail });

        try {
            // ENFORCE UNIQUENESS: Check if user already exists with this email
            const existing = await this.findUserByEmail(sanitizedEmail);
            if (existing) {
                logger.warn('[USER-SERVICE] createUser: user already exists', { email: sanitizedEmail });
                throw new BusinessLogicError('An account with that email already exists', {
                    code: 'USER_ALREADY_EXISTS',
                    category: 'conflict'
                });
            }

            const userId = uuidv4();
            const ts = now();

            let assignedRoles = roles;
            if (!assignedRoles) {
                const firstUser = !(await this.hasAnyUsers());
                assignedRoles = firstUser ? [UserRole.SUPERADMIN] : [UserRole.USER];
            }

            const userDoc = UserDocument.parse({
                id: userId,
                email: sanitizedEmail,
                name: name || sanitizedEmail.split('@')[0],
                passkey_credentials: passkey_credentials || [],
                g8e_key: null,
                g8e_key_created_at: null,
                organization_id: userId,
                roles: assignedRoles,
                operator_id: null,
                created_at: ts,
                updated_at: ts,
                last_login: ts,
                provider: AuthProvider.PASSKEY,
                sessions: [],
            });

            await this._writeUser(userDoc);

            logger.info('[USER-SERVICE] New user created', { userId, email });

            try {
                if (this._organizationService) {
                    await this._organizationService.create({
                        org_id: userId,
                        owner_id: userId,
                        name: `${userDoc.name}'s Organization`,
                    });
                    logger.info('[USER-SERVICE] Organization created for new user', { userId, email });
                }
            } catch (orgError) {
                logger.warn('[USER-SERVICE] Failed to create organization for new user (non-critical)', {
                    userId,
                    email,
                    error: orgError.message
                });
            }

            return userDoc;
        } catch (error) {
            logger.error('[USER-SERVICE] Failed to create user', { email, error: error.message });
            throw error;
        }
    }

    /**
     * Update existing user with arbitrary field changes.
     * @param {string} userId
     * @param {Object} updates - Plain field updates to merge
     * @returns {Promise<UserDocument>}
     */
    async updateUser(userId, updates) {
        logger.info('[USER-SERVICE] Updating user', { userId, updates: Object.keys(updates) });

        try {
            const existing = await this.getUser(userId);
            if (!existing) {
                throw new Error(`User not found: ${userId}`);
            }

            const updatedData = {
                ...updates,
                updated_at: now(),
            };

            const result = await this._cache_aside.updateDocument(this.collectionName, userId, updatedData);
            if (!result.success) {
                throw new Error(result.error || 'Failed to update user in DB');
            }

            const userDoc = await this.getUser(userId);
            logger.info('[USER-SERVICE] User updated', { userId });
            return userDoc;
        } catch (error) {
            logger.error('[USER-SERVICE] Failed to update user', { userId, error: error.message });
            throw error;
        }
    }

    /**
     * Update last login timestamp.
     * @param {string} userId
     * @returns {Promise<UserDocument>}
     */
    async updateLastLogin(userId) {
        return this.updateUser(userId, {
            last_login: now(),
        });
    }

    /**
     * Update user's Operator linkage.
     * @param {string} userId
     * @param {string} operatorId
     * @param {string} operatorStatus
     * @returns {Promise<boolean>}
     */
    async updateUserOperator(userId, operatorId, operatorStatus) {
        try {
            const user = await this.updateUser(userId, {
                operator_id: operatorId,
                operator_status: operatorStatus,
            });
            return !!user;
        } catch (error) {
            // Logged in updateUser
            return false;
        }
    }

    /**
     * Check whether any users exist in the system.
     * @returns {Promise<boolean>}
     */
    async hasAnyUsers() {
        try {
            const result = await this._cache_aside.queryDocuments(this.collectionName, [], 1);
            return result.length > 0;
        } catch (error) {
            logger.warn('[USER-SERVICE] hasAnyUsers query failed, assuming no users', { error: error.message });
            return false;
        }
    }

    /**
     * Find user by email address.
     * @param {string} email
     * @returns {Promise<UserDocument|null>}
     */
    async findUserByEmail(email) {
        try {
            const result = await this._cache_aside.queryDocuments(this.collectionName, [
                { field: 'email', operator: '==', value: email.toLowerCase() }
            ]);

            if (result.length > 0) {
                return UserDocument.parse(result[0]);
            }

            return null;
        } catch (error) {
            logger.error('[USER-SERVICE] Failed to find user by email', { email, error: error.message });
            return null;
        }
    }

    /**
     * Find user by download API key.
     * @param {string} apiKey
     * @returns {Promise<UserDocument|null>}
     */
    async getUserByApiKey(apiKey) {
        try {
            const result = await this._cache_aside.queryDocuments(this.collectionName, [
                { field: 'g8e_key', operator: '==', value: apiKey }
            ]);

            if (result.length > 0) {
                const userDoc = UserDocument.parse(result[0]);
                logger.info('[USER-SERVICE] Found user by API key', { userId: userDoc.id });
                return userDoc;
            }

            return null;
        } catch (error) {
            logger.error('[USER-SERVICE] Failed to find user by API key', { error: error.message });
            return null;
        }
    }

    /**
     * List users with optional limit. Returns array of UserDocument instances.
     * @param {number} limit
     * @returns {Promise<UserDocument[]>}
     */
    async listUsers(limit) {
        try {
            const result = await this._cache_aside.queryDocuments(this.collectionName, [], limit);
            return result.map(raw => UserDocument.parse(raw));
        } catch (error) {
            logger.error('[USER-SERVICE] Failed to list users', { error: error.message });
            return [];
        }
    }

    /**
     * Get aggregate user statistics.
     * @param {number} queryLimit - Max docs to inspect for stats
     * @returns {Promise<Object>}
     */
    async getUserStats(queryLimit) {
        try {
            const result = await this._cache_aside.queryDocuments(this.collectionName, [], queryLimit);
            return {
                success: true,
                data: { total_users: result.length }
            };
        } catch (error) {
            logger.error('[USER-SERVICE] Failed to get user stats', { error: error.message });
            return {
                success: false,
                message: error.message,
                data: { total_users: 0 }
            };
        }
    }

    /**
     * Delete user by ID. Removes from g8es document store and evicts KV cache.
     * @param {string} userId
     * @returns {Promise<boolean>}
     */
    async deleteUser(userId) {
        const result = await this._cache_aside.deleteDocument(this.collectionName, userId);
        if (!result.success) {
            if (result.notFound) {
                logger.warn('[USER-SERVICE] deleteUser: user not found', { userId });
                return false;
            }
            logger.error('[USER-SERVICE] deleteUser: DB error', { userId, error: result.error });
            throw new Error(result.error || 'Failed to delete user');
        }
        logger.info('[USER-SERVICE] User deleted', { userId });
        return true;
    }

    /**
     * Get the user's current download API key. Returns the key string or null.
     * @param {string} userId
     * @returns {Promise<string|null>}
     */
    async getUserG8eKey(userId) {
        const user = await this.getUser(userId);
        return user?.g8e_key ?? null;
    }

    /**
     * Create a new download API key for the user.
     * @param {string} userId
     * @param {string} organizationId
     * @returns {Promise<{success: boolean, api_key?: string, error?: string}>}
     */
    async createUserG8eKey(userId, organizationId) {
        try {
            if (!this._apiKeyService) {
                throw new Error('apiKeyService is required');
            }

            const user = await this.getUser(userId);
            if (!user) {
                return { success: false, error: ApiKeyError.USER_NOT_FOUND };
            }

            if (user.g8e_key) {
                logger.warn('[USER-SERVICE] User already has a download API key', {
                    userId,
                    api_key_prefix: user.g8e_key.substring(0, 10) + '...',
                });
                return { success: false, error: 'User already has a download API key' };
            }

            const downloadApiKey = await this._generateApiKey();
            const ts = now();

            const storeResult = await this._apiKeyService.issueKey(downloadApiKey, {
                user_id: userId,
                organization_id: organizationId,
                operator_id: null,
                client_name: ApiKeyClientName.USER,
                created_at: ts,
                last_used_at: null,
                permissions: ['operator:download'],
                status: ApiKeyStatus.ACTIVE,
            });

            if (!storeResult.success) {
                logger.error('[USER-SERVICE] Failed to store download API key', { userId, error: storeResult.error });
                return { success: false, error: 'Failed to store API key' };
            }

            const result = await this._cache_aside.updateDocument(this.collectionName, userId, {
                g8e_key: downloadApiKey,
                g8e_key_created_at: ts,
                g8e_key_updated_at: ts,
                updated_at: ts,
            });

            if (!result.success) {
                throw new Error(result.error || 'Failed to update user with API key');
            }

            logger.info('[USER-SERVICE] Created user download API key', {
                userId,
                api_key_prefix: downloadApiKey.substring(0, 10) + '...',
            });

            return { success: true, api_key: downloadApiKey };
        } catch (error) {
            logger.error('[USER-SERVICE] Failed to create user download API key', { userId, error: error.message });
            return { success: false, error: error.message };
        }
    }

    /**
     * Refresh (rotate) the user's download API key. Revokes the old key and issues a new one.
     * @param {string} userId
     * @param {string} organizationId
     * @returns {Promise<{success: boolean, api_key?: string, error?: string}>}
     */
    async refreshUserG8eKey(userId, organizationId) {
        try {
            if (!this._apiKeyService) {
                throw new G8eKeyError('apiKeyService is required');
            }

            const user = await this.getUser(userId);
            if (!user) {
                throw new G8eKeyError(ApiKeyError.USER_NOT_FOUND);
            }

            const ts = now();

            // Revoke old key if it exists
            if (user.g8e_key) {
                logger.info('[USER-SERVICE] Revoking old download API key', {
                    userId,
                    api_key_prefix: user.g8e_key.substring(0, 10) + '...',
                });
                const revokeResult = await this._apiKeyService.revokeKey(user.g8e_key);
                if (!revokeResult.success) {
                    logger.warn('[USER-SERVICE] Failed to revoke old API key (non-critical)', {
                        userId,
                        error: revokeResult.error
                    });
                }
            }

            // Generate and issue new key
            const downloadApiKey = await this._generateApiKey();

            const storeResult = await this._apiKeyService.issueKey(downloadApiKey, {
                user_id: userId,
                organization_id: organizationId,
                operator_id: null,
                client_name: ApiKeyClientName.USER,
                created_at: ts,
                last_used_at: null,
                permissions: ['operator:download'],
                status: ApiKeyStatus.ACTIVE,
            });

            if (!storeResult.success) {
                logger.error('[USER-SERVICE] Failed to store new download API key', { userId, error: storeResult.error });
                throw new G8eKeyError('Failed to store API key');
            }

            // Update user document
            const result = await this._cache_aside.updateDocument(this.collectionName, userId, {
                g8e_key: downloadApiKey,
                g8e_key_created_at: user.g8e_key_created_at || ts,
                g8e_key_updated_at: ts,
                updated_at: ts,
            });

            if (!result.success) {
                throw new G8eKeyError(result.error || 'Failed to update user with API key');
            }

            logger.info('[USER-SERVICE] Refreshed user download API key', {
                userId,
                api_key_prefix: downloadApiKey.substring(0, 10) + '...',
            });

            return { success: true, api_key: downloadApiKey };
        } catch (error) {
            if (error instanceof G8eKeyError) {
                throw error;
            }
            logger.error('[USER-SERVICE] Failed to refresh user download API key', { userId, error: error.message });
            throw new G8eKeyError(error.message);
        }
    }
}

export { UserService };
