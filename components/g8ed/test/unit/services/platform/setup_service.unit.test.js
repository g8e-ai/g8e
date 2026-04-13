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
import { SetupService } from '@g8ed/services/platform/setup_service.js';

describe('SetupService [UNIT]', () => {
    let userService;
    let settingsService;
    let service;

    beforeEach(() => {
        userService = {
            createUser: vi.fn(),
            findUserByEmail: vi.fn()
        };
        settingsService = {
            getPlatformSettings: vi.fn(),
            savePlatformSettings: vi.fn(),
            updateUserSettings: vi.fn()
        };
        service = new SetupService({ userService, settingsService });
    });

    describe('isFirstRun', () => {
        it('returns true when platform_settings is null (fresh platform)', async () => {
            settingsService.getPlatformSettings.mockResolvedValue(null);
            expect(await service.isFirstRun()).toBe(true);
        });

        it('returns true when setup_complete is false', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({ setup_complete: false });
            expect(await service.isFirstRun()).toBe(true);
        });

        it('returns true when setup_complete is undefined', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({});
            expect(await service.isFirstRun()).toBe(true);
        });

        it('returns false when setup_complete is true', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({ setup_complete: true });
            expect(await service.isFirstRun()).toBe(false);
        });
    });

    describe('completeSetup', () => {
        it('persists setup_complete as boolean true', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({});
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });
            await service.completeSetup();
            expect(settingsService.savePlatformSettings).toHaveBeenCalledWith({ setup_complete: true });
        });

        it('preserves existing platform settings via spread', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({
                passkey_rp_id: 'my.host.com',
                passkey_origin: 'https://my.host.com'
            });
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            await service.completeSetup();

            const saved = settingsService.savePlatformSettings.mock.calls[0][0];
            expect(saved.passkey_rp_id).toBe('my.host.com');
            expect(saved.passkey_origin).toBe('https://my.host.com');
            expect(saved.setup_complete).toBe(true);
        });

        it('handles null existing settings without crashing', async () => {
            settingsService.getPlatformSettings.mockResolvedValue(null);
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            await service.completeSetup();

            const saved = settingsService.savePlatformSettings.mock.calls[0][0];
            expect(saved.setup_complete).toBe(true);
        });
    });

    describe('performFirstRunSetup', () => {
        const mockReq = {
            get: vi.fn(() => null),
            protocol: 'https',
            hostname: 'localhost'
        };

        it('creates a new user when none exists', async () => {
            const mockUser = { id: 'u1', email: 'admin@test.com' };
            userService.findUserByEmail.mockResolvedValue(null);
            userService.createUser.mockResolvedValue(mockUser);
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            const result = await service.performFirstRunSetup({
                email: 'admin@test.com',
                name: 'Admin',
                userSettings: { llm_primary_provider: 'gemini' },
                req: mockReq
            });

            expect(result).toBe(mockUser);
            expect(userService.findUserByEmail).toHaveBeenCalledWith('admin@test.com');
            expect(userService.createUser).toHaveBeenCalled();
            expect(settingsService.updateUserSettings).toHaveBeenCalledWith('u1', { llm_primary_provider: 'gemini' });
        });

        it('reuses existing user on retry (idempotent)', async () => {
            const existingUser = { id: 'u1', email: 'admin@test.com' };
            userService.findUserByEmail.mockResolvedValue(existingUser);
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            const result = await service.performFirstRunSetup({
                email: 'admin@test.com',
                name: 'Admin',
                userSettings: { llm_primary_provider: 'anthropic' },
                req: mockReq
            });

            expect(result).toBe(existingUser);
            expect(userService.findUserByEmail).toHaveBeenCalledWith('admin@test.com');
            expect(userService.createUser).not.toHaveBeenCalled();
            expect(settingsService.updateUserSettings).toHaveBeenCalledWith('u1', { llm_primary_provider: 'anthropic' });
        });

        it('saves platform settings with setup_complete false and derived passkey fields only', async () => {
            userService.findUserByEmail.mockResolvedValue(null);
            userService.createUser.mockResolvedValue({ id: 'u1' });
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            await service.performFirstRunSetup({
                email: 'a@b.com',
                name: 'A',
                req: mockReq
            });

            const savedSettings = settingsService.savePlatformSettings.mock.calls[0][0];
            expect(savedSettings.setup_complete).toBe(false);
            expect(savedSettings.passkey_rp_id).toBe('localhost');
            expect(savedSettings.passkey_origin).toBe('https://localhost');
            expect(savedSettings.app_url).toBeUndefined();
        });

        it('skips user settings when not provided', async () => {
            userService.findUserByEmail.mockResolvedValue(null);
            userService.createUser.mockResolvedValue({ id: 'u1' });
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            await service.performFirstRunSetup({
                email: 'a@b.com',
                name: 'A',
                req: mockReq
            });

            expect(settingsService.updateUserSettings).not.toHaveBeenCalled();
        });

        it('skips user settings when userSettings is null', async () => {
            userService.findUserByEmail.mockResolvedValue(null);
            userService.createUser.mockResolvedValue({ id: 'u1' });
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            await service.performFirstRunSetup({
                email: 'a@b.com',
                name: 'A',
                userSettings: null,
                req: mockReq
            });

            expect(settingsService.updateUserSettings).not.toHaveBeenCalled();
        });

        it('calls updateUserSettings with empty object when userSettings is {}', async () => {
            userService.findUserByEmail.mockResolvedValue(null);
            userService.createUser.mockResolvedValue({ id: 'u1' });
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            await service.performFirstRunSetup({
                email: 'a@b.com',
                name: 'A',
                userSettings: {},
                req: mockReq
            });

            expect(settingsService.updateUserSettings).toHaveBeenCalledWith('u1', {});
        });
    });

    describe('performFirstRunSetup concurrency', () => {
        it('serializes concurrent calls — second caller reuses the user created by the first', async () => {
            let callCount = 0;
            userService.findUserByEmail.mockImplementation(async () => {
                callCount++;
                if (callCount === 1) return null;       // first caller: no user yet
                return { id: 'u1', email: 'a@b.com' };  // second caller: user exists
            });
            userService.createUser.mockResolvedValue({ id: 'u1', email: 'a@b.com' });
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });

            const req = { get: vi.fn(() => null), protocol: 'https', hostname: 'localhost' };
            const args = { email: 'a@b.com', name: 'A', req };

            const [r1, r2] = await Promise.all([
                service.performFirstRunSetup(args),
                service.performFirstRunSetup(args)
            ]);

            expect(r1.id).toBe('u1');
            expect(r2.id).toBe('u1');
            expect(userService.createUser).toHaveBeenCalledTimes(1);
        });

        it('releases lock even when the first call throws', async () => {
            settingsService.savePlatformSettings.mockRejectedValueOnce(new Error('DB down'));
            settingsService.savePlatformSettings.mockResolvedValue({ success: true });
            userService.findUserByEmail.mockResolvedValue(null);
            userService.createUser.mockResolvedValue({ id: 'u1' });

            const req = { get: vi.fn(() => null), protocol: 'https', hostname: 'localhost' };
            const args = { email: 'a@b.com', name: 'A', req };

            await expect(service.performFirstRunSetup(args)).rejects.toThrow('DB down');

            const result = await service.performFirstRunSetup(args);
            expect(result.id).toBe('u1');
        });
    });

    describe('createAdminUser', () => {
        it('creates user with SUPERADMIN role', async () => {
            userService.createUser.mockResolvedValue({ id: 'u1', roles: ['superadmin'] });

            await service.createAdminUser({ email: 'a@b.com', name: 'Admin' });

            expect(userService.createUser).toHaveBeenCalledWith({
                email: 'a@b.com',
                name: 'Admin',
                roles: ['superadmin']
            });
        });
    });

    describe('derivePasskeyFields', () => {
        it('derives fields from standard headers', () => {
            const req = {
                get: vi.fn((name) => {
                    if (name === 'host') return 'g8e.local:443';
                    return null;
                }),
                protocol: 'https'
            };

            const fields = service.derivePasskeyFields(req);
            expect(fields.passkey_rp_id).toBe('g8e.local');
            expect(fields.passkey_origin).toBe('https://g8e.local:443');
        });

        it('prefers X-Forwarded headers over host/protocol', () => {
            const req = {
                get: vi.fn((name) => {
                    if (name === 'X-Forwarded-Host') return 'forwarded.com';
                    if (name === 'X-Forwarded-Proto') return 'https';
                    return 'something-else';
                })
            };

            const fields = service.derivePasskeyFields(req);
            expect(fields.passkey_rp_id).toBe('forwarded.com');
            expect(fields.passkey_origin).toBe('https://forwarded.com');
        });

        it('falls back to localhost when req is null', () => {
            const fields = service.derivePasskeyFields(null);
            expect(fields.passkey_rp_id).toBe('localhost');
            expect(fields.passkey_origin).toBe('https://localhost');
        });

        it('falls back to localhost when req has no get function', () => {
            const fields = service.derivePasskeyFields({});
            expect(fields.passkey_rp_id).toBe('localhost');
            expect(fields.passkey_origin).toBe('https://localhost');
        });

        it('strips port from rpId', () => {
            const req = {
                get: vi.fn(() => null),
                hostname: 'myhost.com',
                protocol: 'https'
            };
            const fields = service.derivePasskeyFields(req);
            expect(fields.passkey_rp_id).toBe('myhost.com');
        });

        it('uses req.hostname as fallback when host header is absent', () => {
            const req = {
                get: vi.fn(() => null),
                hostname: 'fallback.local',
                protocol: 'http'
            };
            const fields = service.derivePasskeyFields(req);
            expect(fields.passkey_rp_id).toBe('fallback.local');
            expect(fields.passkey_origin).toBe('http://fallback.local');
        });
    });
});
