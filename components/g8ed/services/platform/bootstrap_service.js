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
import crypto from 'crypto';
import { logger } from '../../utils/logger.js';

/**
 * Filename of the tamper-evidence manifest written by g8eo SecretManager
 * alongside the bootstrap secrets on the SSL volume. Consumers verify that
 * the SHA-256 of each secret they loaded from disk matches the digest
 * recorded in this manifest. Must stay in sync with
 * components/g8eo/services/listen/secret_manager.go::BootstrapDigestManifestFile.
 */
export const BOOTSTRAP_DIGEST_MANIFEST_FILE = 'bootstrap_digest.json';

/**
 * Bootstrap Service for g8ed
 * 
 * This service is ONLY responsible for loading values from the g8es data volume.
 * It does not perform any settings management or configuration logic.
 */
class BootstrapService {
    /**
     * @param {string} volumePath - Path to g8es volume (default: /g8es)
     */
    constructor(volumePath = '/g8es') {
        this.volumePath = volumePath;
        this._cachedToken = null;
        this._cachedKey = null;
        this._cachedCaPath = null;
    }

    /**
     * Load internal auth token from g8es volume.
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
                logger.info('[BOOTSTRAP-SERVICE] Loaded internal auth token from g8es volume');
                return this._cachedToken;
            } else {
                logger.info('[BOOTSTRAP-SERVICE] Internal auth token not found in g8es volume');
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
     * Load session encryption key from g8es volume.
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
                logger.info('[BOOTSTRAP-SERVICE] Loaded session encryption key from g8es volume', { 
                    path: keyPath,
                    keyLength: this._cachedKey.length,
                    volumePath: this.volumePath
                });
                return this._cachedKey;
            } else {
                logger.info('[BOOTSTRAP-SERVICE] Session encryption key not found in g8es volume', { 
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
     * Load CA certificate path from g8es volume.
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
                    logger.info('[BOOTSTRAP-SERVICE] Loaded CA cert path from g8es volume', { 
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

        logger.info('[BOOTSTRAP-SERVICE] CA certificate not found in g8es volume');
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
     * Verify the SHA-256 digests of secrets loaded from the g8es volume
     * against the tamper-evidence manifest written by g8eo SecretManager.
     *
     * This closes the silent bootstrap-secret coupling: g8ed otherwise
     * trusts whatever value is on disk, with no cryptographic link back to
     * the DB-authoritative value that SecretManager wrote. A mismatch
     * means the volume file has drifted (partial write, corruption,
     * concurrent writer, manual edit) and authenticating with it would
     * produce confusing downstream 401s instead of a clear startup abort.
     *
     * Behaviour:
     *   - Manifest missing: log a warning and return. g8eo always writes
     *     it during InitPlatformSettings, so absence usually means a
     *     legacy volume or a deployment where g8eo has not yet been
     *     upgraded. We do not want to hard-fail g8ed bootstrap in that
     *     transitional window; the next g8eo boot will create it.
     *   - Manifest present, entry present, digest mismatch: throw.
     *   - Manifest present but no entry for the secret: log and return
     *     (older manifest schema or secret not yet tracked).
     *
     * @param {string} secretName - logical name ("internal_auth_token" | "session_encryption_key")
     * @param {string} value - the value loaded from the volume (already trimmed)
     * @throws {Error} when manifest exists, contains an entry for secretName, and digests disagree.
     */
    verifyAgainstManifest(secretName, value) {
        if (!value) return;

        const manifestPath = path.join(this.volumePath, BOOTSTRAP_DIGEST_MANIFEST_FILE);
        if (!fs.existsSync(manifestPath)) {
            logger.warn('[BOOTSTRAP-SERVICE] Bootstrap digest manifest missing; skipping verification', {
                path: manifestPath,
                secret: secretName
            });
            return;
        }

        let manifest;
        try {
            manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
        } catch (err) {
            throw new Error(
                `Bootstrap digest manifest at ${manifestPath} is unreadable or malformed: ${err.message}. ` +
                `Refusing to start with an unverified ${secretName}.`
            );
        }

        const entry = manifest?.secrets?.[secretName];
        if (!entry || !entry.sha256) {
            logger.warn('[BOOTSTRAP-SERVICE] Bootstrap digest manifest has no entry for secret', {
                secret: secretName,
                manifestVersion: manifest?.version
            });
            return;
        }

        const actual = crypto.createHash('sha256').update(value, 'utf8').digest('hex');
        if (actual !== entry.sha256) {
            throw new Error(
                `Bootstrap secret ${secretName} failed tamper-evidence check: ` +
                `volume SHA-256 ${actual} does not match manifest digest ${entry.sha256}. ` +
                `The on-disk secret has drifted from the DB-authoritative value. ` +
                `Refusing to start to avoid authenticating with a divergent secret.`
            );
        }

        logger.info('[BOOTSTRAP-SERVICE] Bootstrap secret verified against digest manifest', {
            secret: secretName
        });
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
