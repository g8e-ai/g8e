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

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { obfuscateApiKey, copyToClipboardWithFeedback, showConfirmationModal, bindCounter } from '@g8ed/public/js/utils/ui-utils.js';

const TEST_API_KEY = 'dak_abcdefghijklmnopqrstuvwxyz1234567890';

vi.mock('@g8ed/public/js/utils/template-loader.js', () => ({
    templateLoader: {
        createFragment: vi.fn(async (templateName, variables) => {
            const template = document.createElement('template');
            template.innerHTML = `
                <div class="confirmation-modal-overlay">
                    <div class="confirmation-modal">
                        <div class="confirmation-modal-header">
                            <span class="material-symbols-outlined confirmation-modal-icon">${variables.icon || 'warning'}</span>
                        </div>
                        <div class="confirmation-modal-body">
                            <h3 class="confirmation-modal-title">${variables.title || ''}</h3>
                            <p class="confirmation-modal-message">${variables.message || ''}</p>
                        </div>
                        <div class="confirmation-modal-actions">
                            <button data-action="cancel">Cancel</button>
                            <button data-action="confirm" class="${variables.confirmClass || ''}">
                                <span class="material-symbols-outlined">${variables.confirmIcon || 'check'}</span>
                                ${variables.confirmLabel || 'Confirm'}
                            </button>
                        </div>
                    </div>
                </div>
            `;
            return template.content;
        }),
        replace: vi.fn(() => ''),
        renderTo: vi.fn(async () => {}),
        preload: vi.fn(async () => {}),
        clearCache: vi.fn(),
        getCacheStats() { return { size: 0, templates: [], loading: [] }; }
    },
    TemplateLoader: class MockTemplateLoader {
        constructor() {}
        seed() {}
        async load() { return ''; }
        async render() { return ''; }
        replace() { return ''; }
        async createFragment() { return document.createDocumentFragment(); }
        async renderTo() {}
        async preload() {}
        clearCache() {}
        getCacheStats() { return { size: 0, templates: [], loading: [] }; }
    }
}));

describe('ui-utils [UNIT - jsdom]', () => {
    beforeEach(() => {
        window.serviceClient = {
            get: vi.fn()
        };
        vi.stubGlobal('requestAnimationFrame', (cb) => cb());
    });

    afterEach(() => {
        delete window.serviceClient;
        vi.unstubAllGlobals();
        vi.clearAllMocks();
    });

    describe('obfuscateApiKey', () => {
        it('returns placeholder dots for null api key', () => {
            expect(obfuscateApiKey(null)).toBe('••••••••••••••••');
        });

        it('returns placeholder dots for empty string', () => {
            expect(obfuscateApiKey('')).toBe('••••••••••••••••');
        });

        it('returns placeholder dots for short keys (< 20 chars)', () => {
            expect(obfuscateApiKey('short_key_123')).toBe('••••••••••••••••');
        });

        it('shows first 12 and last 4 characters with dots in between for valid keys', () => {
            const result = obfuscateApiKey(TEST_API_KEY);
            expect(result).toBe(TEST_API_KEY.substring(0, 12) + '••••••••••••' + TEST_API_KEY.substring(TEST_API_KEY.length - 4));
        });

        it('preserves the exact prefix and suffix of the key', () => {
            const result = obfuscateApiKey(TEST_API_KEY);
            expect(result.startsWith('dak_abcdefgh')).toBe(true);
            expect(result.endsWith('7890')).toBe(true);
        });
    });

    describe('copyToClipboardWithFeedback', () => {
        beforeEach(() => {
            if (!global.navigator.clipboard) {
                global.navigator.clipboard = {
                    writeText: vi.fn().mockResolvedValue(undefined)
                };
            }
            global.navigator.clipboard.writeText.mockResolvedValue(undefined);
        });

        it('copies text to clipboard', async () => {
            const button = document.createElement('button');
            button.innerHTML = 'Copy';
            
            await copyToClipboardWithFeedback('test text', button);
            
            expect(global.navigator.clipboard.writeText).toHaveBeenCalledWith('test text');
        });

        it('shows check icon on button after copy', async () => {
            const button = document.createElement('button');
            button.innerHTML = 'Copy';
            
            await copyToClipboardWithFeedback('test text', button);
            
            expect(button.innerHTML).toContain('check');
            expect(button.classList.contains('copied')).toBe(true);
        });

        it('restores original button content after 2 seconds', async () => {
            const button = document.createElement('button');
            const originalHTML = '<span>Copy</span>';
            button.innerHTML = originalHTML;
            
            vi.useFakeTimers();
            await copyToClipboardWithFeedback('test text', button);
            
            expect(button.innerHTML).toContain('check');
            
            vi.advanceTimersByTime(2000);
            
            expect(button.innerHTML).toBe(originalHTML);
            expect(button.classList.contains('copied')).toBe(false);
            vi.useRealTimers();
        });

        it('calls logger on success', async () => {
            const button = document.createElement('button');
            const logger = vi.fn();
            
            await copyToClipboardWithFeedback('test text', button, logger);
            
            expect(logger).toHaveBeenCalledWith('Text copied to clipboard');
        });

        it('calls notifier on error', async () => {
            const button = document.createElement('button');
            const notifier = vi.fn();
            const error = new Error('Clipboard error');
            global.navigator.clipboard.writeText.mockRejectedValue(error);
            
            await expect(copyToClipboardWithFeedback('test text', button, null, notifier)).rejects.toThrow('Clipboard error');
            
            expect(notifier).toHaveBeenCalledWith('Failed to copy to clipboard');
        });

        it('calls logger with error on failure', async () => {
            const button = document.createElement('button');
            const logger = vi.fn();
            const error = new Error('Clipboard error');
            global.navigator.clipboard.writeText.mockRejectedValue(error);
            
            await expect(copyToClipboardWithFeedback('test text', button, logger)).rejects.toThrow('Clipboard error');
            
            expect(logger).toHaveBeenCalledWith('Failed to copy to clipboard:', error);
        });

        it('works without logger and notifier', async () => {
            const button = document.createElement('button');
            
            await expect(copyToClipboardWithFeedback('test text', button)).resolves.toBeUndefined();
            expect(global.navigator.clipboard.writeText).toHaveBeenCalledWith('test text');
        });
    });

    describe('bindCounter', () => {
        let container;
        let input;

        beforeEach(() => {
            container = document.createElement('div');
            container.innerHTML = `
                <button id="dec">-</button>
                <input id="count" type="number" value="5">
                <button id="inc">+</button>
            `;
            input = container.querySelector('#count');
        });

        it('increments input value on increment button click', () => {
            bindCounter('#dec', '#inc', input, 1, 10, container);
            container.querySelector('#inc').click();
            expect(input.value).toBe('6');
        });

        it('decrements input value on decrement button click', () => {
            bindCounter('#dec', '#inc', input, 1, 10, container);
            container.querySelector('#dec').click();
            expect(input.value).toBe('4');
        });

        it('respects minimum value', () => {
            input.value = '1';
            bindCounter('#dec', '#inc', input, 1, 10, container);
            container.querySelector('#dec').click();
            expect(input.value).toBe('1');
        });

        it('respects maximum value', () => {
            input.value = '10';
            bindCounter('#dec', '#inc', input, 1, 10, container);
            container.querySelector('#inc').click();
            expect(input.value).toBe('10');
        });

        it('handles non-numeric input value by using minimum', () => {
            input.value = 'invalid';
            bindCounter('#dec', '#inc', input, 1, 10, container);
            container.querySelector('#inc').click();
            expect(input.value).toBe('2'); // min (1) + 1
        });

        it('handles empty input value by using minimum', () => {
            input.value = '';
            bindCounter('#dec', '#inc', input, 1, 10, container);
            container.querySelector('#dec').click();
            expect(input.value).toBe('1'); // Math.max(1, 1 - 1) = 1
        });
    });

    describe('showConfirmationModal', () => {
        it.skip('creates and appends modal to body', async () => {
            const promise = showConfirmationModal({
                title: 'Test Title',
                message: 'Test Message'
            });

            expect(document.querySelector('.confirmation-modal-overlay')).toBeTruthy();
            expect(document.querySelector('.confirmation-modal-title')).toBeTruthy();
            expect(document.querySelector('.confirmation-modal-message')).toBeTruthy();

            document.querySelector('.confirmation-modal-cancel')?.click();
            await promise;
        });

        it.skip('renders title and message correctly', async () => {
            const promise = showConfirmationModal({
                title: 'Delete Item',
                message: 'Are you sure you want to delete this item?'
            });

            const title = document.querySelector('.confirmation-modal-title');
            const message = document.querySelector('.confirmation-modal-message');

            expect(title?.textContent).toBe('Delete Item');
            expect(message?.textContent).toBe('Are you sure you want to delete this item?');

            document.querySelector('.confirmation-modal-cancel')?.click();
            await promise;
        });

        it.skip('uses default confirmLabel and confirmIcon', async () => {
            const promise = showConfirmationModal({
                title: 'Test',
                message: 'Test'
            });

            const confirmBtn = document.querySelector('.confirmation-modal-confirm');
            expect(confirmBtn?.textContent).toContain('Confirm');
            expect(confirmBtn?.querySelector('.material-symbols-outlined')?.textContent).toBe('check');

            document.querySelector('.confirmation-modal-cancel')?.click();
            await promise;
        });

        it.skip('uses custom confirmLabel and confirmIcon', async () => {
            const promise = showConfirmationModal({
                title: 'Test',
                message: 'Test',
                confirmLabel: 'Delete',
                confirmIcon: 'delete'
            });

            const confirmBtn = document.querySelector('.confirmation-modal-confirm');
            expect(confirmBtn?.textContent).toContain('Delete');
            expect(confirmBtn?.querySelector('.material-symbols-outlined')?.textContent).toBe('delete');

            document.querySelector('.confirmation-modal-cancel')?.click();
            await promise;
        });

        it.skip('resolves to false when cancel button is clicked', async () => {
            const promise = showConfirmationModal({
                title: 'Test',
                message: 'Test'
            });

            document.querySelector('.confirmation-modal-cancel')?.click();
            const result = await promise;

            expect(result).toBe(false);
        });

        it.skip('resolves to true when confirm button is clicked', async () => {
            const promise = showConfirmationModal({
                title: 'Test',
                message: 'Test'
            });

            document.querySelector('.confirmation-modal-confirm')?.click();
            const result = await promise;

            expect(result).toBe(true);
        });

        it.skip('resolves to false when overlay is clicked', async () => {
            const promise = showConfirmationModal({
                title: 'Test',
                message: 'Test'
            });

            document.querySelector('.confirmation-modal-overlay')?.click();
            const result = await promise;

            expect(result).toBe(false);
        });

        it.skip('resolves to false when Escape key is pressed', async () => {
            const promise = showConfirmationModal({
                title: 'Test',
                message: 'Test'
            });

            const escapeEvent = new KeyboardEvent('keydown', { key: 'Escape' });
            document.dispatchEvent(escapeEvent);

            const result = await promise;

            expect(result).toBe(false);
        });

        it.skip('removes modal from DOM after close', async () => {
            const promise = showConfirmationModal({
                title: 'Test',
                message: 'Test'
            });

            expect(document.querySelector('.confirmation-modal-overlay')).toBeTruthy();

            document.querySelector('.confirmation-modal-cancel')?.click();
            await promise;

            vi.useFakeTimers();
            vi.advanceTimersByTime(300);
            vi.useRealTimers();

            expect(document.querySelector('.confirmation-modal-overlay')).toBeFalsy();
        });

        it.skip('adds and removes active CSS class', async () => {
            const promise = showConfirmationModal({
                title: 'Test',
                message: 'Test'
            });

            const overlay = document.querySelector('.confirmation-modal-overlay');
            expect(overlay?.classList.contains('active')).toBe(true);

            document.querySelector('.confirmation-modal-cancel')?.click();
            await promise;

            vi.useFakeTimers();
            vi.advanceTimersByTime(300);
            vi.useRealTimers();

            expect(overlay?.classList.contains('active')).toBe(false);
        });
    });
});
