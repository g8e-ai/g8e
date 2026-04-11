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
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const VSOD_ROOT = path.resolve(__dirname, '../../..');

/**
 * Directories to scan for raw internal API path usage
 */
const SCAN_DIRS = ['services', 'routes', 'middleware', 'models', 'utils'];

/**
 * Exclusions:
 * - node_modules
 * - tests, mocks, fixtures
 * - public/ files (frontend)
 * - views
 * - the constants/api_paths.js file itself
 */
const EXCLUDE_PATTERNS = [
    /node_modules/,
    /\.test\./,
    /\.fixture\./,
    /\.mock\./,
    /test\//,
    /public\//,
    /views\//,
    /constants\/api_paths\.js/,
];

/**
 * Load InternalApiPaths to get the strings we want to enforce.
 * We want to catch raw usage of paths like '/api/internal/chat'.
 */
async function getEnforcedPaths() {
    const { InternalApiPaths } = await import('../../../constants/api_paths.js');
    const enforced = new Map(); // path -> { category, key }

    // Enforce vse paths
    for (const [key, pathValue] of Object.entries(InternalApiPaths.vse)) {
        enforced.set(pathValue, { category: 'vse', key });
    }

    // Enforce vsod internal paths
    for (const [key, pathValue] of Object.entries(InternalApiPaths.vsod)) {
        enforced.set(pathValue, { category: 'vsod', key });
    }

    return enforced;
}

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

function findViolations(filePath, enforcedPaths) {
    const content = fs.readFileSync(filePath, 'utf-8');
    const lines = content.split('\n');
    const violations = [];

    for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (line.includes('//') || line.includes('*')) continue; // Simple comment skip

        for (const [pathValue, info] of enforcedPaths.entries()) {
            const escapedPath = pathValue.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
            // Match 'path' or "path"
            const pattern = new RegExp(`['"]${escapedPath}['"]`, 'g');
            
            if (pattern.test(line)) {
                // Skip log lines
                if (/logger\.(info|warn|error|debug|trace)/.test(line)) continue;
                if (/console\.(log|warn|error|info)/.test(line)) continue;
                
                violations.push({
                    lineNumber: i + 1,
                    line: line.trim(),
                    pathValue,
                    category: info.category,
                    key: info.key
                });
            }
        }
    }
    return violations;
}

describe('Internal API Path Enforcement', async () => {
    const enforcedPaths = await getEnforcedPaths();

    it('should not use raw strings for internal API paths', () => {
        const files = getFilesToScan();
        const allViolations = [];

        for (const file of files) {
            const violations = findViolations(file, enforcedPaths);
            for (const v of violations) {
                allViolations.push({
                    file: path.relative(VSOD_ROOT, file),
                    ...v
                });
            }
        }

        if (allViolations.length > 0) {
            let report = `\nFound ${allViolations.length} raw internal API path(s):\n`;
            for (const v of allViolations) {
                report += `  ${v.file}:${v.lineNumber} - '${v.pathValue}'\n`;
                report += `    Use ApiPaths.internal.${v.key}() or InternalApiPaths.${v.category}.${v.key}\n`;
                report += `    Line: ${v.line}\n`;
            }
            expect(allViolations, report).toHaveLength(0);
        }
    });
});
