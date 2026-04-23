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

export const TEMPLATE_FIXTURES = {
    'approval-card': `<div class="approval-compact {{!cardModifier}}">
    <div class="approval-compact__header">
        <span class="material-symbols-outlined approval-compact__icon {{!iconModifier}}">{{{icon}}}</span>
        <span class="approval-compact__title">{{headerText}}</span>
        {{{tribunalHtml}}}
        {{{riskBadgeHtml}}}
    </div>
    <div class="approval-compact__command">{{{promptHtml}}}{{commandDisplay}}</div>
    {{{systemsHtml}}}
    <div class="approval-compact__reason">{{justification}}</div>
    <div class="approval-compact__actions">
        <button class="approval-compact__btn approval-compact__btn--approve" data-approval-id="{{!approvalId}}">
            <span class="material-symbols-outlined">check</span>{{approveButtonText}}
        </button>
        <button class="approval-compact__btn approval-compact__btn--deny" data-approval-id="{{!approvalId}}">
            <span class="material-symbols-outlined">close</span>Deny
        </button>
    </div>
</div>`,

    'approval-card-restored': `<div class="approval-compact approval-compact--restored">
    <div class="approval-compact__header">
        <span class="material-symbols-outlined approval-compact__icon">{{{icon}}}</span>
        <span class="approval-compact__title">{{headerText}}</span>
        {{{timeHtml}}}
    </div>
    <div class="approval-compact__command">{{{promptHtml}}}{{commandDisplay}}</div>
    <div class="approval-compact__reason">{{justification}}</div>
    <div class="approval-compact__actions">
        <div class="approval-compact__status approval-compact__status--{{!statusClass}}">
            <span class="material-symbols-outlined">{{{statusIcon}}}</span>
            {{statusText}}
        </div>
    </div>
</div>`,

    'approval-status': `<div class="approval-compact__status approval-compact__status--{{!statusClass}}">
    <span class="material-symbols-outlined">{{{statusIcon}}}</span>
    {{statusText}}
</div>`,

    'command-result': `<div class="anchored-terminal__result-header">
    <span class="anchored-terminal__result-icon anchored-terminal__result-icon--{{!statusClass}}">{{{statusIcon}}}</span>
    {{{hostnameHtml}}}
    <span class="anchored-terminal__result-command">$ {{{command}}}</span>
    <span class="anchored-terminal__result-time">{{displayTime}}</span>
</div>
<div class="anchored-terminal__result-output anchored-terminal__result-output--{{!statusClass}}">
    {{{outputContent}}}
</div>
{{{exitCodeHtml}}}`,

    'executing-indicator': `<div class="anchored-terminal__executing-spinner"></div>
<span>Executing: {{command}}</span>`,

    'preparing-indicator': `<div class="anchored-terminal__executing-spinner"></div>
<span>Preparing: {{command}}</span>`,

    'results-toggle': `<span class="anchored-terminal__results-toggle-icon">expand_more</span>
<span class="anchored-terminal__results-toggle-label">Results</span>
<span class="anchored-terminal__results-count">0</span>`,

    'activity-indicator': `<div class="anchored-terminal__activity-content"><span class="material-symbols-outlined anchored-terminal__activity-icon">{{{icon}}}</span><span class="anchored-terminal__activity-label">{{{label}}}</span>{{{detailHtml}}}<span class="anchored-terminal__activity-spinner"></span></div>`,

    'operator-deployment': `<div class="opdeploy"><div class="opdeploy__header"><span class="opdeploy__header-text">Getting Started</span></div><div class="opdeploy__steps"></div><div class="opdeploy__footer"></div></div>`,

    'bind-single-confirmation-overlay': `<div class="bind-single-confirmation-overlay">
    <div class="bind-single-header">
        <span class="material-symbols-outlined" data-bind-icon>link</span>
        <h2 data-modal-title>Bind Operator to WebSession</h2>
        <p data-modal-subtitle>Connect to current web session</p>
    </div>
    <div class="bind-single-content">
        <p data-modal-description>This will connect the Operator to your current web session.</p>
        <div data-operators-list></div>
    </div>
    <div class="bind-single-actions">
        <div data-processing-indicator class="initially-hidden">
            <span class="material-symbols-outlined">hourglass_empty</span>
            <span data-processing-label>Binding operator...</span>
        </div>
        <div class="bind-single-buttons">
            <button data-action="cancel">Cancel</button>
            <button data-action="confirm">
                <span class="material-symbols-outlined" data-confirm-icon>link</span>
                <span data-confirm-label>Bind Operator</span>
            </button>
            <button data-action="close" class="bind-single-close">✕</button>
        </div>
    </div>
</div>`,

    'operator-bind-single-overlay': `<div class="operator-bind-single-overlay">
    <div class="operator-bind-single-header">
        <span class="material-symbols-outlined" data-bind-icon>link</span>
        <h2 data-modal-title>Bind Operator to WebSession</h2>
        <p data-modal-subtitle>Connect to current web session</p>
    </div>
    <div class="operator-bind-single-content">
        <p data-modal-description>This will connect the Operator to your current web session.</p>
        <div id="operator-bind-single-operators-list"></div>
    </div>
    <div class="operator-bind-single-actions">
        <div id="operator-bind-single-processing" class="initially-hidden">
            <span class="material-symbols-outlined">hourglass_empty</span>
            <span data-processing-label>Binding operator...</span>
        </div>
        <div class="operator-bind-single-buttons">
            <button id="operator-bind-single-cancel-btn">Cancel</button>
            <button id="operator-bind-single-confirm-btn">
                <span class="material-symbols-outlined" data-confirm-icon>link</span>
                <span data-confirm-label>Bind Operator</span>
            </button>
            <button class="operator-bind-single-close">✕</button>
        </div>
    </div>
</div>`,

    'bind-all-confirmation-overlay': `<div class="bind-all-confirmation-overlay">
    <div class="bind-all-header">
        <h2>Bind All Active Operators</h2>
        <p data-operator-count>0 operators will be bound</p>
    </div>
    <div class="bind-all-content">
        <div data-operators-list></div>
    </div>
    <div class="bind-all-actions">
        <div data-processing-indicator class="initially-hidden">
            <span class="material-symbols-outlined">hourglass_empty</span>
            <span>Binding operators...</span>
        </div>
        <div class="bind-all-buttons">
            <button data-action="cancel">Cancel</button>
            <button data-action="confirm">Bind All</button>
            <button data-action="close" class="bind-all-close">✕</button>
        </div>
    </div>
</div>`,

    'unbind-all-confirmation-overlay': `<div class="unbind-all-confirmation-overlay">
    <div class="unbind-all-header">
        <h2>Unbind All Operators</h2>
        <p data-operator-count>0 operators will be unbound</p>
    </div>
    <div class="unbind-all-content">
        <div data-operators-list></div>
    </div>
    <div class="bind-all-actions">
        <div data-processing-indicator class="initially-hidden">
            <span class="material-symbols-outlined">hourglass_empty</span>
            <span>Unbinding operators...</span>
        </div>
        <div class="bind-all-buttons">
            <button data-action="cancel">Cancel</button>
            <button data-action="confirm">Unbind All</button>
            <button data-action="close" class="unbind-all-close">✕</button>
        </div>
    </div>
</div>`,

    'bind-all-operator-item': `<div class="bind-all-operator-item" data-operator-id="{{!operatorId}}">
    <div class="operator-item-info">
        <span class="material-symbols-outlined" data-ip-icon>{{{ipIcon}}}</span>
        <span class="operator-hostname">{{hostname}}</span>
        <span class="operator-os">{{os}}</span>
        <span class="operator-ip">{{ip}}</span>
    </div>
    <div class="operator-item-status {{!statusClass}}">{{statusLabel}}</div>
</div>`,

    'bind-result-feedback': `<div class="bind-result-feedback {{!resultClass}}">
    <span class="material-symbols-outlined">{{{icon}}}</span>
    <span class="feedback-message">{{message}}</span>
</div>`,

    'operator-bind-all-overlay': `<div class="operator-bind-all-overlay" id="operator-bind-all-overlay">
    <div class="operator-bind-all-header">
        <div class="operator-bind-all-header-content">
            <span class="material-symbols-outlined operator-bind-all-icon">link</span>
            <div class="operator-bind-all-title-section">
                <h3 class="operator-bind-all-title">Bind All Active Operators</h3>
                <span class="operator-bind-all-count" id="operator-bind-all-count"></span>
            </div>
        </div>
        <button class="operator-bind-all-close-btn" id="operator-bind-all-close-btn">
            <span class="material-symbols-outlined">close</span>
        </button>
    </div>

    <div class="operator-bind-all-description">
        The following operators will be bound to your current web session. You will be able to interact with all of them through the chat interface.
    </div>

    <div class="operator-bind-all-operators-container">
        <div class="operator-bind-all-operators-header">
            <input type="checkbox" id="operator-select-all-operators" class="operator-select-all-checkbox" checked>
            <label for="operator-select-all-operators">Select All</label>
        </div>
        <div class="operator-bind-all-operators-list" id="operator-bind-all-operators-list">
        </div>
    </div>

    <div class="operator-bind-all-actions">
        <button class="operator-bind-all-action-btn cancel-btn" id="operator-bind-all-cancel-btn">
            <span class="material-symbols-outlined">cancel</span>
            <span>Cancel</span>
        </button>
        <button class="operator-bind-all-action-btn confirm-btn" id="operator-bind-all-confirm-btn">
            <span class="material-symbols-outlined">link</span>
            <span>Bind All</span>
        </button>
    </div>

    <div class="operator-bind-all-processing initially-hidden" id="operator-bind-all-processing">
        <div class="spinner"></div>
        <span id="operator-bind-all-processing-label">Binding operators...</span>
    </div>
    <div class="operator-bind-all-actions-feedback" id="operator-bind-all-feedback"></div>
</div>`,

    'operator-unbind-all-overlay': `<div class="operator-unbind-all-overlay" id="operator-unbind-all-overlay">
    <div class="operator-unbind-all-header">
        <div class="operator-unbind-all-header-content">
            <span class="material-symbols-outlined operator-unbind-all-icon">link_off</span>
            <div class="operator-unbind-all-title-section">
                <h3 class="operator-unbind-all-title">Unbind All Operators</h3>
                <span class="operator-unbind-all-count" id="operator-unbind-all-count"></span>
            </div>
        </div>
        <button class="operator-unbind-all-close-btn" id="operator-unbind-all-close-btn">
            <span class="material-symbols-outlined">close</span>
        </button>
    </div>

    <div class="operator-unbind-all-description">
        The following operators will be unbound from your current web session. You will no longer be able to interact with them until they are rebound.
    </div>

    <div class="operator-unbind-all-operators-container">
        <div class="operator-unbind-all-operators-list" id="operator-unbind-all-operators-list">
        </div>
    </div>

    <div class="operator-unbind-all-actions">
        <button class="operator-unbind-all-action-btn cancel-btn" id="operator-unbind-all-cancel-btn">
            <span class="material-symbols-outlined">cancel</span>
            <span>Cancel</span>
        </button>
        <button class="operator-unbind-all-action-btn confirm-btn" id="operator-unbind-all-confirm-btn">
            <span class="material-symbols-outlined">link_off</span>
            <span>Unbind All</span>
        </button>
    </div>

    <div class="operator-unbind-all-processing initially-hidden" id="operator-unbind-all-processing">
        <div class="spinner"></div>
        <span id="operator-unbind-all-processing-label">Unbinding operators...</span>
    </div>
    <div class="operator-unbind-all-actions-feedback" id="operator-unbind-all-feedback"></div>
</div>`,

    'confirmation-modal-base': `<div class="download-menu-overlay confirmation-modal-overlay">
    <div class="confirmation-modal" role="dialog" aria-modal="true" aria-labelledby="modal-title">
        <div class="confirmation-modal-header">
            <span class="material-symbols-outlined confirmation-modal-icon {{iconClass}}">{{icon}}</span>
            <h3 class="confirmation-modal-title" id="modal-title">{{title}}</h3>
        </div>
        <div class="confirmation-modal-body">
            <p class="confirmation-modal-message {{descriptionClass}}">{{message}}</p>
        </div>
        <div class="confirmation-modal-actions">
            <button class="confirmation-modal-btn confirmation-modal-cancel" data-action="cancel">Cancel</button>
            <button class="confirmation-modal-btn confirmation-modal-confirm {{confirmClass}}" data-action="confirm">
                <span class="material-symbols-outlined">{{confirmIcon}}</span>
                <span>{{confirmLabel}}</span>
            </button>
        </div>
    </div>
</div>`,
};

export function seedTemplates(templateLoader, names = null) {
    const keys = names ?? Object.keys(TEMPLATE_FIXTURES);
    for (const name of keys) {
        templateLoader.seed(name, TEMPLATE_FIXTURES[name]);
    }
}
