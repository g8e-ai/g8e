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

/**
 * MSW (Mock Service Worker) SSE Handlers
 * 
 * Provides network-level mocking for SSE endpoints, enabling realistic
 * testing of connection failures, malformed data, and network timing.
 * 
 * Usage:
 *   import { setupServer } from 'msw/node';
 *   import { sseHandlers } from '@test/mocks/msw-sse-handlers.js';
 *   
 *   const server = setupServer(...sseHandlers);
 *   server.listen();
 */

import { delay, HttpResponse } from 'msw';
import { readFileSync } from 'fs';
import { resolve } from 'path';

// Load shared SSE event fixtures
const fixturesPath = resolve(__dirname, '../../../../shared/test-fixtures/sse-events.json');
const sseEvents = JSON.parse(readFileSync(fixturesPath, 'utf8'));

/**
 * Create a controllable SSE stream response
 */
class SSEStreamResponse {
    constructor(options = {}) {
        this.options = options;
        this.controller = null;
        this.isClosed = false;
        this.eventQueue = [];
        this.scheduledEvents = [];
    }

    /**
     * Start the SSE stream
     */
    async start(controller) {
        this.controller = controller;
        
        // Set SSE headers
        const headers = {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
            'Connection': 'keep-alive',
            'Access-Control-Allow-Origin': '*',
            'Access-Control-Allow-Headers': 'Cache-Control'
        };

        controller.headers.set(headers);

        // Send initial connection event
        this.sendEvent(sseEvents.platform_sse_connection_established);

        // Process queued events
        this.processEventQueue();
    }

    /**
     * Send an SSE event
     */
    sendEvent(eventData) {
        if (this.isClosed) return;

        const sseFrame = this.formatSSEEvent(eventData);
        
        if (this.controller) {
            this.controller.write(sseFrame);
        } else {
            this.eventQueue.push(eventData);
        }
    }

    /**
     * Format event data as SSE frame
     */
    formatSSEEvent(eventData) {
        const lines = [
            `data: ${JSON.stringify(eventData)}`,
            '', // Empty line to terminate event
            ''
        ];
        return lines.join('\n');
    }

    /**
     * Process queued events
     */
    processEventQueue() {
        while (this.eventQueue.length > 0) {
            const event = this.eventQueue.shift();
            this.sendEvent(event);
        }
    }

    /**
     * Schedule events with delays (for testing timing scenarios)
     */
    async scheduleEvents(events, baseDelay = 50) {
        for (let i = 0; i < events.length; i++) {
            const event = events[i];
            const delayMs = Math.min(baseDelay, 10); // Cap at 10ms for test speed
            
            this.scheduledEvents.push(
                delay(delayMs).then(() => {
                    this.sendEvent(event);
                })
            );
        }

        await Promise.all(this.scheduledEvents);
    }

    /**
     * Simulate connection g8e (broken pipe)
     */
    simulateBrokenPipe(options = {}) {
        const { duringEvent = false, partialData = null } = options;
        
        if (duringEvent && partialData) {
            this.controller?.write(partialData);
        }
        
        this.isClosed = true;
        this.controller?.destroy(new Error('Connection reset by peer'));
    }

    /**
     * Simulate connection timeout
     */
    async simulateTimeout(timeoutMs = 30000) {
        const actualDelay = Math.min(timeoutMs, 100); // Cap at 100ms for test speed
        await delay(actualDelay);
        
        this.isClosed = true;
        const timeoutError = new Error(`SSE connection timeout after ${timeoutMs}ms`);
        timeoutError.code = 'ETIMEDOUT';
        this.controller?.destroy(timeoutError);
    }

    /**
     * Send malformed SSE data
     */
    sendMalformedData(data = 'invalid: format\nwithout proper terminator') {
        this.controller?.write(data);
    }

    /**
     * Close the stream normally
     */
    close() {
        this.isClosed = true;
        this.controller?.close();
    }
}

/**
 * MSW handlers for SSE endpoints
 */
export const sseHandlers = [
    // Handle SSE chat endpoint
    {
        method: 'GET',
        path: '/api/sse/:webSessionId/sse/events',
        response: async ({ params, request }) => {
            const stream = new SSEStreamResponse();
            
            return new HttpResponse(async (controller) => {
                await stream.start(controller);
                
                // Keep connection alive with periodic keepalives
                const keepaliveInterval = setInterval(() => {
                    if (!stream.isClosed) {
                        stream.sendEvent(sseEvents.platform_sse_keepalive_sent);
                    }
                }, 30000);

                // Handle client disconnect
                request.signal.addEventListener('abort', () => {
                    clearInterval(keepaliveInterval);
                    stream.close();
                });

                // Store stream reference for test control
                global.activeSSEStreams = global.activeSSEStreams || new Map();
                global.activeSSEStreams.set(params.webSessionId, stream);
            });
        }
    },

    // Handle internal SSE push endpoint (VSE → VSOD)
    {
        method: 'POST',
        path: '/api/internal/sse/push',
        response: async ({ request }) => {
            const body = await request.json();
            
            // Simulate processing delay
            await delay(10);
            
            // Forward to active streams if any
            if (global.activeSSEStreams && body.web_session_id) {
                const stream = global.activeSSEStreams.get(body.web_session_id);
                if (stream && !stream.isClosed) {
                    stream.sendEvent(body);
                }
            }
            
            return HttpResponse.json({ success: true });
        }
    }
];

/**
 * Helper functions for test control
 */
export const sseTestHelpers = {
    /**
     * Get active stream for a session
     */
    getStream(webSessionId) {
        return global.activeSSEStreams?.get(webSessionId);
    },

    /**
     * Send chat message events to a stream
     */
    async sendChatFlow(webSessionId, investigationId, caseId) {
        const stream = this.getStream(webSessionId);
        if (!stream) return;

        const events = [
            { ...sseEvents.llm_lifecycle_started, investigation_id: investigationId, case_id: caseId, web_session_id: webSessionId },
            { ...sseEvents.text_chunk_received, content: 'Hello', investigation_id: investigationId, case_id: caseId, web_session_id: webSessionId },
            { ...sseEvents.text_chunk_received, content: 'Hello, world!', investigation_id: investigationId, case_id: caseId, web_session_id: webSessionId },
            { ...sseEvents.text_completed, content: 'Hello, world!', investigation_id: investigationId, case_id: caseId, web_session_id: webSessionId },
            { ...sseEvents.llm_lifecycle_completed, investigation_id: investigationId, case_id: caseId, web_session_id: webSessionId }
        ];

        await stream.scheduleEvents(events, 50);
    },

    /**
     * Send search web flow
     */
    async sendSearchWebFlow(webSessionId, investigationId, caseId, query = 'test query') {
        const stream = this.getStream(webSessionId);
        if (!stream) return;

        const events = [
            { ...sseEvents.search_web_requested, query, investigation_id: investigationId, case_id: caseId, web_session_id: webSessionId },
            { ...sseEvents.search_web_completed, query, investigation_id: investigationId, case_id: caseId, web_session_id: webSessionId }
        ];

        await stream.scheduleEvents(events, 100);
    },

    /**
     * Simulate connection failure
     */
    simulateConnectionFailure(webSessionId, options = {}) {
        const stream = this.getStream(webSessionId);
        if (!stream) return;

        stream.simulateBrokenPipe(options);
    },

    /**
     * Simulate malformed data
     */
    sendMalformedData(webSessionId, data) {
        const stream = this.getStream(webSessionId);
        if (!stream) return;

        stream.sendMalformedData(data);
    },

    /**
     * Clean up all streams
     */
    cleanup() {
        if (global.activeSSEStreams) {
            for (const stream of global.activeSSEStreams.values()) {
                stream.close();
            }
            global.activeSSEStreams.clear();
        }
    }
};

export default sseHandlers;
