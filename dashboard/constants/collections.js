// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { _COLLECTIONS } from './shared.js';

/**
 * DB Collection Names
 * Canonical values loaded from protocol/constants/collections.json.
 * That file is the single source of truth shared across g8ed and VSE.
 *
 * IMPORTANT: When renaming collections, update:
 * 1. protocol/constants/collections.json
 * 2. db/schema.sql
 */

const c = _COLLECTIONS['collections'];

export const Collections = Object.freeze({
    USERS:                c['users'],
    WEB_SESSIONS:         c['web.sessions'],
    OPERATOR_SESSIONS:    c['operator.sessions'],
    SESSION_AUDIT_LOGS:   c['session.audit.logs'],
    LOGIN_AUDIT:          c['login.audit'],
    AUTH_ADMIN_AUDIT:     c['auth.admin.audit'],
    ACCOUNT_LOCKS:        c['account.locks'],
    API_KEYS:             c['api.keys'],
    ORGANIZATIONS:        c['organizations'],
    OPERATORS:            c['operators'],
    OPERATOR_USAGE:       c['operator.usage'],
    CASES:                c['cases'],
    INVESTIGATIONS:       c['investigations'],
    TASKS:                c['tasks'],
    MEMORIES:             c['memories'],
    SETTINGS:             c['settings'],
    CONSOLE_AUDIT:        c['console.audit'],
    BOUND_SESSIONS:       c['bound.sessions'],
    PASSKEY_CHALLENGES:   c['passkey.challenges'],
});

export default Collections;
