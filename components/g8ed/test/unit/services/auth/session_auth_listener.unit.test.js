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
import crypto from 'crypto';
import { SessionAuthListener } from '@g8ed/services/auth/session_auth_listener.js';
import { PubSubChannel } from '@g8ed/constants/channels.js';
import { ApiKeyError, SESSION_AUTH_LISTEN_TTL_MS } from '@g8ed/constants/auth.js';
import { DEFAULT_OPERATOR_CONFIG } from '@g8ed/constants/operator_defaults.js';

describe('SessionAuthListener', () => {
    let listener;
    let mockPubSubClient;
    let mockSubscriber;
    let mockOperatorSessionService;
    let mockOperatorService;

    const mockG8eContext = {
        operator_session_id: 'sess-123',
        operator_id: 'op-456',
        user_id: 'user-789',
        organization_id: 'org-abc'
    };

    const sessionHash = crypto.createHash('sha256').update(mockG8eContext.operator_session_id).digest('hex');
    const authChannel = `${PubSubChannel.AUTH_PUBLISH_SESSION_PREFIX}${sessionHash}`;
    const responseChannel = `${PubSubChannel.AUTH_RESPONSE_SESSION_PREFIX}${sessionHash}`;

    beforeEach(() => {
        vi.useFakeTimers();

        mockSubscriber = {
            on: vi.fn(),
            subscribe: vi.fn().mockResolvedValue(true),
            publish: vi.fn().mockResolvedValue(true),
            terminate: vi.fn(),
            // Mock EventEmitter behavior for 'message'
            emitMessage: function(channel, data) {
                const callback = this.on.mock.calls.find(call => call[0] === 'message')?.[1];
                if (callback) return callback(channel, data);
            }
        };

        mockPubSubClient = {
            duplicate: vi.fn().mockResolvedValue(mockSubscriber)
        };

        mockOperatorSessionService = {
            validateSession: vi.fn()
        };

        mockOperatorService = {
            getOperator: vi.fn()
        };

        listener = new SessionAuthListener({
            pubSubClient: mockPubSubClient,
            operatorSessionService: mockOperatorSessionService,
            operatorService: mockOperatorService
        });
    });

    afterEach(() => {
        vi.useRealTimers();
        vi.restoreAllMocks();
    });

    describe('constructor', () => {
        it('should throw if dependencies are missing', () => {
            expect(() => new SessionAuthListener({ 
                operatorSessionService: {}, 
                operatorService: {} 
            })).toThrow('pubSubClient is required');
            expect(() => new SessionAuthListener({ 
                pubSubClient: {}, 
                operatorService: {} 
            })).toThrow('operatorSessionService is required');
            expect(() => new SessionAuthListener({ 
                pubSubClient: {}, 
                operatorSessionService: {} 
            })).toThrow('operatorService is required');
        });
    });

    describe('listen', () => {
        it('should set up subscription and timeout cleanup', async () => {
            listener.listen(mockG8eContext);
            
            // Wait for promise chain
            await vi.runAllTimersAsync();

            expect(mockPubSubClient.duplicate).toHaveBeenCalled();
            expect(mockSubscriber.subscribe).toHaveBeenCalledWith(authChannel);
            expect(mockSubscriber.on).toHaveBeenCalledWith('message', expect.any(Function));
        });

        it('should terminate subscriber after timeout if no message received', async () => {
            listener.listen(mockG8eContext);
            await vi.runAllTimersAsync();

            vi.advanceTimersByTime(SESSION_AUTH_LISTEN_TTL_MS + 100);
            
            expect(mockSubscriber.terminate).toHaveBeenCalled();
        });

        it('should ignore messages on other channels', async () => {
            listener.listen(mockG8eContext);
            await vi.runAllTimersAsync();

            await mockSubscriber.emitMessage('wrong-channel', {});
            
            expect(mockOperatorSessionService.validateSession).not.toHaveBeenCalled();
        });

        it('should publish failure if session is invalid', async () => {
            listener.listen(mockG8eContext);
            await vi.runAllTimersAsync();

            mockOperatorSessionService.validateSession.mockResolvedValue(null);

            await mockSubscriber.emitMessage(authChannel, {});

            expect(mockSubscriber.publish).toHaveBeenCalledWith(responseChannel, expect.objectContaining({
                success: false,
                error: 'Session not found or expired'
            }));
            expect(mockSubscriber.terminate).toHaveBeenCalled();
        });

        it('should publish success response with bootstrap config for valid session', async () => {
            listener.listen(mockG8eContext);
            await vi.runAllTimersAsync();

            const mockSession = { is_active: true };
            const mockOperator = { api_key: 'op-api-key-123' };

            mockOperatorSessionService.validateSession.mockResolvedValue(mockSession);
            mockOperatorService.getOperator.mockResolvedValue(mockOperator);

            await mockSubscriber.emitMessage(authChannel, {});

            expect(mockSubscriber.publish).toHaveBeenCalledWith(responseChannel, expect.objectContaining({
                success: true,
                operator_id: mockG8eContext.operator_id,
                api_key: mockOperator.api_key,
                config: expect.objectContaining(DEFAULT_OPERATOR_CONFIG)
            }));
            expect(mockSubscriber.terminate).toHaveBeenCalled();
        });

        it('should handle errors gracefully and publish failure', async () => {
            listener.listen(mockG8eContext);
            await vi.runAllTimersAsync();

            mockOperatorSessionService.validateSession.mockRejectedValue(new Error('DB Error'));

            await mockSubscriber.emitMessage(authChannel, {});

            expect(mockSubscriber.publish).toHaveBeenCalledWith(responseChannel, expect.objectContaining({
                success: false,
                error: ApiKeyError.AUTH_FAILED
            }));
            expect(mockSubscriber.terminate).toHaveBeenCalled();
        });

        it('should handle setup errors gracefully', async () => {
            mockPubSubClient.duplicate.mockRejectedValue(new Error('Connection failed'));
            
            listener.listen(mockG8eContext);
            await vi.runAllTimersAsync();

            // Should not crash, just log error
            expect(mockSubscriber.subscribe).not.toHaveBeenCalled();
        });
    });
});
