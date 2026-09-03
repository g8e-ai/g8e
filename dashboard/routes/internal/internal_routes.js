// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8ed Internal HTTP Routes
 * 
 * Cluster-internal HTTP endpoints for direct communication from other VSO components.
 * NOT exposed via public routes - only accessible from internal services.
 * 
 * This module aggregates all internal route handlers:
 * - SSE routes: Event delivery from VSE
 * - Operator routes: Operator management
 * - User routes: User queries (read-only)
 * - Settings routes: Platform settings (read-only, non-secret)
 */

import express from 'express';
import { SystemHealth } from '../../constants/ai.js';
import { InternalHealthResponse } from '../../models/response_models.js';
import { createInternalSSERouter } from './internal_sse_routes.js';
import { createInternalOperatorRouter } from './internal_operator_routes.js';
import { createInternalUserRouter } from './internal_user_routes.js';
import { createInternalSessionRouter } from './internal_session_routes.js';
import { createInternalDeviceLinkRouter } from './internal_device_link_routes.js';
import { createInternalSettingsRouter } from './internal_settings_routes.js';

/**
 * @param {Object} options
 * @param {Object} options.sseService - SSEService instance
 * @param {Object} options.operatorService - OperatorDataService instance
 * @param {Object} options.userService - UserService instance
 * @param {Object} options.webSessionService - WebSessionService instance
 * @param {Object} options.passkeyAuthService - PasskeyAuthService instance
 * @param {Object} options.deviceLinkService - DeviceLinkService instance
 * @param {Object} options.settingsService - SettingsService instance
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createInternalRouter({
    sseService,
    operatorService,
    userService,
    webSessionService,
    passkeyAuthService,
    deviceLinkService,
    settingsService,
    authorizationMiddleware
}) {
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    // Mount sub-routers
    router.use('/sse', createInternalSSERouter({ sseService, authorizationMiddleware, operatorService }));
    router.use('/operators', createInternalOperatorRouter({ operatorService, authorizationMiddleware }));
    router.use('/users', createInternalUserRouter({ userService, webSessionService, passkeyAuthService, authorizationMiddleware }));
    router.use('/session', createInternalSessionRouter({ webSessionService, userService, authorizationMiddleware }));
    router.use('/device-links', createInternalDeviceLinkRouter({ deviceLinkService, authorizationMiddleware }));
    router.use('/settings', createInternalSettingsRouter({ settingsService, authorizationMiddleware }));

    /**
     * GET /api/internal/health
     */
    router.get('/health', requireInternalOrigin, (req, res) => {
        res.json(new InternalHealthResponse({
            success: true,
            message: 'Internal API healthy',
            g8eg_status: SystemHealth.HEALTHY,
            vse_status: SystemHealth.HEALTHY,
            vsa_status: SystemHealth.HEALTHY,
            uptime_seconds: Math.floor(process.uptime()),
            memory_usage: process.memoryUsage()
        }).forWire());
    });

    return router;
}
