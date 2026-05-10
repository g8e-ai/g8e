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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { OperatorDownloadService } from '@g8ed/services/operator/operator_download_service.js';
import { PLATFORMS } from '@g8ed/constants/service_config.js';
import fs from 'fs';
import path from 'path';

// Mock fs module
vi.mock('fs');

describe('OperatorDownloadService', () => {
    let service;

    beforeEach(() => {
        vi.clearAllMocks();
        service = new OperatorDownloadService();
    });

    describe('constructor', () => {
        it('should initialize with project root and build directory', () => {
            expect(service._projectRoot).toBeDefined();
            expect(service._buildDir).toContain('components');
            expect(service._buildDir).toContain('g8eo');
            expect(service._buildDir).toContain('build');
        });
    });

    describe('_binaryPath', () => {
        it('should construct correct binary path for platform', () => {
            const binaryPath = service._binaryPath('linux', 'amd64');
            expect(binaryPath).toContain('linux-amd64');
            expect(binaryPath).toContain('g8e.operator');
            expect(binaryPath).toMatch(/components[\/\\]g8eo[\/\\]build[\/\\]linux-amd64[\/\\]g8e\.operator$/);
        });
    });

    describe('getBinary', () => {
        it('should return a buffer on success', async () => {
            const mockBuffer = Buffer.from('fake-binary');
            vi.mocked(fs.existsSync).mockReturnValue(true);
            vi.mocked(fs.readFileSync).mockReturnValue(mockBuffer);

            const result = await service.getBinary('linux', 'amd64');

            expect(result).toBeInstanceOf(Buffer);
            expect(result.toString()).toBe('fake-binary');
            expect(fs.existsSync).toHaveBeenCalled();
            expect(fs.readFileSync).toHaveBeenCalled();
        });

        it('should throw a specific error if binary does not exist', async () => {
            vi.mocked(fs.existsSync).mockReturnValue(false);

            await expect(service.getBinary('linux', 'amd64')).rejects.toThrow(
                'Operator binary not available for platform: linux/amd64'
            );
        });

        it('should throw a specific error on read failure', async () => {
            vi.mocked(fs.existsSync).mockReturnValue(true);
            vi.mocked(fs.readFileSync).mockImplementation(() => {
                throw new Error('Read error');
            });

            await expect(service.getBinary('linux', 'amd64')).rejects.toThrow(
                'Operator binary not available for platform: linux/amd64'
            );
        });
    });

    describe('hasBinary', () => {
        it('should return true if binary exists', async () => {
            vi.mocked(fs.existsSync).mockReturnValue(true);

            const result = await service.hasBinary('linux', 'amd64');

            expect(result).toBe(true);
            expect(fs.existsSync).toHaveBeenCalled();
        });

        it('should return false if binary does not exist', async () => {
            vi.mocked(fs.existsSync).mockReturnValue(false);

            const result = await service.hasBinary('linux', 'amd64');

            expect(result).toBe(false);
        });

        it('should return false on fs error', async () => {
            vi.mocked(fs.existsSync).mockImplementation(() => {
                throw new Error('FS error');
            });

            const result = await service.hasBinary('linux', 'amd64');

            expect(result).toBe(false);
        });
    });

    describe('getPlatformAvailability', () => {
        it('should return availability for all defined platforms', async () => {
            vi.mocked(fs.existsSync).mockReturnValue(true);

            const result = await service.getPlatformAvailability();

            for (const { os, arch } of PLATFORMS) {
                expect(result[`${os}/${arch}`]).toEqual({ available: true });
            }
            expect(fs.existsSync).toHaveBeenCalledTimes(PLATFORMS.length);
        });

        it('should return false for missing platforms', async () => {
            vi.mocked(fs.existsSync).mockReturnValue(false);

            const result = await service.getPlatformAvailability();

            for (const { os, arch } of PLATFORMS) {
                expect(result[`${os}/${arch}`]).toEqual({ available: false });
            }
        });
    });
});
