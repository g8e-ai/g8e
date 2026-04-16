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
 * SSE Service - SSE Event Manager
 * 
 * Architecture:
 * 1. g8ee sends events to g8ed via HTTP (NOT pub/sub)
 * 2. g8ed delivers directly to local SSE connections
 * 3. Fire-and-forget: if client is disconnected, events are missed (client fetches fresh state on reconnect)
 */

import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { G8eBaseModel, now } from '../../models/base.js';
import { SSE_FRAME_TERMINATOR } from '../../constants/service_config.js';
import { writeSSEFrame } from '../../utils/sse.js';
import { LLMConfigEvent, LLMConfigData, InvestigationListEvent, InvestigationListData } from '../../models/sse_models.js';
import { G8eHttpContext } from '../../models/request_models.js';
import { EventType } from '../../constants/events.js';
import { Collections } from '../../constants/collections.js';
import { PROVIDER_MODELS } from '../../constants/ai.js';

class SSEService {
    /**
     * @param {Object} options
     * @param {Object} options.settingsService - SettingsService instance
     * @param {Object} options.internalHttpClient - InternalHttpClient instance
     * @param {Object} options.boundSessionsService - BoundSessionsService instance
     * @param {Object} options.investigationService - InvestigationService instance
     */
    constructor({ settingsService, internalHttpClient, boundSessionsService, investigationService } = {}) {
        this.localConnections = new Map();
        this.connectionsPerSession = new Map();
        this._settingsService = settingsService;
        this._internalHttpClient = internalHttpClient;
        this._boundSessionsService = boundSessionsService;
        this._investigationService = investigationService;
        this.healthy = true;
    }

    /**
     * Sets dependencies after instantiation if not provided in constructor (for circular bypass in init)
     */
    setDependencies({ settingsService, internalHttpClient, boundSessionsService, investigationService }) {
        this._settingsService = settingsService;
        this._internalHttpClient = internalHttpClient;
        this._boundSessionsService = boundSessionsService;
        this._investigationService = investigationService;
    }

    async waitForReady() {
        this.healthy = true;
        logger.info('[SSE-SERVICE] SSE service ready (local)');
    }

    /**
     * Register a new SSE connection (local to this instance)
     * Returns a unique connectionId that must be passed to unregisterConnection
     * to prevent stale connection cleanup from removing a newer active connection.
     */
    async registerConnection(webSessionId, response, metadata = {}) {
        // Generate unique connection ID to prevent stale cleanup race condition
        const connectionId = `${webSessionId}_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;

        // Close previous connection for this session if it exists
        const existing = this.localConnections.get(webSessionId);
        const isReplacing = existing && existing.response !== response;
        if (isReplacing) {
            logger.info('[SSE-SERVICE] Replacing existing connection for session', {
                webSessionId: redactWebSessionId(webSessionId),
                oldConnectionId: existing.connectionId,
                newConnectionId: connectionId
            });
        }

        // Store response object with its unique connection ID
        this.localConnections.set(webSessionId, { response, connectionId });

        // Track per-session connection count (local)
        const currentCount = this.connectionsPerSession.get(webSessionId) || 0;
        const newCount = isReplacing ? currentCount : currentCount + 1;
        this.connectionsPerSession.set(webSessionId, newCount);

        logger.info('[SSE-SERVICE] Connection registered', {
            webSessionId: redactWebSessionId(webSessionId),
            connectionId,
            localConnections: this.localConnections.size,
            sessionConnections: newCount,
            ...metadata
        });

        return {
            connectionId,
            localConnections: this.localConnections.size,
            sessionConnections: newCount
        };
    }

    /**
     * Unregister SSE connection when client disconnects.
     * Only removes the connection if the connectionId matches the currently registered one.
     * This prevents a stale connection's cleanup from removing a newer active connection.
     *
     * @param {string} webSessionId - WebSession ID
     * @param {string} connectionId - Unique connection ID returned by registerConnection
     */
    unregisterConnection(webSessionId, connectionId) {
        const existing = this.localConnections.get(webSessionId);

        // Only remove if this is the same connection that's currently registered
        if (existing && existing.connectionId === connectionId) {
            this.localConnections.delete(webSessionId);
            logger.info('[SSE-SERVICE] Connection unregistered', {
                webSessionId: redactWebSessionId(webSessionId),
                connectionId,
                localConnections: this.localConnections.size
            });
        } else {
            logger.info('[SSE-SERVICE] Skipping unregister - connection already replaced by newer connection', {
                webSessionId: redactWebSessionId(webSessionId),
                staleConnectionId: connectionId,
                activeConnectionId: existing?.connectionId || 'none'
            });
        }

        // Decrement per-session connection count regardless
        const count = this.connectionsPerSession.get(webSessionId) || 0;
        if (count <= 1) {
            this.connectionsPerSession.delete(webSessionId);
        } else {
            this.connectionsPerSession.set(webSessionId, count - 1);
        }
    }

    /**
     * Deliver event to local SSE connection.
     * Events come from g8ee via HTTP, NOT g8es KV pub/sub.
     * Fire-and-forget: returns true even when no connection is present.
     *
     * @param {string} webSessionId - WebSession ID
     * @param {G8eBaseModel} eventData - Typed SSE model instance — serialized via .forWire() at this boundary
     * @param {Function} deliveryCallback - Optional callback invoked with delivery status: {delivered, webSessionId, eventType}
     */
    async publishEvent(webSessionId, eventData, deliveryCallback = null) {
        if (!(eventData instanceof G8eBaseModel)) {
            throw new Error(`SSEService.publishEvent requires a G8eBaseModel instance, got ${typeof eventData}`);
        }
        const wire = eventData.forWire();
        const delivered = await this.sendToLocal(webSessionId, wire);
        if (!delivered) {
            logger.warn('[SSE-SERVICE] No active SSE connection for session', {
                webSessionId: redactWebSessionId(webSessionId),
                eventType: wire.type
            });
        }
        if (deliveryCallback && typeof deliveryCallback === 'function') {
            try {
                deliveryCallback({
                    delivered,
                    webSessionId,
                    eventType: wire.type
                });
            } catch (err) {
                logger.error('[SSE-SERVICE] Delivery status callback threw error', {
                    webSessionId: redactWebSessionId(webSessionId),
                    eventType: wire.type,
                    error: err.message
                });
            }
        }
        return true;
    }

    /**
     * Broadcast event to all sessions (rare use case)
     *
     * @param {G8eBaseModel} eventData - Typed SSE model instance — serialized via .forWire() at this boundary
     */
    async broadcastEvent(eventData) {
        if (!(eventData instanceof G8eBaseModel)) {
            throw new Error(`SSEService.broadcastEvent requires a G8eBaseModel instance, got ${typeof eventData}`);
        }
        try {
            const broadcastId = `broadcast_${Date.now()}`;
            const wire = eventData.forWire();
            logger.info('[SSE-SERVICE] Broadcasting event', {
                broadcastId,
                eventType: wire.type
            });

            const webSessionIds = Array.from(this.localConnections.keys());

            let successCount = 0;
            for (const webSessionId of webSessionIds) {
                if (await this.sendToLocal(webSessionId, wire)) {
                    successCount++;
                }
            }

            logger.info('[SSE-SERVICE] Broadcast complete', {
                broadcastId,
                successCount,
                totalSessions: webSessionIds.length
            });

            return successCount;
        } catch (error) {
            logger.error('[SSE-SERVICE] Broadcast failed', {
                error: error.message
            });
            return 0;
        }
    }

    /**
     * Get connection count for a specific session (local to this instance)
     */
    getSessionConnectionCount(webSessionId) {
        return this.connectionsPerSession.get(webSessionId) || 0;
    }

    /**
     * Check if connection exists locally
     */
    hasLocalConnection(webSessionId) {
        const entry = this.localConnections.get(webSessionId);
        const connection = entry?.response;
        return connection && connection.writable && !connection.destroyed;
    }

    /**
     * Internal: send a pre-serialized wire payload to the local connection.
     * Only called from publishEvent and broadcastEvent after .forWire() has been applied.
     * Returns false if no active connection exists or delivery fails.
     *
     * @param {string} webSessionId - WebSession ID
     * @param {object} wire - Plain object produced by .forWire()
     */
    async sendToLocal(webSessionId, wire) {
        try {
            const entry = this.localConnections.get(webSessionId);
            const connection = entry?.response;

            if (!connection || !connection.writable || connection.destroyed) {
                return false;
            }

            const message = JSON.stringify(wire);
            connection.write(`data: ${message}${SSE_FRAME_TERMINATOR}`);
            if (typeof connection.flush === 'function') connection.flush();

            logger.info('[SSE-SERVICE] Event delivered to local connection', {
                webSessionId: redactWebSessionId(webSessionId),
                eventType: wire.type
            });

            return true;
        } catch (error) {
            logger.error('[SSE-SERVICE] Failed to send to local connection', {
                webSessionId: redactWebSessionId(webSessionId),
                error: error.message
            });
            return false;
        }
    }

    /**
     * Get service statistics
     */
    getStats() {
        return {
            localConnections: this.localConnections.size,
            uniqueSessions: this.connectionsPerSession.size,
            healthy: this.healthy
        };
    }

    /**
     * Pushes initial state to a newly established connection.
     * 1. LLM Configuration
     * 2. Investigation List
     */
    async pushInitialState(userId, webSessionId, organizationId = null) {
        if (!this._settingsService || !this._internalHttpClient || !this._boundSessionsService || !this._investigationService) {
            logger.warn('[SSE-SERVICE] Dependencies not set, skipping initial state push', {
                webSessionId: redactWebSessionId(webSessionId)
            });
            return;
        }

        await Promise.all([
            this._pushLLMConfig(userId, webSessionId),
            this._pushInvestigationList(userId, webSessionId, organizationId)
        ]);
    }

    /**
     * @private
     */
    async _pushLLMConfig(userId, webSessionId) {
        try {
            const platformSettings = await this._settingsService.getPlatformSettings();
            const userSettings = await this._settingsService.getUserSettings(userId);

            const provider = userSettings.llm_primary_provider ?? platformSettings.llm_primary_provider ?? '';
            const assistantProvider = userSettings.llm_assistant_provider ?? platformSettings.llm_assistant_provider ?? '';
            const liteProvider = userSettings.llm_lite_provider ?? platformSettings.llm_lite_provider ?? '';
            const currentPrimary = userSettings.llm_model ?? platformSettings.llm_model ?? '';
            const currentAssistant = userSettings.llm_assistant_model ?? platformSettings.llm_assistant_model ?? '';
            const currentLite = userSettings.llm_lite_model ?? platformSettings.llm_lite_model ?? '';

            const configuredProviders = SSEService._getConfiguredProviders(userSettings, platformSettings);
            const providerModels = SSEService._buildProviderModels(configuredProviders);

            const primaryModels = SSEService._getModelOptionsForProvider(provider, 'llm_model');
            const assistantModels = SSEService._getModelOptionsForProvider(assistantProvider, 'llm_assistant_model');
            const liteModels = SSEService._getModelOptionsForProvider(liteProvider, 'llm_lite_model');

            await this.publishEvent(webSessionId, LLMConfigEvent.parse({
                type: EventType.LLM_CONFIG_RECEIVED,
                data: LLMConfigData.parse({
                    provider,
                    assistant_provider: assistantProvider,
                    lite_provider: liteProvider,
                    default_primary_model: currentPrimary,
                    default_assistant_model: currentAssistant,
                    default_lite_model: currentLite,
                    primary_models: primaryModels,
                    assistant_models: assistantModels,
                    lite_models: liteModels,
                    provider_models: providerModels,
                    timestamp: now()
                })
            }));

            logger.info('[SSE-SERVICE] Pushed LLM config', {
                webSessionId: redactWebSessionId(webSessionId),
                provider,
                configuredProviders,
                primaryModelCount: primaryModels.length,
                assistantModelCount: assistantModels.length,
                liteModelCount: liteModels.length
            });
        } catch (error) {
            logger.error('[SSE-SERVICE] Failed to push LLM config', {
                webSessionId: redactWebSessionId(webSessionId),
                error: error.message
            });
        }
    }

    static _getModelOptionsForProvider(provider, settingKey) {
        const config = PROVIDER_MODELS[provider];
        if (!config) return [];
        
        if (settingKey === 'llm_model') {
            return config.primary || [];
        } else if (settingKey === 'llm_assistant_model') {
            return config.assistant || [];
        } else if (settingKey === 'llm_lite_model') {
            return config.lite || [];
        }
        
        return [];
    }

    static _PROVIDER_CREDENTIAL_KEYS = Object.freeze({
        gemini:    'gemini_api_key',
        anthropic: 'anthropic_api_key',
        openai:    'openai_api_key',
        ollama:    'ollama_endpoint',
    });

    static _PROVIDER_LABELS = Object.freeze({
        gemini:    'Gemini',
        anthropic: 'Anthropic',
        openai:    'OpenAI',
        ollama:    'Ollama',
    });

    static _getConfiguredProviders(userSettings, platformSettings) {
        const providers = [];
        for (const [provider, credKey] of Object.entries(SSEService._PROVIDER_CREDENTIAL_KEYS)) {
            const value = userSettings[credKey] ?? platformSettings[credKey] ?? '';
            if (value && typeof value === 'string' && value.trim() !== '') {
                providers.push(provider);
            }
        }
        return providers;
    }

    static _buildProviderModels(configuredProviders) {
        const result = {};
        for (const provider of configuredProviders) {
            const primaryOpts = SSEService._getModelOptionsForProvider(provider, 'llm_model');
            const assistantOpts = SSEService._getModelOptionsForProvider(provider, 'llm_assistant_model');
            const liteOpts = SSEService._getModelOptionsForProvider(provider, 'llm_lite_model');
            const allOpts = [...new Set([...primaryOpts, ...assistantOpts, ...liteOpts])];
            result[provider] = {
                label: SSEService._PROVIDER_LABELS[provider] || provider,
                all: allOpts,
                primary: primaryOpts,
                assistant: assistantOpts,
                lite: liteOpts,
            };
        }
        return result;
    }

    /**
     * @private
     */
    async _pushInvestigationList(userId, webSessionId, organizationId) {
        try {
            const investigations = await this._investigationService.getInvestigationsByUserId(userId);

            await this.publishEvent(webSessionId, InvestigationListEvent.parse({
                type: EventType.INVESTIGATION_LIST_COMPLETED,
                data: InvestigationListData.parse({
                    investigations: Array.isArray(investigations) ? investigations : [],
                    count: Array.isArray(investigations) ? investigations.length : 0,
                    timestamp: now()
                })
            }));

            logger.info('[SSE-SERVICE] Pushed investigation list', {
                webSessionId: redactWebSessionId(webSessionId),
                userId,
                count: Array.isArray(investigations) ? investigations.length : 0
            });
        } catch (error) {
            logger.error('[SSE-SERVICE] Failed to push investigation list', {
                webSessionId: redactWebSessionId(webSessionId),
                error: error.message
            });
        }
    }

    /**
     * Close all connections and cleanup
     */
    async close() {
        logger.info('[SSE-SERVICE] Closing SSE service');
        this.healthy = false;
        this.localConnections.clear();
        this.connectionsPerSession.clear();
        logger.info('[SSE-SERVICE] SSE service closed');
    }
}

export { SSEService, writeSSEFrame };
