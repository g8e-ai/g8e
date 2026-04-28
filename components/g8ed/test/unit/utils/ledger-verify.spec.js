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

import { describe, it, expect } from 'vitest';
import { canonicalJson, computeEntryHash, genesisHash, verifyChain } from '../../../public/js/utils/ledger-verify.js';

describe('ledger-verify', () => {
    describe('canonicalJson', () => {
        it('produces deterministic output regardless of key order', () => {
            const obj1 = { z: 1, a: 2, m: 3 };
            const obj2 = { a: 2, z: 1, m: 3 };
            const obj3 = { m: 3, a: 2, z: 1 };
            
            const result1 = canonicalJson(obj1);
            const result2 = canonicalJson(obj2);
            const result3 = canonicalJson(obj3);
            
            expect(result1.equals(result2)).toBe(true);
            expect(result2.equals(result3)).toBe(true);
            
            // Verify it's sorted keys, no whitespace
            const decoded = JSON.parse(result1.toString('utf-8'));
            expect(Object.keys(decoded)).toEqual(['a', 'm', 'z']);
        });

        it('produces compact JSON with no whitespace', () => {
            const obj = { a: 1, b: { c: 2, d: [3, 4] } };
            const result = canonicalJson(obj);
            
            const str = result.toString('utf-8');
            expect(str).not.toContain(' ');
            expect(str).not.toContain('\n');
            expect(str).toBe('{"a":1,"b":{"c":2,"d":[3,4]}}');
        });

        it('produces UTF-8 bytes', () => {
            const obj = { test: 'value' };
            const result = canonicalJson(obj);
            
            expect(Buffer.isBuffer(result)).toBe(true);
            expect(result.toString('utf-8')).toBe('{"test":"value"}');
        });
    });

    describe('genesisHash', () => {
        it('produces deterministic hash from investigation_id and created_at', () => {
            const investigationId = 'test-investigation-123';
            const createdAt = '2024-01-01T00:00:00Z';
            
            const hash1 = genesisHash(investigationId, createdAt);
            const hash2 = genesisHash(investigationId, createdAt);
            
            expect(hash1).toBe(hash2);
            expect(hash1.length).toBe(64); // SHA256 hex string
            expect(hash1).toMatch(/^[0-9a-f]{64}$/);
        });

        it('produces different hashes for different inputs', () => {
            const hash1 = genesisHash('inv-1', '2024-01-01T00:00:00Z');
            const hash2 = genesisHash('inv-2', '2024-01-01T00:00:00Z');
            const hash3 = genesisHash('inv-1', '2024-01-02T00:00:00Z');
            
            expect(hash1).not.toBe(hash2);
            expect(hash1).not.toBe(hash3);
            expect(hash2).not.toBe(hash3);
        });
    });

    describe('computeEntryHash', () => {
        it('produces hash from entry and prev_hash', () => {
            const entry = { sender: 'user.chat', content: 'test', timestamp: '2024-01-01T00:00:00Z' };
            const prevHash = 'a'.repeat(64);
            
            const hash1 = computeEntryHash(entry, prevHash);
            const hash2 = computeEntryHash(entry, prevHash);
            
            expect(hash1).toBe(hash2);
            expect(hash1.length).toBe(64);
            expect(hash1).not.toBe(prevHash); // Hash should include entry content
        });

        it('works with null prev_hash (genesis case)', () => {
            const entry = { sender: 'user.chat', content: 'test', timestamp: '2024-01-01T00:00:00Z' };
            
            const hash1 = computeEntryHash(entry, null);
            const hash2 = computeEntryHash(entry, null);
            
            expect(hash1).toBe(hash2);
            expect(hash1.length).toBe(64);
        });

        it('excludes hash fields from computation', () => {
            const entry = {
                sender: 'user.chat',
                content: 'test',
                timestamp: '2024-01-01T00:00:00Z',
                prev_hash: 'a'.repeat(64),
                entry_hash: 'b'.repeat(64),
            };
            const prevHash = 'c'.repeat(64);
            
            const hash1 = computeEntryHash(entry, prevHash);
            const hash2 = computeEntryHash(entry, prevHash);
            
            // Changing the hash fields shouldn't affect the result
            entry.prev_hash = 'd'.repeat(64);
            entry.entry_hash = 'e'.repeat(64);
            const hash3 = computeEntryHash(entry, prevHash);
            
            expect(hash1).toBe(hash2);
            expect(hash2).toBe(hash3);
        });
    });

    describe('verifyChain', () => {
        it('returns true for a valid hash chain', () => {
            const investigationId = 'test-inv';
            const createdAt = '2024-01-01T00:00:00Z';
            
            let prevHash = genesisHash(investigationId, createdAt);
            const entries = [];
            
            for (let i = 0; i < 3; i++) {
                const entry = {
                    id: `msg-${i}`,
                    sender: 'user.chat',
                    content: `message ${i}`,
                    timestamp: `2024-01-01T00:0${i}:00Z`,
                    prev_hash: prevHash,
                };
                const entryHash = computeEntryHash(entry, prevHash);
                entry.entry_hash = entryHash;
                entries.push(entry);
                prevHash = entryHash;
            }
            
            const result = verifyChain(entries, investigationId, createdAt);
            expect(result.isValid).toBe(true);
            expect(result.firstBadIndex).toBeNull();
        });

        it('detects tampered entry', () => {
            const investigationId = 'test-inv';
            const createdAt = '2024-01-01T00:00:00Z';
            
            let prevHash = genesisHash(investigationId, createdAt);
            const entries = [];
            
            for (let i = 0; i < 3; i++) {
                const entry = {
                    id: `msg-${i}`,
                    sender: 'user.chat',
                    content: `message ${i}`,
                    timestamp: `2024-01-01T00:0${i}:00Z`,
                    prev_hash: prevHash,
                };
                const entryHash = computeEntryHash(entry, prevHash);
                entry.entry_hash = entryHash;
                entries.push(entry);
                prevHash = entryHash;
            }
            
            // Tamper with middle entry
            entries[1].content = 'tampered';
            
            const result = verifyChain(entries, investigationId, createdAt);
            expect(result.isValid).toBe(false);
            expect(result.firstBadIndex).toBe(1);
        });

        it('detects broken hash chain link', () => {
            const investigationId = 'test-inv';
            const createdAt = '2024-01-01T00:00:00Z';
            
            let prevHash = genesisHash(investigationId, createdAt);
            const entries = [];
            
            for (let i = 0; i < 3; i++) {
                const entry = {
                    id: `msg-${i}`,
                    sender: 'user.chat',
                    content: `message ${i}`,
                    timestamp: `2024-01-01T00:0${i}:00Z`,
                    prev_hash: prevHash,
                };
                const entryHash = computeEntryHash(entry, prevHash);
                entry.entry_hash = entryHash;
                entries.push(entry);
                prevHash = entryHash;
            }
            
            // Break the link
            entries[1].prev_hash = 'wrong'.repeat(32);
            
            const result = verifyChain(entries, investigationId, createdAt);
            expect(result.isValid).toBe(false);
            expect(result.firstBadIndex).toBe(1);
        });

        it('handles empty chain', () => {
            const result = verifyChain([], 'test-inv', '2024-01-01T00:00:00Z');
            expect(result.isValid).toBe(true);
            expect(result.firstBadIndex).toBeNull();
        });

        it('handles single entry chain', () => {
            const investigationId = 'test-inv';
            const createdAt = '2024-01-01T00:00:00Z';
            
            const prevHash = genesisHash(investigationId, createdAt);
            const entry = {
                id: 'msg-0',
                sender: 'user.chat',
                content: 'message 0',
                timestamp: '2024-01-01T00:00:00Z',
                prev_hash: prevHash,
            };
            const entryHash = computeEntryHash(entry, prevHash);
            entry.entry_hash = entryHash;
            
            const result = verifyChain([entry], investigationId, createdAt);
            expect(result.isValid).toBe(true);
            expect(result.firstBadIndex).toBeNull();
        });

        it('handles entries without hash fields (backward compat)', () => {
            const investigationId = 'test-inv';
            const createdAt = '2024-01-01T00:00:00Z';
            
            // Entry without hash fields (old format)
            const entry = {
                id: 'msg-0',
                sender: 'user.chat',
                content: 'message 0',
                timestamp: '2024-01-01T00:00:00Z',
            };
            
            // Should fail verification since hash fields are required for chain integrity
            const result = verifyChain([entry], investigationId, createdAt);
            expect(result.isValid).toBe(false);
            expect(result.firstBadIndex).toBe(0);
        });
    });
});
