// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8e Dashboard (g8ed) — Static SPA Host
 *
 * Serves the browser SPA from public/ over plain HTTP on port 3000. No TLS,
 * no auth, no session management, no WebSocket proxy. The browser loads the
 * SPA from http://localhost:3000 and makes all auth/API calls directly to
 * the g8e Gateway at https://localhost:8443 with credentials: 'include'.
 *
 * See docs/guides/connect_frontend_to_gateway.md for the browser-direct
 * architecture and docs/guides/build_frontend.md for the SPA contract.
 */

import express from 'express';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const GATEWAY_ORIGIN = process.env.G8E_GATEWAY_URL || 'https://localhost:8443';
const PORT = parseInt(process.env.PORT || '3000', 10);

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
        `connect-src 'self' ${GATEWAY_ORIGIN}`,
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
app.get('*splat', (req, res, next) => {
    if (req.headers.accept?.includes('text/html')) {
        return res.sendFile(path.join(__dirname, 'public', 'index.html'), (err) => {
            if (err) next(err);
        });
    }
    next();
});

export { app };

const isEntryPoint = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isEntryPoint) {
    app.listen(PORT, () => {
        console.log(`g8ed serving on http://localhost:${PORT} (gateway: ${GATEWAY_ORIGIN})`);
    });
}
