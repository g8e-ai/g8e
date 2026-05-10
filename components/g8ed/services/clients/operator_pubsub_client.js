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
 * OperatorPubSubClient — operator Pub/Sub WebSocket client.
 * 
 * Handles pub/sub messaging over WebSocket to operator ($G8E_INTERNAL_PUBSUB_URL/ws/pubsub).
 * Used by Auth Service (auth response channels) and any service needing real-time messaging.
 * 
 * Architecture (from docs/architecture/cases-and-chat-data-flows.md):
 *   WebSocket is used for pub/sub ONLY — command dispatch, results, heartbeats.
 *   DB operations are never routed over WebSocket.
 */

import { readFileSync, existsSync } from 'fs';
import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const { PubSubMessage, PubSubEvent } = require('../../shared/proto/pubsub_pb.cjs');

import WebSocket from 'ws';
import { logger } from '../../utils/logger.js';
import { PUBSUB_RECONNECT_DELAY_MS, OPERATOR_PUBSUB_PATH } from '../../constants/http_client.js';
import { PubSubAction, PubSubMessageType } from '../../constants/channels.js';
import { HTTP_INTERNAL_AUTH_HEADER } from '../../constants/headers.js';

class OperatorPubSubClient {
    /**
     * @param {object} config
     * @param {string} config.pubsubUrl - WSS URL for pub/sub
     * @param {string} [config.caCertPath] - Path to CA certificate for WSS connections
     * @param {string} [config.internalAuthToken] - Shared secret for operator authentication
     */
    constructor({ pubsubUrl, caCertPath, internalAuthToken = null } = {}) {
        if (!pubsubUrl) {
            throw new Error('OperatorPubSubClient: pubsubUrl is required');
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
        const wsUrl = new URL(this.pubsubUrl + OPERATOR_PUBSUB_PATH);
        if (!wsUrl.protocol.startsWith('wss')) {
            logger.warn(`[OPERATOR-CLIENT] Protocol override: forcing WSS for ${this.pubsubUrl}`);
            wsUrl.protocol = 'wss:';
        }
        
        if (this.internalAuthToken) {
            wsUrl.searchParams.set('token', this.internalAuthToken);
        }

        const wsOptions = this._buildTLSOptions();
        if (this.internalAuthToken) {
            wsOptions.headers = { ...wsOptions.headers, [HTTP_INTERNAL_AUTH_HEADER]: this.internalAuthToken };
        }

        this._ws = new WebSocket(wsUrl.toString(), wsOptions);

        this._ws.on('open', () => {
            logger.info('[OPERATOR-CLIENT] Pub/sub WebSocket connected');
        });

        this._ws.on('message', (raw) => {
            try {
                const event = PubSubEvent.deserializeBinary(raw);
                const type = event.getType();
                const channel = event.getChannel();
                const pattern = event.getPattern();
                const data = event.getData();

                if (type === PubSubMessageType.MESSAGE) {
                    for (const handler of this._messageHandlers) {
                        handler(channel, data);
                    }
                } else if (type === PubSubMessageType.PMESSAGE) {
                    for (const handler of this._pmessageHandlers) {
                        handler(pattern, channel, data);
                    }
                }
            } catch (error) {
                logger.error(`[OPERATOR-CLIENT] Failed to parse inbound protobuf message: ${error.message}`);
            }
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
                this._ws.send(msg.serializeBinary());
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
        const msg = new PubSubMessage();
        msg.setAction(PubSubAction.SUBSCRIBE);
        msg.setChannel(channel);
        this._wsSend(msg);
    }

    psubscribe(pattern) {
        const msg = new PubSubMessage();
        msg.setAction(PubSubAction.PSUBSCRIBE);
        msg.setChannel(pattern);
        this._wsSend(msg);
    }

    unsubscribe(channel) {
        const msg = new PubSubMessage();
        msg.setAction(PubSubAction.UNSUBSCRIBE);
        msg.setChannel(channel);
        this._wsSend(msg);
    }

    async publish(channel, data) {
        if (!Buffer.isBuffer(data) && !(data instanceof Uint8Array)) {
            throw new Error(`OperatorPubSubClient.publish: data must be a Buffer or Uint8Array, got ${typeof data}`);
        }
        try {
            const msg = new PubSubMessage();
            msg.setAction(PubSubAction.PUBLISH);
            msg.setChannel(channel);
            msg.setData(data);
            this._wsSend(msg);
            return 1;
        } catch (error) {
            logger.error(`[OPERATOR-PUBSUB] publish failed: ${error.message}`);
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

    removeListener(event, handler) {
        if (event === 'message') {
            const index = this._messageHandlers.indexOf(handler);
            if (index !== -1) {
                this._messageHandlers.splice(index, 1);
            }
        } else if (event === 'pmessage') {
            const index = this._pmessageHandlers.indexOf(handler);
            if (index !== -1) {
                this._pmessageHandlers.splice(index, 1);
            }
        }
    }

    /**
     * Create an independent pub/sub connection for isolated subscriptions.
     */
    duplicate() {
        return new OperatorPubSubClient({
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

        logger.info('[OPERATOR-PUBSUB] Client terminated');
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

export { OperatorPubSubClient };
