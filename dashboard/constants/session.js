// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { _STATUS } from './shared.js';

/**
 * Session Constants
 * All session type, key prefix, lifecycle event, and end-reason constants.
 * Wire-protocol values are sourced from protocol/constants/status.json.
 */

// ---------------------------------------------------------------------------
// WebSession TTLs
// ---------------------------------------------------------------------------
export const SESSION_TTL_SECONDS = 28800;
export const SESSION_REFRESH_THRESHOLD_SECONDS = 3600;
export const ABSOLUTE_SESSION_TIMEOUT_SECONDS = 86400;

// ---------------------------------------------------------------------------
// WebSession Cookie
// ---------------------------------------------------------------------------
export const SESSION_COOKIE_NAME = 'web_session_id';
export const COOKIE_SAME_SITE = 'lax';
export const SESSION_ID_LOG_PREFIX_LENGTH = 15;

/**
 * WebSession Types
 * Identifies the type of session for g8edB KV key generation and session management.
 * Canonical values from protocol/constants/status.json session.type.
 *
 * KV keys built via: KVKey.webSessionKey(id) / KVKey.operatorSessionKey(id)
 */
export const SessionType = Object.freeze({
    WEB:      _STATUS['session.type']['web'],
    OPERATOR: _STATUS['session.type']['operator'],
});

/**
 * KV Key Prefixes for WebSession Types
 * Maps SessionType values to their KV key prefixes.
 * Canonical values from protocol/constants/status.json session.key.prefix.
 * These are the ONLY valid session key prefixes — no generic 'session:' allowed.
 */
export const SessionKeyPrefix = Object.freeze({
    [_STATUS['session.type']['web']]:      _STATUS['session.key.prefix']['web'],
    [_STATUS['session.type']['operator']]: _STATUS['session.key.prefix']['operator'],
});

/**
 * WebSession End Reasons
 * Canonical reason strings passed to endSession() and written to the session audit trail.
 * Canonical values from protocol/constants/status.json session.end.reason.
 */
export const SessionEndReason = Object.freeze({
    USER_LOGOUT:          _STATUS['session.end.reason']['user.logout'],
    INTEGRITY_FAILURE:    _STATUS['session.end.reason']['integrity.failure'],
    SESSION_REGENERATION: _STATUS['session.end.reason']['session.regeneration'],
    INVALIDATE_ALL:       _STATUS['session.end.reason']['invalidate.all'],
    USER_DELETED:         _STATUS['session.end.reason']['user.deleted'],
});

/**
 * WebSession Suspicious Activity Reasons
 * Reason strings recorded in the session audit trail for suspicious activity events.
 * Canonical values from protocol/constants/status.json session.suspicious.reason.
 */
export const SessionSuspiciousReason = Object.freeze({
    EXCESSIVE_IP_CHANGES: _STATUS['session.suspicious.reason']['excessive.ip.changes'],
});

/**
 * WebSession Event Types
 * event_type values written to the session audit trail.
 * Canonical values from protocol/constants/status.json session.event.type.
 */
export const SessionEventType = Object.freeze({
    SESSION_CREATED:             _STATUS['session.event.type']['session.created'],
    SESSION_ENDED:               _STATUS['session.event.type']['session.ended'],
    SESSION_REGENERATED:         _STATUS['session.event.type']['session.regenerated'],
    SESSION_TIMEOUT_ABSOLUTE:    _STATUS['session.event.type']['session.timeout.absolute'],
    SESSION_TIMEOUT_IDLE:        _STATUS['session.event.type']['session.timeout.idle'],
    SESSION_SUSPICIOUS_ACTIVITY: _STATUS['session.event.type']['session.suspicious.activity'],
    OPERATOR_BOUND:              _STATUS['session.event.type']['operator.bound'],
    OPERATOR_UNBOUND:            _STATUS['session.event.type']['operator.unbound'],
});
