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
import { TribunalMemberIcons } from '../constants/agents.js';

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

    _findAIResponseGroup(webSessionId) {
        const id = `ai-response-${webSessionId}`;
        return document.getElementById(id);
    }

    createAIResponse(webSessionId) {
        if (!this.outputContainer) return null;

        const existingId = `ai-response-${webSessionId}`;
        const existing = document.getElementById(existingId);
        if (existing) {
            if (existing.dataset.sealed === 'true') {
                // Rename old sealed bubble to move it to history
                existing.id = `${existingId}-${Date.now()}`;
            } else {
                throw new Error(`AI response element ${existingId} already exists and is not sealed`);
            }
        }

        this._removeWelcome();
        this.hideWaitingIndicator();

        const group = document.createElement('div');
        group.className = 'anchored-terminal__agent-message-group streaming';
        group.id = existingId;

        const header = this._createAgentMessageHeader();

        const content = document.createElement('div');
        content.className = 'anchored-terminal__agent-message-content';

        const streamingTextContainer = document.createElement('div');
        streamingTextContainer.className = 'anchored-terminal__streaming-text-container';
        content.appendChild(streamingTextContainer);

        group.appendChild(header);
        group.appendChild(content);

        this.outputContainer.appendChild(group);
        this.scrollToBottom();

        return streamingTextContainer;
    }

    getAIResponse(webSessionId) {
        const group = this._findAIResponseGroup(webSessionId);
        if (!group || group.dataset.sealed === 'true') {
            return null;
        }
        return group.querySelector('.anchored-terminal__streaming-text-container');
    }

    appendStreamingTextChunk(webSessionId, text) {
        let streamingContainer = this.getAIResponse(webSessionId);
        if (!streamingContainer) {
            streamingContainer = this.createAIResponse(webSessionId);
        }
        if (!streamingContainer) return;

        if (!this._streamingTextAccumulator) {
            this._streamingTextAccumulator = new Map();
        }

        const existing = this._streamingTextAccumulator.get(webSessionId) || '';
        const newText = existing + text;
        this._streamingTextAccumulator.set(webSessionId, newText);

        const renderer = this.markdownRenderer;
        if (renderer) {
            streamingContainer.innerHTML = renderer.parseMarkdown(newText, true);
        } else {
            streamingContainer.textContent = newText;
        }

        this.scrollToBottom();
    }

    replaceStreamingHtml(webSessionId, html) {
        let streamingContainer = this.getAIResponse(webSessionId);
        if (!streamingContainer) {
            streamingContainer = this.createAIResponse(webSessionId);
        }
        if (!streamingContainer) return;

        // TRUSTED: HTML from markdown renderer (should be sanitized by markdown-it with DOMPurify)
        streamingContainer.innerHTML = html;

        this.scrollToBottom();
    }

    finalizeAIResponseChunk(webSessionId, finalHtml, groundingMetadata = null) {
        const group = this._findAIResponseGroup(webSessionId);
        if (!group) return;

        const contentEl = group.querySelector('.anchored-terminal__agent-message-content');
        const streamingContainer = group.querySelector('.anchored-terminal__streaming-text-container');
        
        if (streamingContainer) {
            // TRUSTED: Final HTML from markdown renderer (should be sanitized by markdown-it with DOMPurify)
            streamingContainer.innerHTML = finalHtml;

            // Apply citations if grounding metadata is provided
            if (groundingMetadata && contentEl) {
                this.applyCitations(webSessionId, groundingMetadata, contentEl);
            }
        }

        group.classList.remove('streaming');
        group.dataset.sealed = 'true';
        group.querySelectorAll('.streaming-cursor').forEach(c => c.remove());

        if (this._streamingTextAccumulator) {
            this._streamingTextAccumulator.delete(webSessionId);
        }

        this.scrollToBottom();
    }

    applyCitations(webSessionId, groundingMetadata, contentEl = null) {
        if (!groundingMetadata || !groundingMetadata.grounding_used) return;

        const sources = groundingMetadata.sources;
        if (!sources || !sources.length) return;

        const citationsHandler = this.citationsHandler;
        if (!citationsHandler) {
            console.warn('[ANCHORED TERMINAL] No CitationsHandler available for citations rendering');
            return;
        }

        const group = this._findAIResponseGroup(webSessionId);
        if (!group) return;

        if (!contentEl) {
            contentEl = group.querySelector('.anchored-terminal__agent-message-content');
        }
        if (!contentEl) return;

        const streamingContainer = group.querySelector('.anchored-terminal__streaming-text-container');
        if (!streamingContainer) return;

        // TRUSTED: HTML from citations handler adds citation markers to already-sanitized markdown output
        const citedHtml = citationsHandler.addInlineCitations(streamingContainer.innerHTML, groundingMetadata);
        streamingContainer.innerHTML = citedHtml;

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
            let newRaw;
            if (existing) {
                // Only add newline if existing doesn't already end with one and new chunk doesn't start with one
                const needsSeparator = !existing.endsWith('\n') && !text.startsWith('\n');
                newRaw = needsSeparator ? existing + '\n' + text : existing + text;
            } else {
                newRaw = text;
            }
            this.thinkingContentRaw.set(webSessionId, newRaw);

            const title = this._extractThinkingTitle(newRaw);
            if (title) {
                const titleEl = entry.querySelector('.anchored-terminal__thinking-title');
                if (titleEl) titleEl.textContent = title;
            }

            const renderer = this.markdownRenderer;
            if (renderer) {
                // Only render markdown at sentence boundaries to avoid processing word-per-chunk
                const shouldRender = newRaw.match(/[.!?]\s*$/);
                if (shouldRender) {
                    // TRUSTED: HTML from markdown renderer (should be sanitized by markdown-it with DOMPurify)
                    contentEl.innerHTML = renderer.parseMarkdown(newRaw);
                }
                // For incomplete sentences, don't re-render - keep previous markdown until next complete sentence
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
        const group = this._findAIResponseGroup(webSessionId);
        if (group) {
            group.classList.remove('streaming');
            group.dataset.sealed = 'true';
            group.querySelectorAll('.streaming-cursor').forEach(c => c.remove());
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

    async showTribunal({ id, model, numPasses, request, guidelines, webSessionId, correlationId }) {
        if (!this.outputContainer) return null;

        this._removeWelcome();

        // The Tribunal refining widget visually supersedes the "Preparing"
        // indicator for the same command. Absorb any active preparing
        // indicator by removing it and reusing its execution bubble, so the
        // refining/approval card replaces preparing in place rather than
        // rendering alongside it. This also clears the activeExecutions
        // entry whose execId would otherwise never match a subsequent
        // OPERATOR_COMMAND_STARTED (which uses a per-operator exec_id).
        let content = null;
        if (this.activeExecutions && this.activeExecutions.size > 0) {
            for (const [execId, info] of this.activeExecutions) {
                const indicator = info?.indicatorId ? document.getElementById(info.indicatorId) : null;
                if (!indicator) continue;
                const prepGroup = indicator.closest('.anchored-terminal__agent-message-group');
                indicator.remove();
                this.activeExecutions.delete(execId);
                if (prepGroup) {
                    content = prepGroup.querySelector('.anchored-terminal__agent-message-content');
                }
                break;
            }
        }

        // Otherwise prefer a dedicated execution bubble. Never reuse a still-
        // streaming AI response group (id="ai-response-<wsid>"): its innerHTML
        // is overwritten by subsequent text chunks, which would wipe out the
        // refining/approval card.
        if (!content) {
            const lastExecutionGroup = this.outputContainer.querySelector(
                '.anchored-terminal__agent-message-group[data-execution-bubble]:last-of-type'
            );
            if (lastExecutionGroup) {
                content = lastExecutionGroup.querySelector('.anchored-terminal__agent-message-content');
            }
        }

        if (!content) {
            // Create a fresh execution bubble rather than reusing any active
            // streaming AI response group.
            const group = document.createElement('div');
            group.className = 'anchored-terminal__agent-message-group anchored-terminal__agent-message-group--execution';
            group.setAttribute('data-execution-bubble', id);
            const header = this._createAgentMessageHeader();
            content = document.createElement('div');
            content.className = 'anchored-terminal__agent-message-content';
            group.appendChild(header);
            group.appendChild(content);
            this.outputContainer.appendChild(group);
        }

        const widget = document.createElement('div');
        widget.id = id;
        widget.className = 'anchored-terminal__approval';
        widget.setAttribute('data-approval-refining', '1');
        if (webSessionId) widget.setAttribute('data-web-session-id', webSessionId);
        if (correlationId) widget.setAttribute('data-correlation-id', correlationId);

        const dots = Array.from({ length: numPasses || 3 }, (_, i) => {
            const icon = TribunalMemberIcons[i] || 'circle';
            return `<span class="tribunal__dot" data-pass="${i}" title="Pass ${i + 1}">
                <span class="material-symbols-outlined tribunal__dot-icon">${icon}</span>
            </span>`;
        }).join('');

        const tribunalHtml =
            `<span class="tribunal__passes">${dots}</span>` +
            `<span class="tribunal__status">Generating alternatives...</span>` +
            `<span class="tribunal__spinner"></span>`;

        const parts = [];
        if (request) parts.push(request);
        if (guidelines) parts.push(`Guidelines: ${guidelines}`);
        const refiningSubject = parts.join(' | ');

        await templateLoader.renderTo(widget, 'approval-card', {
            cardModifier: 'approval-compact--refining',
            icon: 'auto_fix_high',
            iconModifier: 'approval-compact__icon--refining',
            headerText: 'Refining command',
            tribunalHtml,
            riskBadgeHtml: '',
            promptHtml: '',
            commandDisplay: refiningSubject,
            systemsHtml: '',
            justification: '',
            approvalId: '',
            approveButtonText: 'Approve',
        });

        content.appendChild(widget);
        this.scrollToBottom();
        return id;
    }

    updateTribunalPass(id, { passIndex, success, candidate }) {
        const widget = document.getElementById(id);
        if (!widget) return;
        const dot = widget.querySelector(`.tribunal__dot[data-pass="${passIndex}"]`);
        if (dot) {
            dot.classList.add(success ? 'tribunal__dot--ok' : 'tribunal__dot--fail');
            if (candidate) {
                dot.setAttribute('title', `Pass ${passIndex + 1}: ${candidate}`);
            }
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

        const spinner = widget.querySelector('.tribunal__spinner');
        if (spinner) spinner.remove();

        const statusEl = widget.querySelector('.tribunal__status');
        if (statusEl) {
            let outcomeLabel;
            if (outcome === TribunalOutcome.VERIFICATION_FAILED) {
                outcomeLabel = 'Revised';
            } else if (outcome === TribunalOutcome.CONSENSUS) {
                outcomeLabel = 'Consensus';
            } else if (outcome === TribunalOutcome.CONSENSUS_FAILED) {
                outcomeLabel = 'Consensus failed';
                statusEl.classList.add('tribunal__status--failed');
            } else {
                outcomeLabel = 'Verified';
            }
            const statusText = finalCommand ? `${outcomeLabel} · ${finalCommand}` : outcomeLabel;
            statusEl.textContent = statusText;
            statusEl.classList.add('tribunal__status--done');
        }
    }

    failTribunal({ id, eventType }) {
        const widget = document.getElementById(id);
        if (!widget) return;

        const card = widget.querySelector('.approval-compact') || widget;
        card.classList.add('approval-compact--refining-failed');

        const spinner = widget.querySelector('.tribunal__spinner');
        if (spinner) spinner.remove();

        const icon = widget.querySelector('.approval-compact__icon');
        if (icon) {
            icon.textContent = 'warning';
            icon.classList.add('approval-compact__icon--failed');
        }

        const statusEl = widget.querySelector('.tribunal__status');
        if (statusEl) {
            statusEl.textContent = this._tribunalFailureLabel(eventType);
            statusEl.classList.add('tribunal__status--failed');
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
                return 'Auditor rejected the candidate — no trusted command';
            default:
                return 'Tribunal halted — no command produced';
        }
    }
}
