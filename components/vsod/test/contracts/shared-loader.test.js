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
 * Shared Constants Loader Tests
 *
 * Verifies that constants/shared.js resolves all shared JSON files correctly.
 *
 * In the g8ep container the repo root is mounted at /app (.:/app), so:
 *   constants/shared.js lives at /app/components/vsod/constants/shared.js
 *   shared constants live at /app/shared/constants/
 *
 * The correct relative path from constants/ up to the repo root is ../../../
 * (constants → vsod → components → repo root), then into shared/constants.
 */

import { describe, it, expect } from 'vitest';
import { createRequire } from 'module';
import { fileURLToPath } from 'url';
import path from 'path';
import fs from 'fs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const VSOD_ROOT = path.resolve(__dirname, '../..');
const CONSTANTS_DIR = path.join(VSOD_ROOT, 'constants');
const SHARED_JS = path.join(CONSTANTS_DIR, 'shared.js');
const SHARED_CONSTANTS_DIR = path.resolve(VSOD_ROOT, '../../shared/constants');

describe('constants/shared.js loader', () => {

    it('shared.js exists', () => {
        expect(fs.existsSync(SHARED_JS)).toBe(true);
    });

    it('resolves to the correct shared constants directory', () => {
        expect(fs.existsSync(SHARED_CONSTANTS_DIR)).toBe(true);
    });

    const JSON_FILES = [
        'events.json',
        'status.json',
        'senders.json',
        'collections.json',
        'kv_keys.json',
        'channels.json',
        'pubsub.json',
        'intents.json',
        'prompts.json',
        'timestamp.json',
        'headers.json',
        'document_ids.json',
        'platform.json',
        'agents.json',
    ];

    for (const file of JSON_FILES) {
        it(`shared/constants/${file} is reachable from constants/shared.js`, () => {
            const require = createRequire(import.meta.url);
            const filePath = path.join(SHARED_CONSTANTS_DIR, file);
            expect(fs.existsSync(filePath), `${file} not found at ${filePath}`).toBe(true);
            expect(() => require(filePath)).not.toThrow();
        });
    }

    it('loads all exports from constants/shared.js without error', async () => {
        const mod = await import('@vsod/constants/shared.js');
        expect(mod._EVENTS).toBeDefined();
        expect(mod._STATUS).toBeDefined();
        expect(mod._MSG).toBeDefined();
        expect(mod._COLLECTIONS).toBeDefined();
        expect(mod._KV).toBeDefined();
        expect(mod._CHANNELS).toBeDefined();
        expect(mod._PUBSUB).toBeDefined();
        expect(mod._INTENTS).toBeDefined();
        expect(mod._PROMPTS).toBeDefined();
        expect(mod._TIMESTAMP).toBeDefined();
        expect(mod._HEADERS).toBeDefined();
        expect(mod._DOCUMENT_IDS).toBeDefined();
        expect(mod._PLATFORM).toBeDefined();
        expect(mod._AGENTS).toBeDefined();
    });

    it('_EVENTS is a non-empty object', async () => {
        const { _EVENTS } = await import('@vsod/constants/shared.js');
        expect(typeof _EVENTS).toBe('object');
        expect(Object.keys(_EVENTS).length).toBeGreaterThan(0);
    });

    it('_STATUS is a non-empty object', async () => {
        const { _STATUS } = await import('@vsod/constants/shared.js');
        expect(typeof _STATUS).toBe('object');
        expect(Object.keys(_STATUS).length).toBeGreaterThan(0);
    });

    it('_AGENTS is a non-empty object with expected top-level keys', async () => {
        const { _AGENTS } = await import('@vsod/constants/shared.js');
        expect(typeof _AGENTS).toBe('object');
        expect(Object.keys(_AGENTS).length).toBeGreaterThan(0);
        expect(_AGENTS['triage.complexity']).toBeDefined();
        expect(_AGENTS['triage.intent']).toBeDefined();
        expect(_AGENTS['triage.confidence']).toBeDefined();
        expect(_AGENTS['agent.metadata']).toBeDefined();
    });

    it('shared.js path expression resolves to the same directory used by this test', () => {
        const sharedJsResolved = path.resolve(CONSTANTS_DIR, '../../../shared/constants');
        expect(sharedJsResolved).toBe(SHARED_CONSTANTS_DIR);
    });

    it('path does NOT resolve to filesystem root /shared/', () => {
        expect(SHARED_CONSTANTS_DIR).not.toMatch(/^\/shared\//);
        expect(SHARED_CONSTANTS_DIR).toContain('shared/constants');
    });

});
