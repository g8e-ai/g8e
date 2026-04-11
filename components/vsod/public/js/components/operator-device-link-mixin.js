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
import { 
    DeviceLinkStatus,
    DEFAULT_DEVICE_LINK_MAX_USES,
    DEVICE_LINK_MAX_USES_MIN,
    DEVICE_LINK_MAX_USES_MAX
} from '../constants/auth-constants.js';
import { operatorPanelService } from '../utils/operator-panel-service.js';
import { OperatorDialogs } from '../constants/operator-messages.js';
import { escapeHtml } from '../utils/html.js';
import { notificationService } from '../utils/notification-service.js';
import { showConfirmationModal } from '../utils/ui-utils.js';

/**
 * OperatorDeviceLinkMixin - Device link token CRUD and UI.
 *
 * Covers: slot availability display, token creation form, token list rendering,
 * revoke/delete operations, and error display within the download overlay.
 *
 * Mixed into OperatorPanel via Object.assign(OperatorPanel.prototype, OperatorDeviceLinkMixin).
 */
export const OperatorDeviceLinkMixin = {

    async _initDeviceLinkDeploymentSection(overlay) {
        devLogger.log('[OPERATOR] Initializing Device Link Deployment section');
        await this._loadDeviceLinkAvailableSlots(overlay);
        await this._loadDeviceLinks(overlay);
    },

    async _loadDeviceLinkAvailableSlots(overlay) {
        const availableSlotsSpan = overlay.querySelector('#device-link-available-slots');
        const maxRegInput = overlay.querySelector('#device-link-max-uses');
        try {
            const maxSlots = this.maxSlots || 1;
            const usedSlots = this.usedSlots || 0;
            const availableSlots = Math.max(0, maxSlots - usedSlots);
            if (availableSlotsSpan) {
                availableSlotsSpan.textContent = availableSlots === Infinity ? 'Unlimited' : availableSlots;
            }
            if (maxRegInput && availableSlots !== Infinity) {
                maxRegInput.max = availableSlots;
                maxRegInput.value = availableSlots;
            }
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to load available slots:', error);
            if (availableSlotsSpan) availableSlotsSpan.textContent = '-';
        }
    },

    _bindDeviceLinkCreateForm(overlay) {
        const createBtn = overlay.querySelector('#device-link-create-btn');
        const nameInput = overlay.querySelector('#device-link-token-name');
        const maxRegInput = overlay.querySelector('#device-link-max-uses');
        const expirySelect = overlay.querySelector('#device-link-expiry');
        const errorDiv = overlay.querySelector('#device-link-create-error');
        const createSection = overlay.querySelector('#device-link-create-section');
        const tokenCreated = overlay.querySelector('#device-link-token-created');
        const createAnotherBtn = overlay.querySelector('#device-link-create-another-btn');

        if (createBtn) {
            createBtn.addEventListener('click', async (e) => {
                e.stopPropagation();
                await this._createDeviceLink(overlay);
            });
        }

        if (createAnotherBtn) {
            createAnotherBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                const maxSlots = this.maxSlots || 1;
                const usedSlots = this.usedSlots || 0;
                const availableSlots = Math.max(0, maxSlots - usedSlots);
                if (nameInput) nameInput.value = '';
                if (maxRegInput) maxRegInput.value = availableSlots === Infinity ? '100' : String(availableSlots);
                if (expirySelect) expirySelect.value = '48';
                if (errorDiv) errorDiv.classList.add('initially-hidden');
                if (tokenCreated) tokenCreated.classList.add('initially-hidden');
                if (createSection) createSection.classList.remove('initially-hidden');
            });
        }
    },

    async _createDeviceLink(overlay) {
        const createBtn = overlay.querySelector('#device-link-create-btn');
        const nameInput = overlay.querySelector('#device-link-token-name');
        const maxRegInput = overlay.querySelector('#device-link-max-uses');
        const expirySelect = overlay.querySelector('#device-link-expiry');
        const errorDiv = overlay.querySelector('#device-link-create-error');
        const errorText = overlay.querySelector('#device-link-create-error-text');

        if (errorDiv) errorDiv.classList.add('initially-hidden');

        const name = nameInput?.value?.trim();
        const rawMaxUses = parseInt(maxRegInput?.value, 10);
        const maxUses = Number.isNaN(rawMaxUses) ? DEFAULT_DEVICE_LINK_MAX_USES : rawMaxUses;
        const rawExpiresInHours = parseInt(expirySelect?.value, 10);
        const expiresInHours = Number.isNaN(rawExpiresInHours) ? 48 : rawExpiresInHours;

        if (maxUses < DEVICE_LINK_MAX_USES_MIN || maxUses > DEVICE_LINK_MAX_USES_MAX) {
            this._showDeviceLinkError(errorDiv, errorText, `Max uses must be between ${DEVICE_LINK_MAX_USES_MIN.toLocaleString()} and ${DEVICE_LINK_MAX_USES_MAX.toLocaleString()}`);
            return;
        }

        if (createBtn) {
            createBtn.disabled = true;
            createBtn.innerHTML = '<span class="material-symbols-outlined rotating">sync</span>';
        }

        try {
            devLogger.log('[OPERATOR] Creating device link:', { name, maxUses, expiresInHours });

            const response = await operatorPanelService.createDeviceLink({
                maxUses: maxUses,
                expiresInHours: expiresInHours,
                name
            });

            const result = await response.json();

            if (!response.ok || !result.success) {
                throw new Error(result.error || 'Failed to create device link');
            }

            devLogger.log('[OPERATOR] Device link created:', result);

            if (nameInput) nameInput.value = '';
            if (expirySelect) expirySelect.value = '1';

            await this._loadDeviceLinks(overlay);
            await this._loadDeviceLinkAvailableSlots(overlay);

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to create device link:', error);
            this._showDeviceLinkError(errorDiv, errorText, error.message || 'Failed to create device link');
        } finally {
            if (createBtn) {
                createBtn.disabled = false;
                createBtn.innerHTML = '<span class="material-symbols-outlined">play_arrow</span>';
            }
        }
    },

    async _loadDeviceLinks(overlay) {
        const tokensList = overlay.querySelector('#device-link-tokens-list');
        const tokensLoading = overlay.querySelector('#device-link-tokens-loading');
        const tokensEmpty = overlay.querySelector('#device-link-tokens-empty');
        if (!tokensList) return;

        if (tokensLoading) tokensLoading.classList.remove('initially-hidden');
        if (tokensEmpty) tokensEmpty.classList.add('initially-hidden');
        tokensList.innerHTML = '';

        try {
            const response = await operatorPanelService.listDeviceLinks();

            if (!response.ok) {
                throw new Error('Failed to load device links');
            }

            const result = await response.json();
            const tokens = result.links;

            if (tokensLoading) tokensLoading.classList.add('initially-hidden');

            if (tokens.length === 0) {
                if (tokensEmpty) tokensEmpty.classList.remove('initially-hidden');
                return;
            }

            tokens.forEach(token => {
                const tokenEl = this._createDeviceLinkElement(token, overlay);
                tokensList.appendChild(tokenEl);
            });

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to load device links:', error);
            if (tokensLoading) tokensLoading.classList.add('initially-hidden');
            if (tokensEmpty) {
                tokensEmpty.innerHTML = '<span class="material-symbols-outlined">error</span><span>Failed to load tokens</span>';
                tokensEmpty.classList.remove('initially-hidden');
            }
        }
    },

    _createDeviceLinkElement(token, overlay) {
        const el = document.createElement('div');
        el.className = 'device-link-item';
        el.setAttribute('data-token-id', token.token);

        const statusClass = token.status === DeviceLinkStatus.ACTIVE ? 'device-link-status-active' :
                           token.status === DeviceLinkStatus.EXHAUSTED ? 'device-link-status-exhausted' : 'device-link-status-revoked';
        const statusIcon = token.status === DeviceLinkStatus.ACTIVE ? 'check_circle' :
                          token.status === DeviceLinkStatus.EXHAUSTED ? 'done_all' : 'cancel';

        const createdDate = new Date(token.created_at).toLocaleDateString('en-US', {
            month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
        });
        const expiresDate = new Date(token.expires_at).toLocaleDateString('en-US', {
            month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
        });

        const isExpired = new Date(token.expires_at) < new Date();
        const displayStatus = isExpired && token.status === DeviceLinkStatus.ACTIVE ? DeviceLinkStatus.EXPIRED : token.status;

        el.innerHTML = `
            <div class="device-link-item-header">
                <div class="device-link-item-name">
                    <span class="material-symbols-outlined ${statusClass}">${statusIcon}</span>
                    <span>${escapeHtml(token.name || '(unnamed)')}</span>
                </div>
                <span class="device-link-item-status ${statusClass}">${displayStatus}</span>
            </div>
            <div class="device-link-item-stats">
                <div class="device-link-item-stat">
                    <span class="device-link-item-stat-label">Claims:</span>
                    <span class="device-link-item-stat-value">${token.uses} / ${token.max_uses}</span>
                </div>
                <div class="device-link-item-stat">
                    <span class="device-link-item-stat-label">Created:</span>
                    <span class="device-link-item-stat-value">${createdDate}</span>
                </div>
                <div class="device-link-item-stat">
                    <span class="device-link-item-stat-label">Expires:</span>
                    <span class="device-link-item-stat-value">${expiresDate}</span>
                </div>
            </div>
            <div class="device-link-item-actions">
                ${token.status === DeviceLinkStatus.ACTIVE && !isExpired ? `
                    <button class="device-link-copy-btn" title="Copy device link token">
                        <span class="material-symbols-outlined">content_copy</span>
                        <span>Copy Token</span>
                    </button>
                    <button class="device-link-copy-cmd-btn" title="Copy operator command">
                        <span class="material-symbols-outlined">terminal</span>
                        <span>Copy Command</span>
                    </button>
                    <button class="device-link-revoke-btn" title="Revoke token">
                        <span class="material-symbols-outlined">delete</span>
                    </button>
                ` : `
                    <button class="device-link-delete-btn" title="Delete token">
                        <span class="material-symbols-outlined">delete</span>
                    </button>
                `}
            </div>
        `;

        const copyBtn = el.querySelector('.device-link-copy-btn');
        if (copyBtn) {
            copyBtn.addEventListener('click', () => {
                this.copyCurlCommand(token.token, copyBtn);
            });
        }

        const copyCmdBtn = el.querySelector('.device-link-copy-cmd-btn');
        if (copyCmdBtn) {
            copyCmdBtn.addEventListener('click', () => {
                this.copyCurlCommand(token.operator_command || `g8e.operator --device-token ${token.token}`, copyCmdBtn);
            });
        }

        const revokeBtn = el.querySelector('.device-link-revoke-btn');
        if (revokeBtn) {
            revokeBtn.addEventListener('click', async () => {
                await this._revokeDeviceLink(token.token, overlay);
            });
        }

        const deleteBtn = el.querySelector('.device-link-delete-btn');
        if (deleteBtn) {
            deleteBtn.addEventListener('click', async () => {
                await this._deleteDeviceLink(token.token, overlay);
            });
        }

        return el;
    },

    async _revokeDeviceLink(tokenId, overlay) {
        const confirmed = await showConfirmationModal({
            title: 'Revoke Device Link',
            message: OperatorDialogs.REVOKE_DEVICE_LINK_CONFIRM,
            confirmLabel: 'Revoke',
            confirmIcon: 'delete'
        });
        if (!confirmed) return;

        try {
            devLogger.log('[OPERATOR] Revoking device link:', tokenId.substring(0, 12) + '...');
            const response = await operatorPanelService.revokeDeviceLink(tokenId);

            if (!response.ok) {
                const result = await response.json();
                throw new Error(result.error || 'Failed to revoke token');
            }

            devLogger.log('[OPERATOR] Device link revoked successfully');
            await this._loadDeviceLinks(overlay);

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to revoke device link:', error);
            notificationService.error(`Failed to revoke token: ${error.message}`);
        }
    },

    async _deleteDeviceLink(tokenId, overlay) {
        try {
            devLogger.log('[OPERATOR] Deleting device link:', tokenId.substring(0, 12) + '...');
            const response = await operatorPanelService.deleteDeviceLink(tokenId);

            if (!response.ok) {
                const result = await response.json();
                throw new Error(result.error || 'Failed to delete token');
            }

            devLogger.log('[OPERATOR] Device link deleted successfully');
            await this._loadDeviceLinks(overlay);

        } catch (error) {
            devLogger.error('[OPERATOR] Failed to delete device link:', error);
            notificationService.error(`Failed to delete token: ${error.message}`);
        }
    },

    _showDeviceLinkError(errorDiv, errorText, message) {
        if (errorDiv && errorText) {
            errorText.textContent = message;
            errorDiv.classList.remove('initially-hidden');
        }
    }
};
