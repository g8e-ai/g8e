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

import { EventType } from '../constants/events.js';
import { SSEClientConfig } from '../constants/sse-constants.js';
import { ComponentName } from '../constants/service-client-constants.js';
import { devLogger } from './dev-logger.js';
import { ApiPaths } from '../constants/api-paths.js';

const _INFRASTRUCTURE_EVENTS = new Set([
    EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED,
    EventType.PLATFORM_SSE_KEEPALIVE_SENT,
]);

class SSEConnectionManager {
    constructor(eventBus) {
        this.eventBus = eventBus;
        this.eventSource = null;
        this.isConnected = false;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = SSEClientConfig.MAX_RECONNECT_ATTEMPTS;
        this.baseReconnectDelay = SSEClientConfig.BASE_RECONNECT_DELAY_MS;
        this.maxReconnectDelay = SSEClientConfig.MAX_RECONNECT_DELAY_MS;
        this.activeWebSessionId = null;
        this.lastWebSessionId = null;
        this.keepaliveTimeout = null;
        this.lastActivity = Date.now();
        this.activityCheckInterval = null;
        this.connectionPromise = null;
        this.reconnectTimer = null;
        this.consecutiveFailures = 0;
        this.connectionStartTime = null;
        this._setupVisibilityHandling();
    }

    _setupVisibilityHandling() {
        if (typeof document !== 'undefined') {
            document.addEventListener('visibilitychange', () => {
                if (document.hidden) {
                    devLogger.log('Tab hidden - connection will be maintained');
                } else {
                    if (!this.isConnectionActive() && this.activeWebSessionId) {
                        devLogger.log('Tab visible - reconnecting SSE');
                        this.reconnectAttempts = 0;
                        this.connect(this.activeWebSessionId);
                    }
                }
            });
        }
    }

    async initializeConnection(webSessionId = null) {
        try {
            await this.connect(webSessionId);
            devLogger.log('SSE connection initialized successfully');
        } catch (error) {
            devLogger.error('Failed to initialize SSE connection:', error);
            this.scheduleReconnect();
        }
    }

    async connect(webSessionId = null) {
        const targetWebSessionId = webSessionId !== null && webSessionId !== undefined
            ? webSessionId
            : this.lastWebSessionId;

        if (!targetWebSessionId) {
            devLogger.warn('SSE connection requested without a webSessionId. Aborting connection attempt.');
            return;
        }

        if (this.isConnectionActiveFor(targetWebSessionId)) {
            return;
        }

        if (this.eventSource) {
            this.eventSource.onopen = null;
            this.eventSource.onmessage = null;
            this.eventSource.onerror = null;
            try {
                this.eventSource.close();
            } catch (closeError) {
                devLogger.warn('Failed to close previous SSE connection cleanly:', closeError);
            }
            this.eventSource = null;
            this.isConnected = false;

            if (this.keepaliveTimeout) {
                clearTimeout(this.keepaliveTimeout);
                this.keepaliveTimeout = null;
            }
        }

        this.lastWebSessionId = targetWebSessionId;
        this.activeWebSessionId = targetWebSessionId;

        this.eventSource = new EventSource(ApiPaths.sse.events(), { withCredentials: true });

        this.lastActivity = Date.now();
        this.connectionStartTime = Date.now();

        this.eventSource.onopen = () => {
            const connectionDuration = Date.now() - this.connectionStartTime;
            this.isConnected = true;
            this.reconnectAttempts = 0;
            this.consecutiveFailures = 0;

            devLogger.log(`[SSE] SSE connection established in ${connectionDuration}ms`);
            this.eventBus.emit(EventType.PLATFORM_SSE_CONNECTION_OPENED, {
                service: ComponentName.G8ED,
                webSessionId: targetWebSessionId,
                connectionTime: connectionDuration
            });
            this.resetKeepaliveTimeout();
        };

        this.eventSource.onmessage = (event) => {
            this.lastActivity = Date.now();
            this.resetKeepaliveTimeout();
            try {
                const data = JSON.parse(event.data);
                devLogger.log(`[SSE] Message received: ${data.type || 'unknown'}`, data);
                this.handleSSEEvent(data);
            } catch (error) {
                devLogger.error('Failed to parse SSE event data:', error, event.data);
            }
        };

        this.eventSource.onerror = (error) => {
            const connectionDuration = Date.now() - this.connectionStartTime;
            const isQuickFailure = connectionDuration < SSEClientConfig.QUICK_FAILURE_THRESHOLD_MS;

            devLogger.error(`SSE connection error after ${connectionDuration}ms:`, error);
            this.isConnected = false;

            if (isQuickFailure) {
                devLogger.warn('Quick connection failure detected - possible server issue');
            }

            if (this.keepaliveTimeout) {
                clearTimeout(this.keepaliveTimeout);
                this.keepaliveTimeout = null;
            }

            this.consecutiveFailures++;

            this.eventBus.emit(EventType.PLATFORM_SSE_CONNECTION_ERROR, {
                service: ComponentName.G8ED,
                error,
                connectionDuration,
                consecutiveFailures: this.consecutiveFailures
            });

            if (isQuickFailure && this.consecutiveFailures > SSEClientConfig.QUICK_FAILURE_BACKOFF_COUNT) {
                devLogger.warn('Multiple quick failures - backing off reconnection');
            }

            this.scheduleReconnect();
        };
    }

    resetKeepaliveTimeout() {
        clearTimeout(this.keepaliveTimeout);
        this.keepaliveTimeout = setTimeout(() => {
            devLogger.warn(`SSE keepalive timeout - no message in ${SSEClientConfig.KEEPALIVE_TIMEOUT_MS / 1000}s. Reconnecting...`);
            if (this.eventSource) {
                try {
                    this.eventSource.close();
                } catch (e) {
                    devLogger.warn('Failed to close EventSource during keepalive timeout:', e);
                }
                this.eventSource = null;
            }
            this.isConnected = false;
            this.scheduleReconnect();
        }, SSEClientConfig.KEEPALIVE_TIMEOUT_MS);
    }

    scheduleReconnect() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            devLogger.error('Max reconnection attempts reached. Manual reconnection required.');
            this.eventBus.emit(EventType.PLATFORM_SSE_CONNECTION_FAILED, {
                service: ComponentName.G8ED,
                reason: SSEClientConfig.RECONNECT_FAILURE_REASON
            });
            return;
        }

        const exponentialDelay = Math.min(
            this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts),
            this.maxReconnectDelay
        );

        const jitter = exponentialDelay * 0.25 * (Math.random() * 2 - 1);
        const delay = Math.max(exponentialDelay + jitter, SSEClientConfig.MIN_RECONNECT_DELAY_MS);

        this.reconnectAttempts++;
        this.consecutiveFailures++;

        devLogger.log(`Scheduling reconnection attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts} in ${Math.round(delay)}ms (base=${exponentialDelay}ms, jitter=${Math.round(jitter)}ms)`);

        this.reconnectTimer = setTimeout(async () => {
            if (this.activeWebSessionId) {
                const shouldReconnect = await this._validateReconnectionNeeded();
                if (shouldReconnect) {
                    this.connect(this.activeWebSessionId);
                } else {
                    devLogger.log('Connection health check passed - reconnect not needed');
                    this.reconnectAttempts = 0;
                    this.consecutiveFailures = 0;
                }
            }
        }, delay);
    }

    async _validateReconnectionNeeded() {
        if (this.isConnectionActive()) {
            return false;
        }

        if (this.connectionPromise) {
            devLogger.log('Connection attempt already in progress');
            return false;
        }

        return true;
    }

    handleSSEEvent(data) {
        const { type: eventType, data: payload } = data;

        if (!eventType || typeof eventType !== 'string') {
            devLogger.warn('[SSE] Received event with missing or non-string type', data);
            return { handled: false, eventType };
        }

        if (eventType === EventType.PLATFORM_SSE_KEEPALIVE_SENT) {
            if (data.operator_list) {
                this.eventBus.emit(EventType.OPERATOR_PANEL_LIST_UPDATED, data.operator_list);
            }
        }

        if (_INFRASTRUCTURE_EVENTS.has(eventType)) {
            return { handled: true, infrastructure: true };
        }

        if (payload === undefined) {
            devLogger.warn('[SSE] Received non-infrastructure event with no data field — dropped', data);
            return { handled: false, eventType };
        }

        const validationError = this._validateEventPayload(eventType, payload);
        if (validationError) {
            devLogger.error('[SSE] Event payload validation failed', {
                eventType,
                error: validationError,
                payload
            });
            return { handled: false, eventType };
        }

        if (eventType.startsWith('g8e.v1.ai.tribunal')) {
            console.log('[SSE] Tribunal event received:', eventType, payload);
        }

        this.eventBus.emit(eventType, payload);
        return { handled: true, eventType };
    }

    _validateEventPayload(eventType, payload) {
        if (payload === undefined) {
            return 'Payload must be provided';
        }

        const requiredFields = this._getRequiredFieldsForEventType(eventType);
        
        // For events with no required fields, allow null as payload
        if (!requiredFields) {
            return null;
        }

        // For events with required fields, null is not valid
        if (payload === null) {
            return 'Payload must be an object';
        }

        if (typeof payload !== 'object') {
            return 'Payload must be an object';
        }

        for (const field of requiredFields) {
            if (!(field in payload)) {
                return `Missing required field: ${field}`;
            }
        }

        return null;
    }

    _getRequiredFieldsForEventType(eventType) {
        const fieldMap = {
            [EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED]: ['web_session_id', 'content'],
            [EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED]: ['web_session_id'],
            [EventType.LLM_CHAT_ITERATION_COMPLETED]: ['web_session_id'],
            [EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED]: ['web_session_id', 'grounding_metadata'],
            [EventType.LLM_CHAT_ITERATION_FAILED]: [],
            [EventType.LLM_CHAT_ITERATION_STOPPED]: ['web_session_id'],
            [EventType.LLM_CHAT_ITERATION_THINKING_STARTED]: ['web_session_id'],
            [EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED]: ['web_session_id', 'execution_id', 'query'],
            [EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED]: ['execution_id'],
            [EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED]: ['execution_id'],
            [EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED]: ['web_session_id', 'execution_id', 'port'],
            [EventType.OPERATOR_NETWORK_PORT_CHECK_STARTED]: ['web_session_id', 'execution_id', 'port'],
            [EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED]: ['execution_id'],
            [EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED]: ['execution_id'],
            [EventType.TRIBUNAL_SESSION_STARTED]: ['web_session_id'],
            [EventType.TRIBUNAL_VOTING_PASS_COMPLETED]: ['web_session_id', 'pass_index', 'success'],
            [EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED]: ['web_session_id'],
            [EventType.TRIBUNAL_VOTING_REVIEW_STARTED]: ['web_session_id'],
            [EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED]: ['web_session_id'],
            [EventType.TRIBUNAL_SESSION_COMPLETED]: ['web_session_id'],
            [EventType.TRIBUNAL_SESSION_DISABLED]: ['web_session_id'],
            [EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED]: ['web_session_id'],
            [EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE]: ['web_session_id'],
            [EventType.TRIBUNAL_SESSION_SYSTEM_ERROR]: ['web_session_id'],
            [EventType.TRIBUNAL_SESSION_GENERATION_FAILED]: ['web_session_id'],
            [EventType.TRIBUNAL_SESSION_VERIFIER_FAILED]: ['web_session_id'],
        };

        return fieldMap[eventType] || null;
    }

    isConnectionActive() {
        return this.isConnected && this.eventSource && this.eventSource.readyState === EventSource.OPEN;
    }

    disconnect() {
        if (this.eventSource) {
            devLogger.log('Closing SSE connection');
            this.eventSource.close();
            this.eventSource = null;
        }

        if (this.keepaliveTimeout) {
            clearTimeout(this.keepaliveTimeout);
            this.keepaliveTimeout = null;
        }

        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        this.isConnected = false;
        this.activeWebSessionId = null;
        this.reconnectAttempts = 0;
        this.consecutiveFailures = 0;
    }

    getConnectionStatus() {
        return {
            isConnected: this.isConnected,
            readyState: this.eventSource ? this.eventSource.readyState : EventSource.CLOSED,
            url: this.eventSource ? this.eventSource.url : null,
            reconnectAttempts: this.reconnectAttempts
        };
    }

    isConnectionActiveFor(webSessionId) {
        if (!webSessionId) {
            return false;
        }

        return this.isConnectionActive() && this.activeWebSessionId === webSessionId;
    }
}

export { SSEConnectionManager };
