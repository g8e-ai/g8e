// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8edBPubSubClient — g8edB Pub/Sub WebSocket client.
 * 
 * Handles pub/sub messaging over WebSocket to g8edB ($DROPOPS_INTERNAL_PUBSUB_URL/ws/pubsub).
 * Used by Auth Service (auth response channels) and any service needing real-time messaging.
 * 
 * Architecture (from docs/architecture/cases-and-chat-data-flows.md):
 *   WebSocket is used for pub/sub ONLY — command dispatch, results, heartbeats.
 *   DB operations are never routed over WebSocket.
 */

import { readFileSync, existsSync } from 'fs';
import WebSocket from 'ws';
import { logger } from '../../utils/logger.js';
import { PUBSUB_RECONNECT_DELAY_MS, g8edB_PUBSUB_PATH } from '../../constants/http_client.js';
import { PubSubAction, PubSubMessageType } from '../../constants/channels.js';
import { HTTP_INTERNAL_AUTH_HEADER } from '../../constants/headers.js';
import { PubSubSubscribeMessage, PubSubPublishMessage, PubSubInboundMessage, PubSubInboundPMessage } from '../../models/pubsub_models.js';

class g8edBPubSubClient {
    /**
     * @param {object} config
     * @param {string} config.pubsubUrl - WSS URL for pub/sub
     * @param {string} [config.caCertPath] - Path to CA certificate for WSS connections
     * @param {string} [config.internalAuthToken] - Shared secret for g8edB authentication
     */
    constructor({ pubsubUrl, caCertPath, internalAuthToken = null } = {}) {
        if (!pubsubUrl) {
            throw new Error('g8edBPubSubClient: pubsubUrl is required');
        }
        this.pubsubUrl = pubsubUrl.replace(/\/$/, '');
        this.caCertPath = caCertPath || null;
        this.internalAuthToken = internalAuthToken;
        this._terminated = false;
        this._ws = null;
        this._wsReconnectTimer = null;
        this._messageHandlers = [];
        this._pmessageHandlers = [];
    }

    _buildTLSOptions() {
        if (this.caCertPath && existsSync(this.caCertPath)) {
            return { ca: readFileSync(this.caCertPath) };
        }
        return {};
    }

    // =========================================================================
    // WebSocket management
    // =========================================================================

    _connectWS() {
        const wsUrl = new URL(this.pubsubUrl + g8edB_PUBSUB_PATH);
        if (!wsUrl.protocol.startsWith('wss')) {
            logger.warn(`[g8edB-CLIENT] Protocol override: forcing WSS for ${this.pubsubUrl}`);
            wsUrl.protocol = 'wss:';
        }
        
        const wsOptions = this._buildTLSOptions();
        if (this.internalAuthToken) {
            wsOptions.headers = { ...wsOptions.headers, [HTTP_INTERNAL_AUTH_HEADER]: this.internalAuthToken };
        }

        this._ws = new WebSocket(wsUrl.toString(), wsOptions);

        this._ws.on('open', () => {
            logger.info('[g8edB-CLIENT] Pub/sub WebSocket connected');
        });

        this._ws.on('message', (raw) => {
            try {
                const parsed = JSON.parse(raw.toString());
                if (parsed.type === PubSubMessageType.MESSAGE) {
                    const msg = PubSubInboundMessage.parse(parsed);
                    for (const handler of this._messageHandlers) {
                        handler(msg.channel, msg.data);
                    }
                } else if (parsed.type === PubSubMessageType.PMESSAGE) {
                    const msg = PubSubInboundPMessage.parse(parsed);
                    for (const handler of this._pmessageHandlers) {
                        handler(msg.pattern, msg.channel, msg.data);
                    }
                }
            } catch {}
        });

        this._ws.on('close', () => {
            this._ws = null;
            if (!this._terminated) {
                this._wsReconnectTimer = setTimeout(() => this._connectWS(), PUBSUB_RECONNECT_DELAY_MS);
            }
        });

        this._ws.on('error', () => {});
    }

    _wsSend(msg) {
        if (!this._ws || this._ws.readyState !== WebSocket.OPEN) {
            this._connectWS();
        }
        const send = () => {
            if (this._ws && this._ws.readyState === WebSocket.OPEN) {
                this._ws.send(JSON.stringify(msg.forWire()));
            }
        };
        if (this._ws.readyState === WebSocket.OPEN) {
            send();
        } else {
            this._ws.once('open', send);
        }
    }

    // =========================================================================
    // Pub/Sub operations
    // =========================================================================

    subscribe(channel) {
        this._wsSend(new PubSubSubscribeMessage({ action: PubSubAction.SUBSCRIBE, channel }));
    }

    psubscribe(pattern) {
        this._wsSend(new PubSubSubscribeMessage({ action: PubSubAction.PSUBSCRIBE, channel: pattern }));
    }

    unsubscribe(channel) {
        this._wsSend(new PubSubSubscribeMessage({ action: PubSubAction.UNSUBSCRIBE, channel }));
    }

    async publish(channel, data) {
        if (data === null || typeof data !== 'object' || Array.isArray(data)) {
            throw new Error(`g8edBPubSubClient.publish: data must be a plain object, got ${typeof data}`);
        }
        try {
            this._wsSend(new PubSubPublishMessage({ action: PubSubAction.PUBLISH, channel, data }));
            return 1;
        } catch (error) {
            logger.error(`[g8edB-PUBSUB] publish failed: ${error.message}`);
            return 0;
        }
    }

    on(event, handler) {
        if (event === 'message') {
            this._messageHandlers.push(handler);
        } else if (event === 'pmessage') {
            this._pmessageHandlers.push(handler);
        }
    }

    /**
     * Create an independent pub/sub connection for isolated subscriptions.
     */
    duplicate() {
        return new g8edBPubSubClient({
            pubsubUrl: this.pubsubUrl,
            caCertPath: this.caCertPath,
            internalAuthToken: this.internalAuthToken,
        });
    }

    // =========================================================================
    // Lifecycle
    // =========================================================================

    async terminate() {
        if (this._terminated) return;
        this._terminated = true;

        if (this._wsReconnectTimer) clearTimeout(this._wsReconnectTimer);
        if (this._ws) {
            this._ws.close();
            this._ws = null;
        }

        logger.info('[g8edB-PUBSUB] Client terminated');
    }

    async disconnect() {
        return this.terminate();
    }

    async quit() {
        return this.terminate();
    }

    isTerminated() {
        return this._terminated;
    }
}

export { g8edBPubSubClient };
