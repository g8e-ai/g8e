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
import { DeviceRegistrationService } from '@g8ed/services/auth/device_registration_service.js';
import { DeviceLinkError } from '@g8ed/constants/auth.js';
import { createMockInternalHttpClient } from '@test/mocks/internal-http-client.mock.js';

describe('DeviceRegistrationService', () => {
    let service;
    let internalHttpClient;

    const token = 'dlk_' + 'a'.repeat(32);

    const mockDeviceInfo = {
        system_fingerprint: 'ABC123DEF456',
        hostname: 'test-host',
        os: 'linux',
        arch: 'x64',
        username: 'test-user',
        ip_address: '192.168.1.100',
        csr_pem: '-----BEGIN CERTIFICATE REQUEST-----\nmock\n-----END CERTIFICATE REQUEST-----'
    };

    beforeEach(() => {
        vi.clearAllMocks();
        internalHttpClient = createMockInternalHttpClient();
        service = new DeviceRegistrationService({ internalHttpClient });
    });

    describe('registerDevice', () => {
        it('should return failure if fingerprint is missing', async () => {
            const result = await service.registerDevice({
                deviceInfo: {},
                device_link_token: token
            });
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MISSING_FINGERPRINT);
            expect(internalHttpClient.registerDeviceLink).not.toHaveBeenCalled();
        });

        it('should return failure if token is missing', async () => {
            const result = await service.registerDevice({
                deviceInfo: mockDeviceInfo
            });
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.INVALID_TOKEN_FORMAT);
            expect(internalHttpClient.registerDeviceLink).not.toHaveBeenCalled();
        });

        it('should successfully register a device through the Operator substrate', async () => {
            internalHttpClient.registerDeviceLink.mockResolvedValue({
                success: true,
                operator_session_id: 'op-sess-999',
                operator_id: 'op-1',
                api_key: 'g8e_deadbeef_' + 'a'.repeat(64),
                operator_cert_pem: 'cert',
                session: { id: 'op-sess-999' },
                config: { endpoint: 'https://localhost:9000' }
            });

            const result = await service.registerDevice({
                operator_id: 'op-1',
                deviceInfo: mockDeviceInfo,
                device_link_token: token
            });

            expect(result.success).toBe(true);
            expect(result.operator_session_id).toBe('op-sess-999');
            expect(result.operator_id).toBe('op-1');
            expect(result.operator_cert).toBe('cert');
            expect(result.config).toEqual({ endpoint: 'https://localhost:9000' });
            expect(internalHttpClient.registerDeviceLink).toHaveBeenCalledWith(token, {
                csr_pem: mockDeviceInfo.csr_pem,
                system_fingerprint: 'abc123def456',
                hostname: 'test-host',
                os: 'linux',
                arch: 'x64',
                username: 'test-user',
                ip_address: '192.168.1.100',
            });
        });

        it('should return substrate registration failures directly', async () => {
            internalHttpClient.registerDeviceLink.mockResolvedValue({
                success: false,
                error: 'CSR required for operator enrollment'
            });

            const result = await service.registerDevice({
                deviceInfo: mockDeviceInfo,
                device_link_token: token
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe('CSR required for operator enrollment');
        });
    });
});
