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
import { EventType } from '@g8ed/public/js/models/investigation-models.js';
import { makeAnchoredTerminalSpy } from '@test/utils/test-helpers.js';

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
            chat.streamingActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
            });

            expect(chat.streamingActive).toBe(true);
        });

        it('rejects event with mismatched investigation_id', () => {
            chat.streamingActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: 'wrong-id',
                content: 'test response',
            });

            expect(chat.streamingActive).toBe(true);
        });

        it('rejects event when casesManager has no current investigation', () => {
            chat.casesManager = makeCasesManagerStub(null);
            chat.streamingActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'test response',
            });

            expect(chat.streamingActive).toBe(true);
        });

        it('accepts event with matching investigation_id', () => {
            chat.streamingActive = true;

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'test response',
            });

            expect(chat.streamingActive).toBe(false);
        });
    });

    describe('handleResponseComplete — uses data.content from TEXT_COMPLETED', () => {
        beforeEach(() => {
            chat.streamingActive = true;
            chat.aiStopBtn = document.getElementById('ai-stop-btn');
            chat.aiStopBtn.disabled = false;
        });

        it('calls anchoredTerminal.finalizeAIResponseChunk with rendered markdown from data.content', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: '**bold** response',
            });

            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
            expect(terminalSpy.finalizeAIResponseChunk.mock.calls[0][0]).toBe(WEB_SESSION_ID);
            expect(terminalSpy.finalizeAIResponseChunk.mock.calls[0][1]).toContain('<strong');
        });

        it('passes grounding_metadata to finalizeAIResponseChunk when present', () => {
            const metadata = { grounding_used: true, sources: [{ url: 'https://a.com', title: 'A' }] };

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'response with citations',
                grounding_metadata: metadata,
            });

            expect(terminalSpy.finalizeAIResponseChunk.mock.calls[0][2]).toBe(metadata);
        });

        it('calls anchoredTerminal.clearActivityIndicators on finalize', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'response',
            });

            expect(terminalSpy.clearActivityIndicators).toHaveBeenCalledOnce();
        });

        it('sets streamingActive to false', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'response',
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

    describe('handleResponseComplete — no data.content', () => {
        it('does not call finalizeAIResponseChunk when content is absent', () => {
            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            });

            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
        });
    });

    describe('handleResponseComplete — event bus routing', () => {
        it('triggers finalization when RESPONSE_COMPLETE event fires with matching ids', () => {
            chat.streamingActive = true;

            eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'response',
            });

            expect(chat.streamingActive).toBe(false);
            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
        });

        it('does not trigger finalization when RESPONSE_COMPLETE investigation_id mismatches', () => {
            chat.streamingActive = true;

            eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: 'wrong-id',
                content: 'response',
            });

            expect(chat.streamingActive).toBe(true);
            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
        });
    });

    describe('handleTurnComplete — multi-turn boundary', () => {
        it('seals streaming response at turn boundary when streaming is active', () => {
            chat.streamingActive = true;

            eventBus.emit(EventType.LLM_CHAT_ITERATION_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                turn: 1,
            });

            expect(terminalSpy.sealStreamingResponse).toHaveBeenCalledOnce();
            expect(terminalSpy.sealStreamingResponse).toHaveBeenCalledWith(WEB_SESSION_ID);
            expect(chat.streamingActive).toBe(false);
        });

        it('does not seal streaming when streaming is not active', () => {
            chat.streamingActive = false;

            eventBus.emit(EventType.LLM_CHAT_ITERATION_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                turn: 1,
            });

            expect(terminalSpy.sealStreamingResponse).not.toHaveBeenCalled();
        });

        it('does not call finalizeAIResponseChunk', () => {
            chat.streamingActive = true;

            eventBus.emit(EventType.LLM_CHAT_ITERATION_COMPLETED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                turn: 1,
            });

            expect(terminalSpy.finalizeAIResponseChunk).not.toHaveBeenCalled();
        });
    });

    describe('handleResponseComplete — indicator map cleanup', () => {
        it('clears _searchWebIndicators on finalize', () => {
            chat._searchWebIndicators = new Map([['exec-1', 'search-web-exec-1']]);

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'response',
            });

            expect(chat._searchWebIndicators.size).toBe(0);
        });

        it('clears _portCheckIndicators on finalize', () => {
            chat._portCheckIndicators = new Map([['exec-2', 'port-check-exec-2']]);

            chat.handleResponseComplete({
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'response',
            });

            expect(chat._portCheckIndicators.size).toBe(0);
        });
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
