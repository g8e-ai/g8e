// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import { now, toISOString } from '../../models/base.js';
import { AuditDownloadResponse } from '../../models/sse_models.js';
import { ErrorResponse, AuditEventResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { AuditPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.auditService - AuditService instance
 * @param {Object} options.bindingService - BoundSessionsService instance
 * @param {Object} options.internalHttpClient - InternalHttpClient instance
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createAuditRouter({
    auditService,
    bindingService,
    internalHttpClient,
    authMiddleware,
    rateLimiters
}) {
    const { requireAuth } = authMiddleware;
    const { auditRateLimiter } = rateLimiters;
    const router = express.Router();

    router.get(AuditPaths.EVENTS, auditRateLimiter, requireAuth, async (req, res, next) => {
        const webSessionId = req.webSessionId;

        logger.info('[AUDIT] Events requested', {
            webSessionId: redactWebSessionId(webSessionId),
            fromDate: req.query.from_date,
            toDate: req.query.to_date
        });

        try {
            const session = req.session;

            const vsoContext = {
                web_session_id: session.id,
                user_id: session.user_id,
                organization_id: session.organization_id ?? session.user_data?.organization_id ?? null,
                bound_operators: await bindingService.resolveBoundOperators(session.id),
                execution_id: `req_audit_events_${now()}`
            };

            const auditQueryParams = new URLSearchParams({ user_id: session.user_id });
            const investigations = await internalHttpClient.queryInvestigations(auditQueryParams, vsoContext);

            const investigationsArray = Array.isArray(investigations) ? investigations : [];

            const filteredEvents = auditService.flattenInvestigationEvents(investigations, {
                fromDate: req.query.from_date || null,
                toDate: req.query.to_date || null,
            });

            logger.info('[AUDIT] Events fetched', {
                webSessionId: redactWebSessionId(webSessionId),
                userId: session.user_id,
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

    router.get(AuditPaths.DOWNLOAD, auditRateLimiter, requireAuth, async (req, res, next) => {
        const webSessionId = req.webSessionId;
        const format = req.query.format ?? 'json';
        
        logger.info('[AUDIT] Download requested', {
            webSessionId: redactWebSessionId(webSessionId),
            format,
            fromDate: req.query.from_date,
            toDate: req.query.to_date
        });

        try {
            const session = req.session;

            const vsoContext = {
                web_session_id: session.id,
                user_id: session.user_id,
                organization_id: session.organization_id ?? session.user_data?.organization_id ?? null,
                bound_operators: await bindingService.resolveBoundOperators(session.id),
                execution_id: `req_audit_download_${now()}`
            };

            const auditQueryParams = new URLSearchParams({ user_id: session.user_id });
            const investigations = await internalHttpClient.queryInvestigations(auditQueryParams, vsoContext);

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
                    user_id: session.user_id,
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
                userId: session.user_id,
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
