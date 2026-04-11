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
 * VSOD Express App Factory
 * 
 * Creates and configures the Express app with all middleware and routes.
 * This allows both production server and integration tests to use the exact same middleware stack.
 */

import express from 'express';
import compression from 'compression';
import cors from 'cors';
import helmet from 'helmet';
import cookieParser from 'cookie-parser';
import path from 'path';
import fs from 'fs';

import { createChatRouter } from './routes/platform/chat_routes.js';
import { createDocsRouter } from './routes/platform/docs_routes.js';
import { createHealthRouter } from './routes/platform/health_routes.js';
import { createAuthRouter } from './routes/auth/auth_routes.js';
import { createPasskeyRouter } from './routes/auth/passkey_routes.js';
import { createLandingRouter } from './routes/auth/landing_routes.js';
import { createSetupRouter } from './routes/auth/setup_routes.js';
import { createSSERouter } from './routes/platform/sse_routes.js';
import { createMetricsRouter } from './routes/platform/metrics_routes.js';
import { createInternalRouter } from './routes/internal/internal_routes.js';
import { createUserRouter } from './routes/platform/user_routes.js';
import { createDeviceLinkRouter } from './routes/auth/device_link_routes.js';
import { createAuditRouter } from './routes/platform/audit_routes.js';
import { createConsoleRouter } from './routes/platform/console_routes.js';
import { createSettingsRouter } from './routes/platform/settings_routes.js';
import { createSystemRouter } from './routes/platform/system_routes.js';
import { createOperatorRouter } from './routes/operator/operator_routes.js';
import { createOperatorApprovalRouter } from './routes/operator/operator_approval_routes.js';
import { createOperatorAuthRouter } from './routes/operator/operator_auth_routes.js';
import { createOperatorStatusRouter } from './routes/operator/operator_status_routes.js';
import { createBindOperatorsRouter } from './routes/operator/operator_bind_routes.js';
import { createOperatorApiKeyRouter } from './routes/operator/operator_api_key_routes.js';
import { createMCPRouter } from './routes/platform/mcp_routes.js';

import { cspNonce } from './middleware/csp_nonce.js';
import { globalContextMiddleware } from './middleware/context.js';

import { 
    HTTP_CONTENT_TYPE_HEADER,
    HTTP_REQUESTED_WITH_HEADER,
    WEB_SESSION_ID_HEADER,
    HTTP_API_KEY_HEADER
} from './constants/headers.js';
import { BasePaths } from './constants/api_paths.js';
import { CORS_INTERNAL_ORIGINS } from './constants/http_client.js';

/**
 * Create and configure an Express app with VSOD middleware and routes
 * 
 * @param {Object} options - Configuration options
 * @param {Object} options.services - All required services
 * @param {Object} options.rateLimiters - Rate limiter instances
 * @param {Object} options.authMiddleware - Authentication middleware
 * @param {Object} options.authorizationMiddleware - Authorization middleware
 * @param {Object} options.apiKeyMiddleware - API key middleware
 * @param {Object} options.requestTimestampMiddleware - Request timestamp middleware
 * @param {Object} options.errorHandlerMiddleware - Error handler middleware
 * @param {Object} options.settings - Application settings
 * @param {Object} options.versionInfo - Version information
 * @param {boolean} options.isTest - Whether this is a test environment (defaults to false)
 * @param {string} options.viewsPath - Path to views directory (optional, defaults to component views)
 * @param {string} options.publicPath - Path to public directory (optional, defaults to component public)
 * @returns {express.Application} Configured Express app
 */
export function createVSODApp({
    services,
    rateLimiters,
    authMiddleware,
    authorizationMiddleware,
    apiKeyMiddleware,
    requestTimestampMiddleware,
    errorHandlerMiddleware,
    versionInfo,
    isTest = false,
    viewsPath = null,
    publicPath = null
}) {
    const app = express();

    // Set view engine and paths
    app.set('view engine', 'ejs');
    if (viewsPath) {
        app.set('views', viewsPath);
    } else {
        app.set('views', path.join(import.meta.url, '..', 'views'));
    }

    // Apply compression
    app.use(compression());

    // CSP nonce middleware (must run before helmet)
    app.use(cspNonce);

    // Test debugging middleware
    if (isTest && process.env.VITEST_DEBUG) {
        app.use((req, res, next) => {
            console.log(`[TEST-DEBUG] Request: ${req.method} ${req.url}`);
            const oldJson = res.json;
            res.json = function(data) {
                console.log(`[TEST-DEBUG] Response: ${res.statusCode}`, JSON.stringify(data, null, 2));
                return oldJson.call(this, data);
            };
            next();
        });
    }

    // Security headers
    app.use(helmet({
        contentSecurityPolicy: {
            directives: {
                defaultSrc:     ["'self'"],
                scriptSrc:      ["'self'", (req, res) => `'nonce-${res.locals.cspNonce}'`],
                scriptSrcAttr:  ["'none'"],
                styleSrc:       ["'self'", "'unsafe-inline'"],
                imgSrc:         ["'self'", 'data:'],
                fontSrc:        ["'self'", 'data:'],
                connectSrc:     ["'self'", 'wss:', 'ws:'],
                workerSrc:      ["'self'", 'blob:'],
                childSrc:       ["'self'", 'blob:'],
                objectSrc:      ["'none'"],
                baseUri:        ["'self'"],
                frameAncestors: ["'none'"],
                formAction:     ["'self'"],
                upgradeInsecureRequests: [],
            },
        },
        strictTransportSecurity: {
            maxAge: 31536000,
            includeSubDomains: true,
            preload: true
        },
        referrerPolicy: { policy: 'strict-origin-when-cross-origin' },
        crossOriginOpenerPolicy: { policy: 'same-origin' },
        crossOriginEmbedderPolicy: false
    }));

    // Additional permissions policy
    app.use((req, res, next) => {
        res.setHeader('Permissions-Policy', 
            'accelerometer=(), camera=(), geolocation=(), gyroscope=(), ' +
            'magnetometer=(), microphone=(), payment=(), usb=()'
        );
        next();
    });

    // CORS configuration
    const settingsService = services.settingsService;
    const allowedOriginsRaw = settingsService.allowed_origins;
    const allowedOrigins = [
        ...CORS_INTERNAL_ORIGINS,
        ...(allowedOriginsRaw ? allowedOriginsRaw.split(',').map(s => s.trim()).filter(Boolean) : []),
    ];
    app.use(cors({
        origin: allowedOrigins,
        credentials: true,
        methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
        allowedHeaders: [
            HTTP_CONTENT_TYPE_HEADER,
            'Authorization',
            HTTP_REQUESTED_WITH_HEADER,
            WEB_SESSION_ID_HEADER,
            HTTP_API_KEY_HEADER,
        ],
        maxAge: 86400
    }));

    // Global rate limiting (skip internal endpoints)
    app.use('/api', (req, res, next) => {
        if (req.path.startsWith('/internal')) {
            return next();
        }
        return rateLimiters.globalPublicRateLimiter(req, res, next);
    });

    // Body parsing middleware
    const bodyLimit = isTest ? '1mb' : '10mb'; // Smaller limit for tests
    app.use(express.json({ limit: bodyLimit }));
    app.use(express.urlencoded({ extended: true, limit: bodyLimit }));
    app.use(cookieParser());

    // Global context middleware (lazy evaluated vsoContext)
    app.use(globalContextMiddleware);

    // Asset version and theme middleware
    app.use((req, res, next) => {
        res.locals.assetVersion = versionInfo?.version || (isTest ? 'test-version' : 'unknown');
        const themeCookie = req.cookies?.theme;
        res.locals.theme = (themeCookie === 'light' || themeCookie === 'dark') ? themeCookie : 'dark';
        res.locals.auth_mode = 'local';
        next();
    });

    // Static files
    const staticPath = publicPath || path.join(import.meta.url, '..', 'public');
    app.use(express.static(staticPath, {
        maxAge: '1y',
        immutable: true,
        etag: true,
        lastModified: true,
        setHeaders: (res, filePath) => {
            if (filePath.endsWith('.xml') || filePath.endsWith('.txt') || filePath.endsWith('.json')) {
                res.setHeader('Cache-Control', 'public, max-age=3600, must-revalidate');
            } else if (filePath.endsWith('.html') || filePath.endsWith('.js') || filePath.endsWith('.css')) {
                res.setHeader('Cache-Control', 'no-cache');
            }
        }
    }));

    // Favicon
    app.get('/favicon.ico', (req, res) => {
        const faviconPath = path.join(staticPath, 'media', 'g8e.ai.logo.tiny.png');
        if (fs.existsSync(faviconPath)) {
            res.sendFile(faviconPath, {
                headers: { 'Content-Type': 'image/png' }
            });
        } else {
            res.status(404).send('Favicon not found');
        }
    });

    // CA certificate endpoint (only in non-test mode)
    if (!isTest) {
        app.get('/ca.crt', (req, res) => {
            const bootstrapService = services.settingsService.getBootstrapService();
            const sslDir = bootstrapService.getSslDir();
            if (!sslDir) {
                return res.status(404).send('SSL directory not available.');
            }
            const caPath = path.join(sslDir, 'ca.crt');
            if (!fs.existsSync(caPath)) {
                return res.status(404).send('CA certificate not yet available.');
            }
            res.setHeader('Content-Type', 'application/x-pem-file');
            res.setHeader('Content-Disposition', 'attachment; filename="g8e-ca.crt"');
            res.sendFile(caPath);
        });
    }

    // Mount all routes
    mountRoutes(app, {
        services,
        rateLimiters,
        authMiddleware,
        authorizationMiddleware,
        apiKeyMiddleware,
        requestTimestampMiddleware
    });

    // Error handler (must be last)
    app.use(errorHandlerMiddleware);

    return app;
}

/**
 * Mount all application routes
 */
function mountRoutes(app, {
    services,
    rateLimiters,
    authMiddleware,
    authorizationMiddleware,
    apiKeyMiddleware,
    requestTimestampMiddleware
}) {
    const { requirePageAuth, requirePageAdmin, optionalAuth } = authMiddleware;

    // Platform Routes
    app.use(BasePaths.HEALTH, createHealthRouter({ 
        services,
        authorizationMiddleware 
    }));
    
    app.use(BasePaths.CHAT, createChatRouter({
        services,
        authMiddleware,
        authorizationMiddleware,
        rateLimiters
    }));

    app.use(BasePaths.USER, createUserRouter({ 
        services,
        authMiddleware 
    }));

    app.use(BasePaths.METRICS, createMetricsRouter({ 
        services,
        authorizationMiddleware 
    }));

    app.use(BasePaths.AUDIT, createAuditRouter({
        services,
        authMiddleware,
        rateLimiters
    }));

    app.use(BasePaths.CONSOLE, createConsoleRouter({
        services,
        authMiddleware,
        rateLimiters
    }));

    app.use(BasePaths.SETTINGS, createSettingsRouter({
        services,
        authMiddleware,
        rateLimiters
    }));

    app.use(BasePaths.SYSTEM, createSystemRouter({
        services,
        authMiddleware,
        rateLimiters
    }));

    app.use(BasePaths.SSE, createSSERouter({
        services,
        authMiddleware,
        authorizationMiddleware,
        rateLimiters
    }));

    app.use(BasePaths.DOCS, createDocsRouter({ 
        services,
        authMiddleware 
    }));

    app.use(BasePaths.MCP, createMCPRouter({
        services,
        authMiddleware,
        rateLimiters
    }));

    // Auth Routes
    app.use(BasePaths.AUTH, createAuthRouter({ 
        services,
        authMiddleware,
        rateLimiters
    }));

    app.use(BasePaths.AUTH_PASSKEY, createPasskeyRouter({ 
        services,
        authMiddleware, 
        rateLimiters
    }));

    const deviceLinkRoutes = createDeviceLinkRouter({
        services,
        authMiddleware,
        rateLimiters
    });
    app.use(BasePaths.AUTH, deviceLinkRoutes.authRouter);
    app.use(BasePaths.DEVICE_LINKS, deviceLinkRoutes.deviceLinkRouter);
    app.use(BasePaths.AUTH_LINK, deviceLinkRoutes.registerRouter);

    const landingRouter = createLandingRouter({
        services
    });
    app.use('/', landingRouter);

    const setupRouterInstance = createSetupRouter({
        services,
        rateLimiters
    });
    app.use('/', setupRouterInstance);

    // Operator Routes
    app.use(BasePaths.OPERATOR, createOperatorRouter({ 
        services,
        authorizationMiddleware 
    }));

    app.use(BasePaths.OPERATOR_APPROVAL, createOperatorApprovalRouter({ 
        services,
        authMiddleware,
        rateLimiters
    }));

    app.use(BasePaths.AUTH, createOperatorAuthRouter({ 
        services,
        rateLimiters,
        requestTimestampMiddleware
    }));

    app.use(BasePaths.OPERATOR_API, createOperatorStatusRouter({
        services,
        authMiddleware,
        authorizationMiddleware
    }));

    app.use(BasePaths.OPERATOR_API, createBindOperatorsRouter({
        services,
        authMiddleware
    }));

    app.use(BasePaths.OPERATOR_API, createOperatorApiKeyRouter({
        services,
        authMiddleware,
        authorizationMiddleware
    }));

    // Internal Routes
    app.use(BasePaths.INTERNAL, createInternalRouter({
        services,
        authorizationMiddleware
    }));

    // Page routes (only in non-test mode)
    if (!process.env.VITEST) {
        const { userService } = services;
        const withDevLogs = async (req) => {
            try {
                const user = await userService.getUser(req.userId);
                return user?.dev_logs_enabled === true;
            } catch {
                return false;
            }
        };

        app.get('/chat', requirePageAuth({ onFail: 'redirect', redirectTo: '/' }), async (req, res) => {
            res.render('chat', { devLogsEnabled: await withDevLogs(req) });
        });

        app.get('/docs', optionalAuth, async (req, res) => {
            res.render('docs', { devLogsEnabled: await withDevLogs(req) });
        });

        app.get('/settings', requirePageAdmin(), async (req, res) => {
            res.render('settings', { devLogsEnabled: await withDevLogs(req) });
        });

        app.get('/console', requirePageAdmin(), async (req, res) => {
            res.render('console', { devLogsEnabled: await withDevLogs(req) });
        });

        app.get('/audit', requirePageAuth(), rateLimiters.auditRateLimiter, async (req, res) => {
            res.render('audit', { devLogsEnabled: await withDevLogs(req) });
        });
    }
}
