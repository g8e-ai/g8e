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
import { notificationService } from '../utils/notification-service.js';
import { OperatorStatus } from '../constants/operator-constants.js';
import { templateLoader } from '../utils/template-loader.js';
import { EventType } from '../constants/events.js';
import { operatorPanelService } from '../utils/operator-panel-service.js';
import { escapeHtml } from '../utils/html.js';
import { showConfirmationModal } from '../utils/ui-utils.js';

/**
 * BindOperatorsMixin - Operator bind/unbind operations and confirmation overlays.
 *
 * Covers: single bind/unbind with confirmation modal, bind-all, unbind-all,
 * overlay lifecycle, and bind/unbind button visibility management.
 *
 * Mixed into OperatorPanel via Object.assign(OperatorPanel.prototype, BindOperatorsMixin).
 */
export const BindOperatorsMixin = {

    async bindOperator(operatorId) {
        try {
            devLogger.log('[OPERATOR] Binding operator:', operatorId);

            const response = await operatorPanelService.bindOperator(operatorId);

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.message || error.error || 'Failed to bind operator');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] Operator bound successfully:', result);

            this.eventBus.emit(EventType.OPERATOR_BOUND, {
                operator_id: operatorId,
                operator: result.operator
            });

            if (!this.boundOperatorIds.includes(operatorId)) {
                this.boundOperatorIds.push(operatorId);
            }
            devLogger.log('[OPERATOR] Bound Operator IDs:', this.boundOperatorIds);

            this.updateBindAllButtonVisibility();
            this.updateUnbindAllButtonVisibility();

            if (result.operator) {
                if (!this.selectedMetricsOperatorId) {
                    this.selectedMetricsOperatorId = operatorId;
                }
                this.updateMetrics(result.operator);
                this.updateStatus(result.operator.status || OperatorStatus.BOUND);
            }

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to bind operator:', error);
            notificationService.error(`Failed to bind operator: ${error.message}`);
        }
    },

    async unbindOperator(operatorId, forceWithOperatorId = false) {
        try {
            devLogger.log('[OPERATOR] Unbinding operator:', operatorId, { forceWithOperatorId });

            const body = { operator_id: operatorId };
            const response = await operatorPanelService.unbindOperator(body);

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to unbind operator');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] Operator unbound successfully:', result);

            this.boundOperatorIds = this.boundOperatorIds.filter(id => id !== operatorId);
            devLogger.log('[OPERATOR] Remaining bound Operator IDs:', this.boundOperatorIds);

            if (this.boundOperatorIds.length === 0) {
                this.updateStatus(OperatorStatus.OFFLINE);
                this.isConnected = false;
                this.clearPanelMetrics();
            }

            this.updateBindAllButtonVisibility();
            this.updateUnbindAllButtonVisibility();

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to unbind operator:', error);
            notificationService.error(`Failed to unbind operator: ${error.message}`);
        }
    },

    async bindOperatorWithConfirmation(operatorId) {
        try {
            const operator = this.operators.find(op => op.operator_id === operatorId);
            await this._showBindSingleModal({ operatorId, operator, mode: 'bind' });
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to bind Operator with confirmation:', error);
        }
    },

    async unbindOperatorWithConfirmation(operatorId, isStale = false) {
        try {
            const operator = this.operators.find(op => op.operator_id === operatorId);
            await this._showBindSingleModal({
                operatorId,
                operator,
                mode: isStale ? 'unbind-stale' : 'unbind'
            });
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to unbind Operator with confirmation:', error);
        }
    },

    async _showBindSingleModal({ operatorId, operator, mode }) {
        const isUnbind = mode === 'unbind' || mode === 'unbind-stale';
        const isStale = mode === 'unbind-stale';
        
        const title = isStale ? 'Unbind Stale Operator' : 
                     (isUnbind ? 'Unbind Operator from WebSession' : 'Bind Operator to WebSession');
        
        const message = isStale ? 'This Operator is stale (bound but offline). Unbinding will free it so it can be rebound when it comes back online.' :
                       (isUnbind ? 'This will disconnect the Operator from your current web session. You will no longer be able to interact with it through the chat interface.' :
                        'This will connect the Operator to your current web session, allowing you to interact with it through the chat interface.');

        const confirmLabel = isUnbind || isStale ? 'Unbind Operator' : 'Bind Operator';
        const confirmIcon = isUnbind || isStale ? 'link_off' : 'link';
        const icon = isUnbind || isStale ? 'link_off' : 'link';
        const iconClass = isUnbind || isStale ? 'unbind-all-icon' : '';
        const confirmClass = isUnbind || isStale ? 'unbind-all-confirm-btn' : '';
        const descriptionClass = isUnbind || isStale ? 'unbind-all-description' : '';

        const htmlContent = `
            <div class="bind-all-operators-container">
                <div class="bind-all-operators-list">
                    ${this._createBindAllOperatorItem(operator || { 
                        operator_id: operatorId, 
                        system_info: { 
                            hostname: operator?.system_info?.hostname || 'Unknown', 
                            os: operator?.system_info?.os || '-', 
                            private_ip: operator?.system_info?.private_ip || '-' 
                        } 
                    })}
                </div>
            </div>
            <div class="bind-all-processing initially-hidden" data-processing-indicator>
                <div class="spinner"></div>
                <span data-processing-label>${isUnbind || isStale ? 'Unbinding operator...' : 'Binding operator...'}</span>
            </div>
            <div class="bind-all-actions-feedback"></div>
        `;

        await showConfirmationModal({
            title,
            message,
            confirmLabel,
            confirmIcon,
            icon,
            iconClass,
            confirmClass,
            descriptionClass,
            htmlContent,
            onConfirm: async (overlay) => {
                const processingIndicator = overlay.querySelector('[data-processing-indicator]');
                const feedbackContainer = overlay.querySelector('.bind-all-actions-feedback');

                if (processingIndicator) processingIndicator.classList.remove('initially-hidden');

                try {
                    if (isUnbind || isStale) {
                        await this.unbindOperator(operatorId, isStale);
                    } else {
                        await this.bindOperator(operatorId);
                    }

                    if (feedbackContainer) {
                        const label = isUnbind || isStale ? 'Operator unbound successfully' : 'Operator bound successfully';
                        await templateLoader.renderTo(feedbackContainer, 'bind-result-feedback', { 
                            resultClass: 'success', 
                            icon: 'check_circle', 
                            message: label 
                        });
                    }
                    if (processingIndicator) processingIndicator.classList.add('initially-hidden');
                    
                    // Keep open for a moment to show success feedback
                    await new Promise(r => setTimeout(r, 1200));
                } catch (error) {
                    devLogger.error('[OPERATOR] Single bind/unbind modal action failed:', error);
                    if (feedbackContainer) {
                        await templateLoader.renderTo(feedbackContainer, 'bind-result-feedback', { 
                            resultClass: 'error', 
                            icon: 'error', 
                            message: error.message 
                        });
                    }
                    if (processingIndicator) processingIndicator.classList.add('initially-hidden');
                    
                    // Keep open longer to show error feedback
                    await new Promise(r => setTimeout(r, 3000));
                    throw error; // Let showConfirmationModal know it failed
                }
            }
        });
    },

    async showBindAllConfirmationOverlay() {
        devLogger.log('[OPERATOR] Showing bind-all confirmation overlay');

        const activeOperators = this.operators.filter(op =>
            op.status === OperatorStatus.ACTIVE &&
            !this.boundOperatorIds.includes(op.operator_id)
        );

        if (activeOperators.length === 0) {
            devLogger.log('[OPERATOR] No active operators to bind');
            notificationService.info('No active operators available to bind. All active operators are already bound to this session.');
            return;
        }

        const operatorsListHtml = activeOperators.map(op => this._createBindAllOperatorItem(op)).join('');
        const htmlContent = `
            <div class="bind-all-operators-container">
                <div class="bind-all-operators-list">
                    ${operatorsListHtml}
                </div>
            </div>
            <div class="bind-all-processing initially-hidden" data-processing-indicator>
                <div class="spinner"></div>
                <span data-processing-label>Binding operators...</span>
            </div>
            <div class="bind-all-actions-feedback"></div>
        `;

        await showConfirmationModal({
            title: 'Bind All Active Operators',
            message: `The following ${activeOperators.length} active operator${activeOperators.length !== 1 ? 's' : ''} will be bound to your current web session. You will be able to interact with all of them through the chat interface.`,
            confirmLabel: 'Bind All',
            confirmIcon: 'link',
            icon: 'link',
            htmlContent,
            onConfirm: async (overlay) => {
                await this.executeBindAll(overlay, activeOperators);
            }
        });

        this.updateBindAllButtonVisibility();
        this.updateUnbindAllButtonVisibility();
    },

    async executeBindAll(overlay, activeOperators) {
        const processingIndicator = overlay.querySelector('[data-processing-indicator]');
        const feedbackContainer = overlay.querySelector('.bind-all-actions-feedback');

        if (processingIndicator) processingIndicator.classList.remove('initially-hidden');

        try {
            const operatorIds = activeOperators.map(op => op.operator_id);
            const service = this.operatorPanelService || operatorPanelService;
            const response = await service.bindAllOperators(operatorIds);

            if (!response) {
                throw new Error('No response from operator panel service');
            }

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.message || error.error || 'Failed to bind operators');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] Bind-all completed successfully:', result);

            for (const opId of result.bound_operator_ids || operatorIds) {
                if (!this.boundOperatorIds.includes(opId)) {
                    this.boundOperatorIds.push(opId);
                }
            }

            if (feedbackContainer) {
                const boundCount = result.bound_count || operatorIds.length;
                const label = `${boundCount} operator${boundCount !== 1 ? 's' : ''} bound successfully`;
                await templateLoader.renderTo(feedbackContainer, 'bind-result-feedback', { 
                    resultClass: 'success', 
                    icon: 'check_circle', 
                    message: label 
                });
            }
            if (processingIndicator) processingIndicator.classList.add('initially-hidden');
            
            await new Promise(r => setTimeout(r, 1500));
        } catch (error) {
            devLogger.error('[OPERATOR] Bind-all failed:', error);
            if (feedbackContainer) {
                await templateLoader.renderTo(feedbackContainer, 'bind-result-feedback', { 
                    resultClass: 'error', 
                    icon: 'error', 
                    message: error.message 
                });
            }
            if (processingIndicator) processingIndicator.classList.add('initially-hidden');
            await new Promise(r => setTimeout(r, 3000));
            throw error;
        }
    },

    _createBindAllOperatorItem(op) {
        const hostname = op.system_info?.hostname || 'Unknown';
        const os = op.system_info?.os || 'Unknown';
        const internalIp = op.system_info?.private_ip || '-';
        const template = templateLoader.cache.get('bind-all-operator-item');
        return templateLoader.replace(template, {
            operatorId: op.operator_id,
            hostname,
            os,
            ip: internalIp,
            ipIcon: 'router',
            statusClass: '',
            statusLabel: 'Active'
        });
    },

    updateBindAllButtonVisibility() {
        if (!this.bindAllBtn) {
            this.bindAllBtn = document.getElementById('bind-all-btn');
        }
        if (!this.bindAllBtn) {
            devLogger.log('[OPERATOR] Bind-all button not found in DOM');
            return;
        }

        const unboundActiveCount = this.operators.filter(op =>
            op.status === OperatorStatus.ACTIVE &&
            !this.boundOperatorIds.includes(op.operator_id)
        ).length;

        if (unboundActiveCount > 0) {
            this.bindAllBtn.classList.remove('initially-hidden');
            const textSpan = this.bindAllBtn.querySelector('span:last-child');
            if (textSpan) textSpan.textContent = `Bind All Active (${unboundActiveCount})`;
        } else {
            this.bindAllBtn.classList.add('initially-hidden');
        }
    },

    async showUnbindAllConfirmationOverlay() {
        devLogger.log('[OPERATOR] Showing unbind-all confirmation overlay');

        const currentWebSessionId = window.authState?.getWebSessionId();

        const boundOperators = this.operators.filter(op =>
            (op.status === OperatorStatus.BOUND && op.web_session_id === currentWebSessionId) ||
            (op.status === OperatorStatus.STALE && this.boundOperatorIds.includes(op.operator_id))
        );

        if (boundOperators.length === 0) {
            devLogger.log('[OPERATOR] No bound operators to unbind');
            notificationService.info('No operators are currently bound to this session.');
            return;
        }

        const operatorsListHtml = boundOperators.map(op => this._createUnbindAllOperatorItem(op)).join('');
        const htmlContent = `
            <div class="bind-all-operators-container">
                <div class="bind-all-operators-list">
                    ${operatorsListHtml}
                </div>
            </div>
            <div class="bind-all-processing initially-hidden" data-processing-indicator>
                <div class="spinner"></div>
                <span data-processing-label>Unbinding operators...</span>
            </div>
            <div class="bind-all-actions-feedback"></div>
        `;

        await showConfirmationModal({
            title: 'Unbind All Operators',
            message: `The following ${boundOperators.length} operator${boundOperators.length !== 1 ? 's' : ''} will be unbound from your current web session. You will no longer be able to interact with them until they are rebound.`,
            confirmLabel: 'Unbind All',
            confirmIcon: 'link_off',
            icon: 'link_off',
            iconClass: 'unbind-all-icon',
            confirmClass: 'unbind-all-confirm-btn',
            descriptionClass: 'unbind-all-description',
            htmlContent,
            onConfirm: async (overlay) => {
                await this.executeUnbindAll(overlay, boundOperators);
            }
        });

        this.updateBindAllButtonVisibility();
        this.updateUnbindAllButtonVisibility();
    },

    async executeUnbindAll(overlay, boundOperators) {
        const processingIndicator = overlay.querySelector('[data-processing-indicator]');
        const feedbackContainer = overlay.querySelector('.bind-all-actions-feedback');

        if (processingIndicator) processingIndicator.classList.remove('initially-hidden');

        try {
            const operatorIds = boundOperators.map(op => op.operator_id);
            const service = this.operatorPanelService || operatorPanelService;
            const response = await service.unbindAllOperators(operatorIds);

            if (!response) {
                throw new Error('No response from operator panel service');
            }

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.message || error.error || 'Failed to unbind operators');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] Unbind-all completed successfully:', result);

            for (const opId of result.unbound_operator_ids || operatorIds) {
                this.boundOperatorIds = this.boundOperatorIds.filter(id => id !== opId);
            }

            if (feedbackContainer) {
                const unboundCount = result.unbound_count || operatorIds.length;
                const label = `${unboundCount} operator${unboundCount !== 1 ? 's' : ''} unbound successfully`;
                await templateLoader.renderTo(feedbackContainer, 'bind-result-feedback', { 
                    resultClass: 'success', 
                    icon: 'check_circle', 
                    message: label 
                });
            }
            if (processingIndicator) processingIndicator.classList.add('initially-hidden');

            if (this.boundOperatorIds.length === 0) {
                this.updateStatus(OperatorStatus.OFFLINE);
                this.isConnected = false;
                this.clearPanelMetrics();
            }

            await new Promise(r => setTimeout(r, 1500));
        } catch (error) {
            devLogger.error('[OPERATOR] Unbind-all failed:', error);
            if (feedbackContainer) {
                await templateLoader.renderTo(feedbackContainer, 'bind-result-feedback', { 
                    resultClass: 'error', 
                    icon: 'error', 
                    message: error.message 
                });
            }
            if (processingIndicator) processingIndicator.classList.add('initially-hidden');
            await new Promise(r => setTimeout(r, 3000));
            throw error;
        }
    },

    _createUnbindAllOperatorItem(op) {
        const hostname = op.system_info?.hostname || 'Unknown';
        const os = op.system_info?.os || 'Unknown';
        const publicIp = op.system_info?.public_ip || '-';
        const isStale = op.status === OperatorStatus.STALE;
        const statusLabel = isStale ? 'Stale' : 'Bound';
        const statusClass = isStale ? 'unbind-all-operator-status-stale' : '';
        const template = templateLoader.cache.get('bind-all-operator-item');
        return templateLoader.replace(template, {
            operatorId: op.operator_id,
            hostname,
            os,
            ip: publicIp,
            ipIcon: 'language',
            statusClass,
            statusLabel
        });
    },

    updateUnbindAllButtonVisibility() {
        if (!this.unbindAllBtn) {
            this.unbindAllBtn = document.getElementById('unbind-all-btn');
        }
        if (!this.unbindAllBtn) {
            devLogger.log('[OPERATOR] Unbind-all button not found in DOM');
            return;
        }

        const boundToMeCount = this.boundOperatorIds.length;

        if (boundToMeCount > 0) {
            this.unbindAllBtn.classList.remove('initially-hidden');
            const textSpan = this.unbindAllBtn.querySelector('span:last-child');
            if (textSpan) textSpan.textContent = `Unbind All (${boundToMeCount})`;
        } else {
            this.unbindAllBtn.classList.add('initially-hidden');
        }
    }
};
