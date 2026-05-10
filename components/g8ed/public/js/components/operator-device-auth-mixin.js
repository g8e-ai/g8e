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
import { operatorPanelService } from '../utils/operator-panel-service.js';
import { notificationService } from '../utils/notification-service.js';

/**
 * OperatorDeviceAuthMixin - Inline device authorization UI and approve/deny flow.
 *
 * Covers: pending auth request display on operator cards, approve/deny actions,
 * auto-dismiss on expiry, and SSE-driven UI clearing.
 *
 * Mixed into OperatorPanel via Object.assign(OperatorPanel.prototype, OperatorDeviceAuthMixin).
 */
export const OperatorDeviceAuthMixin = {

    handleDevicePendingAuthorization(data) {
        const { token, operator_id, device_info = {}, expires_at } = data;

        const operatorCard = this.drawerList?.querySelector(`[data-operator-id="${operator_id}"]`);
        if (!operatorCard) {
            devLogger.warn('[OPERATOR] Operator card not found for pending auth:', operator_id);
            return;
        }

        if (this._pendingAuthRequests.has(operator_id)) {
            this._clearPendingAuthUI(operator_id);
        }

        operatorCard.classList.add('pending-device-auth');

        const hostnameEl = operatorCard.querySelector('.operator-item-hostname');
        if (hostnameEl) {
            const hostname = device_info.hostname || 'Unknown device';
            hostnameEl.textContent = hostname;
            hostnameEl.classList.add('pending-auth-hostname');
        }

        const authContainer = document.createElement('div');
        authContainer.className = 'device-auth-inline-container';

        const approveBtn = document.createElement('button');
        approveBtn.className = 'device-auth-approve-btn';
        approveBtn.setAttribute('data-token', token);
        approveBtn.setAttribute('data-operator-id', operator_id);
        approveBtn.innerHTML = '<span class="material-symbols-outlined">check</span> Approve';
        approveBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.authorizeDevice(token, operator_id);
        });

        const denyBtn = document.createElement('button');
        denyBtn.className = 'device-auth-deny-btn';
        denyBtn.setAttribute('data-token', token);
        denyBtn.setAttribute('data-operator-id', operator_id);
        denyBtn.innerHTML = '<span class="material-symbols-outlined">close</span> Deny';
        denyBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.rejectDevice(token, operator_id);
        });

        authContainer.appendChild(approveBtn);
        authContainer.appendChild(denyBtn);

        const itemInfo = operatorCard.querySelector('.operator-item-info');
        if (itemInfo) {
            itemInfo.after(authContainer);
        } else {
            operatorCard.appendChild(authContainer);
        }

        const expiresMs = new Date(expires_at).getTime() - Date.now();
        let timeout = null;
        if (expiresMs > 0) {
            timeout = setTimeout(() => {
                this.clearInlineAuthUI(operator_id, token, false);
            }, expiresMs);
        }

        this._pendingAuthRequests.set(operator_id, { token, device_info, timeout });
        devLogger.log('[OPERATOR] Pending device auth UI shown:', { operator_id, token: token.substring(0, 12) + '...' });
    },

    _clearPendingAuthUI(operatorId) {
        const pending = this._pendingAuthRequests.get(operatorId);
        if (pending?.timeout) {
            window.clearTimeout(pending.timeout);
        }
        this._pendingAuthRequests.delete(operatorId);

        const operatorCard = this.drawerList?.querySelector(`[data-operator-id="${operatorId}"]`);
        if (operatorCard) {
            operatorCard.classList.remove('pending-device-auth');
            const authContainer = operatorCard.querySelector('.device-auth-inline-container');
            if (authContainer) authContainer.remove();
            const hostnameEl = operatorCard.querySelector('.operator-item-hostname');
            if (hostnameEl) hostnameEl.classList.remove('pending-auth-hostname');
        }
    },

    clearInlineAuthUI(operatorId, expectedToken = null, shouldClearTimeout = true) {
        if (!this._pendingAuthRequests) {
            this._pendingAuthRequests = new Map();
        }

        const pending = this._pendingAuthRequests.get(operatorId);

        if (expectedToken && pending && pending.token !== expectedToken) {
            return;
        }

        if (shouldClearTimeout && pending?.timeout) {
            window.clearTimeout(pending.timeout);
        }

        this._pendingAuthRequests.delete(operatorId);

        const operatorCard = this.drawerList?.querySelector(`[data-operator-id="${operatorId}"]`);
        if (!operatorCard) return;

        operatorCard.classList.remove('pending-device-auth');

        const authContainer = operatorCard.querySelector('.device-auth-inline-container');
        if (authContainer) authContainer.remove();

        const hostnameEl = operatorCard.querySelector('.operator-item-hostname');
        if (hostnameEl) hostnameEl.classList.remove('pending-auth-hostname');

        devLogger.log('[OPERATOR] Cleared inline auth UI:', { operatorId });
    },

    async authorizeDevice(token, operatorId) {
        try {
            const response = await operatorPanelService.authorizeDevice(token);

            if (!response.ok) {
                const data = await response.json();
                notificationService.error(`Failed to authorize device: ${data.error || 'Unknown error'}`);
                return;
            }

            this.clearInlineAuthUI(operatorId);
            devLogger.log('[OPERATOR] Device authorized:', { operatorId, token: token.substring(0, 12) + '...' });

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to authorize device:', error);
            notificationService.error(`Failed to authorize device: ${error.message}`);
        }
    },

    async rejectDevice(token, operatorId) {
        try {
            await operatorPanelService.rejectDevice(token);
            devLogger.log('[OPERATOR] Device rejected:', { operatorId, token: token.substring(0, 12) + '...' });
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to reject device:', error);
        }

        this.clearInlineAuthUI(operatorId);
    },

    handleDeviceAuthorized(data) {
        const { operator_id } = data;
        if (!operator_id) {
            devLogger.warn('[OPERATOR] handleDeviceAuthorized called without operator_id');
            return;
        }
        this.clearInlineAuthUI(operator_id);
        devLogger.log('[OPERATOR] Device authorized via SSE:', { operator_id });
    }
};
