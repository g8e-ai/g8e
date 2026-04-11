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
// @vitest-environment jsdom

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { MockEventBus } from '@test/mocks/mock-browser-env.js';
import { EventType, ThinkingActionType } from '@vsod/public/js/constants/events.js';
import { ThinkingEvent } from '@vsod/public/js/models/ai-event-models.js';

async function loadThinkingManager() {
    const { ThinkingManager } = await import('@vsod/public/js/components/thinking.js');
    return ThinkingManager;
}

describe('ThinkingManager [UNIT]', () => {
    let ThinkingManager;
    let eventBus;
    let appendCalls;
    let completeCalls;
    let aiStopShowCount;
    let aiStopHideCount;

    beforeEach(async () => {
        ThinkingManager = await loadThinkingManager();
        eventBus = new MockEventBus();

        appendCalls = [];
        completeCalls = [];
        aiStopShowCount = 0;
        aiStopHideCount = 0;

        eventBus.on(EventType.OPERATOR_TERMINAL_THINKING_APPEND, ({ webSessionId, text }) => {
            appendCalls.push({ webSessionId, text });
        });
        eventBus.on(EventType.OPERATOR_TERMINAL_THINKING_COMPLETE, ({ webSessionId }) => {
            completeCalls.push(webSessionId);
        });
        eventBus.on(EventType.LLM_CHAT_STOP_SHOW, () => { aiStopShowCount++; });
        eventBus.on(EventType.LLM_CHAT_STOP_HIDE, () => { aiStopHideCount++; });
    });

    afterEach(() => {
        eventBus.removeAllListeners();
        vi.clearAllMocks();
    });

    describe('constructor', () => {
        it('initializes with thinkingActive false', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            expect(mgr.thinkingActive).toBe(false);
        });

        it('initializes with empty activeSessions', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            expect(mgr.activeSessions.size).toBe(0);
        });
    });

    describe('EventType.LLM_CHAT_ITERATION_THINKING_STARTED — ThinkingEvent.parse() integration', () => {
        it('parses raw event bus payload via ThinkingEvent.parse() before dispatch', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                thinking: 'starting',
                action_type: ThinkingActionType.START,
                web_session_id: 'sess-1',
            });
            expect(appendCalls.some(c => c.webSessionId === 'sess-1')).toBe(true);
        });

        it('strips unknown fields from raw payload — phase is not forwarded', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                thinking: 'hi',
                action_type: ThinkingActionType.UPDATE,
                web_session_id: 'sess-2',
                phase: 'start',
                thinking_content: 'legacy',
                thought_content: 'legacy2',
                thought: 'legacy3',
            });
            expect(appendCalls.filter(c => c.webSessionId === 'sess-2')).toHaveLength(1);
        });

        it('routes action_type START to handleThinkingStart', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                thinking: 'thinking...',
                action_type: ThinkingActionType.START,
                web_session_id: 'sess-start',
            });
            expect(mgr.thinkingActive).toBe(true);
            expect(mgr.activeSessions.has('sess-start')).toBe(true);
        });

        it('routes action_type END to handleThinkingEnd', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-end' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END, web_session_id: 'sess-end' });
            expect(mgr.activeSessions.has('sess-end')).toBe(false);
            expect(completeCalls).toContain('sess-end');
        });

        it('routes unknown action_type to handleThinkingUpdate', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                thinking: 'iterating',
                action_type: ThinkingActionType.UPDATE,
                web_session_id: 'sess-upd',
            });
            expect(mgr.thinkingActive).toBe(true);
            expect(appendCalls.some(c => c.webSessionId === 'sess-upd')).toBe(true);
        });

        it('drops event when CHAT.FILTER_EVENT rejects it', () => {
            eventBus.on(EventType.LLM_CHAT_FILTER_EVENT, ({ reject }) => reject());
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                action_type: ThinkingActionType.START,
                web_session_id: 'sess-filtered',
            });
            expect(mgr.thinkingActive).toBe(false);
            expect(appendCalls).toHaveLength(0);
        });
    });

    describe('handleThinkingStart', () => {
        it('adds session to activeSessions and sets thinkingActive', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-a' });
            expect(mgr.activeSessions.has('sess-a')).toBe(true);
            expect(mgr.thinkingActive).toBe(true);
        });

        it('emits CHAT.AI_STOP_SHOW', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-a' });
            expect(aiStopShowCount).toBeGreaterThan(0);
        });

        it('emits THINKING_APPEND with thinking text', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                thinking: 'initial thought',
                action_type: ThinkingActionType.START,
                web_session_id: 'sess-a',
            });
            expect(appendCalls).toContainEqual({ webSessionId: 'sess-a', text: 'initial thought' });
        });

        it('does not emit THINKING_APPEND when thinking is null', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-notext' });
            expect(appendCalls).toHaveLength(0);
        });

        it('warns and skips when web_session_id is missing', () => {
            const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START });
            expect(mgr.thinkingActive).toBe(false);
            expect(appendCalls).toHaveLength(0);
            expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining('web_session_id'));
            warnSpy.mockRestore();
        });

        it('tracks multiple concurrent sessions independently', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-1' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-2' });
            expect(mgr.activeSessions.size).toBe(2);
            expect(mgr.activeSessions.has('sess-1')).toBe(true);
            expect(mgr.activeSessions.has('sess-2')).toBe(true);
        });
    });

    describe('handleThinkingUpdate', () => {
        it('does not emit THINKING_APPEND when thinking is null', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.UPDATE, web_session_id: 'sess-upd' });
            expect(appendCalls).toHaveLength(0);
        });

        it('emits THINKING_APPEND with actual thinking text when present', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                thinking: 'new chunk',
                action_type: ThinkingActionType.UPDATE,
                web_session_id: 'sess-upd',
            });
            expect(appendCalls).toContainEqual({ webSessionId: 'sess-upd', text: 'new chunk' });
        });

        it('warns and skips when web_session_id is missing', () => {
            const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.UPDATE });
            expect(appendCalls).toHaveLength(0);
            expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining('web_session_id'));
            warnSpy.mockRestore();
        });
    });

    describe('handleThinkingEnd', () => {
        it('removes session from activeSessions', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-e' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END, web_session_id: 'sess-e' });
            expect(mgr.activeSessions.has('sess-e')).toBe(false);
        });

        it('sets thinkingActive false when no sessions remain', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-e' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END, web_session_id: 'sess-e' });
            expect(mgr.thinkingActive).toBe(false);
        });

        it('keeps thinkingActive true when other sessions still active', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-1' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-2' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END, web_session_id: 'sess-1' });
            expect(mgr.thinkingActive).toBe(true);
            expect(mgr.activeSessions.has('sess-2')).toBe(true);
        });

        it('emits THINKING_COMPLETE', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-e' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END, web_session_id: 'sess-e' });
            expect(completeCalls).toContain('sess-e');
        });

        it('emits THINKING_APPEND with final text before THINKING_COMPLETE', () => {
            new ThinkingManager(eventBus, null, null);
            const order = [];
            eventBus.on(EventType.OPERATOR_TERMINAL_THINKING_APPEND, () => order.push('append'));
            eventBus.on(EventType.OPERATOR_TERMINAL_THINKING_COMPLETE, () => order.push('complete'));
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-e' });
            appendCalls.length = 0;
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                thinking: 'final thought',
                action_type: ThinkingActionType.END,
                web_session_id: 'sess-e',
            });
            expect(appendCalls).toContainEqual({ webSessionId: 'sess-e', text: 'final thought' });
            expect(completeCalls).toContain('sess-e');
            expect(order.indexOf('append')).toBeLessThan(order.indexOf('complete'));
        });

        it('does not emit THINKING_APPEND when thinking is null on END', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-e' });
            appendCalls.length = 0;
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END, web_session_id: 'sess-e' });
            expect(appendCalls).toHaveLength(0);
        });

        it('emits CHAT.AI_STOP_HIDE', () => {
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-e' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END, web_session_id: 'sess-e' });
            expect(aiStopHideCount).toBeGreaterThan(0);
        });

        it('warns and skips when web_session_id is missing', () => {
            const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
            new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END });
            expect(completeCalls).toHaveLength(0);
            expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining('web_session_id'));
            warnSpy.mockRestore();
        });
    });

    describe('hideThinkingIndicator', () => {
        it('emits THINKING_COMPLETE for a specific session', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-h' });
            mgr.hideThinkingIndicator('sess-h');
            expect(completeCalls).toContain('sess-h');
            expect(mgr.activeSessions.has('sess-h')).toBe(false);
        });

        it('emits THINKING_COMPLETE for all sessions when called without argument', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-1' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-2' });
            mgr.hideThinkingIndicator();
            expect(completeCalls).toContain('sess-1');
            expect(completeCalls).toContain('sess-2');
            expect(mgr.activeSessions.size).toBe(0);
        });

        it('sets thinkingActive false after clearing all sessions', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-1' });
            mgr.hideThinkingIndicator();
            expect(mgr.thinkingActive).toBe(false);
        });

        it('emits CHAT.AI_STOP_HIDE', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            const before = aiStopHideCount;
            mgr.hideThinkingIndicator();
            expect(aiStopHideCount).toBeGreaterThan(before);
        });
    });

    describe('clearAllThinkingData', () => {
        it('resets thinkingActive to false', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-c' });
            mgr.clearAllThinkingData();
            expect(mgr.thinkingActive).toBe(false);
        });

        it('clears all active sessions', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-1' });
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-2' });
            mgr.clearAllThinkingData();
            expect(mgr.activeSessions.size).toBe(0);
        });
    });

    describe('getThinkingText', () => {
        it('returns empty string when data is null', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            expect(mgr.getThinkingText(null)).toBe('');
        });

        it('returns empty string when thinking is absent', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            expect(mgr.getThinkingText({ web_session_id: 'x' })).toBe('');
        });

        it('returns decoded thinking text when present', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            expect(mgr.getThinkingText({ thinking: 'hello &amp; world' })).toBe('hello & world');
        });
    });

    describe('no-op when no listeners registered', () => {
        it('constructs and handles events without throwing', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            expect(() => {
                eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, {
                    thinking: 'hi',
                    action_type: ThinkingActionType.START,
                    web_session_id: 'sess-nocomp',
                });
            }).not.toThrow();
            expect(mgr.thinkingActive).toBe(true);
        });

        it('handles END without throwing', () => {
            const mgr = new ThinkingManager(eventBus, null, null);
            eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.START, web_session_id: 'sess-n' });
            expect(() => {
                eventBus.emit(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, { action_type: ThinkingActionType.END, web_session_id: 'sess-n' });
            }).not.toThrow();
        });
    });
});
