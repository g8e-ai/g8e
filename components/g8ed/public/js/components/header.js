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

import { EventType } from '../constants/events.js';
import { devLogger } from '../utils/dev-logger.js';

/**
 * Header - Top navigation and auth button container component.
 *
 * Owns: #auth-button-container, #nav-categories-menu
 * Communicates exclusively via EventBus.
 */
export class Header {
    constructor(eventBus) {
        this.eventBus = eventBus;
        this._root = null;
        this._authContainer = null;
    }

    init() {
        this._root = document.querySelector('header');
        this._authContainer = document.getElementById('auth-button-container');

        if (!this._root) {
            devLogger.warn('[HEADER] Root element not found');
            return;
        }

        this._bindEvents();
        devLogger.log('[HEADER] Initialized');
    }

    _bindEvents() {
        this.eventBus.on(EventType.AUTH_USER_AUTHENTICATED, () => this._onAuthenticated());
        this.eventBus.on(EventType.AUTH_USER_UNAUTHENTICATED, () => this._onUnauthenticated());
    }

    _onAuthenticated() {
        if (this._root) this._root.classList.add('authenticated');
    }

    _onUnauthenticated() {
        if (this._root) this._root.classList.remove('authenticated');
    }

    destroy() {
        this.eventBus.off(EventType.AUTH_USER_AUTHENTICATED);
        this.eventBus.off(EventType.AUTH_USER_UNAUTHENTICATED);
        this._root = null;
        this._authContainer = null;
    }
}
