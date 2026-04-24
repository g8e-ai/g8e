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

import { ApiPaths } from '../constants/api-paths.js';
import { ComponentName } from '../constants/service-client-constants.js';
import { devLogger } from '../utils/dev-logger.js';
import { formatForDisplay } from '../utils/timestamp.js';

function escHtml(str) {
    if (str == null) return '';
    if (typeof str !== 'string') str = String(str);
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

export class AuditPage {
    constructor() {
        this.tableContainer   = null;
        this.loadingIndicator = null;
        this.totalEventsEl    = null;
        this.chatCountEl      = null;
        this.approvalCountEl  = null;
        this.commandCountEl   = null;
        this.fileEditCountEl  = null;
        this.dateRangeEl      = null;
        this.fromDateInput    = null;
        this.toDateInput      = null;
        this.applyFiltersBtn  = null;
        this.downloadCsvBtn   = null;
        this.downloadJsonBtn  = null;
    }

    init() {
        if (this._initialized) return;
        this._initialized = true;
        this.setupDOMElements();
        this.setupEventListeners();
        this.loadAuditData();
    }

    setupDOMElements() {
        this.tableContainer   = document.getElementById('audit-table-container');
        this.loadingIndicator = document.getElementById('loading-indicator');
        this.totalEventsEl    = document.getElementById('total-events');
        this.chatCountEl      = document.getElementById('chat-count');
        this.approvalCountEl  = document.getElementById('approval-count');
        this.commandCountEl   = document.getElementById('command-count');
        this.fileEditCountEl  = document.getElementById('file-edit-count');
        this.dateRangeEl      = document.getElementById('date-range');
        this.fromDateInput    = document.getElementById('from-date');
        this.toDateInput      = document.getElementById('to-date');
        this.applyFiltersBtn  = document.getElementById('apply-filters-btn');
        this.downloadCsvBtn   = document.getElementById('download-csv-btn');
        this.downloadJsonBtn  = document.getElementById('download-json-btn');
    }

    setupEventListeners() {
        if (this._eventListenersRegistered) return;

        this.applyFiltersBtn.addEventListener('click', () => this.loadAuditData());
        this.downloadCsvBtn.addEventListener('click', () => this._download('csv'));
        this.downloadJsonBtn.addEventListener('click', () => this._download('json'));
        document.addEventListener('click', (e) => this._handleCopyClick(e));

        this._eventListenersRegistered = true;
    }

    formatTimestamp(timestamp) {
        if (!timestamp) return '--';
        return formatForDisplay(timestamp);
    }

    renderTable(events) {
        if (events.length === 0) {
            this.tableContainer.innerHTML = `
                <div class="empty-state">
                    <span class="material-symbols-outlined empty-state-icon">history</span>
                    <p class="empty-state-title">No audit events found</p>
                    <p class="empty-state-subtitle">Start a conversation to generate events</p>
                </div>
            `;
            return;
        }

        const sorted = [...events].sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

        let html = `<table class="audit-table">
            <thead><tr>
                <th>Time</th><th>Type</th><th>Actor</th><th>Details</th><th>Operator</th>
            </tr></thead><tbody>`;

        for (const event of sorted) {
            const eventType    = escHtml(event.event_type || '--');
            const details      = escHtml(event.summary || event.details || event.content || '--');
            const operatorName = escHtml(event.operator_name || event.operator_id?.slice(0, 8) || '--');
            const actor        = escHtml(event.actor || 'system');

            html += `<tr>
                <td class="audit-cell-time">${this.formatTimestamp(event.timestamp)}</td>
                <td><span class="event-type-label">${eventType}</span></td>
                <td><span class="actor-label">${actor}</span></td>
                <td class="audit-cell-details" title="${details}">${details}</td>
                <td class="audit-cell-operator">${operatorName}</td>
            </tr>`;
        }

        html += '</tbody></table>';
        this.tableContainer.innerHTML = html;
    }

    updateStats(events) {
        this.totalEventsEl.textContent = events.length.toLocaleString();

        let chatCount = 0;
        let approvalCount = 0;
        let commandCount = 0;
        let fileEditCount = 0;

        for (const event of events) {
            const type = event.event_type || '';
            if (type.includes('.chat.')) {
                chatCount++;
            } else if (type.includes('.approval.')) {
                approvalCount++;
            } else if (type.includes('.command.')) {
                commandCount++;
            } else if (type.includes('.file.edit.')) {
                fileEditCount++;
            }
        }

        if (this.chatCountEl) this.chatCountEl.textContent = chatCount.toLocaleString();
        if (this.approvalCountEl) this.approvalCountEl.textContent = approvalCount.toLocaleString();
        if (this.commandCountEl) this.commandCountEl.textContent = commandCount.toLocaleString();
        if (this.fileEditCountEl) this.fileEditCountEl.textContent = fileEditCount.toLocaleString();

        if (events.length > 0) {
            const timestamps = events.map(e => new Date(e.timestamp)).filter(d => !isNaN(d));
            if (timestamps.length > 0) {
                const min = new Date(Math.min(...timestamps));
                const max = new Date(Math.max(...timestamps));
                this.dateRangeEl.textContent = `${min.toLocaleDateString()} - ${max.toLocaleDateString()}`;
            }
        } else {
            this.dateRangeEl.textContent = '--';
        }
    }

    showError(message) {
        this.tableContainer.innerHTML = `
            <div class="empty-state">
                <span class="material-symbols-outlined empty-state-icon error">error</span>
                <p class="empty-state-title">${escHtml(message)}</p>
                <button class="audit-btn empty-state-action" id="retry-btn">Retry</button>
            </div>
        `;
        document.getElementById('retry-btn').addEventListener('click', () => {
            window.location.reload();
        });
    }

    async loadAuditData() {
        const params = new URLSearchParams();
        if (this.fromDateInput.value) params.append('from_date', new Date(this.fromDateInput.value).toISOString());
        if (this.toDateInput.value) params.append('to_date', new Date(this.toDateInput.value + 'T23:59:59').toISOString());

        const base = ApiPaths.audit.events();
        const url = params.toString() ? `${base}?${params.toString()}` : base;

        this.loadingIndicator.classList.add('active');
        this.tableContainer.innerHTML = '';
        this.tableContainer.appendChild(this.loadingIndicator);

        try {
            const response = await window.serviceClient.get(ComponentName.G8ED, url);
            const data = await response.json();
            if (!data.success) {
                throw new Error(data.error || 'Failed to load audit log');
            }
            const events = Array.isArray(data.events) ? data.events : [];
            this.updateStats(events);
            this.renderTable(events);
        } catch (e) {
            devLogger.error('Failed to load audit data:', e);
            this.showError(e.message || 'Failed to load audit log');
        }
    }

    _download(format) {
        const params = new URLSearchParams();
        params.append('format', format);
        if (this.fromDateInput.value) params.append('from_date', new Date(this.fromDateInput.value).toISOString());
        if (this.toDateInput.value) params.append('to_date', new Date(this.toDateInput.value + 'T23:59:59').toISOString());
        window.location.href = `${ApiPaths.audit.download()}?${params.toString()}`;
    }

    _handleCopyClick(e) {
        const copyBtn = e.target.closest('.copy-btn');
        if (!copyBtn) return;
        const code = copyBtn.parentElement.querySelector('.command-code');
        if (!code) return;
        navigator.clipboard.writeText(code.textContent).then(() => {
            const icon = copyBtn.querySelector('.material-symbols-outlined');
            icon.textContent = 'check';
            copyBtn.classList.add('copied');
            setTimeout(() => {
                icon.textContent = 'content_copy';
                copyBtn.classList.remove('copied');
            }, 2000);
        });
    }

    destroy() {
        this.tableContainer   = null;
        this.loadingIndicator = null;
        this.totalEventsEl    = null;
        this.chatCountEl      = null;
        this.approvalCountEl  = null;
        this.commandCountEl   = null;
        this.fileEditCountEl  = null;
        this.dateRangeEl      = null;
        this.fromDateInput    = null;
        this.toDateInput      = null;
        this.applyFiltersBtn  = null;
        this.downloadCsvBtn   = null;
        this.downloadJsonBtn  = null;
        this._initialized              = false;
        this._eventListenersRegistered = false;
    }
}

if (typeof document !== 'undefined') {
    const auditPage = new AuditPage();
    auditPage.init();
}
