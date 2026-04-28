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

import { _STATUS, _HEADERS } from './shared.js';
import { _DOCUMENT_IDS } from './shared.js';

/**
 * Auth Constants
 * All identity, role, API key, and device link constants for the auth domain.
 * Wire-protocol values are sourced from shared/constants/status.json.
 */

// ---------------------------------------------------------------------------
// Device Link Defaults
// ---------------------------------------------------------------------------
export const DEFAULT_DEVICE_LINK_MAX_USES = 1;
export const DEVICE_LINK_MAX_USES_MIN = 1;
export const DEVICE_LINK_MAX_USES_MAX = 10000;

// ---------------------------------------------------------------------------
// Session TTL Overrides
// ---------------------------------------------------------------------------
export const DEVICE_LINK_TTL_SECONDS = 3600;
export const DEVICE_LINK_TTL_MIN_SECONDS = 60;
export const DEVICE_LINK_TTL_MAX_SECONDS = 604800;
export const SESSION_AUTH_LISTEN_TTL_MS = 60_000;

// ---------------------------------------------------------------------------
// Distributed Lock
// ---------------------------------------------------------------------------
export const LOCK_TTL_MS = 10000;
export const LOCK_RETRY_DELAY_MS = 200;
export const LOCK_MAX_RETRIES = 25;

// ---------------------------------------------------------------------------
// Request Timestamp Validation
// ---------------------------------------------------------------------------
export const TIMESTAMP_WINDOW_SECONDS = 5 * 60;
export const TIMESTAMP_WINDOW_MS = TIMESTAMP_WINDOW_SECONDS * 1000;
export const NONCE_TTL_SECONDS = 10 * 60;
export const NONCE_CACHE_CLEANUP_INTERVAL_MS = 60 * 1000;

// ---------------------------------------------------------------------------
// Intent Permissions
// ---------------------------------------------------------------------------
export const INTENT_TTL_SECONDS = 60 * 60;
export const INTENT_TTL_MS = INTENT_TTL_SECONDS * 1000;

// ---------------------------------------------------------------------------
// HTTP Authorization
// ---------------------------------------------------------------------------
export const BEARER_PREFIX = 'Bearer ';

// ---------------------------------------------------------------------------
// API Key Hashing
// ---------------------------------------------------------------------------
export const API_KEY_HASH_ALGORITHM = 'sha256';
export const API_KEY_HASH_LENGTH = 64;
export const API_KEY_LOG_PREFIX_LENGTH = 20;

// ---------------------------------------------------------------------------
// API Key Format Validation
// Canonical format from shared/constants/api_key_patterns.json
// ---------------------------------------------------------------------------
export const API_KEY_OPERATOR_REGEX = /^g8e_[a-f0-9]{8}_[a-f0-9]{64}$/;
export const API_KEY_REGULAR_REGEX = /^g8e_[a-f0-9]{64}$/;
export const API_KEY_COMBINED_REGEX = /^g8e_[a-f0-9]{8}_[a-f0-9]{64}$|^g8e_[a-f0-9]{64}$/;

// ---------------------------------------------------------------------------
// CRL
// ---------------------------------------------------------------------------
export const CRL_SERIAL_MIN_LENGTH = 16;

/**
 * User Roles
 * RBAC roles assigned to human user accounts.
 * Canonical values from shared/constants/status.json user.role.
 */
export const UserRole = Object.freeze({
    USER:       _STATUS['user.role']['user'],
    ADMIN:      _STATUS['user.role']['admin'],
    SUPERADMIN: _STATUS['user.role']['superadmin'],
});

/**
 * Operator Session Role
 * Identity discriminator stamped into operator session user_data.roles.
 * Not an RBAC role — used only to distinguish operator sessions from human web sessions.
 * Canonical value from shared/constants/status.json user.role.
 */
export const OperatorSessionRole = Object.freeze({
    OPERATOR: _STATUS['user.role']['operator'],
});

/**
 * Authentication Providers
 * Identifies which identity provider authenticated the user.
 * Canonical values from shared/constants/status.json auth.provider.
 */
export const AuthProvider = Object.freeze({
    LOCAL:   _STATUS['auth.provider']['local'],
    PASSKEY: _STATUS['auth.provider']['passkey'],
});

/**
 * API Key Client Names
 * Identifies the purpose/owner type of an API key.
 * Canonical values from shared/constants/status.json user.role.
 */
export const ApiKeyClientName = Object.freeze({
    OPERATOR: _STATUS['user.role']['operator'],
    USER:     _STATUS['user.role']['user'],
});

/**
 * API Key Status
 * Lifecycle states of an API key.
 * Canonical values from shared/constants/status.json api.key.status.
 */
export const ApiKeyStatus = Object.freeze({
    ACTIVE:    _STATUS['api.key.status']['active'],
    REVOKED:   _STATUS['api.key.status']['revoked'],
    EXPIRED:   _STATUS['api.key.status']['expired'],
    SUSPENDED: _STATUS['api.key.status']['suspended'],
});

/**
 * Authentication Mode identifiers
 * Sent by g8eo in the auth_mode field of POST /api/auth/g8e.
 * Canonical values from shared/constants/status.json auth.mode.
 */
export const AuthMode = Object.freeze({
    API_KEY:          _STATUS['auth.mode']['api_key'],
    OPERATOR_SESSION: _STATUS['auth.mode']['operator_session'],
});

/**
 * Authentication Method identifiers
 * Records how authentication was performed in audit log entries.
 * Canonical values from shared/constants/status.json auth.method.
 */
export const AuthMethod = Object.freeze({
    KV_PUBSUB: _STATUS['auth.method']['kv.pubsub'],
    SESSION:   _STATUS['auth.method']['session'],
});

/**
 * Login Audit Event Types
 * event_type values written to the login audit trail.
 * Canonical values from shared/constants/status.json login.audit.event.type.
 */
export const LoginEventType = Object.freeze({
    LOGIN_SUCCESS:    _STATUS['login.audit.event.type']['login.success'],
    LOGIN_FAILED:     _STATUS['login.audit.event.type']['login.failed'],
    LOGIN_ANOMALY:    _STATUS['login.audit.event.type']['login.anomaly'],
    ACCOUNT_LOCKED:   _STATUS['login.audit.event.type']['account.locked'],
    ACCOUNT_UNLOCKED: _STATUS['login.audit.event.type']['account.unlocked'],
});

/**
 * Auth Audit Event Types
 * event_type values written to the operator auth audit trail.
 * Canonical values from shared/constants/status.json auth.audit.event.type.
 */
export const AuthEventType = Object.freeze({
    AUTH_SUCCESS: _STATUS['auth.audit.event.type']['auth.success'],
    AUTH_FAILED:  _STATUS['auth.audit.event.type']['auth.failed'],
});

/**
 * Download Audit Event Types
 * event_type values written to the login audit trail for operator binary download events.
 * Canonical values from shared/constants/status.json download.audit.event.type.
 */
export const DownloadEventType = Object.freeze({
    DOWNLOAD_TOKEN_FAILED:  _STATUS['download.audit.event.type']['download.token.failed'],
    DOWNLOAD_TOKEN_SUCCESS: _STATUS['download.audit.event.type']['download.token.success'],
});

/**
 * Auth Audit Results
 * result field values written to the operator auth audit trail.
 * Canonical values from shared/constants/status.json auth.audit.result.
 */
export const AuthAuditResult = Object.freeze({
    SUCCESS:         _STATUS['auth.audit.result']['success'],
    FAILURE:         _STATUS['auth.audit.result']['failure'],
    INVALID_API_KEY: _STATUS['auth.audit.result']['invalid.api.key'],
});

/**
 * Device Link Status
 * Lifecycle states of a device link token.
 * Canonical values from shared/constants/status.json device.link.status.
 */
export const DeviceLinkStatus = Object.freeze({
    ACTIVE:    _STATUS['device.link.status']['active'],
    PENDING:   _STATUS['device.link.status']['pending'],
    USED:      _STATUS['device.link.status']['used'],
    EXHAUSTED: _STATUS['device.link.status']['exhausted'],
    EXPIRED:   _STATUS['device.link.status']['expired'],
    REVOKED:   _STATUS['device.link.status']['revoked'],
});

/**
 * Download Key Type Labels
 * Identifies which token type was used for an operator binary download.
 */
export const DownloadKeyType = Object.freeze({
    DOWNLOAD_TOKEN:    'download-token',
    DEVICE_LINK:       'device-link',
    OPERATOR_SPECIFIC: 'operator-specific',
    USER_DOWNLOAD:     'user-download',
});

/**
 * API Key Permission Scopes
 */
export const ApiKeyPermission = Object.freeze({
    OPERATOR_BOOTSTRAP:  'operator:bootstrap',
    OPERATOR_HEARTBEAT:  'operator:heartbeat',
    OPERATOR_DOWNLOAD:   'operator:download',
});

/**
 * Token Format Validation
 */
export const TokenFormat = Object.freeze({
    DEVICE_LINK:    /^dlk_[A-Za-z0-9_-]{32}$/,
    DOWNLOAD_TOKEN: /^dlt_[A-Za-z0-9_-]{32}$/,
});

/**
 * Device Link Success Messages
 * Canonical values from shared/constants/status.json device.link.success.
 */
export const DeviceLinkSuccess = Object.freeze({
    LISTED:   _STATUS['device.link.success']['listed'],
    CREATED:  _STATUS['device.link.success']['created'],
    REVOKED:  _STATUS['device.link.success']['revoked'],
    DELETED:  _STATUS['device.link.success']['deleted'],
});

/**
 * General Auth Error Messages
 * Used by authentication and authorization middleware for session/ownership checks.
 */
export const AuthError = Object.freeze({
    REQUIRED:                    'Authentication required',
    INVALID_SESSION_TYPE:        'Invalid session type',
    OPERATOR_ID_REQUIRED:        'operator_id is required',
    FORBIDDEN_RESOURCE:          'Forbidden - Cannot access other users\' resources',
    FORBIDDEN_OPERATOR:          'Forbidden - Operator not found or not owned by you',
    FORBIDDEN_SESSION:           'Forbidden - WebSession not found or not owned by you',
    FORBIDDEN_INTERNAL:          'Forbidden - Internal endpoint requires authentication',
    SESSION_ID_IN_QUERY_PARAM:   'Web session ID must not be sent in URL parameters',
    WEB_SESSION_ID_IN_QUERY:     'WebSession ID must not be sent in URL query parameters',
    WEB_SESSION_ID_REQUIRED:     'web_session_id is required in path or request body',
    AUTHORIZATION_CHECK_FAILED:  'Authorization check failed',
    SUPERADMIN_REQUIRED:         'Superadmin access required',
    ACCESS_DENIED:               'Access Denied',
    INVALID_OR_EXPIRED_SESSION:  'Invalid or expired session',
    NO_OPERATOR_BOUND:           'No active Operator session found',
});

/**
 * API Key Auth Error Messages
 */
export const ApiKeyError = Object.freeze({
    REQUIRED:              'API key required',
    INVALID_FORMAT:        'Invalid authorization format',
    INVALID:               'Invalid API key',
    INVALID_KEY_FORMAT:    'Invalid API key format',
    INVALID_OR_EXPIRED:    'Invalid or expired API key',
    USER_NOT_FOUND:        'User not found',
    MISSING_USER_ID:       'API key not associated with a user',
    DOWNLOAD_ONLY:         'Download-only API key',
    DOWNLOAD_ONLY_CODE:    'DOWNLOAD_KEY_NOT_ALLOWED',
    AUTH_FAILED:           'Authentication failed',
    NO_DOWNLOAD_PERMISSION: 'API key does not have download permission. Use a download API key or an operator-specific API key.',
    INTERNAL_ERROR:        'Internal server error',
});

/**
 * Operator Auth Error Messages
 * Used by operator_auth_routes.js for API key and session authentication errors.
 */
export const OperatorAuthError = Object.freeze({
    MISSING_API_KEY:              'Missing api_key',
    FINGERPRINT_REQUIRED:         'System fingerprint required',
    OPERATOR_ALREADY_ACTIVE:      'Operator already active',
    OPERATOR_ALREADY_ACTIVE_MSG:  'already running',
    MISSING_OPERATOR_SESSION_ID:  'Missing operator_session_id',
    REFRESH_FAILED:               'WebSession refresh failed',
    OPERATOR_WRONG_ACCOUNT:       'does not belong to your account',
});

/**
 * Device Link Error Messages
 */
export const DeviceLinkError = Object.freeze({
    OPERATOR_NOT_FOUND:          'Operator not found',
    OPERATOR_WRONG_USER:         'Operator does not belong to this user',
    OPERATOR_TERMINATED:         'Cannot link to terminated operator',
    USER_NOT_FOUND:              'User not found',
    MAX_USES_INVALID:            'max_uses must be between 1 and 10,000',
    TTL_INVALID:                 'Expiry must be between 1 minute and 7 days',
    NO_AVAILABLE_SLOTS:          'No available Operator slots',
    SLOT_CREATE_FAILED:          'Failed to create Operator slot on demand',
    INVALID_TOKEN_FORMAT:        'Invalid token format',
    LINK_NOT_FOUND:              'Link not found or expired',
    LINK_EXPIRED:                'Link expired',
    LINK_REVOKED:                'Device link is revoked',
    LINK_ALREADY_USED:           'Device link already used',
    LINK_EXHAUSTED:              'Device link exhausted (max uses reached)',
    DEVICE_ALREADY_REGISTERED:   'This device already registered with this device link',
    REGISTRATION_BUSY:           'Registration busy, please try again',
    MISSING_FINGERPRINT:         'Missing system_fingerprint',
    INVALID_FINGERPRINT:         'Invalid system_fingerprint',
    CANNOT_DELETE_ACTIVE:        'Cannot delete an active token. Revoke it first.',
    UNAUTHORIZED:                'Unauthorized',
    CLAIM_SLOT_FAILED:           'Failed to claim Operator slot',
    UPDATE_OPERATOR_FAILED:      'Failed to update operator',
});

/**
 * Internal component-to-component auth header.
 * All internal endpoints require this token.
 */
export const INTERNAL_AUTH_HEADER = _HEADERS['http.x-internal-auth'].toLowerCase();

/**
 * Web session ID header forwarded on internal requests.
 */
export const WEB_SESSION_ID_HEADER = _HEADERS['http.x-session-id'].toLowerCase();

