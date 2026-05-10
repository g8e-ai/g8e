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
 * Operator Default Configuration Constants
 * Single source of truth for default values sent to operators on authentication
 */

import { _INTENTS } from './shared.js';

/**
 * Default runtime configuration returned to the g8eo operator on successful authentication.
 * Returned by the pub/sub auth flow (AuthService).
 */
export const DEFAULT_OPERATOR_CONFIG = {
    command_timeout: '15m',
    max_concurrent_tasks: 25,
    max_memory_mb: 2048,
    heartbeat_interval_seconds: 30
};

/**
 * Required prefix for all g8e API keys.
 * Used for format validation before hitting the database.
 */
export const API_KEY_PREFIX = 'g8e_';

/**
 * Valid intent permission names for cloud operators.
 * Canonical values loaded from shared/constants/intents.json.
 * Any intent not in this list will be rejected by grantIntent().
 */
export const VALID_CLOUD_INTENTS = Object.keys(_INTENTS.intents);
