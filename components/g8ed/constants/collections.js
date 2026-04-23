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

import { _COLLECTIONS } from './shared.js';

/**
 * DB Collection Names
 * Canonical values loaded from shared/constants/collections.json.
 * That file is the single source of truth shared across g8ed and G8EE.
 *
 * IMPORTANT: When renaming or adding/removing collections, update:
 * 1. shared/constants/collections.json (canonical)
 * 2. components/g8ed/constants/collections.js (this file — extend the Collections object)
 * 3. components/g8ee/app/constants/collections.py
 * 4. docs/architecture/storage.md (Collections table)
 *
 * The drift guard at
 * components/g8ed/test/unit/constants/docs-drift.unit.test.js
 * enforces that (1), (2), and (4) agree.
 */

const c = _COLLECTIONS['collections'];

export const Collections = Object.freeze({
    USERS:                  c['users'],
    WEB_SESSIONS:           c['web_sessions'],
    OPERATOR_SESSIONS:      c['operator_sessions'],
    CLI_SESSIONS:           c['cli_sessions'],
    LOGIN_AUDIT:            c['login_audit'],
    AUTH_ADMIN_AUDIT:       c['auth_admin_audit'],
    ACCOUNT_LOCKS:          c['account_locks'],
    API_KEYS:               c['api_keys'],
    ORGANIZATIONS:          c['organizations'],
    OPERATORS:              c['operators'],
    OPERATOR_USAGE:         c['operator_usage'],
    CASES:                  c['cases'],
    INVESTIGATIONS:         c['investigations'],
    TASKS:                  c['tasks'],
    MEMORIES:               c['memories'],
    SETTINGS:               c['settings'],
    CONSOLE_AUDIT:          c['console_audit'],
    BOUND_SESSIONS:         c['bound_sessions'],
    PASSKEY_CHALLENGES:     c['passkey_challenges'],
    TRIBUNAL_COMMANDS:      c['tribunal_commands'],
    AGENT_ACTIVITY_METADATA: c['agent_activity_metadata'],
});

export default Collections;
