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
 * Configuration for all outbound HTTP/WebSocket clients: g8ed→g8ee internal
 * client, g8ed→g8ed HTTP client, and g8es PubSub WebSocket client.
 * Also includes CORS origins that bypass the ALLOWED_ORIGINS env var check.
 */

// ---------------------------------------------------------------------------
// Internal Cluster URLs - g8es uses ports 9000 (HTTPS) and 9001 (WSS)
// ---------------------------------------------------------------------------
export const G8EE_INTERNAL_URL = 'https://g8ee';
export const G8ED_INTERNAL_URL = 'https://g8ed';
export const G8ES_INTERNAL_HTTP_URL = 'https://g8es:9000';

// ---------------------------------------------------------------------------
// Internal HTTP Client (g8ed -> g8ee)
// ---------------------------------------------------------------------------
export const INTERNAL_HTTP_TIMEOUT_MS = 5000;
export const INTERNAL_HTTP_CLIENT_USER_AGENT = 'g8ed-internal-client/1.0';
export const NEW_CASE_ID = 'new-case-via-g8ed';

/**
 * CORS origins always permitted regardless of ALLOWED_ORIGINS env var.
 * These are internal docker-compose service-to-service origins.
 */
export const CORS_INTERNAL_ORIGINS = Object.freeze([
    'https://g8ee',
    'https://g8ed',
    'https://localhost',
]);

// ---------------------------------------------------------------------------
// g8ed HTTP Client (g8ed -> g8es)
// ---------------------------------------------------------------------------
export const G8ES_HTTP_TIMEOUT_MS = 30000;

// ---------------------------------------------------------------------------
// g8es PubSub WebSocket Client (g8ed -> g8es)
// ---------------------------------------------------------------------------
export const G8ES_INTERNAL_PUBSUB_URL = 'wss://g8es:9001';
export const G8ES_OPERATOR_PUBSUB_URL = 'wss://g8e.local';
export const G8ES_PUBSUB_PATH = '/ws/pubsub';
export const G8ES_PUBSUB_PUBLISH_PATH = '/publish';

// ---------------------------------------------------------------------------
// g8es KV Client
// ---------------------------------------------------------------------------
export const G8ES_KV_CLIENT_STATUS_READY = 'ready';
export const KV_SCAN_DEFAULT_COUNT = 100;
export const KV_CLIENT_READY_WAIT_MS = 5000;
export const KV_CLIENT_POLL_INTERVAL_MS = 50;

// ---------------------------------------------------------------------------
// g8es PubSub Client
// ---------------------------------------------------------------------------
export const PUBSUB_RECONNECT_DELAY_MS = 1000;

