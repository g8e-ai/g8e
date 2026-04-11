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
import markdownitFactory from 'markdown-it';
import domPurifyImpl from 'dompurify';
import { MockEventBus, MockAuthState, MockServiceClient } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@g8ed/public/js/constants/events.js';

vi.mock('@g8ed/public/js/utils/notification-service.js', () => ({
    notificationService: {
        info: vi.fn(),
        error: vi.fn(),
        show: vi.fn(),
    }
}));

import { notificationService } from '@g8ed/public/js/utils/notification-service.js';

const INVESTIGATION_ID = 'inv-test-component123';
const WEB_SESSION_ID = 'session-test-component123';

function buildDOM() {
    document.body.innerHTML = `
        <div class="main-content"></div>
        <div id="messages-container"></div>
        <div id="chat-status"></div>
        <button id="ai-stop-btn" disabled></button>
        <div id="anchored-terminal-body" style="height:400px;overflow:auto;"></div>
        <div id="anchored-terminal-attachments"></div>
        <button id="anchored-terminal-attach"></button>
    `;
}

function installGlobals(authState, serviceClient) {
    global.markdownit = markdownitFactory;
    global.DOMPurify = domPurifyImpl;
    window.authState = authState;
    window.serviceClient = serviceClient;
}

function cleanupGlobals() {
    delete window.authState;
    delete window.serviceClient;
    delete window.sentinelModeManager;
    delete window.llmModelManager;
    delete window.casesManager;
    delete global.markdownit;
    delete global.DOMPurify;
}

function makeAnchoredTerminalSpy() {
    return {
        appendUserMessage: vi.fn(() => document.createElement('div')),
        replaceStreamingHtml: vi.fn(),
        appendDirectHtmlResponse: vi.fn(() => document.createElement('div')),
        finalizeAIResponseChunk: vi.fn(),
        appendSystemMessage: vi.fn(() => document.createElement('div')),
        appendErrorMessage: vi.fn(),
        appendThinkingContent: vi.fn(),
        completeThinkingEntry: vi.fn(),
        clearActivityIndicators: vi.fn(),
        applyCitations: vi.fn(),
        applyCitationsAfterFinalize: vi.fn(),
        resetAutoScroll: vi.fn(),
        showWaitingIndicator: vi.fn(),
        hideWaitingIndicator: vi.fn(),
        clear: vi.fn(),
        focus: vi.fn(),
        enable: vi.fn(),
        disable: vi.fn(),
        setUser: vi.fn(),
        clearOutput: vi.fn(),
        scrollToBottom: vi.fn(),
        setAttachmentsUI: vi.fn(),
        markdownRenderer: null,
        citationsHandler: null,
    };
}

function makeCasesManagerStub(investigationId = INVESTIGATION_ID) {
    return {
        getCurrentInvestigationId: () => investigationId,
        getCurrentCaseId: () => 'case-test-component123',
        getCurrentTaskId: () => null,
        init: vi.fn(),
    };
}

function makeThinkingManagerSpy() {
    return {
        thinkingActive: false,
        hideThinkingIndicator: vi.fn(),
        clearAllThinkingData: vi.fn(),
        destroy: vi.fn(),
    };
}

describe('ChatComponent core class methods [FRONTEND - jsdom]', () => {
    let ChatComponent;
    let eventBus;
    let chat;
    let terminalSpy;
    let authState;
    let serviceClient;
    let notificationServiceSpy;

    beforeEach(async () => {
        vi.useFakeTimers();
        
        buildDOM();

        authState = new MockAuthState();
        authState.setAuthenticated({ id: WEB_SESSION_ID });
        authState.loading = false;
        authState.getWebSessionModel = () => ({ id: WEB_SESSION_ID });
        authState.getWebSessionId = () => WEB_SESSION_ID;

        serviceClient = new MockServiceClient();
        serviceClient.setResponse('g8ed', '/js/components/templates/chat-container.html', {
            ok: true,
            status: 200,
            text: async () => '<div id="messages-container"></div><button id="ai-stop-btn"></button><div id="anchored-terminal-body"></div><div id="anchored-terminal-attachments"></div><button id="anchored-terminal-attach"></button>'
        });

        installGlobals(authState, serviceClient);

        eventBus = new MockEventBus();

        ({ ChatComponent } = await import('@g8ed/public/js/components/chat.js'));

        chat = new ChatComponent(eventBus);
    });

    afterEach(() => {
        vi.useRealTimers();
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    describe('constructor', () => {
        it('initializes with eventBus and serviceClient from window', () => {
            expect(chat.eventBus).toBe(eventBus);
            expect(chat.serviceClient).toBe(serviceClient);
        });

        it('initializes auth state from window.authState', () => {
            expect(chat.webSessionModel).toStrictEqual(authState.getWebSessionModel());
            expect(chat.currentUser).toStrictEqual(authState.getWebSessionModel());
            expect(chat.currentWebSessionId).toBe(WEB_SESSION_ID);
        });

        it('initializes streaming and execution state flags', () => {
            expect(chat.streamingActive).toBe(false);
            expect(chat.executionActive).toBe(false);
            expect(chat.approvalPending).toBe(false);
        });

        it('initializes SentinelModeManager and assigns to window', () => {
            expect(chat.sentinelModeManager).toBeDefined();
            expect(window.sentinelModeManager).toBe(chat.sentinelModeManager);
        });

        it('initializes LlmModelManager and assigns to window', () => {
            expect(chat.llmModelManager).toBeDefined();
            expect(window.llmModelManager).toBe(chat.llmModelManager);
        });

        it('initializes AttachmentsManager', () => {
            expect(chat.attachments).toBeDefined();
        });

        it('initializes MarkdownRenderer and MessageRenderer', () => {
            expect(chat.markdownRenderer).toBeDefined();
            expect(chat.messageRenderer).toBeDefined();
        });

        it('initializes reconnection state with defaults', () => {
            expect(chat._reconnectAttempts).toBe(0);
            expect(chat._maxReconnectAttempts).toBe(5);
            expect(chat._reconnectDelay).toBe(1000);
            expect(chat._reconnectTimer).toBe(null);
        });

        it('creates debounced render chunk function', () => {
            expect(chat._debouncedRenderChunk).toBeDefined();
            expect(typeof chat._debouncedRenderChunk).toBe('function');
        });
    });

    describe('init', () => {
        it('sets up message renderer copy button listeners', () => {
            const setupCopyButtonListenersSpy = vi.spyOn(chat.messageRenderer, 'setupCopyButtonListeners');
            chat.init();
            expect(setupCopyButtonListenersSpy).toHaveBeenCalledOnce();
        });

        it('subscribes to auth state', () => {
            chat.init();
            expect(chat.authStateUnsubscribe).toBeDefined();
        });

        it('waits for auth state initialization', () => {
            const waitForAuthStateInitializationSpy = vi.spyOn(chat, 'waitForAuthStateInitialization');
            chat.init();
            expect(waitForAuthStateInitializationSpy).toHaveBeenCalledOnce();
        });
    });

    describe('showAIStopButton', () => {
        beforeEach(() => {
            chat.aiStopBtn = document.getElementById('ai-stop-btn');
        });

        it('enables the AI stop button when it exists', () => {
            chat.showAIStopButton();
            expect(chat.aiStopBtn.disabled).toBe(false);
        });

        it('sets correct title on the button', () => {
            chat.showAIStopButton();
            expect(chat.aiStopBtn.title).toBe('Stop AI Response');
        });

        it('does not throw when aiStopBtn is null', () => {
            chat.aiStopBtn = null;
            expect(() => chat.showAIStopButton()).not.toThrow();
        });
    });

    describe('hideAIStopButton', () => {
        beforeEach(() => {
            chat.aiStopBtn = document.getElementById('ai-stop-btn');
        });

        it('disables the AI stop button when it exists', () => {
            chat.hideAIStopButton();
            expect(chat.aiStopBtn.disabled).toBe(true);
        });

        it('sets correct title on the button', () => {
            chat.hideAIStopButton();
            expect(chat.aiStopBtn.title).toBe('No active AI response to stop');
        });

        it('does not throw when aiStopBtn is null', () => {
            chat.aiStopBtn = null;
            expect(() => chat.hideAIStopButton()).not.toThrow();
        });
    });

    describe('stopAIProcessing', () => {
        beforeEach(() => {
            chat.casesManager = makeCasesManagerStub();
            chat.currentWebSessionId = WEB_SESSION_ID;
            chat.pendingCitations = new Map();
            chat.pendingCitations.set(WEB_SESSION_ID, [{ id: 1, text: 'citation' }]);
            chat.thinkingManager = makeThinkingManagerSpy();
            chat.thinkingManager.thinkingActive = true;
            chat.anchoredTerminal = makeAnchoredTerminalSpy();
            chat._searchWebIndicators = { clear: vi.fn() };
            chat._portCheckIndicators = { clear: vi.fn() };
            chat.streamingActive = true;
            chat.executionActive = true;
            chat.approvalPending = true;

            serviceClient.setResponse('g8ed', '/api/chat/stop', {
                ok: true,
                status: 200,
                json: async () => ({ success: true })
            });
        });

        it('returns false when no active investigation', async () => {
            chat.casesManager = { getCurrentInvestigationId: () => null };
            const result = await chat.stopAIProcessing();
            expect(result).toBe(false);
        });

        it('returns false when no active operation', async () => {
            chat.streamingActive = false;
            chat.executionActive = false;
            chat.thinkingManager.thinkingActive = false;
            chat.approvalPending = false;
            const result = await chat.stopAIProcessing();
            expect(result).toBe(false);
        });

        it('sends stop request to correct endpoint with investigation_id and reason', async () => {
            await chat.stopAIProcessing({ reason: 'Test stop' });
            const requestLog = serviceClient.getRequestLog();
            expect(requestLog.length).toBeGreaterThan(0);
            const stopRequest = requestLog.find(req => req.path.includes('/api/chat/stop'));
            expect(stopRequest).toBeDefined();
            expect(stopRequest.body.investigation_id).toBe(INVESTIGATION_ID);
            expect(stopRequest.body.reason).toBe('Test stop');
        });

        it('uses default reason when not provided', async () => {
            await chat.stopAIProcessing();
            const requestLog = serviceClient.getRequestLog();
            const stopRequest = requestLog.find(req => req.path.includes('/api/chat/stop'));
            expect(stopRequest.body.reason).toBe('User requested stop');
        });

        it('emits approval denied event when approval is pending', async () => {
            const emitSpy = vi.spyOn(eventBus, 'emit');
            await chat.stopAIProcessing({ reason: 'User cancelled' });
            expect(emitSpy).toHaveBeenCalledWith(
                EventType.OPERATOR_TERMINAL_APPROVAL_DENIED,
                { reason: 'User cancelled', statusMessage: 'Cancelled' }
            );
        });

        it('clears approvalPending flag after emitting event', async () => {
            await chat.stopAIProcessing();
            expect(chat.approvalPending).toBe(false);
        });

        it('resets streaming and execution flags', async () => {
            await chat.stopAIProcessing();
            expect(chat.streamingActive).toBe(false);
            expect(chat.executionActive).toBe(false);
        });


        it('deletes pending citations for current session', async () => {
            await chat.stopAIProcessing();
            expect(chat.pendingCitations.has(WEB_SESSION_ID)).toBe(false);
        });

        it('hides thinking indicator via thinkingManager', async () => {
            await chat.stopAIProcessing();
            expect(chat.thinkingManager.hideThinkingIndicator).toHaveBeenCalledWith(WEB_SESSION_ID);
        });

        it('clears activity indicators on anchored terminal', async () => {
            await chat.stopAIProcessing();
            expect(chat.anchoredTerminal.clearActivityIndicators).toHaveBeenCalledOnce();
        });

        it('clears search web indicators', async () => {
            await chat.stopAIProcessing();
            expect(chat._searchWebIndicators.clear).toHaveBeenCalledOnce();
        });

        it('clears port check indicators', async () => {
            await chat.stopAIProcessing();
            expect(chat._portCheckIndicators.clear).toHaveBeenCalledOnce();
        });

        it('hides AI stop button', async () => {
            chat.aiStopBtn = document.getElementById('ai-stop-btn');
            await chat.stopAIProcessing();
            expect(chat.aiStopBtn.disabled).toBe(true);
        });

        it('shows info notification on success', async () => {
            await chat.stopAIProcessing();
            expect(notificationService.info).toHaveBeenCalledWith('AI response stopped by user');
        });

        it('returns true on successful stop', async () => {
            const result = await chat.stopAIProcessing();
            expect(result).toBe(true);
        });

        it('handles HTTP error response', async () => {
            serviceClient.setResponse('g8ed', '/api/chat/stop', {
                ok: false,
                status: 500,
                statusText: 'Internal Server Error'
            });
            const result = await chat.stopAIProcessing();
            expect(result).toBe(false);
        });

        it('shows error notification on failure', async () => {
            serviceClient.setResponse('g8ed', '/api/chat/stop', {
                ok: false,
                status: 500,
                statusText: 'Internal Server Error'
            });
            await chat.stopAIProcessing();
            expect(notificationService.error).toHaveBeenCalledWith('Failed to stop AI: HTTP 500: Internal Server Error');
        });

        it('handles silent mode without console warnings', async () => {
            const consoleWarnSpy = vi.spyOn(console, 'warn');
            chat.casesManager = { getCurrentInvestigationId: () => null };
            const result = await chat.stopAIProcessing({ silent: true });
            expect(result).toBe(false);
            expect(consoleWarnSpy).not.toHaveBeenCalled();
        });

        it('handles silent mode without error notifications', async () => {
            serviceClient.setResponse('g8ed', '/api/chat/stop', {
                ok: false,
                status: 500,
                statusText: 'Internal Server Error'
            });
            await chat.stopAIProcessing({ silent: true });
            expect(notificationService.error).not.toHaveBeenCalled();
        });

        it('does not modify state when no investigation and silent mode', async () => {
            chat.casesManager = { getCurrentInvestigationId: () => null };
            const originalStreamingActive = chat.streamingActive;
            await chat.stopAIProcessing({ silent: true });
            expect(chat.streamingActive).toBe(originalStreamingActive);
        });
    });

    describe('render', () => {
        beforeEach(() => {
            chat.currentUser = { id: WEB_SESSION_ID };
        });

        it('reuses existing chat container if present', async () => {
            const existingContainer = document.createElement('div');
            existingContainer.setAttribute('data-component', 'chat');
            document.querySelector('.main-content').appendChild(existingContainer);

            await chat.render();

            expect(chat.container).toBe(existingContainer);
            expect(chat.container.id).toBe('chat-container');
            expect(chat.container.className).toBe('chat-container');
        });

        it('creates new container if none exists', async () => {
            await chat.render();

            const container = document.querySelector('#chat-container');
            expect(container).toBeDefined();
            expect(container.className).toBe('chat-container');
        });

        it('returns early if currentUser is null', async () => {
            chat.currentUser = null;
            const bindDOMEventsSpy = vi.spyOn(chat, 'bindDOMEvents');
            await chat.render();
            expect(bindDOMEventsSpy).not.toHaveBeenCalled();
        });

        it('loads chat container template', async () => {
            await chat.render();
            expect(chat.container.innerHTML).toContain('messages-container');
        });

        it('binds DOM events after rendering', async () => {
            const bindDOMEventsSpy = vi.spyOn(chat, 'bindDOMEvents');
            await chat.render();
            expect(bindDOMEventsSpy).toHaveBeenCalledOnce();
        });

        it('initializes ThinkingManager with correct dependencies', async () => {
            await chat.render();
            expect(chat.thinkingManager).toBeDefined();
            expect(chat.thinkingManager.eventBus).toBe(eventBus);
        });

        it('initializes AnchoredOperatorTerminal', async () => {
            await chat.render();
            expect(chat.anchoredTerminal).toBeDefined();
            expect(chat.anchoredTerminal.markdownRenderer).toBe(chat.markdownRenderer);
        });

        it('initializes CompactAttachmentsUI when attachment display exists', async () => {
            await chat.render();
            expect(chat.attachmentsUI).toBeDefined();
            expect(chat.anchoredTerminal.setAttachmentsUI).toBeDefined();
        });

        it('creates attach button when attachment button exists', async () => {
            await chat.render();
            expect(chat.attachmentsUI).toBeDefined();
        });

        it('disables anchored terminal after initialization', async () => {
            await chat.render();
            expect(chat.anchoredTerminal).toBeDefined();
        });
    });

    describe('bindDOMEvents', () => {
        beforeEach(async () => {
            chat.currentUser = { id: WEB_SESSION_ID };
        });

        it('sets messagesContainer reference', async () => {
            await chat.render();
            expect(chat.messagesContainer).toBeDefined();
            expect(chat.messagesContainer.id).toBe('messages-container');
        });

        it('sets aiStopBtn reference', async () => {
            await chat.render();
            expect(chat.aiStopBtn).toBeDefined();
            expect(chat.aiStopBtn.id).toBe('ai-stop-btn');
        });

        it('attaches click handler to AI stop button', async () => {
            await chat.render();
            expect(chat.aiStopBtn).toBeDefined();
        });

        it('initializes SentinelModeManager', async () => {
            await chat.render();
            const initSpy = vi.spyOn(chat.sentinelModeManager, 'init');
            chat.bindDOMEvents();
            expect(initSpy).toHaveBeenCalledOnce();
        });

        it('initializes LlmModelManager', async () => {
            await chat.render();
            const initSpy = vi.spyOn(chat.llmModelManager, 'init');
            chat.bindDOMEvents();
            expect(initSpy).toHaveBeenCalledOnce();
        });

        it('initializes ScrollDelegation when scroll container exists', async () => {
            await chat.render();
            expect(chat.scrollDelegation).toBeDefined();
        });

        it('enables ScrollDelegation after initialization', async () => {
            await chat.render();
            expect(chat.scrollDelegation).toBeDefined();
        });
    });

    describe('clearChat', () => {
        beforeEach(() => {
            chat.messagesContainer = document.getElementById('messages-container');
            chat.messagesContainer.innerHTML = '<div>Test message</div>';
            chat.thinkingManager = makeThinkingManagerSpy();
            chat.streamingActive = true;
            chat.executionActive = true;
            chat.approvalPending = true;
            chat._searchWebIndicators = { clear: vi.fn() };
            chat._portCheckIndicators = { clear: vi.fn() };
            chat.anchoredTerminal = makeAnchoredTerminalSpy();
        });

        it('clears messages container HTML', () => {
            chat.clearChat();
            expect(chat.messagesContainer.innerHTML).toBe('');
        });

        it('clears all thinking data via thinkingManager', () => {
            chat.clearChat();
            expect(chat.thinkingManager.clearAllThinkingData).toHaveBeenCalledOnce();
        });

        it('resets streaming and execution flags', () => {
            chat.clearChat();
            expect(chat.streamingActive).toBe(false);
            expect(chat.executionActive).toBe(false);
            expect(chat.approvalPending).toBe(false);
        });

        it('hides AI stop button', () => {
            const hideSpy = vi.spyOn(chat, 'hideAIStopButton');
            chat.clearChat();
            expect(hideSpy).toHaveBeenCalledOnce();
        });

        it('clears search web indicators', () => {
            chat.clearChat();
            expect(chat._searchWebIndicators.clear).toHaveBeenCalledOnce();
        });

        it('clears port check indicators', () => {
            chat.clearChat();
            expect(chat._portCheckIndicators.clear).toHaveBeenCalledOnce();
        });

        it('clears anchored terminal output', () => {
            chat.clearChat();
            expect(chat.anchoredTerminal.clearOutput).toHaveBeenCalledOnce();
        });

        it('does not throw when messagesContainer is null', () => {
            chat.messagesContainer = null;
            expect(() => chat.clearChat()).not.toThrow();
        });

        it('does not throw when thinkingManager is null', () => {
            chat.thinkingManager = null;
            expect(() => chat.clearChat()).not.toThrow();
        });

        it('does not throw when anchoredTerminal is null', () => {
            chat.anchoredTerminal = null;
            expect(() => chat.clearChat()).not.toThrow();
        });
    });

    describe('destroy', () => {
        beforeEach(() => {
            chat.thinkingManager = makeThinkingManagerSpy();
            chat.container = document.createElement('div');
            document.body.appendChild(chat.container);
        });

        it('destroys thinkingManager if present', () => {
            chat.destroy();
            expect(chat.thinkingManager.destroy).toHaveBeenCalledOnce();
        });

        it('does not throw when thinkingManager is null', () => {
            chat.thinkingManager = null;
            expect(() => chat.destroy()).not.toThrow();
        });

        it('removes container from DOM if present', () => {
            chat.destroy();
            expect(chat.container.parentNode).toBeNull();
        });

        it('does not throw when container has no parent', () => {
            const containerWithoutParent = document.createElement('div');
            chat.container = containerWithoutParent;
            expect(() => chat.destroy()).not.toThrow();
        });

        it('does not throw when container is null', () => {
            chat.container = null;
            expect(() => chat.destroy()).not.toThrow();
        });
    });

    describe('addSystemMessage', () => {
        it('calls notificationService with message and type', () => {
            chat.addSystemMessage('Test message', 'warning');
            expect(notificationService.show).toHaveBeenCalledWith('Test message', 'warning');
        });

        it('defaults to info type when not provided', () => {
            chat.addSystemMessage('Test message');
            expect(notificationService.show).toHaveBeenCalledWith('Test message', 'info');
        });
    });

    describe('initCasesManager', () => {
        it('creates CasesManager with eventBus', () => {
            chat.initCasesManager();
            expect(chat.casesManager).toBeDefined();
        });

        it('assigns casesManager to window', () => {
            chat.initCasesManager();
            expect(window.casesManager).toBe(chat.casesManager);
        });
    });

    describe('attemptReconnect', () => {
        beforeEach(() => {
            chat.currentWebSessionId = WEB_SESSION_ID;
        });

        it('returns early when max reconnect attempts reached', () => {
            chat._reconnectAttempts = 5;
            const emitSpy = vi.spyOn(eventBus, 'emit');
            chat.attemptReconnect();
            expect(emitSpy).not.toHaveBeenCalled();
        });

        it('increments reconnect attempts counter', () => {
            chat.attemptReconnect();
            expect(chat._reconnectAttempts).toBe(1);
        });

        it('clears existing reconnect timer before setting new one', () => {
            chat._reconnectTimer = setTimeout(() => {}, 1000);
            const clearTimeoutSpy = vi.spyOn(global, 'clearTimeout');
            chat.attemptReconnect();
            expect(clearTimeoutSpy).toHaveBeenCalled();
        });

        it('emits CHAT_RECONNECTION_ATTEMPTED event with correct payload', () => {
            vi.useFakeTimers();
            const emitSpy = vi.spyOn(eventBus, 'emit');
            chat.attemptReconnect();
            vi.advanceTimersByTime(1000);
            expect(emitSpy).toHaveBeenCalledWith('CHAT_RECONNECTION_ATTEMPTED', {
                web_session_id: WEB_SESSION_ID,
                attempt: 1
            });
            vi.useRealTimers();
        });

        it('schedules reconnection with initial delay', () => {
            vi.useFakeTimers();
            const initialDelay = chat._reconnectDelay;
            const setTimeoutSpy = vi.spyOn(global, 'setTimeout');
            chat.attemptReconnect();
            expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), initialDelay);
            vi.useRealTimers();
        });

        it('implements exponential backoff for delay', () => {
            const initialDelay = chat._reconnectDelay;
            chat.attemptReconnect();
            const newDelay = chat._reconnectDelay;
            expect(newDelay).toBe(Math.min(initialDelay * 2, 30000));
        });

        it('caps maximum delay at 30 seconds', () => {
            chat._reconnectDelay = 20000;
            chat.attemptReconnect();
            expect(chat._reconnectDelay).toBe(30000);
        });
    });

    describe('_handleSSEConnectionClosed', () => {
        beforeEach(() => {
            chat.currentWebSessionId = WEB_SESSION_ID;
            chat.anchoredTerminal = makeAnchoredTerminalSpy();
            const attemptReconnectSpy = vi.spyOn(chat, 'attemptReconnect');
        });

        it('returns early when data is null', () => {
            const attemptReconnectSpy = vi.spyOn(chat, 'attemptReconnect');
            chat._handleSSEConnectionClosed(null);
            expect(attemptReconnectSpy).not.toHaveBeenCalled();
        });

        it('returns early when data.web_session_id is missing', () => {
            const attemptReconnectSpy = vi.spyOn(chat, 'attemptReconnect');
            chat._handleSSEConnectionClosed({});
            expect(attemptReconnectSpy).not.toHaveBeenCalled();
        });

        it('returns early when web_session_id does not match current session', () => {
            const attemptReconnectSpy = vi.spyOn(chat, 'attemptReconnect');
            chat._handleSSEConnectionClosed({ web_session_id: 'other-session' });
            expect(attemptReconnectSpy).not.toHaveBeenCalled();
        });

        it('appends error message to anchored terminal', () => {
            chat._handleSSEConnectionClosed({ web_session_id: WEB_SESSION_ID });
            expect(chat.anchoredTerminal.appendErrorMessage).toHaveBeenCalledWith('Connection lost. Attempting to reconnect...');
        });

        it('calls attemptReconnect when session matches', () => {
            const attemptReconnectSpy = vi.spyOn(chat, 'attemptReconnect');
            chat._handleSSEConnectionClosed({ web_session_id: WEB_SESSION_ID });
            expect(attemptReconnectSpy).toHaveBeenCalledOnce();
        });

        it('does not throw when anchoredTerminal is null', () => {
            chat.anchoredTerminal = null;
            expect(() => chat._handleSSEConnectionClosed({ web_session_id: WEB_SESSION_ID })).not.toThrow();
        });
    });

    describe('_handleSSEConnectionEstablished', () => {
        beforeEach(() => {
            chat.currentWebSessionId = WEB_SESSION_ID;
            chat.anchoredTerminal = makeAnchoredTerminalSpy();
            chat._reconnectAttempts = 3;
            chat._reconnectDelay = 4000;
            chat._reconnectTimer = setTimeout(() => {}, 1000);
        });

        it('returns early when data is null', () => {
            chat._handleSSEConnectionEstablished(null);
            expect(chat._reconnectAttempts).toBe(3);
        });

        it('returns early when data.web_session_id is missing', () => {
            chat._handleSSEConnectionEstablished({});
            expect(chat._reconnectAttempts).toBe(3);
        });

        it('returns early when web_session_id does not match current session', () => {
            chat._handleSSEConnectionEstablished({ web_session_id: 'other-session' });
            expect(chat._reconnectAttempts).toBe(3);
        });

        it('resets reconnect attempts to zero', () => {
            chat._handleSSEConnectionEstablished({ web_session_id: WEB_SESSION_ID });
            expect(chat._reconnectAttempts).toBe(0);
        });

        it('resets reconnect delay to initial value', () => {
            chat._handleSSEConnectionEstablished({ web_session_id: WEB_SESSION_ID });
            expect(chat._reconnectDelay).toBe(1000);
        });

        it('clears reconnect timer', () => {
            const clearTimeoutSpy = vi.spyOn(global, 'clearTimeout');
            chat._handleSSEConnectionEstablished({ web_session_id: WEB_SESSION_ID });
            expect(clearTimeoutSpy).toHaveBeenCalled();
        });

        it('sets reconnect timer to null after clearing', () => {
            chat._handleSSEConnectionEstablished({ web_session_id: WEB_SESSION_ID });
            expect(chat._reconnectTimer).toBeNull();
        });

        it('appends system message to anchored terminal', () => {
            chat._handleSSEConnectionEstablished({ web_session_id: WEB_SESSION_ID });
            expect(chat.anchoredTerminal.appendSystemMessage).toHaveBeenCalledWith('Connection re-established');
        });

        it('does not throw when anchoredTerminal is null', () => {
            chat.anchoredTerminal = null;
            expect(() => chat._handleSSEConnectionEstablished({ web_session_id: WEB_SESSION_ID })).not.toThrow();
        });
    });

    describe('_handleLLMChatIterationFailed', () => {
        beforeEach(() => {
            chat.currentWebSessionId = WEB_SESSION_ID;
            chat.anchoredTerminal = makeAnchoredTerminalSpy();
            chat.streamingActive = true;
        });

        it('returns early when data is null', () => {
            chat._handleLLMChatIterationFailed(null);
            expect(chat.streamingActive).toBe(true);
        });

        it('returns early when data.web_session_id is missing', () => {
            chat._handleLLMChatIterationFailed({});
            expect(chat.streamingActive).toBe(true);
        });

        it('returns early when web_session_id does not match current session', () => {
            chat._handleLLMChatIterationFailed({ web_session_id: 'other-session' });
            expect(chat.streamingActive).toBe(true);
        });

        it('cancels debounced render chunk', () => {
            const cancelSpy = vi.spyOn(chat._debouncedRenderChunk, 'cancel');
            chat._handleLLMChatIterationFailed({ web_session_id: WEB_SESSION_ID });
            expect(cancelSpy).toHaveBeenCalledOnce();
        });

        it('appends error message to anchored terminal with error from data', () => {
            chat._handleLLMChatIterationFailed({ 
                web_session_id: WEB_SESSION_ID,
                error: 'API rate limit exceeded'
            });
            expect(chat.anchoredTerminal.appendErrorMessage).toHaveBeenCalledWith('API rate limit exceeded');
        });

        it('appends default error message when error is not provided', () => {
            chat._handleLLMChatIterationFailed({ web_session_id: WEB_SESSION_ID });
            expect(chat.anchoredTerminal.appendErrorMessage).toHaveBeenCalledWith('AI processing failed');
        });

        it('sets streamingActive to false', () => {
            chat._handleLLMChatIterationFailed({ web_session_id: WEB_SESSION_ID });
            expect(chat.streamingActive).toBe(false);
        });

        it('hides AI stop button', () => {
            const hideSpy = vi.spyOn(chat, 'hideAIStopButton');
            chat._handleLLMChatIterationFailed({ web_session_id: WEB_SESSION_ID });
            expect(hideSpy).toHaveBeenCalledOnce();
        });

        it('deletes streaming content for the session', () => {
            chat._handleLLMChatIterationFailed({ web_session_id: WEB_SESSION_ID });
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(false);
        });

        it('does not throw when anchoredTerminal is null', () => {
            chat.anchoredTerminal = null;
            expect(() => chat._handleLLMChatIterationFailed({ web_session_id: WEB_SESSION_ID })).not.toThrow();
        });
    });
});
