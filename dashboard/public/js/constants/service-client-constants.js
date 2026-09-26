// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export const BEARER_PREFIX           = 'Bearer ';
export const CONTENT_TYPE_JSON       = 'application/json';
export const RATE_LIMIT_RESET_HEADER = 'RateLimit-Reset';

export const ServiceName = Object.freeze({
    GATEWAY: 'gateway',
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
    PATCH:  'PATCH',
    DELETE: 'DELETE',
});

export const ServiceClientEvent = Object.freeze({
    RATE_LIMITED: 'service-client:rate-limited',
});

export const MAX_ATTACHMENT_SIZE = 10 * 1024 * 1024;
export const MAX_ATTACHMENT_FILES = 3;
export const ALLOWED_ATTACHMENT_CONTENT_TYPES = [
    'image/jpeg',
    'image/png',
    'image/gif',
    'image/webp',
    'application/pdf',
    'text/plain',
    'text/csv',
];
