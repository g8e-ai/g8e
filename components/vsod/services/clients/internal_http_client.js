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
 * Internal HTTP Client for VSOD
 * 
 * Cluster-to-cluster HTTP communication client.
 * Replaces PubSub request-response pattern with direct HTTP calls.
 * 
 * This client communicates with internal HTTP endpoints on:
 * - g8ee (chat streaming, case management, investigations)
 * 
 * Benefits over PubSub:
 * - 10x faster (5-20ms vs 100-300ms)
 * - No VSODB KV polling
 * - Standard HTTP debugging
 * - Synchronous request-response
 */

import { logger } from '../../utils/logger.js';
import {
    OperatorSessionRegistrationRequest,
    StopOperatorRequest,
    StopAIRequest,
    ApprovalRespondRequest,
    DirectCommandRequest,
    VSOHttpContext
} from '../../models/request_models.js';
import { SourceComponent } from '../../constants/ai.js';
import { VSOHeaders, HTTP_INTERNAL_AUTH_HEADER } from '../../constants/headers.js';
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

    _resolveServiceUrl(service) {
        if (service === 'g8ee') {
            const url = (this._settingsService && this._settingsService.g8ee_url) || G8EE_INTERNAL_URL;
            return url.endsWith('/') ? url.slice(0, -1) : url;
        }
        return G8EE_INTERNAL_URL;
    }

    /**
     * Build VSOHttpContext headers for internal cluster calls.
     * VSOD passes full context to all components, eliminating re-authentication.
     *
     * @param {VSOHttpContext} context - VSOHttpContext model instance
     * @param {string} targetService - Target service name (for enforcement logic)
     * @returns {Object} Headers object with X-VSO-* headers
     * @throws {Error} If required session IDs are missing
     */
    buildVSOContextHeaders(context, targetService = null) {
        if (!(context instanceof VSOHttpContext)) {
            throw new Error('ENFORCEMENT VIOLATION: buildVSOContextHeaders requires VSOHttpContext instance');
        }

        const isNewCase = !context.case_id;

        const headers = {};

        // Required context
        headers[VSOHeaders.WEB_SESSION_ID]      = context.web_session_id;
        if (context.user_id)          headers[VSOHeaders.USER_ID]         = context.user_id;
        if (context.organization_id)  headers[VSOHeaders.ORGANIZATION_ID] = context.organization_id;

        // New case signal: VSOD sets the flag and sends NEW_CASE_ID sentinels.
        // g8ee reads X-VSO-New-Case to branch into inline case+investigation creation.
        if (isNewCase) {
            headers[VSOHeaders.NEW_CASE]         = 'true';
            headers[VSOHeaders.CASE_ID]          = NEW_CASE_ID;
            headers[VSOHeaders.INVESTIGATION_ID] = NEW_CASE_ID;
        } else {
            headers[VSOHeaders.CASE_ID]          = context.case_id;
            // ENFORCEMENT: INVESTIGATION_ID is required for all internal requests.
            // If missing in context, we must fallback to the case_id to satisfy g8ee's security requirement
            // for existing cases.
            headers[VSOHeaders.INVESTIGATION_ID] = context.investigation_id || context.case_id;
        }

        if (context.task_id) headers[VSOHeaders.TASK_ID] = context.task_id;

        // Bound operators list (JSON-encoded array)
        // g8ee resolves target operator dynamically per-command from operator_documents
        if (context.bound_operators && context.bound_operators.length > 0) {
            headers[VSOHeaders.BOUND_OPERATORS] = JSON.stringify(context.bound_operators);
        }

        // Tracking context
        if (context.execution_id) headers[VSOHeaders.EXECUTION_ID] = context.execution_id;

        // Source component
        headers[VSOHeaders.SOURCE_COMPONENT] = context.source_component || SourceComponent.VSOD;

        logger.info('[HTTP-INTERNAL] VSOContext headers built and validated from model', {
            web_session_id: context.web_session_id.substring(0, 20) + '...',
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
     * Make internal HTTP request with timeout and error handling.
     *
     * @param {string} service - Key in SERVICES map
     * @param {string} path - URL path
     * @param {Object} options - Fetch options plus:
     *   options.vsoContext  {Object}      - VSOHttpContext for X-VSO-* headers
     *   options.signal      {AbortSignal} - Caller-supplied signal (e.g. req.signal)
     */
    async request(service, path, options = {}) {
        const url = `${this._resolveServiceUrl(service)}${path}`;
        const method = options.method || 'GET';

        logger.info('[HTTP-INTERNAL] Request started', {
            service,
            path,
            method,
            url
        });

        // Combine caller-supplied signal with our own timeout signal
        const timeoutController = new AbortController();
        const timeoutId = setTimeout(() => timeoutController.abort(), this.timeout);

        const signals = [timeoutController.signal];
        if (options.signal) signals.push(options.signal);
        const signal = signals.length === 1 ? signals[0] : AbortSignal.any(signals);

        // Build headers with VSOHttpContext for g8ee calls
        const baseHeaders = {
            'Content-Type': 'application/json',
            'User-Agent': INTERNAL_HTTP_CLIENT_USER_AGENT
        };

        // Add internal auth token if available
        const internalAuthToken = this._resolveInternalAuthToken();
        if (internalAuthToken) {
            baseHeaders[HTTP_INTERNAL_AUTH_HEADER] = internalAuthToken;
        }

        // Add VSOHttpContext headers if context is provided
        const contextHeaders = options.vsoContext ? this.buildVSOContextHeaders(options.vsoContext, service) : {};

        const mergedHeaders = {
            ...baseHeaders,
            ...contextHeaders,
            ...options.headers
        };

        // Strip non-fetch keys before passing to fetch; serialize plain object body
        const { vsoContext: _vsoContext, signal: _signal, headers: _headers, body: _body, ...fetchOptionsRaw } = options;
        const serializedBody = _body !== undefined ? JSON.stringify(_body) : undefined;

        try {
            const fetchOptions = {
                ...fetchOptionsRaw,
                ...(serializedBody !== undefined ? { body: serializedBody } : {}),
                signal,
                headers: mergedHeaders
            };

            const response = await fetch(url, fetchOptions);

            clearTimeout(timeoutId);

            // Handle 204 No Content responses (e.g., DELETE operations)
            if (response.status === 204) {
                logger.info('[HTTP-INTERNAL] Request completed (No Content)', {
                    service,
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
                service,
                path,
                method,
                status: response.status,
                success: data.success
            });

            return data;

        } catch (error) {
            clearTimeout(timeoutId);

            if (error.name === 'AbortError') {
                if (options.signal?.aborted) {
                    logger.info('[HTTP-INTERNAL] Request cancelled by caller', { service, path });
                    throw error;
                }
                logger.error('[HTTP-INTERNAL] Request timeout', {
                    service,
                    path,
                    timeout: this.timeout
                });
                throw new Error(`Request timeout after ${this.timeout}ms`);
            }

            logger.error('[HTTP-INTERNAL] Request failed', {
                service,
                path,
                error: error.message
            });
            throw error;
        }
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
     * @param {Object} vsoContext - REQUIRED VSOHttpContext with session/user info
     * @returns {Object} JSON response { success, data: { case_id, investigation_id } }
     */
    async sendChatMessage(chatData, vsoContext) {
        if (!vsoContext) {
            throw new Error('ENFORCEMENT VIOLATION: vsoContext is REQUIRED for g8ee calls');
        }

        logger.info('[HTTP-INTERNAL] Sending non-streaming chat message', {
            caseId: vsoContext.case_id,
            investigationId: vsoContext.investigation_id,
        });

        return this.request('g8ee', ApiPaths.g8ee.chat(), {
            method: 'POST',
            body: chatData,
            vsoContext,
        });
    }

    /**
     * Query investigations via g8ee API
     * Replaces: PubSub publish to vso-g8ee-investigation-queries-topic
     * 
     * ENFORCEMENT: vsoContext is REQUIRED and must contain web_session_id
     * 
     * @param {Object} query - Query parameters
     * @param {Object} vsoContext - REQUIRED VSOHttpContext with session/user info
     * @throws {Error} If vsoContext is missing or invalid
     */
    async queryInvestigations(params, vsoContext) {
        if (!vsoContext) {
            throw new Error('ENFORCEMENT VIOLATION: vsoContext is REQUIRED for g8ee calls');
        }

        logger.info('[HTTP-INTERNAL] Querying investigations', {
            userId: vsoContext.user_id,
            caseId: vsoContext.case_id,
        });

        let path = ApiPaths.g8ee.investigations();
        if (params) {
            const queryString = params.toString();
            if (queryString) {
                path += `?${queryString}`;
            }
        }

        return this.request('g8ee', path, {
            method: 'GET',
            vsoContext
        });
    }

    /**
     * Get single investigation by ID via g8ee internal API
     * 
     * ENFORCEMENT: vsoContext is REQUIRED and must contain web_session_id
     * 
     * @param {string} investigationId - Investigation ID
     * @param {Object} vsoContext - REQUIRED VSOHttpContext with session/user info
     * @throws {Error} If vsoContext is missing or invalid
     */
    async getInvestigation(investigationId, vsoContext) {
        if (!vsoContext) {
            throw new Error('ENFORCEMENT VIOLATION: vsoContext is REQUIRED for g8ee calls');
        }
        logger.info('[HTTP-INTERNAL] Getting investigation', {
            investigationId,
            hasContext: !!vsoContext
        });

        return this.request('g8ee', ApiPaths.g8ee.investigation(investigationId), {
            method: 'GET',
            vsoContext
        });
    }

    /**
     * Delete case and all related data via g8ee internal API
     * Deletes case, investigations, and memories
     * 
     * ENFORCEMENT: vsoContext is REQUIRED and must contain web_session_id
     * 
     * @param {string} caseId - Case ID to delete
     * @param {Object} vsoContext - REQUIRED VSOHttpContext with session/user info
     * @throws {Error} If vsoContext is missing or invalid
     */
    async deleteCase(caseId, vsoContext) {
        if (!vsoContext) {
            throw new Error('ENFORCEMENT VIOLATION: vsoContext is REQUIRED for g8ee calls');
        }
        logger.info('[HTTP-INTERNAL] Deleting case', {
            caseId,
            hasContext: !!vsoContext
        });

        return this.request('g8ee', ApiPaths.g8ee.case(caseId), {
            method: 'DELETE',
            vsoContext
        });
    }

    /**
     * Stop active AI processing for an investigation via g8ee internal API
     * Cancels the asyncio task processing the AI response
     * 
     * ENFORCEMENT: vsoContext is REQUIRED and must contain web_session_id
     * 
     * @param {Object} stopData - Stop request data (investigation_id, reason)
     * @param {Object} vsoContext - REQUIRED VSOHttpContext with session/user info
     * @throws {Error} If vsoContext is missing or invalid
     */
    async stopAIProcessing(stopData, vsoContext) {
        if (!vsoContext) {
            throw new Error('ENFORCEMENT VIOLATION: vsoContext is REQUIRED for g8ee calls');
        }
        logger.info('[HTTP-INTERNAL] Stopping AI processing', {
            investigationId: stopData.investigation_id,
            reason: stopData.reason || 'User requested stop',
            hasContext: !!vsoContext
        });

        return this.request('g8ee', ApiPaths.g8ee.chatStop(), {
            method: 'POST',
            body: new StopAIRequest(stopData).forWire(),
            vsoContext
        });
    }

    /**
     * List available MCP tools via G8EE.
     *
     * @param {Object} vsoContext - REQUIRED VSOHttpContext with session/user info
     * @returns {Object} JSON response { tools: [...] }
     */
    async mcpToolsList(vsoContext) {
        if (!vsoContext) {
            throw new Error('ENFORCEMENT VIOLATION: vsoContext is REQUIRED for g8ee calls');
        }

        logger.info('[HTTP-INTERNAL] MCP tools/list request', {
            userId: vsoContext.user_id,
        });

        return this.request('g8ee', ApiPaths.g8ee.mcpToolsList(), {
            method: 'POST',
            body: {},
            vsoContext,
        });
    }

    /**
     * Execute an MCP tool call via G8EE.
     *
     * @param {Object} toolCallData - { tool_name, arguments, request_id }
     * @param {Object} vsoContext - REQUIRED VSOHttpContext with session/user info
     * @returns {Object} MCPToolCallResponse { jsonrpc, id, result|error }
     */
    async mcpToolsCall(toolCallData, vsoContext) {
        if (!vsoContext) {
            throw new Error('ENFORCEMENT VIOLATION: vsoContext is REQUIRED for g8ee calls');
        }

        logger.info('[HTTP-INTERNAL] MCP tools/call request', {
            toolName: toolCallData.tool_name,
            requestId: toolCallData.request_id,
        });

        return this.request('g8ee', ApiPaths.g8ee.mcpToolsCall(), {
            method: 'POST',
            body: toolCallData,
            vsoContext,
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
