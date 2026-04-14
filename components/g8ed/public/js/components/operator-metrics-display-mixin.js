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
import { OperatorMetrics } from './operator-metrics.js';
import { webSessionService } from '../utils/web-session-service.js';

/**
 * OperatorMetricsDisplayMixin - Metrics panel update, status display, and panel state.
 *
 * Covers: updateMetrics, updateExpandedDetails, updateStatus, clearPanelMetrics,
 * updatePanelStatusFromOperatorCounts, getProgressLevel, getLatencyQuality,
 * IP obfuscation and visibility toggle, metrics details expand/collapse,
 * and API key / command display helpers.
 *
 * Mixed into OperatorPanel via Object.assign(OperatorPanel.prototype, OperatorMetricsDisplayMixin).
 */
export const OperatorMetricsDisplayMixin = {

    updateStatus(status) {
        const statusLower = (status).toLowerCase();

        if (this.statusElement) {
            this.statusElement.textContent = status.toUpperCase();
            this.statusElement.className = `metric-card-value status-${statusLower}`;
        }

        if (this.panel) {
            const isOnline = [OperatorStatus.ACTIVE, OperatorStatus.BOUND].includes(status);
            this.panel.classList.toggle('offline', !isOnline);
        }
    },

    updateMetrics(data) {
        try {
            const operatorMetrics = new OperatorMetrics(data);

            if (!operatorMetrics.isValid()) {
                devLogger.warn('[OPERATOR] Invalid or incomplete metrics data received:', data);
                return;
            }

            if (this.hostnameElement) {
                this.hostnameElement.textContent = operatorMetrics.getHostnameDisplay();
            }

            if (this.cpuElement && operatorMetrics.cpu !== undefined) {
                this.cpuElement.textContent = operatorMetrics.getCpuDisplay();
                if (this.cpuBarElement) {
                    this.cpuBarElement.style.setProperty('--progress-width', `${Math.min(operatorMetrics.cpu, 100)}%`);
                    this.cpuBarElement.className = `metric-progress-fill ${this.getProgressLevel(operatorMetrics.cpu)}`;
                }
            }

            if (this.memoryElement && operatorMetrics.memory !== undefined) {
                this.memoryElement.textContent = operatorMetrics.getMemoryDisplay();
                if (this.memoryBarElement) {
                    this.memoryBarElement.style.setProperty('--progress-width', `${Math.min(operatorMetrics.memory, 100)}%`);
                    this.memoryBarElement.className = `metric-progress-fill ${this.getProgressLevel(operatorMetrics.memory)}`;
                }
            }

            if (this.diskElement && operatorMetrics.disk !== undefined) {
                this.diskElement.textContent = operatorMetrics.getDiskDisplay();
                if (this.diskBarElement) {
                    this.diskBarElement.style.setProperty('--progress-width', `${Math.min(operatorMetrics.disk, 100)}%`);
                    this.diskBarElement.className = `metric-progress-fill ${this.getProgressLevel(operatorMetrics.disk)}`;
                }
            }

            if (this.latencyElement && operatorMetrics.networkLatency !== undefined) {
                this.latencyElement.textContent = operatorMetrics.getNetworkDisplay();
                if (this.latencyIndicator) {
                    const quality = this.getLatencyQuality(operatorMetrics.networkLatency);
                    this.latencyIndicator.className = `latency-indicator ${quality}`;
                }
            }

            this.updateExpandedDetails(operatorMetrics);

        } catch (error) {
            devLogger.error('[OPERATOR] Error updating Operator metrics:', error);
        }
    },

    updateExpandedDetails(operatorMetrics) {
        if (this.detailHostElement) {
            this.detailHostElement.textContent = operatorMetrics.hostname || ' - ';
        }
        if (this.osElement) {
            this.osElement.textContent = operatorMetrics.osDetails?.distro || operatorMetrics.os || ' - ';
        }
        if (this.kernelElement) {
            this.kernelElement.textContent = operatorMetrics.osDetails?.kernel || ' - ';
        }
        if (this.archElement) {
            this.archElement.textContent = operatorMetrics.architecture || ' - ';
        }
        if (this.uptimeElement) {
            this.uptimeElement.textContent = operatorMetrics.getUptimeDisplay();
        }

        if (this.usernameElement) {
            this.usernameElement.textContent = operatorMetrics.userDetails?.username || operatorMetrics.currentUser || ' - ';
        }
        if (this.shellElement) {
            const shell = operatorMetrics.userDetails?.shell || ' - ';
            this.shellElement.textContent = shell.split('/').pop() || shell;
        }
        if (this.homeElement) {
            this.homeElement.textContent = operatorMetrics.userDetails?.home || ' - ';
        }
        if (this.timezoneElement) {
            this.timezoneElement.textContent = operatorMetrics.environment?.timezone || ' - ';
        }

        if (this.diskUsageElement && operatorMetrics.diskDetails) {
            const used = operatorMetrics.diskDetails.used_gb || operatorMetrics.diskUsedGb || 0;
            const total = operatorMetrics.diskDetails.total_gb || operatorMetrics.diskTotalGb || 0;
            this.diskUsageElement.textContent = `${used.toFixed(1)} / ${total.toFixed(1)} GB`;
        }
        if (this.diskVisualElement && operatorMetrics.disk !== undefined) {
            this.diskVisualElement.style.setProperty('--progress-width', `${Math.min(operatorMetrics.disk, 100)}%`);
            const levelClass = operatorMetrics.disk >= 90 ? 'level-critical' :
                              operatorMetrics.disk >= 75 ? 'level-warning' : '';
            this.diskVisualElement.className = `storage-bar-fill ${levelClass}`;
        }

        if (this.memoryUsageElement && operatorMetrics.memoryDetails) {
            const used = operatorMetrics.memoryDetails.used_mb || operatorMetrics.memoryUsedMb || 0;
            const total = operatorMetrics.memoryDetails.total_mb || operatorMetrics.memoryTotalMb || 0;
            const usedGB = (used / 1024).toFixed(1);
            const totalGB = (total / 1024).toFixed(1);
            this.memoryUsageElement.textContent = `${usedGB} / ${totalGB} GB`;
        }
        if (this.memoryVisualElement && operatorMetrics.memory !== undefined) {
            this.memoryVisualElement.style.setProperty('--progress-width', `${Math.min(operatorMetrics.memory, 100)}%`);
            const levelClass = operatorMetrics.memory >= 90 ? 'level-critical' :
                              operatorMetrics.memory >= 75 ? 'level-warning' : '';
            this.memoryVisualElement.className = `storage-bar-fill ${levelClass}`;
        }

        if (this.publicIpElement) {
            this.actualPublicIp = operatorMetrics.publicIp || null;
            this._renderPublicIp();
        }
        if (this.latencyDetailElement) {
            this.latencyDetailElement.textContent = operatorMetrics.getNetworkDisplay();
        }

        if (this.pwdElement) {
            this.pwdElement.textContent = operatorMetrics.environment?.pwd || ' - ';
        }
        if (this.langElement) {
            this.langElement.textContent = operatorMetrics.environment?.lang || ' - ';
        }
    },

    getProgressLevel(percent) {
        if (percent >= 80) return 'level-high';
        if (percent >= 50) return 'level-medium';
        return 'level-low';
    },

    getLatencyQuality(latencyMs) {
        if (latencyMs <= 1) return 'excellent';
        if (latencyMs <= 5) return 'good';
        if (latencyMs <= 20) return 'fair';
        return 'poor';
    },

    clearPanelMetrics() {
        if (this.hostnameElement) this.hostnameElement.textContent = ' - ';
        if (this.cpuElement) this.cpuElement.textContent = ' - ';
        if (this.memoryElement) this.memoryElement.textContent = ' - ';
        if (this.latencyElement) this.latencyElement.textContent = ' - ';
    },

    updatePanelStatusFromOperatorCounts() {
        const currentWebSessionId = webSessionService.getWebSessionId();
        const boundOperator = this.operators.find(op => op.status === OperatorStatus.BOUND && op.web_session_id === currentWebSessionId);
        const hasBoundOperator = !!boundOperator;

        if (hasBoundOperator && this.isConnected) {
            const status = boundOperator.status || OperatorStatus.BOUND;
            this.updateStatus(status);
            return;
        }

        if (!hasBoundOperator && this.isConnected) {
            this.clearPanelMetrics();
            this.isConnected = false;
            this.lastHeartbeat = null;
        }

        this.updateStatus(OperatorStatus.OFFLINE);
    },

    toggleMetricsDetails() {
        this.metricsDetailsExpanded = !this.metricsDetailsExpanded;

        if (this.metricsDetails) {
            if (this.metricsDetailsExpanded) {
                this.metricsDetails.classList.add('expanded');
            } else {
                this.metricsDetails.classList.remove('expanded');
            }
        }

        if (this.metricsToggle) {
            if (this.metricsDetailsExpanded) {
                this.metricsToggle.classList.add('expanded');
            } else {
                this.metricsToggle.classList.remove('expanded');
            }
        }
    },

    _obfuscateIp(ip) {
        if (!ip) return ' - ';
        const parts = ip.split('.');
        if (parts.length !== 4) return '•••••••••';
        return `${parts[0]}.${'•'.repeat(parts[1].length)}.${'•'.repeat(parts[2].length)}.${'•'.repeat(parts[3].length)}`;
    },

    _renderPublicIp() {
        if (!this.publicIpElement) return;
        if (!this.actualPublicIp) {
            this.publicIpElement.textContent = ' - ';
            return;
        }
        this.publicIpElement.textContent = this.ipVisible
            ? this.actualPublicIp
            : this._obfuscateIp(this.actualPublicIp);
    },

    _toggleIpVisibility() {
        this.ipVisible = !this.ipVisible;
        this._renderPublicIp();
        if (this.ipVisibilityToggle) {
            this.ipVisibilityToggle.textContent = this.ipVisible ? 'visibility' : 'visibility_off';
            this.ipVisibilityToggle.title = this.ipVisible ? 'Hide IP' : 'Show IP';
        }
    },

    _showCopyFeedback(btn) {
        const icon = btn.querySelector('.material-symbols-outlined');
        if (icon) {
            const originalText = icon.textContent;
            icon.textContent = 'check';
            setTimeout(() => {
                icon.textContent = originalText;
            }, 1500);
        }
    },

    populateApiKey() {
        this.apiKeyVisible = false;
        this.commandVisible = false;
        if (this.apiKeyInput) this.apiKeyInput.type = 'password';
        if (this.commandInput) this.commandInput.type = 'password';

        if (this.apiKeyInput) {
            const apiKey = webSessionService.getApiKey();
            if (apiKey) {
                this.apiKeyInput.value = apiKey;
                this._panelApiKey = apiKey;

                const command = `./g8e.operator -k ${apiKey}`;
                if (this.commandInput) {
                    this.commandInput.value = command;
                    this._panelCommand = command;
                }

                devLogger.log('[OPERATOR] API key and command populated in panel');
            } else {
                this.apiKeyInput.value = '';
                this._panelApiKey = null;
                this.apiKeyInput.placeholder = 'No API key available';

                if (this.commandInput) {
                    this.commandInput.value = '';
                    this._panelCommand = null;
                    this.commandInput.placeholder = 'No command available';
                }

                devLogger.warn('[OPERATOR] API key not found in session');
            }
        }
    },

    toggleApiKeyVisibility() {
        if (!this.apiKeyInput) return;
        this.apiKeyVisible = !this.apiKeyVisible;

        if (this.apiKeyVisible) {
            this.apiKeyInput.type = 'text';
            if (this.apiKeyToggleBtn) {
                const icon = this.apiKeyToggleBtn.querySelector('.toggle-icon');
                if (icon) icon.textContent = 'visibility_off';
                this.apiKeyToggleBtn.classList.add('visible');
                this.apiKeyToggleBtn.title = 'Hide API key';
            }
        } else {
            this.apiKeyInput.type = 'password';
            if (this.apiKeyToggleBtn) {
                const icon = this.apiKeyToggleBtn.querySelector('.toggle-icon');
                if (icon) icon.textContent = 'visibility';
                this.apiKeyToggleBtn.classList.remove('visible');
                this.apiKeyToggleBtn.title = 'Show API key';
            }
        }

        devLogger.log('[OPERATOR] API key visibility toggled:', this.apiKeyVisible ? 'visible' : 'hidden');
    },

    toggleCommandVisibility() {
        if (!this.commandInput) return;
        this.commandVisible = !this.commandVisible;

        if (this.commandVisible) {
            this.commandInput.type = 'text';
            if (this.commandToggleBtn) {
                const icon = this.commandToggleBtn.querySelector('.toggle-icon');
                if (icon) icon.textContent = 'visibility_off';
                this.commandToggleBtn.classList.add('visible');
                this.commandToggleBtn.title = 'Hide command';
            }
        } else {
            this.commandInput.type = 'password';
            if (this.commandToggleBtn) {
                const icon = this.commandToggleBtn.querySelector('.toggle-icon');
                if (icon) icon.textContent = 'visibility';
                this.commandToggleBtn.classList.remove('visible');
                this.commandToggleBtn.title = 'Show command';
            }
        }

        devLogger.log('[OPERATOR] Command visibility toggled:', this.commandVisible ? 'visible' : 'hidden');
    },

    async copyApiKeyToClipboard() {
        const apiKey = this._panelApiKey || this.apiKeyInput?.value;
        if (!apiKey) {
            devLogger.warn('[OPERATOR] No API key to copy');
            return;
        }

        try {
            await navigator.clipboard.writeText(apiKey);
            devLogger.log('[OPERATOR] API key copied to clipboard');

            if (this.apiKeyCopyBtn) {
                const originalIcon = this.apiKeyCopyBtn.querySelector('.copy-icon')?.textContent;
                const copyIcon = this.apiKeyCopyBtn.querySelector('.copy-icon');
                if (copyIcon) {
                    copyIcon.textContent = 'check';
                    this.apiKeyCopyBtn.classList.add('copied');
                    setTimeout(() => {
                        copyIcon.textContent = originalIcon || 'content_copy';
                        this.apiKeyCopyBtn.classList.remove('copied');
                    }, 2000);
                }
            }
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to copy API key:', error);
            if (this.apiKeyInput) {
                this.apiKeyInput.select();
                this.apiKeyInput.setSelectionRange(0, 99999);
            }
        }
    },

    async copyCommandToClipboard() {
        const command = this._panelCommand || this.commandInput?.value;
        if (!command) {
            devLogger.warn('[OPERATOR] No command to copy');
            return;
        }

        try {
            await navigator.clipboard.writeText(command);
            devLogger.log('[OPERATOR] Command copied to clipboard');

            if (this.commandCopyBtn) {
                const originalIcon = this.commandCopyBtn.querySelector('.copy-icon')?.textContent;
                const copyIcon = this.commandCopyBtn.querySelector('.copy-icon');
                if (copyIcon) {
                    copyIcon.textContent = 'check';
                    this.commandCopyBtn.classList.add('copied');
                    setTimeout(() => {
                        copyIcon.textContent = originalIcon || 'content_copy';
                        this.commandCopyBtn.classList.remove('copied');
                    }, 2000);
                }
            }
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to copy command:', error);
            if (this.commandInput) {
                this.commandInput.select();
                this.commandInput.setSelectionRange(0, 99999);
            }
        }
    },

    findValueInObject(obj, paths) {
        if (!obj || typeof obj !== 'object') return undefined;
        for (const path of paths) {
            if (path.includes('.')) {
                const parts = path.split('.');
                let current = obj;
                let found = true;
                for (const part of parts) {
                    if (current && typeof current === 'object' && part in current) {
                        current = current[part];
                    } else {
                        found = false;
                        break;
                    }
                }
                if (found && current !== undefined) return current;
            } else {
                if (path in obj && obj[path] !== undefined) return obj[path];
            }
        }
        return undefined;
    }
};
