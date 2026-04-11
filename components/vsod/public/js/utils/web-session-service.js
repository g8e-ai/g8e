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

import { WebSessionModel } from '../models/session-model.js';

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
        return this._session?.id ?? null;
    }

    isAuthenticated() {
        return this._session?.isValid() ?? false;
    }

    getApiKey() {
        return this._session?.getApiKey() ?? null;
    }

    setApiKey(apiKey) {
        if (this._session) {
            this._session.setApiKey(apiKey);
        }
    }

    hasRole(role) {
        return this._session?.hasRole(role) ?? false;
    }

    isAdmin() {
        return this._session?.isAdmin() ?? false;
    }
}

export const webSessionService = new WebSessionService();
