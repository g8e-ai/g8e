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
 * HTTP Client Constants
 * Configuration for all outbound HTTP/WebSocket clients: VSOD→VSE internal
 * client, VSOD→VSODB HTTP client, and VSODB PubSub WebSocket client.
 * Also includes CORS origins that bypass the ALLOWED_ORIGINS env var check.
 */

// ---------------------------------------------------------------------------
// Internal Cluster URLs - VSODB uses ports 9000 (HTTPS) and 9001 (WSS)
// ---------------------------------------------------------------------------
export const VSE_INTERNAL_URL = 'https://vse';
export const VSOD_INTERNAL_URL = 'https://vsod';
export const VSODB_INTERNAL_HTTP_URL = 'https://vsodb:9000';

// ---------------------------------------------------------------------------
// Internal HTTP Client (VSOD -> VSE)
// ---------------------------------------------------------------------------
export const INTERNAL_HTTP_TIMEOUT_MS = 5000;
export const INTERNAL_HTTP_CLIENT_USER_AGENT = 'vsod-internal-client/1.0';
export const NEW_CASE_ID = 'new-case-via-vsod';

/**
 * CORS origins always permitted regardless of ALLOWED_ORIGINS env var.
 * These are internal docker-compose service-to-service origins.
 */
export const CORS_INTERNAL_ORIGINS = Object.freeze([
    'https://vse',
    'https://vsod',
    'https://localhost',
]);

// ---------------------------------------------------------------------------
// VSODB HTTP Client (VSOD -> VSODB)
// ---------------------------------------------------------------------------
export const VSODB_HTTP_TIMEOUT_MS = 30000;

// ---------------------------------------------------------------------------
// VSODB PubSub WebSocket Client (VSOD -> VSODB)
// ---------------------------------------------------------------------------
export const VSODB_INTERNAL_PUBSUB_URL = 'wss://vsodb:9001';
export const VSODB_OPERATOR_PUBSUB_URL = 'wss://g8e.local';
export const VSODB_PUBSUB_PATH = '/ws/pubsub';
export const VSODB_PUBSUB_PUBLISH_PATH = '/publish';

// ---------------------------------------------------------------------------
// VSODB KV Client
// ---------------------------------------------------------------------------
export const VSODB_KV_CLIENT_STATUS_READY = 'ready';
export const KV_SCAN_DEFAULT_COUNT = 100;
export const KV_CLIENT_READY_WAIT_MS = 5000;
export const KV_CLIENT_POLL_INTERVAL_MS = 50;

// ---------------------------------------------------------------------------
// VSODB PubSub Client
// ---------------------------------------------------------------------------
export const PUBSUB_RECONNECT_DELAY_MS = 1000;

