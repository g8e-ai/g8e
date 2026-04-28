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

// Cross-language fixture parity for ledger hashing.
// Consumes shared/test-fixtures/ledger-hash-fixtures.json (generated from the
// Python implementation) and asserts the JS verifier produces byte-identical
// canonical JSON, entry hashes, genesis hashes, and chain validation results.

import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { dirname, resolve } from 'path';
import { fileURLToPath } from 'url';
import {
    canonicalJson,
    computeEntryHash,
    genesisHash,
    verifyChain,
} from '../../../public/js/utils/ledger-verify.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const fixturesPath = resolve(__dirname, '../../../../../shared/test-fixtures/ledger-hash-fixtures.json');
const fixtures = JSON.parse(readFileSync(fixturesPath, 'utf8'));

describe('ledger-verify cross-language fixture parity', () => {
    describe('canonicalJson matches Python output', () => {
        for (const c of fixtures.canonical_json) {
            it(c.name, () => {
                const actual = canonicalJson(c.input).toString('utf-8');
                expect(actual).toBe(c.expected_utf8);
            });
        }
    });

    describe('computeEntryHash matches Python output', () => {
        for (const c of fixtures.entry_hash) {
            it(c.name, () => {
                const actual = computeEntryHash(c.entry, c.prev_hash);
                expect(actual).toBe(c.expected_hash);
            });
        }
    });

    describe('genesisHash matches Python output', () => {
        for (const c of fixtures.genesis_hash) {
            it(`${c.investigation_id}@${c.created_at}`, () => {
                const actual = genesisHash(c.investigation_id, c.created_at);
                expect(actual).toBe(c.expected_hash);
            });
        }
    });

    describe('chain fixture', () => {
        it('verifies cleanly', () => {
            const { entries, investigation_id, created_at } = fixtures.chain;
            const result = verifyChain(entries, investigation_id, created_at);
            expect(result.isValid).toBe(true);
            expect(result.firstBadIndex).toBeNull();
        });

        it('detects tampering on the middle entry', () => {
            const { entries, investigation_id, created_at } = fixtures.chain;
            const tampered = entries.map((e) => ({ ...e }));
            tampered[1] = { ...tampered[1], content: 'tampered' };
            const result = verifyChain(tampered, investigation_id, created_at);
            expect(result.isValid).toBe(false);
            expect(result.firstBadIndex).toBe(1);
        });
    });
});
