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
 * Mock SSE Browser Client
 * 
 * Simulates browser EventType behavior for unit testing SSE endpoints
 * without requiring real HTTP connections. Works directly with Express
 * response objects or mock response objects.
 * 
 * Industry Standard Approach:
 * - Parses SSE format (data: {...}\n\n) correctly per W3C spec
 * - Provides same API patterns as real EventType
 * - Supports event listeners, reconnection simulation, metrics
 * - Zero network dependencies for fast unit tests
 * 
 * Usage:
 *   // Unit test with mock response
 *   const { client, mockResponse } = MockSSEBrowser.createPair();
 *   mockResponse.write('data: {"type":"test"}\n\n');
 *   const event = await client.waitForEvent('test');
 * 
 *   // Or attach to real Express response for integration
 *   const client = new MockSSEBrowser();
 *   client.attachToResponse(expressRes);
 */

import { EventEmitter } from 'events';
import { SSE_FRAME_TERMINATOR } from '@g8ed/constants/service_config.js';

/**
 * Mock SSE Response - Simulates Express response object for SSE
 * 
 * Captures all writes and parses SSE format, emitting parsed events
 * to any attached MockSSEBrowser clients.
 */
export class MockSSEResponse extends EventEmitter {
    constructor() {
        super();
        this.headers = {};
        this.statusCode = null;
        this.headersSent = false;
        this.writable = true;
        this.destroyed = false;
        this.socket = {
            writable: true,
            destroyed: false,
            uncork: () => {},
            cork: () => {},
            setTimeout: () => {},
            setNoDelay: () => {},
        };
        
        this._written = [];
        this._buffer = '';
        this._closed = false;
        this._parsedEvents = [];
    }

    /**
     * Set response header
     */
    setHeader(name, value) {
        this.headers[name.toLowerCase()] = value;
        return this;
    }

    /**
     * Get response header
     */
    getHeader(name) {
        return this.headers[name.toLowerCase()];
    }

    /**
     * Set status code
     */
    status(code) {
        this.statusCode = code;
        return this;
    }

    /**
     * Flush headers immediately (required for SSE)
     */
    flushHeaders() {
        this.headersSent = true;
        this.emit('headers', this.headers);
    }

    /**
     * Set timeout (no-op for mock)
     */
    setTimeout(ms, callback) {
        return this;
    }

    /**
     * Write data to the response stream
     * Parses SSE format and emits events
     */
    write(data) {
        if (this._closed || this.destroyed) {
            return false;
        }

        const str = typeof data === 'string' ? data : data.toString();
        this._written.push(str);
        this._buffer += str;

        this._parseBuffer();
        return true;
    }

    /**
     * Parse SSE buffer and emit events
     * SSE format: "data: {...}\n\n" or "event: name\ndata: {...}\n\n"
     */
    _parseBuffer() {
        const events = this._buffer.split(SSE_FRAME_TERMINATOR);
        
        if (events.length > 1) {
            this._buffer = events.pop();
            
            for (const eventBlock of events) {
                if (!eventBlock.trim()) continue;
                
                const lines = eventBlock.split('\n');
                let eventType = 'message';
                let eventData = null;
                let eventId = null;
                let retry = null;

                for (const line of lines) {
                    if (line.startsWith('event:')) {
                        eventType = line.substring(6).trim();
                    } else if (line.startsWith('data:')) {
                        const dataStr = line.substring(5).trim();
                        try {
                            eventData = JSON.parse(dataStr);
                        } catch {
                            eventData = dataStr;
                        }
                    } else if (line.startsWith('id:')) {
                        eventId = line.substring(3).trim();
                    } else if (line.startsWith('retry:')) {
                        retry = parseInt(line.substring(6).trim(), 10);
                    }
                }

                if (eventData !== null) {
                    const sseEvent = {
                        type: eventData.type || eventType,
                        data: eventData,
                        lastEventId: eventId,
                        retry,
                        timestamp: Date.now()
                    };
                    this._parsedEvents.push(eventData);
                    this.emit('sse-event', sseEvent);
                }
            }
        }
    }

    /**
     * Flush buffered data (no-op for mock — writes are synchronous)
     */
    flush() {}

    /**
     * End the response
     */
    end(data) {
        if (data) {
            this.write(data);
        }
        this._closed = true;
        this.writable = false;
        this.emit('close');
        this.emit('finish');
    }

    /**
     * Destroy the response (simulates connection drop)
     */
    destroy(error) {
        this.destroyed = true;
        this.writable = false;
        this._closed = true;
        if (error) {
            this.emit('error', error);
        }
        this.emit('close');
    }

    /**
     * Get all raw written data
     */
    getWrittenData() {
        return [...this._written];
    }

    /**
     * Get parsed events from written data
     */
    getParsedEvents() {
        return [...this._parsedEvents];
    }

    /**
     * Clear written data (for test isolation)
     */
    clear() {
        this._written = [];
        this._buffer = '';
        this._parsedEvents = [];
    }
}

/**
 * Mock SSE Browser Client
 * 
 * Simulates browser EventType for testing. Can attach to MockSSEResponse
 * or real Express response objects.
 */
export class MockSSEBrowser extends EventEmitter {
    constructor(options = {}) {
        super();
        
        this.url = options.url || 'mock://sse';
        this.withCredentials = options.withCredentials;
        this.readyState = ReadyState.CONNECTING;
        
        this._events = [];
        this._eventListeners = new Map();
        this._pendingWaiters = [];
        this._response = null;
        this._lastEventId = null;
        this._reconnectCount = 0;
        
        this._metrics = {
            connectedAt: null,
            disconnectedAt: null,
            totalEvents: 0,
            eventsByType: new Map()
        };

        this.onopen = null;
        this.onmessage = null;
        this.onerror = null;
    }

    /**
     * Attach to a MockSSEResponse or Express response object
     */
    attachToResponse(response) {
        this._response = response;
        
        response.on('sse-event', (event) => {
            this._handleEvent(event);
        });

        response.on('close', () => {
            this._handleClose();
        });

        response.on('error', (error) => {
            this._handleError(error);
        });

        response.on('headers', () => {
            this._handleOpen();
        });

        if (response.headersSent) {
            this._handleOpen();
        }

        return this;
    }

    /**
     * Handle connection open
     */
    _handleOpen() {
        this.readyState = ReadyState.OPEN;
        this._metrics.connectedAt = Date.now();
        
        if (this.onopen) {
            this.onopen({ type: 'open' });
        }
        this.emit('open');
    }

    /**
     * Handle incoming SSE event
     */
    _handleEvent(event) {
        this._events.push(event);
        this._metrics.totalEvents++;
        
        const type = event.type || 'message';
        const count = this._metrics.eventsByType.get(type) || 0;
        this._metrics.eventsByType.set(type, count + 1);

        if (event.lastEventId) {
            this._lastEventId = event.lastEventId;
        }

        if (this.onmessage) {
            this.onmessage({
                type: 'message',
                data: JSON.stringify(event.data),
                lastEventId: event.lastEventId
            });
        }

        const listeners = this._eventListeners.get(type);
        if (listeners) {
            for (const listener of listeners) {
                listener({
                    type,
                    data: JSON.stringify(event.data),
                    lastEventId: event.lastEventId
                });
            }
        }

        this.emit('message', event);
        this.emit(type, event);

        this._notifyWaiters(event);
    }

    /**
     * Handle connection close
     */
    _handleClose() {
        this.readyState = ReadyState.CLOSED;
        this._metrics.disconnectedAt = Date.now();
        this.emit('close');
    }

    /**
     * Handle connection error
     */
    _handleError(error) {
        if (this.onerror) {
            this.onerror(error);
        }
        if (this.listenerCount('error') > 0) {
            this.emit('error', error);
        }
    }

    /**
     * Notify waiting promises
     */
    _notifyWaiters(event) {
        const stillWaiting = [];
        
        for (const waiter of this._pendingWaiters) {
            if (waiter.predicate(event.data)) {
                waiter.resolve(event.data);
            } else {
                stillWaiting.push(waiter);
            }
        }
        
        this._pendingWaiters = stillWaiting;
    }

    /**
     * Add event listener (W3C EventType API)
     */
    addEventListener(type, listener) {
        if (!this._eventListeners.has(type)) {
            this._eventListeners.set(type, []);
        }
        this._eventListeners.get(type).push(listener);
    }

    /**
     * Remove event listener (W3C EventType API)
     */
    removeEventListener(type, listener) {
        if (!this._eventListeners.has(type)) return;
        const listeners = this._eventListeners.get(type);
        const index = listeners.indexOf(listener);
        if (index > -1) {
            listeners.splice(index, 1);
        }
    }

    /**
     * Close the connection (W3C EventType API)
     */
    close() {
        this.readyState = ReadyState.CLOSED;
        this._metrics.disconnectedAt = Date.now();
        
        for (const waiter of this._pendingWaiters) {
            waiter.reject(new Error('Connection closed'));
        }
        this._pendingWaiters = [];
        
        this.emit('close');
    }

    /**
     * Wait for a specific event type
     * 
     * @param {string} eventType - Event type to wait for
     * @param {number} timeout - Timeout in ms (default: 5000)
     * @returns {Promise<Object>} Event data
     */
    async waitForEvent(eventType, timeout = 5000) {
        return this.waitForEventMatching(
            (data) => data.type === eventType,
            timeout,
            `event type '${eventType}'`
        );
    }

    /**
     * Wait for an event matching a predicate
     * 
     * @param {Function} predicate - Function that returns true for matching events
     * @param {number} timeout - Timeout in ms
     * @param {string} description - Description for error messages
     * @returns {Promise<Object>} Matching event data
     */
    async waitForEventMatching(predicate, timeout = 5000, description = 'matching event') {
        const existing = this._events.find(e => predicate(e.data));
        if (existing) {
            return existing.data;
        }

        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                const index = this._pendingWaiters.findIndex(w => w.resolve === resolve);
                if (index > -1) {
                    this._pendingWaiters.splice(index, 1);
                }
                reject(new Error(`Timeout waiting for ${description} after ${timeout}ms`));
            }, timeout);

            this._pendingWaiters.push({
                predicate,
                resolve: (data) => {
                    clearTimeout(timer);
                    resolve(data);
                },
                reject
            });
        });
    }

    /**
     * Collect multiple events of a specific type
     * 
     * @param {string} eventType - Event type to collect
     * @param {number} count - Number of events to collect
     * @param {number} timeout - Timeout in ms
     * @returns {Promise<Array>} Array of event data objects
     */
    async collectEvents(eventType, count, timeout = 5000) {
        const already = this._events
            .filter(e => e.data.type === eventType)
            .map(e => e.data);
        if (already.length >= count) {
            return already.slice(0, count);
        }

        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                const got = this._events.filter(e => e.data.type === eventType).length;
                const idx = this._pendingWaiters.findIndex(w => w._collectResolve === resolve);
                if (idx > -1) this._pendingWaiters.splice(idx, 1);
                reject(new Error(`Timeout collecting ${count} '${eventType}' events. Got ${got}`));
            }, timeout);

            const waiter = {
                _collectResolve: resolve,
                predicate: (data) => {
                    return this._events.filter(e => e.data.type === eventType).length >= count;
                },
                resolve: (data) => {
                    clearTimeout(timer);
                    resolve(
                        this._events
                            .filter(e => e.data.type === eventType)
                            .map(e => e.data)
                            .slice(0, count)
                    );
                },
                reject: (err) => {
                    clearTimeout(timer);
                    reject(err);
                }
            };
            this._pendingWaiters.push(waiter);
        });
    }

    /**
     * Collect events for a duration
     * 
     * @param {number} duration - Duration to collect in ms
     * @param {string} eventType - Optional: only collect events of this type
     * @returns {Promise<Array>} Array of event data objects
     */
    async collectEventsFor(duration, eventType = null) {
        const startIndex = this._events.length;
        await new Promise(resolve => setTimeout(resolve, duration));

        let collected = this._events.slice(startIndex).map(e => e.data);
        if (eventType) {
            collected = collected.filter(e => e.type === eventType);
        }
        return collected;
    }

    /**
     * Get all events of a specific type
     */
    getEventsByType(eventType) {
        return this._events
            .filter(e => e.data.type === eventType)
            .map(e => e.data);
    }

    /**
     * Get all received events
     */
    getAllEvents() {
        return [...this._events];
    }

    /**
     * Get the most recent event
     */
    getLastEvent(eventType = null) {
        const filtered = eventType
            ? this._events.filter(e => e.data.type === eventType)
            : this._events;
        
        if (filtered.length === 0) return null;
        return filtered[filtered.length - 1].data;
    }

    /**
     * Clear collected events
     */
    clearEvents() {
        this._events = [];
    }

    /**
     * Check if connected
     */
    isConnected() {
        return this.readyState === ReadyState.OPEN;
    }

    /**
     * Get connection metrics
     */
    getMetrics() {
        return {
            ...this._metrics,
            connectionDuration: this._metrics.connectedAt
                ? (this._metrics.disconnectedAt || Date.now()) - this._metrics.connectedAt
                : 0,
            eventsByType: Object.fromEntries(this._metrics.eventsByType),
            readyState: this.readyState,
            reconnectCount: this._reconnectCount,
            totalEvents: this._metrics.totalEvents
        };
    }

    /**
     * Assert that an event was received
     */
    assertEventReceived(eventType, expectedData = null) {
        const events = this.getEventsByType(eventType);
        if (events.length === 0) {
            throw new Error(`Expected event '${eventType}' was not received`);
        }

        if (expectedData) {
            const matching = events.find(event => {
                for (const [key, value] of Object.entries(expectedData)) {
                    if (JSON.stringify(event[key]) !== JSON.stringify(value) &&
                        JSON.stringify(event.data?.[key]) !== JSON.stringify(value)) {
                        return false;
                    }
                }
                return true;
            });

            if (!matching) {
                throw new Error(
                    `Event '${eventType}' received but data doesn't match. ` +
                    `Expected: ${JSON.stringify(expectedData)}, Got: ${JSON.stringify(events)}`
                );
            }
        }
    }

    /**
     * Assert event count
     */
    assertEventCount(eventType, expectedCount, comparison = 'eq') {
        const count = this.getEventsByType(eventType).length;
        const comparisons = {
            eq: count === expectedCount,
            gte: count >= expectedCount,
            lte: count <= expectedCount,
            gt: count > expectedCount,
            lt: count < expectedCount
        };

        if (!comparisons[comparison]) {
            throw new Error(
                `Expected ${comparison} ${expectedCount} '${eventType}' events, got ${count}`
            );
        }
    }

    /**
     * Simulate reconnection
     */
    async reconnect(delayMs = 100) {
        this.close();
        this._reconnectCount++;
        
        await new Promise(resolve => setTimeout(resolve, delayMs));
        
        this.readyState = ReadyState.CONNECTING;
        this._events = [];
        
        return this;
    }

    /**
     * Create a connected pair of MockSSEResponse and MockSSEBrowser
     * Convenience factory for common test setup
     * 
     * @returns {{ response: MockSSEResponse, client: MockSSEBrowser }}
     */
    static createPair(options = {}) {
        const response = new MockSSEResponse();
        const client = new MockSSEBrowser(options);
        client.attachToResponse(response);
        return { response, client };
    }

    /**
     * Simulate sending an SSE event (for testing without response object)
     * Directly injects an event as if received from server
     */
    injectEvent(eventData) {
        const event = {
            type: eventData.type || 'message',
            data: eventData,
            lastEventId: eventData.id || null,
            timestamp: Date.now()
        };
        this._handleEvent(event);
    }

    /**
     * Simulate broken pipe scenario - abrupt connection g8e during streaming
     * 
     * @param {Object} options - Break options
     * @param {boolean} options.duringEvent - If true, drops while processing an event
     * @param {string} options.partialData - Partial SSE data to write before drop
     */
    simulateBrokenPipe(options = {}) {
        const { duringEvent = false, partialData = null } = options;
        
        if (duringEvent && partialData) {
            // Write partial SSE frame then g8e connection
            this._response.write(partialData);
        }
        
        // Simulate abrupt connection termination
        this._response.destroy(new Error('Connection reset by peer'));
        this._handleClose();
    }

    /**
     * Simulate connection timeout
     * 
     * @param {number} timeoutMs - Timeout duration in ms
     */
    simulateConnectionTimeout(timeoutMs = 30000) {
        setTimeout(() => {
            const timeoutError = new Error(`SSE connection timeout after ${timeoutMs}ms`);
            timeoutError.code = 'ETIMEDOUT';
            this._response.destroy(timeoutError);
            this._handleClose();
        }, Math.min(timeoutMs, 100)); // Cap at 100ms for test speed
    }

    /**
     * Simulate malformed SSE data
     * 
     * @param {string} malformedData - Invalid SSE format data
     */
    simulateMalformedSSE(malformedData = 'invalid: format\nwithout proper terminator') {
        this._response.write(malformedData);
        // Don't terminate - let the parser handle the malformed data
    }

    /**
     * Simulate slow connection with delays between events
     * 
     * @param {Array} events - Array of events to send with delays
     * @param {number} baseDelay - Base delay between events in ms
     */
    async simulateSlowConnection(events = [], baseDelay = 100) {
        for (let i = 0; i < events.length; i++) {
            const event = events[i];
            const delay = Math.min(baseDelay, 10); // Cap at 10ms for test speed
            
            await new Promise(resolve => setTimeout(resolve, delay));
            this.injectEvent(event);
        }
    }
}

/**
 * Create a mock request object for testing SSE routes
 * 
 * @param {Object} options - Request options
 * @returns {Object} Mock request object
 */
export function createMockRequest(options = {}) {
    const req = new EventEmitter();
    
    req.headers = options.headers || {
        'accept': 'text/event-stream',
        'cookie': options.sessionId ? `web_session_id=${options.sessionId}` : ''
    };
    req.cookies = options.cookies || {};
    req.webSessionId = options.sessionId || 'mock-session-id';
    req.userId = options.userId || 'mock-user-id';
    req.session = options.session || {
        id: req.webSessionId,
        user_id: req.userId,
        organization_id: options.organizationId || 'mock-org-id',
        user_data: options.userData || { email: 'mock@test.com' }
    };
    req.ip = options.ip || '127.0.0.1';
    req.connection = {
        remoteAddress: options.remoteAddress || '127.0.0.1'
    };
    
    req.setTimeout = () => req;
    
    req.simulateClose = () => {
        req.emit('close');
    };
    
    req.simulateError = (error) => {
        req.emit('error', error || new Error('Mock connection error'));
    };
    
    return req;
}

/**
 * SSE Test Harness - Complete setup for testing SSE routes
 * 
 * Creates mock request, response, and client in one call
 * 
 * @param {Object} options - Harness options
 * @returns {{ req: Object, res: MockSSEResponse, client: MockSSEBrowser }}
 */
export function createSSETestHarness(options = {}) {
    const req = createMockRequest(options);
    const { response, client } = MockSSEBrowser.createPair(options);
    
    return {
        req,
        res: response,
        client,
        cleanup: () => {
            client.close();
            response.destroy();
        }
    };
}

export default MockSSEBrowser;
