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

import { logger } from '../../utils/logger.js';
import { PLATFORMS } from '../../constants/service_config.js';
import fs from 'fs';
import path from 'path';
import { resolveProjectRoot } from '../../utils/path.js';

/**
 * OperatorDownloadService
 *
 * Owns all operator binary retrieval from local file system.
 * The operator binary is built locally and served directly from disk.
 *
 * Architecture:
 *   g8ed → read  {PROJECT_ROOT}/components/g8eo/build/{os}-{arch}/g8e.operator  → local file system
 */
class OperatorDownloadService {
    constructor() {
        // The operator binary is built at: $PROJECT_ROOT/components/g8eo/build/{os}-{arch}/g8e.operator
        this._projectRoot = resolveProjectRoot();
        this._buildDir = path.join(this._projectRoot, 'components', 'g8eo', 'build');
    }

    _binaryPath(os, arch) {
        return path.join(this._buildDir, `${os}-${arch}`, 'g8e.operator');
    }

    /**
     * Read a binary from local file system for the given platform.
     *
     * @param {string} os
     * @param {string} arch
     * @returns {Promise<Buffer>}
     * @throws {Error} 'Operator binary not available for platform: {os}/{arch}' on any failure
     */
    async getBinary(os, arch) {
        const platform = `${os}/${arch}`;
        const binaryPath = this._binaryPath(os, arch);

        try {
            if (!fs.existsSync(binaryPath)) {
                logger.error(`[OPERATOR-DOWNLOAD-SERVICE] Operator binary not found at ${binaryPath} for platform: ${platform}`);
                throw new Error(`Operator binary not available for platform: ${platform}`);
            }

            const buffer = fs.readFileSync(binaryPath);
            logger.info(`[OPERATOR-DOWNLOAD-SERVICE] Read ${platform} binary from local file system`, {
                path: binaryPath,
                size_mb: (buffer.length / 1024 / 1024).toFixed(2),
            });
            return buffer;
        } catch (error) {
            if (error.message.startsWith('Operator binary not available')) {
                throw error;
            }
            logger.error(`[OPERATOR-DOWNLOAD-SERVICE] Failed to read binary from local file system`, { platform, error: error.message });
            throw new Error(`Operator binary not available for platform: ${platform}`);
        }
    }

    /**
     * Check whether a binary is available for a given platform without reading it.
     *
     * @param {string} os
     * @param {string} arch
     * @returns {Promise<boolean>}
     */
    async hasBinary(os, arch) {
        const binaryPath = this._binaryPath(os, arch);
        try {
            return fs.existsSync(binaryPath);
        } catch {
            return false;
        }
    }

    /**
     * Return availability status for every platform defined in PLATFORMS.
     *
     * @returns {Promise<Record<string, { available: boolean }>>}
     */
    async getPlatformAvailability() {
        const info = {};
        for (const { os, arch } of PLATFORMS) {
            const platformKey = `${os}/${arch}`;
            const available = await this.hasBinary(os, arch);
            info[platformKey] = { available };
        }
        return info;
    }
}

export { OperatorDownloadService };
