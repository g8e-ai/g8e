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
 * Settings Service
 *
 * Two distinct schemas:
 *
 *   USER_SETTINGS  — user-configurable settings shown/saved via the Settings UI.
 *                      LLM, search, passkey, app URL, secrets.
 *
 *   PLATFORM_SETTINGS    — deployment-time config resolved once at boot via
 *                      loadResolvedConfig(). Ports, TLS paths, session tuning,
 *                      internal URLs. Never shown in the UI, never writable via
 *                      saveSettings().
 *
 * Both schemas feed into a plain config object returned by loadResolvedConfig().
 * Zero process.env reads after that point.
 *
 * Precedence:
 *   USER_SETTINGS:    DB value > schema default  (user-configurable, persisted)
 *   PLATFORM_SETTINGS: schema default always      (deployment config, never overridden by DB)
 * writeOnce keys are skipped by saveSettings() once set in the DB.
 */

import { logger } from '../../utils/logger.js';
import { Collections } from '../../constants/collections.js';
import { SETTINGS_DOC_ID, USER_SETTINGS_DOC_PREFIX } from '../../constants/service_config.js';
import { apiPaths } from '../../constants/api_paths.js';
import { 
    USER_SETTINGS,
    PLATFORM_SETTINGS,
    SETTINGS_PAGE_SECTIONS,
    validateUserSettings,
    validatePlatformSettings
} from '../../models/settings_model.js';
import { BootstrapService } from './bootstrap_service.js';
import { VSODB_INTERNAL_HTTP_URL } from '../../constants/http_client.js';
import { now } from '../../models/base.js';



// ---------------------------------------------------------------------------
// SettingsService
// ---------------------------------------------------------------------------

class SettingsService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     * @param {InternalHttpClient} [options.internalHttpClient] - InternalHttpClient instance
     * @param {BootstrapService} [options.bootstrapService] - BootstrapService instance
     * @param {string} [options.collectionName] - Collection name override
     */
    constructor({ cacheAsideService, internalHttpClient, bootstrapService, collectionName }) {
        if (!cacheAsideService) {
            throw new Error('SettingsService requires a cacheAsideService instance');
        }
        this._cache_aside = cacheAsideService;
        this.internalHttpClient = internalHttpClient;
        this.bootstrap = bootstrapService || new BootstrapService();
        this.collectionName = collectionName || Collections.SETTINGS;
        this.initialized = false;
        this.resolvedConfig = {};

        // Use Proxy to allow access to settings directly on the service instance
        return new Proxy(this, {
            get(target, prop, receiver) {
                // Return instance property if it exists (methods/internal state)
                if (prop in target) {
                    return Reflect.get(target, prop, receiver);
                }
                // Fallback to resolved config if it has the property
                if (target.resolvedConfig && prop in target.resolvedConfig) {
                    return target.resolvedConfig[prop];
                }
                return undefined;
            }
        });
    }

    /**
     * Bootstraps the SettingsService by resolving secrets and loading initial configuration.
     * This moves logic out of initialization.js into the service where it belongs.
     */
    async initialize() {
        if (this.initialized) return this.resolvedConfig;

        const internalAuthToken = this.bootstrap.loadInternalAuthToken();
        const caCertPath = this.bootstrap.loadCaCertPath();

        logger.info('[SETTINGS-SERVICE] Initializing platform settings', {
            vsodbUrl: VSODB_INTERNAL_HTTP_URL ,
            hasInternalAuthToken: !!internalAuthToken,
            caCertPath,
            vsodbVolumePath: this.bootstrap.volumePath
        });

        // 1. Load PLATFORM_SETTINGS defaults
        for (const setting of PLATFORM_SETTINGS) {
            this.resolvedConfig[setting.key] = setting.default;
        }

        // 2. Load USER_SETTINGS defaults
        for (const setting of USER_SETTINGS) {
            this.resolvedConfig[setting.key] = setting.default;
        }

        // 3. Resolve from DB (platform-wide)
        try {
            const dbSettings = await this.getPlatformSettings();
            
            // Critical failure if platform settings document is missing during bootstrap
            if (Object.keys(dbSettings).length === 0) {
                logger.error('[SETTINGS-SERVICE] Platform settings document missing in VSODB');
                throw new Error('Platform settings document missing in VSODB');
            }

            for (const [key, value] of Object.entries(dbSettings)) {
                if (value !== null && value !== undefined && value !== '') {
                    this.resolvedConfig[key] = value;
                }
            }
        } catch (err) {
            logger.error('[SETTINGS-SERVICE] Failed to load platform settings from DB', {
                error: err.message
            });
            throw err;
        }

        // 4. Inject secrets from bootstrap (if present)
        if (internalAuthToken) this.resolvedConfig.internal_auth_token = internalAuthToken;
        if (caCertPath) this.resolvedConfig.tls_cert_path = caCertPath;

        const sessionKey = this.bootstrap.loadSessionEncryptionKey();
        if (sessionKey) this.resolvedConfig.session_encryption_key = sessionKey;

        this.initialized = true;
        return this.resolvedConfig;
    }


    /**
     * Get platform-wide settings from the DB.
     * @returns {Promise<Object>}
     */
    async getPlatformSettings() {
        const doc = await this._cache_aside.getDocument(this.collectionName, SETTINGS_DOC_ID);
        return (doc && doc.settings) ? doc.settings : {};
    }

    /**
     * Get user-specific settings from the DB.
     * @param {string} userId
     * @returns {Promise<Object>}
     */
    async getUserSettings(userId) {
        if (!userId) return {};
        const userDocId = `${USER_SETTINGS_DOC_PREFIX}${userId}`;
        const doc = await this._cache_aside.getDocument(this.collectionName, userDocId);
        return (doc && doc.settings) ? doc.settings : {};
    }

    /**
     * Save platform-wide settings in the DB.
     * @param {Object} updates
     * @returns {Promise<Object>}
     */
    async savePlatformSettings(updates) {
        const existingDoc = await this._cache_aside.getDocument(this.collectionName, SETTINGS_DOC_ID);
        const existingSettings = (existingDoc && existingDoc.settings) ? existingDoc.settings : {};
        
        // Validate at model level
        const validation = validatePlatformSettings(updates);
        if (validation.invalid.length > 0) {
            logger.warn('[SETTINGS-SERVICE] Invalid platform settings', { 
                invalid: validation.invalid, 
                errors: validation.errors 
            });
        }
        
        const newSettings = { ...existingSettings, ...validation.valid };
        logger.info('[SETTINGS-SERVICE] Saving platform settings', { 
            keys: Object.keys(newSettings),
            isUpdate: !!existingDoc 
        });

        let result;
        if (existingDoc) {
            result = await this._cache_aside.updateDocument(this.collectionName, SETTINGS_DOC_ID, {
                settings: newSettings,
                updated_at: now(),
            });
        } else {
            result = await this._cache_aside.createDocument(this.collectionName, SETTINGS_DOC_ID, {
                settings: newSettings,
                created_at: now(),
                updated_at: now(),
            });
        }

        if (!result || result.success === false) {
            throw new Error(result?.error || 'Failed to save platform settings');
        }

        return { success: true, saved: Object.keys(validation.valid) };
    }

    /**
     * Update user-specific settings in the DB and sync to G8EE.
     * @param {string} userId
     * @param {Object} updates
     * @returns {Promise<Object>}
     */
    async updateUserSettings(userId, updates) {
        if (!userId) throw new Error('userId is required to update user settings');

        // Validate at model level
        const validation = validateUserSettings(updates);
        if (validation.invalid.length > 0) {
            logger.warn('[SETTINGS-SERVICE] Invalid user settings', { 
                userId,
                invalid: validation.invalid, 
                errors: validation.errors 
            });
        }

        const userDocId = `${USER_SETTINGS_DOC_PREFIX}${userId}`;
        const existingDoc = await this._cache_aside.getDocument(this.collectionName, userDocId);
        const existingSettings = (existingDoc && existingDoc.settings) ? existingDoc.settings : {};
        const newSettings = { ...existingSettings, ...validation.valid };

        let result;
        if (existingDoc) {
            result = await this._cache_aside.updateDocument(this.collectionName, userDocId, {
                settings: newSettings,
                updated_at: now(),
            });
        } else {
            result = await this._cache_aside.createDocument(this.collectionName, userDocId, {
                settings: newSettings,
                created_at: now(),
                updated_at: now(),
            });
        }

        if (!result || result.success === false) {
            throw new Error(result?.error || 'Failed to update user settings');
        }

        // Sync to g8ee if internalHttpClient is available
        if (this.internalHttpClient) {
            try {
                await this.internalHttpClient.request('g8ee', apiPaths.g8ee.settingsUser(), {
                    method: 'PATCH',
                    body: validation.valid,
                });
                logger.info('[SETTINGS-SERVICE] Synced user settings to g8ee', { userId });
            } catch (syncError) {
                logger.warn('[SETTINGS-SERVICE] Failed to sync user settings to g8ee (non-critical)', {
                    userId,
                    error: syncError.message
                });
            }
        }

        return { success: true, saved: Object.keys(validation.valid) };
    }

    

    /**
     * Returns settings schema for UI consumption.
     */
    getSchema() {
        return USER_SETTINGS;
    }

    getPageSections() {
        return SETTINGS_PAGE_SECTIONS;
    }

    /**
     * Get the internal auth token from resolved config.
     * @returns {string|null}
     */
    getInternalAuthToken() {
        if (!this.initialized) {
            throw new Error('SettingsService must be initialized before accessing config');
        }
        return this.resolvedConfig?.internal_auth_token || null;
    }

    /**
     * Get the bootstrap service dependency.
     * @returns {BootstrapService}
     */
    getBootstrapService() {
        return this.bootstrap;
    }
}

export { SettingsService };
