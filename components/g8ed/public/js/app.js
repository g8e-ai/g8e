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

import { EventBus } from './utils/eventbus.js';
import { initFavicon } from './utils/favicon.js';
import { AuthManager } from './components/auth.js';
import { ChatComponent } from './components/chat.js';
import { OperatorPanel } from './components/operator-panel.js';
import { Header } from './components/header.js';
import { Footer } from './components/footer.js';
import { SSEConnectionManager } from './utils/sse-connection-manager.js';
import { EventType } from './constants/events.js';
import { notificationService } from './utils/notification-service.js';
import { CssClass } from './constants/ui-constants.js';
import { webSessionService } from './utils/web-session-service.js';
import { operatorSessionService } from './utils/operator-session-service.js';

class g8eApp {
    constructor() {
        this.eventBus = new EventBus();
        this.serviceClient = null;

        this.auth = null;
        this.sseConnectionManager = null;

        this.header = null;
        this.chat = null;
        this.operatorPanel = null;
        this.footer = null;
    }

    init() {
        initFavicon().catch(e => console.warn('[App] Favicon init failed:', e));
        this.serviceClient = window.serviceClient;
        notificationService.init();

        this.auth = new AuthManager(this.eventBus);
        window.authState = this.auth;
        window.webSessionService = webSessionService;
        window.operatorSessionService = operatorSessionService;

        this.sseConnectionManager = new SSEConnectionManager(this.eventBus);
        window.sseConnectionManager = this.sseConnectionManager;

        try {
            this.header = new Header(this.eventBus);
            this.header.init();
        } catch (error) {
            console.error('[g8eApp] Failed to create Header:', error);
        }

        try {
            this.chat = new ChatComponent(this.eventBus);
            this.chat.init();
        } catch (error) {
            console.error('[g8eApp] Failed to create ChatComponent:', error);
        }

        try {
            this.operatorPanel = new OperatorPanel(this.eventBus);
            window.operatorPanel = this.operatorPanel;
        } catch (error) {
            console.error('[g8eApp] Failed to create OperatorPanel:', error);
        }

        try {
            this.footer = new Footer(this.eventBus);
            this.footer.init();
        } catch (error) {
            console.error('[g8eApp] Failed to create Footer:', error);
        }

        this.setupEventListeners();
        this.auth.init();
    }

    setupUI() {
        this.handleUrlCallbacks();
    }

    setupEventListeners() {
        this.eventBus.once(EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE, (data) => {
            this.setupUI();
        });

        this.eventBus.once(EventType.AUTH_COMPONENT_INITIALIZED_CHAT, () => {
            console.log('[g8eApp] AUTH_COMPONENT_INITIALIZED_CHAT fired, initializing SSE and operatorPanel');
            const authState = window.authState.get();
            if (authState.isAuthenticated && authState.webSessionId) {
                this.sseConnectionManager.initializeConnection(authState.webSessionId);
            }
            
            if (this.operatorPanel) {
                this.operatorPanel.init().catch(error => {
                    console.error('[g8eApp] Failed to initialize OperatorPanel:', error);
                });
            } else {
                console.error('[g8eApp] AUTH_COMPONENT_INITIALIZED_CHAT fired but operatorPanel is null');
            }
        });

        this.eventBus.on(EventType.PLATFORM_TERMINAL_OPENED, () => {
            const terminal = document.querySelector('[data-component="terminal"]');
            if (terminal) {
                terminal.classList.remove(CssClass.INITIALLY_HIDDEN);
            }
        });

        this.eventBus.on(EventType.PLATFORM_TERMINAL_MINIMIZED, () => {
            const terminal = document.querySelector('[data-component="terminal"]');
            if (terminal) {
                terminal.classList.add(CssClass.INITIALLY_HIDDEN);
            }
        });

        this.eventBus.on(EventType.PLATFORM_TERMINAL_MAXIMIZED, () => {
            const terminal = document.querySelector('[data-component="terminal"]');
            if (terminal) {
                terminal.classList.remove(CssClass.INITIALLY_HIDDEN);
            }
        });
    }

    handleUrlCallbacks() {
        const urlParams = new URLSearchParams(window.location.search);

        if (urlParams.has('error')) {
            const error = urlParams.get('error');
            window.history.replaceState({}, document.title, window.location.pathname);

            if (error === 'user_creation_failed') {
                notificationService.error('Failed to create account. Please try again or contact support.', { duration: 443 });
            } else if (error === 'auth_failed') {
                const details = urlParams.get('details');
                notificationService.error(`Authentication failed${details ? ': ' + details : ''}`, { duration: 443 });
            } else {
                notificationService.error(`Authentication error: ${error}`, { duration: 443 });
            }
        }
    }

    async fileToBase64(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = () => {
                const base64 = reader.result.split(',')[1];
                resolve(base64);
            };
            reader.onerror = reject;
            reader.readAsDataURL(file);
        });
    }

    showCriticalError(message) {
        console.error('[APP] CRITICAL ERROR:', message);
        notificationService.error('CRITICAL ERROR: ' + message);
    }

}

document.addEventListener('DOMContentLoaded', () => {
    if (window.g8eApp) {
        console.warn('[g8eApp] App already initialized, skipping duplicate initialization');
        return;
    }

    try {
        window.g8eApp = new g8eApp();
        window.g8eApp.init();
    } catch (error) {
        console.error('[g8eApp] Failed to initialize g8eApp:', error);
    }
});

export default g8eApp;