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

import { templateLoader } from './template-loader.js';

/**
 * UI utility functions for common UI patterns
 */

/**
 * Obfuscate an API key for display purposes
 * Shows first 12 characters, bullets, then last 4 characters
 * @param {string} apiKey - The API key to obfuscate
 * @returns {string} Obfuscated API key
 */
export function obfuscateApiKey(apiKey) {
    if (!apiKey || apiKey.length < 20) return '••••••••••••••••';
    return apiKey.substring(0, 12) + '••••••••••••' + apiKey.substring(apiKey.length - 4);
}

/**
 * Copy text to clipboard with visual feedback on button
 * @param {string} text - Text to copy
 * @param {HTMLElement} button - Button element to show feedback on
 * @param {Function} logger - Optional logger function for success/error
 * @param {Function} notifier - Optional notification function for errors
 * @returns {Promise<void>}
 */
export async function copyToClipboardWithFeedback(text, button, logger, notifier) {
    try {
        await navigator.clipboard.writeText(text);
        if (logger) logger('Text copied to clipboard');

        const originalText = button.innerHTML;
        button.innerHTML = '<span class="copy-icon material-symbols-outlined">check</span>';
        button.classList.add('copied');

        setTimeout(() => {
            button.innerHTML = originalText;
            button.classList.remove('copied');
        }, 2000);
    } catch (error) {
        if (logger) logger('Failed to copy to clipboard:', error);
        if (notifier) notifier('Failed to copy to clipboard');
        throw error;
    }
}

/**
 * Bind decrement and increment buttons to an input field
 * @param {string} decId - CSS selector for decrement button
 * @param {string} incId - CSS selector for increment button
 * @param {HTMLInputElement} input - Input element to modify
 * @param {number} min - Minimum value
 * @param {number} max - Maximum value
 * @param {HTMLElement} container - Container element to query buttons from
 */
export function bindCounter(decId, incId, input, min, max, container) {
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
}

/**
 * Show a confirmation modal dialog
 * @param {Object} options - Modal options
 * @param {string} options.title - Modal title
 * @param {string} options.message - Modal message
 * @param {string} [options.confirmLabel='Confirm'] - Confirm button label
 * @param {string} [options.confirmIcon='check'] - Confirm button icon (Material Symbols)
 * @param {string} [options.icon='warning'] - Header icon
 * @param {string} [options.iconClass=''] - Additional class for the header icon
 * @param {string} [options.confirmClass=''] - Additional class for the confirm button
 * @param {string} [options.descriptionClass=''] - Additional class for the message/description
 * @param {string} [options.htmlContent=''] - Optional HTML content to insert between message and actions
 * @param {Function} [options.onConfirm=null] - Optional async function to run when confirmed. If provided, the modal won't close until it resolves.
 * @returns {Promise<boolean>} Resolves to true if confirmed, false if cancelled
 */
export async function showConfirmationModal({ 
    title, 
    message, 
    confirmLabel = 'Confirm', 
    confirmIcon = 'check',
    icon = null,
    iconClass = '',
    confirmClass = '',
    descriptionClass = '',
    htmlContent = '',
    onConfirm = null
}) {
    // Auto-select icon if not provided
    if (!icon) {
        const msg = message.toLowerCase();
        if (msg.includes('error') || msg.includes('fail') || msg.includes('danger')) {
            icon = 'error';
        } else if (msg.includes('warn') || msg.includes('delete') || msg.includes('remove') || msg.includes('unbind')) {
            icon = 'warning';
        } else {
            icon = 'info';
        }
    }

    return new Promise(async (resolve) => {
        const fragment = await templateLoader.createFragment('confirmation-modal-base', {
            title,
            message,
            confirmLabel,
            confirmIcon,
            icon,
            iconClass,
            confirmClass,
            descriptionClass
        });

        const overlay = fragment.firstElementChild;
        const body = overlay.querySelector('.confirmation-modal-body');
        
        // Safely insert htmlContent if provided
        if (htmlContent && body) {
            const contentContainer = document.createElement('div');
            contentContainer.innerHTML = htmlContent;
            // Append all children from the content container
            while (contentContainer.firstChild) {
                body.appendChild(contentContainer.firstChild);
            }
        }

        document.body.appendChild(overlay);

        const modal = overlay.querySelector('.confirmation-modal');
        const confirmBtn = overlay.querySelector('[data-action="confirm"]');
        const cancelBtn = overlay.querySelector('[data-action="cancel"]');
        
        // Focus management
        const focusableElements = modal.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
        const firstFocusable = focusableElements[0];
        const lastFocusable = focusableElements[focusableElements.length - 1];

        const close = (result) => {
            overlay.classList.remove('active');
            document.removeEventListener('keydown', globalKeyHandler);
            setTimeout(() => {
                if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
                resolve(result);
            }, 300);
        };

        const handleConfirm = async () => {
            if (onConfirm) {
                try {
                    confirmBtn.disabled = true;
                    cancelBtn.disabled = true;
                    await onConfirm(overlay);
                } catch (error) {
                    confirmBtn.disabled = false;
                    cancelBtn.disabled = false;
                    return; // Don't close if handler failed
                }
            }
            close(true);
        };

        const globalKeyHandler = (e) => {
            if (e.key === 'Escape') {
                close(false);
            }
            
            // Focus trapping
            if (e.key === 'Tab') {
                if (e.shiftKey) {
                    if (document.activeElement === firstFocusable) {
                        lastFocusable.focus();
                        e.preventDefault();
                    }
                } else {
                    if (document.activeElement === lastFocusable) {
                        firstFocusable.focus();
                        e.preventDefault();
                    }
                }
            }
        };

        overlay.querySelector('[data-action="cancel"]')?.addEventListener('click', () => close(false));
        overlay.querySelector('[data-action="confirm"]')?.addEventListener('click', handleConfirm);
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) close(false);
        });

        document.addEventListener('keydown', globalKeyHandler);

        requestAnimationFrame(() => {
            overlay.classList.add('active');
            confirmBtn?.focus();
        });
    });
}
