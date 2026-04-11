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
 * SSE Test Client - Production-Quality SSE Testing
 * 
 * Provides a robust, reusable SSE client for integration testing.
 * Uses the EventSource API via the 'eventsource' package.
 * 
 * Features:
 * - Promise-based event waiting with configurable timeouts
 * - Event collection with filtering by type
 * - Automatic reconnection testing
 * - Connection state tracking
 * - Proper cleanup and resource management
 * - Support for authenticated connections
 * 
 * Usage:
 *   const client = new SSETestClient(baseUrl, { sessionId: 'web-session-123' });
 *   await client.connect();
 *   const event = await client.waitForEvent('connection_established');
 *   const events = await client.collectEvents('keepalive', 3);
 *   client.close();
 */

import { EventSource } from 'eventsource';
import { EventType, ConnectionState } from '../../constants/events.js';

export { ConnectionState };

/**
 * SSE Test Client Class
 * 
 * A comprehensive client for testing Server-Sent Events endpoints.
 */
export class SSETestClient {
    /**
     * Create a new SSE test client
     * 
     * @param {string} baseUrl - Base URL of the server
     * @param {Object} options - Configuration options
     * @param {string} options.sessionId - Web session ID for authentication
     * @param {string} options.path - SSE endpoint path (default: '/sse/events')
     * @param {number} options.defaultTimeout - Default timeout for operations in ms (default: 10000)
     * @param {Object} options.headers - Additional headers to send
     * @param {boolean} options.withCredentials - Whether to send credentials (default: true)
     */
    constructor(baseUrl, options = {}) {
        this.baseUrl = baseUrl.replace(/\/$/, ''); // Remove trailing slash
        this.path = options.path || '/sse/events';
        this.sessionId = options.sessionId;
        this.defaultTimeout = options.defaultTimeout || 10000;
        this.headers = options.headers || {};
        this.withCredentials = options.withCredentials !== false;

        // State tracking
        this.eventSource = null;
        this.state = ConnectionState.DISCONNECTED;
        this.events = [];
        this.eventListeners = new Map();
        this.connectionId = null;
        this.reconnectCount = 0;
        this.lastEventId = null;

        // Error tracking
        this.lastError = null;
        this.errors = [];

        // Metrics
        this.metrics = {
            connectedAt: null,
            disconnectedAt: null,
            totalEvents: 0,
            eventsByType: new Map()
        };
    }

    /**
     * Get the full SSE URL with authentication
     */
    get url() {
        return `${this.baseUrl}${this.path}`;
    }

    /**
     * Connect to the SSE endpoint
     * 
     * @param {number} timeout - Connection timeout in ms
     * @returns {Promise<Object>} Connection established event data
     */
    async connect(timeout = this.defaultTimeout) {
        if (this.state === ConnectionState.CONNECTED) {
            throw new Error('SSETestClient: Already connected');
        }

        this.state = ConnectionState.CONNECTING;

        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                if (this.eventSource) {
                    this.eventSource.close();
                }
                this.state = ConnectionState.ERROR;
                reject(new Error(`SSETestClient: Connection timeout after ${timeout}ms`));
            }, timeout);

            // Build headers with session cookie for eventsource v4 fetch wrapper
            const customHeaders = { ...this.headers };
            if (this.sessionId) {
                customHeaders['Cookie'] = `web_session_id=${this.sessionId}`;
            }

            try {
                // eventsource v4 API: use fetch option to customize request
                this.eventSource = new EventSource(this.url, {
                    fetch: (input, init) => fetch(input, {
                        ...init,
                        headers: {
                            ...init.headers,
                            ...customHeaders
                        }
                    })
                });
            } catch (err) {
                clearTimeout(timer);
                this.state = ConnectionState.ERROR;
                reject(new Error(`SSETestClient: Failed to create EventSource: ${err.message}`));
                return;
            }

            // Handle connection open
            this.eventSource.onopen = () => {
                this.state = ConnectionState.CONNECTED;
                this.metrics.connectedAt = Date.now();
            };

            // Handle errors
            this.eventSource.onerror = (err) => {
                const errorInfo = {
                    timestamp: Date.now(),
                    readyState: this.eventSource?.readyState,
                    message: err.message || 'Unknown SSE error',
                    code: err.code
                };
                this.errors.push(errorInfo);
                this.lastError = errorInfo;

                // Only reject if still connecting (not for reconnection attempts)
                if (this.state === ConnectionState.CONNECTING) {
                    clearTimeout(timer);
                    this.state = ConnectionState.ERROR;
                    reject(new Error(`SSETestClient: Connection failed: ${errorInfo.message}`));
                }
            };

            // Handle messages
            this.eventSource.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    this._recordEvent(data, event);

                    // Track Last-Event-ID if present
                    if (event.lastEventId) {
                        this.lastEventId = event.lastEventId;
                    }

                    if (data.type === EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED) {
                        clearTimeout(timer);
                        this.connectionId = data.connectionId;
                        resolve(data);
                    }

                    // Notify any waiting listeners
                    this._notifyListeners(data);
                } catch (parseError) {
                    // Non-JSON message, store as raw
                    this._recordEvent({ type: 'raw', data: event.data }, event);
                }
            };
        });
    }

    /**
     * Record an event and update metrics
     */
    _recordEvent(data, rawEvent) {
        const record = {
            data,
            timestamp: Date.now(),
            lastEventId: rawEvent?.lastEventId || null
        };
        this.events.push(record);
        this.metrics.totalEvents++;

        const type = data.type || 'None';
        const count = this.metrics.eventsByType.get(type) || 0;
        this.metrics.eventsByType.set(type, count + 1);
    }

    /**
     * Notify registered event listeners
     */
    _notifyListeners(data) {
        for (const [listenerId, listener] of this.eventListeners) {
            if (listener.predicate(data)) {
                listener.resolve(data);
                this.eventListeners.delete(listenerId);
            }
        }
    }

    /**
     * Wait for a specific event type
     * 
     * @param {string} eventType - Event type to wait for
     * @param {number} timeout - Timeout in ms (default: defaultTimeout)
     * @returns {Promise<Object>} Event data
     */
    async waitForEvent(eventType, timeout = this.defaultTimeout) {
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
    async waitForEventMatching(predicate, timeout = this.defaultTimeout, description = 'matching event') {
        // Check if we already have a matching event
        const existing = this.events.find(e => predicate(e.data));
        if (existing) {
            return existing.data;
        }

        return new Promise((resolve, reject) => {
            const listenerId = `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

            const timer = setTimeout(() => {
                this.eventListeners.delete(listenerId);
                reject(new Error(`SSETestClient: Timeout waiting for ${description} after ${timeout}ms`));
            }, timeout);

            this.eventListeners.set(listenerId, {
                predicate,
                resolve: (data) => {
                    clearTimeout(timer);
                    resolve(data);
                }
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
    async collectEvents(eventType, count, timeout = this.defaultTimeout) {
        const already = this.events
            .filter(e => e.data.type === eventType)
            .map(e => e.data);
        if (already.length >= count) {
            return already.slice(0, count);
        }

        return new Promise((resolve, reject) => {
            const listenerId = `collect-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

            const timer = setTimeout(() => {
                this.eventListeners.delete(listenerId);
                const got = this.events.filter(e => e.data.type === eventType).length;
                reject(new Error(
                    `SSETestClient: Timeout collecting ${count} '${eventType}' events. Got ${got}`
                ));
            }, timeout);

            this.eventListeners.set(listenerId, {
                predicate: () => {
                    return this.events.filter(e => e.data.type === eventType).length >= count;
                },
                resolve: () => {
                    clearTimeout(timer);
                    resolve(
                        this.events
                            .filter(e => e.data.type === eventType)
                            .map(e => e.data)
                            .slice(0, count)
                    );
                },
                reject: (err) => {
                    clearTimeout(timer);
                    reject(err);
                }
            });
        });
    }

    /**
     * Collect all events for a duration
     * 
     * @param {number} duration - Duration to collect in ms
     * @param {string} eventType - Optional: only collect events of this type
     * @returns {Promise<Array>} Array of event data objects
     */
    async collectEventsFor(duration, eventType = null) {
        const startIndex = this.events.length;
        await new Promise(resolve => setTimeout(resolve, duration));

        let collected = this.events.slice(startIndex).map(e => e.data);
        if (eventType) {
            collected = collected.filter(e => e.type === eventType);
        }
        return collected;
    }

    /**
     * Get all events of a specific type received so far
     * 
     * @param {string} eventType - Event type to filter by
     * @returns {Array} Array of event data objects
     */
    getEventsByType(eventType) {
        return this.events
            .filter(e => e.data.type === eventType)
            .map(e => e.data);
    }

    /**
     * Get all received events
     * 
     * @returns {Array} Array of event records with data and metadata
     */
    getAllEvents() {
        return [...this.events];
    }

    /**
     * Get the most recent event of a type
     * 
     * @param {string} eventType - Event type to find
     * @returns {Object|null} Event data or null if not found
     */
    getLastEvent(eventType = null) {
        const filtered = eventType
            ? this.events.filter(e => e.data.type === eventType)
            : this.events;
        
        if (filtered.length === 0) return null;
        return filtered[filtered.length - 1].data;
    }

    /**
     * Clear collected events
     */
    clearEvents() {
        this.events = [];
    }

    /**
     * Simulate a reconnection by closing and reopening the connection
     * 
     * @param {number} delayMs - Delay before reconnecting (default: 100ms)
     * @returns {Promise<Object>} New connection established event
     */
    async reconnect(delayMs = 100) {
        const previousConnectionId = this.connectionId;
        this.close();
        
        this.state = ConnectionState.RECONNECTING;
        this.reconnectCount++;

        await new Promise(resolve => setTimeout(resolve, delayMs));

        const result = await this.connect();
        result.previousConnectionId = previousConnectionId;
        return result;
    }

    /**
     * Close the SSE connection
     */
    close() {
        if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
        }
        this.state = ConnectionState.CLOSED;
        this.metrics.disconnectedAt = Date.now();

        for (const [listenerId, listener] of this.eventListeners) {
            listener.reject(new Error('SSETestClient: Connection closed'));
        }
        this.eventListeners.clear();
    }

    /**
     * Check if connected
     * 
     * @returns {boolean} True if connected
     */
    isConnected() {
        return this.state === ConnectionState.CONNECTED &&
               this.eventSource?.readyState === EventSource.OPEN;
    }

    /**
     * Get connection metrics
     * 
     * @returns {Object} Metrics including connection time, event counts, etc.
     */
    getMetrics() {
        const now = Date.now();
        return {
            ...this.metrics,
            connectionDuration: this.metrics.connectedAt
                ? (this.metrics.disconnectedAt || now) - this.metrics.connectedAt
                : 0,
            eventsPerSecond: this.metrics.connectedAt
                ? (this.metrics.totalEvents / ((now - this.metrics.connectedAt) / 1000)).toFixed(2)
                : 0,
            eventsByType: Object.fromEntries(this.metrics.eventsByType),
            state: this.state,
            reconnectCount: this.reconnectCount,
            errorCount: this.errors.length
        };
    }

    /**
     * Assert that a specific event was received
     * 
     * @param {string} eventType - Event type to check
     * @param {Object} expectedData - Optional data properties to match
     * @throws {Error} If event not found or data doesn't match
     */
    assertEventReceived(eventType, expectedData = null) {
        const events = this.getEventsByType(eventType);
        if (events.length === 0) {
            throw new Error(`SSETestClient: Expected event '${eventType}' was not received`);
        }

        if (expectedData) {
            const matching = events.find(event => {
                for (const [key, value] of Object.entries(expectedData)) {
                    if (JSON.stringify(event.data?.[key]) !== JSON.stringify(value) &&
                        JSON.stringify(event[key]) !== JSON.stringify(value)) {
                        return false;
                    }
                }
                return true;
            });

            if (!matching) {
                throw new Error(
                    `SSETestClient: Event '${eventType}' received but data doesn't match. ` +
                    `Expected: ${JSON.stringify(expectedData)}, Got: ${JSON.stringify(events)}`
                );
            }
        }
    }

    /**
     * Assert event count for a type
     * 
     * @param {string} eventType - Event type to check
     * @param {number} expectedCount - Expected count
     * @param {string} comparison - 'eq', 'gte', 'lte', 'gt', 'lt' (default: 'eq')
     * @throws {Error} If count doesn't match
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
                `SSETestClient: Expected ${comparison} ${expectedCount} '${eventType}' events, got ${count}`
            );
        }
    }
}

/**
 * Factory function to create and connect an SSE test client
 * 
 * @param {string} baseUrl - Server base URL
 * @param {Object} options - Client options
 * @returns {Promise<SSETestClient>} Connected client
 */
export async function createConnectedClient(baseUrl, options = {}) {
    const client = new SSETestClient(baseUrl, options);
    await client.connect();
    return client;
}

/**
 * Create multiple connected clients for concurrent testing
 * 
 * @param {string} baseUrl - Server base URL
 * @param {number} count - Number of clients to create
 * @param {Function} optionsFactory - Function that takes index and returns options
 * @returns {Promise<Array<SSETestClient>>} Array of connected clients
 */
export async function createClientPool(baseUrl, count, optionsFactory) {
    const clients = [];
    for (let i = 0; i < count; i++) {
        const options = optionsFactory(i);
        const client = await createConnectedClient(baseUrl, options);
        clients.push(client);
    }
    return clients;
}

/**
 * Close all clients in a pool
 * 
 * @param {Array<SSETestClient>} clients - Array of clients to close
 */
export function closeClientPool(clients) {
    for (const client of clients) {
        try {
            client.close();
        } catch (err) {
            // Ignore cleanup errors
        }
    }
}

export default SSETestClient;
