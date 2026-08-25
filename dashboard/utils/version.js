// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Version utility - reads VERSION file.
 * The VERSION file at the component root contains the platform semver (e.g., v4.2.0).
 */

import { readFileSync } from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { VERSION_FALLBACK } from '../constants/service_config.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

let cachedVersionInfo = null;

function getVersion() {
    try {
        const versionPath = path.join(__dirname, '..', 'VERSION');
        return readFileSync(versionPath, 'utf8').trim();
    } catch {
        return VERSION_FALLBACK;
    }
}

export function getVersionInfo() {
    if (cachedVersionInfo) {
        return cachedVersionInfo;
    }

    const version = getVersion();
    cachedVersionInfo = { version };
    return cachedVersionInfo;
}

export default getVersionInfo;
