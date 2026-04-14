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
 * Landing Route - GET /
 *
 * Redirects to /chat when an active session exists.
 * Redirects to /setup when no users exist.
 * Otherwise renders the login page.
 */

import express from 'express';
import { logger } from '../../utils/logger.js';
import { SESSION_COOKIE_NAME } from '../../constants/session.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 */
export function createLandingRouter({ services }) {
    const { webSessionService, setupService } = services;
    const router = express.Router();

    router.get('/', async (req, res, next) => {
        const webSessionId = req.cookies?.[SESSION_COOKIE_NAME];
        if (webSessionId) {
            try {
                const session = await webSessionService.validateSession(webSessionId, {
                    ip: req.ip || req.headers['x-forwarded-for'],
                    userAgent: req.headers['user-agent']
                });
                if (session?.is_active && session.user_id) {
                    return res.redirect('/chat');
                }
            } catch (error) {
                if (process.env.VITEST) {
                    console.log('[TEST-DEBUG] Landing error:', error);
                }
                // Continue to first run check even if session validation fails
                logger.warn('[LANDING] Session validation failed, continuing to first run check', { 
                    error: error.message 
                });
            }
        }

        try {
            const firstRun = await setupService.isFirstRun();
            if (firstRun) {
                return res.redirect('/setup');
            }
        } catch (error) {
            logger.warn('[LANDING] setupService.isFirstRun check failed', { error: error.message });
        }

        res.render('login');
    });

    return router;
}
