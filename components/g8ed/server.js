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
 * g8ed Node.js Server - Main Entry Point
 * 
 * g8e Dashboard (g8ed) - localhost platform with local authentication,
 * Zero-Trust AI for Real Production, and interactive terminal UI.
 */

import express from 'express';
import compression from 'compression';
import cors from 'cors';
import helmet from 'helmet';
import cookieParser from 'cookie-parser';
import https from 'https';
import http from 'http';
import fs from 'fs';
import net from 'net';
import tls from 'tls';

import path from 'path';
import { fileURLToPath } from 'url';

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
import { createG8edApp } from './app_factory.js';

import { 
    initializeServices, 
    getSettingsService, 
    getUserService,
    getOrganizationModel,
    getPubSubClient,
    getCacheAsideService,
    getWebSessionService,
    getOperatorSessionService,
    getCliSessionService,
    getBindingService,
    getApiKeyService,
    getOperatorService,
    getOperatorDownloadService,
    getDownloadAuthService,
    getLoginSecurityService,
    getPasskeyAuthService,
    getAttachmentService,
    getSSEService,
    getDeviceLinkService,
    getCertificateService,
    getConsoleMetricsService,
    getBindOperatorsService,
    getOperatorAuthService,
    getG8ENodeOperatorService,
    getPostLoginService,
    getAuditService,
    getSetupService,
    getG8esBlobClient,
    getInternalHttpClient,
    getHealthCheckService,
    getInvestigationService
} from './services/initialization.js';
import { logger } from './utils/logger.js';
import { 
    createRateLimiters
} from './middleware/rate-limit.js';
import { 
    createRequestTimestampMiddleware
} from './middleware/request_timestamp.js';
import {
    createErrorHandlerMiddleware
} from './middleware/error_handler.js';
import { cspNonce } from './middleware/csp_nonce.js';
import { createAuthMiddleware } from './middleware/authentication.js';
import { createApiKeyMiddleware } from './middleware/api_key_auth.js';
import { createAuthorizationMiddleware } from './middleware/authorization.js';
import { WEB_SESSION_ID_HEADER } from './constants/auth.js';
import { HTTP_API_KEY_HEADER, HTTP_CONTENT_TYPE_HEADER, HTTP_REQUESTED_WITH_HEADER } from './constants/headers.js';
import { CORS_INTERNAL_ORIGINS } from './constants/http_client.js';
import { getVersionInfo } from './utils/version.js';
import { windowsTrustScript, macosTrustScript, linuxTrustScript, g8eDeploy, universalTrustScript, windowsPowerShellTrustScript } from './utils/cert-installers.js';
import { BasePaths } from './constants/api_paths.js';
import { G8ES_PUBSUB_PATH } from './constants/http_client.js';


const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

class G8edServer {
    constructor() {
        this.app = express();
        this.tlsOptions = null;
    }

    async initialize() {
        try {
            await initializeServices();
            
            // Collect all service instances from the initialization module
            this.services = {
                organizationModel: getOrganizationModel(),
                pubSubClient: getPubSubClient(),
                cacheAsideService: getCacheAsideService(),
                webSessionService: getWebSessionService(),
                operatorSessionService: getOperatorSessionService(),
                cliSessionService: getCliSessionService(),
                bindingService: getBindingService(),
                apiKeyService: getApiKeyService(),
                userService: getUserService(),
                operatorService: getOperatorService(),
                operatorDownloadService: getOperatorDownloadService(),
                downloadAuthService: getDownloadAuthService(),
                loginSecurityService: getLoginSecurityService(),
                passkeyAuthService: getPasskeyAuthService(),
                attachmentService: getAttachmentService(),
                sseService: getSSEService(),
                deviceLinkService: getDeviceLinkService(),
                certificateService: getCertificateService(),
                settingsService: getSettingsService(),
                consoleMetricsService: getConsoleMetricsService(),
                bindOperatorsService: getBindOperatorsService(),
                operatorAuthService: getOperatorAuthService(),
                g8eNodeOperatorService: getG8ENodeOperatorService(),
                postLoginService: getPostLoginService(),
                auditService: getAuditService(),
                setupService: getSetupService(),
                blobStorage: getG8esBlobClient(),
                internalHttpClient: getInternalHttpClient(),
                healthCheckService: getHealthCheckService(),
                investigationService: getInvestigationService()
            };

            // No config object - use services directly
            
            const { userService, webSessionService, setupService, operatorService, apiKeyService, settingsService, bindingService } = this.services;
            const bootstrapService = settingsService.getBootstrapService();
            
            // Initialize middleware with minimal dependencies
            this.rateLimiters = createRateLimiters({ });
            this.requestTimestampMiddleware = createRequestTimestampMiddleware({ cacheAsideService: this.services.cacheAsideService });
            this.errorHandlerMiddleware = createErrorHandlerMiddleware({ });
            const { globalPublicRateLimiter } = this.rateLimiters;

            this.authMiddleware = createAuthMiddleware({ 
                userService, 
                webSessionService, 
                setupService,
                settingsService,
                bindingService
            });
            this.apiKeyMiddleware = createApiKeyMiddleware({ apiKeyService, userService });
            this.authorizationMiddleware = createAuthorizationMiddleware({ operatorService, settingsService });

            this.configureMiddleware();

            await this.configureRoutes();

            this.configureErrorHandling();

        } catch (error) {
            logger.error('Failed to initialize g8ed server:', error);
            process.exit(1);
        }
    }

    configureMiddleware() {
        // Use 1 (first hop) instead of true - prevents IP spoofing in rate limiting
        this.app.set('trust proxy', 1);

        this.app.use(cspNonce);

        this.app.set('view engine', 'ejs');
        this.app.set('views', path.join(__dirname, 'views'));

        // Exclude SSE streams and chat routes which require unbuffered delivery
        this.app.use(compression({
            filter: (req, res) => {
                if (req.path.startsWith(BasePaths.SSE + '/')) return false;
                if (req.path.startsWith(BasePaths.CHAT + '/')) return false;
                if (req.path.startsWith(BasePaths.AUDIT + '/stream')) return false;
                return compression.filter(req, res);
            },
            threshold: 1024
        }));

        this.app.use((req, res, next) => {
            if (process.env.VITEST_DEBUG) {
                console.log(`[TEST-DEBUG] Request: ${req.method} ${req.url}`);
                const oldJson = res.json;
                res.json = function(data) {
                    console.log(`[TEST-DEBUG] Response: ${res.statusCode}`, JSON.stringify(data, null, 2));
                    return oldJson.call(this, data);
                };
            }
            next();
        });

        this.app.use(helmet({
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

        this.app.use((req, res, next) => {
            res.setHeader('Permissions-Policy', 
                'accelerometer=(), camera=(), geolocation=(), gyroscope=(), ' +
                'magnetometer=(), microphone=(), payment=(), usb=()'
            );
            next();
        });

        const settingsService = this.services.settingsService;
        const allowedOriginsRaw = settingsService.allowed_origins;
        const allowedOrigins = [
            ...CORS_INTERNAL_ORIGINS,
            ...(allowedOriginsRaw ? allowedOriginsRaw.split(',').map(s => s.trim()).filter(Boolean) : []),
        ];

        this.app.use(cors({
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

        // Internal endpoints exempt - cluster-only, auth-token-protected
        this.app.use('/api', (req, res, next) => {
            if (req.path.startsWith('/internal')) {
                return next();
            }
            return this.rateLimiters.globalPublicRateLimiter(req, res, next);
        });

        this.app.use(express.json({ limit: '10mb' }));
        this.app.use(express.urlencoded({ extended: true, limit: '10mb' }));
        this.app.use(cookieParser());

        const versionInfo = getVersionInfo();
        this.app.use((req, res, next) => {
            res.locals.assetVersion = versionInfo.version;
            const themeCookie = req.cookies?.theme;
            res.locals.theme = (themeCookie === 'light' || themeCookie === 'dark') ? themeCookie : 'dark';
            res.locals.auth_mode = 'local';
            next();
        });

        this.app.use(express.static(path.join(__dirname, 'public'), {
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

        this.app.get('/favicon.ico', (req, res) => {
            res.sendFile(path.join(__dirname, 'public', 'media', 'g8e.ai.logo.tiny.png'), {
                headers: { 'Content-Type': 'image/png' }
            });
        });

        // Fallback for certificate download over HTTPS (if already partially trusted or ignored warning)
        this.app.get('/ca.crt', (req, res) => {
            const bootstrapService = this.services.settingsService.getBootstrapService();
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

    async configureRoutes() {
        // Create the Express app using the factory
        this.app = createG8edApp({
            services: this.services,
            rateLimiters: this.rateLimiters,
            authMiddleware: this.authMiddleware,
            authorizationMiddleware: this.authorizationMiddleware,
            apiKeyMiddleware: this.apiKeyMiddleware,
            requestTimestampMiddleware: this.requestTimestampMiddleware,
            errorHandlerMiddleware: this.errorHandlerMiddleware,
            versionInfo: getVersionInfo(),
            isTest: false,
            viewsPath: path.join(process.cwd(), 'views'),
            publicPath: path.join(process.cwd(), 'public')
        });
    }

    configureErrorHandling() {
        // Error handling is already configured in the app factory
    }

    async start() {
        await this.initialize();
        
        // Use ports from SettingsService/BootstrapService with fallback defaults
        const bootstrapService = this.services.settingsService.getBootstrapService();
        const httpsPort = parseInt(bootstrapService.loadHttpsPort?.()) || 443;
        const httpPort = parseInt(bootstrapService.loadHttpPort?.()) || 80;
        const internalPort = parseInt(bootstrapService.loadInternalPort?.()) || 9000;
        
        // Simple TLS check - look for certs in standard g8es SSL location
        const sslDir = bootstrapService.getSslDir();
        let tlsOptions = null;
        
        if (sslDir) {
            const certPath = path.join(sslDir, 'server.crt');
            const keyPath = path.join(sslDir, 'server.key');
            
            if (fs.existsSync(certPath) && fs.existsSync(keyPath)) {
                tlsOptions = {
                    cert: fs.readFileSync(certPath),
                    key: fs.readFileSync(keyPath)
                };
                logger.info('[g8ed] TLS certificates loaded', { certPath, keyPath });
            }
        }

        if (tlsOptions) {
            // HTTPS server for external traffic (browsers, operators)
            this.server = https.createServer(tlsOptions, this.app);
            this.server.listen(httpsPort, () => {
                logger.info(`g8ed HTTPS server running on port ${httpsPort}`);
            });

            // HTTP server on port 80 - serves CA trust page for all traffic
            this._createHttpServer(httpPort, httpsPort);

        } else {
            // No TLS certs: plain HTTP only on internal port
            this.server = this.app.listen(internalPort, () => {
                logger.info(`g8ed server running on port ${internalPort} (HTTP - no TLS certs found)`);
            });
        }

        // Unlimited request timeout for SSE connections
        this.server.timeout = 0;
        this.server.keepAliveTimeout = 65000;
        this.server.headersTimeout = 66000;

        // Proxy WebSocket upgrade requests for /ws/pubsub to g8es.
        // g8ed is the single external entry point — operators dial g8ed:443 for
        // both HTTP auth and WebSocket pub/sub, and g8ed tunnels the latter to g8es.
        this.server.on('upgrade', (request, socket, head) => {
            const pathname = new URL(request.url, 'https://localhost').pathname;
            if (pathname === G8ES_PUBSUB_PATH) {
                this._proxyWebSocket(request, socket, head);
            } else {
                logger.warn('[g8ed] WebSocket upgrade rejected — unsupported path', { pathname });
                socket.destroy();
            }
        });
    }

    _createHttpServer(httpPort, httpsPort) {
        const bootstrapService = this.services.settingsService.getBootstrapService();
        const sslDir = bootstrapService.getSslDir();
        if (!sslDir) {
            logger.warn('[g8ed] SSL directory not available, HTTP server disabled');
            return;
        }
        
        const caPath = path.join(sslDir, 'ca.crt');
        
        const securityHeaders = {
            'Content-Security-Policy': "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self';",
            'X-Content-Type-Options': 'nosniff',
            'X-Frame-Options': 'DENY',
            'X-XSS-Protection': '1; mode=block',
            'Referrer-Policy': 'strict-origin-when-cross-origin'
        };

        this.httpRedirectServer = http.createServer(async (req, res) => {
            try {
                const url = (req.url || '/').split('?')[0].split('#')[0];
                const host = (req.headers.host || 'localhost').replace(/:\d+$/, '');
                
                // Serve CA certificate file
                if (url === '/ca.crt') {
                    if (!fs.existsSync(caPath)) {
                        res.writeHead(404, { ...securityHeaders, 'Content-Type': 'text/plain' });
                        res.end('CA certificate not yet available.');
                        return;
                    }
                    const cert = fs.readFileSync(caPath);
                    res.writeHead(200, {
                        ...securityHeaders,
                        'Content-Type': 'application/x-pem-file',
                        'Content-Disposition': 'attachment; filename="g8e-ca.crt"',
                        'Content-Length': cert.length,
                    });
                    res.end(cert);
                    return;
                }
                
                // Serve trust scripts
                if (url === '/trust.sh') {
                    const content = macosTrustScript(host, httpPort);
                    res.writeHead(200, {
                        ...securityHeaders,
                        'Content-Type': 'application/x-sh',
                        'Content-Disposition': 'attachment; filename="trust-g8e-cert.sh"',
                        'Content-Length': Buffer.byteLength(content),
                    });
                    res.end(content);
                    return;
                }
                
                if (url === '/trust.bat') {
                    const content = windowsTrustScript(host, httpPort);
                    res.writeHead(200, {
                        ...securityHeaders,
                        'Content-Type': 'application/x-bat',
                        'Content-Disposition': 'attachment; filename="trust-g8e-cert.bat"',
                        'Content-Length': Buffer.byteLength(content),
                    });
                    res.end(content);
                    return;
                }
                
                if (url === '/g8e') {
                    const content = g8eDeploy(host, httpsPort, httpPort);
                    res.writeHead(200, {
                        ...securityHeaders,
                        'Content-Type': 'text/plain; charset=utf-8',
                        'Content-Length': Buffer.byteLength(content),
                    });
                    res.end(content);
                    return;
                }
                
                if (url === '/trust-linux.sh') {
                    const content = linuxTrustScript(host, httpPort);
                    res.writeHead(200, {
                        ...securityHeaders,
                        'Content-Type': 'application/x-sh',
                        'Content-Disposition': 'attachment; filename="trust-g8e-cert-linux.sh"',
                        'Content-Length': Buffer.byteLength(content),
                    });
                    res.end(content);
                    return;
                }
                
                if (url === '/trust') {
                    const ua = (req.headers['user-agent'] || '').toLowerCase();
                    const isWindows = ua.includes('windows') || ua.includes('win32') || ua.includes('win64') || ua.includes('powershell');
                    if (isWindows) {
                        const content = windowsPowerShellTrustScript(host, httpPort);
                        res.writeHead(200, {
                            ...securityHeaders,
                            'Content-Type': 'text/plain; charset=utf-8',
                            'Content-Length': Buffer.byteLength(content),
                        });
                        res.end(content);
                    } else {
                        const content = universalTrustScript(host, httpPort);
                        res.writeHead(200, {
                            ...securityHeaders,
                            'Content-Type': 'text/plain; charset=utf-8',
                            'Content-Length': Buffer.byteLength(content),
                        });
                        res.end(content);
                    }
                    return;
                }
                
                // Serve CA trust page for all other requests
                const secureSetupUrl = `https://${host}/setup`;
                const page = this._loadCaTrustPage(host, secureSetupUrl);
                res.writeHead(200, {
                    ...securityHeaders,
                    'Content-Type': 'text/html; charset=utf-8',
                    'Content-Length': Buffer.byteLength(page),
                });
                res.end(page);
                
            } catch (err) {
                logger.error('[g8ed] HTTP server error', { error: err.message });
                res.writeHead(500, securityHeaders);
                res.end('Internal Server Error');
            }
        });
        
        this.httpRedirectServer.listen(httpPort, () => {
            logger.info(`g8ed HTTP server running on port ${httpPort} (CA trust page)`);
        });
    }

    _loadCaTrustPage(host, secureSetupUrl) {
        const tplPath = path.join(__dirname, 'views', 'ca-trust.html');
        return fs.readFileSync(tplPath, 'utf8')
            .replaceAll('{{HOST}}', host)
            .replaceAll('{{SECURE_SETUP_URL}}', secureSetupUrl);
    }

    _proxyWebSocket(request, socket, head) {
        // Use default g8es WebSocket URL - no config dependency
        const g8esUrl = new URL('wss://g8es:9001');
        const port = parseInt(g8esUrl.port, 10) || 9001;
        const host = g8esUrl.hostname;
        const isSecure = g8esUrl.protocol === 'wss:';

        // Get auth token from bootstrap service
        const bootstrapService = this.services.settingsService.getBootstrapService();
        const internalAuthToken = bootstrapService.loadInternalAuthToken();

        const tlsOptions = isSecure ? (() => {
            const caCertPath = bootstrapService.loadCaCertPath();
            const opts = { host, port, servername: host, rejectUnauthorized: true };
            if (caCertPath && fs.existsSync(caCertPath)) {
                opts.ca = fs.readFileSync(caCertPath);
            }
            return opts;
        })() : null;

        const connect = isSecure
            ? (cb) => tls.connect(tlsOptions, cb)
            : (cb) => net.createConnection({ host, port }, cb);

        const upstream = connect(() => {
            const reqLine = `${request.method} ${request.url} HTTP/${request.httpVersion}\r\n`;
            
            // Inject internal auth token into headers for the upstream g8es connection
            const proxyHeaders = { ...request.headers };
            if (internalAuthToken) {
                proxyHeaders['X-Internal-Auth'] = internalAuthToken;
            }

            const headers = Object.entries(proxyHeaders)
                .map(([k, v]) => `${k}: ${v}`)
                .join('\r\n');
            upstream.write(`${reqLine}${headers}\r\n\r\n`);
            if (head && head.length) upstream.write(head);
            upstream.pipe(socket);
            socket.pipe(upstream);
        });

        upstream.on('error', (err) => {
            logger.error('[g8ed] WebSocket proxy to g8es failed', { error: err.message });
            socket.destroy();
        });

        socket.on('error', () => upstream.destroy());
    }

    async shutdown() {
        logger.info('Shutting down g8ed server...');

        if (this.httpRedirectServer) {
            this.httpRedirectServer.close(() => {
                logger.info('g8ed HTTP redirect server closed');
            });
        }

        if (this.internalServer) {
            this.internalServer.close(() => {
                logger.info('g8ed internal HTTP server closed');
            });
        }

        if (this.server) {
            this.server.close(() => {
                logger.info('g8ed server closed');
                process.exit(0);
            });
        }
    }
}

process.on('unhandledRejection', (reason, promise) => {
    logger.error('[UNHANDLED REJECTION]', {
        reason: reason?.message || String(reason),
        stack: reason?.stack,
        promiseString: String(promise)
    });
});

process.on('uncaughtException', (error) => {
    logger.error('[UNCAUGHT EXCEPTION]', {
        error: error.message,
        stack: error.stack
    });
});

const server = new G8edServer();
server.start().catch((error) => {
    logger.error('Failed to start g8ed server:', error);
    process.exit(1);
});

export default G8edServer;
