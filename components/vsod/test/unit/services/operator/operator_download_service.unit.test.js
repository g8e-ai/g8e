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
import { OperatorDownloadService } from '@vsod/services/operator/operator_download_service.js';
import { PLATFORMS, OPERATOR_BINARY_BLOB_NAMESPACE } from '@vsod/constants/service_config.js';
import { HTTP_INTERNAL_AUTH_HEADER } from '@vsod/constants/headers.js';

describe('OperatorDownloadService', () => {
    let service;
    const listenUrl = 'https://vsodb:9000';
    const authToken = 'test-internal-token';

    beforeEach(() => {
        service = new OperatorDownloadService(listenUrl, authToken);
        global.fetch = vi.fn();
    });

    describe('constructor', () => {
        it('should throw if listenUrl is missing', () => {
            expect(() => new OperatorDownloadService()).toThrow('OperatorDownloadService requires listenUrl');
        });

        it('should strip trailing slash from listenUrl', () => {
            const svc = new OperatorDownloadService('http://host/');
            expect(svc._listenUrl).toBe('http://host');
        });

        it('should store internalAuthToken', () => {
            const svc = new OperatorDownloadService('http://host', 'tok');
            expect(svc._internalAuthToken).toBe('tok');
        });

        it('should default internalAuthToken to null', () => {
            const svc = new OperatorDownloadService('http://host');
            expect(svc._internalAuthToken).toBeNull();
        });
    });

    describe('getBinary', () => {
        it('should return a buffer on success', async () => {
            const mockBuffer = Buffer.from('fake-binary');
            const mockResponse = {
                ok: true,
                arrayBuffer: vi.fn().mockResolvedValue(new Uint8Array(mockBuffer).buffer),
                status: 200,
            };
            vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse));

            const result = await service.getBinary('linux', 'amd64');

            expect(result).toBeInstanceOf(Buffer);
            expect(result.toString()).toBe('fake-binary');
            expect(global.fetch).toHaveBeenCalledWith(
                `https://vsodb:9000/blob/${OPERATOR_BINARY_BLOB_NAMESPACE}/linux-amd64`,
                expect.objectContaining({
                    signal: expect.any(AbortSignal),
                    headers: expect.objectContaining({ [HTTP_INTERNAL_AUTH_HEADER]: authToken }),
                })
            );
        });

        it('should throw a specific error if response is not ok', async () => {
            global.fetch.mockResolvedValue({ ok: false, status: 404 });

            await expect(service.getBinary('linux', 'amd64')).rejects.toThrow(
                'Operator binary not available for platform: linux/amd64'
            );
        });

        it('should throw a specific error on fetch failure', async () => {
            global.fetch.mockRejectedValue(new Error('Network error'));

            await expect(service.getBinary('linux', 'amd64')).rejects.toThrow(
                'Operator binary not available for platform: linux/amd64'
            );
        });

        it('should not include auth header when no token provided', async () => {
            const noAuthService = new OperatorDownloadService(listenUrl);
            const mockResponse = {
                ok: true,
                arrayBuffer: vi.fn().mockResolvedValue(new Uint8Array(Buffer.from('x')).buffer),
                status: 200,
            };
            vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse));

            await noAuthService.getBinary('linux', 'amd64');

            const headers = global.fetch.mock.calls[0][1].headers;
            expect(headers).toEqual({});
        });
    });

    describe('hasBinary', () => {
        it('should return true if blob meta request is ok', async () => {
            global.fetch.mockResolvedValue({ ok: true });

            const result = await service.hasBinary('linux', 'amd64');

            expect(result).toBe(true);
            expect(global.fetch).toHaveBeenCalledWith(
                `https://vsodb:9000/blob/${OPERATOR_BINARY_BLOB_NAMESPACE}/linux-amd64/meta`,
                expect.objectContaining({
                    signal: expect.any(AbortSignal),
                    headers: expect.objectContaining({ [HTTP_INTERNAL_AUTH_HEADER]: authToken }),
                })
            );
        });

        it('should return false if blob meta request is not ok', async () => {
            global.fetch.mockResolvedValue({ ok: false });

            const result = await service.hasBinary('linux', 'amd64');

            expect(result).toBe(false);
        });

        it('should return false on fetch failure', async () => {
            global.fetch.mockRejectedValue(new Error('Abort'));

            const result = await service.hasBinary('linux', 'amd64');

            expect(result).toBe(false);
        });
    });

    describe('getPlatformAvailability', () => {
        it('should return availability for all defined platforms', async () => {
            // Mock hasBinary behavior via fetch
            global.fetch.mockResolvedValue({ ok: true });

            const result = await service.getPlatformAvailability();

            for (const { os, arch } of PLATFORMS) {
                expect(result[`${os}/${arch}`]).toEqual({ available: true });
            }
            expect(global.fetch).toHaveBeenCalledTimes(PLATFORMS.length);
        });
    });
});
