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
import { EventType } from '@vsod/public/js/constants/events.js';

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
    const userMessages = [];
    const aiResponseChunks = [];
    let autoScrollResets = 0;

    return {
        get userMessages() { return userMessages; },
        get aiResponseChunks() { return aiResponseChunks; },
        get autoScrollResets() { return autoScrollResets; },
        appendUserMessage(content, timestamp) {
            const el = document.createElement('div');
            el.className = 'anchored-terminal__user-message';
            userMessages.push({ content, timestamp, el });
            return el;
        },
        appendStreamingTextChunk: vi.fn(),
        replaceStreamingHtml(webSessionId, text) {
            aiResponseChunks.push({ webSessionId, text });
        },
        finalizeAIResponseChunk: vi.fn(),
        applyCitationsAfterFinalize: vi.fn(),
        appendDirectHtmlResponse: vi.fn(() => {
            const el = document.createElement('div');
            el.className = 'anchored-terminal__ai-response';
            return el;
        }),
        appendSystemMessage: vi.fn(() => {
            const el = document.createElement('div');
            el.className = 'anchored-terminal__entry';
            return el;
        }),
        appendErrorMessage: vi.fn(),
        clearActivityIndicators: vi.fn(),
        resetAutoScroll() { autoScrollResets++; },
        showWaitingIndicator: vi.fn(),
        clear: vi.fn(),
        focus: vi.fn(),
        enable: vi.fn(),
        disable: vi.fn(),
        setUser: vi.fn(),
        clearOutput: vi.fn(),
        scrollToBottom: vi.fn(),
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

describe('ChatComponent message rendering [FRONTEND - jsdom]', () => {
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
        serviceClient.setResponse('vsod', '/js/components/templates/streaming-message.html', {
            ok: true, status: 200, text: async () => ''
        });

        installGlobals(authState, serviceClient);

        eventBus = new MockEventBus();

        ({ ChatComponent } = await import('@vsod/public/js/components/chat.js'));

        chat = new ChatComponent(eventBus);

        // Set up SSE listeners to enable debouncing
        chat.setupSSEListeners();

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

    describe('handleAITextChunk — no prefix on first chunk', () => {
        function makeChunk(content) {
            return {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content,
            };
        }

        it('passes raw text directly to appendStreamingTextChunk', () => {
            chat.handleAITextChunk(makeChunk('Good evening!'));

            expect(terminalSpy.appendStreamingTextChunk).toHaveBeenCalledOnce();
            const [, text] = terminalSpy.appendStreamingTextChunk.mock.calls[0];
            expect(text).toBe('Good evening!');
        });

        it('passes each chunk to appendStreamingTextChunk', () => {
            chat.handleAITextChunk(makeChunk('Hello'));
            chat.handleAITextChunk(makeChunk(' world'));

            expect(terminalSpy.appendStreamingTextChunk).toHaveBeenCalledTimes(2);
            expect(terminalSpy.appendStreamingTextChunk.mock.calls[0][1]).toBe('Hello');
            expect(terminalSpy.appendStreamingTextChunk.mock.calls[1][1]).toBe(' world');
        });

        it('resets auto-scroll exactly once for the first chunk of a session', () => {
            chat.handleAITextChunk(makeChunk('chunk 1'));
            chat.handleAITextChunk(makeChunk('chunk 2'));
            chat.handleAITextChunk(makeChunk('chunk 3'));

            expect(terminalSpy.autoScrollResets).toBe(1);
        });

        it('tracks independent sessions without cross-contamination', () => {
            chat.casesManager = makeCasesManagerStub(INVESTIGATION_ID);

            chat.handleAITextChunk({ web_session_id: 'session-A', investigation_id: INVESTIGATION_ID, content: 'A' });
            chat.handleAITextChunk({ web_session_id: 'session-B', investigation_id: INVESTIGATION_ID, content: 'B' });

            expect(terminalSpy.appendStreamingTextChunk).toHaveBeenCalledTimes(2);
            expect(terminalSpy.appendStreamingTextChunk.mock.calls[0][0]).toBe('session-A');
            expect(terminalSpy.appendStreamingTextChunk.mock.calls[0][1]).toBe('A');
            expect(terminalSpy.appendStreamingTextChunk.mock.calls[1][0]).toBe('session-B');
            expect(terminalSpy.appendStreamingTextChunk.mock.calls[1][1]).toBe('B');
        });

    });

    describe('EventType constants', () => {
        it('EventType.EVENT_SOURCE_USER_CHAT is "g8e.v1.source.user.chat", not "user"', () => {
            expect(EventType.EVENT_SOURCE_USER_CHAT).toBe('g8e.v1.source.user.chat');
            expect(EventType.EVENT_SOURCE_USER_CHAT).not.toBe('user');
        });
    });

    describe('handleResponseComplete — no crash when data.content is undefined', () => {
        it('does not throw when data.content is undefined (normal streaming completion)', () => {
            expect(() => {
                chat.handleResponseComplete({
                    web_session_id: WEB_SESSION_ID,
                    investigation_id: INVESTIGATION_ID,
                    content: undefined,
                });
            }).not.toThrow();
        });

        it('does not throw when both total_length and content are absent', () => {
            expect(() => {
                chat.handleResponseComplete({
                    web_session_id: WEB_SESSION_ID,
                    investigation_id: INVESTIGATION_ID,
                });
            }).not.toThrow();
        });
    });
});
