// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * HTTP Client Constants
 * Configuration for the g8ed→VSE internal HTTP client.
 */

// ---------------------------------------------------------------------------
// Internal Cluster URLs - gateway uses port 8443 (HTTPS/WSS)
// ---------------------------------------------------------------------------
export const VSE_INTERNAL_URL = 'https://vse';
export const g8eg_INTERNAL_HTTP_URL = 'https://g8eg:8443';

// ---------------------------------------------------------------------------
// Internal HTTP Client (g8ed -> VSE)
// ---------------------------------------------------------------------------
export const INTERNAL_HTTP_TIMEOUT_MS = 5000;
export const INTERNAL_HTTP_CLIENT_USER_AGENT = 'g8ed-internal-client/1.0';
export const NEW_CASE_ID = 'new-case-via-g8ed';

// ---------------------------------------------------------------------------
// g8eg HTTP Client (g8ed -> g8eg)
// ---------------------------------------------------------------------------
export const g8eg_HTTP_TIMEOUT_MS = 30000;
