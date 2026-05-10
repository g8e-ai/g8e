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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockEventBus } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@g8ed/public/js/constants/events.js';

function buildDOM() {
    document.body.innerHTML = `
        <div id="anchored-terminal-container">
            <div id="anchored-terminal">
                <div id="anchored-terminal-output"></div>
                <input id="anchored-terminal-input" type="text" />
                <button id="anchored-terminal-send"></button>
                <span id="anchored-terminal-hostname"></span>
                <span id="anchored-terminal-prompt"></span>
                <button id="anchored-terminal-attach"></button>
                <div id="anchored-terminal-attachments"></div>
                <div id="anchored-terminal-mode"></div>
                <div id="panel-resize-handle"></div>
                <button id="anchored-terminal-maximize"></button>
                <div class="anchored-terminal__input-area"></div>
                <div id="anchored-terminal-body" style="height:400px;overflow:auto;"></div>
            </div>
        </div>
    `;
}

function makeMockLlmModelManager(primaryModel = '', assistantModel = '') {
    return {
        getPrimaryModel: () => primaryModel,
        getAssistantModel: () => assistantModel,
    };
}

describe('AnchoredTerminal sendChatMessage model validation [FRONTEND - jsdom]', () => {
    let AnchoredOperatorTerminal;
    let eventBus;
    let terminal;

    beforeEach(async () => {
        vi.resetModules();
        buildDOM();

        eventBus = new MockEventBus();
        window.llmModelManager = makeMockLlmModelManager('gemini-2.0-flash', 'gemini-2.0-flash');

        const mod = await import('@g8ed/public/js/components/anchored-terminal.js');
        AnchoredOperatorTerminal = mod.AnchoredOperatorTerminal;

        terminal = new AnchoredOperatorTerminal(eventBus);
        terminal.currentUser = { id: 'test-user' };
        terminal.isAuthenticated = true;
        terminal.appendSystemMessage = vi.fn();
    });

    afterEach(() => {
        vi.restoreAllMocks();
        eventBus.removeAllListeners();
        delete window.llmModelManager;
        document.body.innerHTML = '';
    });

    describe('sendChatMessage model selection validation', () => {
        it('allows sending message when both primary and assistant models are selected', async () => {
            window.llmModelManager = makeMockLlmModelManager('gemini-2.0-flash', 'gemini-2.0-flash');
            
            const emitSpy = vi.spyOn(eventBus, 'emit');
            const appendSystemMessageSpy = vi.spyOn(terminal, 'appendSystemMessage');

            await terminal.sendChatMessage('test message');

            expect(appendSystemMessageSpy).not.toHaveBeenCalled();
            expect(emitSpy).toHaveBeenCalledWith(EventType.LLM_CHAT_SUBMITTED, {
                message: 'test message',
                attachments: []
            });
        });

        it('blocks message and shows error when both models are unselected', async () => {
            window.llmModelManager = makeMockLlmModelManager('', '');
            
            const emitSpy = vi.spyOn(eventBus, 'emit');
            const appendSystemMessageSpy = vi.spyOn(terminal, 'appendSystemMessage');

            await terminal.sendChatMessage('test message');

            expect(appendSystemMessageSpy).toHaveBeenCalledWith('Please select both a primary and assistant model before sending a message');
            expect(emitSpy).not.toHaveBeenCalled();
        });

        it('blocks message and shows error when only primary model is unselected', async () => {
            window.llmModelManager = makeMockLlmModelManager('', 'gemini-2.0-flash');
            
            const emitSpy = vi.spyOn(eventBus, 'emit');
            const appendSystemMessageSpy = vi.spyOn(terminal, 'appendSystemMessage');

            await terminal.sendChatMessage('test message');

            expect(appendSystemMessageSpy).toHaveBeenCalledWith('Please select a primary model before sending a message');
            expect(emitSpy).not.toHaveBeenCalled();
        });

        it('blocks message and shows error when only assistant model is unselected', async () => {
            window.llmModelManager = makeMockLlmModelManager('gemini-2.0-flash', '');
            
            const emitSpy = vi.spyOn(eventBus, 'emit');
            const appendSystemMessageSpy = vi.spyOn(terminal, 'appendSystemMessage');

            await terminal.sendChatMessage('test message');

            expect(appendSystemMessageSpy).toHaveBeenCalledWith('Please select an assistant model before sending a message');
            expect(emitSpy).not.toHaveBeenCalled();
        });

        it('handles missing window.llmModelManager gracefully', async () => {
            delete window.llmModelManager;
            
            const emitSpy = vi.spyOn(eventBus, 'emit');
            const appendSystemMessageSpy = vi.spyOn(terminal, 'appendSystemMessage');

            await terminal.sendChatMessage('test message');

            expect(appendSystemMessageSpy).toHaveBeenCalledWith('Please select both a primary and assistant model before sending a message');
            expect(emitSpy).not.toHaveBeenCalled();
        });

        it('still blocks message when user is not authenticated', async () => {
            window.llmModelManager = makeMockLlmModelManager('gemini-2.0-flash', 'gemini-2.0-flash');
            terminal.currentUser = null;
            terminal.isAuthenticated = false;
            
            const emitSpy = vi.spyOn(eventBus, 'emit');
            const appendSystemMessageSpy = vi.spyOn(terminal, 'appendSystemMessage');

            await terminal.sendChatMessage('test message');

            expect(appendSystemMessageSpy).toHaveBeenCalledWith('Please sign in to send messages');
            expect(emitSpy).not.toHaveBeenCalled();
        });
    });
});
