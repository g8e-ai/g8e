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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { MockEventBus, MockElement } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@vsod/public/js/constants/events.js';
import { LlmModelManager } from '@vsod/public/js/components/llm-model-manager.js';

function buildSelectElement(id) {
    const el = new MockElement('select', id);
    el.innerHTML = '';
    el.options = [];
    el.value = '';
    return el;
}

const PRIMARY_MODELS = [
    { id: 'gemini-3.1-pro-preview', label: 'Gemini 3.1 Pro (Flagship)' },
    { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
];

const ASSISTANT_MODELS = [
    { id: 'gemini-3.1-flash-lite-preview', label: 'Gemini 3.1 Flash Lite' },
    { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
];

function emitConfig(eventBus, overrides = {}) {
    eventBus.emit(EventType.LLM_CONFIG_RECEIVED, {
        primary_models: PRIMARY_MODELS,
        assistant_models: ASSISTANT_MODELS,
        default_primary_model: 'gemini-3.1-pro-preview',
        default_assistant_model: 'gemini-3.1-flash-lite-preview',
        ...overrides,
    });
}

describe('LlmModelManager [UNIT]', () => {
    let eventBus;
    let manager;
    let primarySelect;
    let assistantSelect;

    beforeEach(() => {
        eventBus = new MockEventBus();
        primarySelect = buildSelectElement('llm-primary-model-select');
        assistantSelect = buildSelectElement('llm-assistant-model-select');

        global.document = {
            getElementById: (id) => {
                if (id === 'llm-primary-model-select') return primarySelect;
                if (id === 'llm-assistant-model-select') return assistantSelect;
                if (id === 'llm-primary-model-container') return new MockElement('div', id);
                if (id === 'llm-assistant-model-container') return new MockElement('div', id);
                return null;
            },
            createElement: (tag) => new MockElement(tag),
        };

        manager = new LlmModelManager(eventBus);
        manager.init();
    });

    afterEach(() => {
        manager.destroy?.();
        delete global.document;
        eventBus.clearLog();
    });

    describe('handleConfigReceived', () => {
        it('does not re-emit LLM_CONFIG_RECEIVED (regression: infinite recursion)', () => {
            eventBus.clearLog();
            emitConfig(eventBus);

            const reEmits = eventBus.getEventsOfType(EventType.LLM_CONFIG_RECEIVED);
            expect(reEmits.length).toBe(1);
        });

        it('sets availablePrimaryModels and availableAssistantModels from event data', () => {
            emitConfig(eventBus);

            expect(manager.availablePrimaryModels).toEqual(PRIMARY_MODELS);
            expect(manager.availableAssistantModels).toEqual(ASSISTANT_MODELS);
        });

        it('sets default models from event data', () => {
            emitConfig(eventBus);

            expect(manager.defaultPrimaryModel).toBe('gemini-3.1-pro-preview');
            expect(manager.defaultAssistantModel).toBe('gemini-3.1-flash-lite-preview');
        });

        it('defaults to empty string when defaults are missing', () => {
            emitConfig(eventBus, {
                default_primary_model: undefined,
                default_assistant_model: undefined,
            });

            expect(manager.defaultPrimaryModel).toBe('');
            expect(manager.defaultAssistantModel).toBe('');
        });

        it('sets selected models to defaults when unset', () => {
            manager.selectedPrimaryModel = '';
            manager.selectedAssistantModel = '';

            emitConfig(eventBus);

            expect(manager.selectedPrimaryModel).toBe('gemini-3.1-pro-preview');
            expect(manager.selectedAssistantModel).toBe('gemini-3.1-flash-lite-preview');
        });

        it('preserves selected models when already set', () => {
            manager.selectedPrimaryModel = 'gemini-3-flash-preview';
            manager.selectedAssistantModel = 'gemini-3-flash-preview';

            emitConfig(eventBus);

            expect(manager.selectedPrimaryModel).toBe('gemini-3-flash-preview');
            expect(manager.selectedAssistantModel).toBe('gemini-3-flash-preview');
        });

        it('does not stack overflow when LLM_CONFIG_RECEIVED fires multiple times', () => {
            expect(() => {
                for (let i = 0; i < 10; i++) {
                    emitConfig(eventBus);
                }
            }).not.toThrow();
        });
    });

    describe('getPrimaryModel / getAssistantModel', () => {
        it('returns selected models after config', () => {
            emitConfig(eventBus);

            expect(manager.getPrimaryModel()).toBe('gemini-3.1-pro-preview');
            expect(manager.getAssistantModel()).toBe('gemini-3.1-flash-lite-preview');
        });

        it('returns empty string before any config', () => {
            expect(manager.getPrimaryModel()).toBe('');
            expect(manager.getAssistantModel()).toBe('');
        });
    });

    describe('handleCaseSwitched', () => {
        it('restores saved models from investigation', () => {
            emitConfig(eventBus);

            eventBus.emit(EventType.CASE_SWITCHED, {
                investigation: {
                    llm_primary_model: 'gemini-3-flash-preview',
                    llm_assistant_model: 'gemini-3-flash-preview',
                },
            });

            expect(manager.selectedPrimaryModel).toBe('gemini-3-flash-preview');
            expect(manager.selectedAssistantModel).toBe('gemini-3-flash-preview');
        });

        it('falls back to defaults when investigation has no saved models', () => {
            emitConfig(eventBus);

            eventBus.emit(EventType.CASE_SWITCHED, { investigation: {} });

            expect(manager.selectedPrimaryModel).toBe('gemini-3.1-pro-preview');
            expect(manager.selectedAssistantModel).toBe('gemini-3.1-flash-lite-preview');
        });
    });

    describe('handleCaseCleared', () => {
        it('resets to defaults', () => {
            emitConfig(eventBus);
            manager.selectedPrimaryModel = 'gemini-3-flash-preview';
            manager.selectedAssistantModel = 'gemini-3-flash-preview';

            eventBus.emit(EventType.CASE_CLEARED);

            expect(manager.selectedPrimaryModel).toBe('gemini-3.1-pro-preview');
            expect(manager.selectedAssistantModel).toBe('gemini-3.1-flash-lite-preview');
        });
    });
});
