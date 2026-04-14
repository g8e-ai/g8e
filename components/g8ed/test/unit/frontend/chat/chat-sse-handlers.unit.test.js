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
import { makeAnchoredTerminalSpy } from '@test/utils/test-helpers.js';

const INVESTIGATION_ID = 'inv-dispatch-abc123';
const WEB_SESSION_ID = 'session-dispatch-abc123';

function buildDOM() {
    document.body.innerHTML = `
        <div id="messages-container"></div>
        <div id="chat-status"></div>
        <button id="ai-stop-btn" disabled></button>
        <div id="anchored-terminal-body" style="height:400px;overflow:auto;"></div>
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

function makeCasesManagerStub(investigationId = INVESTIGATION_ID) {
    return {
        getCurrentInvestigationId: () => investigationId,
        getCurrentCaseId: () => 'case-dispatch-abc123',
        getCurrentTaskId: () => null,
        _applyCaseCreationResult: vi.fn(),
        init: vi.fn(),
    };
}

describe('ChatComponent — handleChatError [FRONTEND - jsdom]', () => {
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
        chat.streamingActive = true;
        chat.aiStopBtn = document.getElementById('ai-stop-btn');
        chat.aiStopBtn.disabled = false;
    });

    afterEach(() => {
        vi.useRealTimers();
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    it('routes to anchoredTerminal.appendErrorMessage when user is authenticated', () => {
        chat.handleChatError({
            error: 'AI service unavailable',
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(terminalSpy.appendErrorMessage).toHaveBeenCalledOnce();
    });

    it('includes the error string in the message', () => {
        chat.handleChatError({
            error: 'context window exceeded',
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        const [msg] = terminalSpy.appendErrorMessage.mock.calls[0];
        expect(msg).toContain('context window exceeded');
    });

    it('uses the default error text when data.error is absent', () => {
        chat.handleChatError({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        const [msg] = terminalSpy.appendErrorMessage.mock.calls[0];
        expect(msg).toContain('AI session encountered an error');
    });

    it('includes raw_error details when present and different from error', () => {
        chat.handleChatError({
            error: 'AI service error',
            raw_error: 'TimeoutError: deadline exceeded',
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        const [msg] = terminalSpy.appendErrorMessage.mock.calls[0];
        expect(msg).toContain('TimeoutError: deadline exceeded');
    });

    it('does not duplicate raw_error when it equals the error text', () => {
        chat.handleChatError({
            error: 'same text',
            raw_error: 'same text',
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        const [msg] = terminalSpy.appendErrorMessage.mock.calls[0];
        const occurrences = msg.split('same text').length - 1;
        expect(occurrences).toBe(1);
    });

    it('includes retry attempt count from metadata', () => {
        chat.handleChatError({
            error: 'failed',
            metadata: { attempts: 3, max_attempts: 5 },
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        const [msg] = terminalSpy.appendErrorMessage.mock.calls[0];
        expect(msg).toContain('3');
        expect(msg).toContain('5');
    });

    it('includes backoff info when present in metadata', () => {
        chat.handleChatError({
            error: 'failed',
            metadata: { attempts: 2, initial_backoff_seconds: 1, backoff_multiplier: 2 },
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        const [msg] = terminalSpy.appendErrorMessage.mock.calls[0];
        expect(msg).toContain('1s');
        expect(msg).toContain('x2');
    });

    it('rejects event when investigation_id is missing', () => {
        chat.handleChatError({
            error: 'err',
            web_session_id: WEB_SESSION_ID,
        });

        expect(terminalSpy.appendErrorMessage).not.toHaveBeenCalled();
        expect(chat.streamingActive).toBe(true);
    });

    it('rejects event when investigation_id does not match', () => {
        chat.handleChatError({
            error: 'err',
            investigation_id: 'wrong-investigation',
            web_session_id: WEB_SESSION_ID,
        });

        expect(terminalSpy.appendErrorMessage).not.toHaveBeenCalled();
    });

    it('does not throw when called with an empty object', () => {
        expect(() => chat.handleChatError({})).not.toThrow();
    });

    it('does not throw when called with no argument', () => {
        expect(() => chat.handleChatError()).not.toThrow();
    });

    it('removes streaming class from ai-response element when web_session_id is present', () => {
        const aiResponseDiv = document.createElement('div');
        aiResponseDiv.id = `ai-response-${WEB_SESSION_ID}`;
        aiResponseDiv.classList.add('streaming');
        const cursor = document.createElement('span');
        cursor.classList.add('streaming-cursor');
        aiResponseDiv.appendChild(cursor);
        document.body.appendChild(aiResponseDiv);

        chat.handleChatError({
            error: 'err',
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(aiResponseDiv.classList.contains('streaming')).toBe(false);
        expect(aiResponseDiv.querySelectorAll('.streaming-cursor').length).toBe(0);
    });

    it('integration: ITERATION_FAILED event calls both handleChatError and _handleLLMChatIterationFailed', () => {
        chat.setupSSEListeners();
        const handleChatErrorSpy = vi.spyOn(chat, 'handleChatError');
        const handleLLMChatIterationFailedSpy = vi.spyOn(chat, '_handleLLMChatIterationFailed');

        eventBus.emit(EventType.LLM_CHAT_ITERATION_FAILED, {
            error: 'Test error',
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(handleChatErrorSpy).toHaveBeenCalledOnce();
        expect(handleLLMChatIterationFailedSpy).toHaveBeenCalledOnce();
    });
});

describe('ChatComponent — handleAITextChunk edge cases [FRONTEND - jsdom]', () => {
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
        vi.useRealTimers();
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    it('hides the thinking indicator when thinking is active at chunk arrival', () => {
        const hideThinkingIndicator = vi.fn();
        chat.thinkingManager = { thinkingActive: true, hideThinkingIndicator };

        chat.handleAITextChunk({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            content: 'first text chunk',
        });

        expect(hideThinkingIndicator).toHaveBeenCalledOnce();
        expect(hideThinkingIndicator).toHaveBeenCalledWith(WEB_SESSION_ID);
    });

    it('calls hideThinkingIndicator even when thinking is not active', () => {
        const hideThinkingIndicator = vi.fn();
        chat.thinkingManager = { thinkingActive: false, hideThinkingIndicator };

        chat.handleAITextChunk({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            content: 'chunk',
        });

        expect(hideThinkingIndicator).toHaveBeenCalledOnce();
        expect(hideThinkingIndicator).toHaveBeenCalledWith(WEB_SESSION_ID);
    });

    it('sets streamingActive to true on first chunk', () => {
        chat.streamingActive = false;

        chat.handleAITextChunk({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            content: 'chunk',
        });

        expect(chat.streamingActive).toBe(true);
    });

    it('ignores chunks with no content', () => {
        chat.handleAITextChunk({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
        });

        expect(terminalSpy.appendStreamingTextChunk).not.toHaveBeenCalled();
    });

    it('passes raw text directly to appendStreamingTextChunk', () => {
        chat.handleAITextChunk({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            content: 'raw text chunk',
        });

        expect(terminalSpy.appendStreamingTextChunk).toHaveBeenCalledOnce();
        const [, text] = terminalSpy.appendStreamingTextChunk.mock.calls[0];
        expect(text).toBe('raw text chunk');
    });

    it('ignores chunks when web_session_id is absent', () => {
        chat.handleAITextChunk({
            investigation_id: INVESTIGATION_ID,
            content: 'orphan chunk',
        });

        expect(terminalSpy.appendStreamingTextChunk).not.toHaveBeenCalled();
    });

    it('event bus wiring: LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED triggers handleAITextChunk', () => {
        chat.setupSSEListeners();

        eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, {
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            content: 'streamed token',
        });

        expect(terminalSpy.appendStreamingTextChunk).toHaveBeenCalledOnce();
        const [, text] = terminalSpy.appendStreamingTextChunk.mock.calls[0];
        expect(text).toBe('streamed token');
    });
});

describe('ChatComponent — handleChatStopped [FRONTEND - jsdom]', () => {
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
        chat.streamingActive = true;
        chat.approvalPending = false;
        chat.executionActive = false;
        chat.aiStopBtn = document.getElementById('ai-stop-btn');
        chat.aiStopBtn.disabled = false;
    });

    afterEach(() => {
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    it('sets streamingActive to false', () => {
        chat.handleChatStopped({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(chat.streamingActive).toBe(false);
    });

    it('sets approvalPending to false', () => {
        chat.approvalPending = true;

        chat.handleChatStopped({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(chat.approvalPending).toBe(false);
    });

    it('hides the stop button when no execution is active', () => {
        chat.executionActive = false;

        chat.handleChatStopped({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(chat.aiStopBtn.disabled).toBe(true);
    });

    it('does not hide stop button when execution is still active', () => {
        chat.executionActive = true;

        chat.handleChatStopped({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(chat.aiStopBtn.disabled).toBe(false);
    });

    it('rejects event when investigation_id is missing', () => {
        chat.handleChatStopped({ web_session_id: WEB_SESSION_ID });

        expect(chat.streamingActive).toBe(true);
    });

    it('rejects event when investigation_id does not match', () => {
        chat.handleChatStopped({
            investigation_id: 'wrong-investigation',
            web_session_id: WEB_SESSION_ID,
        });

        expect(chat.streamingActive).toBe(true);
    });

    it('event bus wiring: LLM_CHAT_ITERATION_STOPPED triggers handleChatStopped', () => {
        chat.setupSSEListeners();

        eventBus.emit(EventType.LLM_CHAT_ITERATION_STOPPED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            reason: 'User requested stop',
        });

        expect(chat.streamingActive).toBe(false);
        expect(chat.aiStopBtn.disabled).toBe(true);
    });

    it('does not throw when called with an empty object', () => {
        expect(() => chat.handleChatStopped({})).not.toThrow();
    });

    it('calls hideWaitingIndicator when anchoredTerminal is present', () => {
        chat.handleChatStopped({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(terminalSpy.hideWaitingIndicator).toHaveBeenCalledOnce();
    });

    it('removes streaming class from ai-response element when web_session_id is present', () => {
        const aiResponseDiv = document.createElement('div');
        aiResponseDiv.id = `ai-response-${WEB_SESSION_ID}`;
        aiResponseDiv.classList.add('streaming');
        const cursor = document.createElement('span');
        cursor.classList.add('streaming-cursor');
        aiResponseDiv.appendChild(cursor);
        document.body.appendChild(aiResponseDiv);

        chat.handleChatStopped({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(aiResponseDiv.classList.contains('streaming')).toBe(false);
        expect(aiResponseDiv.querySelectorAll('.streaming-cursor').length).toBe(0);
    });
});

describe('ChatComponent — handleIterationStarted [FRONTEND - jsdom]', () => {
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

    it('calls showWaitingIndicator when investigation_id matches', () => {
        chat.handleIterationStarted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(terminalSpy.showWaitingIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.showWaitingIndicator).toHaveBeenCalledWith(WEB_SESSION_ID);
    });

    it('rejects event when investigation_id is missing', () => {
        chat.handleIterationStarted({
            web_session_id: WEB_SESSION_ID,
        });

        expect(terminalSpy.showWaitingIndicator).not.toHaveBeenCalled();
    });

    it('rejects event when investigation_id does not match', () => {
        chat.handleIterationStarted({
            investigation_id: 'wrong-investigation',
            web_session_id: WEB_SESSION_ID,
        });

        expect(terminalSpy.showWaitingIndicator).not.toHaveBeenCalled();
    });

    it('rejects event when web_session_id is missing', () => {
        chat.handleIterationStarted({
            investigation_id: INVESTIGATION_ID,
        });

        expect(terminalSpy.showWaitingIndicator).not.toHaveBeenCalled();
    });

    it('event bus wiring: LLM_CHAT_ITERATION_STARTED triggers handleIterationStarted', () => {
        chat.setupSSEListeners();

        eventBus.emit(EventType.LLM_CHAT_ITERATION_STARTED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(terminalSpy.showWaitingIndicator).toHaveBeenCalledOnce();
    });
});

describe('ChatComponent — addRestoredMessage [FRONTEND - jsdom]', () => {
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

    it('routes USER_CHAT messages to appendUserMessage', () => {
        chat.addRestoredMessage('Hello', EventType.EVENT_SOURCE_USER_CHAT, new Date().toISOString());

        expect(terminalSpy.appendUserMessage).toHaveBeenCalledOnce();
        expect(terminalSpy.appendDirectHtmlResponse).not.toHaveBeenCalled();
    });

    it('passes the message content to appendUserMessage', () => {
        chat.addRestoredMessage('Good morning', EventType.EVENT_SOURCE_USER_CHAT, null);

        const [content] = terminalSpy.appendUserMessage.mock.calls[0];
        expect(content).toBe('Good morning');
    });

    it('formats originalTimestamp for display when present', () => {
        const ts = '2026-03-15T12:09:30.000Z';
        chat.addRestoredMessage('msg', EventType.EVENT_SOURCE_USER_CHAT, ts);

        const [, displayTime] = terminalSpy.appendUserMessage.mock.calls[0];
        expect(displayTime).not.toBeNull();
        expect(typeof displayTime).toBe('string');
    });

    it('passes null displayTime when originalTimestamp is falsy', () => {
        chat.addRestoredMessage('msg', EventType.EVENT_SOURCE_USER_CHAT, null);

        const [, displayTime] = terminalSpy.appendUserMessage.mock.calls[0];
        expect(displayTime).toBeNull();
    });

    it('routes AI_PRIMARY messages to appendDirectHtmlResponse', () => {
        chat.addRestoredMessage('AI reply', EventType.EVENT_SOURCE_AI_PRIMARY, null);

        expect(terminalSpy.appendDirectHtmlResponse).toHaveBeenCalledOnce();
        expect(terminalSpy.appendUserMessage).not.toHaveBeenCalled();
    });

    it('renders AI content through messageRenderer.renderContent before passing to terminal', () => {
        const renderContent = vi.fn(() => '<p>rendered</p>');
        chat.messageRenderer = { renderContent };

        chat.addRestoredMessage('**bold**', EventType.EVENT_SOURCE_AI_PRIMARY, null);

        expect(renderContent).toHaveBeenCalledWith('**bold**');
        const [html] = terminalSpy.appendDirectHtmlResponse.mock.calls[0];
        expect(html).toBe('<p>rendered</p>');
    });

    it('passes groundingMetadata to appendDirectHtmlResponse', () => {
        const metadata = { grounding_used: true, sources: [{ url: 'x.com', title: 'X' }] };

        chat.addRestoredMessage('AI reply', EventType.EVENT_SOURCE_AI_PRIMARY, null, null, metadata);

        const [, , receivedMetadata] = terminalSpy.appendDirectHtmlResponse.mock.calls[0];
        expect(receivedMetadata).toBe(metadata);
    });

    it('routes SYSTEM messages to appendSystemMessage', () => {
        chat.addRestoredMessage('System event', EventType.EVENT_SOURCE_SYSTEM, null);

        expect(terminalSpy.appendSystemMessage).toHaveBeenCalledOnce();
        expect(terminalSpy.appendUserMessage).not.toHaveBeenCalled();
        expect(terminalSpy.appendDirectHtmlResponse).not.toHaveBeenCalled();
    });

    it('passes the message content to appendSystemMessage', () => {
        chat.addRestoredMessage('Note: operator connected', EventType.EVENT_SOURCE_SYSTEM, null);

        const [content] = terminalSpy.appendSystemMessage.mock.calls[0];
        expect(content).toBe('Note: operator connected');
    });

    it('does nothing when anchoredTerminal is null', () => {
        chat.anchoredTerminal = null;

        expect(() => chat.addRestoredMessage('msg', EventType.EVENT_SOURCE_USER_CHAT, null)).not.toThrow();
    });

    it('does nothing for an unknown sender', () => {
        chat.addRestoredMessage('msg', 'unknown.sender', null);

        expect(terminalSpy.appendUserMessage).not.toHaveBeenCalled();
        expect(terminalSpy.appendDirectHtmlResponse).not.toHaveBeenCalled();
        expect(terminalSpy.appendSystemMessage).not.toHaveBeenCalled();
    });
});

describe('ChatComponent — handleSearchWebIndicator / handleSearchWebCompleted / handleSearchWebFailed [FRONTEND - jsdom]', () => {
    let ChatComponent;
    let eventBus;
    let chat;
    let terminalSpy;
    let authState;
    let serviceClient;

    const EXECUTION_ID = 'exec-search-001';

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

    it('appends an activity indicator with the correct execution_id-based id', () => {
        chat.handleSearchWebIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            query: 'CVE-2025-1234',
        });

        expect(terminalSpy.appendActivityIndicator).toHaveBeenCalledOnce();
        const [opts] = terminalSpy.appendActivityIndicator.mock.calls[0];
        expect(opts.id).toBe(`search-web-${EXECUTION_ID}`);
    });

    it('passes the query as detail to appendActivityIndicator', () => {
        chat.handleSearchWebIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            query: 'exploit mitigation',
        });

        const [opts] = terminalSpy.appendActivityIndicator.mock.calls[0];
        expect(opts.detail).toBe('exploit mitigation');
    });

    it('tracks the indicator id keyed by execution_id', () => {
        chat.handleSearchWebIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            query: 'test query',
        });

        expect(chat._searchWebIndicators.get(EXECUTION_ID)).toBe(`search-web-${EXECUTION_ID}`);
    });

    it('does not append indicator when execution_id is absent', () => {
        chat.handleSearchWebIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            query: 'test query',
        });

        expect(terminalSpy.appendActivityIndicator).not.toHaveBeenCalled();
    });

    it('does not append indicator when query is absent (model violation)', () => {
        chat.handleSearchWebIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.appendActivityIndicator).not.toHaveBeenCalled();
    });

    it('handleSearchWebCompleted calls completeActivityIndicator with the tracked id', () => {
        chat._searchWebIndicators = new Map([[EXECUTION_ID, `search-web-${EXECUTION_ID}`]]);

        chat.handleSearchWebCompleted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith(`search-web-${EXECUTION_ID}`);
    });

    it('handleSearchWebCompleted removes the execution_id from the tracking map', () => {
        chat._searchWebIndicators = new Map([[EXECUTION_ID, `search-web-${EXECUTION_ID}`]]);

        chat.handleSearchWebCompleted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(chat._searchWebIndicators.has(EXECUTION_ID)).toBe(false);
    });

    it('handleSearchWebFailed calls completeActivityIndicator with the tracked id', () => {
        chat._searchWebIndicators = new Map([[EXECUTION_ID, `search-web-${EXECUTION_ID}`]]);

        chat.handleSearchWebFailed({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith(`search-web-${EXECUTION_ID}`);
    });

    it('handleSearchWebFailed removes the execution_id from the tracking map', () => {
        chat._searchWebIndicators = new Map([[EXECUTION_ID, `search-web-${EXECUTION_ID}`]]);

        chat.handleSearchWebFailed({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(chat._searchWebIndicators.has(EXECUTION_ID)).toBe(false);
    });

    it('handleSearchWebCompleted is a no-op when no indicator was registered for the execution_id', () => {
        chat.handleSearchWebCompleted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).not.toHaveBeenCalled();
    });

    it('rejects REQUESTED event when investigation_id mismatches', () => {
        chat.handleSearchWebIndicator({
            investigation_id: 'wrong-inv',
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.appendActivityIndicator).not.toHaveBeenCalled();
    });

    it('rejects COMPLETED event when investigation_id mismatches', () => {
        chat._searchWebIndicators = new Map([[EXECUTION_ID, `search-web-${EXECUTION_ID}`]]);

        chat.handleSearchWebCompleted({
            investigation_id: 'wrong-inv',
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).not.toHaveBeenCalled();
        expect(chat._searchWebIndicators.has(EXECUTION_ID)).toBe(true);
    });

    it('multiple concurrent search indicators are tracked independently', () => {
        chat.handleSearchWebIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: 'exec-a',
            query: 'query A',
        });
        chat.handleSearchWebIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: 'exec-b',
            query: 'query B',
        });

        chat.handleSearchWebCompleted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: 'exec-a',
        });

        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith('search-web-exec-a');
        expect(chat._searchWebIndicators.has('exec-b')).toBe(true);
    });

    it('G8E_SEARCH_WEB event bus wiring: REQUESTED → COMPLETED completes the indicator', () => {
        chat.setupSSEListeners();

        eventBus.emit(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            query: 'test query',
        });

        eventBus.emit(EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.appendActivityIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith(`search-web-${EXECUTION_ID}`);
    });

    it('G8E_SEARCH_WEB event bus wiring: REQUESTED → FAILED completes the indicator', () => {
        chat.setupSSEListeners();

        eventBus.emit(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            query: 'test query',
        });

        eventBus.emit(EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith(`search-web-${EXECUTION_ID}`);
    });
});

describe('ChatComponent — handleTribunalStarted [FRONTEND - jsdom]', () => {
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
        chat.streamingActive = true;
        chat.aiStopBtn = document.getElementById('ai-stop-btn');
        chat.aiStopBtn.disabled = false;
    });

    afterEach(() => {
        vi.useRealTimers();
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    it('calls sealStreamingResponse when streaming is active', () => {
        chat.handleTribunalStarted({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            primary_model: 'claude-4-sonnet',
            num_passes: 3,
            original_command: 'uptime',
        });

        expect(terminalSpy.sealStreamingResponse).toHaveBeenCalledOnce();
        expect(terminalSpy.sealStreamingResponse).toHaveBeenCalledWith(WEB_SESSION_ID);
    });

    it('sets streamingActive to false', () => {
        chat.handleTribunalStarted({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            primary_model: 'claude-4-sonnet',
            num_passes: 3,
            original_command: 'uptime',
        });

        expect(chat.streamingActive).toBe(false);
    });

    it('does not call sealStreamingResponse when streaming is not active', () => {
        chat.streamingActive = false;

        chat.handleTribunalStarted({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            primary_model: 'claude-4-sonnet',
            num_passes: 3,
            original_command: 'uptime',
        });

        expect(terminalSpy.sealStreamingResponse).not.toHaveBeenCalled();
    });

    it('clears activity indicators and tracking maps', () => {
        chat._searchWebIndicators = new Map([['exec-1', 'search-web-exec-1']]);
        chat._portCheckIndicators = new Map([['exec-2', 'port-check-exec-2']]);
        chat._hasResetAutoScrollForSession = new Set([WEB_SESSION_ID]);

        chat.handleTribunalStarted({
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            primary_model: 'claude-4-sonnet',
            num_passes: 3,
            original_command: 'uptime',
        });

        expect(terminalSpy.clearActivityIndicators).toHaveBeenCalledOnce();
        expect(chat._searchWebIndicators.size).toBe(0);
        expect(chat._portCheckIndicators.size).toBe(0);
        expect(chat._hasResetAutoScrollForSession.has(WEB_SESSION_ID)).toBe(false);
    });

    it('event bus wiring: TRIBUNAL_SESSION_STARTED triggers sealStreamingResponse', () => {
        chat.setupSSEListeners();

        eventBus.emit(EventType.TRIBUNAL_SESSION_STARTED, {
            web_session_id: WEB_SESSION_ID,
            investigation_id: INVESTIGATION_ID,
            primary_model: 'claude-4-sonnet',
            num_passes: 3,
            original_command: 'uptime',
        });

        expect(terminalSpy.sealStreamingResponse).toHaveBeenCalledOnce();
        expect(terminalSpy.sealStreamingResponse).toHaveBeenCalledWith(WEB_SESSION_ID);
    });
});

describe('ChatComponent — handleNetworkPortCheckIndicator / handleNetworkPortCheckCompleted / handleNetworkPortCheckFailed [FRONTEND - jsdom]', () => {
    let ChatComponent;
    let eventBus;
    let chat;
    let terminalSpy;
    let authState;
    let serviceClient;

    const EXECUTION_ID = 'exec-port-001';

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

    it('appends an activity indicator with the correct execution_id-based id', () => {
        chat.handleNetworkPortCheckIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            port: '443',
        });

        expect(terminalSpy.appendActivityIndicator).toHaveBeenCalledOnce();
        const [opts] = terminalSpy.appendActivityIndicator.mock.calls[0];
        expect(opts.id).toBe(`port-check-${EXECUTION_ID}`);
    });

    it('passes the port as detail to appendActivityIndicator', () => {
        chat.handleNetworkPortCheckIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            port: '8443',
        });

        const [opts] = terminalSpy.appendActivityIndicator.mock.calls[0];
        expect(opts.detail).toBe('8443');
    });

    it('tracks the indicator id keyed by execution_id', () => {
        chat.handleNetworkPortCheckIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            port: '443',
        });

        expect(chat._portCheckIndicators.get(EXECUTION_ID)).toBe(`port-check-${EXECUTION_ID}`);
    });

    it('does not append indicator when execution_id is absent', () => {
        chat.handleNetworkPortCheckIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            port: '443',
        });

        expect(terminalSpy.appendActivityIndicator).not.toHaveBeenCalled();
    });

    it('does not append indicator when port is absent (model violation)', () => {
        chat.handleNetworkPortCheckIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.appendActivityIndicator).not.toHaveBeenCalled();
    });

    it('handleNetworkPortCheckCompleted calls completeActivityIndicator with the tracked id', () => {
        chat._portCheckIndicators = new Map([[EXECUTION_ID, `port-check-${EXECUTION_ID}`]]);

        chat.handleNetworkPortCheckCompleted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith(`port-check-${EXECUTION_ID}`);
    });

    it('handleNetworkPortCheckCompleted removes the execution_id from the tracking map', () => {
        chat._portCheckIndicators = new Map([[EXECUTION_ID, `port-check-${EXECUTION_ID}`]]);

        chat.handleNetworkPortCheckCompleted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(chat._portCheckIndicators.has(EXECUTION_ID)).toBe(false);
    });

    it('handleNetworkPortCheckFailed calls completeActivityIndicator with the tracked id', () => {
        chat._portCheckIndicators = new Map([[EXECUTION_ID, `port-check-${EXECUTION_ID}`]]);

        chat.handleNetworkPortCheckFailed({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith(`port-check-${EXECUTION_ID}`);
    });

    it('handleNetworkPortCheckFailed removes the execution_id from the tracking map', () => {
        chat._portCheckIndicators = new Map([[EXECUTION_ID, `port-check-${EXECUTION_ID}`]]);

        chat.handleNetworkPortCheckFailed({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(chat._portCheckIndicators.has(EXECUTION_ID)).toBe(false);
    });

    it('handleNetworkPortCheckCompleted is a no-op when no indicator was registered for the execution_id', () => {
        chat.handleNetworkPortCheckCompleted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).not.toHaveBeenCalled();
    });

    it('rejects REQUESTED event when investigation_id mismatches', () => {
        chat.handleNetworkPortCheckIndicator({
            investigation_id: 'wrong-inv',
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.appendActivityIndicator).not.toHaveBeenCalled();
    });

    it('rejects COMPLETED event when investigation_id mismatches', () => {
        chat._portCheckIndicators = new Map([[EXECUTION_ID, `port-check-${EXECUTION_ID}`]]);

        chat.handleNetworkPortCheckCompleted({
            investigation_id: 'wrong-inv',
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).not.toHaveBeenCalled();
        expect(chat._portCheckIndicators.has(EXECUTION_ID)).toBe(true);
    });

    it('multiple concurrent port-check indicators are tracked independently', () => {
        chat.handleNetworkPortCheckIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: 'exec-p1',
            port: '80',
        });
        chat.handleNetworkPortCheckIndicator({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: 'exec-p2',
            port: '443',
        });

        chat.handleNetworkPortCheckCompleted({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: 'exec-p1',
        });

        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith('port-check-exec-p1');
        expect(chat._portCheckIndicators.has('exec-p2')).toBe(true);
    });

    it('PORT_CHECK event bus wiring: REQUESTED → COMPLETED completes the indicator', () => {
        chat.setupSSEListeners();

        eventBus.emit(EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
            port: '22',
        });

        eventBus.emit(EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.appendActivityIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith(`port-check-${EXECUTION_ID}`);
    });

    it('PORT_CHECK event bus wiring: REQUESTED → FAILED completes the indicator', () => {
        chat.setupSSEListeners();

        eventBus.emit(EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        eventBus.emit(EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED, {
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
            execution_id: EXECUTION_ID,
        });

        expect(terminalSpy.completeActivityIndicator).toHaveBeenCalledWith(`port-check-${EXECUTION_ID}`);
    });
});

describe('ChatComponent — handleOperatorExecutionRequest [FRONTEND - jsdom]', () => {
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

    it('handleOperatorExecutionRequest is defined on the prototype (from the mixin)', () => {
        expect(typeof ChatComponent.prototype.handleOperatorExecutionRequest).toBe('function');
    });

    it('handleOperatorExecutionRequest is not an own property of the class body', () => {
        const ownMethods = Object.getOwnPropertyNames(ChatComponent.prototype);
        expect(ownMethods).not.toContain('handleOperatorExecutionRequest');
    });

    it('hides thinking indicator when an operator execution request arrives', () => {
        const hideThinkingIndicator = vi.fn();
        chat.thinkingManager = { thinkingActive: true, hideThinkingIndicator };

        chat.handleOperatorExecutionRequest({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        });

        expect(hideThinkingIndicator).toHaveBeenCalledWith(WEB_SESSION_ID);
    });

    it('does not call hideThinkingIndicator when there is no thinkingManager', () => {
        chat.thinkingManager = null;

        expect(() => chat.handleOperatorExecutionRequest({
            investigation_id: INVESTIGATION_ID,
            web_session_id: WEB_SESSION_ID,
        })).not.toThrow();
    });

    it('rejects the event when investigation_id does not match', () => {
        const hideThinkingIndicator = vi.fn();
        chat.thinkingManager = { thinkingActive: true, hideThinkingIndicator };

        chat.handleOperatorExecutionRequest({
            investigation_id: 'wrong-inv',
            web_session_id: WEB_SESSION_ID,
        });

        expect(hideThinkingIndicator).not.toHaveBeenCalled();
    });
});

describe('ChatComponent — initCasesManager [FRONTEND - jsdom]', () => {
    let ChatComponent;
    let eventBus;
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
    });

    afterEach(() => {
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    it('initCasesManager is defined as an own method on the class prototype', () => {
        const ownMethods = Object.getOwnPropertyNames(ChatComponent.prototype);
        expect(ownMethods).toContain('initCasesManager');
    });

    it('initCasesManager assigns a casesManager with getCurrentInvestigationId', () => {
        const chat = new ChatComponent(eventBus);
        expect(chat.casesManager).toBeNull();

        chat.initCasesManager();

        expect(chat.casesManager).not.toBeNull();
        expect(typeof chat.casesManager.getCurrentInvestigationId).toBe('function');
    });

    it('initCasesManager assigns window.casesManager', () => {
        const chat = new ChatComponent(eventBus);
        chat.initCasesManager();

        expect(window.casesManager).toBe(chat.casesManager);
    });
});

describe('ChatComponent — submitChatMessage [FRONTEND - jsdom]', () => {
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

    it('is defined on the prototype via ChatSSEHandlersMixin', () => {
        expect(typeof chat.submitChatMessage).toBe('function');
    });

    it('appends the user message to the terminal', async () => {
        await chat.submitChatMessage('hello world');

        expect(terminalSpy.appendUserMessage).toHaveBeenCalledOnce();
        expect(terminalSpy.appendUserMessage).toHaveBeenCalledWith('hello world');
    });

    it('shows the waiting indicator with the web session id', async () => {
        await chat.submitChatMessage('test message');

        expect(terminalSpy.showWaitingIndicator).toHaveBeenCalledOnce();
        expect(terminalSpy.showWaitingIndicator).toHaveBeenCalledWith(WEB_SESSION_ID);
    });

    it('POSTs to the chat send endpoint', async () => {
        await chat.submitChatMessage('test message');

        const log = serviceClient.getRequestLog();
        expect(log).toHaveLength(1);
        expect(log[0].method).toBe('POST');
        expect(log[0].path).toBe('/api/chat/send');
    });

    it('includes message, case_id, and investigation_id in the payload', async () => {
        await chat.submitChatMessage('diagnose this');

        const log = serviceClient.getRequestLog();
        expect(log[0].body.message).toBe('diagnose this');
        expect(log[0].body.case_id).toBe('case-dispatch-abc123');
        expect(log[0].body.investigation_id).toBe(INVESTIGATION_ID);
    });

    it('trims whitespace from the message', async () => {
        await chat.submitChatMessage('  trimmed  ');

        const log = serviceClient.getRequestLog();
        expect(log[0].body.message).toBe('trimmed');
        expect(terminalSpy.appendUserMessage).toHaveBeenCalledWith('trimmed');
    });

    it('returns early without posting when message is empty after trim', async () => {
        await chat.submitChatMessage('   ');

        expect(serviceClient.getRequestLog()).toHaveLength(0);
        expect(terminalSpy.appendUserMessage).not.toHaveBeenCalled();
    });

    it('returns early without posting when currentUser is null', async () => {
        chat.currentUser = null;

        await chat.submitChatMessage('hello');

        expect(serviceClient.getRequestLog()).toHaveLength(0);
        expect(terminalSpy.appendUserMessage).not.toHaveBeenCalled();
    });

    it('passes attachments array in the payload', async () => {
        const attachments = [{ type: 'file', name: 'log.txt', content: 'data' }];

        await chat.submitChatMessage('check this', { attachments });

        const log = serviceClient.getRequestLog();
        expect(log[0].body.attachments).toEqual(attachments);
    });

    it('throws when attachments is null', async () => {
        await expect(chat.submitChatMessage('no attachments', { attachments: null }))
            .rejects.toThrow('[CHAT] submitChatMessage: attachments must be an array');
    });

    it('throws when message is not a string', async () => {
        await expect(chat.submitChatMessage(42))
            .rejects.toThrow('[CHAT] submitChatMessage: message must be a string');
    });

    it('sends empty attachments array when no attachments are provided', async () => {
        await chat.submitChatMessage('no attachments');

        const log = serviceClient.getRequestLog();
        expect(log[0].body.attachments).toEqual([]);
    });

    it('rejects and appends error message when the response is not ok', async () => {
        serviceClient.setResponse('g8ed', '/api/chat/send', {
            ok: false,
            status: 500,
            statusText: 'Internal Server Error',
        });

        await expect(chat.submitChatMessage('hello')).rejects.toThrow('HTTP 500');

        expect(terminalSpy.appendErrorMessage).toHaveBeenCalledOnce();
        expect(terminalSpy.appendErrorMessage.mock.calls[0][0]).toContain('Failed to send message');
    });

    it('hides the waiting indicator when the response is not ok', async () => {
        serviceClient.setResponse('g8ed', '/api/chat/send', {
            ok: false,
            status: 500,
            statusText: 'Internal Server Error',
        });

        await expect(chat.submitChatMessage('hello')).rejects.toThrow();

        expect(terminalSpy.hideWaitingIndicator).toHaveBeenCalledOnce();
    });

    it('rejects and appends error message when the request throws', async () => {
        vi.spyOn(serviceClient, 'post').mockRejectedValueOnce(new Error('network failure'));

        await expect(chat.submitChatMessage('hello')).rejects.toThrow('network failure');

        expect(terminalSpy.appendErrorMessage).toHaveBeenCalledOnce();
        expect(terminalSpy.appendErrorMessage.mock.calls[0][0]).toContain('Failed to send message');
    });

    it('hides the waiting indicator when the request throws', async () => {
        vi.spyOn(serviceClient, 'post').mockRejectedValueOnce(new Error('network failure'));

        await expect(chat.submitChatMessage('hello')).rejects.toThrow();

        expect(terminalSpy.hideWaitingIndicator).toHaveBeenCalledOnce();
    });

    it('focuses the terminal after a successful send', async () => {
        await chat.submitChatMessage('focus test');

        expect(terminalSpy.focus).toHaveBeenCalledOnce();
    });

    it('focuses the terminal even after a failed response', async () => {
        serviceClient.setResponse('g8ed', '/api/chat/send', {
            ok: false,
            status: 500,
            statusText: 'Internal Server Error',
        });

        await expect(chat.submitChatMessage('hello')).rejects.toThrow();

        expect(terminalSpy.focus).toHaveBeenCalledOnce();
    });

    it('LLM_CHAT_SUBMITTED event bus wiring invokes submitChatMessage', async () => {
        const spy = vi.spyOn(chat, 'submitChatMessage');
        chat.setupSSEListeners();

        eventBus.emit(EventType.LLM_CHAT_SUBMITTED, {
            message: 'wired message',
            attachments: [],
        });

        await vi.waitFor(() => expect(spy).toHaveBeenCalledOnce());
        expect(spy).toHaveBeenCalledWith('wired message', {
            attachments: [],
        });
    });
});
