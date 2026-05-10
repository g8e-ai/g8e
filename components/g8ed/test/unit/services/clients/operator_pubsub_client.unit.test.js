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
import { createRequire } from 'module';
import path from 'path';
import { resolveProjectRoot } from '@g8ed/utils/path.js';
import { OperatorPubSubClient } from '@g8ed/services/clients/operator_pubsub_client.js';
import { PubSubAction, PubSubMessageType } from '@g8ed/constants/channels.js';

const require = createRequire(import.meta.url);
const { PubSubMessage, PubSubEvent } = require(path.join(resolveProjectRoot(), 'components/g8ed/shared/proto/pubsub_pb.cjs'));

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

describe('OperatorPubSubClient', () => {
    const pubsubUrl = 'wss://localhost:9001';
    const internalAuthToken = 'test-token';
    let client;

    beforeEach(() => {
        global.__LATEST_WS_INSTANCE__ = null;
        client = new OperatorPubSubClient({ pubsubUrl, internalAuthToken });
    });

    afterEach(async () => {
        await client.terminate();
        vi.clearAllMocks();
    });

    describe('constructor', () => {
        it('should throw if pubsubUrl is missing', () => {
            expect(() => new OperatorPubSubClient({})).toThrow('OperatorPubSubClient: pubsubUrl is required');
        });

        it('should strip trailing slash from pubsubUrl', () => {
            const clientWithSlash = new OperatorPubSubClient({ pubsubUrl: 'wss://localhost:9001/' });
            expect(clientWithSlash.pubsubUrl).toBe('wss://localhost:9001');
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

            // Simulate message event
            const event = new PubSubEvent();
            event.setType(PubSubMessageType.MESSAGE);
            event.setChannel('test-channel');
            const data = Buffer.from(JSON.stringify({ foo: 'bar' }));
            event.setData(data);

            wsInstance.emit('message', event.serializeBinary());
            expect(messageHandler).toHaveBeenCalledWith('test-channel', expect.any(Uint8Array));
            const receivedData = messageHandler.mock.calls[0][1];
            expect(Buffer.from(receivedData)).toEqual(data);

            // Simulate pmessage event
            const pEvent = new PubSubEvent();
            pEvent.setType(PubSubMessageType.PMESSAGE);
            pEvent.setPattern('test-*');
            pEvent.setChannel('test-channel');
            const pData = Buffer.from(JSON.stringify({ baz: 'qux' }));
            pEvent.setData(pData);

            wsInstance.emit('message', pEvent.serializeBinary());
            expect(pmessageHandler).toHaveBeenCalledWith('test-*', 'test-channel', expect.any(Uint8Array));
            const receivedPData = pmessageHandler.mock.calls[0][2];
            expect(Buffer.from(receivedPData)).toEqual(pData);
        });
    });

    describe('Pub/Sub operations', () => {
        it('subscribe() should send subscribe action', async () => {
            client.subscribe('my-channel');
            const wsInstance = global.__LATEST_WS_INSTANCE__;
            
            wsInstance.readyState = 1; // OPEN
            wsInstance.emit('open');

            expect(wsInstance.send).toHaveBeenCalled();
            const sentBinary = wsInstance.send.mock.calls[0][0];
            const sentMsg = PubSubMessage.deserializeBinary(sentBinary);
            expect(sentMsg.getAction()).toBe(PubSubAction.SUBSCRIBE);
            expect(sentMsg.getChannel()).toBe('my-channel');
        });

        it('publish() should send publish action with buffer', async () => {
            const data = Buffer.from(JSON.stringify({ event: 'test' }));
            const publishPromise = client.publish('my-channel', data);
            
            const wsInstance = global.__LATEST_WS_INSTANCE__;
            wsInstance.readyState = 1; // OPEN
            wsInstance.emit('open');

            const result = await publishPromise;
            expect(result).toBe(1);
            expect(wsInstance.send).toHaveBeenCalled();
            const sentBinary = wsInstance.send.mock.calls[0][0];
            const sentMsg = PubSubMessage.deserializeBinary(sentBinary);
            expect(sentMsg.getAction()).toBe(PubSubAction.PUBLISH);
            expect(sentMsg.getChannel()).toBe('my-channel');
            expect(Buffer.from(sentMsg.getData())).toEqual(data);
        });

        it('publish() should throw if data is not a Buffer or Uint8Array', async () => {
            await expect(client.publish('chan', { foo: 'bar' })).rejects.toThrow(/must be a Buffer or Uint8Array/);
            await expect(client.publish('chan', 'string')).rejects.toThrow(/must be a Buffer or Uint8Array/);
            await expect(client.publish('chan', null)).rejects.toThrow(/must be a Buffer or Uint8Array/);
            await expect(client.publish('chan', [])).rejects.toThrow(/must be a Buffer or Uint8Array/);
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
            expect(dup).toBeInstanceOf(OperatorPubSubClient);
            expect(dup.pubsubUrl).toBe(client.pubsubUrl);
            expect(dup.internalAuthToken).toBe(client.internalAuthToken);
        });
    });
});
