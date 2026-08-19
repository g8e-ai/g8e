// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { FrontendBaseModel, F } from './base.js';
import { WebSessionModel } from './session-model.js';

/**
 * AuthResponseModel — gateway-direct auth response parser.
 *
 * The g8e Gateway returns two shapes after a successful passkey ceremony:
 *
 *   WebSessionResponse: { success, user_id, web_session_id }
 *   UserMeResponse:     { success, user: { id, passkey_credentials, ... } }
 *
 * Both are normalized into a WebSessionModel so AuthManager holds a single
 * session object regardless of which gateway endpoint produced the response.
 */
export class AuthResponseModel extends FrontendBaseModel {
    static fields = {
        success:         { type: F.boolean, default: false, coerce: true },
        user_id:         { type: F.string,  default: null },
        web_session_id:  { type: F.string,  default: null },
        user:            { type: F.object,  default: null },
        message:         { type: F.string,  default: null },
        needs_setup:     { type: F.boolean, default: false, coerce: true },
    };

    _validate() {
        if (!this.message) {
            this.message = this.success ? '' : 'An unknown authentication error occurred.';
        }
    }

    /**
     * Build a WebSessionModel from the normalized gateway response. The
     * session model is the single source of truth for AuthManager state.
     * Handles both WebSessionResponse ({ user_id, web_session_id }) and
     * UserMeResponse ({ user: { id } }); web_session_id is populated
     * separately by AuthManager via /api/v1/auth/sessions/me.
     */
    get session() {
        if (!this.success) return null;

        const sessionData = {
            web_session_id: this.web_session_id,
            user_id:        this.user_id,
            user:           this.user ?? {},
        };

        if (!sessionData.user_id && sessionData.user?.id) {
            sessionData.user_id = sessionData.user.id;
        }

        const session = WebSessionModel.parse(sessionData);
        return session.isValid() ? session : null;
    }

    get isAuthenticated() {
        return this.success && this.session !== null && this.session.isValid();
    }
}
