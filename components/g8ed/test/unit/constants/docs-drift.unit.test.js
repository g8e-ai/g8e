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
 * Docs-drift regression guard.
 *
 * Catches the class of drift that triggered the Apr 2026 storage audit, where
 * docs/architecture/storage.md listed collections (`pending_commands`,
 * `external-services`, `orgs`) that no longer existed in code, and was missing
 * collections that had been added (`console_audit`, `passkey_challenges`).
 *
 * Three sources of truth must agree:
 *   1. shared/constants/collections.json          (canonical)
 *   2. components/g8ed/constants/collections.js   (JS bindings)
 *   3. docs/architecture/storage.md Collections   (human-readable reference)
 *
 * If this test fails, update whichever of the three is stale. The canonical
 * source is (1); docs and the JS binding follow from it.
 */

import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { join } from 'path';

import { Collections } from '@g8ed/constants/collections.js';

const REPO_ROOT = join(process.cwd(), '..', '..');

const COLLECTIONS_JSON_PATH = join(REPO_ROOT, 'shared', 'constants', 'collections.json');
const STORAGE_MD_PATH       = join(REPO_ROOT, 'docs', 'architecture', 'storage.md');

function loadCanonicalCollections() {
    const raw = JSON.parse(readFileSync(COLLECTIONS_JSON_PATH, 'utf-8'));
    return new Set(Object.values(raw.collections));
}

function loadJsBindingCollections() {
    return new Set(Object.values(Collections));
}

/**
 * Extract collection names from the Collections table in storage.md.
 *
 * The table is a standard GitHub-flavored markdown table:
 *
 *     ### Collections
 *     | Collection | Primary Writer | Contents |
 *     |---|---|---|
 *     | `users` | ... | ... |
 *     | `web_sessions` | ... | ... |
 *
 * We locate the `### Collections` heading, find the next table, and extract
 * the first column of each body row, stripping backticks.
 */
function loadDocsCollections() {
    const md = readFileSync(STORAGE_MD_PATH, 'utf-8');
    const lines = md.split('\n');

    const headingIdx = lines.findIndex(l => l.trim() === '### Collections');
    if (headingIdx === -1) {
        throw new Error('docs/architecture/storage.md is missing the "### Collections" heading');
    }

    const collections = new Set();
    let inTable = false;
    for (let i = headingIdx + 1; i < lines.length; i++) {
        const line = lines[i];

        // Next top-level heading terminates the table scan.
        if (/^#{1,3}\s/.test(line)) break;

        // Table header / separator
        if (/^\|\s*Collection\s*\|/.test(line)) { inTable = true; continue; }
        if (/^\|[\s-]+\|/.test(line))           { continue; }

        if (!inTable) continue;
        // Blank line terminates the table.
        if (line.trim() === '') break;

        // Row: |  `name`  | ... | ... |
        const m = line.match(/^\|\s*`([a-z0-9_]+)`\s*\|/i);
        if (m) collections.add(m[1]);
    }

    if (collections.size === 0) {
        throw new Error('Could not parse any collection rows from the Collections table in storage.md');
    }
    return collections;
}

function diff(a, b) {
    return [...a].filter(x => !b.has(x));
}

describe('docs drift — storage.md ↔ shared/constants ↔ g8ed bindings [UNIT]', () => {
    const canonical = loadCanonicalCollections();
    const jsBinding = loadJsBindingCollections();
    const docs      = loadDocsCollections();

    it('shared/constants/collections.json is non-empty', () => {
        expect(canonical.size).toBeGreaterThan(0);
    });

    it('g8ed Collections binding exports every canonical collection', () => {
        const missing = diff(canonical, jsBinding);
        expect(missing, `missing in components/g8ed/constants/collections.js: ${missing.join(', ')}`)
            .toEqual([]);
    });

    it('g8ed Collections binding has no extra collections beyond canonical', () => {
        const extra = diff(jsBinding, canonical);
        expect(extra, `extra in components/g8ed/constants/collections.js: ${extra.join(', ')}`)
            .toEqual([]);
    });

    it('docs/architecture/storage.md lists every canonical collection', () => {
        const missing = diff(canonical, docs);
        expect(missing, `missing from Collections table in storage.md: ${missing.join(', ')}`)
            .toEqual([]);
    });

    it('docs/architecture/storage.md has no extra collections beyond canonical', () => {
        const extra = diff(docs, canonical);
        expect(extra, `storage.md lists collections not in shared/constants/collections.json: ${extra.join(', ')}`)
            .toEqual([]);
    });
});
