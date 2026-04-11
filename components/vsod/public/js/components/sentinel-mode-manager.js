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

import { EventType } from '../constants/events.js';

export class SentinelModeManager {
    constructor(eventBus) {
        this.eventBus = eventBus;
        this.sentinelModeEnabled = true;
        this.currentInvestigationId = null;
        
        this.toggle = null;
        this.checkbox = null;
    }

    init() {
        if (this._initialized) {
            return;
        }
        this._initialized = true;

        this.setupDOMElements();
        this.setupEventListeners();
        this.updateToggleState();
    }

    setupDOMElements() {
        this.toggle = document.getElementById('sentinel-mode-toggle');
        this.checkbox = document.getElementById('sentinel-mode-checkbox');
        this.container = document.getElementById('sentinel-mode-container');
    }

    setupEventListeners() {
        if (this._eventListenersRegistered) {
            return;
        }

        if (this.checkbox) {
            this.checkbox.addEventListener('change', (e) => {
                this.handleToggleChange(e.target.checked);
            });
        }

        this.eventBus.on(EventType.CASE_SWITCHED, (data) => {
            this.handleCaseSwitched(data);
        });

        this.eventBus.on(EventType.CASE_CLEARED, () => {
            this.handleCaseCleared();
        });

        this.eventBus.on(EventType.INVESTIGATION_LOADED, (data) => {
            this.handleInvestigationLoaded(data);
        });

        this.eventBus.on(EventType.OPERATOR_BOUND, () => {
            this.handleOperatorBound();
        });

        this._eventListenersRegistered = true;
    }

    handleToggleChange(enabled) {
        this.sentinelModeEnabled = enabled;
        
        this.updateToggleState();
        
        this.eventBus.emit(EventType.PLATFORM_SENTINEL_MODE_CHANGED, {
            enabled: this.sentinelModeEnabled,
            investigationId: this.currentInvestigationId
        });
    }

    handleCaseSwitched(data) {
        this.currentInvestigationId = data?.investigationId || null;
        
        if (data?.investigation) {
            this.sentinelModeEnabled = data.investigation.sentinel_mode === true;
        }

        this.updateToggleState();
    }

    handleCaseCleared() {
        this.currentInvestigationId = null;
        this.sentinelModeEnabled = true;
        this.updateToggleState();
    }

    handleInvestigationLoaded(data) {
        if (data?.id) {
            this.sentinelModeEnabled = data.sentinel_mode === true;
            this.currentInvestigationId = data.id;
        } else {
            this.currentInvestigationId = null;
            this.sentinelModeEnabled = true;
        }
        
        this.updateToggleState();
    }

    handleOperatorBound() {
        this.updateToggleState();
    }

    updateToggleState() {
        if (!this.toggle || !this.checkbox) return;

        this.checkbox.checked = this.sentinelModeEnabled;
        this.toggle.classList.toggle('active', this.sentinelModeEnabled);

        this.updateTooltip();
    }

    updateTooltip() {
        if (!this.toggle) return;

        this.toggle.title = this.sentinelModeEnabled
            ? 'Sentinel Mode is ON. Data is scrubbed before storage and AI receives only redacted data.'
            : 'Sentinel Mode is OFF. AI sees full unredacted data.';
    }

    getSentinelMode() {
        return this.sentinelModeEnabled;
    }

    setSentinelMode(enabled) {
        this.sentinelModeEnabled = enabled;
        this.updateToggleState();
    }

    destroy() {
        this.toggle = null;
        this.checkbox = null;
        this.container = null;
        this._initialized = false;
        this._eventListenersRegistered = false;
    }
}
