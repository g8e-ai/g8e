// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import markdownitFactory from 'markdown-it';
import domPurifyImpl from 'dompurify';
import { MockEventBus, MockAuthState, MockServiceClient } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@g8ed/public/js/models/investigation-models.js';

const INVESTIGATION_ID = 'inv-test-abc123';
const WEB_SESSION_ID = 'session-test-abc123';

function buildDOM() {
    document.body.innerHTML = `
        <div id="messages-container"></div>
        <div id="chat-status"></div>
        <button id="ai-stop-btn" disabled></button>
        <div id="anchored-terminal-body" style="height:400px;overflow:auto;"></div>
        <div id="waiting-indicator" style="display:none;"></div>
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
    delete global.markdownit;
    delete global.DOMPurify;
}

function makeAnchoredTerminalSpy() {
    return {
        finalizeAIResponseChunk: vi.fn(),
        clearActivityIndicators: vi.fn(),
        appendAIResponse: vi.fn(() => document.createElement('div')),
        appendAIResponseChunk: vi.fn(),
        appendUserMessage: vi.fn(() => document.createElement('div')),
        appendSystemMessage: vi.fn(() => document.createElement('div')),
        appendErrorMessage: vi.fn(),
        applyCitations: vi.fn(),
        applyCitationsAfterFinalize: vi.fn(),
        completeActivityIndicator: vi.fn(),
        resetAutoScroll: vi.fn(),
        showWaitingIndicator: vi.fn(),
        clear: vi.fn(),
        focus: vi.fn(),
        enable: vi.fn(),
        disable: vi.fn(),
        setUser: vi.fn(),
        clearOutput: vi.fn(),
        scrollToBottom: vi.fn(),
        restoreCommandExecution: vi.fn(),
        restoreCommandResult: vi.fn(),
        restoreApprovalRequest: vi.fn(),
        pendingApprovals: new Map(),
    };
}

function makeCasesManagerStub(investigationId = INVESTIGATION_ID) {
    return {
        getCurrentInvestigationId: () => investigationId,
        getCurrentCaseId: () => 'case-test-abc123',
        getCurrentTaskId: () => null,
        init: vi.fn(),
    };
}

describe('ChatComponent response complete handling [FRONTEND - jsdom]', () => {
    let ChatComponent;
    let eventBus;
    let chat;
    let terminalSpy;
    let authState;
    let serviceClient;

    beforeEach(async () => {
        vi.useFakeTimers();
        
        buildDOM();

        authState = new MockAuthState();
        authState.setAuthenticated({ id: WEB_SESSION_ID });
        authState.loading = false;
        authState.getwebSessionModel = () => ({ id: WEB_SESSION_ID });
        authState.getwebSessionId = () => WEB_SESSION_ID;

        serviceClient = new MockServiceClient();
        serviceClient.setResponse('g8ed', '/js/components/templates/streaming-message.html', {
            ok: true, status: 200, text: async () => ''
        });

        installGlobals(authState, serviceClient);

        eventBus = new MockEventBus();

        ({ ChatComponent } = await import('@g8ed/public/js/components/chat.js'));

        chat = new ChatComponent(eventBus);

        terminalSpy = makeAnchoredTerminalSpy();
        chat.anchoredTerminal = terminalSpy;
        chat.casesManager = makeCasesManagerStub();
        chat.currentUser = { id: WEB_SESSION_ID };
        chat.currentWebSessionId = WEB_SESSION_ID;
        chat.setupSSEListeners();
    });

    afterEach(() => {
        vi.useRealTimers();
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    describe('shouldProcessEvent — investigation_id gating', () => {
        it('rejects event with no investigation_id', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat.streamingActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
            });

            expect(chat.streamingActive).toBe(true);
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(true);
        });

        it('rejects event with mismatched investigation_id', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat.streamingActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: 'other-investigation',
            });

            expect(chat.streamingActive).toBe(true);
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(true);
        });

        it('rejects event when casesManager has no current investigation', () => {
            chat.casesManager = makeCasesManagerStub(null);
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat.streamingActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.streamingActive).toBe(true);
        });

        it('rejects event and does not throw when casesManager is null', () => {
            chat.casesManager = null;
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat.streamingActive = true;

            expect(() => chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            })).not.toThrow();

            expect(chat.streamingActive).toBe(true);
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(true);
        });

        it('processes event with matching investigation_id', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat.streamingActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.streamingActive).toBe(false);
        });
    });

    describe('handleResponseComplete — streamed content path', () => {
        beforeEach(() => {
            chat.streamingContent.set(WEB_SESSION_ID, 'streamed text');
            chat.streamingActive = true;
            chat.aiStopBtn = document.getElementById('ai-stop-btn');
            chat.aiStopBtn.disabled = false;
        });

        it('calls anchoredTerminal.finalizeAIResponseChunk with the web_session_id', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
            expect(terminalSpy.finalizeAIResponseChunk.mock.calls[0][0]).toBe(WEB_SESSION_ID);
        });

        it('calls anchoredTerminal.clearActivityIndicators on finalize', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(terminalSpy.clearActivityIndicators).toHaveBeenCalledOnce();
        });

        it('removes the session from streamingContent', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(false);
        });

        it('sets streamingActive to false', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.streamingActive).toBe(false);
        });

        it('disables the AI stop button when no other operations are active', () => {
            chat.executionActive = false;
            chat.approvalPending = false;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.aiStopBtn.disabled).toBe(true);
        });

        it('does not disable the AI stop button when execution is still active', () => {
            chat.executionActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.aiStopBtn.disabled).toBe(false);
        });

        it('does not disable the AI stop button when thinking is still active', () => {
            chat.thinkingManager = { thinkingActive: true };

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.aiStopBtn.disabled).toBe(false);
        });

        it('does not disable the AI stop button when approval is pending', () => {
            chat.approvalPending = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.aiStopBtn.disabled).toBe(false);
        });
    });

    describe('handleResponseComplete — no streamed content, data.content fallback', () => {
        it('calls appendAIResponse with rendered content when streamingContent is absent', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'Full response body',
            });

            expect(terminalSpy.appendAIResponse).toHaveBeenCalledOnce();
        });

        it('does not call appendAIResponse when content is also absent', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(terminalSpy.appendAIResponse).not.toHaveBeenCalled();
        });
    });

    describe('handleResponseComplete — pending citations', () => {
        it('applies pending citations after finalizeAIResponseChunk', () => {
            const metadata = { grounding_used: true, sources: [] };
            chat.streamingContent.set(WEB_SESSION_ID, 'streamed text');
            if (!chat.pendingCitations) chat.pendingCitations = new Map();
            chat.pendingCitations.set(WEB_SESSION_ID, metadata);

            const callOrder = [];
            terminalSpy.finalizeAIResponseChunk.mockImplementation(() => callOrder.push('finalize'));
            terminalSpy.applyCitationsAfterFinalize.mockImplementation(() => callOrder.push('citations'));

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
            expect(terminalSpy.applyCitationsAfterFinalize).toHaveBeenCalledWith(WEB_SESSION_ID, metadata);
            expect(callOrder).toEqual(['finalize', 'citations']);
            expect(chat.pendingCitations.has(WEB_SESSION_ID)).toBe(false);
        });

        it('clears pendingCitations entry for the session on completion', () => {
            const metadata = { grounding_used: true, sources: [] };
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            if (!chat.pendingCitations) chat.pendingCitations = new Map();
            chat.pendingCitations.set(WEB_SESSION_ID, metadata);

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.pendingCitations.has(WEB_SESSION_ID)).toBe(false);
        });
    });

    describe('handleResponseComplete — event bus routing', () => {
        it('triggers finalization when RESPONSE_COMPLETE event fires with matching ids', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat.streamingActive = true;

            eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.streamingActive).toBe(false);
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(false);
            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
        });

        it('does not trigger finalization when RESPONSE_COMPLETE investigation_id mismatches', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat.streamingActive = true;

            eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: 'wrong-id',
            });

            expect(chat.streamingActive).toBe(true);
            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
        });
    });

    describe('handleResponseComplete — TEXT_TRUNCATED event bus routing', () => {
        it('cleans up streaming state on RESPONSE_TRUNCATED', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat.streamingActive = true;

            eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_TRUNCATED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                finish_reason: 'MAX_TOKENS',
            });

            expect(chat.streamingActive).toBe(false);
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(false);
            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
        });
    });

    describe('_finalizeInterTurnBubble — tool call inter-turn boundary', () => {
        it('calls finalizeAIResponseChunk with rendered markdown when there is streamed content for the session', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'pre-tool text');

            chat._finalizeInterTurnBubble(WEB_SESSION_ID);

            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
            expect(terminalSpy.finalizeAIResponseChunk.mock.calls[0][0]).toBe(WEB_SESSION_ID);
            expect(typeof terminalSpy.finalizeAIResponseChunk.mock.calls[0][1]).toBe('string');
        });

        it('clears streamingContent for the session so the next turn starts a fresh bubble', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'pre-tool text');

            chat._finalizeInterTurnBubble(WEB_SESSION_ID);

            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(false);
        });

        it('does not call finalizeAIResponseChunk when there is no streamed content (no bubble to seal)', () => {
            chat._finalizeInterTurnBubble(WEB_SESSION_ID);

            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
        });

        it('does nothing when webSessionId is falsy', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');

            chat._finalizeInterTurnBubble(null);

            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(true);
            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
        });

        it('seals the bubble and clears streamingContent when called with accumulated content', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'pre-tool text');

            chat._finalizeInterTurnBubble(WEB_SESSION_ID);

            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(false);
        });

        it('post-function-call text chunks accumulate fresh after TOOL_RESULT seals the bubble', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'turn one text');

            chat._finalizeInterTurnBubble(WEB_SESSION_ID);

            const chunk = {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'turn two text',
            };
            chat.handleAITextChunk(chunk);

            expect(chat.streamingContent.get(WEB_SESSION_ID)).toBe('turn two text');
        });
    });

    describe('handleTurnComplete — CHAT.TURN_COMPLETE event bus routing', () => {
        it('seals the current bubble with rendered markdown when TURN_COMPLETE fires with matching investigation_id', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'pre-tool text');

            eventBus.emit(EventType.LLM_CHAT_ITERATION_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                turn: 1,
            });

            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
            expect(terminalSpy.finalizeAIResponseChunk.mock.calls[0][0]).toBe(WEB_SESSION_ID);
            expect(typeof terminalSpy.finalizeAIResponseChunk.mock.calls[0][1]).toBe('string');
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(false);
        });

        it('does not seal the bubble when TURN_COMPLETE investigation_id mismatches', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');

            eventBus.emit(EventType.LLM_CHAT_ITERATION_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: 'wrong-investigation',
                turn: 1,
            });

            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
            expect(chat.streamingContent.has(WEB_SESSION_ID)).toBe(true);
        });

        it('does nothing when TURN_COMPLETE has no web_session_id', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');

            eventBus.emit(EventType.LLM_CHAT_ITERATION_COMPLETED, {
                investigation_id: INVESTIGATION_ID,
                turn: 1,
            });

            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
        });

        it('does not call finalizeAIResponseChunk when there is no pre-tool streamed content', () => {
            eventBus.emit(EventType.LLM_CHAT_ITERATION_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                turn: 1,
            });

            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
        });

        it('post-turn text chunks open a fresh bubble after TURN_COMPLETE seals the previous one', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'turn one text');

            eventBus.emit(EventType.LLM_CHAT_ITERATION_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                turn: 1,
            });

            chat.handleAITextChunk({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'turn two text',
            });

            // Advance timers to trigger debounced function
            vi.advanceTimersByTime(150);

            expect(chat.streamingContent.get(WEB_SESSION_ID)).toBe('turn two text');
            const [, html] = terminalSpy.appendAIResponseChunk.mock.calls[0];
            expect(html).toContain('turn two text');
            expect(html).toContain('streaming-cursor');
        });
    });

    describe('handleResponseComplete — indicator map cleanup', () => {
        it('clears _searchWebIndicators on finalize', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat._searchWebIndicators = new Map([['exec-1', 'search-web-exec-1']]);

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat._searchWebIndicators.size).toBe(0);
        });

        it('clears _portCheckIndicators on finalize', () => {
            chat.streamingContent.set(WEB_SESSION_ID, 'content');
            chat._portCheckIndicators = new Map([['exec-2', 'port-check-exec-2']]);

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat._portCheckIndicators.size).toBe(0);
        });
    });

    describe('session isolation — only the completing session is cleaned up', () => {
        const SESSION_A = 'session-a';
        const SESSION_B = 'session-b';

        it('removes only the completed session from streamingContent', () => {
            chat.streamingContent.set(SESSION_A, 'content A');
            chat.streamingContent.set(SESSION_B, 'content B');

            chat.handleResponseComplete({
                web_session_id: SESSION_A,
                investigation_id: INVESTIGATION_ID,
            });

            expect(chat.streamingContent.has(SESSION_A)).toBe(false);
            expect(chat.streamingContent.has(SESSION_B)).toBe(true);
        });

        it('calls finalizeAIResponseChunk only for the completing session', () => {
            chat.streamingContent.set(SESSION_A, 'content A');
            chat.streamingContent.set(SESSION_B, 'content B');

            chat.handleResponseComplete({
                web_session_id: SESSION_A,
                investigation_id: INVESTIGATION_ID,
            });

            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
            expect(terminalSpy.finalizeAIResponseChunk.mock.calls[0][0]).toBe(SESSION_A);
        });
    });
});

describe('ChatComponent — debounce cancellation on finalize [FRONTEND - jsdom]', () => {
    let ChatComponent;
    let eventBus;
    let chat;
    let terminalSpy;
    let authState;
    let serviceClient;

    beforeEach(async () => {
        buildDOM();

        authState = new MockAuthState();
        authState.setAuthenticated({ id: WEB_SESSION_ID });
        authState.loading = false;
        authState.getWebSessionModel = () => ({ id: WEB_SESSION_ID });
        authState.getWebSessionId = () => WEB_SESSION_ID;

        serviceClient = new MockServiceClient();
        installGlobals(authState, serviceClient);

        eventBus = new MockEventBus();
        ({ ChatComponent } = await import('@g8ed/public/js/components/chat.js'));

        chat = new ChatComponent(eventBus);
        chat.setupSSEListeners();
        terminalSpy = makeAnchoredTerminalSpy();
        chat.anchoredTerminal = terminalSpy;
        chat.casesManager = makeCasesManagerStub();
        chat.currentUser = { id: WEB_SESSION_ID };
        chat.currentWebSessionId = WEB_SESSION_ID;
    });

    afterEach(() => {
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    it('handleResponseComplete cancels the debounced render', () => {
        const cancelSpy = vi.spyOn(chat._debouncedRenderChunk, 'cancel');
        chat.streamingContent.set(WEB_SESSION_ID, 'text');

        chat.handleResponseComplete({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
        });

        expect(cancelSpy).toHaveBeenCalledOnce();
    });

    it('_finalizeInterTurnBubble cancels the debounced render', () => {
        const cancelSpy = vi.spyOn(chat._debouncedRenderChunk, 'cancel');
        chat.streamingContent.set(WEB_SESSION_ID, 'text');

        chat._finalizeInterTurnBubble(WEB_SESSION_ID);

        expect(cancelSpy).toHaveBeenCalledOnce();
    });

    it('handleChatError cancels the debounced render', () => {
        const cancelSpy = vi.spyOn(chat._debouncedRenderChunk, 'cancel');

        chat.handleChatError({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            error: 'test error',
        });

        expect(cancelSpy).toHaveBeenCalledOnce();
    });

    it('_handleLLMChatIterationFailed cancels the debounced render', () => {
        const cancelSpy = vi.spyOn(chat._debouncedRenderChunk, 'cancel');

        chat._handleLLMChatIterationFailed({
            web_session_id: WEB_SESSION_ID,
            error: 'test error',
        });

        expect(cancelSpy).toHaveBeenCalledOnce();
    });
});

describe('ChatComponent — clearChat indicator map cleanup [FRONTEND - jsdom]', () => {
    let ChatComponent;
    let eventBus;
    let chat;
    let terminalSpy;
    let authState;
    let serviceClient;

    beforeEach(async () => {
        buildDOM();

        authState = new MockAuthState();
        authState.setAuthenticated({ id: WEB_SESSION_ID });
        authState.loading = false;
        authState.getWebSessionModel = () => ({ id: WEB_SESSION_ID });
        authState.getWebSessionId = () => WEB_SESSION_ID;

        serviceClient = new MockServiceClient();
        installGlobals(authState, serviceClient);

        eventBus = new MockEventBus();
        ({ ChatComponent } = await import('@g8ed/public/js/components/chat.js'));

        chat = new ChatComponent(eventBus);
        terminalSpy = makeAnchoredTerminalSpy();
        chat.anchoredTerminal = terminalSpy;
        chat.casesManager = makeCasesManagerStub();
        chat.currentUser = { id: WEB_SESSION_ID };
        chat.currentWebSessionId = WEB_SESSION_ID;
    });

    afterEach(() => {
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    it('clears _searchWebIndicators when clearChat is called', () => {
        chat._searchWebIndicators = new Map([['exec-1', 'search-web-exec-1']]);

        chat.clearChat();

        expect(chat._searchWebIndicators.size).toBe(0);
    });

    it('clears _portCheckIndicators when clearChat is called', () => {
        chat._portCheckIndicators = new Map([['exec-2', 'port-check-exec-2']]);

        chat.clearChat();

        expect(chat._portCheckIndicators.size).toBe(0);
    });

    it('does not throw when indicator maps are uninitialised at clearChat time', () => {
        chat._searchWebIndicators = undefined;
        chat._portCheckIndicators = undefined;

        expect(() => chat.clearChat()).not.toThrow();
    });
});
