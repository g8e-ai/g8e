// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export const BEARER_PREFIX           = 'Bearer ';
export const CONTENT_TYPE_JSON       = 'application/json';
export const RATE_LIMIT_RESET_HEADER = 'RateLimit-Reset';

// Legacy auth headers retained only for out-of-scope operator features that
// still use API-key auth against the old g8ed surface. Gateway-direct auth
// uses the HttpOnly g8e_web_session_cookie (sent automatically via
// credentials: 'include'); no auth headers are injected for ServiceName.GATEWAY.
export const WEB_SESSION_ID_HEADER   = 'x-session-id';
export const AUTHORIZATION_HEADER    = 'Authorization';
export const COOKIE_HEADER           = 'Cookie';
export const WEB_SESSION_COOKIE_KEY  = 'web_session_id';
export const API_KEY_HEADER          = 'X-API-Key';

export const ServiceName = Object.freeze({
    VSE:     'vse',
    g8ed:    'g8ed',
    GATEWAY: 'gateway',
});

export const ServiceUrl = Object.freeze({
    VSE: 'https://vse',
});

export const RequestTimeout = Object.freeze({
    AUTH_MS:    30000,
    CASE_MS:    300000,
    CHAT_MS:    300000,
    DEFAULT_MS: 300000,
});

export const RetryConfig = Object.freeze({
    MAX_RETRIES:        3,
    RETRY_DELAY_MS:     1000,
    BACKOFF_MULTIPLIER: 2,
});

export const RequestPath = Object.freeze({
    AUTH_PREFIX:  '/auth/',
    CASES_PREFIX: '/cases',
    CHAT_PREFIX:  '/api/chat/',
});

export const MAX_EVENTBUS_LISTENERS = 5;

export const RATE_LIMIT_FALLBACK_MESSAGE = 'Too many requests. Please try again later.';

export const HttpStatus = Object.freeze({
    UNAUTHORIZED:   401,
    FORBIDDEN:      403,
    NOT_FOUND:      404,
    INTERNAL_ERROR: 500,
});

export const HTTP_STATUS_PATTERN = /^HTTP (\d+)/;

export const HttpMethod = Object.freeze({
    GET:    'GET',
    POST:   'POST',
    PUT:    'PUT',
    DELETE: 'DELETE',
});

export const ServiceClientEvent = Object.freeze({
    READY: 'serviceClientReady',
});

export const MAX_ATTACHMENT_SIZE              = 10 * 1024 * 1024;
export const MAX_TOTAL_ATTACHMENT_SIZE        = 30 * 1024 * 1024;
export const MAX_ATTACHMENT_FILES             = 3;
export const ALLOWED_ATTACHMENT_CONTENT_TYPES = [
    'image/jpeg', 'image/png', 'image/gif', 'image/webp',
    'application/pdf', 'text/plain', 'text/csv',
    'application/msword', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    'application/vnd.ms-excel', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    'application/zip', 'application/x-zip-compressed',
];
