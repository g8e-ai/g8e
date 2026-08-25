// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * HTTP Client Constants
 * Configuration for all outbound HTTP/WebSocket clients: g8ed→VSE internal
 * client, g8ed→g8eg HTTP client, and g8eg PubSub WebSocket client.
 * Also includes CORS origins that bypass the ALLOWED_ORIGINS env var check.
 */

import { ApiPaths } from './api_paths.js';

// ---------------------------------------------------------------------------
// Internal Cluster URLs - gateway uses port 8443 (HTTPS/WSS)
// ---------------------------------------------------------------------------
export const VSE_INTERNAL_URL = 'https://vse';
export const g8ed_INTERNAL_URL = 'https://g8ed';
export const g8eg_INTERNAL_HTTP_URL = 'https://g8eg:8443';

// ---------------------------------------------------------------------------
// Internal HTTP Client (g8ed -> VSE)
// ---------------------------------------------------------------------------
export const INTERNAL_HTTP_TIMEOUT_MS = 5000;
export const INTERNAL_HTTP_CLIENT_USER_AGENT = 'g8ed-internal-client/1.0';
export const NEW_CASE_ID = 'new-case-via-g8ed';

/**
 * CORS origins always permitted regardless of ALLOWED_ORIGINS env var.
 * These are internal docker-compose service-to-service origins.
 */
export const CORS_INTERNAL_ORIGINS = Object.freeze([
    'https://vse',
    'https://g8ed',
    'https://localhost',
]);

// ---------------------------------------------------------------------------
// g8eg HTTP Client (g8ed -> g8eg)
// ---------------------------------------------------------------------------
export const g8eg_HTTP_TIMEOUT_MS = 30000;

// ---------------------------------------------------------------------------
// g8eg PubSub WebSocket Client (g8ed -> g8eg)
// ---------------------------------------------------------------------------
export const g8eg_INTERNAL_PUBSUB_URL = 'wss://g8eg:8443';
export const g8eg_OPERATOR_PUBSUB_URL = 'wss://g8e.local';
export const g8eg_PUBSUB_PATH = ApiPaths.gateway.pubsubWebsocket();
export const g8eg_PUBSUB_PUBLISH_PATH = '/publish';

// ---------------------------------------------------------------------------
// g8eg KV Client
// ---------------------------------------------------------------------------
export const g8eg_KV_CLIENT_STATUS_READY = 'ready';
export const KV_SCAN_DEFAULT_COUNT = 100;
export const KV_CLIENT_READY_WAIT_MS = 5000;
export const KV_CLIENT_POLL_INTERVAL_MS = 50;

// ---------------------------------------------------------------------------
// g8eg PubSub Client
// ---------------------------------------------------------------------------
export const PUBSUB_RECONNECT_DELAY_MS = 1000;

