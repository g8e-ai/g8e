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
