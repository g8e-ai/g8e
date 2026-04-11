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
 * Constants Enforcement Tests
 * 
 * Scans VSOD source files for raw string literals that should use constants
 * from the constants/ modules (operator.js, auth.js, session.js, sse.js, etc.).
 * 
 * This test auto-extracts constant values from the source-of-truth files and
 * flags any raw usage in services, routes, middleware, models, and utils.
 * 
 * Exclusions:
 * - The constants definition files themselves
 * - Test files and fixtures
 * - Frontend public/ files (have their own mirrored constants)
 * - Views (.ejs templates)
 * - String values inside import/export statements
 * - Comments and JSDoc
 */

import { describe, it, expect } from 'vitest';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const VSOD_ROOT = path.resolve(__dirname, '../..');

// =============================================================================
// CONFIGURATION
// =============================================================================

/**
 * Directories to scan for violations (relative to VSOD_ROOT)
 */
const SCAN_DIRS = ['services', 'routes', 'middleware', 'models', 'utils'];

/**
 * Files/patterns to exclude from scanning
 */
const EXCLUDE_PATTERNS = [
    /node_modules/,
    /\.test\./,
    /\.fixture\./,
    /\.mock\./,
    /test\//,
    /public\//,
    /views\//,
    /constants\//,
];

/**
 * String values that are too generic/short to enforce — they appear in many
 * unrelated contexts (CSS, HTTP, etc.) and would cause false positives.
 */
const ALLOWLISTED_VALUES = new Set([
    // Too short / generic
    'active',
    'inactive',
    'error',
    'pending',
    'completed',
    'failed',
    'timeout',
    null,
    'healthy',
    'degraded',
    'unhealthy',

    // Used in multiple external contexts
    'canceled',
    'internal',
    'expired',
    'suspended',
    'google',
    'email',
    'community',
    'priority',
    'dedicated',

    // Display strings, not enum values
    'The Admin',
    'Community',
    'Collaborative Operations',

    // Very short role values used in many contexts
    'user',
    'admin',
    'superadmin',
    'operator',
    'user',

    // WebSocket/EventEmitter protocol name, pub/sub message type, Winston reserved key
    'message',

    // CSS class values from computeStatusDisplayFields
    'bound',
    'stale',
    'terminated',
    'stopped',
    'available',
    'offline',
]);

// =============================================================================
// CONSTANT EXTRACTION
// =============================================================================

/**
 * Extract all exported constant objects and their string values from a JS file.
 * Returns a map of { constantName: { KEY: 'value', ... } }
 */
function extractConstantsFromFile(filePath) {
    const content = fs.readFileSync(filePath, 'utf-8');
    const constants = {};

    // Match: export const NAME = { ... };
    const objectPattern = /export\s+const\s+(\w+)\s*=\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}/g;
    let match;
    while ((match = objectPattern.exec(content)) !== null) {
        const name = match[1];
        const body = match[2];

        // Extract string values: KEY: 'value' or [Something]: 'value'
        const valuePattern = /:\s*['"]([^'"]+)['"]/g;
        let valueMatch;
        const values = {};
        while ((valueMatch = valuePattern.exec(body)) !== null) {
            values[valueMatch[1]] = true;
        }

        if (Object.keys(values).length > 0) {
            constants[name] = values;
        }
    }

    return constants;
}

/**
 * All constants files to enforce.
 */
const CONSTANTS_FILES = [
    'status.js',
    'collections.js',
    'channels.js',
    'kv_keys.js',
    'operator_defaults.js',
    'paths.js',
    'security.js',
    'service_config.js',
    'timing.js',
];

function buildEnforcedValues() {
    const allConstants = {};
    const enforced = new Map(); // value -> [{ file, constantName }]

    for (const file of CONSTANTS_FILES) {
        const filePath = path.join(VSOD_ROOT, 'constants', file);
        if (!fs.existsSync(filePath)) continue;
        const constants = extractConstantsFromFile(filePath);
        allConstants[file] = constants;

        for (const [constName, values] of Object.entries(constants)) {
            for (const value of Object.keys(values)) {
                if (ALLOWLISTED_VALUES.has(value)) continue;
                if (!enforced.has(value)) {
                    enforced.set(value, []);
                }
                enforced.get(value).push({ file, constantName: constName });
            }
        }
    }

    return { allConstants, enforced };
}

// =============================================================================
// FILE SCANNING
// =============================================================================

/**
 * Get all .js files in the scan directories, excluding patterns.
 */
function getFilesToScan() {
    const files = [];

    for (const dir of SCAN_DIRS) {
        const dirPath = path.join(VSOD_ROOT, dir);
        if (!fs.existsSync(dirPath)) continue;

        const entries = fs.readdirSync(dirPath, { recursive: true });
        for (const entry of entries) {
            const fullPath = path.join(dirPath, entry);
            if (!fullPath.endsWith('.js')) continue;
            if (EXCLUDE_PATTERNS.some(p => p.test(fullPath))) continue;
            files.push(fullPath);
        }
    }

    return files;
}

/**
 * Strip comments and import/export lines from JS source to reduce false positives.
 */
function stripNonCodeContent(source) {
    // Remove block comments
    let stripped = source.replace(/\/\*[\s\S]*?\*\//g, '');
    // Remove line comments
    stripped = stripped.replace(/\/\/.*/g, '');
    // Remove import/export lines
    stripped = stripped.replace(/^\s*(import|export)\s+.*$/gm, '');
    return stripped;
}

/**
 * Find violations in a single file.
 * Returns array of { line, lineNumber, value, constantName, constantFile }
 */
function findViolationsInFile(filePath, enforced) {
    const source = fs.readFileSync(filePath, 'utf-8');
    const stripped = stripNonCodeContent(source);
    const lines = stripped.split('\n');
    const violations = [];

    for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (!line.trim()) continue;

        for (const [value, sources] of enforced.entries()) {
            // Match 'value' or "value" as a standalone string literal
            // Use word boundary-like checks to avoid matching substrings
            const patterns = [
                new RegExp(`'${escapeRegex(value)}'`, 'g'),
                new RegExp(`"${escapeRegex(value)}"`, 'g'),
            ];

            for (const pattern of patterns) {
                if (pattern.test(line)) {
                    // Skip if this line is a logger/console message (string in template literal or log)
                    if (isLogLine(line)) continue;
                    // Skip if this is an error message string
                    if (isErrorMessage(line, value)) continue;
                    // Skip if inside a template literal for display purposes
                    if (isDisplayString(line, value)) continue;

                    violations.push({
                        lineNumber: i + 1,
                        line: line.trim(),
                        value,
                        constantName: sources[0].constantName,
                        constantFile: sources[0].file,
                    });
                }
            }
        }
    }

    return violations;
}

function escapeRegex(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function isLogLine(line) {
    return /logger\.(info|warn|error|debug|trace)/.test(line) ||
           /console\.(log|warn|error|info)/.test(line);
}

function isErrorMessage(line, value) {
    // Skip lines where the string is part of an Error message or throw
    return /(throw new Error|new Error)\(/.test(line) && line.includes(value);
}

function isDisplayString(line, value) {
    // Skip template literals used for display/logging
    return /`[^`]*\$\{/.test(line) && line.includes(value);
}

// =============================================================================
// TESTS
// =============================================================================

const { allConstants, enforced } = buildEnforcedValues();

describe('Constants Enforcement', () => {

    it('should extract constants from all constants files', () => {
        const filesWithConstants = Object.keys(allConstants);
        expect(filesWithConstants.length).toBeGreaterThan(0);

        console.log(`\n  Extracted constants from ${filesWithConstants.length} files:`);
        for (const file of filesWithConstants) {
            const constantNames = Object.keys(allConstants[file]);
            const totalValues = constantNames.reduce((sum, name) => sum + Object.keys(allConstants[file][name]).length, 0);
            console.log(`    ${file}: ${constantNames.length} groups, ${totalValues} values`);
        }
    });

    it('should have enforced values after filtering allowlist', () => {
        expect(enforced.size).toBeGreaterThan(0);
        console.log(`\n  Enforcing ${enforced.size} constant values (${ALLOWLISTED_VALUES.size} allowlisted)`);
        console.log(`  Enforced values: ${[...enforced.keys()].sort().join(', ')}`);
    });

    it('should not use raw string literals where constants exist', () => {
        const files = getFilesToScan();
        expect(files.length).toBeGreaterThan(0);

        const allViolations = [];

        for (const filePath of files) {
            const violations = findViolationsInFile(filePath, enforced);
            for (const v of violations) {
                allViolations.push({
                    file: path.relative(VSOD_ROOT, filePath),
                    ...v,
                });
            }
        }

        if (allViolations.length > 0) {
            const summary = new Map();
            for (const v of allViolations) {
                const key = `${v.file}`;
                if (!summary.has(key)) summary.set(key, []);
                summary.get(key).push(v);
            }

            let report = `\n\n  Found ${allViolations.length} raw string literal(s) that should use constants:\n`;
            for (const [file, violations] of summary) {
                report += `\n  ${file}:\n`;
                for (const v of violations) {
                    report += `    Line ${v.lineNumber}: '${v.value}' → use ${v.constantName} from ${v.constantFile}\n`;
                    report += `      ${v.line}\n`;
                }
            }

            expect(allViolations, report).toHaveLength(0);
        }
    });

    it('should scan a meaningful number of source files', () => {
        const files = getFilesToScan();
        console.log(`\n  Scanned ${files.length} source files across: ${SCAN_DIRS.join(', ')}`);
        expect(files.length).toBeGreaterThan(10);
    });
});
