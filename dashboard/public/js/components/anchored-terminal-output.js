// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { templateLoader } from '../utils/template-loader.js';
import { TribunalOutcome, TribunalFallbackReason } from '../constants/events.js';

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

    showWaitingIndicator(webSessionId) {
        if (!this.outputContainer) return null;

        this._removeWelcome();
        this.hideWaitingIndicator();

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__ai-response waiting';
        entry.id = 'waiting-indicator';
        if (webSessionId) {
            entry.dataset.webSessionId = webSessionId;
        }

        const header = document.createElement('div');
        header.className = 'anchored-terminal__ai-response-header';

        const sender = document.createElement('span');
        sender.className = 'anchored-terminal__ai-response-sender';
        sender.textContent = 'DropOps';

        const time = document.createElement('span');
        time.className = 'anchored-terminal__ai-response-time';
        time.textContent = this.formatTimestamp();

        header.appendChild(time);
        header.appendChild(sender);

        const content = document.createElement('div');
        content.className = 'anchored-terminal__ai-response-content';

        const cursor = document.createElement('span');
        cursor.className = 'anchored-terminal__waiting-cursor';
        content.appendChild(cursor);

        entry.appendChild(header);
        entry.appendChild(content);

        this.outputContainer.appendChild(entry);
        this.scrollToBottom();

        return entry;
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

    getOrCreateAIResponse(webSessionId) {
        if (!this.outputContainer) return null;

        this._removeWelcome();
        this.hideWaitingIndicator();

        const existingId = `ai-response-${webSessionId}`;
        const existing = document.getElementById(existingId);
        if (existing) {
            return existing;
        }

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__ai-response streaming';
        entry.id = existingId;

        const header = document.createElement('div');
        header.className = 'anchored-terminal__ai-response-header';

        const sender = document.createElement('span');
        sender.className = 'anchored-terminal__ai-response-sender';
        sender.textContent = 'DropOps';

        const time = document.createElement('span');
        time.className = 'anchored-terminal__ai-response-time';
        time.textContent = this.formatTimestamp();

        header.appendChild(time);
        header.appendChild(sender);

        const content = document.createElement('div');
        content.className = 'anchored-terminal__ai-response-content';

        entry.appendChild(header);
        entry.appendChild(content);

        this.outputContainer.appendChild(entry);
        this.scrollToBottom();

        return entry;
    }

    appendAIResponseChunk(webSessionId, html) {
        const entry = this.getOrCreateAIResponse(webSessionId);
        if (!entry) return;

        const contentEl = entry.querySelector('.anchored-terminal__ai-response-content');
        if (contentEl) {
            contentEl.innerHTML = html;
        }

        this.scrollToBottom();
    }

    finalizeAIResponseChunk(webSessionId, finalHtml) {
        const entry = document.getElementById(`ai-response-${webSessionId}`);
        if (!entry) return;

        const contentEl = entry.querySelector('.anchored-terminal__ai-response-content');
        if (contentEl) {
            contentEl.innerHTML = finalHtml;
        }

        entry.classList.remove('streaming');
        entry.querySelectorAll('.streaming-cursor').forEach(c => c.remove());
        entry.id = `ai-response-${webSessionId}-${Date.now()}`;

        this.scrollToBottom();
    }

    applyCitationsAfterFinalize(webSessionId, groundingMetadata) {
        this.applyCitations(webSessionId, groundingMetadata);
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

        let entry = document.getElementById(`ai-response-${webSessionId}`);
        if (!entry) {
            const candidates = this.outputContainer?.querySelectorAll(`[id^="ai-response-${webSessionId}-"]`);
            if (candidates && candidates.length > 0) {
                entry = candidates[candidates.length - 1];
            }
        }
        if (!entry) return;

        const contentEl = entry.querySelector('.anchored-terminal__ai-response-content');
        if (!contentEl) return;

        const citedHtml = citationsHandler.addInlineCitations(contentEl.innerHTML, groundingMetadata);
        contentEl.innerHTML = citedHtml;

        const sourcesPanel = citationsHandler.renderSourcesPanel(sources);
        contentEl.appendChild(sourcesPanel);

        this.scrollToBottom();
    }

    appendAIResponse(message, timestamp = null, groundingMetadata = null) {
        if (!this.outputContainer) return;

        this._removeWelcome();

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__ai-response';

        const header = document.createElement('div');
        header.className = 'anchored-terminal__ai-response-header';

        const sender = document.createElement('span');
        sender.className = 'anchored-terminal__ai-response-sender';
        sender.textContent = 'DropOps';

        const time = document.createElement('span');
        time.className = 'anchored-terminal__ai-response-time';
        time.textContent = timestamp || this.formatTimestamp();

        header.appendChild(time);
        header.appendChild(sender);

        const citationsHandler = this.citationsHandler;
        const sources = groundingMetadata?.sources;
        if (citationsHandler && groundingMetadata?.grounding_used && sources?.length) {
            message = citationsHandler.addInlineCitations(message, groundingMetadata);
        }

        const content = document.createElement('div');
        content.className = 'anchored-terminal__ai-response-content';
        content.innerHTML = message;

        if (citationsHandler && groundingMetadata?.grounding_used && sources?.length) {
            const sourcesPanel = citationsHandler.renderSourcesPanel(sources);
            content.appendChild(sourcesPanel);
        }

        entry.appendChild(header);
        entry.appendChild(content);

        this.outputContainer.appendChild(entry);
        this.scrollToBottom();

        return entry;
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

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__thinking active';
        entry.id = existingId;

        const header = document.createElement('div');
        header.className = '';
        header.innerHTML = '<span class="anchored-terminal__thinking-label">Thoughts</span>';

        header.addEventListener('click', () => {
            entry.classList.toggle('collapsed');
        });

        const content = document.createElement('div');
        content.className = 'anchored-terminal__thinking-content';

        entry.appendChild(header);
        entry.appendChild(content);

        this.outputContainer.appendChild(entry);
        this.scrollToBottom();

        return entry;
    }

    appendThinkingContent(webSessionId, text) {
        const entry = this.getOrCreateThinkingEntry(webSessionId);
        if (!entry) return;

        const contentEl = entry.querySelector('.anchored-terminal__thinking-content');
        if (contentEl) {
            const existing = this.thinkingContentRaw.get(webSessionId);
            const newRaw = existing ? existing + '\n' + text : text;
            this.thinkingContentRaw.set(webSessionId, newRaw);

            const renderer = this.markdownRenderer;
            if (renderer) {
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
            entry.id = `thinking-${webSessionId}-${Date.now()}`;
        }

        this.thinkingContentRaw.delete(webSessionId);
    }

    appendActivityIndicator(options) {
        if (!this.outputContainer) return null;

        const { id, category, icon, label, detail } = options || {};

        this._removeWelcome();

        const indicator = document.createElement('div');
        indicator.id = id;
        indicator.className = `anchored-terminal__activity category-${category}`;
        const activityTemplate = templateLoader.cache.get('activity-indicator');
        indicator.innerHTML = templateLoader.replace(activityTemplate, {
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

    appendErrorMessage(text) {
        if (!this.outputContainer) return;

        this._removeWelcome();

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__error-message';

        const header = document.createElement('div');
        header.className = 'anchored-terminal__error-header';
        header.innerHTML = '<span class="material-symbols-outlined">error</span>Error';

        const content = document.createElement('div');
        content.className = 'anchored-terminal__error-content';
        content.textContent = text;

        entry.appendChild(header);
        entry.appendChild(content);
        this.outputContainer.appendChild(entry);
        this.scrollToBottom();
    }

    showTribunal({ id, model, numPasses, command }) {
        if (!this.outputContainer) return null;

        this._removeWelcome();

        const widget = document.createElement('div');
        widget.id = id;
        widget.className = 'tribunal';

        const dots = Array.from({ length: numPasses || 3 }, (_, i) =>
            `<span class="tribunal__dot" data-pass="${i}" title="Pass ${i + 1}"></span>`
        ).join('');

        const tribunalTemplate = templateLoader.cache.get('tribunal');
        widget.innerHTML = templateLoader.replace(tribunalTemplate, {
            dots
        });

        const commandEl = widget.querySelector('.tribunal__command');
        if (commandEl) commandEl.textContent = command;

        this.outputContainer.appendChild(widget);
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

    failTribunal({ id, reason }) {
        const widget = document.getElementById(id);
        if (!widget) return;

        widget.classList.add('tribunal--fallback');

        const spinner = widget.querySelector('.tribunal__spinner');
        if (spinner) spinner.remove();

        const icon = widget.querySelector('.tribunal__icon');
        if (icon) {
            icon.textContent = 'info';
            icon.classList.add('tribunal__icon--fallback');
        }

        const statusEl = widget.querySelector('.tribunal__status');
        if (statusEl) {
            let reasonLabel;
            if (reason === TribunalFallbackReason.DISABLED) {
                reasonLabel = 'TribunalDisabled';
            } else if (reason === TribunalFallbackReason.PROVIDER_UNAVAILABLE) {
                reasonLabel = 'Tribunal unavailable';
            } else if (reason === TribunalFallbackReason.ALL_PASSES_FAILED) {
                reasonLabel = 'All passes failed — using original';
            } else if (reason === TribunalFallbackReason.NO_VOTE_WINNER) {
                reasonLabel = 'No consensus — using original';
            } else {
                reasonLabel = 'Using original command';
            }
            statusEl.textContent = reasonLabel;
        }

    }
}
