// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { EventBus } from './utils/eventbus.js';
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

class DropOpsApp {
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
            console.error('[DropOpsApp] Failed to create Header:', error);
        }

        try {
            this.chat = new ChatComponent(this.eventBus);
            this.chat.init();
        } catch (error) {
            console.error('[DropOpsApp] Failed to create ChatComponent:', error);
        }

        try {
            this.operatorPanel = new OperatorPanel(this.eventBus);
            window.operatorPanel = this.operatorPanel;
        } catch (error) {
            console.error('[DropOpsApp] Failed to create OperatorPanel:', error);
        }

        try {
            this.footer = new Footer(this.eventBus);
            this.footer.init();
        } catch (error) {
            console.error('[DropOpsApp] Failed to create Footer:', error);
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
            if (data.isAuthenticated && data.webSessionId) {
                this.sseConnectionManager.initializeConnection(data.webSessionId);
            }
        });

        this.eventBus.once(EventType.AUTH_COMPONENT_INITIALIZED_CHAT, () => {
            if (this.operatorPanel) {
                this.operatorPanel.init().catch(error => {
                    console.error('[DropOpsApp] Failed to initialize OperatorPanel:', error);
                });
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
        alert('CRITICAL ERROR: ' + message);
    }

}

document.addEventListener('DOMContentLoaded', () => {
    if (window.dropOpsApp) {
        console.warn('[DropOpsApp] App already initialized, skipping duplicate initialization');
        return;
    }

    try {
        window.dropOpsApp = new DropOpsApp();
        window.dropOpsApp.init();
    } catch (error) {
        console.error('[DropOpsApp] Failed to initialize DropOpsApp:', error);
    }
});

export default DropOpsApp;