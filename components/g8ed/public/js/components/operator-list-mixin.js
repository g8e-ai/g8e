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

import { devLogger } from '../utils/dev-logger.js';
import { OperatorStatus } from '../constants/operator-constants.js';
import { templateLoader } from '../utils/template-loader.js';
import { timeAgo, parseISOString, formatForDisplay } from '../utils/timestamp.js';
import { operatorPanelService } from '../utils/operator-panel-service.js';
import { webSessionService } from '../utils/web-session-service.js';
import { OperatorDialogs, OperatorAlerts } from '../constants/operator-messages.js';
import { notificationService } from '../utils/notification-service.js';
import { showConfirmationModal } from '../utils/ui-utils.js';

/**
 * OperatorListMixin - Operator list rendering, pagination, and card management.
 *
 * Covers: displayOperators, renderPaginationControls, updateOperatorCardInPlace,
 * metrics-source selection, and operator API actions (copy key, refresh key,
 * generate device link, stop).
 *
 * Mixed into OperatorPanel via Object.assign(OperatorPanel.prototype, OperatorListMixin).
 */
export const OperatorListMixin = {

    displayOperators(operators) {
        if (!this.operatorList) {
            devLogger.warn('[OPERATOR] displayOperators called but operatorList is null');
            return;
        }

        const loadingEl = document.getElementById('operator-drawer-loading');
        if (loadingEl) loadingEl.classList.add('initially-hidden');

        this.boundOperatorIds = [];

        const previousSelection = this.selectedMetricsOperatorId;
        const currentWebSessionId = webSessionService.getWebSessionId();

        devLogger.log(`[OPERATOR] Current web session ID: ${currentWebSessionId}`);
        devLogger.log(`[OPERATOR] Operators to render: ${operators.length}`);

        const statusPriority = (op) => {
            if (op.is_g8ep) return 0;
            const isBoundToMe = op.status === OperatorStatus.BOUND && op.web_session_id === currentWebSessionId;
            const isBoundElsewhere = op.status === OperatorStatus.BOUND && !isBoundToMe;
            if (isBoundToMe) return 1;
            if (isBoundElsewhere) return 2;
            if (op.status === OperatorStatus.ACTIVE) return 3;
            if (op.status === OperatorStatus.STALE) return 4;
            return 5;
        };

        const sortedOperators = [...operators].sort((a, b) => {
            const aPriority = statusPriority(a);
            const bPriority = statusPriority(b);
            if (aPriority !== bPriority) return aPriority - bPriority;
            const aName = (a.name || '').toLowerCase();
            const bName = (b.name || '').toLowerCase();
            return aName.localeCompare(bName);
        });

        const totalPages = Math.ceil(sortedOperators.length / this.operatorsPerPage);
        const startIndex = (this.currentPage - 1) * this.operatorsPerPage;
        const endIndex = startIndex + this.operatorsPerPage;
        const paginatedOperators = sortedOperators.slice(startIndex, endIndex);

        const fragment = document.createDocumentFragment();

        paginatedOperators.forEach(operator => {
            const item = document.createElement('div');
            item.className = 'operator-list-item';
            item.setAttribute('data-operator-id', operator.operator_id);

            const latestSnapshot = operator.latest_heartbeat_snapshot || {};
            const systemInfo = operator.system_info || {};
            const operatorName = operator.name || 'Unknown';
            const hostnameFull = operatorName === 'g8e' ? 'g8ep' : (systemInfo.hostname || ' - ');

            const isBoundToMe = operator.status === OperatorStatus.BOUND && operator.web_session_id === currentWebSessionId;
            const isBoundElsewhere = operator.status === OperatorStatus.BOUND && !isBoundToMe;

            if (isBoundToMe) {
                item.classList.add('bound');
                item.classList.add('expanded');
                if (!this.boundOperatorIds.includes(operator.operator_id)) {
                    this.boundOperatorIds.push(operator.operator_id);
                }
            } else if (isBoundElsewhere) {
                item.classList.add('bound-elsewhere');
            }

            const statusDisplay = operator.status_display || operator.status || OperatorStatus.OFFLINE;
            const statusClass = operator.status_class || 'inactive';
            const isStoppable = [OperatorStatus.ACTIVE, OperatorStatus.BOUND, OperatorStatus.STALE].includes(operator.status);
            const hasName = !!operator.name;

            const formatTimestamp = (timestamp) => {
                if (!timestamp) return ' - ';
                const date = parseISOString(timestamp);
                const diffMs = Date.now() - date.getTime();
                if (diffMs < 7 * 24 * 60 * 60 * 1000) return timeAgo(date);
                return formatForDisplay(date);
            };

            const firstDeployedText = operator.first_deployed ? formatTimestamp(operator.first_deployed) : ' - ';
            const lastHeartbeatText = operator.last_heartbeat ? formatTimestamp(operator.last_heartbeat) : ' - ';

            item.classList.add(statusClass);

            const canBind = operator.status === OperatorStatus.ACTIVE;
            const isStale = operator.status === OperatorStatus.STALE;
            const canUnbind = isBoundToMe || isStale;

            let bindBtnTitle, bindBtnIcon, bindBtnDisabled;
            if (isBoundToMe) {
                bindBtnTitle = 'Unbind from WebSession';
                bindBtnIcon = 'link_off';
                bindBtnDisabled = false;
            } else if (isStale) {
                bindBtnTitle = 'Unbind Stale Operator';
                bindBtnIcon = 'link_off';
                bindBtnDisabled = false;
            } else if (canBind) {
                bindBtnTitle = 'Bind to WebSession';
                bindBtnIcon = 'link';
                bindBtnDisabled = false;
            } else {
                bindBtnTitle = 'Bind to WebSession (operator offline)';
                bindBtnIcon = 'link_off';
                bindBtnDisabled = true;
            }

            const actionsHtml = `
                <div class="operator-actions-inline">
                    <button class="operator-action-btn device-link-btn" title="Get Device Link Token" data-operator-id="${operator.operator_id}">
                        <span class="material-symbols-outlined">dns</span>
                    </button>
                    <button class="operator-action-btn g8ep-reauth-btn" title="Restart g8ep Operator" data-operator-id="${operator.operator_id}">
                        <span class="material-symbols-outlined">restart_alt</span>
                    </button>
                    <button class="operator-action-btn api-key-btn" title="Copy API Key" data-operator-id="${operator.operator_id}">
                        <span class="material-symbols-outlined">vpn_key</span>
                    </button>
                    <button class="operator-action-btn refresh-key-btn" title="Refresh API Key" data-operator-id="${operator.operator_id}">
                        <span class="material-symbols-outlined">key_off</span>
                    </button>
                    <button class="operator-action-btn bind-operator-btn" title="${bindBtnTitle}" data-operator-id="${operator.operator_id}" data-is-bound="${isBoundToMe}" data-is-stale="${isStale}"${bindBtnDisabled ? ' disabled' : ''}>
                        <span class="material-symbols-outlined">${bindBtnIcon}</span>
                    </button>
                    <button class="operator-action-btn stop-btn" title="${isStoppable ? 'Stop Operator' : 'Stop Operator (not running)'}" data-operator-id="${operator.operator_id}"${!isStoppable ? ' disabled' : ''}>
                        <span class="material-symbols-outlined">stop_circle</span>
                    </button>
                </div>
            `;

            const operatorTypeIcon = 'terminal';
            const operatorTypeClass = 'binary-operator';
            const operatorTypeTitle = 'Operator';

            const isAvailable = operator.status === OperatorStatus.AVAILABLE;
            const hostnameDisplay = isAvailable ? 'Available' : hostnameFull;
            const hostnameClass = isAvailable ? 'text-accent-green' : '';

            const template = templateLoader.cache.get('operator-item');
            item.innerHTML = templateLoader.replace(template, {
                hostnameFull: hostnameDisplay,
                hostnameClass,
                actionsHtml,
                statusClass,
                statusDisplay,
                firstDeployedText,
                lastHeartbeatText,
                operatorName,
                nameClass: !hasName ? 'not-deployed' : '',
                operatorTypeIcon,
                operatorTypeClass,
                operatorTypeTitle
            });

            const toggleBtn = item.querySelector('.operator-toggle-btn');
            if (toggleBtn) {
                toggleBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    item.classList.toggle('expanded');
                });
            }

            const bindBtn = item.querySelector('.bind-operator-btn');
            if (bindBtn) {
                bindBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    if (bindBtn.disabled) return;
                    const operatorId = bindBtn.getAttribute('data-operator-id');
                    const isBoundState = bindBtn.getAttribute('data-is-bound') === 'true';
                    const isStaleState = bindBtn.getAttribute('data-is-stale') === 'true';
                    if (isBoundState || isStaleState) {
                        await this.unbindOperatorWithConfirmation(operatorId, isStaleState);
                    } else {
                        await this.bindOperatorWithConfirmation(operatorId);
                    }
                });
            }

            const apiKeyBtn = item.querySelector('.api-key-btn');
            if (apiKeyBtn) {
                apiKeyBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    await this.copyOperatorApiKey(apiKeyBtn.getAttribute('data-operator-id'), apiKeyBtn);
                });
            }

            const refreshKeyBtn = item.querySelector('.refresh-key-btn');
            if (refreshKeyBtn) {
                refreshKeyBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    await this.refreshOperatorApiKey(refreshKeyBtn.getAttribute('data-operator-id'));
                });
            }

            const deviceLinkBtn = item.querySelector('.device-link-btn');
            if (deviceLinkBtn) {
                deviceLinkBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    await this.generateDeviceLink(deviceLinkBtn.getAttribute('data-operator-id'), deviceLinkBtn);
                });
            }

            const g8eNodeReauthBtn = item.querySelector('.g8ep-reauth-btn');
            if (g8eNodeReauthBtn) {
                g8eNodeReauthBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    await this.restartG8ENodeOperator(g8eNodeReauthBtn);
                });
            }

            const stopBtn = item.querySelector('.stop-btn');
            if (stopBtn) {
                stopBtn.addEventListener('click', async (e) => {
                    e.stopPropagation();
                    if (stopBtn.disabled) return;
                    await this.stopOperator(stopBtn.getAttribute('data-operator-id'));
                });
            }

            item.addEventListener('click', () => {
                this._selectMetricsOperator(operator.operator_id);
            });

            fragment.appendChild(item);
        });

        this.operatorList.replaceChildren(fragment);

        this._applyDefaultMetricsSelection(sortedOperators, previousSelection);

        const startSlot = paginatedOperators.length > 0 ? startIndex + 1 : 0;
        const endSlot = startIndex + paginatedOperators.length;
        this.renderPaginationControls(sortedOperators.length, startSlot, endSlot);

        this.updateBindAllButtonVisibility();
        this.updateUnbindAllButtonVisibility();
    },

    renderPaginationControls(totalOperators, startSlot = 1, endSlot = 0) {
        if (!this.drawerFooter) return;

        const totalPages = totalOperators > 0 ? Math.ceil(totalOperators / this.operatorsPerPage) : 1;
        this.drawerFooter.classList.remove('initially-hidden');

        const template = templateLoader.cache.get('operator-pagination');
        if (!template) {
            devLogger.error('[OPERATOR] operator-pagination template not loaded — preload incomplete');
            return;
        }

        this.drawerFooter.innerHTML = templateLoader.replace(template, {
            startSlot,
            endSlot,
            maxSlots: this.maxSlots || 0,
            currentPage: this.currentPage,
            totalPages,
            prevDisabled: this.currentPage === 1 ? 'disabled' : '',
            nextDisabled: this.currentPage === totalPages ? 'disabled' : ''
        });

        const prevBtn = this.drawerFooter.querySelector('.prev-btn');
        const nextBtn = this.drawerFooter.querySelector('.next-btn');

        if (prevBtn) {
            prevBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                if (this.currentPage > 1) {
                    this.currentPage--;
                    this.displayOperators(this.operators);
                }
            });
        }

        if (nextBtn) {
            nextBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                if (this.currentPage < totalPages) {
                    this.currentPage++;
                    this.displayOperators(this.operators);
                }
            });
        }

        this.bindAllBtn = this.drawerFooter.querySelector('#bind-all-btn');
        if (this.bindAllBtn) {
            this.bindAllBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.showBindAllConfirmationOverlay();
            });
        }

        this.unbindAllBtn = this.drawerFooter.querySelector('#unbind-all-btn');
        if (this.unbindAllBtn) {
            this.unbindAllBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.showUnbindAllConfirmationOverlay();
            });
        }

        this.updateBindAllButtonVisibility();
        this.updateUnbindAllButtonVisibility();
    },

    async updateOperatorCardInPlace(operatorId) {
        if (!this.operatorList || !operatorId) return;

        try {
            const response = await operatorPanelService.getOperatorDetails(operatorId);

            if (!response.ok) {
                devLogger.warn('[OPERATOR] Failed to fetch Operator for card update:', operatorId);
                return;
            }

            const result = await response.json();
            const updatedOperator = result.data;
            if (!updatedOperator) return;

            const index = this.operators.findIndex(op => op.operator_id === operatorId);
            if (index !== -1) {
                this.operators[index] = updatedOperator;
            }

            const cardElement = this.operatorList.querySelector(`[data-operator-id="${operatorId}"]`);
            if (cardElement) {
                this.displayOperators(this.operators);
            }

            devLogger.log('[OPERATOR] Card updated in place for:', operatorId);
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to update Operator card:', error);
        }
    },

    _selectMetricsOperator(operatorId) {
        if (this.selectedMetricsOperatorId === operatorId) return;
        this.selectedMetricsOperatorId = operatorId;
        devLogger.log('[OPERATOR] Metrics source selected:', operatorId);

        if (this.operatorList) {
            this.operatorList.querySelectorAll('.operator-list-item').forEach(el => {
                el.classList.toggle('metrics-selected', el.getAttribute('data-operator-id') === operatorId);
            });
        }

        const operator = this.operators.find(op => op.operator_id === operatorId);
        if (operator) {
            this.updateMetrics({ operator_id: operatorId, ...operator });
            const status = operator.status || OperatorStatus.ACTIVE;
            this.updateStatus(status);
        }
    },

    _applyDefaultMetricsSelection(sortedOperators, previousSelection) {
        const currentWebSessionId = webSessionService.getWebSessionId();

        if (previousSelection && sortedOperators.some(op => op.operator_id === previousSelection)) {
            this._selectMetricsOperator(previousSelection);
            return;
        }

        const boundOps = sortedOperators
            .filter(op => op.status === OperatorStatus.BOUND && op.web_session_id === currentWebSessionId)
            .sort((a, b) => (a.name || a.operator_id).localeCompare(b.name || b.operator_id));

        if (boundOps.length > 0) {
            this._selectMetricsOperator(boundOps[0].operator_id);
            return;
        }

        const activeOps = sortedOperators
            .filter(op => op.status === OperatorStatus.ACTIVE)
            .sort((a, b) => (a.name || a.operator_id).localeCompare(b.name || b.operator_id));

        if (activeOps.length > 0) {
            this._selectMetricsOperator(activeOps[0].operator_id);
            return;
        }

        this.selectedMetricsOperatorId = null;
    },

    async copyOperatorApiKey(operatorId, btnElement) {
        try {
            devLogger.log('[OPERATOR] Fetching API key for operator:', operatorId);

            const response = await operatorPanelService.getOperatorApiKey(operatorId);

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to fetch API key');
            }

            const result = await response.json();
            const key = result.api_key;

            await navigator.clipboard.writeText(key);
            const icon = btnElement?.querySelector('.material-symbols-outlined');
            if (icon) {
                icon.textContent = 'check';
                btnElement.classList.add('copied');
                setTimeout(() => {
                    icon.textContent = 'vpn_key';
                    btnElement.classList.remove('copied');
                }, 2000);
            }
            devLogger.log('[OPERATOR] API key copied to clipboard');
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to copy API key:', error);
            notificationService.error(`Failed to copy API key: ${error.message}`);
        }
    },

    async refreshOperatorApiKey(operatorId) {
        try {
            const confirmed = await showConfirmationModal({
                title: 'Refresh API Key',
                message: OperatorDialogs.RESET_SLOT_CONFIRM,
                confirmLabel: 'Refresh',
                confirmIcon: 'key_off'
            });

            if (!confirmed) {
                devLogger.log('[OPERATOR] API key refresh cancelled by user');
                return;
            }

            devLogger.log('[OPERATOR] Refreshing API key for operator:', operatorId);

            const operator = this.operators.find(op => op.operator_id === operatorId);
            const isRunning = operator && [OperatorStatus.ACTIVE, OperatorStatus.BOUND, OperatorStatus.STALE].includes(operator.status);

            if (isRunning) {
                devLogger.log('[OPERATOR] Operator is running, sending stop command before refresh:', operatorId);
                try {
                    const stopResponse = await operatorPanelService.stopOperator(operatorId);
                    if (!stopResponse.ok) {
                        const stopError = await stopResponse.json();
                        devLogger.warn('[OPERATOR] Stop before refresh failed (continuing with refresh):', stopError);
                    } else {
                        devLogger.log('[OPERATOR] Stop command sent successfully before refresh');
                    }
                } catch (stopError) {
                    devLogger.warn('[OPERATOR] Stop before refresh error (continuing with refresh):', stopError);
                }
            }

            const response = await operatorPanelService.refreshOperatorApiKey(operatorId);

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to refresh API key');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] API key refreshed - old terminated, new created:', {
                old_operator_id: result.old_operator_id,
                new_operator_id: result.new_operator_id,
                slot_number: result.slot_number
            });

            this.operators = this.operators.filter(op => op.operator_id !== result.old_operator_id);
            this.displayOperators(this.operators);

            try {
                await navigator.clipboard.writeText(result.new_api_key);
                notificationService.success(OperatorAlerts.SLOT_RESET_KEY_COPIED(result.slot_number));
            } catch (clipboardError) {
                notificationService.info(OperatorAlerts.SLOT_RESET_KEY_DISPLAY(result.slot_number, result.new_api_key));
            }

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to refresh API key:', error);
            notificationService.error(`Failed to refresh API key: ${error.message}`);
        }
    },

    async generateDeviceLink(operatorId, buttonElement) {
        try {
            devLogger.log('[OPERATOR] Generating device link for operator:', operatorId);

            const response = await operatorPanelService.generateDeviceLink(operatorId);

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to generate device link');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] Device link generated (pre-authorized):', {
                token_prefix: result.token?.substring(0, 25) + '...',
                expires_at: result.expires_at
            });

            await navigator.clipboard.writeText(result.token);

            if (buttonElement) {
                const icon = buttonElement.querySelector('.material-symbols-outlined');
                const originalIcon = icon.textContent;
                icon.textContent = 'check';
                buttonElement.classList.add('copied');
                setTimeout(() => {
                    icon.textContent = originalIcon;
                    buttonElement.classList.remove('copied');
                }, 2000);
            }

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to generate device link:', error);
            notificationService.error(`Failed to generate device link: ${error.message}`);
        }
    },

    async restartG8ENodeOperator(buttonElement) {
        const icon = buttonElement?.querySelector('.material-symbols-outlined');
        const originalIcon = icon?.textContent;

        try {
            if (icon) {
                icon.textContent = 'sync';
                icon.classList.add('rotating');
                buttonElement.disabled = true;
            }

            devLogger.log('[OPERATOR] Restarting g8ep operator');

            const response = await operatorPanelService.g8eNodeReauth();

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to restart g8ep operator');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] g8ep operator restarted:', result);

            if (icon) {
                icon.textContent = 'check';
                icon.classList.remove('rotating');
                setTimeout(() => {
                    if (icon) icon.textContent = originalIcon;
                    if (buttonElement) buttonElement.disabled = false;
                }, 2000);
            }

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to restart g8ep operator:', error);
            if (icon) {
                icon.textContent = originalIcon;
                icon.classList.remove('rotating');
            }
            if (buttonElement) buttonElement.disabled = false;
            notificationService.error(`Failed to restart g8ep operator: ${error.message}`);
        }
    },

    async stopOperator(operatorId) {
        try {
            const operator = this.operators.find(op => op.operator_id === operatorId);
            const operatorName = operator?.name || 'Unknown';
            const hostname = operator?.system_info?.hostname || 'unknown host';

            const confirmed = await showConfirmationModal({
                title: 'Stop Operator',
                message: OperatorDialogs.STOP_OPERATOR_CONFIRM,
                confirmLabel: 'Stop',
                confirmIcon: 'stop_circle'
            });

            if (!confirmed) {
                devLogger.log('[OPERATOR] Stop operation cancelled by user');
                return;
            }

            devLogger.log('[OPERATOR] Stopping operator:', operatorId);

            const response = await operatorPanelService.stopOperator(operatorId);

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to stop operator');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] Stop command sent successfully:', result);

            notificationService.info(`${operatorName} on ${hostname} has been shutdown.`);

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to stop operator:', error);
            notificationService.error(`Failed to stop operator: ${error.message}`);
        }
    },

};

