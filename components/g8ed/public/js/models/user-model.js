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

import { FrontendIdentifiableModel, F } from './base.js';
import { UserRole, AuthProvider } from '../constants/auth-constants.js';

// ---------------------------------------------------------------------------
// UserDocument  (frontend read model — matches forClient() shape from g8ed)
//
// Fields match shared/models/user.json.
// Secrets stripped server-side: password_hash and g8e_key are absent.
// Entry point: UserDocument.parse(raw) at every inbound boundary.
// ---------------------------------------------------------------------------

export class UserDocument extends FrontendIdentifiableModel {
    static fields = {
        id:              { type: F.string, required: true },
        email:           { type: F.string, required: true },
        name:            { type: F.string, default: null },
        organization_id: { type: F.string, default: null },
        roles:           { type: F.array,  default: () => [UserRole.USER] },
        operator_id:     { type: F.string, default: null },
        operator_status: { type: F.string, default: null },
        last_login:       { type: F.date,    default: null },
        provider:         { type: F.string,  default: AuthProvider.LOCAL },
        profile_picture:  { type: F.string,  default: null },
        dev_logs_enabled: { type: F.boolean, default: true, coerce: true },
    };

    getDisplayName() {
        return this.name || this.email.split('@')[0] || 'User';
    }

    hasRole(role) {
        return Array.isArray(this.roles) && this.roles.includes(role);
    }

    hasAnyRole(roles) {
        return roles.some(role => this.hasRole(role));
    }

    isAdmin() {
        return this.hasAnyRole([UserRole.ADMIN, UserRole.SUPERADMIN]);
    }

    getAvatar() {
        return this.profile_picture || null;
    }
}
