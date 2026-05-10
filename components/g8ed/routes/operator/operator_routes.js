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
 * g8ed Operator Routes - Node.js Implementation
 *
 * Handles Operator health, binary download, and checksum endpoints.
 * Token authentication is delegated to DownloadAuthService.
 */

import express from 'express';
import { createHash } from 'crypto';
import { logger } from '../../utils/logger.js';
import { SourceComponent, SystemHealth } from '../../constants/ai.js';
import { CacheTTL, PLATFORMS, BINARY_NAME, OperatorRouteError, ContentType } from '../../constants/service_config.js';
import { OperatorPaths } from '../../constants/api_paths.js';
import { ErrorResponse, OperatorBinaryAvailabilityResponse } from '../../models/response_models.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createOperatorRouter({ services, authorizationMiddleware }) {
    const { operatorDownloadService, downloadAuthService } = services;
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    /**
     * Get Operator health status
     * Includes binary availability for each platform
     * SECURITY: Internal only - for load balancer health probes
     */
    router.get(OperatorPaths.HEALTH, requireInternalOrigin, async (req, res, next) => {
        try {
            const binaryStatus = await operatorDownloadService.getPlatformAvailability();
            const availablePlatforms = Object.keys(binaryStatus).filter(p => binaryStatus[p].available);
            const allPlatformsAvailable = Object.values(binaryStatus).every(p => p.available);

            const isHealthy = allPlatformsAvailable;
            const status = isHealthy ? SystemHealth.HEALTHY : SystemHealth.DEGRADED;

            res.json(new OperatorBinaryAvailabilityResponse({
                success: true,
                status,
                component: SourceComponent.G8ED,
                version: '1.0.0', // This should ideally come from a central versioning utility
                platforms: Object.entries(binaryStatus).map(([platform, data]) => ({
                    platform,
                    ...data
                }))
            }).forClient());
        } catch (error) {
            logger.error('Operator health check failed:', error);
            res.status(500).json(new ErrorResponse({ error: 'Operator health check failed' }).forClient());
        }
    });

    /**
     * Download Operator binary for specific platform
     */
    router.get(OperatorPaths.DOWNLOAD, async (req, res, next) => {
        const startTime = Date.now();
        const { os, arch } = req.params;
        const platform = `${os}/${arch}`;

        logger.info('[OPERATOR-DOWNLOAD] Download request received', {
            os,
            arch,
            platform,
            ip: req.ip,
            user_agent: req.headers['user-agent'],
        });

        try {
            const validOs = [...new Set(PLATFORMS.map(p => p.os))];
            const validArch = PLATFORMS.reduce((acc, p) => {
                if (!acc[p.os]) acc[p.os] = [];
                acc[p.os].push(p.arch);
                return acc;
            }, {});

            if (!validOs.includes(os)) {
                logger.warn('[OPERATOR-DOWNLOAD] Invalid OS requested', { os });
                return res.status(400).json(new ErrorResponse({ error: OperatorRouteError.UNSUPPORTED_OS }).forClient());
            }

            if (!validArch[os] || !validArch[os].includes(arch)) {
                logger.warn('[OPERATOR-DOWNLOAD] Invalid architecture requested', { os, arch });
                return res.status(400).json(new ErrorResponse({
                    error: `Invalid architecture for ${os}. Must be one of: ${validArch[os]?.join(', ') || 'N/A'}`
                }).forClient());
            }

            const authResult = await downloadAuthService.validate(req, { allowDlt: true });

            if (!authResult.success) {
                return res.status(authResult.status).json(new ErrorResponse({ error: authResult.error }).forClient());
            }

            const { userId, organizationId, operatorId, keyType } = authResult;

            logger.info(`[OPERATOR-DOWNLOAD] Authenticated download for ${platform}`, {
                user_id:         userId,
                organization_id: organizationId,
                operator_id:     operatorId || 'N/A',
                key_type:        keyType,
            });

            logger.info('[OPERATOR-DOWNLOAD] Fetching binary from g8es...', { os, arch, platform });
            const binary = await operatorDownloadService.getBinary(os, arch);

            res.setHeader('Content-Type', ContentType.OCTET_STREAM);
            res.setHeader('Content-Disposition', `attachment; filename="${BINARY_NAME}"`);
            res.setHeader('Content-Length', binary.length);
            res.setHeader('Cache-Control', `private, max-age=${CacheTTL.OPERATOR}`);

            res.send(binary);

            const duration = Date.now() - startTime;
            logger.info(`[OPERATOR-DOWNLOAD] Served ${platform} binary`, {
                platform,
                size_bytes: binary.length,
                size_mb:    (binary.length / 1024 / 1024).toFixed(2),
                duration_ms: duration,
                user_id:    userId,
            });

        } catch (error) {
            const duration = Date.now() - startTime;
            logger.error('[OPERATOR-DOWNLOAD] Download failed', {
                os,
                arch,
                platform: `${os}/${arch}`,
                error:    error.message,
                stack:    error.stack,
                duration_ms: duration,
            });

            if (!res.headersSent) {
                const isBinaryNotFound = error.message.startsWith('Operator binary not available');
                const statusCode = isBinaryNotFound ? 503 : 500;
                res.status(statusCode).json(new ErrorResponse({
                    error: isBinaryNotFound ? OperatorRouteError.BINARY_NOT_AVAILABLE : OperatorRouteError.DOWNLOAD_FAILED,
                    message: error.message,
                }).forClient());
            }
        }
    });

    /**
     * Download SHA256 checksum for Operator binary
     */
    router.get(OperatorPaths.DOWNLOAD_SHA256, async (req, res, next) => {
        const { os, arch } = req.params;
        const platform = `${os}/${arch}`;

        logger.info('[OPERATOR-CHECKSUM] Checksum request received', { os, arch, platform });

        try {
            const validOs = [...new Set(PLATFORMS.map(p => p.os))];
            const validArch = PLATFORMS.reduce((acc, p) => {
                if (!acc[p.os]) acc[p.os] = [];
                acc[p.os].push(p.arch);
                return acc;
            }, {});

            if (!validOs.includes(os)) {
                return res.status(400).json(new ErrorResponse({ error: OperatorRouteError.UNSUPPORTED_OS }).forClient());
            }

            if (!validArch[os] || !validArch[os].includes(arch)) {
                return res.status(400).json(new ErrorResponse({
                    error: `Invalid architecture for ${os}. Must be one of: ${validArch[os]?.join(', ') || 'N/A'}`,
                }).forClient());
            }

            const authResult = await downloadAuthService.validate(req, { allowDlt: true });

            if (!authResult.success) {
                return res.status(authResult.status).json(new ErrorResponse({ error: authResult.error }).forClient());
            }

            const binary = await operatorDownloadService.getBinary(os, arch);
            const hash = createHash('sha256').update(binary).digest('hex');
            const checksumContent = `${hash}  ${BINARY_NAME}\n`;

            res.setHeader('Content-Type', ContentType.TEXT_PLAIN);
            res.setHeader('Content-Disposition', `attachment; filename="${BINARY_NAME}.sha256"`);
            res.setHeader('Content-Length', checksumContent.length);
            res.setHeader('Cache-Control', `private, max-age=${CacheTTL.OPERATOR}`);

            res.send(checksumContent);

            logger.info(`[OPERATOR-CHECKSUM] Served ${platform} checksum`, {
                platform,
                hash: hash.substring(0, 16) + '...',
            });

        } catch (error) {
            logger.error('[OPERATOR-CHECKSUM] Checksum generation failed', {
                os,
                arch,
                error: error.message,
            });

            if (!res.headersSent) {
                const isBinaryNotFound = error.message.startsWith('Operator binary not available');
                res.status(isBinaryNotFound ? 503 : 500).json(new ErrorResponse({
                    error: isBinaryNotFound ? OperatorRouteError.BINARY_NOT_AVAILABLE : OperatorRouteError.CHECKSUM_FAILED,
                }).forClient());
            }
        }
    });

    return router;
}
