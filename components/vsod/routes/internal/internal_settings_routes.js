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
 * VSOD Internal Settings Routes
 *
 * Endpoints for CLI tooling to read and write platform settings via internal auth.
 *
 * NOT exposed via public routes - only accessible from internal services.
 *
 *   GET /api/internal/settings  - return effective non-secret settings
 *   PUT /api/internal/settings  - save settings (env-locked keys silently skipped)
 */

import express from 'express';
import { logger } from '../../utils/logger.js';
import { ErrorResponse, InternalSettingsResponse, SettingsUpdateResponse } from '../../models/response_models.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createInternalSettingsRouter({ services, authorizationMiddleware }) {
    const { settingsService } = services;
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    /**
     * GET /api/internal/settings
     *
     * Returns all effective platform settings, omitting secret values.
     * If userId is provided, returns user-specific settings.
     *
     * SECURITY: INTERNAL ONLY - cluster-only access
     */
    router.get('/', requireInternalOrigin, async (req, res, next) => {
        try {
            const userId = req.query.user_id;
            let settings_data;
            if (userId) {
                settings_data = await settingsService.getUserSettings(userId);
            } else {
                settings_data = await settingsService.getPlatformSettings();
            }
            const schema = settingsService.getSchema();

            const settings = {};
            for (const s of schema) {
                if (!s.secret) {
                    settings[s.key] = {
                        value:     settings_data[s.key] ?? s.default,
                        envLocked: false,
                        label:     s.label,
                        section:   s.section,
                    };
                }
            }

            // Add truncated security tokens (platform only)
            if (!userId) {
                const internalAuthToken = settings_data.internal_auth_token;
                if (internalAuthToken && typeof internalAuthToken === 'string') {
                    settings.internal_auth_token = {
                        value: `${internalAuthToken.substring(0, 8)}...${internalAuthToken.substring(internalAuthToken.length - 4)}`,
                        envLocked: false,
                        label: 'Internal Auth Token',
                        section: 'security'
                    };
                }

                const sessionEncryptionKey = settings_data.session_encryption_key;
                if (sessionEncryptionKey && typeof sessionEncryptionKey === 'string') {
                    settings.session_encryption_key = {
                        value: `${sessionEncryptionKey.substring(0, 8)}...${sessionEncryptionKey.substring(sessionEncryptionKey.length - 4)}`,
                        envLocked: false,
                        label: 'Session Encryption Key',
                        section: 'security'
                    };
                }
            }

            return res.json(new InternalSettingsResponse({ success: true, message: 'Settings fetched successfully', settings }).forWire());
        } catch (err) {
            logger.error('[INTERNAL-SETTINGS] GET failed', { error: err.message });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to load settings' }).forWire());
        }
    });

    /**
     * PUT /api/internal/settings
     *
     * Persists a batch of setting values.
     * If userId is provided, updates UserSettingsDocument.
     *
     * Body: { settings: { key: value, ... }, user_id: "..." }
     *
     * SECURITY: INTERNAL ONLY - cluster-only access
     */
    router.put('/', requireInternalOrigin, async (req, res, next) => {
        try {
            const updates = req.body?.settings;
            const userId = req.body?.user_id;

            if (!updates || typeof updates !== 'object' || Array.isArray(updates)) {
                return res.status(400).json(new ErrorResponse({ error: 'settings object required' }).forWire());
            }

            let result;
            if (userId) {
                result = await settingsService.updateUserSettings(userId, updates);
            } else {
                result = await settingsService.updatePlatformSettings(updates);
            }

            logger.info('[INTERNAL-SETTINGS] Settings updated via internal API', { saved: result.saved, userId });

            return res.json(new SettingsUpdateResponse({ 
                success: true, 
                message: 'Settings updated successfully',
                saved: result.saved
            }).forWire());
        } catch (err) {
            logger.error('[INTERNAL-SETTINGS] PUT failed', { error: err.message });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to save settings' }).forWire());
        }
    });

    return router;
}
