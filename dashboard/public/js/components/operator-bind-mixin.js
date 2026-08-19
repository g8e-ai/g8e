// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { devLogger } from '../utils/dev-logger.js';
import { OperatorStatus } from '../constants/operator-constants.js';
import { templateLoader } from '../utils/template-loader.js';
import { EventType } from '../constants/events.js';
import { operatorPanelService } from '../utils/operator-panel-service.js';

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
            alert(`Failed to bind operator: ${error.message}`);
        }
    },

    async unbindOperator(operatorId, forceWithOperatorId = false) {
        try {
            devLogger.log('[OPERATOR] Unbinding operator:', operatorId, { forceWithOperatorId });

            const body = forceWithOperatorId ? { operator_id: operatorId } : {};
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
            alert(`Failed to unbind operator: ${error.message}`);
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

    _showBindSingleModal({ operatorId, operator, mode }) {
        return new Promise((resolve) => {
            const template = templateLoader.cache.get('bind-single-confirmation-overlay');
            if (!template) {
                devLogger.error('[OPERATOR] bind-single-confirmation-overlay template not found');
                resolve();
                return;
            }

            const isUnbind = mode === 'unbind' || mode === 'unbind-stale';
            const isStale = mode === 'unbind-stale';

            const hostname = operator?.system_info?.hostname || 'Unknown';
            const os = operator?.system_info?.os || '-';
            const internalIp = operator?.system_info?.private_ip || '-';

            const overlayContainer = document.createElement('div');
            overlayContainer.innerHTML = template;
            const overlay = overlayContainer.firstElementChild;

            const iconEl = overlay.querySelector('[data-bind-icon]');
            const titleEl = overlay.querySelector('[data-modal-title]');
            const subtitleEl = overlay.querySelector('[data-modal-subtitle]');
            const descEl = overlay.querySelector('[data-modal-description]');
            const confirmIconEl = overlay.querySelector('[data-confirm-icon]');
            const confirmLabelEl = overlay.querySelector('[data-confirm-label]');
            const processingLabelEl = overlay.querySelector('[data-processing-label]');
            const confirmBtn = overlay.querySelector('[data-action="confirm"]');

            if (isStale) {
                if (iconEl) { iconEl.textContent = 'link_off'; iconEl.classList.add('unbind-all-icon'); }
                if (titleEl) titleEl.textContent = 'Unbind Stale Operator';
                if (subtitleEl) subtitleEl.textContent = 'Operator is bound but offline';
                if (descEl) { descEl.textContent = 'This Operator is stale (bound but offline). Unbinding will free it so it can be rebound when it comes back online.'; descEl.classList.add('unbind-all-description'); }
                if (confirmIconEl) confirmIconEl.textContent = 'link_off';
                if (confirmLabelEl) confirmLabelEl.textContent = 'Unbind Operator';
                if (processingLabelEl) processingLabelEl.textContent = 'Unbinding operator...';
                if (confirmBtn) confirmBtn.classList.add('unbind-all-confirm-btn');
            } else if (isUnbind) {
                if (iconEl) { iconEl.textContent = 'link_off'; iconEl.classList.add('unbind-all-icon'); }
                if (titleEl) titleEl.textContent = 'Unbind Operator from WebSession';
                if (subtitleEl) subtitleEl.textContent = 'Disconnect from current web session';
                if (descEl) { descEl.textContent = 'This will disconnect the Operator from your current web session. You will no longer be able to interact with it through the chat interface.'; descEl.classList.add('unbind-all-description'); }
                if (confirmIconEl) confirmIconEl.textContent = 'link_off';
                if (confirmLabelEl) confirmLabelEl.textContent = 'Unbind Operator';
                if (processingLabelEl) processingLabelEl.textContent = 'Unbinding operator...';
                if (confirmBtn) confirmBtn.classList.add('unbind-all-confirm-btn');
            } else {
                if (subtitleEl) subtitleEl.textContent = 'Connect to current web session';
            }

            const listEl = overlay.querySelector('[data-operators-list]');
            if (listEl) {
                listEl.innerHTML = this._createBindAllOperatorItem(operator || { operator_id: operatorId, system_info: { hostname, os, private_ip: internalIp } });
            }

            document.body.appendChild(overlay);

            const close = () => {
                if (this._bindSingleEscHandler) {
                    document.removeEventListener('keydown', this._bindSingleEscHandler);
                    this._bindSingleEscHandler = null;
                }
                overlay.classList.remove('active');
                setTimeout(() => {
                    if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
                    resolve();
                }, 300);
            };

            const closeAndCancel = () => {
                devLogger.log('[OPERATOR] Single bind/unbind modal cancelled by user');
                close();
            };

            const closeAndConfirm = async () => {
                const processingIndicator = overlay.querySelector('[data-processing-indicator]');
                const actionsContainer = overlay.querySelector('.bind-all-actions');
                const cancelBtn = overlay.querySelector('[data-action="cancel"]');

                if (confirmBtn) confirmBtn.disabled = true;
                if (cancelBtn) cancelBtn.disabled = true;
                if (processingIndicator) processingIndicator.classList.remove('initially-hidden');

                try {
                    if (isUnbind || isStale) {
                        await this.unbindOperator(operatorId, isStale);
                    } else {
                        await this.bindOperator(operatorId);
                    }

                    if (actionsContainer) {
                        const label = isUnbind || isStale ? 'Operator unbound successfully' : 'Operator bound successfully';
                        actionsContainer.innerHTML = this._renderFeedback('success', 'check_circle', label);
                    }
                    if (processingIndicator) processingIndicator.classList.add('initially-hidden');
                    setTimeout(() => close(), 1200);
                } catch (error) {
                    devLogger.error('[OPERATOR] Single bind/unbind modal action failed:', error);
                    if (actionsContainer) {
                        actionsContainer.innerHTML = this._renderFeedback('error', 'error', this._escapeHtml(error.message));
                    }
                    if (processingIndicator) processingIndicator.classList.add('initially-hidden');
                    setTimeout(() => close(), 3000);
                }
            };

            overlay.querySelector('[data-action="close"]')?.addEventListener('click', closeAndCancel);
            overlay.querySelector('[data-action="cancel"]')?.addEventListener('click', closeAndCancel);
            overlay.querySelector('[data-action="confirm"]')?.addEventListener('click', closeAndConfirm);

            overlay.addEventListener('click', (e) => {
                if (e.target === overlay) closeAndCancel();
            });

            this._bindSingleEscHandler = (e) => {
                if (e.key === 'Escape') closeAndCancel();
            };
            document.addEventListener('keydown', this._bindSingleEscHandler);

            requestAnimationFrame(() => overlay.classList.add('active'));
        });
    },

    showBindAllConfirmationOverlay() {
        devLogger.log('[OPERATOR] Showing bind-all confirmation overlay');

        const activeOperators = this.operators.filter(op =>
            op.status === OperatorStatus.ACTIVE &&
            !this.boundOperatorIds.includes(op.operator_id)
        );

        if (activeOperators.length === 0) {
            devLogger.log('[OPERATOR] No active operators to bind');
            alert('No active operators available to bind. All active operators are already bound to this session.');
            return;
        }

        const template = templateLoader.cache.get('bind-all-confirmation-overlay');
        if (!template) {
            devLogger.error('[OPERATOR] bind-all-confirmation-overlay template not found');
            return;
        }

        const overlayContainer = document.createElement('div');
        overlayContainer.innerHTML = template;
        const overlay = overlayContainer.firstElementChild;

        const countEl = overlay.querySelector('[data-operator-count]');
        if (countEl) {
            countEl.textContent = `${activeOperators.length} operator${activeOperators.length !== 1 ? 's' : ''} will be bound`;
        }

        const listEl = overlay.querySelector('[data-operators-list]');
        if (listEl) {
            listEl.innerHTML = activeOperators.map(op => this._createBindAllOperatorItem(op)).join('');
        }

        document.body.appendChild(overlay);
        this.bindAllOverlay = overlay;

        this._setupBindAllOverlayEvents(overlay, activeOperators);

        requestAnimationFrame(() => overlay.classList.add('active'));
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

    _setupBindAllOverlayEvents(overlay, activeOperators) {
        const closeBtn = overlay.querySelector('[data-action="close"]');
        if (closeBtn) closeBtn.addEventListener('click', () => this.closeBindAllOverlay());

        const cancelBtn = overlay.querySelector('[data-action="cancel"]');
        if (cancelBtn) cancelBtn.addEventListener('click', () => this.closeBindAllOverlay());

        const confirmBtn = overlay.querySelector('[data-action="confirm"]');
        if (confirmBtn) confirmBtn.addEventListener('click', () => this.executeBindAll(overlay, activeOperators));

        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) this.closeBindAllOverlay();
        });

        this._bindAllEscHandler = (e) => {
            if (e.key === 'Escape') this.closeBindAllOverlay();
        };
        document.addEventListener('keydown', this._bindAllEscHandler);
    },

    closeBindAllOverlay() {
        if (!this.bindAllOverlay) return;
        devLogger.log('[OPERATOR] Closing bind-all overlay');

        if (this._bindAllEscHandler) {
            document.removeEventListener('keydown', this._bindAllEscHandler);
            this._bindAllEscHandler = null;
        }

        this.bindAllOverlay.classList.remove('active');
        setTimeout(() => {
            if (this.bindAllOverlay && this.bindAllOverlay.parentNode) {
                this.bindAllOverlay.parentNode.removeChild(this.bindAllOverlay);
            }
            this.bindAllOverlay = null;
        }, 300);
    },

    async executeBindAll(overlay, activeOperators) {
        devLogger.log('[OPERATOR] Executing bind-all for operators:', activeOperators.map(op => op.operator_id));

        const confirmBtn = overlay.querySelector('[data-action="confirm"]');
        const cancelBtn = overlay.querySelector('[data-action="cancel"]');
        const actionsContainer = overlay.querySelector('.bind-all-actions');
        const processingIndicator = overlay.querySelector('[data-processing-indicator]');

        if (confirmBtn) confirmBtn.disabled = true;
        if (cancelBtn) cancelBtn.disabled = true;
        if (processingIndicator) processingIndicator.classList.remove('initially-hidden');

        try {
            const operatorIds = activeOperators.map(op => op.operator_id);
            const response = await operatorPanelService.bindAllOperators(operatorIds);

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

            if (actionsContainer) {
                const boundCount = result.bound_count || operatorIds.length;
                actionsContainer.innerHTML = this._renderFeedback('success', 'check_circle', `${boundCount} operator${boundCount !== 1 ? 's' : ''} bound successfully`);
            }
            if (processingIndicator) processingIndicator.classList.add('initially-hidden');

            setTimeout(() => this.closeBindAllOverlay(), 1500);

        } catch (error) {
            devLogger.error('[OPERATOR] Bind-all failed:', error);

            if (actionsContainer) {
                actionsContainer.innerHTML = this._renderFeedback('error', 'error', `Failed to bind operators: ${this._escapeHtml(error.message)}`);
            }
            if (processingIndicator) processingIndicator.classList.add('initially-hidden');
            setTimeout(() => this.closeBindAllOverlay(), 3000);
        }
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

    showUnbindAllConfirmationOverlay() {
        devLogger.log('[OPERATOR] Showing unbind-all confirmation overlay');

        const currentWebSessionId = window.authState?.getWebSessionId();

        const boundOperators = this.operators.filter(op =>
            (op.status === OperatorStatus.BOUND && op.web_session_id === currentWebSessionId) ||
            (op.status === OperatorStatus.STALE && this.boundOperatorIds.includes(op.operator_id))
        );

        if (boundOperators.length === 0) {
            devLogger.log('[OPERATOR] No bound operators to unbind');
            alert('No operators are currently bound to this session.');
            return;
        }

        const template = templateLoader.cache.get('unbind-all-confirmation-overlay');
        if (!template) {
            devLogger.error('[OPERATOR] unbind-all-confirmation-overlay template not found');
            return;
        }

        const overlayContainer = document.createElement('div');
        overlayContainer.innerHTML = template;
        const overlay = overlayContainer.firstElementChild;

        const countEl = overlay.querySelector('[data-operator-count]');
        if (countEl) {
            countEl.textContent = `${boundOperators.length} operator${boundOperators.length !== 1 ? 's' : ''} will be unbound`;
        }

        const listEl = overlay.querySelector('[data-operators-list]');
        if (listEl) {
            listEl.innerHTML = boundOperators.map(op => this._createUnbindAllOperatorItem(op)).join('');
        }

        document.body.appendChild(overlay);
        this.unbindAllOverlay = overlay;

        this._setupUnbindAllOverlayEvents(overlay, boundOperators);

        requestAnimationFrame(() => overlay.classList.add('active'));
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

    _setupUnbindAllOverlayEvents(overlay, boundOperators) {
        const closeBtn = overlay.querySelector('[data-action="close"]');
        if (closeBtn) closeBtn.addEventListener('click', () => this.closeUnbindAllOverlay());

        const cancelBtn = overlay.querySelector('[data-action="cancel"]');
        if (cancelBtn) cancelBtn.addEventListener('click', () => this.closeUnbindAllOverlay());

        const confirmBtn = overlay.querySelector('[data-action="confirm"]');
        if (confirmBtn) confirmBtn.addEventListener('click', () => this.executeUnbindAll(overlay, boundOperators));

        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) this.closeUnbindAllOverlay();
        });

        this._unbindAllEscHandler = (e) => {
            if (e.key === 'Escape') this.closeUnbindAllOverlay();
        };
        document.addEventListener('keydown', this._unbindAllEscHandler);
    },

    closeUnbindAllOverlay() {
        if (!this.unbindAllOverlay) return;
        devLogger.log('[OPERATOR] Closing unbind-all overlay');

        if (this._unbindAllEscHandler) {
            document.removeEventListener('keydown', this._unbindAllEscHandler);
            this._unbindAllEscHandler = null;
        }

        this.unbindAllOverlay.classList.remove('active');
        setTimeout(() => {
            if (this.unbindAllOverlay && this.unbindAllOverlay.parentNode) {
                this.unbindAllOverlay.parentNode.removeChild(this.unbindAllOverlay);
            }
            this.unbindAllOverlay = null;
        }, 300);
    },

    async executeUnbindAll(overlay, boundOperators) {
        devLogger.log('[OPERATOR] Executing unbind-all for operators:', boundOperators.map(op => op.operator_id));

        const confirmBtn = overlay.querySelector('[data-action="confirm"]');
        const cancelBtn = overlay.querySelector('[data-action="cancel"]');
        const actionsContainer = overlay.querySelector('.bind-all-actions');
        const processingIndicator = overlay.querySelector('[data-processing-indicator]');

        if (confirmBtn) confirmBtn.disabled = true;
        if (cancelBtn) cancelBtn.disabled = true;
        if (processingIndicator) processingIndicator.classList.remove('initially-hidden');

        try {
            const operatorIds = boundOperators.map(op => op.operator_id);
            const response = await operatorPanelService.unbindAllOperators(operatorIds);

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.message || error.error || 'Failed to unbind operators');
            }

            const result = await response.json();
            devLogger.log('[OPERATOR] Unbind-all completed successfully:', result);

            for (const opId of result.unbound_operator_ids || operatorIds) {
                this.boundOperatorIds = this.boundOperatorIds.filter(id => id !== opId);
            }

            if (actionsContainer) {
                const unboundCount = result.unbound_count || operatorIds.length;
                actionsContainer.innerHTML = this._renderFeedback('success', 'check_circle', `${unboundCount} operator${unboundCount !== 1 ? 's' : ''} unbound successfully`);
            }
            if (processingIndicator) processingIndicator.classList.add('initially-hidden');

            if (this.boundOperatorIds.length === 0) {
                this.updateStatus(OperatorStatus.OFFLINE);
                this.isConnected = false;
                this.clearPanelMetrics();
            }

            this.updateBindAllButtonVisibility();
            this.updateUnbindAllButtonVisibility();

            setTimeout(() => this.closeUnbindAllOverlay(), 1500);

        } catch (error) {
            devLogger.error('[OPERATOR] Unbind-all failed:', error);

            if (actionsContainer) {
                actionsContainer.innerHTML = this._renderFeedback('error', 'error', `Failed to unbind operators: ${this._escapeHtml(error.message)}`);
            }
            if (processingIndicator) processingIndicator.classList.add('initially-hidden');
            setTimeout(() => this.closeUnbindAllOverlay(), 3000);
        }
    },

    _renderFeedback(resultClass, icon, message) {
        const template = templateLoader.cache.get('bind-result-feedback');
        return templateLoader.replace(template, { resultClass, icon, message });
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
