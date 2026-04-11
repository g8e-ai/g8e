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

import { logger } from '../utils/logger.js';
import { Collections } from '../constants/collections.js';
import { VSOBaseModel, VSOIdentifiableModel, F, now } from './base.js';

const COLLECTION_NAME = Collections.ORGANIZATIONS;

// ---------------------------------------------------------------------------
// OrgAdmin
// ---------------------------------------------------------------------------

export class OrgAdmin extends VSOBaseModel {
    static fields = {
        user_id: { type: F.string, required: true },
        email:   { type: F.string, default: null },
        name:    { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// TeamMember
// ---------------------------------------------------------------------------

export class TeamMember extends VSOBaseModel {
    static fields = {
        user_id:    { type: F.string, required: true },
        email:      { type: F.string, default: null },
        name:       { type: F.string, default: null },
        role:       { type: F.string, default: 'member' },
        invited_by: { type: F.string, default: null },
        joined_at:  { type: F.date,   default: () => now() },
    };
}

// ---------------------------------------------------------------------------
// OrganizationDocument  (persistent document stored in VSODB)
// ---------------------------------------------------------------------------

export class OrganizationDocument extends VSOIdentifiableModel {
    static fields = {
        name:               { type: F.string, required: true },
        owner_id:           { type: F.string, required: true },
        org_admin:          { type: F.object, model: OrgAdmin,    default: null },
        team_members:       { type: F.array,  items: TeamMember,  default: () => [] },
        total_invites_sent: { type: F.number, default: 0 },
    };

    get org_id() {
        return this.id;
    }

    get stats() {
        return {
            total_members:      1 + (this.team_members ? this.team_members.length : 0),
            total_invites_sent: this.total_invites_sent,
        };
    }

    forClient() {
        const obj = this.forDB();
        obj.org_id = obj.id;
        obj.stats = {
            total_members:      1 + (Array.isArray(obj.team_members) ? obj.team_members.length : 0),
            total_invites_sent: obj.total_invites_sent,
        };
        return obj;
    }
}

// ---------------------------------------------------------------------------
// OrganizationModel  (data-access service — instantiated once at startup)
// ---------------------------------------------------------------------------

export class OrganizationModel {
    constructor({ cacheAsideService }) {
        if (!cacheAsideService) {
            throw new Error('OrganizationModel requires a cacheAsideService instance');
        }
        this._cacheAside = cacheAsideService;
        logger.info('[ORG-MODEL] Initialized');
    }

    async getById(orgId) {
        try {
            const data = await this._cacheAside.getDocument(COLLECTION_NAME, orgId);
            if (data) {
                return OrganizationDocument.parse(data);
            }
            return null;
        } catch (error) {
            logger.error('[ORG-MODEL] Failed to get organization by ID', {
                error: error.message,
                orgId,
            });
            throw error;
        }
    }

    async create({ org_id, owner_id, name, admin_user_id, admin_email, admin_name }) {
        try {
            const effectiveOwnerId = owner_id || admin_user_id;
            const effectiveName = name || `${admin_name || 'User'}'s Organization`;

            const orgAdmin = admin_user_id
                ? new OrgAdmin({ user_id: admin_user_id, email: admin_email || null, name: admin_name || null })
                : null;

            const doc = new OrganizationDocument({
                id:                 org_id,
                name:               effectiveName,
                owner_id:           effectiveOwnerId,
                org_admin:          orgAdmin,
                team_members:       [],
                total_invites_sent: 0,
                created_at:         now(),
                updated_at:         now(),
            });

            const result = await this._cacheAside.createDocument(COLLECTION_NAME, org_id, doc);

            if (!result.success) {
                throw new Error(result.error || 'Failed to create organization');
            }

            logger.info('[ORG-MODEL] Organization created', { org_id, owner_id: effectiveOwnerId });
            return doc;
        } catch (error) {
            logger.error('[ORG-MODEL] Failed to create organization', {
                error: error.message,
                org_id,
            });
            throw error;
        }
    }

    async getByAdminUserId(adminUserId) {
        try {
            const results = await this._cacheAside.queryDocuments(COLLECTION_NAME, [
                { field: 'owner_id', operator: '==', value: adminUserId }
            ], 1);

            if (results && results.length > 0) {
                return OrganizationDocument.parse(results[0]);
            }

            return null;
        } catch (error) {
            logger.error('[ORG-MODEL] Failed to get organization by admin user ID', {
                error: error.message,
                adminUserId,
            });
            throw error;
        }
    }

    async addTeamMember(orgId, { user_id, email, name, invited_by, role }) {
        try {
            const org = await this.getById(orgId);
            if (!org) {
                throw new Error('Organization not found');
            }

            const existing = org.team_members.some(m => m.user_id === user_id);
            if (existing) {
                return org;
            }

            const member = new TeamMember({
                user_id,
                email:      email || null,
                name:       name || null,
                role:       role || 'member',
                invited_by: invited_by || null,
                joined_at:  now(),
            });

            const updatedMembers = [...org.team_members, member];

            await this._cacheAside.updateDocument(COLLECTION_NAME, orgId, {
                team_members: updatedMembers.map(m => m.forDB()),
                updated_at:   now(),
            });

            return await this.getById(orgId);
        } catch (error) {
            logger.error('[ORG-MODEL] Failed to add team member', {
                error: error.message,
                orgId,
                user_id,
            });
            throw error;
        }
    }

    async removeTeamMember(orgId, userId) {
        try {
            const org = await this.getById(orgId);
            if (!org) {
                throw new Error('Organization not found');
            }

            const updatedMembers = org.team_members.filter(m => m.user_id !== userId);

            await this._cacheAside.updateDocument(COLLECTION_NAME, orgId, {
                team_members: updatedMembers.map(m => m.forDB()),
                updated_at:   now(),
            });

            return await this.getById(orgId);
        } catch (error) {
            logger.error('[ORG-MODEL] Failed to remove team member', {
                error: error.message,
                orgId,
                userId,
            });
            throw error;
        }
    }

    async getTeamMembers(orgId) {
        try {
            const org = await this.getById(orgId);
            if (!org) {
                return [];
            }

            const result = [];

            if (org.org_admin) {
                result.push({
                    user_id:  org.org_admin.user_id,
                    email:    org.org_admin.email,
                    name:     org.org_admin.name,
                    role:     'admin',
                    is_admin: true,
                });
            }

            for (const m of org.team_members) {
                result.push({
                    user_id:  m.user_id,
                    email:    m.email,
                    name:     m.name,
                    role:     m.role || 'member',
                    is_admin: false,
                });
            }

            return result;
        } catch (error) {
            logger.error('[ORG-MODEL] Failed to get team members', {
                error: error.message,
                orgId,
            });
            throw error;
        }
    }

    async incrementInvitesSent(orgId) {
        try {
            const org = await this.getById(orgId);
            if (!org) {
                throw new Error('Organization not found');
            }

            await this._cacheAside.updateDocument(COLLECTION_NAME, orgId, {
                total_invites_sent: (org.total_invites_sent || 0) + 1,
                updated_at:         now(),
            });
        } catch (error) {
            logger.error('[ORG-MODEL] Failed to increment invites sent', {
                error: error.message,
                orgId,
            });
            throw error;
        }
    }

    async updateName(orgId, name) {
        try {
            await this._cacheAside.updateDocument(COLLECTION_NAME, orgId, {
                name,
                updated_at: now(),
            });

            logger.info('[ORG-MODEL] Organization name updated', { orgId, name });
        } catch (error) {
            logger.error('[ORG-MODEL] Failed to update organization name', {
                error: error.message,
                orgId,
            });
            throw error;
        }
    }
}
