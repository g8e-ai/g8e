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

import path from 'path';
import fs from 'fs';
import { logger } from '../../utils/logger.js';

/**
 * Bootstrap Service for VSOD
 * 
 * This service is ONLY responsible for loading values from the VSODB data volume.
 * It does not perform any settings management or configuration logic.
 */
class BootstrapService {
    /**
     * @param {string} volumePath - Path to VSODB volume (default: /vsodb)
     */
    constructor(volumePath = '/vsodb') {
        this.volumePath = volumePath;
        this._cachedToken = null;
        this._cachedKey = null;
        this._cachedCaPath = null;
    }

    /**
     * Load internal auth token from VSODB volume.
     * @returns {string|null}
     */
    loadInternalAuthToken() {
        if (this._cachedToken !== null) {
            return this._cachedToken;
        }

        const tokenPath = path.join(this.volumePath, 'internal_auth_token');
        try {
            if (fs.existsSync(tokenPath)) {
                this._cachedToken = fs.readFileSync(tokenPath, 'utf8').trim();
                logger.info('[BOOTSTRAP-SERVICE] Loaded internal auth token from VSODB volume');
                return this._cachedToken;
            } else {
                logger.info('[BOOTSTRAP-SERVICE] Internal auth token not found in VSODB volume');
                return null;
            }
        } catch (err) {
            logger.warn('[BOOTSTRAP-SERVICE] Failed to read internal auth token', { 
                error: err.message 
            });
            return null;
        }
    }

    /**
     * Load session encryption key from VSODB volume.
     * @returns {string|null}
     */
    loadSessionEncryptionKey() {
        if (this._cachedKey !== null) return this._cachedKey;

        const keyPath = path.join(this.volumePath, 'session_encryption_key');
        logger.info('[BOOTSTRAP-SERVICE] Checking for session encryption key', { 
            path: keyPath, 
            exists: fs.existsSync(keyPath),
            volumePath: this.volumePath,
            volumeExists: fs.existsSync(this.volumePath)
        });
        
        try {
            if (fs.existsSync(keyPath)) {
                this._cachedKey = fs.readFileSync(keyPath, 'utf8').trim();
                logger.info('[BOOTSTRAP-SERVICE] Loaded session encryption key from VSODB volume', { 
                    path: keyPath,
                    keyLength: this._cachedKey.length,
                    volumePath: this.volumePath
                });
                return this._cachedKey;
            } else {
                logger.info('[BOOTSTRAP-SERVICE] Session encryption key not found in VSODB volume', { 
                    path: keyPath,
                    volumePath: this.volumePath,
                    volumeExists: fs.existsSync(this.volumePath),
                    volumeContents: this._safeListVolume(this.volumePath)
                });
                return null;
            }
        } catch (err) {
            logger.warn('[BOOTSTRAP-SERVICE] Failed to read session encryption key', { 
                path: keyPath,
                error: err.message,
                volumePath: this.volumePath,
                volumeExists: fs.existsSync(this.volumePath)
            });
            return null;
        }
    }

    /**
     * Load CA certificate path from VSODB volume.
     * @returns {string|null}
     */
    loadCaCertPath() {
        if (this._cachedCaPath !== null) {
            return this._cachedCaPath;
        }

        // Check both possible locations
        const caPaths = [
            path.join(this.volumePath, 'ca.crt'),
            path.join(this.volumePath, 'ca', 'ca.crt')
        ];

        for (const caPath of caPaths) {
            try {
                if (fs.existsSync(caPath)) {
                    this._cachedCaPath = caPath;
                    logger.info('[BOOTSTRAP-SERVICE] Loaded CA cert path from VSODB volume', { 
                        path: this._cachedCaPath 
                    });
                    return this._cachedCaPath;
                }
            } catch (err) {
                logger.warn('[BOOTSTRAP-SERVICE] Failed to read CA cert', { 
                    path: caPath,
                    error: err.message 
                });
            }
        }

        logger.info('[BOOTSTRAP-SERVICE] CA certificate not found in VSODB volume');
        return null;
    }

    /**
     * Get SSL directory path.
     * @returns {string}
     */
    getSslDir() {
        return this.volumePath;
    }

    /**
     * Check if bootstrap data is available.
     * @returns {boolean}
     */
    isAvailable() {
        if (!fs.existsSync(this.volumePath)) {
            return false;
        }

        return (
            this.loadInternalAuthToken() !== null ||
            this.loadSessionEncryptionKey() !== null ||
            this.loadCaCertPath() !== null
        );
    }

    /**
     * Clear cached values - useful for testing or re-initialization.
     */
    clearCache() {
        this._cachedToken = null;
        this._cachedKey = null;
        this._cachedCaPath = null;
    }

    /**
     * Safely list volume contents for debugging.
     * @param {string} volumePath
     * @returns {string[]}
     */
    _safeListVolume(volumePath) {
        try {
            if (!fs.existsSync(volumePath)) {
                return ['volume does not exist'];
            }
            const contents = fs.readdirSync(volumePath);
            return contents.length > 0 ? contents : ['empty directory'];
        } catch (err) {
            return [`error reading directory: ${err.message}`];
        }
    }
}

export { BootstrapService };
