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

import { FrontendBaseModel, F } from './base.js';
import { UserRole } from '../constants/auth-constants.js';
import { isExpired } from '../utils/timestamp.js';

export class WebSessionModel extends FrontendBaseModel {
    static fields = {
        id:            { type: F.string,  default: null },
        user_id:       { type: F.string,  default: null },
        created_at:    { type: F.string,  default: null },
        updated_at:    { type: F.string,  default: null },
        expires_at:    { type: F.string,  default: null },
        last_activity: { type: F.string,  default: null },
        is_active:     { type: F.boolean, default: false, coerce: true },
        ended_at:      { type: F.string,  default: null },
        end_reason:    { type: F.string,  default: null },
        ip_address:    { type: F.string,  default: null },
        operator_id:   { type: F.string,  default: null },
        has_password:  { type: F.any,     default: null },
        api_key:       { type: F.any,     default: null },
        api_key_info:  { type: F.object,  default: () => ({ client_id: null, client_name: null, scopes: null, expires_at: null }) },
        user_data:     { type: F.object,  default: () => ({}) },
    };

    _validate() {
        if (this.api_key_info && typeof this.api_key_info === 'object') {
            this.api_key_info = {
                client_id:   this.api_key_info.client_id   ?? null,
                client_name: this.api_key_info.client_name ?? null,
                scopes:      this.api_key_info.scopes       ?? null,
                expires_at:  this.api_key_info.expires_at  ?? null,
            };
        }
        if (this.user_data && typeof this.user_data === 'object') {
            this.user_data = {
                id:              this.user_data.id              ?? null,
                email:           this.user_data.email           ?? null,
                name:            this.user_data.name            ?? null,
                email_verified:  this.user_data.email_verified  ?? null,
                roles:           this.user_data.roles           ?? [],
                organization_id: this.user_data.organization_id ?? null,
                login_method:    this.user_data.login_method    ?? null,
                api_key_id:      this.user_data.api_key_id      ?? null,
            };
        }
        if (!this.user_id && this.user_data?.user_id) {
            this.user_id = this.user_data.user_id;
        }
    }

    getDisplayName() {
        return this.user_data?.name || this.user_data?.email || 'User';
    }

    getEmail() {
        return this.user_data?.email ?? null;
    }

    getAvatar() {
        return null;
    }

    isValid() {
        if (!this.is_active) return false;
        if (!this.user_id) return false;
        if (this.expires_at && isExpired(this.expires_at)) return false;
        return true;
    }

    hasRole(role) {
        return Array.isArray(this.user_data?.roles) && this.user_data.roles.includes(role);
    }

    hasAnyRole(roles) {
        return roles.some(role => this.hasRole(role));
    }

    isAdmin() {
        return this.hasAnyRole([UserRole.ADMIN, UserRole.SUPERADMIN]);
    }

    getApiKey() {
        return this.api_key;
    }

    setApiKey(apiKey) {
        this.api_key = apiKey;
    }

    getApiScopes() {
        return this.api_key_info?.scopes ?? null;
    }

    hasScope(scope) {
        const scopes = this.getApiScopes();
        return Array.isArray(scopes) && scopes.includes(scope);
    }

    getExpiresAt() {
        return this.expires_at ? new Date(this.expires_at) : null;
    }

    getMinutesUntilExpiry() {
        const expiresAt = this.getExpiresAt();
        if (!expiresAt) return null;
        const diffMs = expiresAt.getTime() - Date.now();
        return Math.floor(diffMs / (1000 * 60));
    }

    toJSON() {
        return this.forWire();
    }

    static fromJSON(json) {
        return WebSessionModel.parse(json);
    }
}
