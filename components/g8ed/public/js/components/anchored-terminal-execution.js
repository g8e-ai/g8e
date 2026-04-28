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

import { EventType } from '../constants/events.js';
import { devLogger } from '../utils/dev-logger.js';
import { nowISOString, formatForDisplay } from '../utils/timestamp.js';
import { templateLoader } from '../utils/template-loader.js';
import { escapeHtml } from '../utils/html.js';
import { webSessionService } from '../utils/web-session-service.js';
import { ComponentName } from '../constants/service-client-constants.js';
import { ApiPaths } from '../constants/api-paths.js';

export class TerminalExecutionMixin {
    initExecutionState() {
        this.pendingApprovals = new Map();
        this.activeExecutions = new Map();
        this.approvalResultsContainers = new Map();
        this.executionResultsContainers = new Map();
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

    _createAIBubbleWrapper(bubbleId) {
        const group = document.createElement('div');
        group.className = 'anchored-terminal__agent-message-group anchored-terminal__agent-message-group--execution';
        if (bubbleId) group.setAttribute('data-execution-bubble', bubbleId);

        const header = this._createAgentMessageHeader();

        const content = document.createElement('div');
        content.className = 'anchored-terminal__agent-message-content';

        group.appendChild(header);
        group.appendChild(content);

        return group;
    }

    async showExecutingIndicator(command) {
        if (!this.outputContainer) return null;

        const id = `exec-${crypto.randomUUID()}`;

        const group = this._createAIBubbleWrapper(id);
        const content = group.querySelector('.anchored-terminal__agent-message-content');

        const indicator = document.createElement('div');
        indicator.className = 'anchored-terminal__executing';
        indicator.id = id;
        await templateLoader.renderTo(indicator, 'executing-indicator', { command });

        content.appendChild(indicator);
        this.outputContainer.appendChild(group);
        this.scrollToBottom();

        return id;
    }

    async showPreparingIndicator(command) {
        if (!this.outputContainer) return null;

        const id = `exec-${crypto.randomUUID()}`;

        const group = this._createAIBubbleWrapper(id);
        const content = group.querySelector('.anchored-terminal__agent-message-content');

        const indicator = document.createElement('div');
        indicator.className = 'anchored-terminal__executing';
        indicator.id = id;
        await templateLoader.renderTo(indicator, 'preparing-indicator', { command });

        content.appendChild(indicator);
        this.outputContainer.appendChild(group);
        this.scrollToBottom();

        return id;
    }

    async _showExecutingIndicatorInContainer(container, command) {
        if (!container || typeof container.querySelector !== 'function') return null;

        const id = `exec-${crypto.randomUUID()}`;
        const body = container.querySelector('.anchored-terminal__results-body');
        if (!body) return await this.showExecutingIndicator(command);

        const indicator = document.createElement('div');
        indicator.className = 'anchored-terminal__executing';
        indicator.id = id;
        await templateLoader.renderTo(indicator, 'executing-indicator', { command });

        body.appendChild(indicator);

        container.classList.remove('collapsed');

        const toggle = container.querySelector('.anchored-terminal__results-toggle');
        if (toggle) {
            toggle.style.display = '';
        }

        const labelEl = container.querySelector('.anchored-terminal__results-toggle-label');
        if (labelEl) {
            labelEl.textContent = 'Executing';
        }

        this.scrollToBottom();
        return id;
    }

    _findBubbleByExecId(execId) {
        if (!this.outputContainer || !execId) return null;
        return this.outputContainer.querySelector(
            `.anchored-terminal__agent-message-group[data-execution-bubble="${execId}"]`
        );
    }

    hideExecutingIndicator(id) {
        if (!this.outputContainer) return;

        if (id) {
            const indicator = document.getElementById(id);
            if (indicator) {
                const parentGroup = indicator.closest('.anchored-terminal__agent-message-group');
                indicator.remove();
                if (parentGroup) {
                    const content = parentGroup.querySelector('.anchored-terminal__agent-message-content');
                    const hasPersistentContent = content && (
                        content.querySelector('.anchored-terminal__approval') || 
                        content.querySelector('.anchored-terminal__results-group')
                    );

                    if (content && content.children.length === 0 && !hasPersistentContent) {
                        parentGroup.remove();
                    }
                }
                return;
            }
        }

        const indicators = this.outputContainer.querySelectorAll('.anchored-terminal__executing');
        indicators.forEach(el => el.remove());
    }

    async handleApprovalRequest(data) {
        if (!this.outputContainer || !data) return;

        const welcome = this.outputContainer.querySelector('.anchored-terminal__welcome');
        if (welcome) welcome.remove();

        const execId = data.execution_id;
        const approvalId = data.approval_id;
        if (!approvalId) {
            devLogger.error('[ANCHORED TERMINAL] Approval event missing approval_id — backend contract violation', data);
            throw new Error('Approval event must include approval_id');
        }
        const webSessionId = data.web_session_id;
        const correlationId = data.correlation_id;

        // If the Tribunal already rendered a refining approval-compact for this
        // session, upgrade that existing DOM node in place instead of rendering
        // a separate card. This preserves the tribunal dots/status inside the
        // approval-compact header.
        //
        // Discovery is done via dataset comparison rather than an attribute
        // selector with CSS.escape: arbitrary session-id strings may contain
        // characters that require escaping, and CSS.escape is not available in
        // every runtime the tests exercise.
        const unclaimedRefining = Array.from(this.outputContainer.querySelectorAll(
            '.anchored-terminal__approval[data-approval-refining="1"]:not([data-approval-id])'
        ));

        let refiningWidget = null;
        if (correlationId) {
            // Primary matching: use correlation_id from Tribunal session
            refiningWidget = unclaimedRefining.find(el => el.dataset.correlationId === correlationId) ?? null;
        } else if (webSessionId) {
            // Fallback: use web_session_id for non-Tribunal flows
            refiningWidget = unclaimedRefining.find(el => el.dataset.webSessionId === webSessionId) ?? null;
        } else {
            devLogger.error('[ANCHORED TERMINAL] Approval event missing both correlation_id and web_session_id — cannot safely claim refining widget, rendering separate card', data);
        }

        let group = null;
        if (refiningWidget) {
            group = refiningWidget.closest('.anchored-terminal__agent-message-group');
        }

        // Always clean up any pending preparing indicator for this execId.
        // Run this even when a refining widget already claimed the group:
        // the preparing indicator may live in a separate bubble (when PREPARING
        // fires after TRIBUNAL_SESSION_STARTED, or when correlation_id differs),
        // and OPERATOR_COMMAND_STARTED uses a per-operator exec_id which will
        // not match this approval_execution_id, so the indicator would otherwise
        // never be removed.
        if (execId) {
            const preparingExec = this.activeExecutions.get(execId);
            if (preparingExec && preparingExec.indicatorId) {
                const indicator = document.getElementById(preparingExec.indicatorId);
                const prepGroup = indicator?.closest('.anchored-terminal__agent-message-group') ?? null;
                if (indicator) indicator.remove();
                // If we don't yet have a group, reuse the preparing bubble.
                if (!group && prepGroup) {
                    group = prepGroup;
                } else if (prepGroup && prepGroup !== group) {
                    // Preparing lived in its own bubble; drop it if now empty.
                    const prepContent = prepGroup.querySelector('.anchored-terminal__agent-message-content');
                    if (prepContent && prepContent.children.length === 0) {
                        prepGroup.remove();
                    }
                }
                this.activeExecutions.delete(execId);
            }
        }

        if (!group) {
            // Prefer an existing execution bubble. Never reuse a streaming AI
            // response group (id="ai-response-<wsid>"): its innerHTML is
            // overwritten by subsequent text chunks, which would wipe the
            // approval card after the user approves.
            const lastExecutionGroup = this.outputContainer.querySelector('.anchored-terminal__agent-message-group--execution:last-of-type');
            if (lastExecutionGroup) {
                group = lastExecutionGroup;
            } else {
                group = this._createAIBubbleWrapper(approvalId);
                this.outputContainer.appendChild(group);
            }
        }
        group.setAttribute('data-execution-bubble', approvalId);

        const command = data.command;
        const justification = data.justification;
        const isFileEdit = data.file_path && data.operation;
        const isIntent = data.intent_name && data.intent_question;
        const isStream = data.kind === 'stream';
        const isAgentContinue = typeof data.turn_limit === 'number';
        const targetSystems = data.target_systems;
        const hosts = data.hosts;
        const isBatchExecution = (data.is_batch_execution && targetSystems && targetSystems.length > 1) || (isStream && hosts && hosts.length > 1);

        if (!data.case_id || !data.investigation_id || !data.task_id) {
            devLogger.error('[ANCHORED TERMINAL] Approval data missing required fields (case_id, investigation_id, task_id)', data);
            throw new Error('Approval data must include case_id, investigation_id, and task_id');
        }

        this.pendingApprovals.set(approvalId, data);

        // Reuse the refining widget if present, otherwise create a fresh one.
        const approval = refiningWidget ?? document.createElement('div');
        if (!refiningWidget) {
            approval.className = 'anchored-terminal__approval';
        }
        approval.setAttribute('data-approval-id', approvalId);
        approval.removeAttribute('data-approval-refining');
        approval.id = approvalId;

        // Preserve tribunal passes + status (final state) when upgrading a refining widget.
        let tribunalHtml = '';
        if (refiningWidget) {
            const tribunalEl = refiningWidget.querySelector('.tribunal');
            if (tribunalEl) {
                tribunalHtml = tribunalEl.outerHTML;
            } else {
                // Fallback for older structure if needed
                const passesEl = refiningWidget.querySelector('.tribunal__passes');
                const gapEl = refiningWidget.querySelector('.tribunal__gap');
                const auditorEl = refiningWidget.querySelector('.tribunal__dot--auditor');
                const statusEl = refiningWidget.querySelector('.tribunal__status');
                tribunalHtml = (passesEl ? passesEl.outerHTML : '') + 
                              (gapEl ? gapEl.outerHTML : '') +
                              (auditorEl ? auditorEl.outerHTML : '') +
                              (statusEl ? statusEl.outerHTML : '');
                
                if (tribunalHtml) {
                    tribunalHtml = `<div class="tribunal">${tribunalHtml}</div>`;
                }
            }
        }

        let headerText = 'Command';
        let commandDisplay = command;
        let cardModifier = '';

        const riskLevel = data.risk_analysis?.risk_level?.toUpperCase() || 'LOW';
        let icon = 'check_circle';
        let iconModifier = 'approval-compact__icon--low';
        if (riskLevel === 'HIGH') {
            icon = 'warning';
            iconModifier = 'approval-compact__icon--high';
        } else if (riskLevel === 'MEDIUM') {
            icon = 'priority_high';
            iconModifier = 'approval-compact__icon--medium';
        }

        if (isAgentContinue) {
            headerText = 'Agent Turn Limit';
            commandDisplay = `Agent reached ${data.turn_limit} tool-use turns. Continue?`;
            cardModifier = 'approval-compact--agent-continue';
            icon = 'hourglass_empty';
            iconModifier = '';
        } else if (isFileEdit) {
            headerText = 'File Edit';
            commandDisplay = `${data.operation}: ${data.file_path}`;
            cardModifier = 'approval-compact--file';
            icon = 'edit_document';
            iconModifier = '';
        } else if (isIntent) {
            headerText = 'Escalation';
            commandDisplay = data.intent_question;
            cardModifier = 'approval-compact--intent';
            icon = 'shield';
            iconModifier = '';
        } else if (isStream) {
            headerText = isBatchExecution ? `Stream (${hosts.length} hosts)` : 'Stream';
            commandDisplay = data.preview_command;
            cardModifier = 'approval-compact--stream';
            icon = 'ship';
            iconModifier = '';
        } else if (isBatchExecution) {
            headerText = `Command (${targetSystems.length} systems)`;
        }

        const systemsHtml = isBatchExecution ? (isStream ? this._buildHostsHtml(hosts) : this._buildTargetSystemsHtml(targetSystems)) : '';
        const approveButtonText = isAgentContinue
            ? 'Continue'
            : (isBatchExecution ? `Approve for ${isStream ? hosts.length : targetSystems.length} ${isStream ? 'Hosts' : 'Systems'}` : 'Approve');

        const riskBadgeHtml = (!isFileEdit && !isIntent && !isAgentContinue && !isStream)
            ? this._buildRiskBadgeHtml(data.risk_analysis)
            : '';

        await templateLoader.renderTo(approval, 'approval-card', {
            cardModifier,
            icon,
            iconModifier,
            headerText,
            tribunalHtml,
            riskBadgeHtml,
            promptHtml: (isFileEdit || isAgentContinue || isStream) ? '' : '<span class="approval-compact__prompt">$</span>',
            commandDisplay,
            systemsHtml,
            justification: justification || 'No justification provided',
            approvalId,
            approveButtonText
        });

        const approveBtn = approval.querySelector('.approval-compact__btn--approve');
        const denyBtn = approval.querySelector('.approval-compact__btn--deny');

        approveBtn.addEventListener('click', () => this.handleApprovalResponse(approvalId, true));
        denyBtn.addEventListener('click', () => this.handleApprovalResponse(approvalId, false));

        if (!refiningWidget) {
            const contentEl = group.querySelector('.anchored-terminal__agent-message-content');
            contentEl.appendChild(approval);
        }
        this.scrollToBottom();
    }

    async handleApprovalResponse(approvalId, approved) {
        const approvalData = this.pendingApprovals.get(approvalId);
        if (!approvalData) return;

        const approvalEl = this.outputContainer?.querySelector(`.anchored-terminal__approval[data-approval-id="${approvalId}"]`);
        if (approvalEl) {
            const buttons = approvalEl.querySelectorAll('.approval-compact__btn');
            buttons.forEach(btn => btn.disabled = true);
        }

        try {
            const webSessionId = webSessionService.getWebSessionId();
            if (!webSessionId) {
                throw new Error('No active session');
            }

            await window.serviceClient.post(ComponentName.G8ED, ApiPaths.approval.respond(), {
                approval_id: approvalId,
                approved: approved,
                reason: approved ? 'User approved via terminal' : 'User denied via terminal',
                case_id: approvalData.case_id,
                investigation_id: approvalData.investigation_id,
                task_id: approvalData.task_id,
            });

            const isAgentContinue = typeof approvalData.turn_limit === 'number';
            const isStream = approvalData.kind === 'stream';

            if (approvalEl) {
                const actionsDiv = approvalEl.querySelector('.approval-compact__actions');
                if (actionsDiv) {
                    await templateLoader.renderTo(actionsDiv, 'approval-status', {
                        statusClass: approved ? 'approved' : 'denied',
                        statusIcon: approved ? 'check' : 'close',
                        statusText: approved
                            ? (isAgentContinue ? 'Continuing' : 'Approved')
                            : (isAgentContinue ? 'Stopped' : 'Denied')
                    });
                }

                if (approved && !isAgentContinue) {
                    const resultsContainer = await this._createResultsContainer(approvalId, approvalEl, true);
                    this._pendingExecutingIndicator = await this._showExecutingIndicatorInContainer(
                        resultsContainer,
                        isStream ? approvalData.preview_command : approvalData.command
                    );
                }
            }

            this.pendingApprovals.delete(approvalId);

        } catch (error) {
            console.error('[ANCHORED TERMINAL] Approval response failed:', error);

            if (approvalEl) {
                const buttons = approvalEl.querySelectorAll('.approval-compact__btn');
                buttons.forEach(btn => btn.disabled = false);
            }
        }
    }

    _buildRiskBadgeHtml(riskAnalysis) {
        if (!riskAnalysis || !riskAnalysis.risk_level) return '';

        const riskLevel = (riskAnalysis.risk_level || 'UNKNOWN').toUpperCase();
        const riskScore = riskAnalysis.risk_score;
        const isDestructive = riskAnalysis.is_destructive;
        const blastRadius = riskAnalysis.blast_radius;

        let icon = 'info';
        if (riskLevel === 'HIGH') {
            icon = 'warning';
        } else if (riskLevel === 'MEDIUM') {
            icon = 'priority_high';
        } else if (riskLevel === 'LOW') {
            icon = 'check_circle';
        }

        const tooltipParts = [];
        if (riskScore !== undefined) {
            tooltipParts.push(`Score: ${riskScore}/10`);
        }
        if (isDestructive) {
            tooltipParts.push('Destructive operation');
        }
        if (blastRadius) {
            tooltipParts.push(`Blast radius: ${blastRadius}`);
        }
        const tooltip = tooltipParts.length > 0 ? escapeHtml(tooltipParts.join(' | ')) : '';

        return `
            <span class="operator-terminal__risk-badge operator-terminal__risk-badge--${riskLevel.toLowerCase()}" title="${tooltip}">
                <span class="material-symbols-outlined icon-14">${icon}</span>
                <span class="operator-terminal__risk-level">${riskLevel}</span>
            </span>
        `;
    }

    _buildTargetSystemsHtml(targetSystems) {
        if (!targetSystems || targetSystems.length === 0) return '';

        const systemItems = targetSystems.map(sys => {
            const hostname = escapeHtml(sys.hostname || 'unknown');
            const opType = sys.operator_type === 'cloud' ? 'cloud' : 'system';
            const icon = opType === 'cloud' ? 'cloud' : 'computer';

            return `
                <div class="operator-terminal__target-system">
                    <span class="material-symbols-outlined icon-16">${icon}</span>
                    <span class="operator-terminal__target-hostname">${hostname}</span>
                    <span class="operator-terminal__target-type">${opType}</span>
                </div>
            `;
        }).join('');

        return `
            <div class="operator-terminal__target-systems">
                <div class="operator-terminal__target-systems-header">
                    <span class="material-symbols-outlined icon-16">devices</span>
                    Impacted Systems (${targetSystems.length})
                </div>
                <div class="operator-terminal__target-systems-list">
                    ${systemItems}
                </div>
            </div>
        `;
    }
    
    _buildHostsHtml(hosts) {
        if (!hosts || hosts.length === 0) return '';

        const hostItems = hosts.map(host => {
            const hostname = escapeHtml(host || 'unknown');
            return `
                <div class="operator-terminal__target-system">
                    <span class="material-symbols-outlined icon-16">computer</span>
                    <span class="operator-terminal__target-hostname">${hostname}</span>
                    <span class="operator-terminal__target-type">ssh</span>
                </div>
            `;
        }).join('');

        return `
            <div class="operator-terminal__target-systems">
                <div class="operator-terminal__target-systems-header">
                    <span class="material-symbols-outlined icon-16">dns</span>
                    Target Hosts (${hosts.length})
                </div>
                <div class="operator-terminal__target-systems-list">
                    ${hostItems}
                </div>
            </div>
        `;
    }

    async _createResultsContainer(containerId, approvalEl, isApproval = false) {
        if (!this.outputContainer) return null;

        const containersMap = isApproval ? this.approvalResultsContainers : this.executionResultsContainers;

        if (containersMap.has(containerId)) {
            return containersMap.get(containerId);
        }

        const container = document.createElement('div');
        container.className = 'anchored-terminal__results-group';
        container.setAttribute('data-execution-id', containerId);

        const toggle = document.createElement('div');
        toggle.className = 'anchored-terminal__results-toggle';
        toggle.style.display = 'none';
        await templateLoader.renderTo(toggle, 'results-toggle', {});

        toggle.addEventListener('click', () => {
            container.classList.toggle('collapsed');
        });

        const body = document.createElement('div');
        body.className = 'anchored-terminal__results-body';

        container.appendChild(toggle);
        container.appendChild(body);

        const parentGroup = approvalEl?.closest('.anchored-terminal__agent-message-group');
        if (parentGroup) {
            const contentEl = parentGroup.querySelector('.anchored-terminal__agent-message-content');
            if (contentEl) {
                contentEl.appendChild(container);
            } else {
                parentGroup.appendChild(container);
            }
        } else {
            this.outputContainer.appendChild(container);
        }

        containersMap.set(containerId, container);
        return container;
    }

    async _appendResultToContainer(container, resultData) {
        if (!container || typeof container.querySelector !== 'function') return;

        const body = container.querySelector('.anchored-terminal__results-body');
        if (!body) return;

        const { command, stdout, stderr, exitCode, status, timestamp, operatorId, hostname } = resultData;

        const isSuccess = status === EventType.OPERATOR_COMMAND_COMPLETED || status === EventType.OPERATOR_FILE_EDIT_COMPLETED || status === 'success';
        const statusClass = isSuccess ? 'success' : 'error';
        const statusIcon = isSuccess ? 'check_circle' : 'error';

        const displayTime = timestamp
            ? formatForDisplay(timestamp)
            : this.formatTimestamp();

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__result-entry';

        let outputContent = '';
        if (stdout) {
            outputContent += stdout;
        }
        if (stderr) {
            outputContent += (outputContent ? '\n' : '') + stderr;
        }
        if (!outputContent) {
            outputContent = '(No output)';
        }

        const hostnameHtml = hostname
            ? `<span class="anchored-terminal__result-hostname"><span class="material-symbols-outlined icon-12">computer</span>${escapeHtml(hostname)}</span>`
            : '';

        await templateLoader.renderTo(entry, 'command-result', {
            statusClass,
            statusIcon,
            hostnameHtml,
            command: escapeHtml(command),
            displayTime,
            outputContent: escapeHtml(outputContent),
            exitCodeHtml: exitCode !== undefined
                ? `<div class="anchored-terminal__result-exit anchored-terminal__result-exit--${exitCode === 0 ? 'success' : 'error'}">Exit code: ${exitCode}</div>`
                : '',
        });

        body.appendChild(entry);

        const countEl = container.querySelector('.anchored-terminal__results-count');
        const currentCount = body.querySelectorAll('.anchored-terminal__result-entry').length;
        if (countEl) {
            countEl.textContent = currentCount;
        }

        const toggle = container.querySelector('.anchored-terminal__results-toggle');
        if (toggle) {
            toggle.style.display = '';
        }

        const labelEl = container.querySelector('.anchored-terminal__results-toggle-label');
        if (labelEl) {
            labelEl.textContent = currentCount === 1 ? 'Result' : 'Results';
        }
    }

    async handleCommandExecutionEvent(data) {
        if (!data || !this.outputContainer) return;

        const eventType = data.eventType;
        const command = data.command || data.cmd;
        const executionResult = data.execution_result || {};
        const execId = data.execution_id;
        const approvalId = data.approval_id;

        const isFinal = eventType === EventType.OPERATOR_COMMAND_COMPLETED
            || eventType === EventType.OPERATOR_COMMAND_FAILED
            || eventType === EventType.OPERATOR_COMMAND_CANCELLED
            || eventType === EventType.OPERATOR_COMMAND_APPROVAL_GRANTED
            || eventType === EventType.OPERATOR_COMMAND_APPROVAL_REJECTED
            || eventType === EventType.OPERATOR_FILE_EDIT_COMPLETED
            || eventType === EventType.OPERATOR_FILE_EDIT_FAILED
            || eventType === EventType.OPERATOR_FILE_EDIT_APPROVAL_GRANTED
            || eventType === EventType.OPERATOR_FILE_EDIT_APPROVAL_REJECTED;

        if (eventType === EventType.OPERATOR_COMMAND_APPROVAL_PREPARING) {
            const perOperatorExecIds = data.per_operator_execution_ids || [];
            
            if (execId && !this.activeExecutions.has(execId)) {
                const indicatorId = await this.showPreparingIndicator(command);
                this.activeExecutions.set(execId, { command, startedAt: Date.now(), indicatorId });
                
                // For batch operations, also map each per-operator execution ID to the same indicator
                // so STARTED events can find and clean it up
                for (const perOpExecId of perOperatorExecIds) {
                    if (!this.activeExecutions.has(perOpExecId)) {
                        this.activeExecutions.set(perOpExecId, { command, startedAt: Date.now(), indicatorId });
                    }
                }
            }
        } else if (eventType === EventType.OPERATOR_COMMAND_STARTED) {
            if (execId) {
                const existing = this.activeExecutions.get(execId);
                if (existing) {
                    this.hideExecutingIndicator(existing.indicatorId);
                }

                const existingContainer =
                    (approvalId ? this.approvalResultsContainers.get(approvalId) : null) ||
                    (execId ? this.executionResultsContainers.get(execId) : null);

                if (existingContainer) {
                    const body = existingContainer.querySelector('.anchored-terminal__results-body');
                    if (body) {
                        body.querySelectorAll('.anchored-terminal__executing').forEach(el => el.remove());
                    }
                    const indicatorId = await this._showExecutingIndicatorInContainer(existingContainer, command);
                    this.activeExecutions.set(execId, { command, startedAt: Date.now(), indicatorId, inContainer: true });
                } else {
                    const indicatorId = await this.showExecutingIndicator(command);
                    this.activeExecutions.set(execId, { command, startedAt: Date.now(), indicatorId });
                }
            }
        } else if (isFinal) {
            const stdout = data.output || executionResult.output || executionResult.stdout;
            const stderr = data.error || executionResult.error || executionResult.stderr;
            const exitCode = data.return_code ?? data.exit_code ?? executionResult.exit_code;

            const execInfo = execId ? this.activeExecutions.get(execId) : null;
            this.hideExecutingIndicator(execInfo?.indicatorId);

            let resultsContainer = null;
            if (approvalId) {
                resultsContainer = this.approvalResultsContainers.get(approvalId);
            }

            if (!resultsContainer) {
                const containerId = approvalId;
                if (!containerId) {
                    console.error('[TERMINAL] Final command event missing approval_id — backend contract violation', data);
                    return;
                }
                resultsContainer = await this._createResultsContainer(containerId, null, true);
            }

            if (resultsContainer) {
                await this._appendResultToContainer(resultsContainer, {
                    command,
                    stdout,
                    stderr,
                    exitCode,
                    status: eventType,
                    timestamp: data.timestamp,
                    operatorId: data.operator_id,
                    hostname: data.hostname
                });
            }

            if (execId) {
                this.activeExecutions.delete(execId);
            }
        }

        this.scrollToBottom();
    }

    async handleIntentResult(data) {
        if (!data || !this.outputContainer) return;

        const intentName = data.intent_name || 'permission';
        const granted = data.granted || data.eventType === EventType.OPERATOR_INTENT_GRANTED || data.eventType === EventType.OPERATOR_INTENT_APPROVAL_GRANTED;
        const status = granted ? EventType.OPERATOR_COMMAND_COMPLETED : EventType.OPERATOR_COMMAND_FAILED;

        const containerId = data.approval_id;
        if (!containerId) {
            console.error('[TERMINAL] Intent result missing approval_id — backend contract violation', data);
            return;
        }
        const container = await this._createResultsContainer(containerId, null, true);
        if (container) {
            await this._appendResultToContainer(container, {
                command: `Permission: ${intentName}`,
                stdout: granted ? 'Permission granted' : 'Permission denied',
                stderr: '',
                exitCode: granted ? 0 : 1,
                status,
                timestamp: data.timestamp || nowISOString()
            });
        }
    }

    async restoreCommandExecution(data) {
        if (!this.outputContainer || !data) return;

        const welcome = this.outputContainer.querySelector('.anchored-terminal__welcome');
        if (welcome) welcome.remove();

        const command = data.command;
        const content = data.content;
        const status = data.status || 'completed';
        const exitCode = data.exit_code;
        const timestamp = data.timestamp;

        const output = this._extractOutputFromContent(content, command);

        const containerId = data.execution_id;
        if (!containerId) {
            console.error('[TERMINAL] restoreCommandExecution called with no execution_id — cannot restore result', data);
            return;
        }
        const container = await this._createResultsContainer(containerId, null, false);
        if (container) {
            await this._appendResultToContainer(container, {
                command,
                stdout: output !== '(No output)' ? output : '',
                stderr: '',
                exitCode,
                status,
                timestamp,
                hostname: data.hostname
            });
        }
    }

    _extractOutputFromContent(content, command) {
        if (!content) return '(No output)';

        const outputMatch = content.match(/(?:Output|Error):\n([\s\S]*)/);
        if (outputMatch) {
            return outputMatch[1].trim() || '(No output)';
        }

        const commandLine = `Command: ${command}`;
        if (content.startsWith(commandLine)) {
            return content.substring(commandLine.length).trim() || '(No output)';
        }

        return content;
    }

    async restoreApprovalRequest(data, wasApproved, executionId = null) {
        if (!this.outputContainer || !data) return null;

        const welcome = this.outputContainer.querySelector('.anchored-terminal__welcome');
        if (welcome) welcome.remove();

        const command = data.command;
        const justification = data.justification;
        const isFileEdit = data.file_path && data.operation;
        const isIntent = data.intent_name && data.intent_question;
        const timestamp = data.timestamp;

        let headerText = 'Command';
        let commandDisplay = command;
        let icon = 'terminal';

        if (isFileEdit) {
            headerText = 'File Edit';
            commandDisplay = `${data.operation}: ${data.file_path}`;
            icon = 'edit_document';
        } else if (isIntent) {
            headerText = 'Escalation';
            commandDisplay = data.intent_question;
            icon = 'shield';
        }

        const displayTime = timestamp
            ? formatForDisplay(timestamp)
            : '';

        const statusClass = wasApproved ? 'approved' : 'denied';
        const statusIcon = wasApproved ? 'check' : 'close';
        const statusText = wasApproved ? 'Approved' : 'Denied';

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__approval restored';
        const approvalId = data.approval_id;
        if (approvalId) {
            entry.setAttribute('data-approval-id', approvalId);
        }

        await templateLoader.renderTo(entry, 'approval-card-restored', {
            icon,
            headerText,
            timeHtml: displayTime ? `<span class="approval-compact__time">${displayTime}</span>` : '',
            promptHtml: isFileEdit ? '' : '<span class="approval-compact__prompt">$</span>',
            commandDisplay,
            justification,
            statusClass,
            statusIcon,
            statusText,
            timeHtml_raw: displayTime ? `<span class="approval-compact__time">${displayTime}</span>` : ''
        });

        const group = this._createAIBubbleWrapper(executionId);
        const contentEl = group.querySelector('.anchored-terminal__agent-message-content');
        contentEl.appendChild(entry);
        this.outputContainer.appendChild(group);

        if (wasApproved && executionId) {
            await this._createResultsContainer(executionId, entry, false);
        }

        return entry;
    }

    async denyAllPendingApprovals(reason, statusMessage = 'Cancelled') {
        if (!this.pendingApprovals?.size) return;

        const webSessionId = webSessionService.getWebSessionId();
        let totalDenied = 0;
        const denyPromises = [];

        for (const [approvalId, approvalData] of this.pendingApprovals) {
            if (webSessionId) {
                denyPromises.push(
                    window.serviceClient?.post(ComponentName.G8ED, ApiPaths.approval.respond(), {
                        approval_id: approvalId,
                        approved: false,
                        reason: reason,
                        case_id: approvalData.case_id,
                        investigation_id: approvalData.investigation_id,
                        task_id: approvalData.task_id,
                    }).catch(error => {
                        console.error(`[TERMINAL] Failed to deny approval ${approvalId}:`, error);
                        return { approvalId, failed: true };
                    })
                );
            }

            const approvalEl = this.outputContainer?.querySelector(`[data-approval-id="${approvalId}"]`);
            if (approvalEl) {
                const actionsDiv = approvalEl.querySelector('.approval-compact__actions');
                if (actionsDiv) {
                    await templateLoader.renderTo(actionsDiv, 'approval-status', {
                        statusClass: 'denied',
                        statusIcon: 'close',
                        statusText: statusMessage
                    });
                }
            }
            totalDenied++;
        }

        const results = await Promise.all(denyPromises);
        const failures = results.filter(r => r?.failed);
        if (failures.length > 0) {
            console.error(`[TERMINAL] Failed to deny ${failures.length} approval(s)`);
        }

        this.pendingApprovals.clear();

        if (totalDenied > 0) {
            console.log(`[TERMINAL] Denied ${totalDenied} pending approval(s) - ${reason}`);
        }
    }

    async restoreCommandResult(executionId, data) {
        if (!data) return;

        const container = executionId ? this.executionResultsContainers.get(executionId) : null;

        if (container) {
            const command = data.command;
            const content = data.content;
            const output = this._extractOutputFromContent(content, command);

            await this._appendResultToContainer(container, {
                command,
                stdout: output !== '(No output)' ? output : '',
                stderr: '',
                exitCode: data.exit_code,
                status: data.status || 'completed',
                timestamp: data.timestamp,
                operatorId: data.operator_id,
                hostname: data.hostname
            });
        } else {
            await this.restoreCommandExecution(data);
        }
    }
}
