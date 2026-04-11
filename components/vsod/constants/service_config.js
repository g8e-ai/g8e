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
 * Service Configuration Constants
 * Single source of truth for service URLs, binary config, certificate paths,
 * operator slots, file size limits, and platform definitions across VSOD
 */

import { _DOCUMENT_IDS } from './shared.js';

// ---------------------------------------------------------------------------
// First-Run Admin
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Operator Binary
// ---------------------------------------------------------------------------
export const BINARY_NAME = 'g8e.operator';
export const OPERATOR_BINARY_BLOB_NAMESPACE = 'operator-binary';
export const PLATFORMS = [
    { os: 'linux', arch: 'amd64' },
    { os: 'linux', arch: 'arm64' },
    { os: 'linux', arch: '386' }
];

export const BinaryStatus = Object.freeze({
    ALL_AVAILABLE:      'all_available',
    PARTIAL_OR_MISSING: 'partial_or_missing',
});

export const OperatorRouteError = Object.freeze({
    UNSUPPORTED_OS:          'g8e Operator only supports Linux',
    BINARY_NOT_AVAILABLE:    'g8e Operator binary not available',
    DOWNLOAD_FAILED:         'Failed to download Operator binary',
    CHECKSUM_FAILED:         'Failed to generate checksum',
});

export const ContentType = Object.freeze({
    OCTET_STREAM: 'application/octet-stream',
    TEXT_PLAIN:   'text/plain',
});

// ---------------------------------------------------------------------------
// Certificate Paths
// ---------------------------------------------------------------------------
export const CLIENT_CERT_VALIDITY_DAYS = 365;
export const DEFAULT_CERT_DIR = '/vsodb/certs';
export const DEFAULT_SSL_DIR = '/vsodb';
export const CERT_SUBJECT_ORG = 'g8e Operator';
export const CERT_SUBJECT_COUNTRY = 'US';
export const CRL_ISSUER = 'g8e Operator CA';

// ---------------------------------------------------------------------------
// Attachment Limits
// ---------------------------------------------------------------------------
export const MAX_ATTACHMENT_SIZE = 10 * 1024 * 1024;        // 10MB per file
export const MAX_TOTAL_ATTACHMENT_SIZE = 30 * 1024 * 1024;  // 30MB per investigation
export const MAX_ATTACHMENT_FILES = 3;                       // max files per message
export const ALLOWED_ATTACHMENT_CONTENT_TYPES = [
    'image/jpeg', 'image/png', 'image/gif', 'image/webp',
    'application/pdf', 'text/plain', 'text/csv',
    'application/msword', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    'application/vnd.ms-excel', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    'application/zip', 'application/x-zip-compressed'
];

// ---------------------------------------------------------------------------
// Settings Service
// ---------------------------------------------------------------------------
export const SETTINGS_DOC_ID = _DOCUMENT_IDS.document_ids.platform_settings;
export const USER_SETTINGS_DOC_PREFIX = _DOCUMENT_IDS.document_ids.user_settings_prefix;
export const DEFAULT_LOG_LEVEL = 'info';

// ---------------------------------------------------------------------------
// Cache Type Identifiers
// Used as cacheType labels in cacheMetrics.recordHit/Miss/Error() calls
// ---------------------------------------------------------------------------
export const CacheType = {
    USER: 'user',
    CONFIG: 'config',
    OPERATOR: 'operator',
};

// ---------------------------------------------------------------------------
// Internal User Query Limits
// ---------------------------------------------------------------------------
export const USER_STATS_QUERY_LIMIT = 1000;
export const USER_LIST_DEFAULT_LIMIT = 100;
export const USER_LIST_MAX_LIMIT = 500;
export const USER_LIST_MIN_LIMIT = 1;

// ---------------------------------------------------------------------------
// EventBus
// ---------------------------------------------------------------------------
export const MAX_EVENTBUS_LISTENERS = 5;

// ---------------------------------------------------------------------------
// Docs
// ---------------------------------------------------------------------------
export const DEFAULT_DOCS_DIR = '/docs';

// ---------------------------------------------------------------------------
// g8e node Operator
// ---------------------------------------------------------------------------
export const G8E_GATEWAY_CONTAINER_NAME = 'g8ep';
export const G8E_GATEWAY_OPERATOR_BINARY_PATH = '/home/g8e/g8e.operator';
export const G8E_GATEWAY_OPERATOR_LAUNCH_TIMEOUT_MS = 10000;

// ---------------------------------------------------------------------------
// Version
// ---------------------------------------------------------------------------
export const VERSION_FALLBACK = 'v0.0.0-unknown';
export const VERSION_DEV_SUFFIX = 'dev';

// ---------------------------------------------------------------------------
// Cache TTLs (seconds)
// ---------------------------------------------------------------------------
export const CacheTTL = {
    USER: 3600,
    OPERATOR: 3600,
    HEARTBEAT: 300,
    API_KEY: 86400,
    SETTINGS: 86400,
    ATTACHMENT: 3600,
    CASE: 1800,
    INVESTIGATION: 1800,
    ORGANIZATION: 7200,
    PASSKEY_CHALLENGE: 300,
    QUERY: 300,
    DEFAULT: 3600,
};

// ---------------------------------------------------------------------------
// Cache Warming
// ---------------------------------------------------------------------------
export const CACHE_WARMING_PERIODIC_INTERVAL_HOURS = 12;
export const CACHE_WARMING_PERIODIC_INTERVAL_MS = CACHE_WARMING_PERIODIC_INTERVAL_HOURS * 60 * 60 * 1000;
export const CACHE_WARMING_INVESTIGATION_PRELOAD_LIMIT = 50;

// ---------------------------------------------------------------------------
// Certificate / CRL
// ---------------------------------------------------------------------------
export const CRL_NEXT_UPDATE_SECONDS = 24 * 60 * 60;
export const CRL_NEXT_UPDATE_MS = CRL_NEXT_UPDATE_SECONDS * 1000;

// ---------------------------------------------------------------------------
// SSE Wire Protocol
// ---------------------------------------------------------------------------
export const SSE_FRAME_TERMINATOR = '\n\n';


// ---------------------------------------------------------------------------
// Operator Stale Detection
// ---------------------------------------------------------------------------
export const OperatorStaleThreshold = Object.freeze({
    SECONDS: 60,
});

// ---------------------------------------------------------------------------
// Console Metrics
// ---------------------------------------------------------------------------
export const CONSOLE_METRICS_CACHE_TTL_MS = 30000;
export const CONSOLE_METRICS_WINDOW_1_DAY_SECONDS = 24 * 60 * 60;
export const CONSOLE_METRICS_WINDOW_7_DAYS_SECONDS = 7 * 24 * 60 * 60;
export const CONSOLE_METRICS_WINDOW_30_DAYS_SECONDS = 30 * 24 * 60 * 60;
export const CONSOLE_METRICS_WINDOW_1_DAY_MS = CONSOLE_METRICS_WINDOW_1_DAY_SECONDS * 1000;
export const CONSOLE_METRICS_WINDOW_7_DAYS_MS = CONSOLE_METRICS_WINDOW_7_DAYS_SECONDS * 1000;
export const CONSOLE_METRICS_WINDOW_30_DAYS_MS = CONSOLE_METRICS_WINDOW_30_DAYS_SECONDS * 1000;
