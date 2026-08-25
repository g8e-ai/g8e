// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import crypto from 'crypto';

/**
 * Generates a per-request CSP nonce and attaches it to res.locals.cspNonce.
 * Must run before the helmet CSP middleware so the nonce is available for the
 * contentSecurityPolicy directives callback.
 */
export function cspNonce(req, res, next) {
    res.locals.cspNonce = crypto.randomBytes(16).toString('base64');
    next();
}
