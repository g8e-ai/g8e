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
 * LlmModelManager - Handles LLM model selection in the chat UI
 *
 * Architecture:
 * - Two dropdowns: primary (complex tasks) and assistant (simple tasks)
 * - Both values are included with every chat message (empty string = server default)
 * - Available models per provider are delivered via SSE llm.config event on connect
 * - Models are grouped by provider using <optgroup> elements
 * - Provider cannot be changed here — only in the Settings page
 */

import { EventType } from '../constants/events.js';

export class LlmModelManager {
    constructor(eventBus) {
        this.eventBus = eventBus;

        this.selectedPrimaryModel = '';
        this.selectedAssistantModel = '';

        this.providerModels = {};

        this.defaultPrimaryModel = '';
        this.defaultAssistantModel = '';

        this.primarySelectElement = null;
        this.assistantSelectElement = null;
        this.primaryContainer = null;
        this.assistantContainer = null;
    }

    init() {
        if (this._initialized) {
            return;
        }
        this._initialized = true;

        this.setupDOMElements();
        this.setupEventListeners();

        // Request config in case we missed the initial SSE push
        this.requestConfig();
    }

    requestConfig() {
        this.eventBus.emit(EventType.LLM_CONFIG_REQUESTED);
    }

    setupDOMElements() {
        this.primaryContainer = document.getElementById('llm-primary-model-container');
        this.assistantContainer = document.getElementById('llm-assistant-model-container');
        this.primarySelectElement = document.getElementById('llm-primary-model-select');
        this.assistantSelectElement = document.getElementById('llm-assistant-model-select');
    }

    setupEventListeners() {
        if (this._eventListenersRegistered) {
            return;
        }

        if (this.primarySelectElement) {
            this.primarySelectElement.addEventListener('change', (e) => {
                this.selectedPrimaryModel = e.target.value;
            });
        }

        if (this.assistantSelectElement) {
            this.assistantSelectElement.addEventListener('change', (e) => {
                this.selectedAssistantModel = e.target.value;
            });
        }

        this.eventBus.on(EventType.LLM_CONFIG_RECEIVED, (data) => {
            this.handleConfigReceived(data);
        });

        this.eventBus.on(EventType.CASE_SWITCHED, (data) => {
            this.handleCaseSwitched(data);
        });

        this.eventBus.on(EventType.CASE_CLEARED, () => {
            this.handleCaseCleared();
        });

        this._eventListenersRegistered = true;
    }

    handleConfigReceived(data) {
        this.providerModels = data.provider_models || {};

        this.defaultPrimaryModel = data.default_primary_model || '';
        this.defaultAssistantModel = data.default_assistant_model || '';

        if (!this.selectedPrimaryModel) {
            this.selectedPrimaryModel = this.defaultPrimaryModel;
        }
        if (!this.selectedAssistantModel) {
            this.selectedAssistantModel = this.defaultAssistantModel;
        }

        this._populateSelects();
    }

    _populateSelects() {
        this._populateGrouped(this.primarySelectElement, 'primary', this.selectedPrimaryModel);
        this._populateGrouped(this.assistantSelectElement, 'assistant', this.selectedAssistantModel);
    }

    _populateGrouped(selectElement, role, selectedValue) {
        if (!selectElement) return;
        selectElement.innerHTML = '';

        const hasModels = Object.keys(this.providerModels).length > 0;

        if (!hasModels) {
            const placeholder = document.createElement('option');
            placeholder.value = '';
            placeholder.textContent = 'Loading models...';
            placeholder.disabled = true;
            placeholder.selected = true;
            selectElement.appendChild(placeholder);
            return;
        }

        for (const [provider, providerData] of Object.entries(this.providerModels)) {
            const models = providerData[role] || [];
            if (models.length === 0) continue;

            const group = document.createElement('optgroup');
            group.label = providerData.label || 'Unknown';

            for (const model of models) {
                const option = document.createElement('option');
                option.value = model.id;
                option.textContent = model.label || model.id;
                option.dataset.provider = provider;
                if (model.id === selectedValue) {
                    option.selected = true;
                }
                group.appendChild(option);
            }
            selectElement.appendChild(group);
        }

        if (selectElement.options.length === 0) {
            const placeholder = document.createElement('option');
            placeholder.value = '';
            placeholder.textContent = 'No models available';
            placeholder.disabled = true;
            placeholder.selected = true;
            selectElement.appendChild(placeholder);
        }
    }

    handleCaseSwitched(data) {
        const savedPrimary = data?.investigation?.llm_primary_model;
        const savedAssistant = data?.investigation?.llm_assistant_model;

        this.selectedPrimaryModel = savedPrimary || this.defaultPrimaryModel;
        this.selectedAssistantModel = savedAssistant || this.defaultAssistantModel;
        this._syncSelects();
    }

    handleCaseCleared() {
        this.selectedPrimaryModel = this.defaultPrimaryModel;
        this.selectedAssistantModel = this.defaultAssistantModel;
        this._syncSelects();
    }

    _syncSelects() {
        if (this.primarySelectElement) {
            this.primarySelectElement.value = this.selectedPrimaryModel;
        }
        if (this.assistantSelectElement) {
            this.assistantSelectElement.value = this.selectedAssistantModel;
        }
    }

    getPrimaryModel() {
        return this.selectedPrimaryModel || '';
    }

    getAssistantModel() {
        return this.selectedAssistantModel || '';
    }

    getPrimaryProvider() {
        if (!this.selectedPrimaryModel || !this.primarySelectElement) return '';
        const selectedOption = this.primarySelectElement.options[this.primarySelectElement.selectedIndex];
        if (selectedOption && selectedOption.dataset.provider) {
            return selectedOption.dataset.provider;
        }
        for (const [provider, providerData] of Object.entries(this.providerModels)) {
            const models = providerData.primary || [];
            if (models.some(m => m.id === this.selectedPrimaryModel)) {
                return provider;
            }
        }
        return '';
    }

    getAssistantProvider() {
        if (!this.selectedAssistantModel || !this.assistantSelectElement) return '';
        const selectedOption = this.assistantSelectElement.options[this.assistantSelectElement.selectedIndex];
        if (selectedOption && selectedOption.dataset.provider) {
            return selectedOption.dataset.provider;
        }
        for (const [provider, providerData] of Object.entries(this.providerModels)) {
            const models = providerData.assistant || [];
            if (models.some(m => m.id === this.selectedAssistantModel)) {
                return provider;
            }
        }
        return '';
    }

    destroy() {
        this.primarySelectElement = null;
        this.assistantSelectElement = null;
        this.primaryContainer = null;
        this.assistantContainer = null;
        this._eventListenersRegistered = false;
    }
}
