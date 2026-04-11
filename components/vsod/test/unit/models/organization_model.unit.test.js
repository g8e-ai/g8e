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
import { OrgAdmin, TeamMember, OrganizationDocument, OrganizationModel } from '@vsod/models/organization_model.js';
import { Collections } from '@vsod/constants/collections.js';
import { VSOBaseModel, VSOIdentifiableModel, F, now } from '@vsod/models/base.js';

describe('OrgAdmin [UNIT]', () => {
    describe('required fields', () => {
        it('throws when user_id is missing', () => {
            expect(() => OrgAdmin.parse({ email: 'admin@example.com' })).toThrow();
        });

        it('parses with only required field', () => {
            const admin = OrgAdmin.parse({ user_id: 'user-001' });
            expect(admin.user_id).toBe('user-001');
        });
    });

    describe('defaults', () => {
        it('defaults email to null', () => {
            expect(OrgAdmin.parse({ user_id: 'user-001' }).email).toBeNull();
        });

        it('defaults name to null', () => {
            expect(OrgAdmin.parse({ user_id: 'user-001' }).name).toBeNull();
        });
    });

    describe('field assignment', () => {
        it('assigns all provided fields', () => {
            const admin = OrgAdmin.parse({
                user_id: 'user-001',
                email: 'admin@example.com',
                name: 'Admin User',
            });
            expect(admin.user_id).toBe('user-001');
            expect(admin.email).toBe('admin@example.com');
            expect(admin.name).toBe('Admin User');
        });
    });

    describe('forDB / forWire round-trip', () => {
        it('round-trips through forDB and parse', () => {
            const original = OrgAdmin.parse({
                user_id: 'user-001',
                email: 'admin@example.com',
                name: 'Admin User',
            });

            const restored = OrgAdmin.parse(original.forDB());

            expect(restored.user_id).toBe(original.user_id);
            expect(restored.email).toBe(original.email);
            expect(restored.name).toBe(original.name);
        });
    });
});

describe('TeamMember [UNIT]', () => {
    describe('required fields', () => {
        it('throws when user_id is missing', () => {
            expect(() => TeamMember.parse({ email: 'member@example.com' })).toThrow();
        });

        it('parses with only required field', () => {
            const member = TeamMember.parse({ user_id: 'user-002' });
            expect(member.user_id).toBe('user-002');
        });
    });

    describe('defaults', () => {
        it('defaults email to null', () => {
            expect(TeamMember.parse({ user_id: 'user-002' }).email).toBeNull();
        });

        it('defaults name to null', () => {
            expect(TeamMember.parse({ user_id: 'user-002' }).name).toBeNull();
        });

        it('defaults role to member', () => {
            expect(TeamMember.parse({ user_id: 'user-002' }).role).toBe('member');
        });

        it('defaults invited_by to null', () => {
            expect(TeamMember.parse({ user_id: 'user-002' }).invited_by).toBeNull();
        });

        it('defaults joined_at to current time', () => {
            const before = Date.now();
            const member = TeamMember.parse({ user_id: 'user-002' });
            const after = Date.now();
            expect(member.joined_at).toBeInstanceOf(Date);
            expect(member.joined_at.getTime()).toBeGreaterThanOrEqual(before);
            expect(member.joined_at.getTime()).toBeLessThanOrEqual(after);
        });
    });

    describe('field assignment', () => {
        it('assigns all provided fields', () => {
            const joinedAt = new Date('2026-01-01T00:00:00.000Z');
            const member = TeamMember.parse({
                user_id: 'user-002',
                email: 'member@example.com',
                name: 'Team Member',
                role: 'admin',
                invited_by: 'user-001',
                joined_at: joinedAt,
            });
            expect(member.user_id).toBe('user-002');
            expect(member.email).toBe('member@example.com');
            expect(member.name).toBe('Team Member');
            expect(member.role).toBe('admin');
            expect(member.invited_by).toBe('user-001');
            expect(member.joined_at).toBe(joinedAt);
        });
    });

    describe('forDB / forWire round-trip', () => {
        it('round-trips through forDB and parse', () => {
            const joinedAt = new Date('2026-01-01T00:00:00.000Z');
            const original = TeamMember.parse({
                user_id: 'user-002',
                email: 'member@example.com',
                name: 'Team Member',
                role: 'admin',
                joined_at: joinedAt,
            });

            const restored = TeamMember.parse(original.forDB());

            expect(restored.user_id).toBe(original.user_id);
            expect(restored.email).toBe(original.email);
            expect(restored.name).toBe(original.name);
            expect(restored.role).toBe(original.role);
            expect(restored.joined_at.toISOString()).toBe(original.joined_at.toISOString());
        });
    });
});

describe('OrganizationDocument [UNIT]', () => {
    function makeBaseOrg(overrides = {}) {
        return {
            id: 'org-001',
            name: 'Test Organization',
            owner_id: 'user-001',
            ...overrides,
        };
    }

    describe('required fields', () => {
        it('throws when name is missing', () => {
            expect(() => OrganizationDocument.parse({ id: 'org-001', owner_id: 'user-001' })).toThrow();
        });

        it('throws when owner_id is missing', () => {
            expect(() => OrganizationDocument.parse({ id: 'org-001', name: 'Test Org' })).toThrow();
        });

        it('parses with only required fields', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg());
            expect(doc.id).toBe('org-001');
            expect(doc.name).toBe('Test Organization');
            expect(doc.owner_id).toBe('user-001');
        });
    });

    describe('defaults', () => {
        it('defaults org_admin to null', () => {
            expect(OrganizationDocument.parse(makeBaseOrg()).org_admin).toBeNull();
        });

        it('defaults team_members to empty array', () => {
            expect(OrganizationDocument.parse(makeBaseOrg()).team_members).toEqual([]);
        });

        it('defaults total_invites_sent to 0', () => {
            expect(OrganizationDocument.parse(makeBaseOrg()).total_invites_sent).toBe(0);
        });

        it('defaults created_at from VSOIdentifiableModel', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg());
            expect(doc.created_at).toBeInstanceOf(Date);
        });

        it('defaults updated_at from VSOIdentifiableModel', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg());
            expect(doc.updated_at).toBeNull();
        });
    });

    describe('field assignment', () => {
        it('assigns all provided fields', () => {
            const orgAdmin = new OrgAdmin({ user_id: 'user-001', email: 'admin@example.com', name: 'Admin' });
            const teamMember = new TeamMember({ user_id: 'user-002', email: 'member@example.com', name: 'Member' });
            const doc = OrganizationDocument.parse({
                ...makeBaseOrg(),
                org_admin: orgAdmin,
                team_members: [teamMember],
                total_invites_sent: 5,
            });
            expect(doc.org_admin).toBeInstanceOf(OrgAdmin);
            expect(doc.org_admin.user_id).toBe('user-001');
            expect(doc.team_members).toHaveLength(1);
            expect(doc.team_members[0]).toBeInstanceOf(TeamMember);
            expect(doc.total_invites_sent).toBe(5);
        });
    });

    describe('org_id getter', () => {
        it('returns id', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg({ id: 'org-123' }));
            expect(doc.org_id).toBe('org-123');
        });
    });

    describe('stats getter', () => {
        it('calculates total_members as 1 (owner) when no team members', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg());
            expect(doc.stats.total_members).toBe(1);
        });

        it('calculates total_members as 1 + team_members length', () => {
            const member1 = new TeamMember({ user_id: 'user-002' });
            const member2 = new TeamMember({ user_id: 'user-003' });
            const doc = OrganizationDocument.parse({
                ...makeBaseOrg(),
                team_members: [member1, member2],
            });
            expect(doc.stats.total_members).toBe(3);
        });

        it('returns total_invites_sent from document', () => {
            const doc = OrganizationDocument.parse({ ...makeBaseOrg(), total_invites_sent: 10 });
            expect(doc.stats.total_invites_sent).toBe(10);
        });

        it('handles null team_members gracefully', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg());
            doc.team_members = null;
            expect(doc.stats.total_members).toBe(1);
        });
    });

    describe('forClient', () => {
        it('adds org_id field', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg({ id: 'org-123' }));
            const client = doc.forClient();
            expect(client.org_id).toBe('org-123');
        });

        it('adds stats field with total_members', () => {
            const member = new TeamMember({ user_id: 'user-002' });
            const doc = OrganizationDocument.parse({ ...makeBaseOrg(), team_members: [member] });
            const client = doc.forClient();
            expect(client.stats.total_members).toBe(2);
        });

        it('adds stats field with total_invites_sent', () => {
            const doc = OrganizationDocument.parse({ ...makeBaseOrg(), total_invites_sent: 7 });
            const client = doc.forClient();
            expect(client.stats.total_invites_sent).toBe(7);
        });

        it('handles null team_members in stats calculation', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg());
            doc.team_members = null;
            const client = doc.forClient();
            expect(client.stats.total_members).toBe(1);
        });

        it('does not mutate the original document', () => {
            const doc = OrganizationDocument.parse(makeBaseOrg());
            const originalStats = doc.stats;
            doc.forClient();
            expect(doc.stats).toEqual(originalStats);
        });
    });

    describe('forDB / forWire round-trip', () => {
        it('round-trips through forDB and parse', () => {
            const orgAdmin = new OrgAdmin({ user_id: 'user-001', email: 'admin@example.com' });
            const teamMember = new TeamMember({ user_id: 'user-002', email: 'member@example.com' });
            const original = OrganizationDocument.parse({
                ...makeBaseOrg(),
                org_admin: orgAdmin,
                team_members: [teamMember],
                total_invites_sent: 3,
            });

            const restored = OrganizationDocument.parse(original.forDB());

            expect(restored.id).toBe(original.id);
            expect(restored.name).toBe(original.name);
            expect(restored.owner_id).toBe(original.owner_id);
            expect(restored.org_admin.user_id).toBe(original.org_admin.user_id);
            expect(restored.team_members).toHaveLength(1);
            expect(restored.team_members[0].user_id).toBe(original.team_members[0].user_id);
            expect(restored.total_invites_sent).toBe(original.total_invites_sent);
        });
    });
});

describe('OrganizationModel [UNIT]', () => {
    let mockCacheAside;
    let orgModel;

    beforeEach(() => {
        mockCacheAside = {
            getDocument: vi.fn(),
            createDocument: vi.fn(),
            queryDocuments: vi.fn(),
            updateDocument: vi.fn(),
        };
        orgModel = new OrganizationModel({ cacheAsideService: mockCacheAside });
    });

    describe('constructor', () => {
        it('throws when cacheAsideService is not provided', () => {
            expect(() => new OrganizationModel({ cacheAsideService: null })).toThrow('OrganizationModel requires a cacheAsideService instance');
        });

        it('throws when cacheAsideService is undefined', () => {
            expect(() => new OrganizationModel({})).toThrow('OrganizationModel requires a cacheAsideService instance');
        });

        it('initializes with cacheAsideService', () => {
            const model = new OrganizationModel({ cacheAsideService: mockCacheAside });
            expect(model._cacheAside).toBe(mockCacheAside);
        });
    });

    describe('getById', () => {
        it('returns parsed organization document when found', async () => {
            const orgData = {
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                org_admin: null,
                team_members: [],
                total_invites_sent: 0,
                created_at: new Date().toISOString(),
                updated_at: null,
            };
            mockCacheAside.getDocument.mockResolvedValue(orgData);

            const result = await orgModel.getById('org-001');

            expect(mockCacheAside.getDocument).toHaveBeenCalledWith(Collections.ORGANIZATIONS, 'org-001');
            expect(result).toBeInstanceOf(OrganizationDocument);
            expect(result.id).toBe('org-001');
            expect(result.name).toBe('Test Org');
        });

        it('returns null when organization not found', async () => {
            mockCacheAside.getDocument.mockResolvedValue(null);

            const result = await orgModel.getById('org-001');

            expect(result).toBeNull();
        });

        it('throws and logs error on cache failure', async () => {
            const error = new Error('Cache error');
            mockCacheAside.getDocument.mockRejectedValue(error);

            await expect(orgModel.getById('org-001')).rejects.toThrow('Cache error');
            expect(mockCacheAside.getDocument).toHaveBeenCalledWith(Collections.ORGANIZATIONS, 'org-001');
        });
    });

    describe('create', () => {
        it('creates organization with all parameters', async () => {
            mockCacheAside.createDocument.mockResolvedValue({ success: true });

            const result = await orgModel.create({
                org_id: 'org-001',
                owner_id: 'user-001',
                name: 'Test Organization',
                admin_user_id: 'admin-001',
                admin_email: 'admin@example.com',
                admin_name: 'Admin User',
            });

            expect(mockCacheAside.createDocument).toHaveBeenCalledWith(
                Collections.ORGANIZATIONS,
                'org-001',
                expect.objectContaining({
                    id: 'org-001',
                    name: 'Test Organization',
                    owner_id: 'user-001',
                    org_admin: expect.objectContaining({
                        user_id: 'admin-001',
                        email: 'admin@example.com',
                        name: 'Admin User',
                    }),
                    team_members: [],
                    total_invites_sent: 0,
                })
            );
            expect(result).toBeInstanceOf(OrganizationDocument);
        });

        it('uses admin_user_id as owner_id when owner_id not provided', async () => {
            mockCacheAside.createDocument.mockResolvedValue({ success: true });

            await orgModel.create({
                org_id: 'org-001',
                admin_user_id: 'admin-001',
                admin_name: 'Admin',
            });

            const callArgs = mockCacheAside.createDocument.mock.calls[0];
            expect(callArgs[2].owner_id).toBe('admin-001');
        });

        it('generates default name from admin_name when name not provided', async () => {
            mockCacheAside.createDocument.mockResolvedValue({ success: true });

            await orgModel.create({
                org_id: 'org-001',
                admin_user_id: 'admin-001',
                admin_name: 'John Doe',
            });

            const callArgs = mockCacheAside.createDocument.mock.calls[0];
            expect(callArgs[2].name).toBe("John Doe's Organization");
        });

        it('generates default name as "User\'s Organization" when neither name nor admin_name provided', async () => {
            mockCacheAside.createDocument.mockResolvedValue({ success: true });

            await orgModel.create({
                org_id: 'org-001',
                admin_user_id: 'admin-001',
            });

            const callArgs = mockCacheAside.createDocument.mock.calls[0];
            expect(callArgs[2].name).toBe("User's Organization");
        });

        it('sets org_admin to null when admin_user_id not provided', async () => {
            mockCacheAside.createDocument.mockResolvedValue({ success: true });

            await orgModel.create({
                org_id: 'org-001',
                owner_id: 'user-001',
                name: 'Test Org',
            });

            const callArgs = mockCacheAside.createDocument.mock.calls[0];
            expect(callArgs[2].org_admin).toBeNull();
        });

        it('throws when createDocument fails', async () => {
            mockCacheAside.createDocument.mockResolvedValue({ success: false, error: 'Creation failed' });

            await expect(orgModel.create({
                org_id: 'org-001',
                owner_id: 'user-001',
                name: 'Test Org',
            })).rejects.toThrow('Creation failed');
        });

        it('throws and logs error on cache failure', async () => {
            const error = new Error('Cache error');
            mockCacheAside.createDocument.mockRejectedValue(error);

            await expect(orgModel.create({
                org_id: 'org-001',
                owner_id: 'user-001',
                name: 'Test Org',
            })).rejects.toThrow('Cache error');
        });
    });

    describe('getByAdminUserId', () => {
        it('returns organization when found by owner_id', async () => {
            const orgData = [{
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                org_admin: null,
                team_members: [],
                total_invites_sent: 0,
                created_at: new Date().toISOString(),
                updated_at: null,
            }];
            mockCacheAside.queryDocuments.mockResolvedValue(orgData);

            const result = await orgModel.getByAdminUserId('user-001');

            expect(mockCacheAside.queryDocuments).toHaveBeenCalledWith(
                Collections.ORGANIZATIONS,
                [{ field: 'owner_id', operator: '==', value: 'user-001' }],
                1
            );
            expect(result).toBeInstanceOf(OrganizationDocument);
            expect(result.owner_id).toBe('user-001');
        });

        it('returns null when no organization found', async () => {
            mockCacheAside.queryDocuments.mockResolvedValue([]);

            const result = await orgModel.getByAdminUserId('user-001');

            expect(result).toBeNull();
        });

        it('returns null when query returns empty array', async () => {
            mockCacheAside.queryDocuments.mockResolvedValue([]);

            const result = await orgModel.getByAdminUserId('user-001');

            expect(result).toBeNull();
        });

        it('throws and logs error on cache failure', async () => {
            const error = new Error('Query error');
            mockCacheAside.queryDocuments.mockRejectedValue(error);

            await expect(orgModel.getByAdminUserId('user-001')).rejects.toThrow('Query error');
        });
    });

    describe('addTeamMember', () => {
        it('adds team member to organization', async () => {
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                team_members: [],
                total_invites_sent: 0,
                created_at: new Date(),
            });
            const newMember = new TeamMember({
                user_id: 'user-002',
                email: 'member@example.com',
                name: 'Team Member',
                role: 'member',
                invited_by: 'user-001',
            });
            const updatedOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                team_members: [newMember],
                total_invites_sent: 0,
                created_at: new Date(),
                updated_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValueOnce(existingOrg.forDB()).mockResolvedValueOnce(updatedOrg.forDB());
            mockCacheAside.updateDocument.mockResolvedValue();

            const result = await orgModel.addTeamMember('org-001', {
                user_id: 'user-002',
                email: 'member@example.com',
                name: 'Team Member',
                role: 'member',
                invited_by: 'user-001',
            });

            expect(mockCacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.ORGANIZATIONS,
                'org-001',
                expect.objectContaining({
                    team_members: expect.arrayContaining([
                        expect.objectContaining({
                            user_id: 'user-002',
                            email: 'member@example.com',
                            name: 'Team Member',
                            role: 'member',
                            invited_by: 'user-001',
                        }),
                    ]),
                })
            );
            expect(result).toBeInstanceOf(OrganizationDocument);
        });

        it('defaults role to member when not provided', async () => {
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                team_members: [],
                total_invites_sent: 0,
                created_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValue(existingOrg.forDB());
            mockCacheAside.getDocument.mockResolvedValueOnce(existingOrg.forDB()).mockResolvedValueOnce(existingOrg.forDB());
            mockCacheAside.updateDocument.mockResolvedValue();

            await orgModel.addTeamMember('org-001', {
                user_id: 'user-002',
            });

            const callArgs = mockCacheAside.updateDocument.mock.calls[0];
            expect(callArgs[2].team_members[0].role).toBe('member');
        });

        it('does not add duplicate member', async () => {
            const existingMember = new TeamMember({ user_id: 'user-002', email: 'member@example.com' });
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                team_members: [existingMember],
                total_invites_sent: 0,
                created_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValue(existingOrg.forDB());
            mockCacheAside.getDocument.mockResolvedValueOnce(existingOrg.forDB()).mockResolvedValueOnce(existingOrg.forDB());

            const result = await orgModel.addTeamMember('org-001', {
                user_id: 'user-002',
            });

            expect(mockCacheAside.updateDocument).not.toHaveBeenCalled();
            expect(result.team_members).toHaveLength(1);
        });

        it('throws when organization not found', async () => {
            mockCacheAside.getDocument.mockResolvedValue(null);

            await expect(orgModel.addTeamMember('org-001', {
                user_id: 'user-002',
            })).rejects.toThrow('Organization not found');
        });

        it('throws and logs error on cache failure', async () => {
            const error = new Error('Cache error');
            mockCacheAside.getDocument.mockRejectedValue(error);

            await expect(orgModel.addTeamMember('org-001', {
                user_id: 'user-002',
            })).rejects.toThrow('Cache error');
        });
    });

    describe('removeTeamMember', () => {
        it('removes team member from organization', async () => {
            const member1 = new TeamMember({ user_id: 'user-002', email: 'member1@example.com' });
            const member2 = new TeamMember({ user_id: 'user-003', email: 'member2@example.com' });
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                team_members: [member1, member2],
                total_invites_sent: 0,
                created_at: new Date(),
            });
            const updatedOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                team_members: [member2],
                total_invites_sent: 0,
                created_at: new Date(),
                updated_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValueOnce(existingOrg.forDB()).mockResolvedValueOnce(updatedOrg.forDB());
            mockCacheAside.updateDocument.mockResolvedValue();

            const result = await orgModel.removeTeamMember('org-001', 'user-002');

            expect(mockCacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.ORGANIZATIONS,
                'org-001',
                expect.objectContaining({
                    team_members: expect.arrayContaining([
                        expect.objectContaining({ user_id: 'user-003' }),
                    ]),
                })
            );
            expect(result.team_members).toHaveLength(1);
        });

        it('throws when organization not found', async () => {
            mockCacheAside.getDocument.mockResolvedValue(null);

            await expect(orgModel.removeTeamMember('org-001', 'user-002')).rejects.toThrow('Organization not found');
        });

        it('throws and logs error on cache failure', async () => {
            const error = new Error('Cache error');
            mockCacheAside.getDocument.mockRejectedValue(error);

            await expect(orgModel.removeTeamMember('org-001', 'user-002')).rejects.toThrow('Cache error');
        });
    });

    describe('getTeamMembers', () => {
        it('returns empty array when organization not found', async () => {
            mockCacheAside.getDocument.mockResolvedValue(null);

            const result = await orgModel.getTeamMembers('org-001');

            expect(result).toEqual([]);
        });

        it('returns admin and team members', async () => {
            const orgAdmin = new OrgAdmin({ user_id: 'user-001', email: 'admin@example.com', name: 'Admin' });
            const member1 = new TeamMember({ user_id: 'user-002', email: 'member1@example.com', name: 'Member 1', role: 'member' });
            const member2 = new TeamMember({ user_id: 'user-003', email: 'member2@example.com', name: 'Member 2', role: 'admin' });
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                org_admin: orgAdmin,
                team_members: [member1, member2],
                total_invites_sent: 0,
                created_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValue(existingOrg.forDB());

            const result = await orgModel.getTeamMembers('org-001');

            expect(result).toHaveLength(3);
            expect(result[0]).toEqual({
                user_id: 'user-001',
                email: 'admin@example.com',
                name: 'Admin',
                role: 'admin',
                is_admin: true,
            });
            expect(result[1]).toEqual({
                user_id: 'user-002',
                email: 'member1@example.com',
                name: 'Member 1',
                role: 'member',
                is_admin: false,
            });
            expect(result[2]).toEqual({
                user_id: 'user-003',
                email: 'member2@example.com',
                name: 'Member 2',
                role: 'admin',
                is_admin: false,
            });
        });

        it('returns only admin when no team members', async () => {
            const orgAdmin = new OrgAdmin({ user_id: 'user-001', email: 'admin@example.com', name: 'Admin' });
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                org_admin: orgAdmin,
                team_members: [],
                total_invites_sent: 0,
                created_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValue(existingOrg.forDB());

            const result = await orgModel.getTeamMembers('org-001');

            expect(result).toHaveLength(1);
            expect(result[0].is_admin).toBe(true);
        });

        it('returns only team members when no admin', async () => {
            const member1 = new TeamMember({ user_id: 'user-002', email: 'member1@example.com', name: 'Member 1' });
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                org_admin: null,
                team_members: [member1],
                total_invites_sent: 0,
                created_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValue(existingOrg.forDB());

            const result = await orgModel.getTeamMembers('org-001');

            expect(result).toHaveLength(1);
            expect(result[0].is_admin).toBe(false);
        });

        it('defaults team member role to member when null', async () => {
            const member = new TeamMember({ user_id: 'user-002', email: 'member@example.com', name: 'Member', role: null });
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                org_admin: null,
                team_members: [member],
                total_invites_sent: 0,
                created_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValue(existingOrg.forDB());

            const result = await orgModel.getTeamMembers('org-001');

            expect(result[0].role).toBe('member');
        });

        it('throws and logs error on cache failure', async () => {
            const error = new Error('Cache error');
            mockCacheAside.getDocument.mockRejectedValue(error);

            await expect(orgModel.getTeamMembers('org-001')).rejects.toThrow('Cache error');
        });
    });

    describe('incrementInvitesSent', () => {
        it('increments total_invites_sent', async () => {
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                team_members: [],
                total_invites_sent: 5,
                created_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValue(existingOrg.forDB());
            mockCacheAside.updateDocument.mockResolvedValue();

            await orgModel.incrementInvitesSent('org-001');

            expect(mockCacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.ORGANIZATIONS,
                'org-001',
                expect.objectContaining({
                    total_invites_sent: 6,
                })
            );
        });

        it('increments from 0 when total_invites_sent is null', async () => {
            const existingOrg = new OrganizationDocument({
                id: 'org-001',
                name: 'Test Org',
                owner_id: 'user-001',
                team_members: [],
                total_invites_sent: null,
                created_at: new Date(),
            });
            mockCacheAside.getDocument.mockResolvedValue(existingOrg.forDB());
            mockCacheAside.updateDocument.mockResolvedValue();

            await orgModel.incrementInvitesSent('org-001');

            const callArgs = mockCacheAside.updateDocument.mock.calls[0];
            expect(callArgs[2].total_invites_sent).toBe(1);
        });

        it('throws when organization not found', async () => {
            mockCacheAside.getDocument.mockResolvedValue(null);

            await expect(orgModel.incrementInvitesSent('org-001')).rejects.toThrow('Organization not found');
        });

        it('throws and logs error on cache failure', async () => {
            const error = new Error('Cache error');
            mockCacheAside.getDocument.mockRejectedValue(error);

            await expect(orgModel.incrementInvitesSent('org-001')).rejects.toThrow('Cache error');
        });
    });

    describe('updateName', () => {
        it('updates organization name', async () => {
            mockCacheAside.updateDocument.mockResolvedValue();

            await orgModel.updateName('org-001', 'New Organization Name');

            expect(mockCacheAside.updateDocument).toHaveBeenCalledWith(
                Collections.ORGANIZATIONS,
                'org-001',
                expect.objectContaining({
                    name: 'New Organization Name',
                })
            );
        });

        it('throws and logs error on cache failure', async () => {
            const error = new Error('Cache error');
            mockCacheAside.updateDocument.mockRejectedValue(error);

            await expect(orgModel.updateName('org-001', 'New Name')).rejects.toThrow('Cache error');
        });
    });
});
