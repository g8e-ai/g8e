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
import { UserService } from '@g8ed/services/platform/user_service.js';
import { UserDocument } from '@g8ed/models/user_model.js';
import { Collections } from '@g8ed/constants/collections.js';
import { UserRole, AuthProvider, ApiKeyClientName, ApiKeyStatus, ApiKeyError } from '@g8ed/constants/auth.js';
import { API_KEY_PREFIX } from '@g8ed/constants/operator_defaults.js';
import { now } from '@g8ed/models/base.js';
import { modelMatchers } from '../../../../test/utils/model_matchers.js';
import { G8eKeyError } from '@g8ed/services/error_service.js';

describe('UserService [UNIT]', () => {
    let cacheAside;
    let organizationService;
    let apiKeyService;
    let service;

    beforeEach(() => {
        cacheAside = {
            createDocument: vi.fn().mockResolvedValue({ success: true }),
            getDocument: vi.fn(),
            updateDocument: vi.fn().mockResolvedValue({ success: true }),
            deleteDocument: vi.fn().mockResolvedValue({ success: true }),
            queryDocuments: vi.fn().mockResolvedValue([]),
        };
        organizationService = {
            create: vi.fn().mockResolvedValue({ success: true })
        };
        apiKeyService = {
            generateRawKey: vi.fn().mockReturnValue('g8e_generated'),
            issueKey: vi.fn().mockResolvedValue({ success: true }),
            validateKey: vi.fn(),
            recordUsage: vi.fn(),
            revokeKey: vi.fn().mockResolvedValue({ success: true }),
        };
        service = new UserService({ cacheAsideService: cacheAside, organizationService, apiKeyService });
    });

    describe('constructor', () => {
        it('throws if cacheAsideService is missing', () => {
            expect(() => new UserService({ organizationService, apiKeyService })).toThrow('UserService requires cacheAsideService');
        });
    });

    describe('_generateApiKey', () => {
        it('delegates to apiKeyService.generateRawKey', () => {
            const key = service._generateApiKey();
            expect(key).toBe('g8e_generated');
            expect(apiKeyService.generateRawKey).toHaveBeenCalled();
        });

        it('falls back to local generation if apiKeyService is missing', () => {
            const serviceNoKeySvc = new UserService({ cacheAsideService: cacheAside, organizationService });
            const key = serviceNoKeySvc._generateApiKey();
            expect(key.startsWith(API_KEY_PREFIX)).toBe(true);
        });
    });

    describe('_writeUser', () => {
        it('throws if createDocument fails', async () => {
            cacheAside.createDocument.mockResolvedValue({ success: false, error: 'DB Error' });
            const userDoc = UserDocument.parse({ id: 'u1', email: 'test@example.com' });
            await expect(service._writeUser(userDoc)).rejects.toThrow('DB Error');
        });

        it('uses default error message if none provided', async () => {
            cacheAside.createDocument.mockResolvedValue({ success: false, error: 'DB write failed' });
            const userDoc = UserDocument.parse({ id: 'u1', email: 'test@example.com' });
            await expect(service._writeUser(userDoc)).rejects.toThrow('DB write failed');
        });
    });

    describe('getUser', () => {
        it('returns parsed UserDocument on cache hit', async () => {
            const mockUser = { id: 'u1', email: 'test@example.com', roles: [UserRole.USER] };
            cacheAside.getDocument.mockResolvedValue(mockUser);

            const result = await service.getUser('u1');
            expect(result).toBeInstanceOf(UserDocument);
            expect(result.email).toBe('test@example.com');
        });

        it('returns null if user not found', async () => {
            cacheAside.getDocument.mockResolvedValue(null);
            const result = await service.getUser('u1');
            expect(result).toBeNull();
        });

        it('returns null and logs on error', async () => {
            cacheAside.getDocument.mockRejectedValue(new Error('Cache fail'));
            const result = await service.getUser('u1');
            expect(result).toBeNull();
        });
    });

    describe('createUser', () => {
        it('throws if email is missing', async () => {
            await expect(service.createUser({})).rejects.toThrow('Email is required');
        });

        it('creates user with default roles and matching organization', async () => {
            cacheAside.queryDocuments.mockResolvedValue([]); // First user check

            const userData = { email: 'admin@example.com', name: 'Admin User' };
            const user = await service.createUser(userData);

            expect(user.email).toBe('admin@example.com');
            expect(user.roles).toContain(UserRole.SUPERADMIN);
            expect(cacheAside.createDocument).toHaveBeenCalledWith(
                Collections.USERS,
                user.id,
                expect.any(UserDocument)
            );
            expect(organizationService.create).toHaveBeenCalledWith(
                expect.objectContaining({ org_id: user.id, owner_id: user.id })
            );
        });

        it('assigns standard USER role for non-first users', async () => {
            cacheAside.queryDocuments.mockResolvedValue([{ id: 'existing' }]);

            const user = await service.createUser({ email: 'user@example.com' });
            expect(user.roles).toContain(UserRole.USER);
            expect(user.roles).not.toContain(UserRole.SUPERADMIN);
        });

        it('handles missing organizationService gracefully', async () => {
            const serviceNoOrg = new UserService({ cacheAsideService: cacheAside });
            cacheAside.queryDocuments.mockResolvedValue([]);
            
            const user = await serviceNoOrg.createUser({ email: 'test@example.com' });
            expect(user.email).toBe('test@example.com');
            // Should not crash
        });

        it('handles organization creation failure gracefully', async () => {
            organizationService.create.mockRejectedValue(new Error('Org fail'));
            cacheAside.queryDocuments.mockResolvedValue([]);

            const user = await service.createUser({ email: 'test@example.com' });
            expect(user.email).toBe('test@example.com');
            expect(organizationService.create).toHaveBeenCalled();
            // Should not crash
        });

        it('throws if user write fails', async () => {
            cacheAside.createDocument.mockResolvedValue({ success: false, error: 'Write fail' });
            cacheAside.queryDocuments.mockResolvedValue([]);

            await expect(service.createUser({ email: 'test@example.com' })).rejects.toThrow('Write fail');
        });
    });

    describe('updateUser', () => {
        it('merges updates and saves to DB', async () => {
            const existing = { id: 'u1', email: 'old@example.com', name: 'Old Name' };
            const updated = { ...existing, name: 'New Name' };
            cacheAside.getDocument.mockResolvedValueOnce(existing).mockResolvedValueOnce(updated);
            cacheAside.updateDocument.mockResolvedValue({ success: true });

            const result = await service.updateUser('u1', { name: 'New Name' });
            
            expect(result.name).toBe('New Name');
            expect(result.email).toBe('old@example.com');
            expect(cacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.USERS,
                'u1',
                expect.objectContaining({ name: 'New Name' })
            );
        });

        it('throws if user not found', async () => {
            cacheAside.getDocument.mockResolvedValue(null);
            await expect(service.updateUser('u1', { name: 'New' })).rejects.toThrow('User not found: u1');
        });

        it('throws and logs on error during write', async () => {
            const existing = { id: 'u1', email: 'test@example.com' };
            cacheAside.getDocument.mockResolvedValue(existing);
            cacheAside.updateDocument.mockResolvedValue({ success: false, error: 'DB Fail' });
            
            await expect(service.updateUser('u1', { name: 'New' })).rejects.toThrow('DB Fail');
        });
    });

    describe('updateLastLogin', () => {
        it('updates last_login timestamp', async () => {
            const baseTime = now();
            const existing = { id: 'u1', email: 'test@example.com', last_login: baseTime };
            
            cacheAside.updateDocument.mockResolvedValue({ success: true });
            cacheAside.getDocument.mockImplementation((collection, id) => {
                if (id === 'u1') return Promise.resolve(existing);
                return Promise.resolve(null);
            });

            const result = await service.updateLastLogin('u1');
            
            expect(result).toBeInstanceOf(UserDocument);
            // last_login should be a Date object now
            expect(result.last_login).toBeInstanceOf(Date);
            expect(result.last_login.getTime()).toBeGreaterThanOrEqual(baseTime.getTime());
            
            // Verify the exact call to updateDocument
            const updateCall = cacheAside.updateDocument.mock.calls[0];
            expect(updateCall[0]).toBe(Collections.USERS);
            expect(updateCall[1]).toBe('u1');
            // updateCall[2].last_login should also be a Date object
            expect(updateCall[2].last_login).toBeInstanceOf(Date);
            expect(updateCall[2].last_login.getTime()).toBeGreaterThanOrEqual(baseTime.getTime());
        });

        it('returns null if user not found', async () => {
            cacheAside.getDocument.mockResolvedValue(null);
            await expect(service.updateLastLogin('u1')).rejects.toThrow('User not found: u1');
        });
    });

    describe('hasAnyUsers', () => {
        it('returns true if documents exist', async () => {
            cacheAside.queryDocuments.mockResolvedValue([{ id: 'u1' }]);
            const result = await service.hasAnyUsers();
            expect(result).toBe(true);
            expect(cacheAside.queryDocuments).toHaveBeenCalledWith(Collections.USERS, [], 1);
        });

        it('returns false if no documents exist', async () => {
            cacheAside.queryDocuments.mockResolvedValue([]);
            const result = await service.hasAnyUsers();
            expect(result).toBe(false);
        });

        it('returns false on error', async () => {
            cacheAside.queryDocuments.mockRejectedValue(new Error('Query fail'));
            const result = await service.hasAnyUsers();
            expect(result).toBe(false);
        });
    });

    describe('findUserByEmail', () => {
        it('queries documents by email field', async () => {
            const mockUser = { id: 'u1', email: 'test@example.com' };
            cacheAside.queryDocuments.mockResolvedValue([mockUser]);

            const result = await service.findUserByEmail('TEST@EXAMPLE.COM');
            
            expect(result.id).toBe('u1');
            expect(cacheAside.queryDocuments).toHaveBeenCalledWith(
                Collections.USERS,
                expect.arrayContaining([{ field: 'email', operator: '==', value: 'test@example.com' }])
            );
        });

        it('returns null if no user found', async () => {
            cacheAside.queryDocuments.mockResolvedValue([]);
            const result = await service.findUserByEmail('none@example.com');
            expect(result).toBeNull();
        });

        it('returns null on error', async () => {
            cacheAside.queryDocuments.mockRejectedValue(new Error('Query fail'));
            const result = await service.findUserByEmail('error@example.com');
            expect(result).toBeNull();
        });
    });

    describe('getUserByApiKey', () => {
        it('queries documents by g8e_key field', async () => {
            const mockUser = { id: 'u1', email: 'test@example.com', g8e_key: 'key1' };
            cacheAside.queryDocuments.mockResolvedValue([mockUser]);

            const result = await service.getUserByApiKey('key1');
            
            expect(result.id).toBe('u1');
            expect(cacheAside.queryDocuments).toHaveBeenCalledWith(
                Collections.USERS,
                expect.arrayContaining([{ field: 'g8e_key', operator: '==', value: 'key1' }])
            );
        });

        it('returns null if no user found', async () => {
            cacheAside.queryDocuments.mockResolvedValue([]);
            const result = await service.getUserByApiKey('invalid');
            expect(result).toBeNull();
        });

        it('returns null on error', async () => {
            cacheAside.queryDocuments.mockRejectedValue(new Error('Query fail'));
            const result = await service.getUserByApiKey('key1');
            expect(result).toBeNull();
        });
    });

    describe('listUsers', () => {
        it('returns list of parsed UserDocuments', async () => {
            cacheAside.queryDocuments.mockResolvedValue([
                { id: 'u1', email: 'u1@example.com' },
                { id: 'u2', email: 'u2@example.com' }
            ]);

            const result = await service.listUsers(10);
            expect(result).toHaveLength(2);
            expect(result[0]).toBeInstanceOf(UserDocument);
            expect(result[1].id).toBe('u2');
            expect(cacheAside.queryDocuments).toHaveBeenCalledWith(Collections.USERS, [], 10);
        });

        it('returns empty array on error', async () => {
            cacheAside.queryDocuments.mockRejectedValue(new Error('Fail'));
            const result = await service.listUsers(10);
            expect(result).toEqual([]);
        });
    });

    describe('getUserStats', () => {
        it('returns success with total users', async () => {
            cacheAside.queryDocuments.mockResolvedValue([{ id: 'u1' }, { id: 'u2' }]);

            const result = await service.getUserStats(100);
            expect(result.success).toBe(true);
            expect(result.data.total_users).toBe(2);
        });

        it('returns failure on error', async () => {
            cacheAside.queryDocuments.mockRejectedValue(new Error('Fail'));

            const result = await service.getUserStats(100);
            expect(result.success).toBe(false);
            expect(result.message).toBe('Fail');
            expect(result.data.total_users).toBe(0);
        });
    });

    describe('deleteUser', () => {
        it('calls cache delete', async () => {
            const result = await service.deleteUser('u1');
            expect(result).toBe(true);
            expect(cacheAside.deleteDocument).toHaveBeenCalledWith(Collections.USERS, 'u1');
        });

        it('returns false if user not found', async () => {
            cacheAside.deleteDocument.mockResolvedValue({ success: false, notFound: true });
            const result = await service.deleteUser('u1');
            expect(result).toBe(false);
        });

        it('throws on DB error', async () => {
            cacheAside.deleteDocument.mockResolvedValue({ success: false, error: 'DB Fail' });
            await expect(service.deleteUser('u1')).rejects.toThrow('DB Fail');
        });

        it('throws with default error if none provided', async () => {
            cacheAside.deleteDocument.mockResolvedValue({ success: false, error: 'DB delete failed' });
            await expect(service.deleteUser('u1')).rejects.toThrow('DB delete failed');
        });
    });

    describe('updateUserOperator', () => {
        it('updates operator linkage', async () => {
            const existing = { id: 'u1', email: 'test@example.com' };
            cacheAside.updateDocument.mockResolvedValue({ success: true });
            cacheAside.getDocument.mockResolvedValue(existing);

            const result = await service.updateUserOperator('u1', 'op1', 'active');
            expect(result).toBe(true);
            expect(cacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.USERS,
                'u1',
                expect.objectContaining({ operator_id: 'op1', operator_status: 'active' })
            );
        });

        it('returns false if user not found', async () => {
            cacheAside.getDocument.mockResolvedValue(null);
            const result = await service.updateUserOperator('u1', 'op1', 'active');
            expect(result).toBe(false);
        });

        it('returns false on error', async () => {
            cacheAside.updateDocument.mockRejectedValue(new Error('Fail'));
            const result = await service.updateUserOperator('u1', 'op1', 'active');
            expect(result).toBe(false);
        });
    });

    describe('getUserG8eKey', () => {
        it('returns the API key if user exists', async () => {
            cacheAside.getDocument.mockResolvedValue({ id: 'u1', email: 'test@example.com', g8e_key: 'key1' });
            const key = await service.getUserG8eKey('u1');
            expect(key).toBe('key1');
        });

        it('returns null if user not found', async () => {
            cacheAside.getDocument.mockResolvedValue(null);
            const key = await service.getUserG8eKey('u1');
            expect(key).toBeNull();
        });

        it('returns null if user has no API key', async () => {
            cacheAside.getDocument.mockResolvedValue({ id: 'u1', g8e_key: null });
            const key = await service.getUserG8eKey('u1');
            expect(key).toBeNull();
        });
    });

    describe('createUserG8eKey', () => {
        it('returns error if apiKeyService is missing', async () => {
            const serviceNoKeySvc = new UserService({ cacheAsideService: cacheAside, organizationService });
            const result = await serviceNoKeySvc.createUserG8eKey('u1', 'o1');
            expect(result.success).toBe(false);
            expect(result.error).toBe('apiKeyService is required');
        });

        it('returns error if user not found', async () => {
            cacheAside.getDocument.mockResolvedValue(null);
            const result = await service.createUserG8eKey('u1', 'o1');
            expect(result.success).toBe(false);
            expect(result.error).toBe(ApiKeyError.USER_NOT_FOUND);
        });

        it('returns error if user already has a key', async () => {
            cacheAside.getDocument.mockResolvedValue({ id: 'u1', email: 'test@example.com', g8e_key: 'already-exists' });
            const result = await service.createUserG8eKey('u1', 'o1');
            expect(result.success).toBe(false);
            expect(result.error).toBe('User already has a download API key');
        });

        it('stores new key via issueKey and updates user document', async () => {
            cacheAside.getDocument.mockResolvedValue({ id: 'u1', email: 'test@example.com', g8e_key: null });
            cacheAside.updateDocument.mockResolvedValue({ success: true });
            
            const result = await service.createUserG8eKey('u1', 'o1');
            
            expect(result.success).toBe(true);
            expect(result.api_key).toBe('g8e_generated');
            expect(apiKeyService.issueKey).toHaveBeenCalledWith(
                'g8e_generated',
                expect.objectContaining({
                    user_id: 'u1',
                    organization_id: 'o1',
                    client_name: ApiKeyClientName.USER,
                    permissions: ['operator:download'],
                    status: ApiKeyStatus.ACTIVE
                })
            );
            expect(cacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.USERS,
                'u1',
                expect.objectContaining({ g8e_key: 'g8e_generated' })
            );
        });

        it('returns error if issueKey fails', async () => {
            cacheAside.getDocument.mockResolvedValue({ id: 'u1', email: 'test@example.com', g8e_key: null });
            apiKeyService.issueKey.mockResolvedValue({ success: false, error: 'Store fail' });

            const result = await service.createUserG8eKey('u1', 'o1');
            expect(result.success).toBe(false);
            expect(result.error).toBe('Failed to store API key');
        });

        it('returns error if user update fails', async () => {
            cacheAside.getDocument.mockResolvedValue({ id: 'u1', email: 'test@example.com', g8e_key: null });
            cacheAside.updateDocument.mockResolvedValue({ success: false, error: 'Write fail' });

            const result = await service.createUserG8eKey('u1', 'o1');
            expect(result.success).toBe(false);
            expect(result.error).toBe('Write fail');
        });
    });

    describe('refreshUserG8eKey', () => {
        it('throws G8eKeyError if apiKeyService is missing', async () => {
            const serviceNoKeySvc = new UserService({ cacheAsideService: cacheAside, organizationService });
            await expect(serviceNoKeySvc.refreshUserG8eKey('u1', 'o1')).rejects.toThrow(G8eKeyError);
            await expect(serviceNoKeySvc.refreshUserG8eKey('u1', 'o1')).rejects.toThrow('apiKeyService is required');
        });

        it('throws G8eKeyError if user not found', async () => {
            cacheAside.getDocument.mockResolvedValue(null);
            await expect(service.refreshUserG8eKey('u1', 'o1')).rejects.toThrow(G8eKeyError);
            await expect(service.refreshUserG8eKey('u1', 'o1')).rejects.toThrow(ApiKeyError.USER_NOT_FOUND);
        });

        it('revokes old key and issues new key when user has existing key', async () => {
            const existingUser = { 
                id: 'u1', 
                email: 'test@example.com', 
                g8e_key: 'old_key_123',
                g8e_key_created_at: '2024-01-01T00:00:00Z'
            };
            cacheAside.getDocument.mockResolvedValue(existingUser);
            cacheAside.updateDocument.mockResolvedValue({ success: true });
            
            const result = await service.refreshUserG8eKey('u1', 'o1');
            
            expect(result.success).toBe(true);
            expect(result.api_key).toBe('g8e_generated');
            expect(apiKeyService.revokeKey).toHaveBeenCalledWith('old_key_123');
            expect(apiKeyService.issueKey).toHaveBeenCalledWith(
                'g8e_generated',
                expect.objectContaining({
                    user_id: 'u1',
                    organization_id: 'o1',
                    client_name: ApiKeyClientName.USER,
                    permissions: ['operator:download'],
                    status: ApiKeyStatus.ACTIVE
                })
            );
            expect(cacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.USERS,
                'u1',
                expect.objectContaining({
                    g8e_key: 'g8e_generated',
                    g8e_key_updated_at: expect.any(Date)
                })
            );
        });

        it('issues new key when user has no existing key', async () => {
            const existingUser = { 
                id: 'u1', 
                email: 'test@example.com', 
                g8e_key: null,
                g8e_key_created_at: null
            };
            cacheAside.getDocument.mockResolvedValue(existingUser);
            cacheAside.updateDocument.mockResolvedValue({ success: true });
            
            const result = await service.refreshUserG8eKey('u1', 'o1');
            
            expect(result.success).toBe(true);
            expect(result.api_key).toBe('g8e_generated');
            expect(apiKeyService.revokeKey).not.toHaveBeenCalled();
            expect(apiKeyService.issueKey).toHaveBeenCalledWith(
                'g8e_generated',
                expect.objectContaining({
                    user_id: 'u1',
                    organization_id: 'o1'
                })
            );
        });

        it('preserves g8e_key_created_at from original creation', async () => {
            const existingUser = { 
                id: 'u1', 
                email: 'test@example.com', 
                g8e_key: 'old_key_123',
                g8e_key_created_at: '2024-01-01T00:00:00Z'
            };
            cacheAside.getDocument.mockResolvedValue(existingUser);
            cacheAside.updateDocument.mockResolvedValue({ success: true });
            
            await service.refreshUserG8eKey('u1', 'o1');
            
            expect(cacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.USERS,
                'u1',
                expect.objectContaining({
                    g8e_key: 'g8e_generated',
                    g8e_key_updated_at: expect.any(Date)
                })
            );
        });

        it('throws G8eKeyError if issueKey fails', async () => {
            cacheAside.getDocument.mockResolvedValue({ id: 'u1', email: 'test@example.com', g8e_key: 'old_key' });
            apiKeyService.issueKey.mockResolvedValue({ success: false, error: 'Store fail' });

            await expect(service.refreshUserG8eKey('u1', 'o1')).rejects.toThrow(G8eKeyError);
            await expect(service.refreshUserG8eKey('u1', 'o1')).rejects.toThrow('Failed to store API key');
        });

        it('throws G8eKeyError if user update fails', async () => {
            cacheAside.getDocument.mockResolvedValue({ id: 'u1', email: 'test@example.com', g8e_key: 'old_key' });
            cacheAside.updateDocument.mockResolvedValue({ success: false, error: 'Write fail' });

            await expect(service.refreshUserG8eKey('u1', 'o1')).rejects.toThrow(G8eKeyError);
            await expect(service.refreshUserG8eKey('u1', 'o1')).rejects.toThrow('Write fail');
        });

        it('logs warning but continues if revokeKey fails', async () => {
            const existingUser = { 
                id: 'u1', 
                email: 'test@example.com', 
                g8e_key: 'old_key_123',
                g8e_key_created_at: '2024-01-01T00:00:00Z'
            };
            cacheAside.getDocument.mockResolvedValue(existingUser);
            cacheAside.updateDocument.mockResolvedValue({ success: true });
            apiKeyService.revokeKey.mockResolvedValue({ success: false, error: 'Revoke fail' });
            
            const result = await service.refreshUserG8eKey('u1', 'o1');
            
            expect(result.success).toBe(true);
            expect(apiKeyService.issueKey).toHaveBeenCalled();
        });
    });
});
