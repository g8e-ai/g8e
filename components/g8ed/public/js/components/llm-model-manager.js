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
        this.selectedLiteModel = '';

        this.providerModels = {};

        this.defaultPrimaryModel = '';
        this.defaultAssistantModel = '';
        this.defaultLiteModel = '';

        this.drawerElement = null;
        this.drawerTextElement = null;
        this.primaryMenuElement = null;
        this.assistantMenuElement = null;
        this.liteMenuElement = null;
        this.activeTab = 'primary';

        this.primaryModelMap = new Map();
        this.assistantModelMap = new Map();
        this.liteModelMap = new Map();
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
        this.drawerElement = document.getElementById('llm-model-drawer');
        this.drawerTextElement = document.getElementById('llm-model-drawer-text');
        this.primaryMenuElement = document.getElementById('llm-primary-model-menu');
        this.assistantMenuElement = document.getElementById('llm-assistant-model-menu');
        this.liteMenuElement = document.getElementById('llm-lite-model-menu');
    }

    setupEventListeners() {
        if (this._eventListenersRegistered) {
            return;
        }

        // Drawer toggle
        if (this.drawerElement) {
            this.drawerElement.addEventListener('click', (e) => {
                e.stopPropagation();
                this._toggleDrawer();
            });

            this.drawerElement.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    this._toggleDrawer();
                } else if (e.key === 'Escape') {
                    this._closeDrawer();
                }
            });
        }

        // Tab switching
        const tabs = document.querySelectorAll('.llm-model-drawer__tab');
        tabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                e.stopPropagation();
                const tabName = tab.dataset.tab;
                this._switchTab(tabName);
            });
        });

        // Close drawer when clicking outside
        document.addEventListener('click', () => {
            this._closeDrawer();
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

    _toggleDrawer() {
        if (!this.drawerElement) {
            return;
        }
        const isOpen = this.drawerElement.classList.contains('open');

        this._closeDrawer();

        if (!isOpen) {
            this.drawerElement.classList.add('open');
        }
    }

    _closeDrawer() {
        if (this.drawerElement) {
            this.drawerElement.classList.remove('open');
        }
    }

    _switchTab(tabName) {
        this.activeTab = tabName;

        // Update tab active states
        const tabs = document.querySelectorAll('.llm-model-drawer__tab');
        tabs.forEach(tab => {
            if (tab.dataset.tab === tabName) {
                tab.classList.add('active');
            } else {
                tab.classList.remove('active');
            }
        });

        // Update tab content visibility
        const tabContents = document.querySelectorAll('.llm-model-drawer__tab-content');
        tabContents.forEach(content => {
            if (content.id === `llm-model-tab-${tabName}`) {
                content.classList.add('active');
            } else {
                content.classList.remove('active');
            }
        });
    }

    handleConfigReceived(data) {
        this.providerModels = data.provider_models || {};

        this.defaultPrimaryModel = data.default_primary_model || '';
        this.defaultAssistantModel = data.default_assistant_model || '';
        this.defaultLiteModel = data.default_lite_model || '';

        if (!this.selectedPrimaryModel) {
            this.selectedPrimaryModel = this.defaultPrimaryModel;
        }
        if (!this.selectedAssistantModel) {
            this.selectedAssistantModel = this.defaultAssistantModel;
        }
        if (!this.selectedLiteModel) {
            this.selectedLiteModel = this.defaultLiteModel;
        }

        this._populateDropdowns();
        this._updateDrawerText();
    }

    _populateDropdowns() {
        this._populateGrouped('primary', this.selectedPrimaryModel);
        this._populateGrouped('assistant', this.selectedAssistantModel);
        this._populateGrouped('lite', this.selectedLiteModel);
    }

    _populateGrouped(role, selectedValue) {
        const menuElement = role === 'primary' ? this.primaryMenuElement : role === 'assistant' ? this.assistantMenuElement : this.liteMenuElement;
        const modelMap = role === 'primary' ? this.primaryModelMap : role === 'assistant' ? this.assistantModelMap : this.liteModelMap;

        if (!menuElement) return;
        menuElement.innerHTML = '';
        modelMap.clear();

        const hasModels = Object.keys(this.providerModels).length > 0;

        if (!hasModels) {
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
                option.textContent = `${model.label || model.id} (${model.id})`;
                option.dataset.value = model.id;
                option.dataset.provider = provider;

                if (model.id === selectedValue) {
                    option.classList.add('selected');
                }

                if (!firstOption) {
                    firstOption = model;
                }

                // Store mapping for provider lookup
                modelMap.set(model.id, provider);

                option.addEventListener('click', (e) => {
                    e.stopPropagation();
                    this._selectModel(role, model.id, provider, `${model.label || model.id} (${model.id})`);
                });

                menuElement.appendChild(option);
            }
        }

        if (menuElement.children.length === 0) {
            throw new Error(`[LlmModelManager] No ${role} models available from any configured provider`);
        }

        if (!selectedValue) {
            throw new Error(`[LlmModelManager] No ${role} model configured in settings`);
        }

        if (!modelMap.has(selectedValue)) {
            throw new Error(`[LlmModelManager] Configured ${role} model '${selectedValue}' is not a valid ${role}-tier model for any configured provider`);
        }
    }

    _selectModel(role, modelId, provider, label) {
        if (role === 'primary') {
            this.selectedPrimaryModel = modelId;
        } else if (role === 'assistant') {
            this.selectedAssistantModel = modelId;
        } else {
            this.selectedLiteModel = modelId;
        }

        // Update selected state in UI
        this._updateSelectedState(role, modelId);

        // Update drawer text to show currently selected models
        this._updateDrawerText();

        // Close drawer
        this._closeDrawer();
    }

    _updateDrawerText() {
        if (!this.drawerTextElement) return;

        const primaryLabel = this._getModelLabel('primary', this.selectedPrimaryModel);
        this.drawerTextElement.textContent = primaryLabel;
    }

    _getModelLabel(role, modelId) {
        const modelMap = role === 'primary' ? this.primaryModelMap : role === 'assistant' ? this.assistantModelMap : this.liteModelMap;
        const provider = modelMap.get(modelId);
        if (provider && this.providerModels[provider]) {
            const models = this.providerModels[provider][role] || [];
            const model = models.find(m => m.id === modelId);
            if (model) {
                return `${model.label || model.id} (${model.id})`;
            }
        }
        return modelId || 'Not set';
    }

    _updateSelectedState(role, selectedValue) {
        const menuElement = role === 'primary' ? this.primaryMenuElement : role === 'assistant' ? this.assistantMenuElement : this.liteMenuElement;
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
        const modelMap = role === 'primary' ? this.primaryModelMap : role === 'assistant' ? this.assistantModelMap : this.liteModelMap;
        return modelMap.get(modelId) || '';
    }

    handleCaseSwitched(data) {
        const savedPrimary = data?.investigation?.llm_primary_model;
        const savedAssistant = data?.investigation?.llm_assistant_model;
        const savedLite = data?.investigation?.llm_lite_model;

        this.selectedPrimaryModel = savedPrimary || this.defaultPrimaryModel;
        this.selectedAssistantModel = savedAssistant || this.defaultAssistantModel;
        this.selectedLiteModel = savedLite || this.defaultLiteModel;
        this._syncDropdowns();
    }

    handleCaseCleared() {
        this.selectedPrimaryModel = this.defaultPrimaryModel;
        this.selectedAssistantModel = this.defaultAssistantModel;
        this.selectedLiteModel = this.defaultLiteModel;
        this._syncDropdowns();
    }

    _syncDropdowns() {
        // Re-populate to update selected state
        this._populateDropdowns();
        // Update drawer text to reflect current selections
        this._updateDrawerText();
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

    getLiteModel() {
        return this.selectedLiteModel || '';
    }

    getLiteProvider() {
        return this._findProviderForModel('lite', this.selectedLiteModel);
    }

    destroy() {
        this.drawerElement = null;
        this.drawerTextElement = null;
        this.primaryMenuElement = null;
        this.assistantMenuElement = null;
        this.liteMenuElement = null;
        this.primaryModelMap.clear();
        this.assistantModelMap.clear();
        this.liteModelMap.clear();
        this._eventListenersRegistered = false;
    }
}
