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
 * System Routes
 *
 * Exposes read-only host system information to authenticated users.
 *
 *   GET  /api/system/network-interfaces  - return IPv4 addresses of host network interfaces
 */

import os from 'os';
import express from 'express';
import { logger } from '../../utils/logger.js';
import { ErrorResponse, SystemNetworkInterfacesResponse } from '../../models/response_models.js';
import { SystemPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createSystemRouter({
    services,
    authMiddleware,
    rateLimiters
}) {
    const { settingsService } = services;
    const { requireAuth } = authMiddleware;
    const { apiRateLimiter } = rateLimiters;
    const router = express.Router();

    /**
     * GET /api/system/network-interfaces
     *
     * Returns endpoint candidates for the --endpoint flag of the deploy UI.
     *
     * Priority:
     *   1. HOST_IPS — comma-separated list of IPv4 addresses collected from the
     *      Docker host by build.sh _preflight and injected via docker-compose.
     *      This is the authoritative source since g8ed runs in a bridged container
     *      and cannot enumerate host interfaces directly.
     *   2. APP_URL hostname — always set; used as a named fallback entry so the
     *      dropdown always has at least one meaningful option.
     *
     * Response:
     *   { success: true, interfaces: [{ name: "ens32", address: "192.168.1.5" }, ...] }
     */
    router.get(SystemPaths.NETWORK_INTERFACES, requireAuth, apiRateLimiter, (req, res, next) => {
        try {
            const interfaces = [];
            const seen = new Set();

            const addEntry = (name, address) => {
                if (address && !seen.has(address)) {
                    seen.add(address);
                    interfaces.push({ name, address });
                }
            };

            const hostIps = settingsService.host_ips?.trim();
            if (hostIps) {
                for (const ip of hostIps.split(',')) {
                    addEntry('host', ip.trim());
                }
            }

            const appUrl = settingsService.app_url?.trim();
            if (appUrl) {
                try {
                    const hostname = new URL(appUrl).hostname;
                    addEntry('APP_URL', hostname);
                } catch (_) {
                    // malformed APP_URL — skip
                }
            }

            return res.json(new SystemNetworkInterfacesResponse({ success: true, interfaces }).forClient());
        } catch (err) {
            logger.error('[SYSTEM-API] Failed to enumerate network interfaces', { error: err.message });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to enumerate network interfaces' }).forClient());
        }
    });

    return router;
}
