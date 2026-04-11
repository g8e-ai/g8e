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

// @vitest-environment jsdom

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { AuthManager } from '@vsod/public/js/components/auth.js';
import { WebSessionModel } from '@vsod/public/js/models/session-model.js';
import { EventType } from '@vsod/public/js/constants/events.js';
import { UserRole } from '@vsod/constants/auth.js';
import { ServiceName } from '@vsod/public/js/constants/service-client-constants.js';
import { ApiPaths } from '@vsod/public/js/constants/api-paths.js';
import { MockServiceClient } from '@test/mocks/mock-browser-env.js';

const FUTURE = new Date(Date.now() + 86400000).toISOString();

function makeValidSessionResponse(overrides = {}) {
    return {
        success: true,
        session: {
            id: 'web_session_test_123',
            user_id: 'user_test_456',
            created_at: new Date().toISOString(),
            expires_at: FUTURE,
            is_active: true,
            api_key: 'ak_test_key_789',
            api_key_info: { client_id: 'client_123', scopes: ['read', 'write'] },
            user_data: {
                id: 'user_test_456',
                email: 'test@g8e.ai',
                name: 'Test User',
                roles: [UserRole.USER],
                organization_id: 'org_test_789'
            },
            ...overrides.session
        },
        ...overrides
    };
}

function makeInvalidSessionResponse(message = 'WebSession expired or invalid') {
    return { success: false, authenticated: false, message };
}

function makeOkResponse(body) {
    return { ok: true, status: 200, json: async () => body };
}

describe('AuthManager [UNIT]', () => {
    let serviceClient;

    beforeEach(() => {
        document.body.innerHTML = `
            <div id="auth-button-container"></div>
            <div id="unauthenticated-banner"></div>
        `;

        window.APP_CONFIG = {};
        window.sseConnectionManager = {
            isConnectionActiveFor: vi.fn().mockReturnValue(false),
            initializeConnection: vi.fn(),
            disconnect: vi.fn()
        };

        serviceClient = new MockServiceClient();
        window.serviceClient = serviceClient;
    });

    afterEach(() => {
        vi.restoreAllMocks();
        delete window.APP_CONFIG;
        delete window.sseConnectionManager;
        delete window.serviceClient;
    });

    function makeAuth() {
        return new AuthManager(null);
    }

    describe('constructor isolation', () => {
        it('does not call init() during construction', () => {
            const initSpy = vi.spyOn(AuthManager.prototype, 'init').mockImplementation(() => {});
            makeAuth();
            expect(initSpy).not.toHaveBeenCalled();
            initSpy.mockRestore();
        });

        it('does not call serviceClient during construction', () => {
            makeAuth();
            expect(serviceClient.getRequestLog()).toHaveLength(0);
        });

        it('initialized is false before validateSession()', () => {
            const auth = makeAuth();
            expect(auth.initialized).toBe(false);
        });

        it('session is null before validateSession()', () => {
            const auth = makeAuth();
            expect(auth.session).toBeNull();
        });
    });

    describe('_handleUnauthenticatedInit()', () => {
        it('calls showPasskeyLoginModal() when pathname is /', () => {
            const auth = makeAuth();
            Object.defineProperty(window, 'location', {
                value: { pathname: '/' },
                writable: true,
                configurable: true,
            });
            const loginSpy = vi.spyOn(auth, 'showPasskeyLoginModal').mockImplementation(() => {});

            auth._handleUnauthenticatedInit();

            expect(loginSpy).toHaveBeenCalledOnce();
        });

        it('calls showPasskeyLoginModal() for any other unauthenticated page', () => {
            const auth = makeAuth();
            Object.defineProperty(window, 'location', {
                value: { pathname: '/chat' },
                writable: true,
                configurable: true,
            });
            const loginSpy = vi.spyOn(auth, 'showPasskeyLoginModal').mockImplementation(() => {});

            auth._handleUnauthenticatedInit();

            expect(loginSpy).toHaveBeenCalledOnce();
        });
    });

    describe('_fetchSession()', () => {
        it('GETs ApiPaths.auth.webSession() via window.serviceClient', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth._fetchSession();

            const log = serviceClient.getRequestLog();
            expect(log).toHaveLength(1);
            expect(log[0].method).toBe('GET');
            expect(log[0].service).toBe(ServiceName.VSOD);
            expect(log[0].path).toBe(ApiPaths.auth.webSession());
        });

        it('returns response and parsed data', async () => {
            const body = makeValidSessionResponse();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(body));

            const auth = makeAuth();
            const { response, data } = await auth._fetchSession();

            expect(response.ok).toBe(true);
            expect(data.success).toBe(true);
            expect(data.session.id).toBe('web_session_test_123');
        });

        it('throws when window.serviceClient is undefined', async () => {
            delete window.serviceClient;
            const auth = makeAuth();
            await expect(auth._fetchSession()).rejects.toThrow();
        });
    });

    describe('_applySessionData()', () => {
        it('calls setSession() when response is ok and data has a session', () => {
            const auth = makeAuth();
            const setSessionSpy = vi.spyOn(auth, 'setSession');
            const body = makeValidSessionResponse();

            auth._applySessionData({ ok: true }, body);

            expect(setSessionSpy).toHaveBeenCalledOnce();
            const arg = setSessionSpy.mock.calls[0][0];
            expect(arg).toBeInstanceOf(WebSessionModel);
        });

        it('calls clearSession() when response is not ok', () => {
            const auth = makeAuth();
            const clearSpy = vi.spyOn(auth, 'clearSession');

            auth._applySessionData({ ok: false }, { success: false });

            expect(clearSpy).toHaveBeenCalledOnce();
        });

        it('calls clearSession() when data.success is false', () => {
            const auth = makeAuth();
            const clearSpy = vi.spyOn(auth, 'clearSession');

            auth._applySessionData({ ok: true }, { success: false });

            expect(clearSpy).toHaveBeenCalledOnce();
        });

        it('calls clearSession() when data.session is missing', () => {
            const auth = makeAuth();
            const clearSpy = vi.spyOn(auth, 'clearSession');

            auth._applySessionData({ ok: true }, { success: true });

            expect(clearSpy).toHaveBeenCalledOnce();
        });
    });

    describe('validateSession() — valid session response', () => {
        it('sets session to a WebSessionModel instance', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.session).toBeInstanceOf(WebSessionModel);
        });

        it('isAuthenticated() returns true', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.isAuthenticated()).toBe(true);
        });

        it('getWebSessionId() returns the session id', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.getWebSessionId()).toBe('web_session_test_123');
        });

        it('marks initialized = true', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.initialized).toBe(true);
        });

        it('adds user-authenticated class to body', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(document.body.classList.contains('user-authenticated')).toBe(true);
        });

        it('GETs ApiPaths.auth.webSession() via window.serviceClient', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            const log = serviceClient.getRequestLog();
            expect(log).toHaveLength(1);
            expect(log[0].method).toBe('GET');
            expect(log[0].service).toBe(ServiceName.VSOD);
            expect(log[0].path).toBe(ApiPaths.auth.webSession());
        });
    });

    describe('validateSession() — invalid session response', () => {
        it('session remains null', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeInvalidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.session).toBeNull();
        });

        it('isAuthenticated() returns false', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeInvalidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.isAuthenticated()).toBe(false);
        });

        it('marks initialized = true', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeInvalidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.initialized).toBe(true);
        });

        it('does not add user-authenticated class to body', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeInvalidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(document.body.classList.contains('user-authenticated')).toBe(false);
        });
    });

    describe('validateSession() — window.serviceClient undefined (load-order regression)', () => {
        beforeEach(() => {
            delete window.serviceClient;
        });

        afterEach(() => {
            window.serviceClient = serviceClient;
        });

        it('does not throw', async () => {
            const auth = makeAuth();
            await expect(auth.validateSession()).resolves.toBeUndefined();
        });

        it('marks initialized = true', async () => {
            const auth = makeAuth();
            await auth.validateSession();
            expect(auth.initialized).toBe(true);
        });

        it('session remains null', async () => {
            const auth = makeAuth();
            await auth.validateSession();
            expect(auth.session).toBeNull();
            expect(auth.isAuthenticated()).toBe(false);
        });

        it('emits AUTH.COMPONENT.INITIALIZED.AUTHSTATE with isAuthenticated=false', async () => {
            const auth = makeAuth();
            const events = [];
            auth.subscribe((event, data) => events.push({ event, data }));

            await auth.validateSession();

            const initEvent = events.find(e => e.event === EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE);
            expect(initEvent).toBeDefined();
            expect(initEvent.data.isAuthenticated).toBe(false);
        });
    });

    describe('validateSession() — network error', () => {
        it('session remains null', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), Promise.reject(new Error('Network error')));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.session).toBeNull();
        });

        it('isAuthenticated() returns false', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), Promise.reject(new Error('Network error')));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.isAuthenticated()).toBe(false);
        });

        it('marks initialized = true', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), Promise.reject(new Error('Network error')));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.initialized).toBe(true);
        });
    });

    describe('validateSession() — event emission', () => {
        it('emits AUTH.USER.AUTHENTICATED after successful validation', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            const events = [];
            auth.subscribe((event, data) => events.push({ event, data }));

            await auth.validateSession();

            const authEvent = events.find(e => e.event === EventType.AUTH_USER_AUTHENTICATED);
            expect(authEvent).toBeDefined();
            expect(authEvent.data.isAuthenticated).toBe(true);
            expect(authEvent.data.webSessionId).toBe('web_session_test_123');
            expect(authEvent.data.webSessionModel).toBeInstanceOf(WebSessionModel);
        });

        it('emits AUTH.COMPONENT.INITIALIZED.AUTHSTATE after valid validation', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            const events = [];
            auth.subscribe((event, data) => events.push({ event, data }));

            await auth.validateSession();

            const initEvent = events.find(e => e.event === EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE);
            expect(initEvent).toBeDefined();
            expect(initEvent.data.isAuthenticated).toBe(true);
        });

        it('AUTH.USER.AUTHENTICATED is emitted before AUTH.COMPONENT.INITIALIZED.AUTHSTATE', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            const order = [];
            auth.subscribe((event) => order.push(event));

            await auth.validateSession();

            const authIdx = order.indexOf(EventType.AUTH_USER_AUTHENTICATED);
            const initIdx = order.indexOf(EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE);

            expect(authIdx).toBeGreaterThanOrEqual(0);
            expect(initIdx).toBeGreaterThan(authIdx);
        });

        it('does not emit AUTH.USER.AUTHENTICATED for an invalid session response', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeInvalidSessionResponse()));

            const auth = makeAuth();
            const events = [];
            auth.subscribe((event) => events.push(event));

            await auth.validateSession();

            expect(events).not.toContain(EventType.AUTH_USER_AUTHENTICATED);
        });

        it('emits AUTH.COMPONENT.INITIALIZED.AUTHSTATE with isAuthenticated=false for invalid session', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeInvalidSessionResponse()));

            const auth = makeAuth();
            const events = [];
            auth.subscribe((event, data) => events.push({ event, data }));

            await auth.validateSession();

            const initEvent = events.find(e => e.event === EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE);
            expect(initEvent).toBeDefined();
            expect(initEvent.data.isAuthenticated).toBe(false);
        });

        it('emits AUTH.USER.UNAUTHENTICATED when previously authenticated session expires', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();
            expect(auth.isAuthenticated()).toBe(true);

            const events = [];
            auth.subscribe((event, data) => events.push({ event, data }));

            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeInvalidSessionResponse()));
            await auth.validateSession();

            const unauthEvent = events.find(e => e.event === EventType.AUTH_USER_UNAUTHENTICATED);
            expect(unauthEvent).toBeDefined();
            expect(unauthEvent.data.isAuthenticated).toBe(false);
        });
    });

    describe('subscribe()', () => {
        it('notifies all subscribers on authentication', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            const sub1 = [];
            const sub2 = [];
            auth.subscribe((event) => sub1.push(event));
            auth.subscribe((event) => sub2.push(event));

            await auth.validateSession();

            expect(sub1).toContain(EventType.AUTH_USER_AUTHENTICATED);
            expect(sub2).toContain(EventType.AUTH_USER_AUTHENTICATED);
            expect(sub1).toEqual(sub2);
        });

        it('returns an unsubscribe function that stops delivery', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            const events = [];
            const unsubscribe = auth.subscribe((event) => events.push(event));
            unsubscribe();

            await auth.validateSession();

            expect(events).toHaveLength(0);
        });

        it('isolates subscriber errors so other subscribers still receive events', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            const goodEvents = [];

            auth.subscribe(() => { throw new Error('Subscriber error'); });
            auth.subscribe((event) => goodEvents.push(event));

            await auth.validateSession();

            expect(goodEvents).toContain(EventType.AUTH_USER_AUTHENTICATED);
        });
    });

    describe('getState()', () => {
        it('returns loading=true before validateSession()', () => {
            const auth = makeAuth();
            expect(auth.getState().loading).toBe(true);
            expect(auth.initialized).toBe(false);
        });

        it('returns loading=false after validateSession()', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeInvalidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.getState().loading).toBe(false);
        });

        it('is consistent with isAuthenticated() and getWebSessionId()', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            const state = auth.getState();
            expect(state.isAuthenticated).toBe(auth.isAuthenticated());
            expect(state.webSessionId).toBe(auth.getWebSessionId());
            expect(state.webSessionModel).toBe(auth.session);
            expect(state.loading).toBe(!auth.initialized);
        });
    });

    describe('logout()', () => {
        it('calls ApiPaths.auth.logout(), clears session, and navigates home', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.logout(), makeOkResponse({}));

            const auth = makeAuth();
            await auth.validateSession();

            const navigateSpy = vi.spyOn(auth, '_navigate').mockImplementation(() => {});

            await auth.logout();

            const log = serviceClient.getRequestLog();
            const logoutCall = log.find(r => r.path === ApiPaths.auth.logout());
            expect(logoutCall).toBeDefined();
            expect(logoutCall.service).toBe(ServiceName.VSOD);
            expect(auth.session).toBeNull();
            expect(navigateSpy).toHaveBeenCalled();
        });

        it('clears session even when logout request fails', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.logout(), Promise.reject(new Error('Network error')));

            const auth = makeAuth();
            await auth.validateSession();
            expect(auth.isAuthenticated()).toBe(true);

            const navigateSpy = vi.spyOn(auth, '_navigate').mockImplementation(() => {});
            await auth.logout();

            expect(auth.session).toBeNull();
            expect(navigateSpy).toHaveBeenCalled();
        });
    });

    describe('handleSessionExpired()', () => {
        it('clears session', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();
            expect(auth.isAuthenticated()).toBe(true);

            auth.handleSessionExpired();

            expect(auth.session).toBeNull();
            expect(auth.isAuthenticated()).toBe(false);
        });

        it('emits AUTH.USER.SESSION_EXPIRED event to subscribers', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            const events = [];
            auth.subscribe((event, data) => events.push({ event, data }));

            auth.handleSessionExpired();

            const expiredEvent = events.find(e => e.event === EventType.AUTH_SESSION_EXPIRED);
            expect(expiredEvent).toBeDefined();
            expect(expiredEvent.data.message).toBeDefined();
        });
    });

    describe('_subscribeToSSEFailed() / SSE-driven session invalidation', () => {
        it('calls handleSessionExpired() when SSE FAILED fires and user is authenticated', async () => {
            const { EventBus } = await import('@vsod/public/js/utils/eventbus.js');
            const eventBus = new EventBus();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = new AuthManager(eventBus);
            await auth.validateSession();
            expect(auth.isAuthenticated()).toBe(true);

            const expiredSpy = vi.spyOn(auth, 'handleSessionExpired');
            eventBus.emit(EventType.PLATFORM_SSE_CONNECTION_FAILED, {});

            expect(expiredSpy).toHaveBeenCalledOnce();
        });

        it('does not call handleSessionExpired() when SSE FAILED fires but user is not authenticated', async () => {
            const { EventBus } = await import('@vsod/public/js/utils/eventbus.js');
            const eventBus = new EventBus();

            const auth = new AuthManager(eventBus);
            const expiredSpy = vi.spyOn(auth, 'handleSessionExpired');

            eventBus.emit(EventType.PLATFORM_SSE_CONNECTION_FAILED, {});

            expect(expiredSpy).not.toHaveBeenCalled();
        });

        it('subscribes only once even if _subscribeToSSEFailed is called multiple times', async () => {
            const { EventBus } = await import('@vsod/public/js/utils/eventbus.js');
            const eventBus = new EventBus();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = new AuthManager(eventBus);
            await auth.validateSession();

            auth._subscribeToSSEFailed();
            auth._subscribeToSSEFailed();

            const expiredSpy = vi.spyOn(auth, 'handleSessionExpired');
            eventBus.emit(EventType.PLATFORM_SSE_CONNECTION_FAILED, {});

            expect(expiredSpy).toHaveBeenCalledOnce();
        });
    });

    describe('hasRole() / isAdmin()', () => {
        it('hasRole() delegates to session.hasRole()', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse({
                session: { user_data: { roles: [UserRole.ADMIN] } }
            })));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.hasRole(UserRole.ADMIN)).toBe(true);
            expect(auth.hasRole(UserRole.SUPERADMIN)).toBe(false);
        });

        it('isAdmin() returns true for admin role', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse({
                session: { user_data: { roles: [UserRole.ADMIN] } }
            })));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.isAdmin()).toBe(true);
        });

        it('isAdmin() returns false for plain user', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.isAdmin()).toBe(false);
        });

        it('hasRole() returns false when session is null', () => {
            const auth = makeAuth();
            expect(auth.hasRole(UserRole.ADMIN)).toBe(false);
        });

        it('isAdmin() returns false when session is null', () => {
            const auth = makeAuth();
            expect(auth.isAdmin()).toBe(false);
        });
    });

    describe('_renderPasskeyRegisterForm() — passkey cancel and retry', () => {
        function renderRegisterForm(auth) {
            const card = document.createElement('div');
            auth._renderPasskeyRegisterForm(card);
            document.body.appendChild(card);
            return card;
        }

        function getSubmitBtn(card) {
            return card.querySelector('button[type="submit"]');
        }

        function getInput(card, id) {
            return card.querySelector(`#${id}`);
        }

        function getError(card) {
            return card.querySelector('.auth-modal-error');
        }

        async function clickSubmit(card) {
            const btn = getSubmitBtn(card);
            btn.click();
            await new Promise(resolve => setTimeout(resolve, 0));
        }

        function fillInputs(card, name, email) {
            const nameInput = getInput(card, 'passkey-register-name');
            const emailInput = getInput(card, 'passkey-register-email');
            nameInput.value = name;
            emailInput.value = email;
        }

        beforeEach(() => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), {
                ok: false, status: 401, json: async () => ({ success: false })
            });
        });

        it('calls register API on first submit', async () => {
            const auth = makeAuth();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.register(), {
                ok: true, status: 201, json: async () => ({ success: true, user_id: 'user_123' })
            });
            vi.spyOn(auth, 'startPasskeyRegistration').mockResolvedValue({ success: true });
            vi.spyOn(auth, '_navigate').mockImplementation(() => {});

            const card = renderRegisterForm(auth);
            fillInputs(card, 'Alice', 'alice@example.com');
            await clickSubmit(card);

            const registerCalls = serviceClient.getRequestLog().filter(r => r.path === ApiPaths.auth.register());
            expect(registerCalls).toHaveLength(1);
        });

        it('does NOT call register API again when passkey is cancelled and user retries', async () => {
            const auth = makeAuth();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.register(), {
                ok: true, status: 201, json: async () => ({ success: true, user_id: 'user_123' })
            });
            vi.spyOn(auth, 'startPasskeyRegistration')
                .mockResolvedValueOnce({ success: false, message: 'Registration cancelled.' })
                .mockResolvedValueOnce({ success: true });
            vi.spyOn(auth, '_navigate').mockImplementation(() => {});

            const card = renderRegisterForm(auth);
            fillInputs(card, 'Alice', 'alice@example.com');

            await clickSubmit(card);
            await clickSubmit(card);

            const registerCalls = serviceClient.getRequestLog().filter(r => r.path === ApiPaths.auth.register());
            expect(registerCalls).toHaveLength(1);
        });

        it('calls startPasskeyRegistration with the same userId on retry after cancel', async () => {
            const auth = makeAuth();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.register(), {
                ok: true, status: 201, json: async () => ({ success: true, user_id: 'user_abc' })
            });
            const registrationSpy = vi.spyOn(auth, 'startPasskeyRegistration')
                .mockResolvedValueOnce({ success: false, message: 'Registration cancelled.' })
                .mockResolvedValueOnce({ success: true });
            vi.spyOn(auth, '_navigate').mockImplementation(() => {});

            const card = renderRegisterForm(auth);
            fillInputs(card, 'Alice', 'alice@example.com');

            await clickSubmit(card);
            await clickSubmit(card);

            expect(registrationSpy).toHaveBeenCalledTimes(2);
            expect(registrationSpy.mock.calls[0][0]).toBe('user_abc');
            expect(registrationSpy.mock.calls[1][0]).toBe('user_abc');
        });

        it('disables name and email inputs after account is created', async () => {
            const auth = makeAuth();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.register(), {
                ok: true, status: 201, json: async () => ({ success: true, user_id: 'user_123' })
            });
            vi.spyOn(auth, 'startPasskeyRegistration').mockResolvedValue({ success: false, message: 'Registration cancelled.' });
            vi.spyOn(auth, '_navigate').mockImplementation(() => {});

            const card = renderRegisterForm(auth);
            fillInputs(card, 'Alice', 'alice@example.com');
            await clickSubmit(card);

            expect(getInput(card, 'passkey-register-name').disabled).toBe(true);
            expect(getInput(card, 'passkey-register-email').disabled).toBe(true);
        });

        it('shows error message when passkey is cancelled', async () => {
            const auth = makeAuth();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.register(), {
                ok: true, status: 201, json: async () => ({ success: true, user_id: 'user_123' })
            });
            vi.spyOn(auth, 'startPasskeyRegistration').mockResolvedValue({ success: false, message: 'Registration cancelled.' });
            vi.spyOn(auth, '_navigate').mockImplementation(() => {});

            const card = renderRegisterForm(auth);
            fillInputs(card, 'Alice', 'alice@example.com');
            await clickSubmit(card);

            const errorEl = getError(card);
            expect(errorEl.classList.contains('hidden')).toBe(false);
            expect(errorEl.textContent).toBe('Registration cancelled.');
        });

        it('button text changes to "Set Up Passkey" after account creation and cancel', async () => {
            const auth = makeAuth();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.register(), {
                ok: true, status: 201, json: async () => ({ success: true, user_id: 'user_123' })
            });
            vi.spyOn(auth, 'startPasskeyRegistration').mockResolvedValue({ success: false, message: 'Registration cancelled.' });
            vi.spyOn(auth, '_navigate').mockImplementation(() => {});

            const card = renderRegisterForm(auth);
            fillInputs(card, 'Alice', 'alice@example.com');
            await clickSubmit(card);

            expect(getSubmitBtn(card).textContent).toBe('Set Up Passkey');
        });

        it('does not call register API if name is missing', async () => {
            const auth = makeAuth();
            const card = renderRegisterForm(auth);
            fillInputs(card, '', 'alice@example.com');
            await clickSubmit(card);

            const registerCalls = serviceClient.getRequestLog().filter(r => r.path === ApiPaths.auth.register());
            expect(registerCalls).toHaveLength(0);
        });

        it('shows account creation error without blocking retry when register API fails', async () => {
            const auth = makeAuth();
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.register(), {
                ok: false, status: 409, json: async () => ({ success: false, error: 'An account with that email already exists' })
            });

            const card = renderRegisterForm(auth);
            fillInputs(card, 'Alice', 'alice@example.com');
            await clickSubmit(card);

            const errorEl = getError(card);
            expect(errorEl.classList.contains('hidden')).toBe(false);
            expect(errorEl.textContent).toBe('An account with that email already exists');
            expect(getSubmitBtn(card).disabled).toBe(false);
            expect(getSubmitBtn(card).textContent).toBe('Create Account');
        });
    });

    describe('session model fields after validateSession()', () => {
        it('getEmail() returns the session email', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse()));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.session.getEmail()).toBe('test@g8e.ai');
        });

        it('getApiKey() returns the api key from session', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse({
                session: { api_key: 'ak_admin_key' }
            })));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.getApiKey()).toBe('ak_admin_key');
        });

        it('hasRole() reflects roles from session user_data', async () => {
            serviceClient.setResponse(ServiceName.VSOD, ApiPaths.auth.webSession(), makeOkResponse(makeValidSessionResponse({
                session: { user_data: { roles: [UserRole.USER, UserRole.ADMIN] } }
            })));

            const auth = makeAuth();
            await auth.validateSession();

            expect(auth.hasRole(UserRole.ADMIN)).toBe(true);
            expect(auth.hasRole(UserRole.SUPERADMIN)).toBe(false);
        });
    });
});
