// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8e Dashboard (g8ed) — Static SPA Host
 *
 * Serves the browser SPA from public/ over plain HTTP on port 3000. No TLS,
 * no auth, no session management, no WebSocket proxy. The browser loads the
 * SPA from http://localhost:3000 and makes all auth/API calls directly to
 * the g8e Gateway at the origin configured via G8E_GATEWAY_URL (required,
 * no fallback) with credentials: 'include'.
 *
 * The gateway origin is injected into the browser via a dedicated
 * /g8e-config.js endpoint served by this host. The SPA loads it before any
 * other script. There is no hardcoded localhost fallback — deployments that
 * do not set G8E_GATEWAY_URL fail closed at startup.
 *
 * See docs/guides/connect_frontend_to_gateway.md for the browser-direct
 * architecture and docs/guides/build_frontend.md for the SPA contract.
 */

import express from 'express';
import rateLimit from 'express-rate-limit';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const ERR_GATEWAY_ORIGIN_REQUIRED = 'G8E_GATEWAY_URL is required: no gateway origin configured for the browser SPA';

/**
 * Build the Express application. The gateway origin is an explicit
 * construction-phase parameter — no env reads, no fallbacks, no magic.
 *
 * @param {Object} config
 * @param {string} config.gatewayOrigin - HTTPS origin of the g8e Gateway (e.g. "https://host:8443")
 * @returns {import('express').Express}
 */
export function createApp({ gatewayOrigin }) {
    if (!gatewayOrigin) {
        throw new Error(ERR_GATEWAY_ORIGIN_REQUIRED);
    }

    const app = express();

    // CSP: allow the SPA to connect to the gateway over HTTPS for API calls and
    // SSE. No ws:/wss: — the gateway WebSocket endpoint requires mTLS and is not
    // available to browsers (per docs/guides/connect_frontend_to_gateway.md).
    app.use((req, res, next) => {
        res.setHeader('Content-Security-Policy', [
            "default-src 'self'",
            "script-src 'self' 'unsafe-inline'",
            "style-src 'self' 'unsafe-inline'",
            "img-src 'self' data:",
            "font-src 'self' data:",
            `connect-src 'self' ${gatewayOrigin}`,
            "worker-src 'self' blob:",
            "child-src 'self' blob:",
            "object-src 'none'",
            "base-uri 'self'",
            "frame-ancestors 'none'",
            "form-action 'self'",
        ].join('; '));
        res.setHeader('X-Content-Type-Options', 'nosniff');
        res.setHeader('X-Frame-Options', 'DENY');
        res.setHeader('Referrer-Policy', 'strict-origin-when-cross-origin');
        res.setHeader('Permissions-Policy', 'accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()');
        next();
    });

    // Request logging. Records method, path, status, and duration for every
    // request. Uses res.on('finish') so the logged status reflects the actual
    // response code, including errors thrown by downstream handlers.
    app.use((req, res, next) => {
        const start = process.hrtime.bigint();
        res.on('finish', () => {
            const ms = Number(process.hrtime.bigint() - start) / 1e6;
            console.log(`${req.method} ${req.originalUrl} ${res.statusCode} ${ms.toFixed(1)}ms`);
        });
        next();
    });

    // Browser-facing gateway origin. Served as a dedicated JS endpoint so the
    // SPA can load it before any other script. The value is the explicit
    // gatewayOrigin passed at construction — no client-side fallback.
    app.get('/g8e-config.js', (req, res) => {
        res.type('application/javascript');
        res.set('Cache-Control', 'no-cache, no-store, must-revalidate');
        res.send(`window.G8E_GATEWAY_URL = ${JSON.stringify(gatewayOrigin)};`);
    });

    app.use(express.static(path.join(__dirname, 'public'), {
        maxAge: '1y',
        immutable: true,
        etag: true,
        lastModified: true,
        setHeaders: (res, filePath) => {
            if (filePath.endsWith('.html') || filePath.endsWith('.js') || filePath.endsWith('.css')) {
                res.setHeader('Cache-Control', 'no-cache');
            }
        },
    }));

    // SPA fallback: unknown GET routes that accept HTML serve index.html so
    // client-side routing works. path-to-regexp v8 (Express 5) requires a named
    // wildcard — the bare '*' form throws PathError at route registration.
    // Rate-limited to prevent DoS via repeated file system access.
    const spaFallbackRateLimiter = rateLimit({
        windowMs: 60 * 1000,
        max: 100,
        standardHeaders: true,
        message: 'Too many requests, please try again later.',
    });
    app.get('*splat', spaFallbackRateLimiter, (req, res, next) => {
        if (req.headers.accept?.includes('text/html')) {
            return res.sendFile(path.join(__dirname, 'public', 'index.html'), (err) => {
                if (err) next(err);
            });
        }
        next();
    });

    return app;
}

/**
 * Run the startup enrollment phase: resolve the dashboard's mTLS app
 * identity before the Express server listens. Try to load an existing
 * valid cert from disk first; if none is available (missing, expired, or
 * near-expiry), enroll with the gateway to obtain a fresh one.
 *
 * Fail-closed: on an unexpected (non-ConfigurationError) load failure or
 * on enrollment failure, the callback is invoked with the error so the
 * caller can `process.exit(1)`. On success, the resolved `AppIdentity` is
 * stored via `app-identity.js` and returned.
 *
 * Extracted from the entrypoint block so the startup phase is unit-testable
 * without spawning a process: tests construct a mock `AppEnrollmentService`
 * and assert the load-then-enroll decision and the fail-closed exit paths.
 *
 * @param {Object} deps
 * @param {import('./services/infra/app-enrollment-service.js').AppEnrollmentService} deps.enrollmentService
 * @param {function(Error): void} deps.onFatalError - Invoked when the caller
 *   should exit(1). Decoupled from `process.exit` so tests can assert the
 *   fail-closed behavior without terminating the test process.
 * @returns {Promise<import('./services/infra/app-enrollment-service.js').AppIdentity | null>}
 *   The resolved identity, or null if `onFatalError` was invoked.
 */
export async function runStartupEnrollment({ enrollmentService, onFatalError }) {
    const { ConfigurationError } = await import('./services/infra/app-enrollment-service.js');
    const { setAppIdentity } = await import('./services/infra/app-identity.js');

    let appIdentity;
    try {
        appIdentity = await enrollmentService.loadIdentity();
    } catch (err) {
        if (!(err instanceof ConfigurationError)) {
            onFatalError(err);
            return null;
        }
        try {
            appIdentity = await enrollmentService.enroll();
        } catch (enrollErr) {
            onFatalError(enrollErr);
            return null;
        }
    }
    setAppIdentity(appIdentity);
    return appIdentity;
}

const isEntryPoint = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isEntryPoint) {
    const gatewayOrigin = process.env.G8E_GATEWAY_URL;
    if (!gatewayOrigin) {
        console.error(`[g8ed] ${ERR_GATEWAY_ORIGIN_REQUIRED}`);
        process.exit(1);
    }
    const PORT = parseInt(process.env.PORT || '3000', 10);

    // -- Startup enrollment phase --
    // Resolve the dashboard's mTLS app identity before listening. See
    // `runStartupEnrollment` for the load-then-enroll decision and the
    // fail-closed exit paths. The stored identity is consumed by future
    // backend service wiring (g8eg mTLS clients).
    const { AppEnrollmentService } = await import('./services/infra/app-enrollment-service.js');
    const enrollmentService = new AppEnrollmentService();
    const appIdentity = await runStartupEnrollment({
        enrollmentService,
        onFatalError: (err) => {
            console.error(`[g8ed] AppEnrollmentService: ${err.message}`);
            process.exit(1);
        },
    });
    if (appIdentity === null) {
        // onFatalError already called process.exit(1); this is unreachable
        // in production but keeps static analysis happy in tests that stub
        // onFatalError without exiting.
        throw new Error('unreachable: runStartupEnrollment returned null without exiting');
    }

    const app = createApp({ gatewayOrigin });
    app.listen(PORT, () => {
        console.log(`g8ed serving on http://localhost:${PORT} (gateway: ${gatewayOrigin})`);
    });
}
