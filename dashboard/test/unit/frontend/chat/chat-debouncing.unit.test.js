// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockEventBus, MockAuthState, MockServiceClient } from '@test/mocks/mock-browser-env.js';

const INVESTIGATION_ID = 'inv-test-debounce123';
const WEB_SESSION_ID = 'session-test-debounce123';

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
    window.authState = authState;
    window.serviceClient = serviceClient;
}

function cleanupGlobals() {
    delete window.authState;
    delete window.serviceClient;
    delete window.sentinelModeManager;
    delete window.llmModelManager;
}

function makeAnchoredTerminalSpy() {
    const renderedChunks = [];
    let renderCount = 0;

    return {
        get renderedChunks() { return renderedChunks; },
        get renderCount() { return renderCount; },
        appendAIResponseChunk(webSessionId, text) {
            renderedChunks.push({ webSessionId, text, timestamp: Date.now() });
            renderCount++;
        },
        finalizeAIResponseChunk: vi.fn(),
        applyCitationsAfterFinalize: vi.fn(),
        appendAIResponse: vi.fn(() => {
            const el = document.createElement('div');
            el.className = 'anchored-terminal__ai-response';
            return el;
        }),
        appendSystemMessage: vi.fn(),
        appendErrorMessage: vi.fn(),
        clearActivityIndicators: vi.fn(),
        resetAutoScroll: vi.fn(),
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

describe('ChatComponent debouncing behavior with fake timers [FRONTEND - jsdom]', () => {
    let ChatComponent;
    let eventBus;
    let chat;
    let terminalSpy;
    let authState;
    let serviceClient;
    let EventType;

    beforeEach(async () => {
        // Use fake timers for deterministic testing
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

        // Provide basic mocks for markdown dependencies
        globalThis.markdownit = () => ({
            enable: vi.fn(),
            render: (text) => text,
            renderer: { rules: {} },
            core: { ruler: { push: vi.fn() } }
        });
        globalThis.DOMPurify = { sanitize: (html) => html };
        globalThis.hljs = { highlight: () => ({ value: '' }) };

        ({ ChatComponent } = await import('@g8ed/public/js/components/chat.js'));
        ({ EventType } = await import('@g8ed/public/js/constants/events.js'));

        chat = new ChatComponent(eventBus);

        // Set up SSE listeners to enable debouncing
        chat.setupSSEListeners();

        terminalSpy = makeAnchoredTerminalSpy();
        chat.anchoredTerminal = terminalSpy;
        chat.casesManager = { getCurrentInvestigationId: () => INVESTIGATION_ID };
        chat.currentUser = { id: WEB_SESSION_ID };
        chat.currentWebSessionId = WEB_SESSION_ID;
    });

    afterEach(() => {
        vi.useRealTimers(); // Restore real timers
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    describe('rapid text chunk handling with debouncing', () => {
        it('should debounce rapid text chunks to prevent DOM thrashing', async () => {
            // Simulate rapid SSE text chunks arriving within 50ms
            const chunks = [
                'Hello',
                'Hello, world',
                'Hello, world! This',
                'Hello, world! This is',
                'Hello, world! This is a',
                'Hello, world! This is a test'
            ];

            // Send chunks rapidly (every 10ms)
            chunks.forEach((content, index) => {
                setTimeout(() => {
                    eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, {
                        web_session_id: WEB_SESSION_ID,
                        investigation_id: INVESTIGATION_ID,
                        content
                    });
                }, index * 10);
            });

            // Advance time to trigger all timers and debouncing
            await vi.advanceTimersByTimeAsync(200);

            // Should have rendered chunks, but potentially debounced
            expect(terminalSpy.renderCount).toBeGreaterThan(0);
            expect(terminalSpy.renderedChunks.length).toBeGreaterThan(0);
            
            // The last rendered chunk should contain the full accumulated text
            const lastRendered = terminalSpy.renderedChunks[terminalSpy.renderedChunks.length - 1];
            expect(lastRendered.text).toContain('Hello, world! This is a test');
        });

        it('should handle delayed chunks with proper timing', async () => {
            // Simulate chunks with longer delays (200ms apart)
            const chunks = ['Part 1', 'Part 2', 'Part 3'];

            chunks.forEach((content, index) => {
                setTimeout(() => {
                    eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, {
                        web_session_id: WEB_SESSION_ID,
                        investigation_id: INVESTIGATION_ID,
                        content
                    });
                }, index * 200);
            });

            // Advance time gradually to observe debouncing behavior
            await vi.advanceTimersByTimeAsync(100); // First chunk
            expect(terminalSpy.renderCount).toBeGreaterThanOrEqual(1);

            await vi.advanceTimersByTimeAsync(300); // Second chunk
            expect(terminalSpy.renderCount).toBeGreaterThanOrEqual(2);

            await vi.advanceTimersByTimeAsync(200); // Third chunk + final debouncing
            expect(terminalSpy.renderCount).toBeGreaterThanOrEqual(3);
        });

        it('should not lose chunks during debouncing', async () => {
            const allChunks = [];
            
            // Track all emitted chunks
            eventBus.on(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, (event) => {
                allChunks.push(event.content);
            });

            // Send many rapid chunks
            const rapidChunks = Array.from({ length: 20 }, (_, i) => `Chunk ${i + 1}`);
            rapidChunks.forEach((content, index) => {
                setTimeout(() => {
                    eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, {
                        web_session_id: WEB_SESSION_ID,
                        investigation_id: INVESTIGATION_ID,
                        content
                    });
                }, index * 5); // 5ms intervals
            });

            await vi.advanceTimersByTimeAsync(500);

            // All chunks should have been processed
            expect(allChunks).toEqual(rapidChunks);
            
            // Terminal should have received the accumulated content
            expect(terminalSpy.renderedChunks.length).toBeGreaterThan(0);
            const finalContent = terminalSpy.renderedChunks[terminalSpy.renderedChunks.length - 1].text;
            expect(finalContent).toContain('Chunk 20');
        });
    });

    describe('connection failure scenarios with timing', () => {
        it('should handle connection drops during active streaming', async () => {
            // Start streaming
            eventBus.emit(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                content: 'Initial response'
            });

            await vi.advanceTimersByTimeAsync(50);

            // Simulate connection drop
            eventBus.emit(EventType.LLM_CHAT_ITERATION_FAILED, {
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
                error: 'Connection reset by peer'
            });

            await vi.advanceTimersByTimeAsync(100);

            // Should have handled the error state
            expect(terminalSpy.appendErrorMessage).toHaveBeenCalled();
        });

        it('should handle reconnection attempts with proper timing', async () => {
            let reconnectAttempts = 0;
            
            // Provide attemptReconnect method if not natively present in component
            if (!chat.attemptReconnect) {
                chat.attemptReconnect = vi.fn();
            } else {
                chat.attemptReconnect = vi.fn(chat.attemptReconnect);
            }
            
            const originalReconnect = chat.attemptReconnect;
            chat.attemptReconnect = vi.fn(() => {
                reconnectAttempts++;
                // Simulate successful reconnection after delay
                setTimeout(() => {
                    eventBus.emit(EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED, {
                        web_session_id: WEB_SESSION_ID
                    });
                }, 100);
            });

            // Simulate connection failure that triggers reconnect
            eventBus.emit(EventType.PLATFORM_SSE_CONNECTION_CLOSED, {
                web_session_id: WEB_SESSION_ID
            });

            // Wait for reconnect delay
            await vi.advanceTimersByTimeAsync(150);

            expect(chat.attemptReconnect).toHaveBeenCalled();
            expect(reconnectAttempts).toBe(1);
        });
    });

    describe('performance with fake timers', () => {
        it('should complete complex streaming scenario quickly with fake timers', async () => {
            const startTime = Date.now();

            const events = [
                { type: EventType.LLM_LIFECYCLE_STARTED, delay: 0 },
                { type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, content: 'Thinking', delay: 10 },
                { type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, content: 'Thinking...', delay: 20 },
                { type: 'LLM_TOOL_DROPOPS_WEB_SEARCH_REQUESTED', query: 'test', delay: 30 },
                { type: 'LLM_TOOL_DROPOPS_WEB_SEARCH_COMPLETED', delay: 50 },
                { type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, content: 'Based on search', delay: 60 },
                { type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED, delay: 200 },
                { type: EventType.LLM_LIFECYCLE_COMPLETED, delay: 220 }
            ];

            events.forEach(event => {
                setTimeout(() => {
                    if (event.type === EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED) {
                        eventBus.emit(event.type, {
                            web_session_id: WEB_SESSION_ID,
                            investigation_id: INVESTIGATION_ID,
                            content: event.content
                        });
                    } else if (event.type === 'LLM_TOOL_DROPOPS_WEB_SEARCH_REQUESTED') {
                        eventBus.emit(event.type, {
                            web_session_id: WEB_SESSION_ID,
                            investigation_id: INVESTIGATION_ID,
                            query: event.query
                        });
                    } else {
                        eventBus.emit(event.type, {
                            web_session_id: WEB_SESSION_ID,
                            investigation_id: INVESTIGATION_ID
                        });
                    }
                }, event.delay);
            });

            await vi.advanceTimersByTimeAsync(300);

            const endTime = Date.now();
            const executionTime = endTime - startTime;

            expect(executionTime).toBeLessThan(500);

            expect(terminalSpy.renderCount).toBeGreaterThan(0);
            expect(terminalSpy.finalizeAIResponseChunk).toHaveBeenCalledOnce();
        });
    });
});
