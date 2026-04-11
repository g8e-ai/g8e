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
 * Rate Limit Constants
 * Window sizes, request caps, and error messages for every rate-limited
 * endpoint in g8ed. Keep all rate limit configuration here so thresholds
 * can be reviewed and adjusted in one place.
 */

// ---------------------------------------------------------------------------
// Global Public API
// ---------------------------------------------------------------------------
export const GlobalPublicRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 100,
});

// ---------------------------------------------------------------------------
// User Authentication (login, register)
// ---------------------------------------------------------------------------
export const UserAuthRateLimit = Object.freeze({
    WINDOW_MS: 5 * 60 * 1000,
    MAX: 20,
});

// ---------------------------------------------------------------------------
// Chat / Message Endpoints
// ---------------------------------------------------------------------------
export const ChatRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 30,
});

// ---------------------------------------------------------------------------
// SSE Connection Attempts
// ---------------------------------------------------------------------------
export const SSERateLimit = Object.freeze({
    WINDOW_MS: 5 * 60 * 1000,
    MAX: 30,
});

// ---------------------------------------------------------------------------
// General API Endpoints
// ---------------------------------------------------------------------------
export const ApiRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 60,
});

// ---------------------------------------------------------------------------
// File Uploads
// ---------------------------------------------------------------------------
export const UploadRateLimit = Object.freeze({
    WINDOW_MS: 15 * 60 * 1000,
    MAX: 20,
});

// ---------------------------------------------------------------------------
// Operator WebSession Refresh
// ---------------------------------------------------------------------------
export const OperatorRefreshRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 10,
});

// ---------------------------------------------------------------------------
// Operator Auth (POST /api/auth/operator)
// Keyed per API key, not IP — see middleware/rate-limit.js for rationale
// ---------------------------------------------------------------------------
export const AuthRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX_PER_KEY: 5,
    MAX_PER_IP: 1000,
});

// ---------------------------------------------------------------------------
// Operator API (authenticated operator calls)
// ---------------------------------------------------------------------------
export const OperatorApiRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 120,
});

// ---------------------------------------------------------------------------
// Audit Log Endpoints
// ---------------------------------------------------------------------------
export const AuditRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 10,
});

// ---------------------------------------------------------------------------
// Console Endpoints
// ---------------------------------------------------------------------------
export const ConsoleRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 30,
});

// ---------------------------------------------------------------------------
// Device Link (public register endpoint)
// ---------------------------------------------------------------------------
export const DeviceLinkRateLimit = Object.freeze({
    WINDOW_MS: 5 * 60 * 1000,
    MAX: 30,
});

// ---------------------------------------------------------------------------
// Device Link Generation (authenticated)
// ---------------------------------------------------------------------------
export const DeviceLinkGenerateRateLimit = Object.freeze({
    WINDOW_MS: 15 * 60 * 1000,
    MAX: 10,
});

// ---------------------------------------------------------------------------
// Device Link List (authenticated)
// ---------------------------------------------------------------------------
export const DeviceLinkListRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 20,
});

// ---------------------------------------------------------------------------
// Device Link Revoke (authenticated)
// ---------------------------------------------------------------------------
export const DeviceLinkRevokeRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 10,
});

// ---------------------------------------------------------------------------
// Settings API (admin-only)
// ---------------------------------------------------------------------------
export const SettingsRateLimit = Object.freeze({
    WINDOW_MS: 60 * 1000,
    MAX: 20,
});

// ---------------------------------------------------------------------------
// Passkey Auth (challenge + verify endpoints)
// ---------------------------------------------------------------------------
export const PasskeyRateLimit = Object.freeze({
    WINDOW_MS: 5 * 60 * 1000,
    MAX: 20,
});

// ---------------------------------------------------------------------------
// Login Security Thresholds
// ---------------------------------------------------------------------------
export const LoginSecurity = Object.freeze({
    MAX_FAILED_ATTEMPTS: 3,
    PROGRESSIVE_DELAYS: [0, 1000, 2000],
    FAILED_ATTEMPT_WINDOW_SECONDS: 15 * 60,
    CAPTCHA_THRESHOLD: 2,
    ANOMALY_MULTI_ACCOUNT_THRESHOLD: 3,
    ANOMALY_MULTI_ACCOUNT_WINDOW_SECONDS: 5 * 60,
});

// ---------------------------------------------------------------------------
// Rate Limit Error Messages
// ---------------------------------------------------------------------------
export const RateLimitError = Object.freeze({
    GENERIC:                 'Too many requests. Please try again later.',
    GENERIC_WAIT:            'Too many requests. Please wait before trying again.',
    RATE_EXCEEDED:           'Rate limit exceeded. Please try again later.',
    RATE_EXCEEDED_WAIT:      'Rate limit exceeded. Please wait before retrying.',
    AUTH:                    'Too many authentication attempts. Please try again later.',
    AUTH_OPERATOR:           'Too many authentication attempts for this g8e.',
    AUTH_IP:                 'Too many authentication attempts from this IP.',
    CHAT_SLOW:               'Too many messages. Please slow down.',
    CHAT_WAIT:               'Too many messages. Please wait a moment before sending more.',
    SSE_ATTEMPTS:            'Too many connection attempts.',
    SSE_ATTEMPTS_WAIT:       'Too many connection attempts. Please try again later.',
    UPLOAD:                  'Too many file uploads. Please try again later.',
    UPLOAD_WAIT:             'Too many file uploads. Please wait before uploading more files.',
    REFRESH:                 'Too many refresh attempts.',
    REFRESH_WAIT:            'Too many refresh attempts. Please try again later.',
    AUDIT_SLOW:              'Too many audit log requests. Please slow down.',
    AUDIT_WAIT:              'Too many audit log requests. Please wait before making more requests.',
    CONSOLE_SLOW:            'Too many console requests. Please slow down.',
    CONSOLE_WAIT:            'Too many console requests. Please wait before making more requests.',
    DEVICE_LINK:             'Too many requests. Please try again in a few minutes.',
    DEVICE_LINK_CREATE:      'Too many device links created. Please try again later.',
    DEVICE_LINK_CREATE_WAIT: 'Too many device links created. Please wait before creating more.',
    DEVICE_LINK_REGISTER:    'Registration failed',
    SETTINGS_SLOW:           'Too many settings requests. Please slow down.',
    SETTINGS_WAIT:           'Too many settings requests. Please wait before trying again.',
});
