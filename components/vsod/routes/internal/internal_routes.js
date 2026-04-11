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
 * VSOD Internal HTTP Routes
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
import { SourceComponent, SystemHealth } from '../../constants/ai.js';
import { InternalApiPaths } from '../../constants/api_paths.js';
import { ErrorResponse, InternalHealthResponse } from '../../models/response_models.js';
import { createInternalSSERouter } from './internal_sse_routes.js';
import { createInternalOperatorRouter } from './internal_operator_routes.js';
import { createInternalUserRouter } from './internal_user_routes.js';
import { createInternalSessionRouter } from './internal_session_routes.js';
import { createInternalDeviceLinkRouter } from './internal_device_link_routes.js';
import { createInternalSettingsRouter } from './internal_settings_routes.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createInternalRouter({
    services,
    authorizationMiddleware
}) {
    const { sseService, operatorService, userService, webSessionService, passkeyAuthService, deviceLinkService, settingsService, g8eNodeOperatorService } = services;
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    // Mount sub-routers
    router.use('/sse', createInternalSSERouter({ services, authorizationMiddleware }));
    router.use('/operators', createInternalOperatorRouter({ services, authorizationMiddleware }));
    router.use('/users', createInternalUserRouter({ services, authorizationMiddleware }));
    router.use('/session', createInternalSessionRouter({ services, authorizationMiddleware }));
    router.use('/device-links', createInternalDeviceLinkRouter({ services, authorizationMiddleware }));
    router.use('/settings', createInternalSettingsRouter({ services, authorizationMiddleware }));

    /**
     * GET /api/internal/health
     */
    router.get('/health', requireInternalOrigin, (req, res) => {
        res.json(new InternalHealthResponse({
            success: true,
            message: 'Internal API healthy',
            vsodb_status: SystemHealth.HEALTHY,
            vse_status: SystemHealth.HEALTHY,
            vsa_status: SystemHealth.HEALTHY,
            uptime_seconds: Math.floor(process.uptime()),
            memory_usage: process.memoryUsage()
        }).forWire());
    });

    return router;
}
