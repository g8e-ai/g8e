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

import express from 'express';
import { now, toISOString } from '../../models/base.js';
import { AuditDownloadResponse } from '../../models/sse_models.js';
import { ErrorResponse, AuditEventResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { AuditPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.auditService - Audit service object
 * @param {Object} options.bindingService - Binding service object
 * @param {Object} options.internalHttpClient - Internal HTTP client object
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createAuditRouter({
    services,
    authMiddleware,
    rateLimiters
}) {
    const { auditService, bindingService, internalHttpClient } = services;
    const { requireAuth, requireOperatorBinding } = authMiddleware;
    const { auditRateLimiter } = rateLimiters;
    const router = express.Router();

    router.get(AuditPaths.EVENTS, auditRateLimiter, requireAuth, requireOperatorBinding, async (req, res, next) => {
        const webSessionId = req.webSessionId;

        logger.info('[AUDIT] Events requested', {
            webSessionId: redactWebSessionId(webSessionId),
            fromDate: req.query.from_date,
            toDate: req.query.to_date
        });

        try {
            const auditQueryParams = new URLSearchParams({ user_id: req.userId });
            const investigations = await internalHttpClient.queryInvestigations(auditQueryParams, req.g8eContext);

            const investigationsArray = Array.isArray(investigations) ? investigations : [];

            const filteredEvents = auditService.flattenInvestigationEvents(investigations, {
                fromDate: req.query.from_date || null,
                toDate: req.query.to_date || null,
            });

            logger.info('[AUDIT] Events fetched', {
                webSessionId: redactWebSessionId(webSessionId),
                userId: req.userId,
                totalEvents: filteredEvents.length,
                totalInvestigations: investigationsArray.length
            });

            res.json(new AuditEventResponse({
                events: filteredEvents,
                count: filteredEvents.length,
                total_investigations: investigationsArray.length,
            }).forClient());

        } catch (error) {
            logger.error('[AUDIT-ROUTES] Failed to fetch audit log', {
                userId: req.userId,
                webSessionId: redactWebSessionId(webSessionId),
                error: error.message
            });
            return next(error);
        }
    });

    router.get(AuditPaths.DOWNLOAD, auditRateLimiter, requireAuth, requireOperatorBinding, async (req, res, next) => {
        const webSessionId = req.webSessionId;
        const format = req.query.format ?? 'json';
        
        logger.info('[AUDIT] Download requested', {
            webSessionId: redactWebSessionId(webSessionId),
            format,
            fromDate: req.query.from_date,
            toDate: req.query.to_date
        });

        try {
            const auditQueryParams = new URLSearchParams({ user_id: req.userId });
            const investigations = await internalHttpClient.queryInvestigations(auditQueryParams, req.g8eContext);

            const investigationsArray = Array.isArray(investigations) ? investigations : [];

            const filteredEvents = auditService.flattenInvestigationEvents(investigations, {
                fromDate: req.query.from_date || null,
                toDate: req.query.to_date || null,
            });

            const timestamp = toISOString(now()).replace(/[:.]/g, '-');
            
            if (format === 'csv') {
                const csv = auditService.buildCsvFromEvents(filteredEvents);

                res.setHeader('Content-Type', 'text/csv');
                res.setHeader('Content-Disposition', `attachment; filename="audit-log-${timestamp}.csv"`);
                res.send(csv);
            } else {
                const auditLog = new AuditDownloadResponse({
                    exported_at: now(),
                    user_id: req.userId,
                    total_events: filteredEvents.length,
                    total_investigations: investigationsArray.length,
                    filters: {
                        from_date: req.query.from_date || null,
                        to_date: req.query.to_date || null
                    },
                    events: filteredEvents
                }).forClient();
                
                res.setHeader('Content-Type', 'application/json');
                res.setHeader('Content-Disposition', `attachment; filename="audit-log-${timestamp}.json"`);
                res.json(auditLog);
            }

            logger.info('[AUDIT] Download completed', {
                webSessionId: redactWebSessionId(webSessionId),
                userId: req.userId,
                format,
                totalEvents: filteredEvents.length
            });

        } catch (error) {
            logger.error('[AUDIT] Download error', {
                webSessionId: redactWebSessionId(webSessionId),
                error: error.message
            });

            return next(error);
        }
    });

    return router;
}
