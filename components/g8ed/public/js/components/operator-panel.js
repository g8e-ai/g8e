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
import { devLogger } from '../utils/dev-logger.js';
import { OperatorStatus } from '../constants/operator-constants.js';
import { templateLoader } from '../utils/template-loader.js';
import { notificationService } from '../utils/notification-service.js';
import { operatorSessionService } from '../utils/operator-session-service.js';
import { escapeHtml } from '../utils/html.js';
import { OperatorDownloadMixin } from './operator-download-mixin.js';
import { OperatorDeviceLinkMixin } from './operator-device-link-mixin.js';
import { BindOperatorsMixin } from './operator-bind-mixin.js';
import { OperatorDeviceAuthMixin } from './operator-device-auth-mixin.js';
import { OperatorLayoutMixin } from './operator-layout-mixin.js';
import { OperatorListMixin } from './operator-list-mixin.js';
import { OperatorMetricsDisplayMixin } from './operator-metrics-display-mixin.js';

const _STATUS_UPDATED_VALUES = [
    EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
    EventType.OPERATOR_STATUS_UPDATED_AVAILABLE,
    EventType.OPERATOR_STATUS_UPDATED_UNAVAILABLE,
    EventType.OPERATOR_STATUS_UPDATED_BOUND,
    EventType.OPERATOR_STATUS_UPDATED_OFFLINE,
    EventType.OPERATOR_STATUS_UPDATED_STALE,
    EventType.OPERATOR_STATUS_UPDATED_STOPPED,
    EventType.OPERATOR_STATUS_UPDATED_TERMINATED,
];

const _OFFLINE_STATUSES = new Set([
    OperatorStatus.OFFLINE,
    OperatorStatus.STOPPED,
    OperatorStatus.STALE,
]);

const _ACTIVE_STATUSES = new Set([
    OperatorStatus.ACTIVE,
    OperatorStatus.BOUND,
]);

/**
 * OperatorPanel - Operator instrument panel DOM component.
 *
 * Owns panel DOM lifecycle, operator wire-event state aggregation, user
 * interaction, and visual updates.  Subscribes directly to canonical wire
 * events — no intermediate aggregation layer.
 * All domain-specific behaviour is implemented in the mixin modules applied below.
 */
export class OperatorPanel {
    constructor(eventBus) {
        this.eventBus = eventBus;

        // Operator state (previously in OperatorSSEHandler)
        this._operators = [];
        this._totalOperatorCount = 0;
        this._activeOperatorCount = 0;
        this._usedSlots = 0;
        this._maxSlots = 1;
        this._isConnected = false;
        this._lastHeartbeat = null;

        // Component state
        this.isCollapsed = false;
        this._isRendered = false;
        this._pendingRender = null;

        // DOM elements — populated in render()
        this.panelContainer = null;

        // Bound handlers for cleanup
        this.boundHandlers = {};

        this._panelApiKey = null;
        this._panelCommand = null;

        // Device authorization pending requests map (operator_id -> {token, timeout})
        this._pendingAuthRequests = new Map();

        this._wireHandlers = null;
        this._setupWireListeners();
    }

    async init() {
        await this.preloadTemplates();
        await this.render();
        this._isRendered = true;
        this.bindEvents();
        this.setupThemeListener();
        this._setupAuthStateListener();
        if (this._pendingRender) {
            this._applyOperatorState(this._pendingRender);
            this._pendingRender = null;
        }
        this.eventBus.emit(EventType.AUTH_COMPONENT_INITIALIZED_OPERATOR, {
            isAuthenticated: true
        });
    }

    _setupWireListeners() {
        this._wireHandlers = {
            onListUpdated:   (data) => this._onListUpdated(data),
            onStatusUpdated: (data) => this._onStatusUpdated(data),
            onHeartbeat:     (data) => this._onHeartbeat(data),
        };

        this.eventBus.on(EventType.OPERATOR_PANEL_LIST_UPDATED, this._wireHandlers.onListUpdated);
        this.eventBus.on(EventType.OPERATOR_HEARTBEAT_RECEIVED, this._wireHandlers.onHeartbeat);

        for (const eventType of _STATUS_UPDATED_VALUES) {
            this.eventBus.on(eventType, this._wireHandlers.onStatusUpdated);
        }
    }

    _onListUpdated(data) {
        this._operators = data.operators || [];
        this._totalOperatorCount = data.total_count || 0;
        this._activeOperatorCount = data.active_count || 0;
        this._usedSlots = data.used_slots || 0;
        this._maxSlots = data.max_slots ?? 1;
        operatorSessionService.setBoundOperators(this._operators);
        devLogger.log('[OPERATOR-PANEL] List updated:', this._totalOperatorCount, 'total,', this._activeOperatorCount, 'active');
        this._applyOperatorState({ cause: 'list_updated' });
    }

    _onStatusUpdated(data) {
        if (data.total_count !== undefined) {
            this._totalOperatorCount = data.total_count;
            this._activeOperatorCount = data.active_count || 0;
        }

        operatorSessionService.setBoundOperators(this._operators);
        devLogger.log('[OPERATOR-PANEL] Status updated:', data.operator_id, data.status);
        this._applyOperatorState({ cause: 'status_updated' });
    }

    _onHeartbeat(data) {
        const authState = window.authState?.getState();
        if (!authState?.isAuthenticated) return;

        this._lastHeartbeat = data.timestamp ? new Date(data.timestamp).getTime() : Date.now();
        this._isConnected = true;
        devLogger.log('[OPERATOR-PANEL] Heartbeat:', data.operator_id);
        this._applyOperatorState({ cause: 'heartbeat' });
    }

    _applyOperatorState({ cause }) {
        if (!this._isRendered) {
            this._pendingRender = { cause };
            return;
        }

        this.operators           = this._operators;
        this.totalOperatorCount  = this._totalOperatorCount;
        this.activeOperatorCount = this._activeOperatorCount;
        this.usedSlots           = this._usedSlots;
        this.maxSlots            = this._maxSlots;
        this.isConnected         = this._isConnected;
        this.lastHeartbeat       = this._lastHeartbeat;

        if (cause === 'heartbeat' && this._isConnected) {
            const heartbeatData = this.operators.find(op => op.operator_id === this.selectedMetricsOperatorId);
            if (heartbeatData) {
                this.updateMetrics(heartbeatData);
                this.updateStatus(heartbeatData.status || OperatorStatus.ACTIVE);
            }
            if (this.selectedMetricsOperatorId) {
                this.updateOperatorCardInPlace(this.selectedMetricsOperatorId);
            }
            return;
        }

        if (cause === 'status_updated') {
            const offlineStatuses = [OperatorStatus.OFFLINE, OperatorStatus.STOPPED, OperatorStatus.STALE, OperatorStatus.AVAILABLE];
            const selectedOp = this.operators.find(op => op.operator_id === this.selectedMetricsOperatorId);
            if (selectedOp && offlineStatuses.includes(selectedOp.status)) {
                this.clearPanelMetrics();
            }
        }

        this.updatePanelStatusFromOperatorCounts();
        this.displayOperators(this.operators);
        this.updateBindAllButtonVisibility();
        this.updateUnbindAllButtonVisibility();
    }

    async preloadTemplates() {
        try {
            await templateLoader.preload([
                'operator-panel-container',
                'operator-platform-selection',
                'operator-initial-download-overlay',
                'operator-download-layer',
                'operator-item',
                'operator-pagination',
                'bind-single-confirmation-overlay',
                'bind-all-operator-item',
                'bind-result-feedback',
                'approval-card',
                'approval-card-restored',
                'approval-status',
                'command-result',
                'executing-indicator',
                'preparing-indicator',
                'results-toggle',
                'activity-indicator',
                'tribunal'
            ]);
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to preload templates:', error);
        }
    }

    async render() {
        const container = document.getElementById('operator-panel-container');
        if (!container) {
            devLogger.error('[OPERATOR] Container not found: operator-panel-container');
            return;
        }

        const template = templateLoader.cache.get('operator-panel-container');
        container.innerHTML = template;

        // Core panel references
        this.panelContainer = document.getElementById('operator-panel-container');

        // Expanded details elements
        this.metricsDetails = document.getElementById('operator-metrics-details');
        this.detailHostElement = document.getElementById('operator-detail-host');
        this.osElement = document.getElementById('operator-os');
        this.kernelElement = document.getElementById('operator-kernel');
        this.archElement = document.getElementById('operator-arch');
        this.uptimeElement = document.getElementById('operator-uptime');
        this.usernameElement = document.getElementById('operator-username');
        this.shellElement = document.getElementById('operator-shell');
        this.homeElement = document.getElementById('operator-home');
        this.timezoneElement = document.getElementById('operator-timezone');
        this.diskUsageElement = document.getElementById('operator-disk-usage');
        this.diskVisualElement = document.getElementById('operator-disk-visual');
        this.memoryUsageElement = document.getElementById('operator-memory-usage');
        this.memoryVisualElement = document.getElementById('operator-memory-visual');
        this.publicIpElement = document.getElementById('operator-public-ip');
        this.ipVisibilityToggle = document.getElementById('ip-visibility-toggle');
        this.latencyDetailElement = document.getElementById('operator-latency-detail');
        this.pwdElement = document.getElementById('operator-pwd');
        this.langElement = document.getElementById('operator-lang');

        // IP obfuscation state
        this.ipVisible = false;
        this.actualPublicIp = null;
        if (this.ipVisibilityToggle) {
            this.ipVisibilityToggle.addEventListener('click', () => this._toggleIpVisibility());
        }

        this.metricsDetailsExpanded = false;

        this.operatorListCollapsible = document.getElementById('operator-list-collapsible');
        this.operatorListCollapsibleBar = document.getElementById('operator-list-collapsible-bar');
        this.operatorListBarTitle = document.getElementById('operator-list-bar-title');
        this.operatorListBarChevron = document.getElementById('operator-list-bar-chevron');
        this.operatorList = document.getElementById('operator-list');
        this.drawerFooter = document.getElementById('operator-drawer-footer');
        this.bindAllBtn = document.getElementById('bind-all-btn');
        this.unbindAllBtn = document.getElementById('unbind-all-btn');

        devLogger.log('[OPERATOR] DOM references set - operatorList:', !!this.operatorList, 'drawerFooter:', !!this.drawerFooter, 'bindAllBtn:', !!this.bindAllBtn, 'unbindAllBtn:', !!this.unbindAllBtn);

        // Download section
        this.downloadCollapsible = document.getElementById('operator-download-collapsible');
        this.downloadCollapsibleBar = document.getElementById('operator-download-collapsible-bar');
        this.downloadCollapsibleContent = document.getElementById('operator-download-collapsible-content');
        this.downloadSectionExpanded = true;
        this.downloadSectionPopulated = false;

        // Operator list collapsible section
        this.operatorListSectionExpanded = true;

        // Operator list state
        this.operators = [];
        this.boundOperatorIds = [];
        this.selectedMetricsOperatorId = null;
        this.bindAllOverlay = null;
        this.unbindAllOverlay = null;

        // Counts for dynamic title
        this.totalOperatorCount = 0;
        this.activeOperatorCount = 0;
        this.usedSlots = 0;
        this.maxSlots = 1;

        // Pagination state
        this.currentPage = 1;
        this.operatorsPerPage = 20;

        // Download overlay stack state
        this.downloadMenuStack = [];
        this.currentOS = null;
        this.currentPlatform = null;

        // Platform options for binary download
        this.platformOptions = {
            mac: [
                { arch: 'amd64', name: 'macOS Intel', sub: 'x64', icon: 'computer' },
                { arch: 'arm64', name: 'macOS Apple Silicon', sub: 'M1/M2/M3', icon: 'chip' }
            ],
            linux: [
                { arch: 'amd64', name: 'Linux x64', sub: '64-bit', icon: 'terminal' },
                { arch: 'arm64', name: 'Linux ARM64', sub: '64-bit', icon: 'developer_board' },
                { arch: '386', name: 'Linux x86', sub: '32-bit', icon: 'memory' }
            ]
        };

        this._initPanelResize();

        if (this.downloadSectionExpanded) {
            this.expandDownloadSection();
        }

        if (this.operatorListSectionExpanded && this.operatorListCollapsible) {
            this.operatorListCollapsible.classList.add('expanded');
        }
    }

    bindEvents() {
        const containScroll = (el) => {
            el.addEventListener('wheel', (e) => {
                const hasOverflow = el.scrollHeight > el.clientHeight;
                if (!hasOverflow) return;
                const atTop = el.scrollTop <= 0;
                const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight;
                const scrollingUp = e.deltaY < 0;
                const scrollingDown = e.deltaY > 0;
                if ((scrollingUp && atTop) || (scrollingDown && atBottom)) {
                    e.preventDefault();
                }
                e.stopPropagation();
            }, { passive: false });
        };

        if (this.downloadCollapsibleContent) containScroll(this.downloadCollapsibleContent);
        if (this.drawerList) containScroll(this.drawerList);

        if (this.downloadCollapsibleBar) {
            this.downloadCollapsibleBar.addEventListener('click', (e) => {
                e.stopPropagation();
                this.toggleDownloadSection();
            });
        }

        if (this.operatorListCollapsibleBar) {
            this.operatorListCollapsibleBar.addEventListener('click', (e) => {
                e.stopPropagation();
                this.toggleOperatorListSection();
            });
        }

        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                if (this.downloadMenuStack.length > 0) {
                    this.closeCurrentOverlay();
                    return;
                }
                if (this.downloadSectionExpanded) {
                    this.collapseDownloadSection();
                    return;
                }
                if (this.instructionsModalOpen) {
                    this.hideInstructionsModal();
                }
            }
        });

        if (this.bindAllBtn) {
            this.bindAllBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.showBindAllConfirmationOverlay();
            });
        }

        if (this.unbindAllBtn) {
            this.unbindAllBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.showUnbindAllConfirmationOverlay();
            });
        }
    }

    _setupAuthStateListener() {
        this.authStateUnsubscribe = window.authState.subscribe((event, data) => {
            switch (event) {
                case EventType.AUTH_USER_AUTHENTICATED:
                    this.webSessionModel = data.webSessionModel || window.authState.getWebSessionModel();
                    this.populateApiKey();
                    this.displayInitialOperatorStatus();
                    break;
                case EventType.AUTH_USER_UNAUTHENTICATED:
                    this.webSessionModel = null;
                    this.clearOperatorData();
                    break;
            }
        });
    }

    clearOperatorData() {
        this.updateStatus(OperatorStatus.OFFLINE);
    }

    toggleOperatorListSection() {
        this.operatorListSectionExpanded = !this.operatorListSectionExpanded;
        if (this.operatorListCollapsible) {
            if (this.operatorListSectionExpanded) {
                this.operatorListCollapsible.classList.add('expanded');
                this.operatorListCollapsible.classList.remove('collapsed');
            } else {
                this.operatorListCollapsible.classList.remove('expanded');
                this.operatorListCollapsible.classList.add('collapsed');
            }
        }
        devLogger.log('[OPERATOR] Operator list section toggled:', this.operatorListSectionExpanded);
    }

    updateOperatorListBarTitle() {
        if (!this.operatorListBarTitle) return;
        const boundCount = this.boundOperatorIds.length;
        this.operatorListBarTitle.textContent = `(${boundCount}) Operators Bound`;
    }

    displayInitialOperatorStatus() {
        if (!this.webSessionModel) return;

        const operatorId = this.webSessionModel.operator_id;
        const operatorStatus = this.webSessionModel.operator_status;

        if (operatorStatus && operatorId) {
            devLogger.log(`[OPERATOR] Displaying initial status: ${operatorStatus}`);
            this.updateStatus(operatorStatus);
        } else if (operatorStatus) {
            devLogger.log(`[OPERATOR] Displaying initial status (no Operator in cache): ${operatorStatus}`);
            this.updateStatus(operatorStatus);
        } else {
            devLogger.log('[OPERATOR] Operator ID present but no status, showing as offline');
            this.updateStatus(OperatorStatus.OFFLINE);
        }
    }

    setupThemeListener() {
        if (window.ThemeManager) {
            this.themeUnsubscribe = window.ThemeManager.onChange(() => this.updatePlatformIcons());
        } else {
            const observer = new MutationObserver(() => this.updatePlatformIcons());
            observer.observe(document.body, { attributes: true, attributeFilter: ['data-theme'] });
            this.themeObserver = observer;
        }
        this.updatePlatformIcons();
    }

    addSystemMessage(message, type = 'info') {
        notificationService.show(message, type);
    }

    destroy() {
        if (this._wireHandlers) {
            this.eventBus.off(EventType.OPERATOR_PANEL_LIST_UPDATED, this._wireHandlers.onListUpdated);
            this.eventBus.off(EventType.OPERATOR_HEARTBEAT_RECEIVED, this._wireHandlers.onHeartbeat);
            for (const eventType of _STATUS_UPDATED_VALUES) {
                this.eventBus.off(eventType, this._wireHandlers.onStatusUpdated);
            }
            this._wireHandlers = null;
        }

        if (this.authStateUnsubscribe) {
            this.authStateUnsubscribe();
            this.authStateUnsubscribe = null;
        }

        if (this.themeObserver) {
            this.themeObserver.disconnect();
            this.themeObserver = null;
        }

        this.panelContainer = null;
    }
}

Object.assign(OperatorPanel.prototype, OperatorDownloadMixin);
Object.assign(OperatorPanel.prototype, OperatorDeviceLinkMixin);
Object.assign(OperatorPanel.prototype, BindOperatorsMixin);
Object.assign(OperatorPanel.prototype, OperatorDeviceAuthMixin);
Object.assign(OperatorPanel.prototype, OperatorLayoutMixin);
Object.assign(OperatorPanel.prototype, OperatorListMixin);
Object.assign(OperatorPanel.prototype, OperatorMetricsDisplayMixin);
