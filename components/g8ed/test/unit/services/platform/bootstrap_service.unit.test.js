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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { mkdtempSync, rmSync, mkdirSync, writeFileSync, existsSync, readFileSync } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: {
        info: vi.fn(),
        warn: vi.fn(),
        error: vi.fn(),
        debug: vi.fn()
    }
}));

describe('BootstrapService [UNIT - filesystem isolated]', () => {
    let BootstrapService;
    let bootstrapService;
    let tmpVolumePath;

    beforeEach(async () => {
        vi.clearAllMocks();
        const module = await import('@g8ed/services/platform/bootstrap_service.js');
        BootstrapService = module.BootstrapService;
        tmpVolumePath = mkdtempSync(join(tmpdir(), 'g8e-bootstrap-'));
        bootstrapService = new BootstrapService(tmpVolumePath);
    });

    afterEach(() => {
        rmSync(tmpVolumePath, { recursive: true, force: true });
    });

    describe('constructor', () => {
        it('should use default volume path /g8es when not provided', () => {
            const service = new BootstrapService();
            expect(service.volumePath).toBe('/g8es');
        });

        it('should use custom volume path when provided', () => {
            const customPath = '/custom/g8es';
            const service = new BootstrapService(customPath);
            expect(service.volumePath).toBe(customPath);
        });

        it('should initialize with null cached values', () => {
            expect(bootstrapService._cachedToken).toBeNull();
            expect(bootstrapService._cachedKey).toBeNull();
            expect(bootstrapService._cachedCaPath).toBeNull();
        });
    });

    describe('loadInternalAuthToken', () => {
        it('should load and cache token from volume', () => {
            const testToken = 'test-internal-auth-token-12345';
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), testToken);

            const result = bootstrapService.loadInternalAuthToken();

            expect(result).toBe(testToken);
            expect(bootstrapService._cachedToken).toBe(testToken);
        });

        it('should trim whitespace from token', () => {
            const testToken = '  test-token-with-spaces  ';
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), testToken);

            const result = bootstrapService.loadInternalAuthToken();

            expect(result).toBe('test-token-with-spaces');
        });

        it('should return cached token on subsequent calls', () => {
            const testToken = 'cached-token';
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), testToken);

            const firstCall = bootstrapService.loadInternalAuthToken();
            const secondCall = bootstrapService.loadInternalAuthToken();

            expect(firstCall).toBe(testToken);
            expect(secondCall).toBe(testToken);
            expect(secondCall).toBe(firstCall);
        });

        it('should return null when token file does not exist', () => {
            const result = bootstrapService.loadInternalAuthToken();

            expect(result).toBeNull();
            expect(bootstrapService._cachedToken).toBeNull();
        });

        it('should return null and log warning when read fails', () => {
            mkdirSync(tmpVolumePath, { recursive: true });
            const tokenPath = join(tmpVolumePath, 'internal_auth_token');
            writeFileSync(tokenPath, 'test-token');

            vi.spyOn(require('fs'), 'readFileSync').mockImplementationOnce(() => {
                throw new Error('Permission denied');
            });

            const result = bootstrapService.loadInternalAuthToken();

            expect(result).toBeNull();
            expect(bootstrapService._cachedToken).toBeNull();
        });

        it('should return null when volume path does not exist', () => {
            const service = new BootstrapService('/nonexistent/path');
            const result = service.loadInternalAuthToken();

            expect(result).toBeNull();
        });
    });

    describe('loadSessionEncryptionKey', () => {
        it('should load and cache key from volume', () => {
            const testKey = 'a'.repeat(64);
            writeFileSync(join(tmpVolumePath, 'session_encryption_key'), testKey);

            const result = bootstrapService.loadSessionEncryptionKey();

            expect(result).toBe(testKey);
            expect(bootstrapService._cachedKey).toBe(testKey);
        });

        it('should trim whitespace from key', () => {
            const testKey = '  ' + 'a'.repeat(64) + '  ';
            writeFileSync(join(tmpVolumePath, 'session_encryption_key'), testKey);

            const result = bootstrapService.loadSessionEncryptionKey();

            expect(result).toBe('a'.repeat(64));
        });

        it('should return cached key on subsequent calls', () => {
            const testKey = 'b'.repeat(64);
            writeFileSync(join(tmpVolumePath, 'session_encryption_key'), testKey);

            const firstCall = bootstrapService.loadSessionEncryptionKey();
            const secondCall = bootstrapService.loadSessionEncryptionKey();

            expect(firstCall).toBe(testKey);
            expect(secondCall).toBe(testKey);
            expect(secondCall).toBe(firstCall);
        });

        it('should return null when key file does not exist', () => {
            const result = bootstrapService.loadSessionEncryptionKey();

            expect(result).toBeNull();
            expect(bootstrapService._cachedKey).toBeNull();
        });

        it('should return null and log warning when read fails', () => {
            mkdirSync(tmpVolumePath, { recursive: true });
            const keyPath = join(tmpVolumePath, 'session_encryption_key');
            writeFileSync(keyPath, 'a'.repeat(64));

            vi.spyOn(require('fs'), 'readFileSync').mockImplementationOnce(() => {
                throw new Error('IO error');
            });

            const result = bootstrapService.loadSessionEncryptionKey();

            expect(result).toBeNull();
            expect(bootstrapService._cachedKey).toBeNull();
        });

        it('should return null when volume path does not exist', () => {
            const service = new BootstrapService('/nonexistent/path');
            const result = service.loadSessionEncryptionKey();

            expect(result).toBeNull();
        });
    });

    describe('loadCaCertPath', () => {
        it('should find and cache CA cert at /g8es/ca.crt', () => {
            const caDir = join(tmpVolumePath, 'ca');
            mkdirSync(caDir, { recursive: true });
            writeFileSync(join(caDir, 'ca.crt'), 'ca-cert-content');

            const result = bootstrapService.loadCaCertPath();

            expect(result).toBe(join(tmpVolumePath, 'ca', 'ca.crt'));
            expect(bootstrapService._cachedCaPath).toBe(join(tmpVolumePath, 'ca', 'ca.crt'));
        });

        it('should find and cache CA cert at /g8es/ca/ca.crt (legacy path)', () => {
            const caDir = join(tmpVolumePath, 'ca');
            mkdirSync(caDir, { recursive: true });
            writeFileSync(join(caDir, 'ca.crt'), 'ca-cert-content');

            const result = bootstrapService.loadCaCertPath();

            expect(result).toBe(join(tmpVolumePath, 'ca', 'ca.crt'));
        });

        it('should prefer /g8es/ca.crt over /g8es/ca/ca.crt', () => {
            writeFileSync(join(tmpVolumePath, 'ca.crt'), 'preferred-ca-cert');
            const caDir = join(tmpVolumePath, 'ca');
            mkdirSync(caDir, { recursive: true });
            writeFileSync(join(caDir, 'ca.crt'), 'legacy-ca-cert');

            const result = bootstrapService.loadCaCertPath();

            expect(result).toBe(join(tmpVolumePath, 'ca.crt'));
        });

        it('should return cached path on subsequent calls', () => {
            const caDir = join(tmpVolumePath, 'ca');
            mkdirSync(caDir, { recursive: true });
            writeFileSync(join(caDir, 'ca.crt'), 'ca-cert-content');

            const firstCall = bootstrapService.loadCaCertPath();
            const secondCall = bootstrapService.loadCaCertPath();

            expect(firstCall).toBe(join(tmpVolumePath, 'ca', 'ca.crt'));
            expect(secondCall).toBe(firstCall);
        });

        it('should return null when neither CA cert location exists', () => {
            const result = bootstrapService.loadCaCertPath();

            expect(result).toBeNull();
            expect(bootstrapService._cachedCaPath).toBeNull();
        });

        it('should return null when volume path does not exist', () => {
            const service = new BootstrapService('/nonexistent/path');
            const result = service.loadCaCertPath();

            expect(result).toBeNull();
        });

        it('should continue to second path if first check fails', () => {
            const caDir = join(tmpVolumePath, 'ca');
            mkdirSync(caDir, { recursive: true });
            writeFileSync(join(caDir, 'ca.crt'), 'ca-cert-content');

            const result = bootstrapService.loadCaCertPath();

            expect(result).toBe(join(tmpVolumePath, 'ca', 'ca.crt'));
        });

        it('should return null and log warning when both paths fail', () => {
            mkdirSync(tmpVolumePath, { recursive: true });

            const result = bootstrapService.loadCaCertPath();

            expect(result).toBeNull();
        });
    });

    describe('getSslDir', () => {
        it('should return the volume path', () => {
            const customPath = '/custom/ssl/dir';
            const service = new BootstrapService(customPath);

            expect(service.getSslDir()).toBe(customPath);
        });

        it('should return default /g8es when no custom path set', () => {
            const service = new BootstrapService();

            expect(service.getSslDir()).toBe('/g8es');
        });
    });

    describe('isAvailable', () => {
        it('should return true when volume exists and contains token', () => {
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), 'test-token');

            expect(bootstrapService.isAvailable()).toBe(true);
        });

        it('should return true when volume exists and contains encryption key', () => {
            writeFileSync(join(tmpVolumePath, 'session_encryption_key'), 'a'.repeat(64));

            expect(bootstrapService.isAvailable()).toBe(true);
        });

        it('should return true when volume exists and contains CA cert', () => {
            const caDir = join(tmpVolumePath, 'ca');
            mkdirSync(caDir, { recursive: true });
            writeFileSync(join(caDir, 'ca.crt'), 'ca-cert');

            expect(bootstrapService.isAvailable()).toBe(true);
        });

        it('should return false when volume does not exist', () => {
            const service = new BootstrapService('/nonexistent/path');

            expect(service.isAvailable()).toBe(false);
        });

        it('should return false when volume exists but is empty', () => {
            mkdirSync(tmpVolumePath, { recursive: true });

            expect(bootstrapService.isAvailable()).toBe(false);
        });

        it('should return true when any bootstrap data is available', () => {
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), 'token');

            expect(bootstrapService.isAvailable()).toBe(true);
        });
    });

    describe('clearCache', () => {
        it('should clear all cached values', () => {
            const testToken = 'test-token';
            const testKey = 'a'.repeat(64);
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), testToken);
            writeFileSync(join(tmpVolumePath, 'session_encryption_key'), testKey);
            const caDir = join(tmpVolumePath, 'ca');
            mkdirSync(caDir, { recursive: true });
            writeFileSync(join(caDir, 'ca.crt'), 'ca-cert');

            bootstrapService.loadInternalAuthToken();
            bootstrapService.loadSessionEncryptionKey();
            bootstrapService.loadCaCertPath();

            expect(bootstrapService._cachedToken).toBe(testToken);
            expect(bootstrapService._cachedKey).toBe(testKey);
            expect(bootstrapService._cachedCaPath).not.toBeNull();

            bootstrapService.clearCache();

            expect(bootstrapService._cachedToken).toBeNull();
            expect(bootstrapService._cachedKey).toBeNull();
            expect(bootstrapService._cachedCaPath).toBeNull();
        });

        it('should allow reloading after cache clear', () => {
            const testToken = 'test-token';
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), testToken);

            bootstrapService.loadInternalAuthToken();
            bootstrapService.clearCache();
            const reloaded = bootstrapService.loadInternalAuthToken();

            expect(reloaded).toBe(testToken);
        });
    });

    describe('_safeListVolume', () => {
        it('should return volume contents when directory exists', () => {
            writeFileSync(join(tmpVolumePath, 'file1.txt'), 'content1');
            writeFileSync(join(tmpVolumePath, 'file2.txt'), 'content2');

            const result = bootstrapService._safeListVolume(tmpVolumePath);

            expect(result).toContain('file1.txt');
            expect(result).toContain('file2.txt');
        });

        it('should return empty directory message when directory is empty', () => {
            mkdirSync(tmpVolumePath, { recursive: true });

            const result = bootstrapService._safeListVolume(tmpVolumePath);

            expect(result).toEqual(['empty directory']);
        });

        it('should return error message when directory does not exist', () => {
            const result = bootstrapService._safeListVolume('/nonexistent/path');

            expect(result).toEqual(['volume does not exist']);
        });

        it('should return error message when read fails', () => {
            vi.spyOn(require('fs'), 'readdirSync').mockImplementationOnce(() => {
                throw new Error('Permission denied');
            });

            const result = bootstrapService._safeListVolume(tmpVolumePath);

            expect(result[0]).toContain('error reading directory');
        });
    });

    describe('integration scenarios', () => {
        it('should load all bootstrap data when all files present', () => {
            const testToken = 'test-token';
            const testKey = 'a'.repeat(64);
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), testToken);
            writeFileSync(join(tmpVolumePath, 'session_encryption_key'), testKey);
            const caDir = join(tmpVolumePath, 'ca');
            mkdirSync(caDir, { recursive: true });
            writeFileSync(join(caDir, 'ca.crt'), 'ca-cert');

            const token = bootstrapService.loadInternalAuthToken();
            const key = bootstrapService.loadSessionEncryptionKey();
            const caPath = bootstrapService.loadCaCertPath();

            expect(token).toBe(testToken);
            expect(key).toBe(testKey);
            expect(caPath).toBe(join(tmpVolumePath, 'ca', 'ca.crt'));
            expect(bootstrapService.isAvailable()).toBe(true);
        });

        it('should handle partial bootstrap data gracefully', () => {
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), 'token-only');

            const token = bootstrapService.loadInternalAuthToken();
            const key = bootstrapService.loadSessionEncryptionKey();
            const caPath = bootstrapService.loadCaCertPath();

            expect(token).toBe('token-only');
            expect(key).toBeNull();
            expect(caPath).toBeNull();
            expect(bootstrapService.isAvailable()).toBe(true);
        });

        it('should handle cache correctly across multiple service instances', () => {
            const testToken = 'shared-token';
            writeFileSync(join(tmpVolumePath, 'internal_auth_token'), testToken);

            const service1 = new BootstrapService(tmpVolumePath);
            const service2 = new BootstrapService(tmpVolumePath);

            const token1 = service1.loadInternalAuthToken();
            const token2 = service2.loadInternalAuthToken();

            expect(token1).toBe(testToken);
            expect(token2).toBe(testToken);
            expect(service1._cachedToken).toBe(testToken);
            expect(service2._cachedToken).toBe(testToken);
        });
    });
});
