// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import { logger } from '../../utils/logger.js';
import { ErrorResponse, SettingsResponse, SettingsUpdateResponse } from '../../models/response_models.js';
import { SettingsPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.settingsService - SettingsService instance
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createSettingsRouter({
    settingsService,
    authMiddleware,
    rateLimiters
}) {
    const { requireAdmin } = authMiddleware;
    const { settingsRateLimiter } = rateLimiters;
    const router = express.Router();

    router.use(requireAdmin);
    router.use(settingsRateLimiter);

    router.get(SettingsPaths.ROOT, async (req, res, next) => {
        try {
            const platformSettings = await settingsService.getPlatformSettings();
            const userSettings = await settingsService.getUserSettings(req.userId);
            const schema = settingsService.getSchema();

            // Minimal mapping for UI: Precedence resolved here or in UI
            const settings = schema.map(s => {
                const value = userSettings[s.key] ?? platformSettings[s.key] ?? s.default;
                return {
                    ...s,
                    value: s.secret ? '' : value,
                    dbValue: s.secret ? '' : value,
                };
            });

            const sections = settingsService.getPageSections();

            return res.json(new SettingsResponse({
                success: true,
                message: 'Settings fetched successfully',
                settings,
                sections
            }).forClient());
        } catch (err) {
            logger.error('[SETTINGS-API] GET failed', { error: err.message, userId: req.userId });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to load settings' }).forClient());
        }
    });

    router.put(SettingsPaths.ROOT, async (req, res, next) => {
        try {
            const updates = req.body?.settings;

            if (!updates || typeof updates !== 'object' || Array.isArray(updates)) {
                return res.status(400).json(new ErrorResponse({ error: 'settings object required' }).forClient());
            }

            // Explicitly separate user vs platform updates if needed, 
            // but for now we assume UI only sends user-level overrides.
            const result = await settingsService.updateUserSettings(req.userId, updates);

            logger.info('[SETTINGS-API] Settings updated', { userId: req.userId, saved: result.saved });

            return res.json(new SettingsUpdateResponse({ 
                success: true, 
                message: 'Settings updated successfully',
                saved: result.saved
            }).forClient());
        } catch (err) {
            logger.error('[SETTINGS-API] PUT failed', { error: err.message, userId: req.userId });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to save settings' }).forClient());
        }
    });

    return router;
}
