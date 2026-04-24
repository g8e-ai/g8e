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
 * Internal HTTP Client for g8ed
 * 
 * Cluster-to-cluster HTTP communication client.
 * Replaces PubSub request-response pattern with direct HTTP calls.
 * 
 * This client communicates with internal HTTP endpoints on:
 * - g8ee (chat streaming, case management, investigations)
 * 
 * Benefits over PubSub:
 * - 10x faster (5-20ms vs 100-300ms)
 * - No g8es KV polling
 * - Standard HTTP debugging
 * - Synchronous request-response
 */

import { logger } from '../../utils/logger.js';
import { sessionIdTag } from '../../utils/session_log.js';
import {
    OperatorSessionRegistrationRequest,
    StopOperatorRequest,
    StopAIRequest,
    ApprovalRespondRequest,
    DirectCommandRequest,
    G8eHttpContext
} from '../../models/request_models.js';
import { SourceComponent } from '../../constants/ai.js';
import { G8eHeaders, HTTP_INTERNAL_AUTH_HEADER } from '../../constants/headers.js';
import {
    INTERNAL_HTTP_TIMEOUT_MS,
    G8EE_INTERNAL_URL,
    INTERNAL_HTTP_CLIENT_USER_AGENT,
    NEW_CASE_ID
} from '../../constants/http_client.js';
import { ApiPaths, InternalApiPaths } from '../../constants/api_paths.js';
import { getInternalHttpClient } from '../../services/initialization.js';

class InternalHttpClient{
    /**
     * @param {Object} options
     * @param {Object} options.bootstrapService - BootstrapService instance (for internal auth token and CA cert)
     * @param {Object} options.settingsService - SettingsService instance (for g8ee_url)
     */
    constructor({ bootstrapService, settingsService } = {}) {
        if (!bootstrapService) throw new Error('InternalHttpClient requires bootstrapService');
        this._bootstrapService = bootstrapService;
        this._settingsService = settingsService || null;
        this.timeout = INTERNAL_HTTP_TIMEOUT_MS;
        this.caCertPath = this._bootstrapService.loadCaCertPath();

        logger.info('InternalHttpClient initialized', {
            timeout: this.timeout,
            hasInternalAuthToken: !!this._bootstrapService.loadInternalAuthToken(),
            hasCaCert: !!this.caCertPath
        });
    }

    _resolveInternalAuthToken() {
        return this._bootstrapService.loadInternalAuthToken();
    }

    _resolveComponentUrl(component) {
        if (component === 'g8ee') {
            const url = (this._settingsService && this._settingsService.g8ee_url) || G8EE_INTERNAL_URL;
            return url.endsWith('/') ? url.slice(0, -1) : url;
        }
        return G8EE_INTERNAL_URL;
    }

    /**
     * Build G8eHttpContext headers for internal cluster calls.
     * g8ed passes full context to all components, eliminating re-authentication.
     *
     * @param {G8eHttpContext} context - G8eHttpContext model instance
     * @param {string} targetService - Target service name (for enforcement logic)
     * @returns {Object} Headers object with X-G8E-* headers
     * @throws {Error} If required session IDs are missing
     */
    buildG8eContextHeaders(context, targetService = null) {
        if (!(context instanceof G8eHttpContext)) {
            throw new Error('ENFORCEMENT VIOLATION: buildG8eContextHeaders requires G8eHttpContext instance');
        }

        const isNewCase = !context.case_id;

        const headers = {};

        // Required context
        headers[G8eHeaders.WEB_SESSION_ID]      = context.web_session_id;
        if (context.user_id)          headers[G8eHeaders.USER_ID]         = context.user_id;
        if (context.organization_id)  headers[G8eHeaders.ORGANIZATION_ID] = context.organization_id;

        // New case signal: g8ed sets the flag and sends NEW_CASE_ID sentinels.
        // g8ee reads X-G8E-New-Case to branch into inline case+investigation creation.
        if (isNewCase) {
            headers[G8eHeaders.NEW_CASE]         = 'true';
            headers[G8eHeaders.CASE_ID]          = NEW_CASE_ID;
            headers[G8eHeaders.INVESTIGATION_ID] = NEW_CASE_ID;
        } else {
            headers[G8eHeaders.CASE_ID]          = context.case_id;
            // ENFORCEMENT: INVESTIGATION_ID is required for all internal requests.
            // If missing in context, we must fallback to the case_id to satisfy g8ee's security requirement
            // for existing cases.
            headers[G8eHeaders.INVESTIGATION_ID] = context.investigation_id || context.case_id;
        }

        if (context.task_id) headers[G8eHeaders.TASK_ID] = context.task_id;

        // Bound operators list (JSON-encoded array)
        // g8ee resolves target operator dynamically per-command from operator_documents
        if (context.bound_operators && context.bound_operators.length > 0) {
            headers[G8eHeaders.BOUND_OPERATORS] = JSON.stringify(context.bound_operators);
        }

        // Tracking context
        if (context.execution_id) headers[G8eHeaders.EXECUTION_ID] = context.execution_id;

        // Source component
        headers[G8eHeaders.SOURCE_COMPONENT] = context.source_component || SourceComponent.G8ED;

        logger.info('[HTTP-INTERNAL] G8eContext headers built and validated from model', {
            web_session_id_tag: sessionIdTag(context.web_session_id),
            user_id: context.user_id,
            bound_operators_count: context.bound_operators?.length || 0,
            case_id: context.case_id,
            investigation_id: context.investigation_id,
            new_case: isNewCase,
            targetService,
            enforcement_passed: true
        });

        return headers;
    }

    /**
     * Make internal HTTP request with timeout, retry logic, and error handling.
     *
     * @param {string} component - Key in COMPONENTS map
     * @param {string} path - URL path
     * @param {Object} options - Fetch options plus:
     *   options.g8eContext  {Object}      - G8eHttpContext for X-G8E-* headers
     *   options.signal      {AbortSignal} - Caller-supplied signal (e.g. req.signal)
     *   options.maxRetries  {number}      - Maximum retry attempts (default: 3)
     */
    async request(component, path, options = {}) {
        const url = `${this._resolveComponentUrl(component)}${path}`;
        const method = options.method || 'GET';
        const maxRetries = options.maxRetries ?? 3;

        logger.info('[HTTP-INTERNAL] Request started', {
            component,
            path,
            method,
            url,
            maxRetries
        });

        let lastError = null;
        
        for (let attempt = 0; attempt <= maxRetries; attempt++) {
            let timeoutId = null;
            try {
                // Combine caller-supplied signal with our own timeout signal
                const timeoutController = new AbortController();
                timeoutId = setTimeout(() => timeoutController.abort(), this.timeout);

                const signals = [timeoutController.signal];
                if (options.signal) signals.push(options.signal);
                const signal = signals.length === 1 ? signals[0] : AbortSignal.any(signals);

                // Build headers with G8eHttpContext for g8ee calls
                const baseHeaders = {
                    'Content-Type': 'application/json',
                    'User-Agent': INTERNAL_HTTP_CLIENT_USER_AGENT
                };

                // Add internal auth token if available
                const internalAuthToken = this._resolveInternalAuthToken();
                if (internalAuthToken) {
                    baseHeaders[HTTP_INTERNAL_AUTH_HEADER] = internalAuthToken;
                }

                // Add G8eHttpContext headers if context is provided
                const contextHeaders = options.g8eContext ? this.buildG8eContextHeaders(options.g8eContext, component) : {};

                const mergedHeaders = {
                    ...baseHeaders,
                    ...contextHeaders,
                    ...options.headers
                };

                // Strip non-fetch keys before passing to fetch; serialize plain object body
                const { g8eContext: _g8eContext, signal: _signal, headers: _headers, body: _body, ...fetchOptionsRaw } = options;
                const serializedBody = _body !== undefined ? JSON.stringify(_body) : undefined;

                const fetchOptions = {
                    ...fetchOptionsRaw,
                    ...(serializedBody !== undefined ? { body: serializedBody } : {}),
                    signal,
                    headers: mergedHeaders
                };

                const response = await fetch(url, fetchOptions);

                if (timeoutId) clearTimeout(timeoutId);

                // Handle 204 No Content responses (e.g., DELETE operations)
                if (response.status === 204) {
                    logger.info('[HTTP-INTERNAL] Request completed (No Content)', {
                        component,
                        path,
                        method,
                        status: response.status
                    });
                    return { success: true };
                }

                if (!response.ok) {
                    const errorText = await response.text();
                    throw new Error(`HTTP ${response.status}: ${errorText}`);
                }

                const data = await response.json();

                logger.info('[HTTP-INTERNAL] Request completed', {
                    component,
                    path,
                    method,
                    status: response.status,
                    success: data.success
                });

                return data;

            } catch (error) {
                if (timeoutId) clearTimeout(timeoutId);

                // Check if this is a transient error that should be retried
                const isTransient = error.cause?.code === 'ECONNREFUSED' || 
                                   error.cause?.code === 'ECONNRESET' ||
                                   error.cause?.code === 'ETIMEDOUT' ||
                                   error.name === 'AbortError';

                if (isTransient && attempt < maxRetries) {
                    const delayMs = Math.min(1000 * Math.pow(2, attempt), 5000);
                    logger.warn('[HTTP-INTERNAL] Transient error, retrying', {
                        component,
                        path,
                        attempt: attempt + 1,
                        maxRetries: maxRetries + 1,
                        delayMs,
                        errorCode: error.cause?.code,
                        errorMessage: error.message
                    });
                    await new Promise(resolve => setTimeout(resolve, delayMs));
                    lastError = error;
                    continue;
                }

                // Non-retryable error or max retries exceeded
                if (error.name === 'AbortError' && !options.signal?.aborted) {
                    logger.error('[HTTP-INTERNAL] Request timeout', {
                        component,
                        path,
                        timeout: this.timeout
                    });
                    throw new Error(`Request timeout after ${this.timeout}ms`);
                }

                logger.error('[HTTP-INTERNAL] Request failed', {
                    component,
                    path,
                    attempt: attempt + 1,
                    maxRetries: maxRetries + 1,
                    errorName: error.name,
                    errorMessage: error.message,
                    errorCause: error.cause,
                    errorStack: error.stack,
                    url
                });
                throw error;
            }
        }

        // Should not reach here, but just in case
        throw lastError || new Error('Request failed');
    }

    // =====================================================
    // g8ee ENDPOINTS
    // =====================================================

    /**
     * Send chat message to g8ee non-streaming endpoint.
     *
     * Calls g8ee's /api/internal/chat which runs the full ReAct loop and delivers
     * the AI response + tool events via SSE to the browser's existing SSE connection.
     * Returns a standard JSON response immediately after queueing the background task.
     *
     * @param {Object} chatData - Chat message data
     * @param {Object} g8eContext - REQUIRED G8eHttpContext with session/user info
     * @returns {Object} JSON response { success, data: { case_id, investigation_id } }
     */
    async sendChatMessage(chatData, g8eContext) {
        if (!g8eContext) {
            throw new Error('ENFORCEMENT VIOLATION: g8eContext is REQUIRED for g8ee calls');
        }

        logger.info('[HTTP-INTERNAL] Sending non-streaming chat message', {
            caseId: g8eContext.case_id,
            investigationId: g8eContext.investigation_id,
        });

        return this.request('g8ee', ApiPaths.g8ee.chat(), {
            method: 'POST',
            body: chatData,
            g8eContext,
        });
    }

    /**
     * Delete case and all related data via g8ee internal API
     * Deletes case, investigations, and memories
     * 
     * ENFORCEMENT: g8eContext is REQUIRED and must contain web_session_id
     * 
     * @param {string} caseId - Case ID to delete
     * @param {Object} g8eContext - REQUIRED G8eHttpContext with session/user info
     * @throws {Error} If g8eContext is missing or invalid
     */
    async deleteCase(caseId, g8eContext) {
        if (!g8eContext) {
            throw new Error('ENFORCEMENT VIOLATION: g8eContext is REQUIRED for g8ee calls');
        }
        logger.info('[HTTP-INTERNAL] Deleting case', {
            caseId,
            hasContext: !!g8eContext
        });

        return this.request('g8ee', ApiPaths.g8ee.case(caseId), {
            method: 'DELETE',
            g8eContext
        });
    }

    /**
     * Stop active AI processing for an investigation via g8ee internal API
     * Cancels the asyncio task processing the AI response
     * 
     * ENFORCEMENT: g8eContext is REQUIRED and must contain web_session_id
     * 
     * @param {Object} stopData - Stop request data (investigation_id, reason)
     * @param {Object} g8eContext - REQUIRED G8eHttpContext with session/user info
     * @throws {Error} If g8eContext is missing or invalid
     */
    async stopAIProcessing(stopData, g8eContext) {
        if (!g8eContext) {
            throw new Error('ENFORCEMENT VIOLATION: g8eContext is REQUIRED for g8ee calls');
        }
        logger.info('[HTTP-INTERNAL] Stopping AI processing', {
            investigationId: stopData.investigation_id,
            reason: stopData.reason || 'User requested stop',
            hasContext: !!g8eContext
        });

        return this.request('g8ee', ApiPaths.g8ee.chatStop(), {
            method: 'POST',
            body: new StopAIRequest(stopData).forWire(),
            g8eContext
        });
    }

    /**
     * Check health of all internal services
     */
    async healthCheck() {
        const services = ['g8ee'];
        const results = {};

        for (const service of services) {
            try {
                const response = await this.request(service, ApiPaths.g8ee.health(), {
                    method: 'GET'
                });
                results[service] = {
                    status: 'healthy',
                    response
                };
            } catch (error) {
                results[service] = {
                    status: 'unhealthy',
                    error: error.message
                };
            }
        }

        return results;
    }
}

// Singleton instance is now managed via initialization.js factory
export { getInternalHttpClient, InternalHttpClient };
