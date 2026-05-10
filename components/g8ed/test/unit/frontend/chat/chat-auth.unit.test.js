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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { MockEventBus, MockAuthState } from '@test/mocks/mock-browser-env.js';

let ChatAuthMixin;
let CasesManager;

function buildMockContainer() {
    const container = document.createElement('div');
    container.innerHTML = `
        <div class="chat-input-panel chat-input-panel--disabled"></div>
        <div class="anchored-terminal-container anchored-terminal-container--disabled"></div>
    `;
    return container;
}

function createMixinContext(overrides = {}) {
    const ctx = Object.create(null);
    Object.assign(ctx, ChatAuthMixin);
    ctx.eventBus = new MockEventBus();
    ctx.container = buildMockContainer();
    ctx.render = vi.fn().mockResolvedValue(undefined);
    ctx.setupSSEListeners = vi.fn();
    Object.assign(ctx, overrides);
    return ctx;
}

function makeMockWebSession(overrides = {}) {
    return {
        id: 'session_test_123',
        user_id: 'user_test_456',
        email: 'test@example.com',
        ...overrides
    };
}

beforeEach(async () => {
    vi.resetModules();

    class MockCasesManager {
        constructor(eventBus) {
            this.eventBus = eventBus;
        }
        init() {
            return;
        }
    }

    vi.doMock('@g8ed/public/js/components/cases-manager.js', () => ({
        CasesManager: MockCasesManager,
    }));

    const mod = await import('@g8ed/public/js/components/chat-auth.js');
    ChatAuthMixin = mod.ChatAuthMixin;

    const casesMod = await import('@g8ed/public/js/components/cases-manager.js');
    CasesManager = casesMod.CasesManager;
});

afterEach(() => {
    vi.restoreAllMocks();
    delete window.authState;
    delete window.casesManager;
});

describe('ChatAuthMixin [UNIT - jsdom]', () => {
    describe('subscribeToAuthState()', () => {
        it('subscribes to window.authState when available', () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            window.authState = mockAuthState;
            const subscribeSpy = vi.spyOn(mockAuthState, 'subscribe');

            ctx.subscribeToAuthState();

            expect(subscribeSpy).toHaveBeenCalledOnce();
            expect(ctx.authStateUnsubscribe).toBeDefined();
        });

        it('calls handleAuthStateChange when authState emits event', () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            window.authState = mockAuthState;
            const handleSpy = vi.spyOn(ctx, 'handleAuthStateChange');

            ctx.subscribeToAuthState();

            const testData = { isAuthenticated: true, webSessionModel: makeMockWebSession() };
            mockAuthState.notifySubscribers(EventType.AUTH_USER_AUTHENTICATED, testData);

            expect(handleSpy).toHaveBeenCalledWith(EventType.AUTH_USER_AUTHENTICATED, testData);
        });

        it('returns unsubscribe function', () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            window.authState = mockAuthState;

            ctx.subscribeToAuthState();

            expect(typeof ctx.authStateUnsubscribe).toBe('function');
        });

        it('warns when window.authState is not available', () => {
            const ctx = createMixinContext();
            const consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

            ctx.subscribeToAuthState();

            expect(consoleWarnSpy).toHaveBeenCalledWith(
                '[CHAT] Global authState not available or does not support subscription'
            );
            consoleWarnSpy.mockRestore();
        });

        it('warns when window.authState does not support subscription', () => {
            const ctx = createMixinContext();
            window.authState = {};
            const consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

            ctx.subscribeToAuthState();

            expect(consoleWarnSpy).toHaveBeenCalledWith(
                '[CHAT] Global authState not available or does not support subscription'
            );
            consoleWarnSpy.mockRestore();
        });
    });

    describe('waitForAuthStateInitialization()', () => {
        it('calls completeInitialization immediately when authState is loaded', () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            Object.defineProperty(mockAuthState, 'loading', {
                get: () => mockAuthState._state.loading,
                configurable: true
            });
            mockAuthState._state.loading = false;
            mockAuthState._state.isAuthenticated = true;
            mockAuthState._state.webSessionModel = makeMockWebSession();
            window.authState = mockAuthState;
            const completeSpy = vi.spyOn(ctx, 'completeInitialization');

            ctx.waitForAuthStateInitialization();

            expect(completeSpy).toHaveBeenCalledWith(mockAuthState.getState());
        });

        it('listens for AUTH_COMPONENT_INITIALIZED_AUTHSTATE when authState is loading', () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            mockAuthState._state.loading = true;
            window.authState = mockAuthState;
            const completeSpy = vi.spyOn(ctx, 'completeInitialization');

            ctx.waitForAuthStateInitialization();

            const testData = { isAuthenticated: true, webSessionModel: makeMockWebSession() };
            ctx.eventBus.emit(EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE, testData);

            expect(completeSpy).toHaveBeenCalledWith(testData);
        });

        it('does not call completeInitialization when authState is loading before event', () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            mockAuthState._state.loading = true;
            window.authState = mockAuthState;
            const completeSpy = vi.spyOn(ctx, 'completeInitialization');

            ctx.waitForAuthStateInitialization();

            expect(completeSpy).not.toHaveBeenCalled();
        });
    });

    describe('completeInitialization()', () => {
        it('sets currentUser, webSessionModel, and currentWebSessionId when authenticated', async () => {
            const ctx = createMixinContext();
            const mockSession = makeMockWebSession();
            const data = { isAuthenticated: true, webSessionModel: mockSession };

            await ctx.completeInitialization(data);

            expect(ctx.currentUser).toBe(mockSession);
            expect(ctx.webSessionModel).toBe(mockSession);
            expect(ctx.currentWebSessionId).toBe(mockSession.id);
        });

        it('calls render() when authenticated', async () => {
            const ctx = createMixinContext();
            const data = { isAuthenticated: true, webSessionModel: makeMockWebSession() };

            await ctx.completeInitialization(data);

            expect(ctx.render).toHaveBeenCalledOnce();
        });

        it('creates and initializes CasesManager when authenticated', async () => {
            const ctx = createMixinContext();
            const data = { isAuthenticated: true, webSessionModel: makeMockWebSession() };
            const initSpy = vi.spyOn(CasesManager.prototype, 'init').mockImplementation(() => {});

            await ctx.completeInitialization(data);

            expect(ctx.casesManager).toBeInstanceOf(CasesManager);
            expect(initSpy).toHaveBeenCalledOnce();
            initSpy.mockRestore();
        });

        it('sets window.casesManager when authenticated', async () => {
            const ctx = createMixinContext();
            const data = { isAuthenticated: true, webSessionModel: makeMockWebSession() };

            await ctx.completeInitialization(data);

            expect(window.casesManager).toBe(ctx.casesManager);
        });

        it('calls setupSSEListeners when authenticated', async () => {
            const ctx = createMixinContext();
            const data = { isAuthenticated: true, webSessionModel: makeMockWebSession() };

            await ctx.completeInitialization(data);

            expect(ctx.setupSSEListeners).toHaveBeenCalledOnce();
        });

        it('logs error and returns early when render() fails', async () => {
            const ctx = createMixinContext();
            const renderError = new Error('Render failed');
            ctx.render.mockRejectedValue(renderError);
            const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            const data = { isAuthenticated: true, webSessionModel: makeMockWebSession() };

            await ctx.completeInitialization(data);

            expect(consoleErrorSpy).toHaveBeenCalledWith('[CHAT] Failed to render chat component:', renderError);
            expect(ctx.casesManager).toBeUndefined();
            expect(ctx.setupSSEListeners).not.toHaveBeenCalled();
            consoleErrorSpy.mockRestore();
        });

        it('calls updateChatInputForAuthState regardless of authentication', async () => {
            const ctx = createMixinContext();
            const updateSpy = vi.spyOn(ctx, 'updateChatInputForAuthState');
            const data = { isAuthenticated: false };

            await ctx.completeInitialization(data);

            expect(updateSpy).toHaveBeenCalledWith(false);
        });

        it('emits AUTH_COMPONENT_INITIALIZED_CHAT with correct data when authenticated', async () => {
            const ctx = createMixinContext();
            const mockSession = makeMockWebSession();
            const data = { isAuthenticated: true, webSessionModel: mockSession };

            await ctx.completeInitialization(data);

            const emitted = ctx.eventBus.getEmitted(EventType.AUTH_COMPONENT_INITIALIZED_CHAT);
            expect(emitted).toHaveLength(1);
            expect(emitted[0].payload).toEqual({
                isAuthenticated: true,
                user: mockSession
            });
        });

        it('emits AUTH_COMPONENT_INITIALIZED_CHAT with isAuthenticated=false when not authenticated', async () => {
            const ctx = createMixinContext();
            const data = { isAuthenticated: false };

            await ctx.completeInitialization(data);

            const emitted = ctx.eventBus.getEmitted(EventType.AUTH_COMPONENT_INITIALIZED_CHAT);
            expect(emitted).toHaveLength(1);
            expect(emitted[0].payload.isAuthenticated).toBe(false);
            expect(emitted[0].payload.user).toBeUndefined();
        });

        it('does not create CasesManager when not authenticated', async () => {
            const ctx = createMixinContext();
            const data = { isAuthenticated: false };

            await ctx.completeInitialization(data);

            expect(ctx.casesManager).toBeUndefined();
        });

        it('does not call setupSSEListeners when not authenticated', async () => {
            const ctx = createMixinContext();
            const data = { isAuthenticated: false };

            await ctx.completeInitialization(data);

            expect(ctx.setupSSEListeners).not.toHaveBeenCalled();
        });

        it('handles missing webSessionModel gracefully when isAuthenticated is true', async () => {
            const ctx = createMixinContext();
            const data = { isAuthenticated: true, webSessionModel: null };

            await ctx.completeInitialization(data);

            expect(ctx.currentUser).toBeUndefined();
            expect(ctx.webSessionModel).toBeUndefined();
            expect(ctx.currentWebSessionId).toBeUndefined();
        });
    });

    describe('handleAuthStateChange()', () => {
        it('sets user state on AUTH_USER_AUTHENTICATED', () => {
            const ctx = createMixinContext();
            const mockSession = makeMockWebSession();
            const data = { webSessionModel: mockSession };

            ctx.handleAuthStateChange(EventType.AUTH_USER_AUTHENTICATED, data);

            expect(ctx.currentUser).toBe(mockSession);
            expect(ctx.webSessionModel).toBe(mockSession);
            expect(ctx.currentWebSessionId).toBe(mockSession.id);
        });

        it('clears user state on AUTH_USER_UNAUTHENTICATED', () => {
            const ctx = createMixinContext();
            ctx.currentUser = makeMockWebSession();
            ctx.webSessionModel = makeMockWebSession();
            ctx.currentWebSessionId = 'session_123';
            const reloadSpy = vi.fn();
            vi.stubGlobal('window', { ...window, location: { ...window.location, reload: reloadSpy } });

            ctx.handleAuthStateChange(EventType.AUTH_USER_UNAUTHENTICATED, {});

            expect(ctx.currentUser).toBeNull();
            expect(ctx.webSessionModel).toBeNull();
            expect(ctx.currentWebSessionId).toBeNull();
            expect(reloadSpy).toHaveBeenCalledOnce();
            vi.unstubAllGlobals();
        });

        it('clears user state on AUTH_SESSION_EXPIRED', () => {
            const ctx = createMixinContext();
            ctx.currentUser = makeMockWebSession();
            ctx.webSessionModel = makeMockWebSession();
            ctx.currentWebSessionId = 'session_123';
            const reloadSpy = vi.fn();
            vi.stubGlobal('window', { ...window, location: { ...window.location, reload: reloadSpy } });

            ctx.handleAuthStateChange(EventType.AUTH_SESSION_EXPIRED, {});

            expect(ctx.currentUser).toBeNull();
            expect(ctx.webSessionModel).toBeNull();
            expect(ctx.currentWebSessionId).toBeNull();
            expect(reloadSpy).toHaveBeenCalledOnce();
            vi.unstubAllGlobals();
        });

        it('does nothing on AUTH_COMPONENT_INITIALIZED_AUTHSTATE', () => {
            const ctx = createMixinContext();
            ctx.currentUser = makeMockWebSession();
            const currentUserBefore = ctx.currentUser;

            ctx.handleAuthStateChange(EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE, {});

            expect(ctx.currentUser).toBe(currentUserBefore);
        });

        it('does nothing on unknown events', () => {
            const ctx = createMixinContext();
            ctx.currentUser = makeMockWebSession();
            const currentUserBefore = ctx.currentUser;

            ctx.handleAuthStateChange('unknown.event', {});

            expect(ctx.currentUser).toBe(currentUserBefore);
        });
    });

    describe('updateChatInputForAuthState()', () => {
        it('removes disabled class from chat-input-panel when authenticated', () => {
            const ctx = createMixinContext();

            ctx.updateChatInputForAuthState(true);

            const chatInputPanel = ctx.container.querySelector('.chat-input-panel');
            expect(chatInputPanel.classList.contains('chat-input-panel--disabled')).toBe(false);
        });

        it('removes disabled class from anchored-terminal-container when authenticated', () => {
            const ctx = createMixinContext();

            ctx.updateChatInputForAuthState(true);

            const terminalContainer = ctx.container.querySelector('.anchored-terminal-container');
            expect(terminalContainer.classList.contains('anchored-terminal-container--disabled')).toBe(false);
        });

        it('adds disabled class to chat-input-panel when not authenticated', () => {
            const ctx = createMixinContext();
            const chatInputPanel = ctx.container.querySelector('.chat-input-panel');
            chatInputPanel.classList.remove('chat-input-panel--disabled');

            ctx.updateChatInputForAuthState(false);

            expect(chatInputPanel.classList.contains('chat-input-panel--disabled')).toBe(true);
        });

        it('adds disabled class to anchored-terminal-container when not authenticated', () => {
            const ctx = createMixinContext();
            const terminalContainer = ctx.container.querySelector('.anchored-terminal-container');
            terminalContainer.classList.remove('anchored-terminal-container--disabled');

            ctx.updateChatInputForAuthState(false);

            expect(terminalContainer.classList.contains('anchored-terminal-container--disabled')).toBe(true);
        });

        it('emits OPERATOR_TERMINAL_AUTH_STATE_CHANGED with user when authenticated', () => {
            const ctx = createMixinContext();
            const mockUser = makeMockWebSession();
            ctx.currentUser = mockUser;

            ctx.updateChatInputForAuthState(true);

            const emitted = ctx.eventBus.getEmitted(EventType.OPERATOR_TERMINAL_AUTH_STATE_CHANGED);
            expect(emitted).toHaveLength(1);
            expect(emitted[0].payload).toEqual({
                isAuthenticated: true,
                user: mockUser
            });
        });

        it('emits OPERATOR_TERMINAL_AUTH_STATE_CHANGED with null user when not authenticated', () => {
            const ctx = createMixinContext();

            ctx.updateChatInputForAuthState(false);

            const emitted = ctx.eventBus.getEmitted(EventType.OPERATOR_TERMINAL_AUTH_STATE_CHANGED);
            expect(emitted).toHaveLength(1);
            expect(emitted[0].payload).toEqual({
                isAuthenticated: false,
                user: null
            });
        });

        it('handles missing container gracefully', () => {
            const ctx = createMixinContext();
            ctx.container = null;

            expect(() => ctx.updateChatInputForAuthState(true)).not.toThrow();
        });

        it('handles missing chat-input-panel gracefully', () => {
            const ctx = createMixinContext();
            ctx.container = document.createElement('div');

            expect(() => ctx.updateChatInputForAuthState(true)).not.toThrow();
        });

        it('handles missing anchored-terminal-container gracefully', () => {
            const ctx = createMixinContext();
            ctx.container.innerHTML = '<div class="chat-input-panel"></div>';

            expect(() => ctx.updateChatInputForAuthState(true)).not.toThrow();
        });
    });

    describe('integration scenarios', () => {
        it('full flow: subscribeToAuthState -> waitForAuthStateInitialization -> completeInitialization', async () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            Object.defineProperty(mockAuthState, 'loading', {
                get: () => mockAuthState._state.loading,
                configurable: true
            });
            const mockSession = makeMockWebSession();
            mockAuthState._state.loading = false;
            mockAuthState._state.isAuthenticated = true;
            mockAuthState._state.webSessionModel = mockSession;
            window.authState = mockAuthState;
            const completeSpy = vi.spyOn(ctx, 'completeInitialization');

            ctx.subscribeToAuthState();
            ctx.waitForAuthStateInitialization();

            expect(completeSpy).toHaveBeenCalledWith(mockAuthState.getState());
        });

        it('full flow: auth state change triggers UI update', () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            window.authState = mockAuthState;
            const updateSpy = vi.spyOn(ctx, 'updateChatInputForAuthState');

            ctx.subscribeToAuthState();

            const mockSession = makeMockWebSession();
            mockAuthState.notifySubscribers(EventType.AUTH_USER_AUTHENTICATED, {
                webSessionModel: mockSession
            });

            expect(ctx.currentUser).toBe(mockSession);
            expect(updateSpy).not.toHaveBeenCalled();
        });

        it('full flow: unauthenticated triggers reload', () => {
            const ctx = createMixinContext();
            const mockAuthState = new MockAuthState();
            window.authState = mockAuthState;
            const reloadSpy = vi.fn();
            vi.stubGlobal('window', { ...window, location: { ...window.location, reload: reloadSpy } });

            ctx.subscribeToAuthState();

            mockAuthState.notifySubscribers(EventType.AUTH_USER_UNAUTHENTICATED, {});

            expect(ctx.currentUser).toBeNull();
            expect(reloadSpy).toHaveBeenCalledOnce();
            vi.unstubAllGlobals();
        });
    });
});
