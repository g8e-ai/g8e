// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { WebSessionModel } from '../models/session-model.js';

/**
 * WebSessionService — in-memory cache of the gateway-direct session.
 *
 * The g8e Gateway authenticates browser sessions via the HttpOnly
 * g8e_web_session_cookie, which is not readable from JavaScript. This service
 * holds only the WebSessionModel (user_id, web_session_id, user) derived from
 * gateway JSON responses so other components can query auth state without
 * re-fetching. The cookie itself is sent automatically via credentials:
 * 'include'.
 */
class WebSessionService {
    constructor() {
        this._session = null;
    }

    setSession(session) {
        if (session !== null && !(session instanceof WebSessionModel)) {
            throw new Error('WebSessionService.setSession requires a WebSessionModel instance or null');
        }
        this._session = session;
    }

    clearSession() {
        this._session = null;
    }

    getSession() {
        return this._session;
    }

    getWebSessionId() {
        return this._session?.web_session_id ?? null;
    }

    getUserId() {
        return this._session?.user_id ?? null;
    }

    isAuthenticated() {
        return this._session?.isValid() ?? false;
    }

    hasRole(role) {
        return this._session?.hasRole(role) ?? false;
    }

    isAdmin() {
        return this._session?.isAdmin() ?? false;
    }
}

export const webSessionService = new WebSessionService();
