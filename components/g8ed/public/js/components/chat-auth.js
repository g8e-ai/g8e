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
import { CasesManager } from './cases-manager.js';

export const ChatAuthMixin = {
    subscribeToAuthState() {
        if (window.authState && typeof window.authState.subscribe === 'function') {
            this.authStateUnsubscribe = window.authState.subscribe((event, data) => {
                this.handleAuthStateChange(event, data);
            });
        } else {
            console.warn('[CHAT] Global authState not available or does not support subscription');
        }
    },

    waitForAuthStateInitialization() {
        if (window.authState && window.authState.loading === false) {
            const authData = window.authState.getState();
            this.completeInitialization(authData);
            return;
        }

        this.eventBus.once(EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE, (data) => {
            this.completeInitialization(data);
        });
    },

    async completeInitialization(data) {
        if (data.isAuthenticated && data.webSessionModel) {
            this.currentUser = data.webSessionModel;
            this.webSessionModel = data.webSessionModel;
            this.currentWebSessionId = data.webSessionModel.id;

            try {
                await this.render();
            } catch (err) {
                console.error('[CHAT] Failed to render chat component:', err);
                return;
            }

            this.casesManager = new CasesManager(this.eventBus);
            this.casesManager.init();
            window.casesManager = this.casesManager;

            this.setupSSEListeners();
        }

        this.updateChatInputForAuthState(data.isAuthenticated);

        this.eventBus.emit(EventType.AUTH_COMPONENT_INITIALIZED_CHAT, {
            isAuthenticated: data.isAuthenticated,
            user: this.currentUser
        });
    },

    handleAuthStateChange(event, data) {
        switch (event) {
            case EventType.AUTH_USER_AUTHENTICATED:
                this.currentUser = data.webSessionModel;
                this.webSessionModel = data.webSessionModel;
                this.currentWebSessionId = data.webSessionModel.id;
                break;

            case EventType.AUTH_USER_UNAUTHENTICATED:
            case EventType.AUTH_SESSION_EXPIRED:
                this.currentUser = null;
                this.webSessionModel = null;
                this.currentWebSessionId = null;
                window.location.reload();
                break;

            case EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE:
                break;

            default:
                // Unhandled auth state event
        }
    },

    updateChatInputForAuthState(isAuthenticated) {
        const chatInputPanel = this.container?.querySelector('.chat-input-panel');
        if (chatInputPanel) {
            if (isAuthenticated) {
                chatInputPanel.classList.remove('chat-input-panel--disabled');
            } else {
                chatInputPanel.classList.add('chat-input-panel--disabled');
            }
        }

        const anchoredTerminalContainer = this.container?.querySelector('.anchored-terminal-container');
        if (anchoredTerminalContainer) {
            if (isAuthenticated) {
                anchoredTerminalContainer.classList.remove('anchored-terminal-container--disabled');
            } else {
                anchoredTerminalContainer.classList.add('anchored-terminal-container--disabled');
            }
        }

        this.eventBus.emit(EventType.OPERATOR_TERMINAL_AUTH_STATE_CHANGED, {
            isAuthenticated,
            user: isAuthenticated ? this.currentUser : null
        });
    },
};
