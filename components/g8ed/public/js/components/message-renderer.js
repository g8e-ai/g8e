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

import { CitationsHandler } from './citations.js';
import { decodeHtmlEntities } from '../utils/html.js';
import { EventType } from '../constants/events.js';
import { CopyText } from '../constants/ui-constants.js';

/**
 * Unified Message Renderer
 * Single source of truth for rendering all chat messages
 * Eliminates duplicate formatting paths between live and restored messages
 */

class MessageRenderer {
    constructor(markdownRenderer) {
        this.markdownRenderer = markdownRenderer;
        this.citationsHandler = new CitationsHandler();
    }

    /**
     * Render a complete message element with header and content
     * Used by both live messages and restored messages
     */
    renderMessage({ content, sender, timestamp, contextInfo = null, groundingMetadata = null, eventType = 'normal' }) {
        const messageElement = document.createElement('div');
        messageElement.className = `message-item ${sender} ${eventType}`;

        const displayTime = this._formatTimestamp(timestamp);
        const senderInfo = this._getSenderInfo(sender);
        const copyButtonHtml = sender === EventType.EVENT_SOURCE_AI_PRIMARY ? this._getCopyButton() : '';
        const contextDisplay = this._renderContextInfo(contextInfo);

        // Decode and render content using unified markdown renderer
        const decodedContent = decodeHtmlEntities(content);
        let formattedContent = this.markdownRenderer.parseMarkdown(decodedContent);

        // Add inline citations if grounding metadata exists
        if (groundingMetadata && groundingMetadata.sources && groundingMetadata.sources.length > 0) {
            formattedContent = this.citationsHandler.addInlineCitations(formattedContent, groundingMetadata);
        }

        messageElement.innerHTML = `
            <div class="message-header">
                <div class="message-sender">
                    <span class="sender-icon">${senderInfo.icon}</span>
                    <span class="sender-label">${senderInfo.label}</span>
                    <div class="message-timestamp">${displayTime}</div>
                </div>
            </div>
            <div class="message-content ${senderInfo.className}">
                <div class="content-text">${formattedContent}</div>
                ${contextDisplay}${copyButtonHtml}
            </div>
        `;

        // Add sources panel if grounding metadata exists
        if (groundingMetadata && groundingMetadata.sources && groundingMetadata.sources.length > 0) {
            const contentText = messageElement.querySelector('.content-text');
            if (contentText) {
                const sourcesPanel = this.citationsHandler.renderSourcesPanel(groundingMetadata.sources);
                contentText.appendChild(sourcesPanel);
            }
        }

        // Render any Mermaid diagrams in the message
        this.markdownRenderer.renderMermaidDiagrams(messageElement);

        return messageElement;
    }

    renderContent(content, streaming = false) {
        const decodedContent = decodeHtmlEntities(content);
        return this.markdownRenderer.parseMarkdown(decodedContent, streaming);
    }

    setupCopyButtonListeners() {
        document.addEventListener('click', async (e) => {
            const copyBtn = e.target.closest('.copy-response-btn');
            if (!copyBtn) return;

            const messageContent = copyBtn.closest('.message-content');
            if (!messageContent) return;

            const contentText = messageContent.querySelector('.content-text');
            if (!contentText) return;

            try {
                const clonedContent = contentText.cloneNode(true);

                const citationPills = clonedContent.querySelectorAll('.citation-pill');
                citationPills.forEach(pill => {
                    const citationNum = pill.getAttribute('data-citation') || pill.querySelector('.citation-number')?.textContent;
                    if (citationNum) {
                        const textNode = document.createTextNode(`[${citationNum}]`);
                        pill.parentNode.replaceChild(textNode, pill);
                    } else {
                        pill.remove();
                    }
                });

                const sourcesPanel = contentText.querySelector('.sources-panel');
                let sourcesText = '';

                if (sourcesPanel) {
                    const sourceItems = sourcesPanel.querySelectorAll('.source-item');
                    if (sourceItems.length > 0) {
                        sourcesText = `${CopyText.PARAGRAPH_BREAK}Sources\n`;
                        sourceItems.forEach((item, index) => {
                            const citationNum = item.querySelector('.source-citation-number')?.textContent || (index + 1);
                            const title = item.querySelector('.source-title')?.textContent;
                            const domain = item.querySelector('.source-domain')?.textContent?.trim();
                            const url = item.getAttribute('href');

                            sourcesText += `${citationNum}. ${title}\n   ${domain}\n   ${url}${CopyText.PARAGRAPH_BREAK}`;
                        });
                    }

                    const clonedSourcesPanel = clonedContent.querySelector('.sources-panel');
                    if (clonedSourcesPanel) {
                        clonedSourcesPanel.remove();
                    }
                }

                const textContent = clonedContent.innerText || clonedContent.textContent;
                const finalText = textContent.trim() + sourcesText;

                await navigator.clipboard.writeText(finalText);

                const icon = copyBtn.querySelector('.material-symbols-outlined');
                const originalIcon = icon.textContent;
                icon.textContent = 'check';
                copyBtn.classList.add('copied');

                setTimeout(() => {
                    icon.textContent = originalIcon;
                    copyBtn.classList.remove('copied');
                }, 2000);
            } catch (error) {
                console.error('[MESSAGE RENDERER] Failed to copy response:', error);

                const icon = copyBtn.querySelector('.material-symbols-outlined');
                const originalIcon = icon.textContent;
                icon.textContent = 'error';
                copyBtn.classList.add('copy-error');

                setTimeout(() => {
                    icon.textContent = originalIcon;
                    copyBtn.classList.remove('copy-error');
                }, 2000);
            }
        });
    }

    // Citation methods delegated to CitationsHandler
    addInlineCitations(htmlContent, groundingMetadata) {
        return this.citationsHandler.addInlineCitations(htmlContent, groundingMetadata);
    }

    renderSourcesPanel(sources) {
        return this.citationsHandler.renderSourcesPanel(sources);
    }

    _formatTimestamp(timestamp) {
        if (timestamp) {
            const msgDate = new Date(timestamp);
            return msgDate.toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            });
        }
        return new Date().toLocaleTimeString([], {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit'
        });
    }

    _getSenderInfo(sender) {
        const senderMap = {
            [EventType.EVENT_SOURCE_USER_CHAT]: {
                label: 'You',
                icon: '',
                className: 'user-message'
            },
            [EventType.EVENT_SOURCE_AI_PRIMARY]: {
                label: 'g8e',
                icon: '',
                className: 'g8e_ai_agent'
            },
            [EventType.EVENT_SOURCE_SYSTEM]: {
                label: 'SYSTEM',
                icon: '<span class="material-symbols-outlined">settings</span>',
                className: 'system_message'
            }
        };

        return senderMap[sender] || {
            label: 'Unknown',
            icon: '<span class="material-symbols-outlined">help</span>',
            className: 'unknown'
        };
    }

    _getCopyButton() {
        return `
            <button class="copy-response-btn" title="Copy response">
                <span class="material-symbols-outlined">content_copy</span>
            </button>`;
    }

    _renderContextInfo(contextInfo) {
        if (!contextInfo || (!contextInfo.case_id && !contextInfo.investigation_id)) {
            return '';
        }

        const parts = [];
        if (contextInfo.case_id) parts.push(`Case: ${contextInfo.case_id}`);
        if (contextInfo.investigation_id) parts.push(`Investigation: ${contextInfo.investigation_id}`);
        
        if (parts.length === 0) return '';
        
        return `<div class="message-context">${parts.join(' | ')}</div>`;
    }

}

export { MessageRenderer };
