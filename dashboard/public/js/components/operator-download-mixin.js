// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { devLogger } from '../utils/dev-logger.js';
import { templateLoader } from '../utils/template-loader.js';
import { webSessionService } from '../utils/web-session-service.js';
import { BEARER_PREFIX } from '../constants/service-client-constants.js';
import { operatorPanelService } from '../utils/operator-panel-service.js';

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

        apiKeyValue.dataset.apiKey = apiKey;
        apiKeyValue.textContent = this._obfuscateApiKey(apiKey);

        if (apiKeyToggle) {
            apiKeyToggle.addEventListener('click', () => {
                const isObfuscated = apiKeyValue.classList.toggle('obfuscated');
                const icon = apiKeyToggle.querySelector('.material-symbols-outlined');
                if (icon) icon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
                apiKeyValue.textContent = isObfuscated
                    ? this._obfuscateApiKey(apiKey)
                    : apiKey;
            });
        }

        if (apiKeyCopy) {
            apiKeyCopy.addEventListener('click', () => {
                this.copyCurlCommand(apiKey, apiKeyCopy);
            });
        }

        if (apiKeyRefresh) {
            apiKeyRefresh.addEventListener('click', () => {
                this.handleRefreshDropKey(apiKeyRefresh, apiKeyValue);
            });
        }
    },

    async handleRefreshDropKey(button, apiKeyValue) {
        const originalIcon = button.innerHTML;
        button.disabled = true;
        button.innerHTML = '<span class="material-symbols-outlined rotating">sync</span>';

        try {
            const response = await fetch('/api/user/me/refresh-drop-key', {
                method: 'POST',
                credentials: 'include'
            });

            const result = await response.json();

            if (!response.ok || !result.success) {
                throw new Error(result.error || 'Failed to refresh drop key');
            }

            const newDropKey = result.drop_key;
            
            apiKeyValue.dataset.apiKey = newDropKey;
            apiKeyValue.textContent = this._obfuscateApiKey(newDropKey);

            webSessionService.setApiKey(newDropKey);

            devLogger.log('[OPERATOR] Drop key refreshed successfully');
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to refresh drop key:', error);
            alert(`Failed to refresh drop key: ${error.message}`);
        } finally {
            button.disabled = false;
            button.innerHTML = originalIcon;
        }
    },

    _bindDeviceLinkGeneration(container, apiKey) {
        const countInput = container.querySelector('#device-link-cmd-count');
        const ttlInput = container.querySelector('#device-link-cmd-ttl');
        const generateBtn = container.querySelector('#device-link-generate-btn');
        const resultDiv = container.querySelector('#device-link-result');
        const curlCmdDiv = container.querySelector('#device-link-curl-cmd');
        const tokenDiv = container.querySelector('#device-link-token');
        const copyCurlBtn = container.querySelector('#device-link-copy-curl');
        const copyTokenBtn = container.querySelector('#device-link-copy-token');
        const errorDiv = container.querySelector('#device-link-generate-error');

        const bindCounter = (decId, incId, input, min, max) => {
            const decBtn = container.querySelector(decId);
            const incBtn = container.querySelector(incId);
            if (decBtn) {
                decBtn.addEventListener('click', () => {
                    const val = parseInt(input.value, 10) || min;
                    input.value = Math.max(min, val - 1);
                });
            }
            if (incBtn) {
                incBtn.addEventListener('click', () => {
                    const val = parseInt(input.value, 10) || min;
                    input.value = Math.min(max, val + 1);
                });
            }
        };

        if (countInput) bindCounter('#device-link-cmd-count-dec', '#device-link-cmd-count-inc', countInput, 1, 10000);
        if (ttlInput) bindCounter('#device-link-cmd-ttl-dec', '#device-link-cmd-ttl-inc', ttlInput, 1, 8760);

        if (generateBtn) {
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
                    const dropUrl = `http://${window.location.hostname}/drop`;
                    const curlCommand = `curl -fsSL ${dropUrl} | sh -s -- ${token}`;

                    if (curlCmdDiv) curlCmdDiv.textContent = curlCommand;
                    if (tokenDiv) tokenDiv.textContent = token;

                    if (copyCurlBtn) {
                        copyCurlBtn.onclick = () => this.copyCurlCommand(curlCommand, copyCurlBtn);
                    }
                    if (copyTokenBtn) {
                        copyTokenBtn.onclick = () => this.copyCurlCommand(token, copyTokenBtn);
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
        }
    },

    showInitialDownloadOverlay() {
        this.expandDownloadSection();
    },

    populateDownloadDetails(overlay, os, arch, cloudMode = false) {
        const apiKey = webSessionService.getApiKey();

        const downloadUrl = `${window.location.origin}/operator/download/${os}/${arch}`;
        const checksumUrl = `${window.location.origin}/operator/download/${os}/${arch}/sha256`;
        const filename = os === 'windows' ? 'g8e-operator.exe' : 'g8e-operator';

        const cloudFlag = '';

        const osNames = { mac: 'macOS', linux: 'Linux' };
        const archLabels = { amd64: 'x64', arm64: 'ARM64', '386': 'x86' };
        const osName = osNames[os] || os;
        const archLabel = archLabels[arch] || arch.toUpperCase();
        const logoMap = { mac: 'apple', linux: 'linux' };
        const logoName = logoMap[os];

        const curlCommand = `curl -fsSL ${downloadUrl} -H "Authorization: Bearer $G8E_DROP_KEY" -o ${filename} && chmod +x ${filename} && ./${filename}`;
        const curlSudoCommand = `sudo curl -fsSL ${downloadUrl} -H "Authorization: Bearer $G8E_DROP_KEY" -o ${filename} && sudo chmod +x ${filename} && sudo ./${filename}`;
        const secureDownloadCommand = `curl -fsSL ${downloadUrl} -H "Authorization: Bearer $G8E_DROP_KEY" -o ${filename} && curl -fsSL ${checksumUrl} -H "Authorization: Bearer $G8E_DROP_KEY" -o ${filename}.sha256`;
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
            directPlatformIcon.src = `/media/${logoName}-logo.png`;
            directPlatformIcon.alt = osName;
            directPlatformIcon.dataset.logo = logoName;
        }
        if (directPlatformName) directPlatformName.textContent = `${osName} ${archLabel}`;
        if (directPlatformFile) directPlatformFile.textContent = filename;
        if (downloadText) downloadText.textContent = `Download for ${osName} ${archLabel}`;

        const methodTabs = overlay.querySelectorAll('.download-method-tab');
        const methodPanels = overlay.querySelectorAll('.download-method-panel');
        methodTabs.forEach(tab => {
            tab.addEventListener('click', () => {
                const method = tab.dataset.method;
                methodTabs.forEach(t => t.classList.remove('active'));
                tab.classList.add('active');
                methodPanels.forEach(panel => {
                    panel.classList.toggle('active', panel.dataset.method === method);
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
            secureApiKeyDisplay.dataset.apiKey = apiKey;
            secureApiKeyDisplay.textContent = '••••••••••••••••';
        }
        if (secureApiKeyCopy) {
            secureApiKeyCopy.onclick = () => this.copyCurlCommand(apiKey, secureApiKeyCopy);
        }
        if (secureApiKeyToggle && secureApiKeyDisplay) {
            secureApiKeyToggle.onclick = () => {
                const isObfuscated = secureApiKeyDisplay.classList.toggle('obfuscated');
                const icon = secureApiKeyToggle.querySelector('.material-symbols-outlined');
                if (icon) icon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
                secureApiKeyDisplay.textContent = isObfuscated ? '••••••••••••••••' : secureApiKeyDisplay.dataset.apiKey;
            };
        }
        if (secureEnvCopy) {
            secureEnvCopy.onclick = () => this.copyCurlCommand('read -s G8E_DROP_KEY && export G8E_DROP_KEY', secureEnvCopy);
        }
        if (secureDownloadCopy) {
            secureDownloadCopy.onclick = () => this.copyCurlCommand(secureDownloadCommand, secureDownloadCopy);
        }
        if (verifyChecksumCopy) {
            verifyChecksumCopy.onclick = () => this.copyCurlCommand(verifyChecksumCommand, verifyChecksumCopy);
        }
        if (secureRunCopy) {
            secureRunCopy.onclick = () => this.copyCurlCommand(runCommand, secureRunCopy);
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
            curlEnvCopy.onclick = () => this.copyCurlCommand('read -s G8E_DROP_KEY && export G8E_DROP_KEY', curlEnvCopy);
        }
        if (curlCopy) {
            curlCopy.onclick = () => {
                const cmd = curlSudoCheckbox?.checked ? curlSudoCommand : curlCommand;
                this.copyCurlCommand(cmd, curlCopy);
            };
        }
        if (apiKeyCopy) {
            apiKeyCopy.onclick = () => this.copyCurlCommand(apiKey, apiKeyCopy);
        }
        if (apiKeyToggle && apiKeyDisplay) {
            apiKeyToggle.onclick = () => {
                const isObfuscated = apiKeyDisplay.classList.toggle('obfuscated');
                const icon = apiKeyToggle.querySelector('.material-symbols-outlined');
                if (icon) icon.textContent = isObfuscated ? 'visibility' : 'visibility_off';
                apiKeyDisplay.textContent = isObfuscated ? '••••••••••••••••' : apiKeyDisplay.dataset.apiKey;
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
            directMethodNote.innerHTML = `After downloading, open terminal and run: <code>chmod +x g8e-operator && ./g8e-operator</code>`;
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
        overlay.dataset.layer = 'platform-selection';

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
        const filename = os === 'windows' ? 'g8e-operator.exe' : 'g8e-operator';
        const curlCommand = `curl -fsSL ${downloadUrl} -H "Authorization: Bearer $G8E_OPERATOR_API_KEY" -o ${filename} && chmod +x ${filename}`;

        const osNames = { mac: 'macOS', linux: 'Linux' };
        const osName = osNames[os] || os;
        const archLabel = arch.toUpperCase();
        const logoMap = { mac: 'apple', linux: 'linux' };
        const logoName = logoMap[os];

        const overlay = document.createElement('div');
        overlay.className = 'download-menu-overlay';
        overlay.dataset.layer = 'download';

        const template = templateLoader.cache.get('operator-download-layer');
        overlay.innerHTML = templateLoader.replace(template, {
            curlCommand,
            apiKey,
            os,
            arch,
            logoName,
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
            curlCopyBtn.addEventListener('click', () => this.copyCurlCommand(curlCommand, curlCopyBtn));
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
            apiKeyCopyBtn.addEventListener('click', () => this.copyCurlCommand(apiKey, apiKeyCopyBtn));
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

    async copyCurlCommand(command, button) {
        try {
            await navigator.clipboard.writeText(command);
            devLogger.log('[OPERATOR] Curl command copied to clipboard');

            const originalText = button.innerHTML;
            button.innerHTML = '<span class="copy-icon material-symbols-outlined">check</span>';
            button.classList.add('copied');

            setTimeout(() => {
                button.innerHTML = originalText;
                button.classList.remove('copied');
            }, 2000);
        } catch (error) {
            devLogger.error('[OPERATOR] Failed to copy curl command:', error);
            alert('Failed to copy to clipboard');
        }
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

    _obfuscateApiKey(apiKey) {
        if (!apiKey || apiKey.length < 20) return '••••••••••••••••';
        return apiKey.substring(0, 12) + '••••••••••••' + apiKey.substring(apiKey.length - 4);
    },

    updatePlatformIcons() {
        const currentTheme = window.ThemeManager ? window.ThemeManager.getTheme() : (document.body.getAttribute('data-theme') || 'dark');
        const isDarkMode = currentTheme !== 'light';
        devLogger.log(' [OPERATOR] Updating platform icons:', { currentTheme, isDarkMode });

        const platformIcons = document.querySelectorAll('.platform-icon[data-logo], .platform-icon-small[data-logo]');
        platformIcons.forEach(icon => {
            const logoType = icon.getAttribute('data-logo');
            let newSrc;
            switch (logoType) {
                case 'apple':
                    newSrc = isDarkMode ? '/media/apple-logo-white.png' : '/media/apple-logo.png';
                    break;
                case 'linux':
                    newSrc = isDarkMode ? '/media/linux-logo-white.png' : '/media/linux-logo.png';
                    break;
            }
            icon.src = newSrc;
        });
    },

    async handleOperatorDownload(platform, apiKey) {
        devLogger.log(`[OPERATOR] Download initiated for platform: ${platform}`);
        try {
            if (!apiKey) {
                devLogger.error('[OPERATOR] No API key available for download');
                alert('No API key found. Please copy an Operator API key from the Operator List above.');
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
            alert(`Failed to download operator: ${error.message}`);
        }
    },

    _initCloudOperatorSection(_container) {
        devLogger.log('[OPERATOR] Cloud operator section initialized');
    }
};
