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
import { EventType } from '@g8ed/public/js/constants/events.js';
import { LlmModelManager } from '@g8ed/public/js/components/llm-model-manager.js';

function buildDrawerElement(id) {
    const el = new MockElement('div', id);
    el.classList = {
        _classes: new Set(),
        add: function(className) { this._classes.add(className); },
        remove: function(className) { this._classes.delete(className); },
        contains: function(className) { return this._classes.has(className); },
    };
    el.innerHTML = '';
    return el;
}

function buildTextElement(id) {
    const el = new MockElement('span', id);
    el.textContent = '';
    return el;
}

function buildMenuElement(id) {
    const el = new MockElement('div', id);
    el.innerHTML = '';
    el.querySelectorAll = (selector) => [];
    return el;
}

const PROVIDER_MODELS = {
    gemini: {
        label: 'Gemini',
        all: [
            { id: 'gemini-3.1-pro-preview', label: 'Gemini 3.1 Pro (Flagship)' },
            { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
        ],
        primary: [
            { id: 'gemini-3.1-pro-preview', label: 'Gemini 3.1 Pro (Flagship)' },
            { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
        ],
        assistant: [
            { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
        ],
        lite: [
            { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
        ],
    },
};

const MULTI_PROVIDER_MODELS = {
    gemini: {
        label: 'Gemini',
        all: [
            { id: 'gemini-3.1-pro-preview', label: 'Gemini 3.1 Pro' },
            { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
        ],
        primary: [
            { id: 'gemini-3.1-pro-preview', label: 'Gemini 3.1 Pro' },
        ],
        assistant: [
            { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
        ],
        lite: [
            { id: 'gemini-3-flash-preview', label: 'Gemini 3 Flash' },
        ],
    },
    anthropic: {
        label: 'Anthropic',
        all: [
            { id: 'claude-opus-4-6', label: 'Claude Opus 4.6' },
            { id: 'claude-haiku-4-5', label: 'Claude Haiku 4.5' },
        ],
        primary: [
            { id: 'claude-opus-4-6', label: 'Claude Opus 4.6' },
        ],
        assistant: [
            { id: 'claude-haiku-4-5', label: 'Claude Haiku 4.5' },
        ],
        lite: [
            { id: 'claude-haiku-4-5', label: 'Claude Haiku 4.5' },
        ],
    },
};

function emitConfig(eventBus, overrides = {}) {
    eventBus.emit(EventType.LLM_CONFIG_RECEIVED, {
        provider_models: PROVIDER_MODELS,
        default_primary_model: 'gemini-3.1-pro-preview',
        default_assistant_model: 'gemini-3-flash-preview',
        default_lite_model: 'gemini-3-flash-preview',
        ...overrides,
    });
}

describe('LlmModelManager [UNIT]', () => {
    let eventBus;
    let manager;
    let drawerElement;
    let drawerText;
    let primaryMenu;
    let assistantMenu;
    let liteMenu;

    beforeEach(() => {
        eventBus = new MockEventBus();
        drawerElement = buildDrawerElement('llm-model-drawer');
        drawerText = buildTextElement('llm-model-drawer-text');
        primaryMenu = buildMenuElement('llm-primary-model-menu');
        assistantMenu = buildMenuElement('llm-assistant-model-menu');
        liteMenu = buildMenuElement('llm-lite-model-menu');

        global.document = {
            getElementById: (id) => {
                if (id === 'llm-model-drawer') return drawerElement;
                if (id === 'llm-model-drawer-text') return drawerText;
                if (id === 'llm-primary-model-menu') return primaryMenu;
                if (id === 'llm-assistant-model-menu') return assistantMenu;
                if (id === 'llm-lite-model-menu') return liteMenu;
                return null;
            },
            createElement: (tag) => new MockElement(tag),
            addEventListener: vi.fn(),
            querySelectorAll: () => [],
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

        it('stores provider_models from event data', () => {
            emitConfig(eventBus);

            expect(manager.providerModels).toEqual(PROVIDER_MODELS);
        });

        it('handles multi-provider config', () => {
            emitConfig(eventBus, { provider_models: MULTI_PROVIDER_MODELS });

            expect(Object.keys(manager.providerModels)).toEqual(['gemini', 'anthropic']);
            expect(manager.providerModels.gemini.label).toBe('Gemini');
            expect(manager.providerModels.anthropic.label).toBe('Anthropic');
        });

        it('sets default models from event data', () => {
            emitConfig(eventBus);

            expect(manager.defaultPrimaryModel).toBe('gemini-3.1-pro-preview');
            expect(manager.defaultAssistantModel).toBe('gemini-3-flash-preview');
            expect(manager.defaultLiteModel).toBe('gemini-3-flash-preview');
        });

        it('throws when no model defaults are configured', () => {
            expect(() => emitConfig(eventBus, {
                default_primary_model: undefined,
                default_assistant_model: undefined,
                default_lite_model: undefined,
            })).toThrow(/No .* model configured in settings/);
        });

        it('sets selected models to defaults when unset', () => {
            manager.selectedPrimaryModel = '';
            manager.selectedAssistantModel = '';
            manager.selectedLiteModel = '';

            emitConfig(eventBus);

            expect(manager.selectedPrimaryModel).toBe('gemini-3.1-pro-preview');
            expect(manager.selectedAssistantModel).toBe('gemini-3-flash-preview');
            expect(manager.selectedLiteModel).toBe('gemini-3-flash-preview');
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
            expect(manager.getAssistantModel()).toBe('gemini-3-flash-preview');
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
            expect(manager.selectedAssistantModel).toBe('gemini-3-flash-preview');
        });
    });

    describe('requestConfig', () => {
        it('only emits LLM_CONFIG_REQUESTED, no HTTP calls (regression: /sse/config 404)', () => {
            eventBus.clearLog();
            global.window = { serviceClient: { post: vi.fn() } };

            manager.requestConfig();

            const requested = eventBus.getEventsOfType(EventType.LLM_CONFIG_REQUESTED);
            expect(requested.length).toBe(1);
            expect(global.window.serviceClient.post).not.toHaveBeenCalled();

            delete global.window;
        });
    });

    describe('handleCaseCleared', () => {
        it('resets to defaults', () => {
            emitConfig(eventBus);
            manager.selectedPrimaryModel = 'gemini-3-flash-preview';
            manager.selectedAssistantModel = 'gemini-3-flash-preview';

            eventBus.emit(EventType.CASE_CLEARED);

            expect(manager.selectedPrimaryModel).toBe('gemini-3.1-pro-preview');
            expect(manager.selectedAssistantModel).toBe('gemini-3-flash-preview');
        });
    });
});
