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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { DownloadAuthService } from '@g8ed/services/auth/download_auth_service.js';
import { ApiKeyError, DownloadKeyType, DeviceLinkStatus } from '@g8ed/constants/auth.js';
import { KVKey } from '@g8ed/constants/kv_keys.js';
import { DeviceLinkData } from '@g8ed/models/auth_models.js';
import { mockUsers } from '@test/fixtures/users.fixture.js';
import { createKVMock } from '@test/mocks/kv.mock.js';

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: {
        debug: vi.fn(),
        info: vi.fn(),
        warn: vi.fn(),
        error: vi.fn(),
    },
}));

const VALID_DLK = 'dlk_AbCdEfGhIjKlMnOpQrStUvWxYz123456';
const RAW_KEY   = 'g8e_test_api_key_' + '0'.repeat(20);

function makeReq(authHeader) {
    return {
        headers: { authorization: authHeader, 'user-agent': 'test-agent' },
        ip: '127.0.0.1',
    };
}

describe('DownloadAuthService [UNIT TEST]', () => {
    let kvMock;
    let cacheAside;
    let userService;
    let apiKeyService;
    let svc;

    beforeEach(() => {
        kvMock = createKVMock();
        kvMock.clear();

        cacheAside = {
            kvGetJson:      (key) => kvMock.get_json(key),
            kvDel:          (...keys) => kvMock.del(...keys),
            createDocument: vi.fn().mockResolvedValue({ success: true }),
        };

        userService = {
            getUserByApiKey: vi.fn().mockResolvedValue(null),
        };

        apiKeyService = {
            validateKey: vi.fn(),
        };

        svc = new DownloadAuthService({ cacheAsideService: cacheAside, userService, apiKeyService });
    });

    describe('constructor', () => {
        it('throws when cacheAsideService is missing', () => {
            expect(() => new DownloadAuthService({ userService, apiKeyService }))
                .toThrow('cacheAsideService is required');
        });

        it('throws when userService is missing', () => {
            expect(() => new DownloadAuthService({ cacheAsideService: cacheAside, apiKeyService }))
                .toThrow('userService is required');
        });

        it('throws when apiKeyService is missing', () => {
            expect(() => new DownloadAuthService({ cacheAsideService: cacheAside, userService }))
                .toThrow('apiKeyService is required');
        });
    });

    describe('missing / malformed Authorization header', () => {
        it('returns 401 when Authorization header is absent', async () => {
            const result = await svc.validate(makeReq(undefined));
            expect(result).toMatchObject({ success: false, status: 401, error: expect.stringContaining('Authentication required') });
        });

        it('returns 401 when Authorization is not Bearer scheme', async () => {
            const result = await svc.validate(makeReq('Basic dXNlcjpwYXNz'));
            expect(result).toMatchObject({ success: false, status: 401, error: expect.stringContaining('Authentication required') });
        });
    });

    describe('dlk_ (device-link token) branch', () => {
        it('returns 401 for malformed dlk_ format (too short)', async () => {
            const result = await svc.validate(makeReq('Bearer dlk_tooshort'));
            expect(result).toMatchObject({ success: false, status: 401, error: ApiKeyError.AUTH_FAILED });
        });

        it('returns 401 when dlk_ not found in KV', async () => {
            const result = await svc.validate(makeReq(`Bearer ${VALID_DLK}`));
            expect(result).toMatchObject({ success: false, status: 401, error: ApiKeyError.AUTH_FAILED });
        });

        it('returns success and does NOT delete token', async () => {
            const linkData = new DeviceLinkData({
                token:           VALID_DLK,
                user_id:         mockUsers.primary.id,
                operator_id:     mockUsers.primary.operator_id,
                organization_id: mockUsers.primary.organization_id,
                status:          DeviceLinkStatus.ACTIVE,
            });
            await kvMock.set_json(KVKey.deviceLink(VALID_DLK), linkData.forKV());

            const result = await svc.validate(makeReq(`Bearer ${VALID_DLK}`));

            expect(result).toMatchObject({
                success:        true,
                userId:         mockUsers.primary.id,
                organizationId: mockUsers.primary.organization_id,
                operatorId:     mockUsers.primary.operator_id,
                keyType:        DownloadKeyType.DEVICE_LINK,
            });
            expect(await kvMock.get_json(KVKey.deviceLink(VALID_DLK))).not.toBeNull();
        });

        it('rejects dlk_ when allowDlt is false', async () => {
            const result = await svc.validate(makeReq(`Bearer ${VALID_DLK}`), { allowDlt: false });
            expect(result).toMatchObject({
                success: false,
                status:  403,
                error:   expect.stringContaining('not permitted'),
            });
        });

        it('returns success for multi-use dlk_ with no operator_id', async () => {
            const linkData = new DeviceLinkData({
                token:           VALID_DLK,
                user_id:         mockUsers.primary.id,
                organization_id: mockUsers.primary.organization_id,
                status:          DeviceLinkStatus.ACTIVE,
            });
            await kvMock.set_json(KVKey.deviceLink(VALID_DLK), linkData.forKV());

            const result = await svc.validate(makeReq(`Bearer ${VALID_DLK}`));

            expect(result.success).toBe(true);
            expect(result.operatorId).toBeFalsy();
            expect(result.keyType).toBe(DownloadKeyType.DEVICE_LINK);
            expect(await kvMock.get_json(KVKey.deviceLink(VALID_DLK))).not.toBeNull();
        });
    });

    describe('g8e_key (user document) branch', () => {
        it('returns success with USER_DOWNLOAD when user has matching g8e_key', async () => {
            userService.getUserByApiKey.mockResolvedValue({
                id:              mockUsers.primary.id,
                organization_id: mockUsers.primary.organization_id,
            });

            const result = await svc.validate(makeReq(`Bearer ${RAW_KEY}`));

            expect(result).toMatchObject({
                success:        true,
                userId:         mockUsers.primary.id,
                organizationId: mockUsers.primary.organization_id,
                operatorId:     null,
                keyType:        DownloadKeyType.USER_DOWNLOAD,
            });
            expect(userService.getUserByApiKey).toHaveBeenCalledWith(RAW_KEY);
            expect(apiKeyService.validateKey).not.toHaveBeenCalled();
        });
    });

    describe('operator API key branch', () => {
        it('returns 401 when apiKeyService returns failure', async () => {
            apiKeyService.validateKey.mockResolvedValue({ success: false, data: null });

            const result = await svc.validate(makeReq(`Bearer ${RAW_KEY}`));
            expect(result).toMatchObject({ success: false, status: 401, error: ApiKeyError.INVALID_OR_EXPIRED });
            expect(apiKeyService.validateKey).toHaveBeenCalledWith(RAW_KEY);
        });

        it('returns 403 when key has no operator_id and no operator:download permission', async () => {
            apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: {
                    user_id:         mockUsers.primary.id,
                    organization_id: mockUsers.primary.organization_id,
                    operator_id:     null,
                    permissions:     ['some:other:permission'],
                },
            });

            const result = await svc.validate(makeReq(`Bearer ${RAW_KEY}`));
            expect(result).toMatchObject({ success: false, status: 403, error: ApiKeyError.NO_DOWNLOAD_PERMISSION });
        });

        it('returns success with OPERATOR_SPECIFIC when key has operator_id', async () => {
            apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: {
                    user_id:         mockUsers.primary.id,
                    organization_id: mockUsers.primary.organization_id,
                    operator_id:     mockUsers.primary.operator_id,
                    permissions:     [],
                },
            });

            const result = await svc.validate(makeReq(`Bearer ${RAW_KEY}`));
            expect(result).toMatchObject({
                success:        true,
                userId:         mockUsers.primary.id,
                organizationId: mockUsers.primary.organization_id,
                operatorId:     mockUsers.primary.operator_id,
                keyType:        DownloadKeyType.OPERATOR_SPECIFIC,
            });
        });

        it('returns success with USER_DOWNLOAD when key has operator:download permission and no operator_id', async () => {
            apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: {
                    user_id:         mockUsers.primary.id,
                    organization_id: mockUsers.primary.organization_id,
                    operator_id:     null,
                    permissions:     ['operator:download'],
                },
            });

            const result = await svc.validate(makeReq(`Bearer ${RAW_KEY}`));
            expect(result).toMatchObject({ success: true, keyType: DownloadKeyType.USER_DOWNLOAD });
            expect(result.operatorId).toBeNull();
        });

        it('operator_id takes precedence — OPERATOR_SPECIFIC even when operator:download is also present', async () => {
            apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: {
                    user_id:         mockUsers.primary.id,
                    organization_id: mockUsers.primary.organization_id,
                    operator_id:     mockUsers.primary.operator_id,
                    permissions:     ['operator:download'],
                },
            });

            const result = await svc.validate(makeReq(`Bearer ${RAW_KEY}`));
            expect(result).toMatchObject({ success: true, keyType: DownloadKeyType.OPERATOR_SPECIFIC });
        });
    });
});
