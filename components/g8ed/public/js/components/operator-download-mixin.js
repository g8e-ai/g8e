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
import { templateLoader } from '../utils/template-loader.js';
import { webSessionService } from '../utils/web-session-service.js';
import { BEARER_PREFIX } from '../constants/service-client-constants.js';
import { operatorPanelService } from '../utils/operator-panel-service.js';
import { notificationService } from '../utils/notification-service.js';
import { bindCounter, obfuscateApiKey, obfuscateCurlCommand, copyToClipboardWithFeedback } from '../utils/ui-utils.js';

/**
 * OperatorDownloadMixin - Operator binary download and platform selection UI.
 *
 * Covers: collapsible download section, platform/arch overlay stack,
 * download detail population, curl command copy, and theme-aware platform icons.
 *
 * Mixed into OperatorPanel via Object.assign(OperatorPanel.prototype, OperatorDownloadMixin).
 */
export const OperatorDownloadMixin = {

    toggleDownloadSection() {
        if (this.downloadSectionExpanded) {
            this.collapseDownloadSection();
        } else {
            this.expandDownloadSection();
        }
    },

    expandDownloadSection() {
        if (!this.downloadCollapsible) return;
        if (!this.downloadSectionPopulated) {
            this.populateDownloadSection();
        }
        this.downloadCollapsible.classList.add('expanded');
        this.downloadSectionExpanded = true;
        devLogger.log('[OPERATOR] Download section expanded');
    },

    collapseDownloadSection() {
        if (!this.downloadCollapsible) return;
        this.downloadCollapsible.classList.remove('expanded');
        this.downloadSectionExpanded = false;
        devLogger.log('[OPERATOR] Download section collapsed');
    },

    populateDownloadSection() {
        if (!this.downloadCollapsibleContent) return;
        devLogger.log('[OPERATOR] Populating collapsible download section');

        const template = templateLoader.cache.get('operator-initial-download-overlay');
        this.downloadCollapsibleContent.innerHTML = templateLoader.replace(template, {});

        const container = this.downloadCollapsibleContent;

        this._populateBinaryDownloadLinks(container);

        this.downloadSectionPopulated = true;
    },

    _populateBinaryDownloadLinks(container) {
        const apiKey = webSessionService.getApiKey();

        const links = container.querySelectorAll('#operator-binary-downloads .operator-download-link');
        links.forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const os = link.getAttribute('data-os');
                const arch = link.getAttribute('data-arch');
                this.handleOperatorDownload(`${os}/${arch}`, apiKey);
                this.collapseDownloadSection();
            });
        });

        this._bindDeployApiKey(container, apiKey);
        this._bindDeviceLinkGeneration(container, apiKey);
    },

    _bindDeployApiKey(container, apiKey) {
        const apiKeyValue = container.querySelector('#deploy-api-key-value');
        const apiKeyToggle = container.querySelector('#deploy-api-key-toggle');
        const apiKeyCopy = container.querySelector('#deploy-api-key-copy');
        const apiKeyRefresh = container.querySelector('#deploy-api-key-refresh');

        if (!apiKeyValue) return;

        apiKeyValue.setAttribute('data-api-key', apiKey);
        apiKeyValue.textContent = obfuscateApiKey(apiKey);

        if (apiKeyToggle) {
            apiKeyToggle.addEventListener('click', () => {
                const isObfuscated = apiKeyValue.classList.toggle('obfuscated');
                const icon = apiKeyToggle.querySelector('.material-symbols-outlined');
                if (icon) icon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
                apiKeyValue.textContent = isObfuscated
                    ? obfuscateApiKey(apiKey)
                    : apiKey;
            });
        }

        if (apiKeyCopy) {
            apiKeyCopy.addEventListener('click', () => {
                copyToClipboardWithFeedback(apiKey, apiKeyCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
            });
        }

        if (apiKeyRefresh) {
            apiKeyRefresh.addEventListener('click', () => {
                this.handleRefreshG8eKey(apiKeyRefresh, apiKeyValue);
            });
        }
    },

    async handleRefreshG8eKey(button, apiKeyValue) {
        const originalIcon = button.innerHTML;
        button.disabled = true;
        button.innerHTML = '<span class="material-symbols-outlined rotating">sync</span>';

        try {
            const response = await fetch('/api/user/me/refresh-g8e-key', {
                method: 'POST',
                credentials: 'include'
            });

            const result = await response.json();

            if (!response.ok || !result.success) {
                throw new Error(result.error || 'Failed to refresh g8e key');
            }

            const newG8eKey = result.g8e_key;

            apiKeyValue.setAttribute('data-api-key', newG8eKey);
            apiKeyValue.textContent = obfuscateApiKey(newG8eKey);

            webSessionService.setApiKey(newG8eKey);

            devLogger.log('[OPERATOR] g8e key refreshed successfully');
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to refresh g8e key:', error);
            notificationService.error(`Failed to refresh g8e key: ${error.message}`);
        } finally {
            button.disabled = false;
            button.innerHTML = originalIcon;
        }
    },

    _bindDeviceLinkGeneration(container, apiKey) {
        const countInput = container.querySelector('#device-link-cmd-count');
        const ttlInput = container.querySelector('#device-link-cmd-ttl');
        const generateBtn = container.querySelector('#device-link-generate-btn');

        if (countInput) bindCounter('#device-link-cmd-count-dec', '#device-link-cmd-count-inc', countInput, 1, 10000, container);
        if (ttlInput) bindCounter('#device-link-cmd-ttl-dec', '#device-link-cmd-ttl-inc', ttlInput, 1, 8760, container);

        if (generateBtn) {
            const resultDiv = container.querySelector('#device-link-result');
            const curlCmdDiv = container.querySelector('#device-link-curl-cmd');
            const tokenDiv = container.querySelector('#device-link-token');
            const copyCurlBtn = container.querySelector('#device-link-copy-curl');
            const copyTokenBtn = container.querySelector('#device-link-copy-token');
            const errorDiv = container.querySelector('#device-link-generate-error');

            generateBtn.addEventListener('click', async () => {
                const maxUses = parseInt(countInput?.value, 10) || 1;
                const expiresInHours = parseInt(ttlInput?.value, 10) || 24;

                if (errorDiv) {
                    errorDiv.textContent = '';
                    errorDiv.classList.add('initially-hidden');
                }
                if (resultDiv) resultDiv.classList.add('initially-hidden');

                generateBtn.disabled = true;
                generateBtn.innerHTML = '<span class="material-symbols-outlined rotating">sync</span>';

                try {
                    const response = await operatorPanelService.createDeviceLink({ maxUses, expiresInHours });
                    const result = await response.json();

                    if (!response.ok || !result.success) {
                        throw new Error(result.error || 'Failed to generate device link');
                    }

                    const token = result.token;
                    const dropUrl = `http://${window.location.hostname}/g8e`;
                    const curlCommand = `curl -fsSL ${dropUrl} | sh -s -- ${token}`;

                    if (curlCmdDiv) {
                        curlCmdDiv.setAttribute('data-curl-command', curlCommand);
                        curlCmdDiv.textContent = obfuscateCurlCommand(curlCommand);
                    }
                    if (tokenDiv) {
                        tokenDiv.setAttribute('data-token', token);
                        tokenDiv.textContent = obfuscateApiKey(token);
                    }

                    if (copyCurlBtn) {
                        copyCurlBtn.onclick = () => copyToClipboardWithFeedback(curlCommand, copyCurlBtn, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
                    }
                    if (copyTokenBtn) {
                        copyTokenBtn.onclick = () => copyToClipboardWithFeedback(token, copyTokenBtn, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
                    }

                    if (resultDiv) resultDiv.classList.remove('initially-hidden');

                } catch (error) {
                    devLogger.error('[OPERATOR] Failed to generate device link:', error);
                    if (errorDiv) {
                        errorDiv.textContent = error.message || 'Failed to generate device link';
                        errorDiv.classList.remove('initially-hidden');
                    }
                } finally {
                    generateBtn.disabled = false;
                    generateBtn.innerHTML = '<span class="material-symbols-outlined">add_link</span> Generate';
                }
            });

        const tokenToggleBtn = container.querySelector('#device-link-token-toggle');
        if (tokenToggleBtn && tokenDiv) {
            tokenToggleBtn.addEventListener('click', () => {
                const isObfuscated = tokenDiv.classList.toggle('obfuscated');
                const icon = tokenToggleBtn.querySelector('.material-symbols-outlined');
                if (icon) icon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
                const token = tokenDiv.getAttribute('data-token');
                tokenDiv.textContent = isObfuscated ? obfuscateApiKey(token) : token;
            });
        }

        const curlToggleBtn = container.querySelector('#device-link-curl-toggle');
        if (curlToggleBtn && curlCmdDiv) {
            curlToggleBtn.addEventListener('click', () => {
                const isObfuscated = curlCmdDiv.classList.toggle('obfuscated');
                const icon = curlToggleBtn.querySelector('.material-symbols-outlined');
                if (icon) icon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
                const curlCommand = curlCmdDiv.getAttribute('data-curl-command');
                curlCmdDiv.textContent = isObfuscated ? obfuscateCurlCommand(curlCommand) : curlCommand;
            });
        }
        }
    },

    showInitialDownloadOverlay() {
        this.expandDownloadSection();
    },

    populateDownloadDetails(overlay, os, arch, cloudMode = false) {
        const apiKey = webSessionService.getApiKey();

        const downloadUrl = `${window.location.origin}/operator/download/${os}/${arch}`;
        const checksumUrl = `${window.location.origin}/operator/download/${os}/${arch}/sha256`;
        const filename = os === 'windows' ? 'g8e.operator.exe' : 'g8e.operator';

        const cloudFlag = '';

        const osNames = { mac: 'macOS', linux: 'Linux' };
        const archLabels = { amd64: 'x64', arm64: 'ARM64', '386': 'x86' };
        const osName = osNames[os] || os;
        const archLabel = archLabels[arch] || arch.toUpperCase();
        const platformIconMap = { mac: 'apple', linux: 'terminal' };
        const platformIcon = platformIconMap[os] || 'terminal';

        const curlCommand = `curl -fsSL ${downloadUrl} -H "Authorization: Bearer $G8E_DOWNLOAD_KEY" -o ${filename} && chmod +x ${filename} && ./${filename}`;
        const curlSudoCommand = `sudo curl -fsSL ${downloadUrl} -H "Authorization: Bearer $G8E_DOWNLOAD_KEY" -o ${filename} && sudo chmod +x ${filename} && sudo ./${filename}`;
        const secureDownloadCommand = `curl -fsSL ${downloadUrl} -H "Authorization: Bearer $G8E_DOWNLOAD_KEY" -o ${filename} && curl -fsSL ${checksumUrl} -H "Authorization: Bearer $G8E_DOWNLOAD_KEY" -o ${filename}.sha256`;
        const verifyChecksumCommand = `sha256sum -c ${filename}.sha256`;
        const runCommand = `chmod +x ${filename} && ./${filename}${cloudFlag}`;

        const secureDownloadCmd = overlay.querySelector('#secure-download-command');
        const verifyChecksumCmd = overlay.querySelector('#verify-checksum-command');
        if (secureDownloadCmd) secureDownloadCmd.textContent = secureDownloadCommand;
        if (verifyChecksumCmd) verifyChecksumCmd.textContent = verifyChecksumCommand;

        const curlCmd = overlay.querySelector('#curl-command');
        const apiKeyDisplay = overlay.querySelector('#api-key-display');
        if (curlCmd) curlCmd.textContent = curlCommand;

        const curlSudoCheckbox = overlay.querySelector('#curl-sudo-checkbox');
        if (curlSudoCheckbox && curlCmd) {
            curlSudoCheckbox.addEventListener('change', () => {
                curlCmd.textContent = curlSudoCheckbox.checked ? curlSudoCommand : curlCommand;
            });
        }

        if (apiKeyDisplay) {
            apiKeyDisplay.dataset.apiKey = apiKey;
            apiKeyDisplay.textContent = '••••••••••••••••';
        }

        const directPlatformIcon = overlay.querySelector('#direct-platform-icon');
        const directPlatformName = overlay.querySelector('#direct-platform-name');
        const directPlatformFile = overlay.querySelector('#direct-platform-file');
        const downloadText = overlay.querySelector('#download-final-text');

        if (directPlatformIcon) {
            directPlatformIcon.textContent = platformIcon;
            directPlatformIcon.setAttribute('data-logo', os);
        }
        if (directPlatformName) directPlatformName.textContent = `${osName} ${archLabel}`;
        if (directPlatformFile) directPlatformFile.textContent = filename;
        if (downloadText) downloadText.textContent = `Download for ${osName} ${archLabel}`;

        const methodTabs = overlay.querySelectorAll('.download-method-tab');
        const methodPanels = overlay.querySelectorAll('.download-method-panel');
        methodTabs.forEach(tab => {
            tab.addEventListener('click', () => {
                const method = tab.getAttribute('data-method');
                methodTabs.forEach(t => t.classList.remove('active'));
                tab.classList.add('active');
                methodPanels.forEach(panel => {
                    panel.classList.toggle('active', panel.getAttribute('data-method') === method);
                });
            });
        });

        const secureApiKeyDisplay = overlay.querySelector('#secure-api-key-display');
        const secureApiKeyToggle = overlay.querySelector('#secure-api-key-toggle');
        const secureApiKeyCopy = overlay.querySelector('#secure-api-key-copy');
        const secureEnvCopy = overlay.querySelector('#secure-env-copy');
        const secureDownloadCopy = overlay.querySelector('#secure-download-copy');
        const verifyChecksumCopy = overlay.querySelector('#verify-checksum-copy');
        const secureRunCopy = overlay.querySelector('#secure-run-copy');

        if (secureApiKeyDisplay) {
            secureApiKeyDisplay.setAttribute('data-api-key', apiKey);
            secureApiKeyDisplay.textContent = '••••••••••••••••';
        }
        if (secureApiKeyCopy) {
            secureApiKeyCopy.onclick = () => copyToClipboardWithFeedback(apiKey, secureApiKeyCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
        }
        if (secureApiKeyToggle && secureApiKeyDisplay) {
            secureApiKeyToggle.onclick = () => {
                const isObfuscated = secureApiKeyDisplay.classList.toggle('obfuscated');
                const icon = secureApiKeyToggle.querySelector('.material-symbols-outlined');
                if (icon) icon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
                secureApiKeyDisplay.textContent = isObfuscated ? '••••••••••••••••' : secureApiKeyDisplay.getAttribute('data-api-key');
            };
        }
        if (secureEnvCopy) {
            secureEnvCopy.onclick = () => copyToClipboardWithFeedback('read -s G8E_DOWNLOAD_KEY && export G8E_DOWNLOAD_KEY', secureEnvCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
        }
        if (secureDownloadCopy) {
            secureDownloadCopy.onclick = () => copyToClipboardWithFeedback(secureDownloadCommand, secureDownloadCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
        }
        if (verifyChecksumCopy) {
            verifyChecksumCopy.onclick = () => copyToClipboardWithFeedback(verifyChecksumCommand, verifyChecksumCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
        }
        if (secureRunCopy) {
            secureRunCopy.onclick = () => copyToClipboardWithFeedback(runCommand, secureRunCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
        }

        const secureRunCmd = overlay.querySelector('.download-method-panel[data-method="secure"] .secure-step:last-child .download-command-box');
        if (secureRunCmd) {
            secureRunCmd.textContent = runCommand;
        }

        const curlEnvCopy = overlay.querySelector('#curl-env-copy');
        const curlCopy = overlay.querySelector('#curl-copy');
        const apiKeyCopy = overlay.querySelector('#api-key-copy');
        const apiKeyToggle = overlay.querySelector('#api-key-toggle');

        if (curlEnvCopy) {
            curlEnvCopy.onclick = () => copyToClipboardWithFeedback('read -s G8E_DOWNLOAD_KEY && export G8E_DOWNLOAD_KEY', curlEnvCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
        }
        if (curlCopy) {
            curlCopy.onclick = () => {
                const cmd = curlSudoCheckbox?.checked ? curlSudoCommand : curlCommand;
                copyToClipboardWithFeedback(cmd, curlCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
            };
        }
        if (apiKeyCopy) {
            apiKeyCopy.onclick = () => copyToClipboardWithFeedback(apiKey, apiKeyCopy, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService));
        }
        if (apiKeyToggle && apiKeyDisplay) {
            apiKeyToggle.onclick = () => {
                const isObfuscated = apiKeyDisplay.classList.toggle('obfuscated');
                const icon = apiKeyToggle.querySelector('.material-symbols-outlined');
                if (icon) icon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
                apiKeyDisplay.textContent = isObfuscated ? '••••••••••••••••' : apiKeyDisplay.getAttribute('data-api-key');
            };
        }

        const downloadBtn = overlay.querySelector('#download-final-btn');
        if (downloadBtn) {
            downloadBtn.onclick = () => {
                const platform = `${os}/${arch}`;
                this.handleOperatorDownload(platform, apiKey);
                this.collapseDownloadSection();
            };
        }

        const directMethodNote = overlay.querySelector('.download-method-panel[data-method="direct"] .method-note');
        if (directMethodNote) {
            directMethodNote.innerHTML = `After downloading, open terminal and run: <code>chmod +x g8e.operator && ./g8e.operator</code>`;
        }

        setTimeout(() => this.updatePlatformIcons(), 10);
    },

    showPlatformSelection(os) {
        devLogger.log(`[OPERATOR] Showing platform selection for ${os}`);
        this.currentOS = os;
        const options = this.platformOptions[os];

        if (!options) {
            devLogger.error(`[OPERATOR] No platform options found for ${os}`);
            return;
        }

        const osNames = { mac: 'macOS', linux: 'Linux' };
        const osName = osNames[os] || os;

        const overlay = document.createElement('div');
        overlay.className = 'download-menu-overlay';
        overlay.setAttribute('data-layer', 'platform-selection');

        const optionsHtml = options.map(option => `
            <button class="platform-option-btn" data-arch="${option.arch}">
                <span class="platform-option-label">
                    <span>${option.label}</span>
                </span>
                <span class="platform-option-arrow">→</span>
            </button>
        `).join('');

        const template = templateLoader.cache.get('operator-platform-selection');
        overlay.innerHTML = templateLoader.replace(template, { optionsHtml });

        const drawerContent = document.querySelector('.operator-drawer-content');
        if (!drawerContent) {
            devLogger.error('[OPERATOR] Drawer content not found');
            return;
        }

        drawerContent.appendChild(overlay);

        const backBtn = overlay.querySelector('.download-menu-back');
        backBtn.addEventListener('click', () => this.closeCurrentOverlay());

        const platformBtns = overlay.querySelectorAll('.platform-option-btn');
        platformBtns.forEach(btn => {
            btn.addEventListener('click', () => {
                const arch = btn.getAttribute('data-arch');
                this.showDownloadLayer(os, arch);
            });
        });

        this.downloadMenuStack.push(overlay);
        setTimeout(() => overlay.classList.add('active'), 10);
    },

    showDownloadLayer(os, arch) {
        devLogger.log(`[OPERATOR] Showing download layer for ${os}/${arch}`);
        this.currentPlatform = arch;

        const apiKey = webSessionService.getApiKey() || 'YOUR_API_KEY';

        const downloadUrl = `${window.location.origin}/operator/download/${os}/${arch}`;
        const filename = os === 'windows' ? 'g8e.operator.exe' : 'g8e.operator';
        const curlCommand = `curl -fsSL ${downloadUrl} -H "Authorization: Bearer $G8E_OPERATOR_API_KEY" -o ${filename} && chmod +x ${filename}`;

        const osNames = { mac: 'macOS', linux: 'Linux' };
        const osName = osNames[os] || os;
        const archLabel = arch.toUpperCase();
        const platformIconMap = { mac: 'apple', linux: 'terminal' };
        const platformIcon = platformIconMap[os] || 'terminal';

        const overlay = document.createElement('div');
        overlay.className = 'download-menu-overlay';
        overlay.setAttribute('data-layer', 'download');

        const template = templateLoader.cache.get('operator-download-layer');
        overlay.innerHTML = templateLoader.replace(template, {
            curlCommand,
            apiKey,
            os,
            arch,
            platformIcon,
            osName,
            archLabel
        });

        const drawerContent = document.querySelector('.operator-drawer-content');
        if (!drawerContent) {
            devLogger.error('[OPERATOR] Drawer content not found');
            return;
        }

        drawerContent.appendChild(overlay);

        const backBtn = overlay.querySelector('.download-menu-back');
        backBtn.addEventListener('click', () => this.closeCurrentOverlay());

        const curlCopyBtn = overlay.querySelector('.curl-copy-btn');
        if (curlCopyBtn) {
            curlCopyBtn.addEventListener('click', () => copyToClipboardWithFeedback(curlCommand, curlCopyBtn, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService)));
        }

        const apiKeyToggleBtn = overlay.querySelector('.api-key-toggle-btn');
        const apiKeyText = overlay.querySelector('.api-key-text');
        if (apiKeyToggleBtn && apiKeyText) {
            apiKeyToggleBtn.addEventListener('click', () => {
                const isObfuscated = apiKeyText.classList.toggle('obfuscated');
                const toggleText = apiKeyToggleBtn.querySelector('.toggle-text');
                const toggleIcon = apiKeyToggleBtn.querySelector('.toggle-icon');
                if (toggleText) toggleText.textContent = isObfuscated ? 'Show' : 'Hide';
                if (toggleIcon) toggleIcon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
            });
        }

        const apiKeyCopyBtn = overlay.querySelector('.api-key-copy-btn');
        if (apiKeyCopyBtn) {
            apiKeyCopyBtn.addEventListener('click', () => copyToClipboardWithFeedback(apiKey, apiKeyCopyBtn, devLogger.log.bind(devLogger, '[OPERATOR]'), notificationService.error.bind(notificationService)));
        }

        const downloadBtn = overlay.querySelector('.download-direct-btn');
        if (downloadBtn) {
            downloadBtn.addEventListener('click', () => {
                const platform = `${os}/${arch}`;
                this.handleOperatorDownload(platform);
                this.closeAllOverlays();
            });
        }

        this.downloadMenuStack.push(overlay);
        setTimeout(() => {
            overlay.classList.add('active');
            this.updatePlatformIcons();
        }, 10);
    },

    closeCurrentOverlay() {
        if (this.downloadMenuStack.length === 0) return;
        const overlay = this.downloadMenuStack.pop();
        overlay.classList.remove('active');
        setTimeout(() => overlay.remove(), 300);
    },

    closeAllOverlays() {
        while (this.downloadMenuStack.length > 0) {
            const overlay = this.downloadMenuStack.pop();
            overlay.classList.remove('active');
            setTimeout(() => overlay.remove(), 300);
        }
        this.currentOS = null;
        this.currentPlatform = null;
    },

    _isTestEnvironment() {
        // Explicit flag takes precedence (set in tests)
        if (this._isTestEnv !== undefined) {
            return this._isTestEnv;
        }
        // Fallback to environment detection for backward compatibility
        return typeof window !== 'undefined' && (window.__vitest__ || navigator.userAgent.includes('jsdom'));
    },

    /**
     * Explicitly set test environment mode.
     * Used in tests to avoid fragile environment detection.
     * @param {boolean} isTestEnv - Whether running in test environment
     */
    _setTestEnvironment(isTestEnv) {
        this._isTestEnv = isTestEnv;
    },

    updatePlatformIcons() {
        const platformIcons = document.querySelectorAll('.platform-icon[data-logo], .platform-icon-small[data-logo]');
        platformIcons.forEach(icon => {
            const logoType = icon.getAttribute('data-logo');
            const iconMap = { mac: 'apple', linux: 'terminal' };
            icon.textContent = iconMap[logoType] || 'terminal';
        });
    },

    async handleOperatorDownload(platform, apiKey) {
        devLogger.log(`[OPERATOR] Download initiated for platform: ${platform}`);
        try {
            if (!apiKey) {
                devLogger.error('[OPERATOR] No API key available for download');
                notificationService.warning('No API key found. Please copy an Operator API key from the Operator List above.');
                return;
            }

            const downloadUrl = `/operator/download/${platform}`;
            devLogger.log(`[OPERATOR] Downloading from: ${downloadUrl}`);

            const response = await fetch(downloadUrl, {
                method: 'GET',
                headers: { 'Authorization': `${BEARER_PREFIX}${apiKey}` },
                credentials: 'include'
            });

            if (!response.ok) {
                throw new Error(`Download failed with status: ${response.status}`);
            }

            const blob = await response.blob();

            if (this._isTestEnvironment()) {
                devLogger.log(`[OPERATOR] Test environment detected - skipping anchor click for ${platform}, blob size: ${blob.size} bytes`);
                return;
            }

            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.classList.add('initially-hidden');
            a.href = url;
            a.download = platform === 'windows' ? 'operator.exe' : 'operator';
            document.body.appendChild(a);
            a.click();
            window.URL.revokeObjectURL(url);
            document.body.removeChild(a);

            devLogger.log(`[OPERATOR] Download initiated successfully for ${platform}`);
        } catch (error) {
            devLogger.error('[OPERATOR] Download failed:', error);
            notificationService.error(`Failed to download operator: ${error.message}`);
        }
    },

    _initCloudOperatorSection(_container) {
        devLogger.log('[OPERATOR] Cloud operator section initialized');
    }
};
