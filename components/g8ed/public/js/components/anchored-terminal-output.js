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

/**
 * IMPORTANT: All innerHTML assignments in this file depend on the markdown renderer's
 * sanitization via DOMPurify. The markdown renderer (markdown-it with DOMPurify plugin)
 * sanitizes HTML before it reaches these innerHTML assignments. This is a trusted
 * dependency - if the markdown renderer configuration changes, these usages must be
 * reviewed for XSS vulnerabilities.
 */

import { templateLoader } from '../utils/template-loader.js';
import { TribunalOutcome, EventType } from '../constants/events.js';

export class TerminalOutputMixin {
    _cancelPendingTimers() {
        if (this._pendingTimers) {
            for (const id of this._pendingTimers) clearTimeout(id);
            this._pendingTimers.clear();
        }
    }

    _trackTimer(id) {
        if (!this._pendingTimers) this._pendingTimers = new Set();
        this._pendingTimers.add(id);
        return id;
    }

    _removeWelcome() {
        const welcome = this.outputContainer?.querySelector('.anchored-terminal__welcome');
        if (welcome) welcome.remove();
    }

    _createAgentMessageHeader(timestamp = null) {
        const header = document.createElement('div');
        header.className = 'anchored-terminal__agent-message-header';

        const sender = document.createElement('span');
        sender.className = 'anchored-terminal__ai-response-sender';
        sender.textContent = 'g8e';

        const time = document.createElement('span');
        time.className = 'anchored-terminal__ai-response-time';
        time.textContent = timestamp || this.formatTimestamp();

        header.appendChild(sender);
        header.appendChild(time);

        return header;
    }

    showWaitingIndicator(webSessionId) {
        if (!this.outputContainer) return null;

        this._removeWelcome();
        this.hideWaitingIndicator();

        const group = document.createElement('div');
        group.className = 'anchored-terminal__agent-message-group waiting';
        group.id = 'waiting-indicator';
        if (webSessionId) {
            group.setAttribute('data-web-session-id', webSessionId);
        }

        const header = this._createAgentMessageHeader();

        const content = document.createElement('div');
        content.className = 'anchored-terminal__agent-message-content';

        const cursor = document.createElement('span');
        cursor.className = 'anchored-terminal__waiting-cursor';
        content.appendChild(cursor);

        group.appendChild(header);
        group.appendChild(content);

        this.outputContainer.appendChild(group);
        this.scrollToBottom();

        return group;
    }

    hideWaitingIndicator() {
        const indicator = document.getElementById('waiting-indicator');
        if (indicator) {
            indicator.remove();
        }
    }

    appendUserMessage(message, timestamp = null) {
        if (!this.outputContainer) return;

        this._removeWelcome();

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__user-message';

        const header = document.createElement('div');
        header.className = 'anchored-terminal__user-message-header';

        const sender = document.createElement('span');
        sender.className = 'anchored-terminal__user-message-sender';
        sender.textContent = 'You';

        const time = document.createElement('span');
        time.className = 'anchored-terminal__user-message-time';
        time.textContent = timestamp || this.formatTimestamp();

        header.appendChild(sender);
        header.appendChild(time);

        const content = document.createElement('div');
        content.className = 'anchored-terminal__user-message-content';
        content.textContent = message;

        entry.appendChild(header);
        entry.appendChild(content);

        this.outputContainer.appendChild(entry);
        this.scrollToBottom({ force: true });

        return entry;
    }

    createAIResponse(webSessionId) {
        if (!this.outputContainer) return null;

        const existingId = `ai-response-${webSessionId}`;
        const existing = document.getElementById(existingId);
        if (existing) {
            throw new Error(`AI response element ${existingId} already exists`);
        }

        this._removeWelcome();
        this.hideWaitingIndicator();

        const group = document.createElement('div');
        group.className = 'anchored-terminal__agent-message-group streaming';
        group.id = existingId;

        const header = this._createAgentMessageHeader();

        const content = document.createElement('div');
        content.className = 'anchored-terminal__agent-message-content';

        group.appendChild(header);
        group.appendChild(content);

        this.outputContainer.appendChild(group);
        this.scrollToBottom();

        return content;
    }

    getAIResponse(webSessionId) {
        const existingId = `ai-response-${webSessionId}`;
        const existing = document.getElementById(existingId);
        if (!existing) {
            return null;
        }
        return existing.querySelector('.anchored-terminal__agent-message-content');
    }

    appendStreamingTextChunk(webSessionId, text) {
        let contentEl = this.getAIResponse(webSessionId);
        if (!contentEl) {
            contentEl = this.createAIResponse(webSessionId);
        }
        if (!contentEl) return;

        if (!this._streamingTextAccumulator) {
            this._streamingTextAccumulator = new Map();
        }

        const existing = this._streamingTextAccumulator.get(webSessionId) || '';
        const newText = existing + text;
        this._streamingTextAccumulator.set(webSessionId, newText);

        const renderer = this.markdownRenderer;
        if (renderer) {
            contentEl.innerHTML = renderer.parseMarkdown(newText, true);
        } else {
            contentEl.textContent = newText;
        }

        this.scrollToBottom();
    }

    replaceStreamingHtml(webSessionId, html) {
        let contentEl = this.getAIResponse(webSessionId);
        if (!contentEl) {
            contentEl = this.createAIResponse(webSessionId);
        }
        if (!contentEl) return;

        // TRUSTED: HTML from markdown renderer (should be sanitized by markdown-it with DOMPurify)
        contentEl.innerHTML = html;

        this.scrollToBottom();
    }

    finalizeAIResponseChunk(webSessionId, finalHtml, groundingMetadata = null) {
        const group = document.getElementById(`ai-response-${webSessionId}`);
        if (!group) return;

        const contentEl = group.querySelector('.anchored-terminal__agent-message-content');
        if (contentEl) {
            // TRUSTED: Final HTML from markdown renderer (should be sanitized by markdown-it with DOMPurify)
            contentEl.innerHTML = finalHtml;

            // Apply citations if grounding metadata is provided
            if (groundingMetadata) {
                this.applyCitations(webSessionId, groundingMetadata);
            }
        }

        group.classList.remove('streaming');
        group.querySelectorAll('.streaming-cursor').forEach(c => c.remove());
        group.id = `ai-response-${webSessionId}-${Date.now()}`;

        if (this._streamingTextAccumulator) {
            this._streamingTextAccumulator.delete(webSessionId);
        }

        this.scrollToBottom();
    }

    applyCitations(webSessionId, groundingMetadata) {
        if (!groundingMetadata || !groundingMetadata.grounding_used) return;

        const sources = groundingMetadata.sources;
        if (!sources || !sources.length) return;

        const citationsHandler = this.citationsHandler;
        if (!citationsHandler) {
            console.warn('[ANCHORED TERMINAL] No CitationsHandler available for citations rendering');
            return;
        }

        let group = document.getElementById(`ai-response-${webSessionId}`);
        if (!group) {
            const candidates = this.outputContainer?.querySelectorAll(`[id^="ai-response-${webSessionId}-"]`);
            if (candidates && candidates.length > 0) {
                group = candidates[candidates.length - 1];
            }
        }
        if (!group) return;

        const contentEl = group.querySelector('.anchored-terminal__agent-message-content');
        if (!contentEl) return;

        // TRUSTED: HTML from citations handler adds citation markers to already-sanitized markdown output
        const citedHtml = citationsHandler.addInlineCitations(contentEl.innerHTML, groundingMetadata);
        contentEl.innerHTML = citedHtml;

        const sourcesPanel = citationsHandler.renderSourcesPanel(sources);
        contentEl.appendChild(sourcesPanel);

        this.scrollToBottom();
    }

    appendDirectHtmlResponse(message, timestamp = null, groundingMetadata = null) {
        if (!this.outputContainer) return;

        this._removeWelcome();

        const group = document.createElement('div');
        group.className = 'anchored-terminal__agent-message-group';

        const header = this._createAgentMessageHeader(timestamp);

        const citationsHandler = this.citationsHandler;
        const sources = groundingMetadata?.sources;
        if (citationsHandler && groundingMetadata?.grounding_used && sources?.length) {
            message = citationsHandler.addInlineCitations(message, groundingMetadata);
        }

        const content = document.createElement('div');
        content.className = 'anchored-terminal__agent-message-content';
        // TRUSTED: HTML from markdown renderer (should be sanitized by markdown-it with DOMPurify)
        content.innerHTML = message;

        if (citationsHandler && groundingMetadata?.grounding_used && sources?.length) {
            const sourcesPanel = citationsHandler.renderSourcesPanel(sources);
            content.appendChild(sourcesPanel);
        }

        group.appendChild(header);
        group.appendChild(content);

        this.outputContainer.appendChild(group);
        this.scrollToBottom();

        return group;
    }

    getOrCreateThinkingEntry(webSessionId) {
        if (!this.outputContainer) return null;

        this._removeWelcome();
        this.hideWaitingIndicator();

        const existingId = `thinking-${webSessionId}`;
        const existing = document.getElementById(existingId);
        if (existing) {
            return existing;
        }

        // Create a dedicated thinking group
        const group = document.createElement('div');
        group.className = 'anchored-terminal__agent-message-group anchored-terminal__agent-message-group--thinking';
        group.setAttribute('data-thinking-bubble', webSessionId);

        const header = this._createAgentMessageHeader();

        const content = document.createElement('div');
        content.className = 'anchored-terminal__agent-message-content';

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__thinking active';
        entry.id = existingId;

        const thinkingHeader = document.createElement('div');
        thinkingHeader.className = 'anchored-terminal__thinking-header';

        const toggle = document.createElement('span');
        toggle.className = 'anchored-terminal__thinking-toggle';
        toggle.textContent = '–';

        const title = document.createElement('span');
        title.className = 'anchored-terminal__thinking-title';
        title.textContent = 'Thinking...';

        thinkingHeader.appendChild(toggle);
        thinkingHeader.appendChild(title);

        thinkingHeader.addEventListener('click', () => {
            const isCollapsed = entry.classList.toggle('collapsed');
            toggle.textContent = isCollapsed ? '+' : '–';
        });

        const thinkingContent = document.createElement('div');
        thinkingContent.className = 'anchored-terminal__thinking-content';

        entry.appendChild(thinkingHeader);
        entry.appendChild(thinkingContent);
        content.appendChild(entry);
        group.appendChild(header);
        group.appendChild(content);
        this.outputContainer.appendChild(group);
        this.scrollToBottom();

        return entry;
    }

    _extractThinkingTitle(text) {
        const lines = text.split('\n');
        for (let i = lines.length - 1; i >= 0; i--) {
            const line = lines[i].trim();
            const boldMatch = line.match(/^\*\*(.+?)\*\*$/);
            if (boldMatch) return boldMatch[1];
            const headingMatch = line.match(/^#{1,3}\s+(.+)$/);
            if (headingMatch) return headingMatch[1];
        }
        return null;
    }

    appendThinkingContent(webSessionId, text) {
        const entry = this.getOrCreateThinkingEntry(webSessionId);
        if (!entry) return;

        const contentEl = entry.querySelector('.anchored-terminal__thinking-content');
        if (contentEl) {
            const existing = this.thinkingContentRaw.get(webSessionId);
            const newRaw = existing ? existing + '\n' + text : text;
            this.thinkingContentRaw.set(webSessionId, newRaw);

            const title = this._extractThinkingTitle(newRaw);
            if (title) {
                const titleEl = entry.querySelector('.anchored-terminal__thinking-title');
                if (titleEl) titleEl.textContent = title;
            }

            const renderer = this.markdownRenderer;
            if (renderer) {
                // TRUSTED: HTML from markdown renderer (should be sanitized by markdown-it with DOMPurify)
                contentEl.innerHTML = renderer.parseMarkdown(newRaw);
            } else {
                contentEl.textContent = newRaw;
            }

            contentEl.scrollTop = contentEl.scrollHeight;
        }
    }

    completeThinkingEntry(webSessionId) {
        const entry = document.getElementById(`thinking-${webSessionId}`);
        if (entry) {
            entry.classList.remove('active');
            entry.classList.add('collapsed');

            const toggle = entry.querySelector('.anchored-terminal__thinking-toggle');
            if (toggle) toggle.textContent = '+';

            entry.id = `thinking-${webSessionId}-${Date.now()}`;
        }

        this.thinkingContentRaw.delete(webSessionId);
    }

    async appendActivityIndicator(options) {
        if (!this.outputContainer) return null;

        const { id, category, icon, label, detail } = options || {};

        this._removeWelcome();

        const indicator = document.createElement('div');
        indicator.id = id;
        indicator.className = `anchored-terminal__activity category-${category}`;
        await templateLoader.renderTo(indicator, 'activity-indicator', {
            icon: this.escapeHtml(icon),
            label: this.escapeHtml(label),
            detailHtml: detail ? `<span class="anchored-terminal__activity-detail">${this.escapeHtml(detail)}</span>` : ''
        });

        this.outputContainer.appendChild(indicator);
        this.scrollToBottom();

        return id;
    }

    completeActivityIndicator(indicatorId) {
        const indicator = document.getElementById(indicatorId);
        if (!indicator) return;

        indicator.classList.add('completed');

        this._trackTimer(setTimeout(() => {
            indicator.remove();
        }, 300));
    }

    clearActivityIndicators() {
        if (!this.outputContainer) return;

        const indicators = this.outputContainer.querySelectorAll('.anchored-terminal__activity');
        indicators.forEach(el => {
            el.classList.add('completed');
            this._trackTimer(setTimeout(() => el.remove(), 300));
        });
    }

    appendSystemMessage(text) {
        if (!this.outputContainer) return;

        this._removeWelcome();

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__entry';

        const msg = document.createElement('div');
        msg.className = 'anchored-terminal__cmd-output system-message';
        msg.textContent = `[${this.formatTimestamp()}] ${text}`;

        entry.appendChild(msg);
        this.outputContainer.appendChild(entry);
        this.scrollToBottom();

        return entry;
    }

    sealStreamingResponse(webSessionId) {
        const group = document.getElementById(`ai-response-${webSessionId}`);
        if (group) {
            group.classList.remove('streaming');
            group.querySelectorAll('.streaming-cursor').forEach(c => c.remove());
            group.id = `ai-response-${webSessionId}-${Date.now()}`;
        }
        if (this._streamingTextAccumulator) {
            this._streamingTextAccumulator.delete(webSessionId);
        }
    }

    clearStreamingAccumulator() {
        if (this._streamingTextAccumulator) {
            this._streamingTextAccumulator.clear();
        }
    }

    appendErrorMessage(text) {
        if (!this.outputContainer) return;

        this._removeWelcome();

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__error-message';

        const header = document.createElement('div');
        header.className = 'anchored-terminal__error-header';
        const icon = document.createElement('span');
        icon.className = 'material-symbols-outlined';
        icon.textContent = 'error';
        header.appendChild(icon);
        header.appendChild(document.createTextNode('Error'));

        const content = document.createElement('div');
        content.className = 'anchored-terminal__error-content';
        content.textContent = text;

        entry.appendChild(header);
        entry.appendChild(content);
        this.outputContainer.appendChild(entry);
        this.scrollToBottom();
    }

    async showTribunal({ id, model, numPasses, request, guidelines, webSessionId }) {
        if (!this.outputContainer) return null;

        this._removeWelcome();

        // Get or create agent message group (prefer execution group if exists)
        const lastExecutionGroup = this.outputContainer.querySelector('.anchored-terminal__agent-message-group[data-execution-bubble]:last-of-type');
        const lastAgentGroup = this.outputContainer.querySelector('.anchored-terminal__agent-message-group:last-of-type');
        let content;

        if (lastExecutionGroup) {
            content = lastExecutionGroup.querySelector('.anchored-terminal__agent-message-content');
        } else if (lastAgentGroup) {
            content = lastAgentGroup.querySelector('.anchored-terminal__agent-message-content');
        } else if (webSessionId) {
            // Check if there's already an AI response group for this session
            const existingResponse = this.getAIResponse(webSessionId);
            if (existingResponse) {
                content = existingResponse;
            } else {
                content = this.createAIResponse(webSessionId);
            }
        } else {
            // Create new agent message group if none exists and no webSessionId
            content = this.createAIResponse(id);
        }

        const widget = document.createElement('div');
        widget.id = id;
        widget.className = 'tribunal';

        const dots = Array.from({ length: numPasses || 3 }, (_, i) =>
            `<span class="tribunal__dot" data-pass="${i}" title="Pass ${i + 1}"></span>`
        ).join('');

        await templateLoader.renderTo(widget, 'tribunal', { dots });

        const commandEl = widget.querySelector('.tribunal__command');
        if (commandEl) {
            const parts = [];
            if (request) parts.push(request);
            if (guidelines) parts.push(`Guidelines: ${guidelines}`);
            commandEl.textContent = parts.join(' | ') || '';
        }

        content.appendChild(widget);
        this.scrollToBottom();
        return id;
    }

    updateTribunalPass(id, { passIndex, success }) {
        const widget = document.getElementById(id);
        if (!widget) return;
        const dot = widget.querySelector(`.tribunal__dot[data-pass="${passIndex}"]`);
        if (dot) {
            dot.classList.add(success ? 'tribunal__dot--ok' : 'tribunal__dot--fail');
        }
        const statusEl = widget.querySelector('.tribunal__status');
        if (statusEl) {
            statusEl.textContent = `Pass ${passIndex + 1} ${success ? 'complete' : 'failed'}`;
        }
    }

    updateTribunalStatus(id, text) {
        const widget = document.getElementById(id);
        if (!widget) return;
        const statusEl = widget.querySelector('.tribunal__status');
        if (statusEl) statusEl.textContent = text;
    }

    completeTribunal({ id, finalCommand, outcome }) {
        const widget = document.getElementById(id);
        if (!widget) return;

        widget.classList.add('tribunal--done');

        const spinner = widget.querySelector('.tribunal__spinner');
        if (spinner) spinner.remove();

        const icon = widget.querySelector('.tribunal__icon');
        if (icon) icon.textContent = 'check_circle';

        const statusEl = widget.querySelector('.tribunal__status');
        if (statusEl) {
            let outcomeLabel;
            if (outcome === TribunalOutcome.VERIFICATION_FAILED) {
                outcomeLabel = 'Revised';
            } else if (outcome === TribunalOutcome.CONSENSUS) {
                outcomeLabel = 'Consensus';
            } else {
                outcomeLabel = 'Verified';
            }
            statusEl.textContent = `${outcomeLabel} · ${finalCommand}`;
            statusEl.classList.add('tribunal__status--done');
        }

    }

    failTribunal({ id, eventType }) {
        const widget = document.getElementById(id);
        if (!widget) return;

        widget.classList.add('tribunal--failed');

        const spinner = widget.querySelector('.tribunal__spinner');
        if (spinner) spinner.remove();

        const icon = widget.querySelector('.tribunal__icon');
        if (icon) {
            icon.textContent = 'warning';
            icon.classList.add('tribunal__icon--failed');
        }

        const statusEl = widget.querySelector('.tribunal__status');
        if (statusEl) {
            statusEl.textContent = this._tribunalFailureLabel(eventType);
        }
    }

    _tribunalFailureLabel(eventType) {
        switch (eventType) {
            case EventType.TRIBUNAL_SESSION_DISABLED:
                return 'Tribunal disabled — no command produced';
            case EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED:
                return 'No model configured — Tribunal cannot run';
            case EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE:
                return 'LLM provider unavailable — Tribunal halted';
            case EventType.TRIBUNAL_SESSION_SYSTEM_ERROR:
                return 'System error — all passes failed (auth/network/config)';
            case EventType.TRIBUNAL_SESSION_GENERATION_FAILED:
                return 'All generation passes failed — no candidate produced';
            case EventType.TRIBUNAL_SESSION_VERIFIER_FAILED:
                return 'Verifier rejected the candidate — no trusted command';
            default:
                return 'Tribunal halted — no command produced';
        }
    }
}
