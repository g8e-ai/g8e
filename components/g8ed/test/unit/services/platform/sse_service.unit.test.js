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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { SSEService } from '@g8ed/services/platform/sse_service.js';
import { G8eBaseModel } from '@g8ed/models/base.js';
import { LLMProvider } from '@g8ed/constants/ai.js';

class MockEvent extends G8eBaseModel {
    static fields = {
        type: { type: String, required: true },
        data: { type: Object, default: {} }
    };
}

describe('SSEService [UNIT]', () => {
    let service;
    let mockResponse;

    beforeEach(() => {
        service = new SSEService();
        mockResponse = {
            write: vi.fn(),
            flush: vi.fn(),
            writable: true,
            destroyed: false
        };
    });

    describe('registerConnection', () => {
        it('registers a new connection and returns connectionId', async () => {
            const result = await service.registerConnection('session_123', 'u-test', mockResponse);
            
            expect(result.connectionId).toBeDefined();
            expect(service.localConnections.size).toBe(1);
            expect(service.connectionsPerSession.get('session_123')).toBe(1);
        });

        it('replaces existing connection for the same session and increments count correctly', async () => {
            const res1 = { writable: true };
            const res2 = { writable: true };
            
            const reg1 = await service.registerConnection('s1', 'u-test', res1);
            const reg2 = await service.registerConnection('s1', 'u-test', res2);
            
            expect(reg1.connectionId).not.toBe(reg2.connectionId);
            expect(service.localConnections.size).toBe(1);
            expect(service.localConnections.get('s1').connectionId).toBe(reg2.connectionId);
            expect(service.connectionsPerSession.get('s1')).toBe(1);
        });

        it('supports multiple unique sessions', async () => {
            await service.registerConnection('s1', 'u-test', { writable: true });
            await service.registerConnection('s2', 'u-test', { writable: true });
            expect(service.localConnections.size).toBe(2);
            expect(service.connectionsPerSession.size).toBe(2);
        });
    });

    describe('unregisterConnection', () => {
        it('unregisters only if connectionId matches', async () => {
            const { connectionId } = await service.registerConnection('s1', 'u-test', mockResponse);
            
            service.unregisterConnection('s1', 'wrong_id');
            expect(service.localConnections.size).toBe(1);
            
            service.unregisterConnection('s1', connectionId);
            expect(service.localConnections.size).toBe(0);
        });

        it('decrements session count and cleans up map when zero', async () => {
            const { connectionId } = await service.registerConnection('s1', 'u-test', mockResponse);
            expect(service.connectionsPerSession.get('s1')).toBe(1);
            
            service.unregisterConnection('s1', connectionId);
            expect(service.connectionsPerSession.has('s1')).toBe(false);
        });

        it('logs skipping unregister if connection was replaced', async () => {
            const res1 = { writable: true };
            const { connectionId: id1 } = await service.registerConnection('s1', 'u-test', res1);
            const res2 = { writable: true };
            const { connectionId: id2 } = await service.registerConnection('s1', 'u-test', res2);

            service.unregisterConnection('s1', id1);
            expect(service.localConnections.get('s1').connectionId).toBe(id2);
            // Count was 1, then replaced (still 1), then unregister decrements to 0 and deletes
            expect(service.connectionsPerSession.has('s1')).toBeFalsy(); 
        });
    });

    describe('publishEvent', () => {
        it('writes to response in SSE format', async () => {
            await service.registerConnection('s1', 'u-test', mockResponse);
            const event = new MockEvent({ type: 'test_event', data: { foo: 'bar' } });
            
            await service.publishEvent('s1', event);
            
            expect(mockResponse.write).toHaveBeenCalledWith(
                expect.stringContaining('data: {"type":"test_event","data":{"foo":"bar"}}')
            );
            expect(mockResponse.flush).toHaveBeenCalled();
        });

        it('returns true even if no connection (fire-and-forget)', async () => {
            const event = new MockEvent({ type: 'test', data: {} });
            const result = await service.publishEvent('missing_session', event);
            expect(result).toBe(true);
        });

        it('returns true even if connection is not writable', async () => {
            const res = { writable: false };
            await service.registerConnection('s1', 'u-test', res);
            const event = new MockEvent({ type: 'test', data: {} });
            const result = await service.publishEvent('s1', event);
            expect(result).toBe(true);
        });

        it('throws if event is not a G8eBaseModel', async () => {
            await expect(service.publishEvent('s1', { type: 'wrong' }))
                .rejects.toThrow('requires a G8eBaseModel instance');
        });

        it('calls delivery callback with delivered=true when event is successfully delivered', async () => {
            await service.registerConnection('s1', 'u-test', mockResponse);
            const event = new MockEvent({ type: 'test', data: {} });
            const callback = vi.fn();

            await service.publishEvent('s1', event, callback);

            expect(callback).toHaveBeenCalledWith({
                delivered: true,
                webSessionId: 's1',
                eventType: 'test'
            });
        });

        it('calls delivery callback with delivered=false when no connection exists', async () => {
            const event = new MockEvent({ type: 'test', data: {} });
            const callback = vi.fn();

            await service.publishEvent('missing_session', event, callback);

            expect(callback).toHaveBeenCalledWith({
                delivered: false,
                webSessionId: 'missing_session',
                eventType: 'test'
            });
        });

        it('calls delivery callback with delivered=false when connection is not writable', async () => {
            const res = { writable: false };
            await service.registerConnection('s1', 'u-test', res);
            const event = new MockEvent({ type: 'test', data: {} });
            const callback = vi.fn();

            await service.publishEvent('s1', event, callback);

            expect(callback).toHaveBeenCalledWith({
                delivered: false,
                webSessionId: 's1',
                eventType: 'test'
            });
        });

        it('does not throw when delivery callback throws an error', async () => {
            await service.registerConnection('s1', 'u-test', mockResponse);
            const event = new MockEvent({ type: 'test', data: {} });
            const callback = vi.fn(() => {
                throw new Error('Callback error');
            });

            const result = await service.publishEvent('s1', event, callback);

            expect(result).toBe(true);
            expect(callback).toHaveBeenCalled();
        });

        it('works without callback (backward compatibility)', async () => {
            await service.registerConnection('s1', 'u-test', mockResponse);
            const event = new MockEvent({ type: 'test', data: {} });

            const result = await service.publishEvent('s1', event);

            expect(result).toBe(true);
            expect(mockResponse.write).toHaveBeenCalled();
        });
    });

    describe('registerConnection requires userId', () => {
        it('throws when userId is missing', async () => {
            await expect(service.registerConnection('s1', undefined, mockResponse))
                .rejects.toThrow('requires a userId');
            await expect(service.registerConnection('s1', '', mockResponse))
                .rejects.toThrow('requires a userId');
        });
    });

    describe('publishToUser', () => {
        it('fans out to every locally connected session for the user', async () => {
            const res1 = { write: vi.fn(), writable: true, flush: vi.fn() };
            const res2 = { write: vi.fn(), writable: true, flush: vi.fn() };
            const resOther = { write: vi.fn(), writable: true, flush: vi.fn() };

            await service.registerConnection('s1', 'user-A', res1);
            await service.registerConnection('s2', 'user-A', res2);
            await service.registerConnection('s3', 'user-B', resOther);

            const event = new MockEvent({ type: 'fan_out', data: { x: 1 } });
            const count = await service.publishToUser('user-A', event);

            expect(count).toBe(2);
            expect(res1.write).toHaveBeenCalled();
            expect(res2.write).toHaveBeenCalled();
            expect(resOther.write).not.toHaveBeenCalled();
        });

        it('returns 0 when user has no local sessions', async () => {
            const event = new MockEvent({ type: 'fan_out', data: {} });
            const count = await service.publishToUser('user-none', event);
            expect(count).toBe(0);
        });

        it('throws when event is not a G8eBaseModel', async () => {
            await expect(service.publishToUser('user-A', { type: 'wrong' }))
                .rejects.toThrow('requires a G8eBaseModel instance');
        });

        it('removes the user-sessions entry when the last session disconnects', async () => {
            const { connectionId } = await service.registerConnection('s1', 'user-A', mockResponse);
            expect(service.userSessions.get('user-A').size).toBe(1);

            service.unregisterConnection('s1', connectionId);
            expect(service.userSessions.has('user-A')).toBe(false);
        });

        it('keeps the user-sessions entry while other sessions remain', async () => {
            const { connectionId: id1 } = await service.registerConnection('s1', 'user-A', { writable: true });
            await service.registerConnection('s2', 'user-A', { writable: true });

            service.unregisterConnection('s1', id1);
            expect(service.userSessions.get('user-A').size).toBe(1);
            expect(service.userSessions.get('user-A').has('s2')).toBe(true);
        });
    });

    describe('broadcastEvent', () => {
        it('sends to all registered connections', async () => {
            const res1 = { write: vi.fn(), writable: true, flush: vi.fn() };
            const res2 = { write: vi.fn(), writable: true, flush: vi.fn() };
            
            await service.registerConnection('s1', 'u-test', res1);
            await service.registerConnection('s2', 'u-test', res2);
            
            const event = new MockEvent({ type: 'broadcast', data: {} });
            const count = await service.broadcastEvent(event);
            
            expect(count).toBe(2);
            expect(res1.write).toHaveBeenCalled();
            expect(res2.write).toHaveBeenCalled();
        });

        it('returns 0 and logs error if broadcast fails', async () => {
            await service.registerConnection('s1', 'u-test', { 
                get writable() { throw new Error('Crashed'); } 
            });
            const event = new MockEvent({ type: 'broadcast', data: {} });
            const count = await service.broadcastEvent(event);
            expect(count).toBe(0);
        });

        it('throws if event is not G8eBaseModel', async () => {
            await expect(service.broadcastEvent({}))
                .rejects.toThrow('requires a G8eBaseModel instance');
        });
    });

    describe('hasLocalConnection', () => {
        it('returns true for healthy connection', async () => {
            await service.registerConnection('s1', 'u-test', mockResponse);
            expect(service.hasLocalConnection('s1')).toBeTruthy();
        });

        it('returns false for destroyed connection', async () => {
            mockResponse.destroyed = true;
            await service.registerConnection('s1', 'u-test', mockResponse);
            expect(service.hasLocalConnection('s1')).toBeFalsy();
        });

        it('returns false for missing connection', () => {
            expect(service.hasLocalConnection('none')).toBeFalsy();
        });
    });

    describe('getSessionConnectionCount', () => {
        it('returns correct count', async () => {
            await service.registerConnection('s1', 'u-test', { writable: true });
            expect(service.getSessionConnectionCount('s1')).toBe(1);
            expect(service.getSessionConnectionCount('none')).toBe(0);
        });
    });

    describe('getStats', () => {
        it('returns service statistics', async () => {
            await service.registerConnection('s1', 'u-test', mockResponse);
            const stats = service.getStats();
            expect(stats).toEqual({
                localConnections: 1,
                uniqueSessions: 1,
                healthy: true
            });
        });
    });

    describe('healthy property', () => {
        it('reflects healthy state', () => {
            expect(service.healthy).toBe(true);
            service.healthy = false;
            expect(service.healthy).toBe(false);
        });
    });

    describe('waitForReady', () => {
        it('sets healthy to true', async () => {
            service.healthy = false;
            await service.waitForReady();
            expect(service.healthy).toBe(true);
        });
    });

    describe('close', () => {
        it('cleans up all connections and sets healthy to false', async () => {
            await service.registerConnection('s1', 'u-test', mockResponse);
            await service.close();
            expect(service.healthy).toBe(false);
            expect(service.localConnections.size).toBe(0);
            expect(service.connectionsPerSession.size).toBe(0);
        });
    });

    describe('pushInitialState', () => {
        let mockSettingsService;
        let mockInternalHttpClient;
        let mockBoundSessionsService;
        let mockInvestigationService;

        beforeEach(() => {
            mockSettingsService = {
                getPlatformSettings: vi.fn().mockResolvedValue({ llm_primary_provider: LLMProvider.OPENAI, llm_model: 'gpt-4' }),
                getUserSettings: vi.fn().mockResolvedValue({})
            };
            mockInternalHttpClient = {
                queryInvestigations: vi.fn().mockResolvedValue([{ id: 'inv_1', case_title: 'Test Case' }])
            };
            mockBoundSessionsService = {
                resolveBoundOperators: vi.fn().mockResolvedValue([])
            };
            mockInvestigationService = {
                getInvestigationsByUserId: vi.fn().mockResolvedValue([{ id: 'inv_1', case_title: 'Test Case' }])
            };
            service.setDependencies({
                settingsService: mockSettingsService,
                internalHttpClient: mockInternalHttpClient,
                boundSessionsService: mockBoundSessionsService,
                investigationService: mockInvestigationService
            });
            vi.spyOn(service, 'publishEvent').mockResolvedValue(true);
        });

        it('pushes LLM config and investigation list', async () => {
            await service.pushInitialState('user_123', 'session_123', 'org_123');

            expect(mockSettingsService.getPlatformSettings).toHaveBeenCalled();
            expect(mockInvestigationService.getInvestigationsByUserId).toHaveBeenCalledWith('user_123');
            expect(service.publishEvent).toHaveBeenCalledTimes(2);
        });

        it('skips if dependencies are missing', async () => {
            const emptyService = new SSEService();
            const logSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
            await emptyService.pushInitialState('user_123', 'session_123');
            // Using internal logger would be better but checking logic for now
        });
    });

    describe('sendToLocal (Internal Error Handling)', () => {
        it('returns false if connection write throws', async () => {
            const badResponse = {
                writable: true,
                write: vi.fn(() => { throw new Error('Write failed'); })
            };
            await service.registerConnection('s1', 'u-test', badResponse);
            const result = await service.sendToLocal('s1', { type: 'test' });
            expect(result).toBe(false);
        });
    });
});
