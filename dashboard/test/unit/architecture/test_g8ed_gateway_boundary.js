// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect } from 'vitest';
import { readFileSync, existsSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const dashboardRoot = join(fileURLToPath(new URL('../../../', import.meta.url)));

function read(relPath) {
    return readFileSync(join(dashboardRoot, relPath), 'utf8');
}

function walkJsFiles(dir, files = []) {
    for (const entry of readdirSync(dir)) {
        const full = join(dir, entry);
        if (entry === 'g8e-adapter') continue;
        if (statSync(full).isDirectory()) {
            walkJsFiles(full, files);
            continue;
        }
        if (entry.endsWith('.js')) {
            files.push(full);
        }
    }
    return files;
}

describe('g8ed gateway boundary guards', () => {
    it('server.js is static host only (no API routers)', () => {
        const source = read('server.js');
        expect(source).not.toMatch(/express\.Router\(/);
        expect(source).not.toMatch(/app\.use\(['"]\/api/);
        expect(source).toContain('createApp');
    });

    it('legacy BFF trees are removed', () => {
        for (const rel of ['routes', 'services/operator', 'services/platform', 'services/cache', 'models', 'views', 'middleware']) {
            expect(existsSync(join(dashboardRoot, rel))).toBe(false);
        }
    });

    it('production browser JS does not call ServiceName.g8ed', () => {
        const publicJs = join(dashboardRoot, 'public/js');
        const offenders = walkJsFiles(publicJs)
            .filter((file) => {
                const text = readFileSync(file, 'utf8');
                return text.includes('ServiceName.g8ed') || text.includes("'g8ed'");
            })
            .map((file) => file.replace(`${dashboardRoot}/`, ''));
        expect(offenders).toEqual([]);
    });

    it('SSE manager uses gateway stream URL', () => {
        const source = read('public/js/utils/sse-connection-manager.js');
        expect(source).toContain('ApiPaths.sse.stream()');
        expect(source).toContain('gatewayUrl');
        expect(source).not.toMatch(/new EventSource\(ApiPaths\.sse\.events\(\)/);
    });
});
