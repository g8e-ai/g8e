// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { FrontendBaseModel, F } from './base.js';
import { UserRole } from '../constants/auth-constants.js';

/**
 * WebSessionModel — gateway-direct session shape.
 *
 * Mirrors the g8e Gateway's WebSessionResponse ({ success, user_id,
 * web_session_id }) and User ({ id, passkey_credentials, roles, ... }) shapes
 * from docs/guides/gui_enrollment.md. The HttpOnly g8e_web_session_cookie is
 * not JS-readable; this model holds only the user/session identifiers returned
 * in the JSON body so the SPA can drive UI state. The cookie itself is sent
 * automatically by the browser via credentials: 'include'.
 */
export class WebSessionModel extends FrontendBaseModel {
    static fields = {
        web_session_id: { type: F.string, default: null },
        user_id:        { type: F.string, default: null },
        user:           { type: F.object, default: () => ({}) },
    };

    _validate() {
        if (this.user && typeof this.user === 'object') {
            this.user = {
                id:                  this.user.id                  ?? this.user_id ?? null,
                passkey_credentials: this.user.passkey_credentials ?? [],
                provider:            this.user.provider            ?? null,
                organization_id:     this.user.organization_id     ?? null,
                roles:               this.user.roles               ?? [],
                status:              this.user.status              ?? null,
                is_bootstrap:        this.user.is_bootstrap        ?? false,
                local_os_user:       this.user.local_os_user       ?? null,
                webauthn_user_id:    this.user.webauthn_user_id    ?? null,
            };
        }
        if (!this.user_id && this.user?.id) {
            this.user_id = this.user.id;
        }
    }

    // FrontendBaseModel uses `id` as the canonical identifier in some derived
    // classes; expose web_session_id as the session id and user_id as the user
    // id so AuthManager/ChatAuthMixin callers keep working.
    get id() {
        return this.web_session_id;
    }

    getDisplayName() {
        const osUser = this.user?.local_os_user;
        if (osUser?.username) return osUser.username;
        if (this.user?.webauthn_user_id) return this.user.webauthn_user_id;
        return this.user_id || 'User';
    }

    getEmail() {
        return null;
    }

    getAvatar() {
        return '/media/default-avatar.png';
    }

    isValid() {
        // user_id is the authoritative authentication marker: it is present in
        // both WebSessionResponse ({ user_id, web_session_id }) and
        // UserMeResponse ({ user: { id } }). web_session_id is populated
        // separately via /api/v1/auth/sessions/me and is required only for SSE.
        if (!this.user_id) return false;
        return true;
    }

    hasRole(role) {
        return Array.isArray(this.user?.roles) && this.user.roles.includes(role);
    }

    hasAnyRole(roles) {
        return roles.some(role => this.hasRole(role));
    }

    isAdmin() {
        return this.hasAnyRole([UserRole.ADMIN, UserRole.SUPERADMIN]);
    }

    getExpiresAt() {
        return null;
    }

    getMinutesUntilExpiry() {
        return null;
    }

    toJSON() {
        return this.forWire();
    }

    static fromJSON(json) {
        return WebSessionModel.parse(json);
    }
}
