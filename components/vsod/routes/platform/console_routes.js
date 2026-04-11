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
import { logger, getLogRingBuffer, addLogListener, removeLogListener } from '../../utils/logger.js';
import { Collections } from '../../constants/collections.js';
import { ConsolePaths } from '../../constants/api_paths.js';
import { EventType } from '../../constants/events.js';
import { now } from '../../models/base.js';
import { LogStreamEvent, LogStreamConnectedEvent } from '../../models/sse_models.js';
import { PlatformOverviewResponse, UserStatsResponse, OperatorStatsResponse, SessionStatsResponse, AIUsageStatsResponse, LoginAuditStatsResponse, RealTimeMetricsResponse, ComponentHealthResponse, DBCollectionsResponse, DBQueryResponse, ErrorResponse, KVKeyResponse, KVScanResponse, SimpleSuccessResponse } from '../../models/response_models.js';
import { writeSSEFrame } from '../../utils/sse.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createConsoleRouter({
    services,
    authMiddleware,
    rateLimiters
}) {
    const { consoleMetricsService } = services;
    const { requireSuperAdmin } = authMiddleware;
    const { consoleRateLimiter } = rateLimiters;
    const router = express.Router();

    router.use(requireSuperAdmin);
    router.use(consoleRateLimiter);

    router.get(ConsolePaths.OVERVIEW, async (req, res, next) => {
        try {
            const data = await consoleMetricsService.getPlatformOverview();
            res.json(new PlatformOverviewResponse({ 
                success: true, 
                message: 'Overview fetched successfully',
                timestamp: data.timestamp,
                users: data.users,
                operators: data.operators,
                sessions: data.sessions,
                cache: data.cache,
                system: data.system
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to get platform overview', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to fetch platform overview' }).forClient());
        }
    });

    router.get(ConsolePaths.METRICS_USERS, async (req, res, next) => {
        try {
            const data = await consoleMetricsService.getUserStats();
            res.json(new UserStatsResponse({ 
                success: true, 
                message: 'User stats fetched successfully',
                total: data.total,
                activity: data.activity,
                newUsersLastWeek: data.newUsersLastWeek
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to get user stats', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to fetch user stats' }).forClient());
        }
    });

    router.get(ConsolePaths.METRICS_OPERATORS, async (req, res, next) => {
        try {
            const data = await consoleMetricsService.getOperatorStats();
            res.json(new OperatorStatsResponse({ 
                success: true, 
                message: 'Operator stats fetched successfully',
                total: data.total,
                statusDistribution: data.statusDistribution,
                typeDistribution: data.typeDistribution,
                health: data.health
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to get operator stats', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to fetch operator stats' }).forClient());
        }
    });

    router.get(ConsolePaths.METRICS_SESSIONS, async (req, res, next) => {
        try {
            const data = await consoleMetricsService.getSessionStats();
            res.json(new SessionStatsResponse({ 
                success: true, 
                message: 'Session stats fetched successfully',
                web: data.web,
                operator: data.operator,
                total: data.total,
                boundOperators: data.boundOperators
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to get session stats', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to fetch session stats' }).forClient());
        }
    });

    router.get(ConsolePaths.METRICS_AI, async (req, res, next) => {
        try {
            const data = await consoleMetricsService.getAIUsageStats();
            res.json(new AIUsageStatsResponse({ 
                success: true, 
                message: 'AI stats fetched successfully',
                totalInvestigations: data.totalInvestigations,
                activeInvestigations: data.activeInvestigations,
                completedInvestigations: data.completedInvestigations
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to get AI usage stats', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to fetch AI usage stats' }).forClient());
        }
    });

    router.get(ConsolePaths.METRICS_LOGIN_AUDIT, async (req, res, next) => {
        try {
            const data = await consoleMetricsService.getLoginAuditStats();
            res.json(new LoginAuditStatsResponse({ 
                success: true, 
                message: 'Login audit stats fetched successfully',
                total: data.total,
                successful: data.successful,
                failed: data.failed,
                locked: data.locked,
                anomalies: data.anomalies,
                byHour: data.byHour
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to get login audit stats', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to fetch login audit stats' }).forClient());
        }
    });

    router.get(ConsolePaths.METRICS_REALTIME, async (req, res, next) => {
        try {
            const data = await consoleMetricsService.getRealTimeMetrics();
            res.json(new RealTimeMetricsResponse({ 
                success: true, 
                message: 'Real-time metrics fetched successfully',
                timestamp: data.timestamp,
                vsodb: data.vsodb,
                cache: data.cache
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to get real-time metrics', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to fetch real-time metrics' }).forClient());
        }
    });

    router.post(ConsolePaths.CACHE_CLEAR, async (req, res, next) => {
        try {
            consoleMetricsService.clearCache();
            logger.info('[CONSOLE] Metrics cache cleared by superadmin', { userId: req.userId });
            res.json(new SimpleSuccessResponse({ success: true, message: 'Metrics cache cleared' }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to clear metrics cache', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to clear cache' }).forClient());
        }
    });

    router.get(ConsolePaths.COMPONENTS_HEALTH, async (req, res, next) => {
        try {
            const data = await consoleMetricsService.getComponentHealth();
            logger.info('[CONSOLE] Component health check', { overall: data.overall, userId: req.userId });
            res.json(new ComponentHealthResponse({ 
                success: true, 
                message: 'Component health fetched successfully',
                overall: data.overall,
                timestamp: data.timestamp,
                components: data.components
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] Failed to get component health', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'Failed to fetch component health' }).forClient());
        }
    });

    router.get(ConsolePaths.KV_SCAN, async (req, res, next) => {
        try {
            const pattern = req.query.pattern || '*';
            const cursor = req.query.cursor || '0';
            const count = Math.min(parseInt(req.query.count, 10) || 50, 200);

            const data = await consoleMetricsService.scanKV(pattern, cursor, count);

            logger.info('[CONSOLE] KV scan', { pattern, count: data.count, userId: req.userId });
            res.json(new KVScanResponse({ 
                success: true, 
                message: 'KV scan completed successfully', 
                pattern, 
                cursor: data.cursor || null,
                keys: data.keys || [],
                count: data.count || 0,
                has_more: data.has_more || false
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] KV scan failed', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'KV scan failed' }).forClient());
        }
    });

    router.get(ConsolePaths.KV_KEY, async (req, res, next) => {
        try {
            const { key } = req.query;
            if (!key) {
                return res.status(400).json(new ErrorResponse({ error: 'key query parameter is required' }).forClient());
            }

            const data = await consoleMetricsService.getKVKey(key);

            logger.info('[CONSOLE] KV key get', { key, found: data.exists, userId: req.userId });
            res.json(new KVKeyResponse({ 
                success: true, 
                message: 'KV key fetched successfully', 
                key,
                exists: data.exists,
                value: data.value,
                content_type: data.content_type,
                created_at: data.created_at,
                updated_at: data.updated_at
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] KV key get failed', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'KV key lookup failed' }).forClient());
        }
    });

    router.get(ConsolePaths.DB_COLLECTIONS, (req, res) => {
        res.json(new DBCollectionsResponse({ 
            success: true, 
            message: 'Collections fetched successfully', 
            collections: Object.values(Collections) 
        }).forClient());
    });

    router.get(ConsolePaths.DB_QUERY, async (req, res, next) => {
        try {
            const { collection, limit: limitParam } = req.query;

            const allowed = Object.values(Collections);
            if (!collection || !allowed.includes(collection)) {
                return res.status(400).json(new ErrorResponse({ error: 'Invalid or missing collection' }).forClient());
            }

            const limit = Math.min(parseInt(limitParam, 10) || 50, 200);
            const data = await consoleMetricsService.queryCollection(collection, limit);

            logger.info('[CONSOLE] DB query', { collection, limit, found: data.count, userId: req.userId });
            res.json(new DBQueryResponse({ 
                success: true, 
                message: 'DB query completed successfully', 
                collection,
                documents: data.documents || [],
                count: data.count || 0,
                limit
            }).forClient());
        } catch (error) {
            logger.error('[CONSOLE] DB query failed', { error: error.message, userId: req.userId });
            res.status(500).json(new ErrorResponse({ error: 'DB query failed' }).forClient());
        }
    });

    router.get(ConsolePaths.LOGS_STREAM, async (req, res, next) => {
        const level = req.query.level || 'info';
        const limit = Math.min(parseInt(req.query.limit, 10) || 100, 500);

        res.setHeader('Content-Type', 'text/event-stream');
        res.setHeader('Cache-Control', 'no-cache, no-transform');
        res.setHeader('Connection', 'keep-alive');
        res.setHeader('X-Accel-Buffering', 'no');
        res.status(200);
        res.flushHeaders();
        if (res.socket) res.socket.setNoDelay(true);

        const ringBuffer = getLogRingBuffer();

        const LEVELS = ['error', 'warn', 'info', 'debug'];
        const levelIdx = LEVELS.indexOf(level);

        const matchesLevel = (entry) => {
            const entryIdx = LEVELS.indexOf(entry.level);
            return entryIdx !== -1 && entryIdx <= levelIdx;
        };

        if (ringBuffer) {
            const recent = ringBuffer.slice(-limit).filter(matchesLevel);
            for (const entry of recent) {
                try { writeSSEFrame(res, new LogStreamEvent({ type: EventType.PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED, entry })); } catch (_) {}
            }
        }

        try {
            writeSSEFrame(res, new LogStreamConnectedEvent({
                type: EventType.PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED,
                timestamp: now(),
                buffered: ringBuffer ? Math.min(ringBuffer.length, limit) : 0
            }));
        } catch (_) {}

        const listener = (entry) => {
            if (matchesLevel(entry)) {
                try { writeSSEFrame(res, new LogStreamEvent({ type: EventType.PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED, entry })); } catch (_) {}
            }
        };

        addLogListener(listener);

        logger.info('[CONSOLE] Log stream connected', { level, limit, userId: req.userId });

        req.on('close', () => {
            removeLogListener(listener);
            logger.info('[CONSOLE] Log stream disconnected', { userId: req.userId });
        });
    });

    return router;
}
