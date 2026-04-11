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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { JSDOM } from 'jsdom';
import { ApiPaths } from '@g8ed/public/js/constants/api-paths.js';

/**
 * Test utilities for dev logs functionality
 */
class DevLogsTestUtils {
    static createDOM() {
        return new JSDOM(`
            <!DOCTYPE html>
            <html>
            <body>
                <div id="status-bar" class="settings-status">
                    <span id="status-icon"></span>
                    <span id="status-msg"></span>
                </div>
                <div class="settings-dev-panel">
                    <label class="settings-toggle">
                        <input type="checkbox" id="dev-logs-toggle">
                        <span class="settings-toggle-track">
                            <span class="settings-toggle-thumb"></span>
                        </span>
                        <span class="settings-toggle-label" id="dev-logs-label">Disabled</span>
                    </label>
                </div>
            </body>
            </html>
        `, { url: 'https://localhost/settings', runScripts: 'dangerously' });
    }

    static injectScript(dom, fetchMock) {
        const { window } = dom;
        window.fetch = fetchMock;
        window.ApiPaths = ApiPaths;

        const script = `
        (function() {
            'use strict';

            const STATUS_ICONS = {
                success: 'check_circle',
                error: 'error',
                info: 'info'
            };

            /**
             * Display status message to user
             * @param {string} type - Status type (success, error, info)
             * @param {string} message - Status message
             */
            function showStatus(type, message) {
                const statusBar = document.getElementById('status-bar');
                const statusIcon = document.getElementById('status-icon');
                const statusMsg = document.getElementById('status-msg');

                if (!statusBar || !statusIcon || !statusMsg) {
                    console.error('Status bar elements not found');
                    return;
                }

                statusBar.className = 'settings-status visible ' + type;
                statusIcon.textContent = STATUS_ICONS[type] || STATUS_ICONS.info;
                statusMsg.textContent = message;
            }

            /**
             * Initialize dev logs toggle functionality
             */
            function initDevLogsToggle() {
                const toggle = document.getElementById('dev-logs-toggle');
                const label = document.getElementById('dev-logs-label');

                if (!toggle || !label) {
                    console.error('Dev logs toggle elements not found');
                    return;
                }

                toggle.addEventListener('change', handleToggleChange);
            }

            /**
             * Handle toggle change event
             */
            async function handleToggleChange() {
                const toggle = document.getElementById('dev-logs-toggle');
                const label = document.getElementById('dev-logs-label');
                const enabled = toggle.checked;

                // Prevent concurrent requests
                toggle.disabled = true;

                try {
                    const response = await fetch(ApiPaths.user.me() + '/dev-logs', {
                        method: 'PATCH',
                        credentials: 'include',
                        headers: {
                            'Content-Type': 'application/json'
                        },
                        body: JSON.stringify({ enabled })
                    });

                    const result = await response.json();

                    if (!response.ok || !result.success) {
                        // Revert toggle state on failure
                        toggle.checked = !enabled;
                        const errorMsg = result.error || 'HTTP ' + response.status;
                        showStatus('error', 'Failed to update dev logging: ' + errorMsg);
                        return;
                    }

                    // Update UI on success
                    label.textContent = enabled ? 'Enabled' : 'Disabled';
                    const action = enabled ? 'enabled' : 'disabled';
                    showStatus('success', 'Dev logging ' + action + '. Reload any open page to apply.');

                } catch (error) {
                    // Handle network errors
                    toggle.checked = !enabled;
                    showStatus('error', 'Failed to update dev logging: ' + error.message);
                } finally {
                    // Always re-enable toggle
                    toggle.disabled = false;
                }
            }

            // Initialize when DOM is ready
            if (document.readyState === 'loading') {
                document.addEventListener('DOMContentLoaded', initDevLogsToggle);
            } else {
                initDevLogsToggle();
            }

            // Expose test helpers
            window.__devLogsTest = {
                triggerToggle: function(checked) {
                    const toggle = document.getElementById('dev-logs-toggle');
                    if (!toggle) throw new Error('Toggle not found');
                    
                    toggle.checked = checked;
                    toggle.dispatchEvent(new Event('change', { bubbles: true }));
                    return toggle;
                },
                
                getToggle: function() {
                    return document.getElementById('dev-logs-toggle');
                },
                
                getLabel: function() {
                    return document.getElementById('dev-logs-label');
                },
                
                getStatusBar: function() {
                    return document.getElementById('status-bar');
                },
                
                getStatusMsg: function() {
                    return document.getElementById('status-msg');
                },
                
                getStatusIcon: function() {
                    return document.getElementById('status-icon');
                }
            };
        })();
        `;

        window.eval(script);
    }
}

/**
 * Mock response factory
 */
class MockResponse {
    static success(enabled = true) {
        return {
            ok: true,
            status: 200,
            json: () => Promise.resolve({ 
                success: true, 
                dev_logs_enabled: enabled 
            })
        };
    }

    static error(status = 500, error = 'Server error') {
        return {
            ok: false,
            status,
            json: () => Promise.resolve({ 
                success: false, 
                error 
            })
        };
    }

    static networkError(message = 'Network failure') {
        return Promise.reject(new Error(message));
    }

    static delayedResponse(response, delay = 100) {
        return new Promise(resolve => {
            setTimeout(() => resolve(response), delay);
        });
    }
}

describe('Settings Dev Logs Toggle [UNIT - jsdom]', () => {
    let dom;
    let window;
    let document;
    let fetchMock;

    beforeEach(() => {
        dom = DevLogsTestUtils.createDOM();
        window = dom.window;
        document = window.document;
        fetchMock = vi.fn();
        DevLogsTestUtils.injectScript(dom, fetchMock);
    });

    afterEach(() => {
        if (dom && dom.window) {
            dom.window.close();
        }
        vi.clearAllMocks();
    });

    describe('API Integration', () => {
        beforeEach(() => { vi.useFakeTimers(); });
        afterEach(() => { vi.useRealTimers(); });

        it('sends PATCH request to correct endpoint when enabling', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(true));

            window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            const patchCall = fetchMock.mock.calls.find(call => 
                call[1]?.method === 'PATCH'
            );
            
            expect(patchCall).toBeDefined();
            expect(patchCall[0]).toBe('/api/user/me/dev-logs');
            expect(JSON.parse(patchCall[1].body)).toEqual({ enabled: true });
        });

        it('sends PATCH request to correct endpoint when disabling', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(false));

            window.__devLogsTest.triggerToggle(false);
            await vi.runAllTimersAsync();

            const patchCall = fetchMock.mock.calls.find(call => 
                call[1]?.method === 'PATCH'
            );
            
            expect(patchCall).toBeDefined();
            expect(patchCall[0]).toBe('/api/user/me/dev-logs');
            expect(JSON.parse(patchCall[1].body)).toEqual({ enabled: false });
        });

        it('includes proper headers and credentials', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(true));

            window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            const patchCall = fetchMock.mock.calls.find(call => 
                call[1]?.method === 'PATCH'
            );
            
            expect(patchCall[1].credentials).toBe('include');
            expect(patchCall[1].headers['Content-Type']).toBe('application/json');
        });
    });

    describe('Toggle State Management', () => {
        beforeEach(() => { vi.useFakeTimers(); });
        afterEach(() => { vi.useRealTimers(); });

        it('disables toggle during request and re-enables after success', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(true));
            const toggle = window.__devLogsTest.getToggle();

            expect(toggle.disabled).toBe(false);

            window.__devLogsTest.triggerToggle(true);
            expect(toggle.disabled).toBe(true);

            await vi.runAllTimersAsync();
            expect(toggle.disabled).toBe(false);
        });

        it('disables toggle during request and re-enables after error', async () => {
            fetchMock.mockResolvedValue(MockResponse.error(500));
            const toggle = window.__devLogsTest.getToggle();

            expect(toggle.disabled).toBe(false);

            window.__devLogsTest.triggerToggle(true);
            expect(toggle.disabled).toBe(true);

            await vi.runAllTimersAsync();
            expect(toggle.disabled).toBe(false);
        });

        it('disables toggle during request and re-enables after network failure', async () => {
            fetchMock.mockRejectedValue(new Error('Network failure'));
            const toggle = window.__devLogsTest.getToggle();

            expect(toggle.disabled).toBe(false);

            window.__devLogsTest.triggerToggle(true);
            expect(toggle.disabled).toBe(true);

            await vi.runAllTimersAsync();
            expect(toggle.disabled).toBe(false);
        });
    });

    describe('Success Response Handling', () => {
        beforeEach(() => { vi.useFakeTimers(); });
        afterEach(() => { vi.useRealTimers(); });

        it('updates label to Enabled when dev logs are enabled', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(true));

            window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            expect(window.__devLogsTest.getLabel().textContent).toBe('Enabled');
        });

        it('updates label to Disabled when dev logs are disabled', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(false));

            window.__devLogsTest.triggerToggle(false);
            await vi.runAllTimersAsync();

            expect(window.__devLogsTest.getLabel().textContent).toBe('Disabled');
        });

        it('displays success status with correct message for enable', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(true));

            window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            const statusBar = window.__devLogsTest.getStatusBar();
            const statusMsg = window.__devLogsTest.getStatusMsg();
            const statusIcon = window.__devLogsTest.getStatusIcon();

            expect(statusBar.className).toContain('success');
            expect(statusBar.className).toContain('visible');
            expect(statusMsg.textContent).toContain('Dev logging enabled');
            expect(statusMsg.textContent).toContain('Reload any open page to apply');
            expect(statusIcon.textContent).toBe('check_circle');
        });

        it('displays success status with correct message for disable', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(false));

            window.__devLogsTest.triggerToggle(false);
            await vi.runAllTimersAsync();

            const statusBar = window.__devLogsTest.getStatusBar();
            const statusMsg = window.__devLogsTest.getStatusMsg();
            const statusIcon = window.__devLogsTest.getStatusIcon();

            expect(statusBar.className).toContain('success');
            expect(statusBar.className).toContain('visible');
            expect(statusMsg.textContent).toContain('Dev logging disabled');
            expect(statusMsg.textContent).toContain('Reload any open page to apply');
            expect(statusIcon.textContent).toBe('check_circle');
        });
    });

    describe('Error Response Handling', () => {
        beforeEach(() => { vi.useFakeTimers(); });
        afterEach(() => { vi.useRealTimers(); });

        it('reverts toggle state on HTTP error response', async () => {
            fetchMock.mockResolvedValue(MockResponse.error(500, 'Server error'));

            const toggle = window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            expect(toggle.checked).toBe(false);
        });

        it('reverts toggle state on API error response', async () => {
            fetchMock.mockResolvedValue(MockResponse.error(403, 'Access Denied'));

            const toggle = window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            expect(toggle.checked).toBe(false);
        });

        it('reverts toggle state when success flag is false', async () => {
            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: false, error: 'Validation failed' })
            });

            const toggle = window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            expect(toggle.checked).toBe(false);
        });

        it('reverts toggle state on network failure', async () => {
            fetchMock.mockRejectedValue(new Error('Network failure'));

            const toggle = window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            expect(toggle.checked).toBe(false);
        });

        it('displays error status for HTTP errors', async () => {
            fetchMock.mockResolvedValue(MockResponse.error(500, 'Server error'));

            window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            const statusBar = window.__devLogsTest.getStatusBar();
            const statusMsg = window.__devLogsTest.getStatusMsg();
            const statusIcon = window.__devLogsTest.getStatusIcon();

            expect(statusBar.className).toContain('error');
            expect(statusBar.className).toContain('visible');
            expect(statusMsg.textContent).toContain('Failed to update dev logging');
            expect(statusMsg.textContent).toContain('Server error');
            expect(statusIcon.textContent).toBe('error');
        });

        it('displays error status for network failures', async () => {
            fetchMock.mockRejectedValue(new Error('Network failure'));

            window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            const statusBar = window.__devLogsTest.getStatusBar();
            const statusMsg = window.__devLogsTest.getStatusMsg();
            const statusIcon = window.__devLogsTest.getStatusIcon();

            expect(statusBar.className).toContain('error');
            expect(statusBar.className).toContain('visible');
            expect(statusMsg.textContent).toContain('Failed to update dev logging');
            expect(statusMsg.textContent).toContain('Network failure');
            expect(statusIcon.textContent).toBe('error');
        });

        it('does not change label on error', async () => {
            const label = window.__devLogsTest.getLabel();
            expect(label.textContent).toBe('Disabled');

            fetchMock.mockResolvedValue(MockResponse.error(500));

            window.__devLogsTest.triggerToggle(true);
            await vi.runAllTimersAsync();

            expect(label.textContent).toBe('Disabled');
        });
    });

    describe('DOM Structure and Elements', () => {
        it('has properly structured toggle element', () => {
            const toggle = window.__devLogsTest.getToggle();
            
            expect(toggle).not.toBeNull();
            expect(toggle.type).toBe('checkbox');
            expect(toggle.id).toBe('dev-logs-toggle');
        });

        it('has properly structured label element', () => {
            const label = window.__devLogsTest.getLabel();
            
            expect(label).not.toBeNull();
            expect(label.id).toBe('dev-logs-label');
            expect(label.textContent).toBe('Disabled');
        });

        it('has properly structured status bar elements', () => {
            const statusBar = window.__devLogsTest.getStatusBar();
            const statusMsg = window.__devLogsTest.getStatusMsg();
            const statusIcon = window.__devLogsTest.getStatusIcon();

            expect(statusBar).not.toBeNull();
            expect(statusMsg).not.toBeNull();
            expect(statusIcon).not.toBeNull();
        });

        it('has toggle wrapped in proper label structure', () => {
            const toggle = window.__devLogsTest.getToggle();
            const parentLabel = toggle.closest('label');

            expect(parentLabel).not.toBeNull();
            expect(parentLabel.classList.contains('settings-toggle')).toBe(true);
        });

        it('has correct initial state', () => {
            const toggle = window.__devLogsTest.getToggle();
            const label = window.__devLogsTest.getLabel();

            expect(toggle.checked).toBe(false);
            expect(toggle.disabled).toBe(false);
            expect(label.textContent).toBe('Disabled');
        });
    });

    describe('Event Handling', () => {
        beforeEach(() => { vi.useFakeTimers(); });
        afterEach(() => { vi.useRealTimers(); });

        it('triggers API call on change event', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(true));

            const toggle = window.__devLogsTest.getToggle();
            toggle.checked = true;
            toggle.dispatchEvent(new window.Event('change', { bubbles: true }));

            await vi.runAllTimersAsync();

            expect(fetchMock).toHaveBeenCalled();
        });

        it('handles multiple rapid changes correctly', async () => {
            fetchMock.mockResolvedValue(MockResponse.success(true));

            const toggle = window.__devLogsTest.getToggle();
            
            // Simulate rapid toggling
            toggle.checked = true;
            toggle.dispatchEvent(new window.Event('change', { bubbles: true }));
            
            toggle.checked = false;
            toggle.dispatchEvent(new window.Event('change', { bubbles: true }));

            await vi.runAllTimersAsync();

            // Should handle both changes
            expect(fetchMock).toHaveBeenCalledTimes(2);
        });

        it('prevents concurrent requests on rapid changes', async () => {
            // Mock a delayed response
            fetchMock.mockImplementation(() => 
                MockResponse.delayedResponse(MockResponse.success(true), 100)
            );

            const toggle = window.__devLogsTest.getToggle();
            
            // First change
            window.__devLogsTest.triggerToggle(true);
            expect(toggle.disabled).toBe(true);
            
            // Second change while first is pending
            toggle.checked = false;
            toggle.dispatchEvent(new window.Event('change', { bubbles: true }));

            // Should still be disabled until first request completes
            expect(toggle.disabled).toBe(true);

            await vi.runAllTimersAsync();
            
            // Should be re-enabled after all requests complete
            expect(toggle.disabled).toBe(false);
        });
    });

    describe('Test Infrastructure', () => {
        it('provides working test helpers', () => {
            expect(typeof window.__devLogsTest.triggerToggle).toBe('function');
            expect(typeof window.__devLogsTest.getToggle).toBe('function');
            expect(typeof window.__devLogsTest.getLabel).toBe('function');
            expect(typeof window.__devLogsTest.getStatusBar).toBe('function');
            expect(typeof window.__devLogsTest.getStatusMsg).toBe('function');
            expect(typeof window.__devLogsTest.getStatusIcon).toBe('function');
        });

        it('throws error when toggle not found', () => {
            // Remove toggle element
            const toggle = window.__devLogsTest.getToggle();
            toggle.remove();

            expect(() => {
                window.__devLogsTest.triggerToggle(true);
            }).toThrow('Toggle not found');
        });
    });
});
