// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockEventBus, MockServiceClient, MockAuthState } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { CssClass } from '@g8ed/public/js/constants/ui-constants.js';

vi.mock('@g8ed/public/js/utils/notification-service.js', () => ({
    notificationService: {
        init: vi.fn(),
        error: vi.fn(),
    }
}));

vi.mock('@g8ed/public/js/components/auth.js', () => {
    class MockAuthManager {
        constructor(eventBus) {
            this.eventBus = eventBus;
            this.init = vi.fn();
        }
    }
    return { AuthManager: MockAuthManager };
});

vi.mock('@g8ed/public/js/components/chat.js', () => {
    class MockChatComponent {
        constructor(eventBus) {
            this.eventBus = eventBus;
            this.init = vi.fn();
        }
    }
    return { ChatComponent: MockChatComponent };
});

vi.mock('@g8ed/public/js/components/operator-panel.js', () => {
    class MockOperatorPanel {
        constructor(eventBus) {
            this.eventBus = eventBus;
            this.init = vi.fn().mockResolvedValue(undefined);
        }
    }
    return { OperatorPanel: MockOperatorPanel };
});

vi.mock('@g8ed/public/js/components/header.js', () => {
    class MockHeader {
        constructor(eventBus) {
            this.eventBus = eventBus;
            this.init = vi.fn();
        }
    }
    return { Header: MockHeader };
});

vi.mock('@g8ed/public/js/components/footer.js', () => {
    class MockFooter {
        constructor(eventBus) {
            this.eventBus = eventBus;
            this.init = vi.fn();
        }
    }
    return { Footer: MockFooter };
});

vi.mock('@g8ed/public/js/utils/sse-connection-manager.js', () => {
    class MockSSEConnectionManager {
        constructor(eventBus) {
            this.eventBus = eventBus;
            this.initializeConnection = vi.fn();
        }
    }
    return { SSEConnectionManager: MockSSEConnectionManager };
});

vi.mock('@g8ed/public/js/utils/web-session-service.js', () => ({
    webSessionService: {},
}));

vi.mock('@g8ed/public/js/utils/operator-session-service.js', () => ({
    operatorSessionService: {},
}));

import DropOpsApp from '@g8ed/public/js/app.js';
import { notificationService } from '@g8ed/public/js/utils/notification-service.js';
import { AuthManager } from '@g8ed/public/js/components/auth.js';
import { ChatComponent } from '@g8ed/public/js/components/chat.js';
import { OperatorPanel } from '@g8ed/public/js/components/operator-panel.js';
import { Header } from '@g8ed/public/js/components/header.js';
import { Footer } from '@g8ed/public/js/components/footer.js';
import { SSEConnectionManager } from '@g8ed/public/js/utils/sse-connection-manager.js';

describe('DropOpsApp [FRONTEND - jsdom]', () => {
    let eventBus;
    let serviceClient;
    let authState;
    let app;
    let app2;

    beforeEach(() => {
        eventBus = new MockEventBus();
        serviceClient = new MockServiceClient();
        authState = new MockAuthState();

        window.serviceClient = serviceClient;
        window.sseConnectionManager = null;
        window.authState = null;
        window.webSessionService = {};
        window.operatorSessionService = {};
        window.operatorPanel = null;
        window.dropOpsApp = null;

        delete window.location;
        window.location = {
            pathname: '/chat',
            search: '',
            href: 'https://localhost/chat',
            origin: 'https://localhost'
        };
        window.history = {
            replaceState: vi.fn(),
        };
    });

    afterEach(() => {
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        delete window.serviceClient;
        delete window.sseConnectionManager;
        delete window.authState;
        delete window.webSessionService;
        delete window.operatorSessionService;
        delete window.operatorPanel;
        delete window.dropOpsApp;
    });

    describe('constructor', () => {
        it('initializes eventBus', () => {
            app = new DropOpsApp();
            expect(app.eventBus).toBeDefined();
        });

        it('initializes serviceClient as null', () => {
            app = new DropOpsApp();
            expect(app.serviceClient).toBeNull();
        });

        it('initializes auth as null', () => {
            app = new DropOpsApp();
            expect(app.auth).toBeNull();
        });

        it('initializes sseConnectionManager as null', () => {
            app = new DropOpsApp();
            expect(app.sseConnectionManager).toBeNull();
        });

        it('initializes header as null', () => {
            app = new DropOpsApp();
            expect(app.header).toBeNull();
        });

        it('initializes chat as null', () => {
            app = new DropOpsApp();
            expect(app.chat).toBeNull();
        });

        it('initializes operatorPanel as null', () => {
            app = new DropOpsApp();
            expect(app.operatorPanel).toBeNull();
        });

        it('initializes footer as null', () => {
            app = new DropOpsApp();
            expect(app.footer).toBeNull();
        });

        it('creates new EventBus instance', () => {
            app = new DropOpsApp();
            app2 = new DropOpsApp();
            expect(app.eventBus).not.toBe(app2.eventBus);
        });
    });

    describe('init', () => {
        beforeEach(() => {
            document.body.innerHTML = `
                <div data-component="terminal" class="${CssClass.INITIALLY_HIDDEN}"></div>
            `;
        });

        it('sets serviceClient from window.serviceClient', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.serviceClient).toBe(serviceClient);
        });

        it('calls notificationService.init', () => {
            app = new DropOpsApp();
            app.init();
            expect(notificationService.init).toHaveBeenCalledTimes(1);
        });

        it('creates AuthManager', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.auth).toBeDefined();
            expect(window.authState).toBe(app.auth);
        });

        it('sets window.webSessionService', () => {
            app = new DropOpsApp();
            app.init();
            expect(window.webSessionService).toBeDefined();
        });

        it('sets window.operatorSessionService', () => {
            app = new DropOpsApp();
            app.init();
            expect(window.operatorSessionService).toBeDefined();
        });

        it('creates SSEConnectionManager with eventBus', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.sseConnectionManager).toBeDefined();
            expect(window.sseConnectionManager).toBe(app.sseConnectionManager);
        });

        it('creates Header', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.header).toBeDefined();
        });

        it('creates ChatComponent', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.chat).toBeDefined();
        });

        it('creates OperatorPanel and sets window.operatorPanel', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.operatorPanel).toBeDefined();
            expect(window.operatorPanel).toBe(app.operatorPanel);
        });

        it('does not call OperatorPanel.init in init()', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.operatorPanel.init).not.toHaveBeenCalled();
        });

        it('creates Footer', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.footer).toBeDefined();
        });

        it('calls auth.init', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.auth.init).toHaveBeenCalledTimes(1);
        });

        it('calls header.init', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.header.init).toHaveBeenCalledTimes(1);
        });

        it('calls chat.init', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.chat.init).toHaveBeenCalledTimes(1);
        });

        it('calls footer.init', () => {
            app = new DropOpsApp();
            app.init();
            expect(app.footer.init).toHaveBeenCalledTimes(1);
        });

        it('creates new EventBus instance on each app instance', () => {
            app = new DropOpsApp();
            app.init();
            app2 = new DropOpsApp();
            app2.init();
            expect(app.eventBus).not.toBe(app2.eventBus);
        });

        it('does not throw when window.serviceClient is undefined', () => {
            delete window.serviceClient;
            app = new DropOpsApp();
            expect(() => app.init()).not.toThrow();
            expect(app.serviceClient).toBeUndefined();
        });

        it('sets window.sseConnectionManager', () => {
            app = new DropOpsApp();
            app.init();
            expect(window.sseConnectionManager).toBe(app.sseConnectionManager);
        });

        it('sets window.authState', () => {
            app = new DropOpsApp();
            app.init();
            expect(window.authState).toBe(app.auth);
        });

        it('sets window.operatorPanel', () => {
            app = new DropOpsApp();
            app.init();
            expect(window.operatorPanel).toBe(app.operatorPanel);
        });

        it('can be initialized multiple times', () => {
            app = new DropOpsApp();
            expect(() => {
                app.init();
                app.init();
            }).not.toThrow();
        });

        it('does not throw when init called without DOM elements', () => {
            document.body.innerHTML = '';
            app = new DropOpsApp();
            expect(() => app.init()).not.toThrow();
        });

    });

    describe('setupUI', () => {
        it('calls handleUrlCallbacks', () => {
            app = new DropOpsApp();
            const handleUrlCallbacksSpy = vi.spyOn(app, 'handleUrlCallbacks');
            app.setupUI();
            expect(handleUrlCallbacksSpy).toHaveBeenCalledTimes(1);
        });

        it('can be called multiple times', () => {
            app = new DropOpsApp();
            expect(() => {
                app.setupUI();
                app.setupUI();
            }).not.toThrow();
        });
    });

    describe('setupEventListeners', () => {
        it('calls setupEventListeners during init', () => {
            document.body.innerHTML = `
                <div data-component="terminal" class="${CssClass.INITIALLY_HIDDEN}"></div>
            `;
            app = new DropOpsApp();
            const setupEventListenersSpy = vi.spyOn(app, 'setupEventListeners');
            app.init();
            expect(setupEventListenersSpy).toHaveBeenCalledTimes(1);
        });

        it('PLATFORM_TERMINAL_OPENED does not throw when terminal element does not exist', () => {
            document.body.innerHTML = '';
            app = new DropOpsApp();
            app.init();
            expect(() => {
                eventBus.emit(EventType.PLATFORM_TERMINAL_OPENED);
            }).not.toThrow();
        });

        it('PLATFORM_TERMINAL_MINIMIZED does not throw when terminal element does not exist', () => {
            document.body.innerHTML = '';
            app = new DropOpsApp();
            app.init();
            expect(() => {
                eventBus.emit(EventType.PLATFORM_TERMINAL_MINIMIZED);
            }).not.toThrow();
        });

        it('PLATFORM_TERMINAL_MAXIMIZED does not throw when terminal element does not exist', () => {
            document.body.innerHTML = '';
            app = new DropOpsApp();
            app.init();
            expect(() => {
                eventBus.emit(EventType.PLATFORM_TERMINAL_MAXIMIZED);
            }).not.toThrow();
        });
    });

    describe('handleUrlCallbacks', () => {
        beforeEach(() => {
            app = new DropOpsApp();
        });

        it('does nothing when URL has no error parameter', () => {
            window.location.search = '';
            const notificationSpy = vi.spyOn(notificationService, 'error');
            app.handleUrlCallbacks();
            expect(notificationSpy).not.toHaveBeenCalled();
        });

        it('shows notification for user_creation_failed error', () => {
            window.location.search = '?error=user_creation_failed';
            const notificationSpy = vi.spyOn(notificationService, 'error');
            app.handleUrlCallbacks();
            expect(notificationSpy).toHaveBeenCalledWith('Failed to create account. Please try again or contact support.', { duration: 443 });
        });

        it('shows notification for auth_failed error without details', () => {
            window.location.search = '?error=auth_failed';
            const notificationSpy = vi.spyOn(notificationService, 'error');
            app.handleUrlCallbacks();
            expect(notificationSpy).toHaveBeenCalledWith('Authentication failed', { duration: 443 });
        });

        it('shows notification for auth_failed error with details', () => {
            window.location.search = '?error=auth_failed&details=Invalid%20credentials';
            const notificationSpy = vi.spyOn(notificationService, 'error');
            app.handleUrlCallbacks();
            expect(notificationSpy).toHaveBeenCalledWith('Authentication failed: Invalid credentials', { duration: 443 });
        });

        it('shows generic notification for unknown error', () => {
            window.location.search = '?error=unknown_error';
            const notificationSpy = vi.spyOn(notificationService, 'error');
            app.handleUrlCallbacks();
            expect(notificationSpy).toHaveBeenCalledWith('Authentication error: unknown_error', { duration: 443 });
        });

        it('calls history.replaceState to remove query parameters', () => {
            window.location.search = '?error=user_creation_failed';
            app.handleUrlCallbacks();
            expect(window.history.replaceState).toHaveBeenCalledWith({}, document.title, window.location.pathname);
        });

        it('handles multiple URL parameters correctly', () => {
            window.location.search = '?error=auth_failed&details=Session%20expired&other=value';
            const notificationSpy = vi.spyOn(notificationService, 'error');
            app.handleUrlCallbacks();
            expect(notificationSpy).toHaveBeenCalledWith('Authentication failed: Session expired', { duration: 443 });
        });

        it('handles empty error parameter', () => {
            window.location.search = '?error=';
            const notificationSpy = vi.spyOn(notificationService, 'error');
            app.handleUrlCallbacks();
            expect(notificationSpy).toHaveBeenCalledWith('Authentication error: ', { duration: 443 });
        });

        it('handles error with special characters in details', () => {
            window.location.search = '?error=auth_failed&details=Error%3A%20%40%23%24%25';
            const notificationSpy = vi.spyOn(notificationService, 'error');
            app.handleUrlCallbacks();
            expect(notificationSpy).toHaveBeenCalledWith('Authentication failed: Error: @#$%', { duration: 443 });
        });
    });

    describe('fileToBase64', () => {
        beforeEach(() => {
            app = new DropOpsApp();
        });

        it('converts file to base64 string', async () => {
            const mockFile = new File(['test content'], 'test.txt', { type: 'text/plain' });
            const result = await app.fileToBase64(mockFile);
            expect(result).toBe('dGVzdCBjb250ZW50');
        });

        it('rejects when FileReader fails', async () => {
            const mockFile = new File(['test'], 'test.txt', { type: 'text/plain' });
            const originalReadAsDataURL = FileReader.prototype.readAsDataURL;
            FileReader.prototype.readAsDataURL = function() {
                setTimeout(() => {
                    this.onerror(new Error('Read failed'));
                }, 0);
            };
            await expect(app.fileToBase64(mockFile)).rejects.toThrow('Read failed');
            FileReader.prototype.readAsDataURL = originalReadAsDataURL;
        });

        it('handles binary files', async () => {
            const binaryData = new Uint8Array([0x48, 0x65, 0x6c, 0x6c, 0x6f]);
            const mockFile = new File([binaryData], 'binary.bin', { type: 'application/octet-stream' });
            const result = await app.fileToBase64(mockFile);
            expect(result).toBe('SGVsbG8=');
        });

        it('handles empty file', async () => {
            const mockFile = new File([''], 'empty.txt', { type: 'text/plain' });
            const result = await app.fileToBase64(mockFile);
            expect(result).toBe('');
        });

        it('handles large file', async () => {
            const largeContent = 'x'.repeat(10000);
            const mockFile = new File([largeContent], 'large.txt', { type: 'text/plain' });
            const result = await app.fileToBase64(mockFile);
            expect(result).toBeTruthy();
            expect(result.length).toBeGreaterThan(0);
        });

        it('handles file with special characters', async () => {
            const mockFile = new File(['test © ® ™'], 'special.txt', { type: 'text/plain' });
            const result = await app.fileToBase64(mockFile);
            expect(result).toBeTruthy();
        });

        it('handles file with unicode characters', async () => {
            const mockFile = new File(['test 你好 🎉'], 'unicode.txt', { type: 'text/plain' });
            const result = await app.fileToBase64(mockFile);
            expect(result).toBeTruthy();
        });
    });

    describe('showCriticalError', () => {
        beforeEach(() => {
            app = new DropOpsApp();
        });

        it('logs error to console', () => {
            const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
            app.showCriticalError('Test error');
            expect(consoleErrorSpy).toHaveBeenCalledWith('[APP] CRITICAL ERROR:', 'Test error');
            consoleErrorSpy.mockRestore();
            alertSpy.mockRestore();
        });

        it('shows alert with error message', () => {
            const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
            app.showCriticalError('Test error');
            expect(alertSpy).toHaveBeenCalledWith('CRITICAL ERROR: Test error');
            alertSpy.mockRestore();
        });

        it('handles empty error message', () => {
            const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
            app.showCriticalError('');
            expect(consoleErrorSpy).toHaveBeenCalledWith('[APP] CRITICAL ERROR:', '');
            expect(alertSpy).toHaveBeenCalledWith('CRITICAL ERROR: ');
            consoleErrorSpy.mockRestore();
            alertSpy.mockRestore();
        });

        it('handles error with special characters', () => {
            const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
            app.showCriticalError('Error: "test" @#$%');
            expect(alertSpy).toHaveBeenCalledWith('CRITICAL ERROR: Error: "test" @#$%');
            alertSpy.mockRestore();
        });

        it('handles long error message', () => {
            const longError = 'x'.repeat(1000);
            const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
            app.showCriticalError(longError);
            expect(alertSpy).toHaveBeenCalledWith('CRITICAL ERROR: ' + longError);
            alertSpy.mockRestore();
        });
    });

    describe('DOMContentLoaded listener', () => {
        beforeEach(() => {
            document.body.innerHTML = `
                <div data-component="terminal" class="${CssClass.INITIALLY_HIDDEN}"></div>
            `;
        });

        it('initializes app on DOMContentLoaded', () => {
            expect(window.dropOpsApp).toBeNull();
            const event = new Event('DOMContentLoaded');
            document.dispatchEvent(event);
            expect(window.dropOpsApp).toBeInstanceOf(DropOpsApp);
        });

        it('calls init on the app instance', () => {
            const initSpy = vi.spyOn(DropOpsApp.prototype, 'init');
            const event = new Event('DOMContentLoaded');
            document.dispatchEvent(event);
            expect(initSpy).toHaveBeenCalled();
            initSpy.mockRestore();
        });

        it('skips initialization if window.dropOpsApp already exists', () => {
            window.dropOpsApp = {};
            const consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
            const initSpy = vi.spyOn(DropOpsApp.prototype, 'init');
            const event = new Event('DOMContentLoaded');
            document.dispatchEvent(event);
            expect(consoleWarnSpy).toHaveBeenCalledWith('[DropOpsApp] App already initialized, skipping duplicate initialization');
            expect(initSpy).not.toHaveBeenCalled();
            consoleWarnSpy.mockRestore();
            initSpy.mockRestore();
        });

        it('logs error when initialization fails', () => {
            vi.spyOn(DropOpsApp.prototype, 'init').mockImplementation(() => {
                throw new Error('Init failed');
            });
            const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            const event = new Event('DOMContentLoaded');
            document.dispatchEvent(event);
            expect(consoleErrorSpy).toHaveBeenCalledWith('[DropOpsApp] Failed to initialize DropOpsApp:', expect.any(Error));
            consoleErrorSpy.mockRestore();
        });

        it('sets window.dropOpsApp after successful initialization', () => {
            const event = new Event('DOMContentLoaded');
            document.dispatchEvent(event);
            expect(window.dropOpsApp).toBeDefined();
            expect(window.dropOpsApp).toBeInstanceOf(DropOpsApp);
        });

        it('does not overwrite window.dropOpsApp if already exists', () => {
            window.dropOpsApp = { existing: true };
            const consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
            const event = new Event('DOMContentLoaded');
            document.dispatchEvent(event);
            expect(window.dropOpsApp).toEqual({ existing: true });
            consoleWarnSpy.mockRestore();
        });

        it('handles multiple DOMContentLoaded events', () => {
            const event = new Event('DOMContentLoaded');
            document.dispatchEvent(event);
            const firstApp = window.dropOpsApp;
            document.dispatchEvent(event);
            expect(window.dropOpsApp).toBe(firstApp);
        });
    });
});
