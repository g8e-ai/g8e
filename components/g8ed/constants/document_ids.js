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

import { _DOCUMENT_IDS } from './shared.js';

/**
 * Document IDs and Sentinel Values
 * Canonical values loaded from shared/constants/document_ids.json.
 * That file is the single source of truth shared across g8ed and G8EE.
 */

const d = _DOCUMENT_IDS['document_ids'];
const s = _DOCUMENT_IDS['sentinel_id'];

export const DocumentIds = Object.freeze({
    PLATFORM_SETTINGS: d['platform_settings'],
    USER_SETTINGS_PREFIX: d['user_settings_prefix']
});

export const SentinelId = Object.freeze({
    UNKNOWN: s['unknown']
});

export default { DocumentIds, SentinelId };
