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

import { createHash } from 'crypto';

/**
 * Convert object to canonical JSON bytes (sorted keys, no whitespace, UTF-8).
 * This ensures deterministic serialization for hash computation.
 * 
 * @param {Object} obj - Dictionary to serialize
 * @returns {Buffer} UTF-8 encoded JSON bytes with sorted keys and no whitespace
 */
function sortObject(value) {
    if (Array.isArray(value)) {
        return value.map(sortObject);
    }
    if (value !== null && typeof value === 'object') {
        return Object.keys(value).sort().reduce((acc, key) => {
            acc[key] = sortObject(value[key]);
            return acc;
        }, {});
    }
    return value;
}

export function canonicalJson(obj) {
    // JSON.stringify with no spacing matches Python's separators=(",",":")
    // sortObject mirrors Python's sort_keys=True (recursive).
    const json = JSON.stringify(sortObject(obj));
    return Buffer.from(json, 'utf-8');
}

/**
 * Compute the hash for a ledger entry.
 * The hash is computed from the entry content (excluding prev_hash and entry_hash
 * fields themselves) concatenated with the previous entry's hash, forming a chain.
 * 
 * @param {Object} entry - Dictionary representing the ledger entry
 * @param {string|null} prevHash - Hash of the previous entry in the chain (null for genesis)
 * @returns {string} Hexadecimal SHA256 hash string (64 characters)
 */
export function computeEntryHash(entry, prevHash) {
    // Create a copy without the hash fields to avoid circular dependency
    const entryCopy = { ...entry };
    delete entryCopy.prev_hash;
    delete entryCopy.entry_hash;
    
    const hasher = createHash('sha256');
    hasher.update(canonicalJson(entryCopy));
    
    if (prevHash) {
        hasher.update(Buffer.from(prevHash, 'utf-8'));
    }
    
    return hasher.digest('hex');
}

/**
 * Compute the genesis hash for a new investigation chain.
 * The genesis hash is the starting point of the hash chain, derived from
 * the investigation's identity and creation timestamp.
 * 
 * @param {string} investigationId - UUID of the investigation
 * @param {string} createdAt - ISO 8601 timestamp string
 * @returns {string} Hexadecimal SHA256 hash string (64 characters)
 */
export function genesisHash(investigationId, createdAt) {
    const hasher = createHash('sha256');
    hasher.update(Buffer.from(`${investigationId}:${createdAt}`, 'utf-8'));
    return hasher.digest('hex');
}

/**
 * Verify the integrity of a hash chain.
 * Checks that each entry's hash correctly chains to the previous entry,
 * starting from the genesis hash.
 * 
 * @param {Array<Object>} entries - List of ledger entries in order
 * @param {string} investigationId - Investigation UUID for genesis computation
 * @param {string} createdAt - Investigation creation timestamp for genesis computation
 * @returns {Object} Object with properties:
 *   - isValid: boolean - True if chain is valid, False otherwise
 *   - firstBadIndex: number|null - Index of first invalid entry, or null if valid
 */
export function verifyChain(entries, investigationId, createdAt) {
    if (!entries || entries.length === 0) {
        return { isValid: true, firstBadIndex: null };
    }
    
    // Start with genesis hash
    let expectedPrevHash = genesisHash(investigationId, createdAt);
    
    for (let idx = 0; idx < entries.length; idx++) {
        const entry = entries[idx];
        
        // Check if entry has required hash fields
        if (entry.entry_hash === undefined || entry.prev_hash === undefined) {
            return { isValid: false, firstBadIndex: idx };
        }
        
        // Verify prev_hash matches expected
        if (entry.prev_hash !== expectedPrevHash) {
            return { isValid: false, firstBadIndex: idx };
        }
        
        // Verify entry_hash is correct
        const computedHash = computeEntryHash(entry, entry.prev_hash);
        if (entry.entry_hash !== computedHash) {
            return { isValid: false, firstBadIndex: idx };
        }
        
        // Move to next entry
        expectedPrevHash = entry.entry_hash;
    }
    
    return { isValid: true, firstBadIndex: null };
}
