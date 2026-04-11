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
 * - Provider cannot be changed here — only in the Settings page
 */

import { EventType } from '../constants/events.js';
import { CssClass } from '../constants/ui-constants.js';

export class LlmModelManager {
    constructor(eventBus) {
        this.eventBus = eventBus;

        this.selectedPrimaryModel = '';
        this.selectedAssistantModel = '';

        this.availablePrimaryModels = [];
        this.availableAssistantModels = [];

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
        this.availablePrimaryModels = data.primary_models || [];
        this.availableAssistantModels = data.assistant_models || [];

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
        this._populateOne(this.primarySelectElement, this.availablePrimaryModels, this.selectedPrimaryModel);
        this._populateOne(this.assistantSelectElement, this.availableAssistantModels, this.selectedAssistantModel);

        if (this.primaryContainer) {
            this.primaryContainer.classList.toggle(CssClass.INITIALLY_HIDDEN, this.availablePrimaryModels.length === 0);
        }
        if (this.assistantContainer) {
            this.assistantContainer.classList.toggle(CssClass.INITIALLY_HIDDEN, this.availableAssistantModels.length === 0);
        }
    }

    _populateOne(selectElement, models, selectedValue) {
        if (!selectElement) return;
        selectElement.innerHTML = '';

        for (const model of models) {
            const option = document.createElement('option');
            option.value = model.id;
            option.textContent = model.label || model.id;
            if (model.id === selectedValue) {
                option.selected = true;
            }
            selectElement.appendChild(option);
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

    destroy() {
        this.primarySelectElement = null;
        this.assistantSelectElement = null;
        this.primaryContainer = null;
        this.assistantContainer = null;
        this._eventListenersRegistered = false;
    }
}
