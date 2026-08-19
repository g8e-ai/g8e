// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Operator Default Configuration Constants
 * Single source of truth for default values sent to operators on authentication
 */

import { _INTENTS } from './shared.js';

/**
 * Default runtime configuration returned to the VSA operator on successful authentication.
 * Returned by the pub/sub auth flow (AuthService).
 */
export const DEFAULT_OPERATOR_CONFIG = {
    command_timeout: '15m',
    max_concurrent_tasks: 25,
    max_memory_mb: 2048,
    heartbeat_interval_seconds: 30
};

/**
 * Required prefix for all DropOps API keys.
 * Used for format validation before hitting the database.
 */
export const API_KEY_PREFIX = 'dropops_';

/**
 * Valid intent permission names for cloud operators.
 * Canonical values loaded from protocol/constants/intents.json.
 * Any intent not in this list will be rejected by grantIntent().
 */
export const VALID_CLOUD_INTENTS = Object.keys(_INTENTS.intents);
