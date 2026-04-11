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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { EventEmitter } from 'events';
import { VSODBPubSubClient } from '@vsod/services/clients/vsodb_pubsub_client.js';
import { PubSubAction, PubSubMessageType } from '@vsod/constants/channels.js';

// Simple global tracker for the latest mock instance
let latestWsInstance = null;

// Mock WS using a global variable that is accessible to both the mock and the test
vi.mock('ws', () => {
    return {
        default: class MockWS extends EventEmitter {
            static OPEN = 1;
            static CLOSED = 3;
            static CONNECTING = 0;
            static CLOSING = 2;
            constructor(url, options) {
                super();
                this.url = url;
                this.options = options;
                this.readyState = 0; // CONNECTING
                this.send = vi.fn();
                this.close = vi.fn();
                this.terminate = vi.fn();
                global.__LATEST_WS_INSTANCE__ = this;
            }
        },
        OPEN: 1,
        CLOSED: 3,
        CONNECTING: 0,
        CLOSING: 2
    };
});

describe('VSODBPubSubClient', () => {
    const pubsubUrl = 'wss://vsodb:9001';
    const internalAuthToken = 'test-token';
    let client;

    beforeEach(() => {
        global.__LATEST_WS_INSTANCE__ = null;
        client = new VSODBPubSubClient({ pubsubUrl, internalAuthToken });
    });

    afterEach(async () => {
        await client.terminate();
        vi.clearAllMocks();
    });

    describe('constructor', () => {
        it('should throw if pubsubUrl is missing', () => {
            expect(() => new VSODBPubSubClient({})).toThrow('VSODBPubSubClient: pubsubUrl is required');
        });

        it('should strip trailing slash from pubsubUrl', () => {
            const clientWithSlash = new VSODBPubSubClient({ pubsubUrl: 'wss://vsodb:9001/' });
            expect(clientWithSlash.pubsubUrl).toBe('wss://vsodb:9001');
        });
    });

    describe('WebSocket management', () => {
        it('should connect with token in query params and headers', () => {
            client.subscribe('test-channel');
            
            const wsInstance = global.__LATEST_WS_INSTANCE__;
            const url = wsInstance.url;
            const options = wsInstance.options;

            expect(url).toContain('token=test-token');
            expect(options.headers['X-Internal-Auth']).toBe('test-token');
        });

        it('should handle incoming messages and pmessages', () => {
            const messageHandler = vi.fn();
            const pmessageHandler = vi.fn();
            client.on('message', messageHandler);
            client.on('pmessage', pmessageHandler);

            client.subscribe('test-channel');
            const wsInstance = global.__LATEST_WS_INSTANCE__;

            // Simulate message event - data must be a string for PubSubInboundMessage
            const msgPayload = {
                type: PubSubMessageType.MESSAGE,
                channel: 'test-channel',
                data: JSON.stringify({ foo: 'bar' })
            };
            wsInstance.emit('message', JSON.stringify(msgPayload));
            expect(messageHandler).toHaveBeenCalledWith('test-channel', JSON.stringify({ foo: 'bar' }));

            // Simulate pmessage event - data must be a string for PubSubInboundPMessage
            const pmsgPayload = {
                type: PubSubMessageType.PMESSAGE,
                pattern: 'test-*',
                channel: 'test-channel',
                data: JSON.stringify({ baz: 'qux' })
            };
            wsInstance.emit('message', JSON.stringify(pmsgPayload));
            expect(pmessageHandler).toHaveBeenCalledWith('test-*', 'test-channel', JSON.stringify({ baz: 'qux' }));
        });
    });

    describe('Pub/Sub operations', () => {
        it('subscribe() should send subscribe action', async () => {
            client.subscribe('my-channel');
            const wsInstance = global.__LATEST_WS_INSTANCE__;
            
            wsInstance.readyState = 1; // OPEN
            wsInstance.emit('open');

            expect(wsInstance.send).toHaveBeenCalledWith(expect.stringContaining(PubSubAction.SUBSCRIBE));
            expect(wsInstance.send).toHaveBeenCalledWith(expect.stringContaining('my-channel'));
        });

        it('publish() should send publish action with plain object', async () => {
            const data = { event: 'test' };
            const publishPromise = client.publish('my-channel', data);
            
            const wsInstance = global.__LATEST_WS_INSTANCE__;
            wsInstance.readyState = 1; // OPEN
            wsInstance.emit('open');

            const result = await publishPromise;
            expect(result).toBe(1);
            expect(wsInstance.send).toHaveBeenCalledWith(expect.stringContaining(PubSubAction.PUBLISH));
            expect(wsInstance.send).toHaveBeenCalledWith(expect.stringContaining(JSON.stringify(data)));
        });

        it('publish() should throw if data is not a plain object', async () => {
            await expect(client.publish('chan', 'string')).rejects.toThrow(/must be a plain object/);
            await expect(client.publish('chan', null)).rejects.toThrow(/must be a plain object/);
            await expect(client.publish('chan', [])).rejects.toThrow(/must be a plain object/);
        });
    });

    describe('Lifecycle', () => {
        it('terminate() should close socket and prevent reconnect', async () => {
            client.subscribe('test');
            const wsInstance = global.__LATEST_WS_INSTANCE__;
            
            await client.terminate();
            
            expect(wsInstance.close).toHaveBeenCalled();
            expect(client.isTerminated()).toBe(true);
        });

        it('duplicate() should create fresh client with same config', () => {
            const dup = client.duplicate();
            expect(dup).toBeInstanceOf(VSODBPubSubClient);
            expect(dup.pubsubUrl).toBe(client.pubsubUrl);
            expect(dup.internalAuthToken).toBe(client.internalAuthToken);
        });
    });
});
