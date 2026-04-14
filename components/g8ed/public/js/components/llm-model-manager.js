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
 * - Two custom dropdowns: primary (complex tasks) and assistant (simple tasks)
 * - Both values are included with every chat message (empty string = server default)
 * - Available models per provider are delivered via SSE llm.config event on connect
 * - Models are grouped by provider using custom category headers
 * - Dropdowns drop UP to avoid being cut off at the bottom of the viewport
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

        this.primaryDropdown = null;
        this.assistantDropdown = null;
        this.primaryTextElement = null;
        this.assistantTextElement = null;
        this.primaryMenuElement = null;
        this.assistantMenuElement = null;

        this.primaryModelMap = new Map();
        this.assistantModelMap = new Map();
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
        this.primaryDropdown = document.getElementById('llm-primary-model-dropdown');
        this.assistantDropdown = document.getElementById('llm-assistant-model-dropdown');
        this.primaryTextElement = document.getElementById('llm-primary-model-text');
        this.assistantTextElement = document.getElementById('llm-assistant-model-text');
        this.primaryMenuElement = document.getElementById('llm-primary-model-menu');
        this.assistantMenuElement = document.getElementById('llm-assistant-model-menu');
    }

    setupEventListeners() {
        if (this._eventListenersRegistered) {
            return;
        }

        // Primary dropdown toggle
        if (this.primaryDropdown) {
            this.primaryDropdown.addEventListener('click', (e) => {
                e.stopPropagation();
                this._toggleDropdown('primary');
            });

            this.primaryDropdown.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    this._toggleDropdown('primary');
                } else if (e.key === 'Escape') {
                    this._closeAllDropdowns();
                }
            });
        }

        // Assistant dropdown toggle
        if (this.assistantDropdown) {
            this.assistantDropdown.addEventListener('click', (e) => {
                e.stopPropagation();
                this._toggleDropdown('assistant');
            });

            this.assistantDropdown.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    this._toggleDropdown('assistant');
                } else if (e.key === 'Escape') {
                    this._closeAllDropdowns();
                }
            });
        }

        // Close dropdowns when clicking outside
        document.addEventListener('click', () => {
            this._closeAllDropdowns();
        });

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

    _toggleDropdown(role) {
        const dropdown = role === 'primary' ? this.primaryDropdown : this.assistantDropdown;
        if (!dropdown) {
            return;
        }
        const isOpen = dropdown.classList.contains('open');

        this._closeAllDropdowns();

        if (!isOpen) {
            dropdown.classList.add('open');
        }
    }

    _closeAllDropdowns() {
        if (this.primaryDropdown) {
            this.primaryDropdown.classList.remove('open');
        }
        if (this.assistantDropdown) {
            this.assistantDropdown.classList.remove('open');
        }
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

        this._populateDropdowns();
    }

    _populateDropdowns() {
        this._populateGrouped('primary', this.selectedPrimaryModel);
        this._populateGrouped('assistant', this.selectedAssistantModel);
    }

    _populateGrouped(role, selectedValue) {
        const menuElement = role === 'primary' ? this.primaryMenuElement : this.assistantMenuElement;
        const textElement = role === 'primary' ? this.primaryTextElement : this.assistantTextElement;
        const modelMap = role === 'primary' ? this.primaryModelMap : this.assistantModelMap;

        if (!menuElement) return;
        menuElement.innerHTML = '';
        modelMap.clear();

        const hasModels = Object.keys(this.providerModels).length > 0;

        if (!hasModels) {
            if (textElement) {
                textElement.textContent = 'Loading...';
            }
            return;
        }

        let firstOption = null;

        for (const [provider, providerData] of Object.entries(this.providerModels)) {
            const models = providerData[role] || [];
            if (models.length === 0) continue;

            // Category header
            const category = document.createElement('div');
            category.className = 'llm-model-dropdown__category';
            category.textContent = providerData.label || 'Unknown';
            menuElement.appendChild(category);

            // Model options
            for (const model of models) {
                const option = document.createElement('div');
                option.className = 'llm-model-dropdown__option';
                option.textContent = model.label || model.id;
                option.dataset.value = model.id;
                option.dataset.provider = provider;

                if (model.id === selectedValue) {
                    option.classList.add('selected');
                    if (textElement) {
                        textElement.textContent = model.label || model.id;
                    }
                }

                if (!firstOption) {
                    firstOption = model;
                }

                // Store mapping for provider lookup
                modelMap.set(model.id, provider);

                option.addEventListener('click', (e) => {
                    e.stopPropagation();
                    this._selectModel(role, model.id, provider, model.label || model.id);
                });

                menuElement.appendChild(option);
            }
        }

        if (menuElement.children.length === 0) {
            if (textElement) {
                textElement.textContent = 'No models';
            }
        } else if (!selectedValue && firstOption) {
            // Select first available model if none selected
            const provider = this._findProviderForModel(role, firstOption.id);
            this._selectModel(role, firstOption.id, provider, firstOption.label || firstOption.id);
        }
    }

    _selectModel(role, modelId, provider, label) {
        if (role === 'primary') {
            this.selectedPrimaryModel = modelId;
            if (this.primaryTextElement) {
                this.primaryTextElement.textContent = label;
            }
        } else {
            this.selectedAssistantModel = modelId;
            if (this.assistantTextElement) {
                this.assistantTextElement.textContent = label;
            }
        }

        // Update selected state in UI
        this._updateSelectedState(role, modelId);

        // Close dropdown
        this._closeAllDropdowns();
    }

    _updateSelectedState(role, selectedValue) {
        const menuElement = role === 'primary' ? this.primaryMenuElement : this.assistantMenuElement;
        if (!menuElement) return;

        const options = menuElement.querySelectorAll('.llm-model-dropdown__option');
        options.forEach(option => {
            if (option.dataset.value === selectedValue) {
                option.classList.add('selected');
            } else {
                option.classList.remove('selected');
            }
        });
    }

    _findProviderForModel(role, modelId) {
        const modelMap = role === 'primary' ? this.primaryModelMap : this.assistantModelMap;
        return modelMap.get(modelId) || '';
    }

    handleCaseSwitched(data) {
        const savedPrimary = data?.investigation?.llm_primary_model;
        const savedAssistant = data?.investigation?.llm_assistant_model;

        this.selectedPrimaryModel = savedPrimary || this.defaultPrimaryModel;
        this.selectedAssistantModel = savedAssistant || this.defaultAssistantModel;
        this._syncDropdowns();
    }

    handleCaseCleared() {
        this.selectedPrimaryModel = this.defaultPrimaryModel;
        this.selectedAssistantModel = this.defaultAssistantModel;
        this._syncDropdowns();
    }

    _syncDropdowns() {
        // Re-populate to update selected state
        this._populateDropdowns();
    }

    getPrimaryModel() {
        return this.selectedPrimaryModel || '';
    }

    getAssistantModel() {
        return this.selectedAssistantModel || '';
    }

    getPrimaryProvider() {
        return this._findProviderForModel('primary', this.selectedPrimaryModel);
    }

    getAssistantProvider() {
        return this._findProviderForModel('assistant', this.selectedAssistantModel);
    }

    destroy() {
        this.primaryDropdown = null;
        this.assistantDropdown = null;
        this.primaryTextElement = null;
        this.assistantTextElement = null;
        this.primaryMenuElement = null;
        this.assistantMenuElement = null;
        this.primaryModelMap.clear();
        this.assistantModelMap.clear();
        this._eventListenersRegistered = false;
    }
}
