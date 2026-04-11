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
 * Shared Constants Loader
 * Loads canonical wire-protocol values from shared/constants/*.json.
 * These JSON files are the single source of truth shared across g8ee (Python),
 * VSA (Go), and VSOD (JavaScript).
 *
 * Usage: import { _EVENTS, _STATUS, _MSG, _COLLECTIONS, _KV, _INTENTS } from './shared.js';
 * Single source of truth for all canonical wire-protocol values.
 */

import { createRequire } from 'module';
import { fileURLToPath } from 'url';
import path from 'path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);

const sharedDir = path.resolve(__dirname, '../../../shared/constants');

export const _EVENTS      = require(path.join(sharedDir, 'events.json'));
export const _STATUS = require(path.join(sharedDir, 'status.json'));
export const _MSG = require(path.join(sharedDir, 'senders.json'));
export const _COLLECTIONS = require(path.join(sharedDir, 'collections.json'));
export const _KV = require(path.join(sharedDir, 'kv_keys.json'));
export const _CHANNELS = require(path.join(sharedDir, 'channels.json'));
export const _PUBSUB = require(path.join(sharedDir, 'pubsub.json'));
export const _INTENTS = require(path.join(sharedDir, 'intents.json'));
export const _PROMPTS = require(path.join(sharedDir, 'prompts.json'));
export const _TIMESTAMP = require(path.join(sharedDir, 'timestamp.json'));
export const _HEADERS = require(path.join(sharedDir, 'headers.json'));
export const _DOCUMENT_IDS = require(path.join(sharedDir, 'document_ids.json'));
export const _PLATFORM = require(path.join(sharedDir, 'platform.json'));
export const _AGENTS = require(path.join(sharedDir, 'agents.json'));
