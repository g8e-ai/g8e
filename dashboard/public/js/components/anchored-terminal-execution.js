// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { EventType } from '../constants/events.js';
import { nowISOString } from '../utils/timestamp.js';
import { templateLoader } from '../utils/template-loader.js';
import { webSessionService } from '../utils/web-session-service.js';
import { ServiceName } from '../constants/service-client-constants.js';
import { ApiPaths } from '../constants/api-paths.js';

export class TerminalExecutionMixin {
    initExecutionState() {
        this.pendingApprovals = new Map();
        this.activeExecutions = new Map();
        this.executionResultsContainers = new Map();
    }

    showExecutingIndicator(command) {
        if (!this.outputContainer) return null;

        if (this._execCounter === undefined) this._execCounter = 0;
        const id = `exec-${Date.now()}-${++this._execCounter}`;

        const execTemplate = templateLoader.cache.get('executing-indicator');
        const indicator = document.createElement('div');
        indicator.className = 'anchored-terminal__executing';
        indicator.id = id;
        indicator.innerHTML = templateLoader.replace(execTemplate, { command: this.escapeHtml(command) });

        this.outputContainer.appendChild(indicator);
        this.scrollToBottom();

        return id;
    }

    showPreparingIndicator(command) {
        if (!this.outputContainer) return null;

        if (this._execCounter === undefined) this._execCounter = 0;
        const id = `exec-${Date.now()}-${++this._execCounter}`;

        const prepTemplate = templateLoader.cache.get('preparing-indicator');
        const indicator = document.createElement('div');
        indicator.className = 'anchored-terminal__executing';
        indicator.id = id;
        indicator.innerHTML = templateLoader.replace(prepTemplate, { command: this.escapeHtml(command) });

        this.outputContainer.appendChild(indicator);
        this.scrollToBottom();

        return id;
    }

    _showExecutingIndicatorInContainer(container, command) {
        if (!container) return null;

        const id = `exec-${Date.now()}`;
        const body = container.querySelector('.anchored-terminal__results-body');
        if (!body) return this.showExecutingIndicator(command);

        const execTemplate = templateLoader.cache.get('executing-indicator');
        const indicator = document.createElement('div');
        indicator.className = 'anchored-terminal__executing';
        indicator.id = id;
        indicator.innerHTML = templateLoader.replace(execTemplate, { command: this.escapeHtml(command) });

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

    hideExecutingIndicator(id) {
        if (!this.outputContainer) return;

        if (id) {
            const indicator = document.getElementById(id);
            if (indicator) {
                indicator.remove();
                return;
            }
        }

        const indicators = this.outputContainer.querySelectorAll('.anchored-terminal__executing');
        indicators.forEach(el => el.remove());
    }

    handleApprovalRequest(data) {
        if (!this.outputContainer || !data) return;

        const welcome = this.outputContainer.querySelector('.anchored-terminal__welcome');
        if (welcome) welcome.remove();

        // Hide the "Preparing" indicator if it exists for this execution
        const execId = data.execution_id;
        if (execId) {
            const preparingExec = this.activeExecutions.get(execId);
            if (preparingExec && preparingExec.indicatorId) {
                this.hideExecutingIndicator(preparingExec.indicatorId);
                this.activeExecutions.delete(execId);
            }
        }

        const approvalId = data.approval_id || data.execution_id;
        const command = data.command;
        const justification = data.justification;
        const isFileEdit = data.file_path && data.operation;
        const isIntent = data.intent_name && data.intent_question;
        const targetSystems = data.target_systems;
        const isBatchExecution = data.is_batch_execution && targetSystems && targetSystems.length > 1;

        this.pendingApprovals.set(approvalId, data);

        const approval = document.createElement('div');
        approval.className = 'anchored-terminal__approval';
        approval.dataset.approvalId = approvalId;

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

        if (isFileEdit) {
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
        } else if (isBatchExecution) {
            headerText = `Command (${targetSystems.length} systems)`;
        }

        const systemsHtml = isBatchExecution ? this._buildTargetSystemsHtml(targetSystems) : '';
        const approveButtonText = isBatchExecution ? `Approve for ${targetSystems.length} Systems` : 'Approve';

        const riskBadgeHtml = (!isFileEdit && !isIntent) ? this._buildRiskBadgeHtml(data.risk_analysis) : '';

        const cardTemplate = templateLoader.cache.get('approval-card');
        approval.innerHTML = templateLoader.replace(cardTemplate, {
            cardModifier,
            icon,
            iconModifier,
            headerText,
            riskBadgeHtml,
            promptHtml: isFileEdit ? '' : '<span class="approval-compact__prompt">$</span>',
            commandDisplay: this.escapeHtml(commandDisplay),
            systemsHtml,
            justification: this.escapeHtml(justification || 'No justification provided'),
            approvalId,
            approveButtonText
        });

        const approveBtn = approval.querySelector('.approval-compact__btn--approve');
        const denyBtn = approval.querySelector('.approval-compact__btn--deny');

        approveBtn.addEventListener('click', () => this.handleApprovalResponse(approvalId, true));
        denyBtn.addEventListener('click', () => this.handleApprovalResponse(approvalId, false));

        this.outputContainer.appendChild(approval);
        this.scrollToBottom();
    }

    async handleApprovalResponse(approvalId, approved) {
        const approvalData = this.pendingApprovals.get(approvalId);
        if (!approvalData) return;

        const approvalEl = this.outputContainer?.querySelector(`[data-approval-id="${approvalId}"]`);
        if (approvalEl) {
            const buttons = approvalEl.querySelectorAll('.approval-compact__btn');
            buttons.forEach(btn => btn.disabled = true);
        }

        try {
            const webSessionId = webSessionService.getWebSessionId();
            if (!webSessionId) {
                throw new Error('No active session');
            }

            await window.serviceClient.post(ServiceName.g8ed, ApiPaths.approval.respond(), {
                approval_id: approvalId,
                approved: approved,
                reason: approved ? 'User approved via terminal' : 'User denied via terminal',
                case_id: approvalData.case_id,
                investigation_id: approvalData.investigation_id,
                task_id: approvalData.task_id,
            });

            if (approvalEl) {
                const actionsDiv = approvalEl.querySelector('.approval-compact__actions');
                if (actionsDiv) {
                    const statusTemplate = templateLoader.cache.get('approval-status');
                    actionsDiv.innerHTML = templateLoader.replace(statusTemplate, {
                        statusClass: approved ? 'approved' : 'denied',
                        statusIcon: approved ? 'check' : 'close',
                        statusText: approved ? 'Approved' : 'Denied'
                    });
                }

                if (approved) {
                    const resultsContainer = this._createResultsContainer(approvalId, approvalEl);
                    this._pendingExecutingIndicator = this._showExecutingIndicatorInContainer(
                        resultsContainer,
                        approvalData.command
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
        const tooltip = tooltipParts.length > 0 ? this.escapeHtml(tooltipParts.join(' | ')) : '';

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
            const hostname = this.escapeHtml(sys.hostname || 'unknown');
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

    _createResultsContainer(executionId, approvalEl) {
        if (!this.outputContainer) return null;

        if (this.executionResultsContainers.has(executionId)) {
            return this.executionResultsContainers.get(executionId);
        }

        const container = document.createElement('div');
        container.className = 'anchored-terminal__results-group';
        container.dataset.executionId = executionId;

        const toggleTemplate = templateLoader.cache.get('results-toggle');
        const toggle = document.createElement('div');
        toggle.className = 'anchored-terminal__results-toggle';
        toggle.style.display = 'none';
        toggle.innerHTML = templateLoader.replace(toggleTemplate, {});

        toggle.addEventListener('click', () => {
            container.classList.toggle('collapsed');
        });

        const body = document.createElement('div');
        body.className = 'anchored-terminal__results-body';

        container.appendChild(toggle);
        container.appendChild(body);

        if (approvalEl) {
            if (approvalEl.nextSibling) {
                this.outputContainer.insertBefore(container, approvalEl.nextSibling);
            } else {
                this.outputContainer.appendChild(container);
            }
        } else {
            this.outputContainer.appendChild(container);
        }

        this.executionResultsContainers.set(executionId, container);
        return container;
    }

    _appendResultToContainer(container, resultData) {
        if (!container) return;

        const body = container.querySelector('.anchored-terminal__results-body');
        if (!body) return;

        const { command, stdout, stderr, exitCode, status, timestamp, hostname } = resultData;

        const isSuccess = status === EventType.OPERATOR_COMMAND_COMPLETED || status === EventType.OPERATOR_FILE_EDIT_COMPLETED || status === 'success';
        const statusClass = isSuccess ? 'success' : 'error';
        const statusIcon = isSuccess ? 'check_circle' : 'error';

        const displayTime = timestamp
            ? new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
            : this.formatTimestamp();

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__result-entry';

        let outputContent = '';
        if (stdout) {
            outputContent += this.escapeHtml(stdout);
        }
        if (stderr) {
            outputContent += (outputContent ? '\n' : '') + this.escapeHtml(stderr);
        }
        if (!outputContent) {
            outputContent = '(No output)';
        }

        const hostnameHtml = hostname
            ? `<span class="anchored-terminal__result-hostname"><span class="material-symbols-outlined icon-12">computer</span>${this.escapeHtml(hostname)}</span>`
            : '';

        const resultTemplate = templateLoader.cache.get('command-result');
        entry.innerHTML = templateLoader.replace(resultTemplate, {
            statusClass,
            statusIcon,
            hostnameHtml,
            command: this.escapeHtml(command),
            displayTime,
            outputContent,
            exitCodeHtml: exitCode !== undefined
                ? `<div class="anchored-terminal__result-exit anchored-terminal__result-exit--${exitCode === 0 ? 'success' : 'error'}">Exit code: ${exitCode}</div>`
                : ''
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

    handleCommandExecutionEvent(data) {
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
            if (execId && !this.activeExecutions.has(execId)) {
                const indicatorId = this.showPreparingIndicator(command);
                this.activeExecutions.set(execId, { command, startedAt: Date.now(), indicatorId });
            }
        } else if (eventType === EventType.OPERATOR_COMMAND_STARTED) {
            if (execId) {
                const existing = this.activeExecutions.get(execId);
                if (existing) {
                    this.hideExecutingIndicator(existing.indicatorId);
                }

                const existingContainer =
                    (approvalId ? this.executionResultsContainers.get(approvalId) : null) ||
                    (execId ? this.executionResultsContainers.get(execId) : null);

                if (existingContainer) {
                    const body = existingContainer.querySelector('.anchored-terminal__results-body');
                    if (body) {
                        body.querySelectorAll('.anchored-terminal__executing').forEach(el => el.remove());
                    }
                    const indicatorId = this._showExecutingIndicatorInContainer(existingContainer, command);
                    this.activeExecutions.set(execId, { command, startedAt: Date.now(), indicatorId, inContainer: true });
                } else {
                    const indicatorId = this.showExecutingIndicator(command);
                    this.activeExecutions.set(execId, { command, startedAt: Date.now(), indicatorId });
                }
            }
        } else if (isFinal) {
            const stdout = data.output || executionResult.output || executionResult.stdout;
            const stderr = data.error || executionResult.error || executionResult.stderr;
            const exitCode = data.return_code ?? data.exit_code ?? executionResult.exit_code;

            const execInfo = execId ? this.activeExecutions.get(execId) : null;
            this.hideExecutingIndicator(execInfo?.indicatorId);

            let resultsContainer =
                (approvalId ? this.executionResultsContainers.get(approvalId) : null) ||
                (execId ? this.executionResultsContainers.get(execId) : null);

            if (!resultsContainer) {
                const containerId = approvalId || execId;
                if (!containerId) {
                    console.error('[TERMINAL] Received final command event with no execution_id or approval_id — cannot render result', data);
                    return;
                }
                resultsContainer = this._createResultsContainer(containerId);
            }

            if (resultsContainer) {
                this._appendResultToContainer(resultsContainer, {
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

    handleIntentResult(data) {
        if (!data || !this.outputContainer) return;

        const intentName = data.intent_name || 'permission';
        const granted = data.granted || data.eventType === EventType.OPERATOR_INTENT_GRANTED || data.eventType === EventType.OPERATOR_INTENT_APPROVAL_GRANTED;
        const status = granted ? EventType.OPERATOR_COMMAND_COMPLETED : EventType.OPERATOR_COMMAND_FAILED;

        const containerId = data.approval_id || data.execution_id;
        if (!containerId) {
            console.error('[TERMINAL] Received intent result with no approval_id or execution_id — cannot render result', data);
            return;
        }
        const container = this._createResultsContainer(containerId);
        if (container) {
            this._appendResultToContainer(container, {
                command: `Permission: ${intentName}`,
                stdout: granted ? 'Permission granted' : 'Permission denied',
                stderr: '',
                exitCode: granted ? 0 : 1,
                status,
                timestamp: data.timestamp || nowISOString()
            });
        }
    }

    restoreCommandExecution(data) {
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
        const container = this._createResultsContainer(containerId);
        if (container) {
            this._appendResultToContainer(container, {
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

    restoreApprovalRequest(data, wasApproved, executionId = null) {
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
            ? new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
            : '';

        const statusClass = wasApproved ? 'approved' : 'denied';
        const statusIcon = wasApproved ? 'check' : 'close';
        const statusText = wasApproved ? 'Approved' : 'Denied';

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__approval restored';
        if (executionId) {
            entry.dataset.approvalId = executionId;
        }

        const restoredTemplate = templateLoader.cache.get('approval-card-restored');
        entry.innerHTML = templateLoader.replace(restoredTemplate, {
            icon,
            headerText,
            timeHtml: displayTime ? `<span class="approval-compact__time">${displayTime}</span>` : '',
            promptHtml: isFileEdit ? '' : '<span class="approval-compact__prompt">$</span>',
            commandDisplay: this.escapeHtml(commandDisplay),
            justification: this.escapeHtml(justification),
            statusClass,
            statusIcon,
            statusText
        });

        this.outputContainer.appendChild(entry);

        if (wasApproved && executionId) {
            this._createResultsContainer(executionId, entry);
        }

        return entry;
    }

    denyAllPendingApprovals(reason, statusMessage = 'Cancelled') {
        if (!this.pendingApprovals?.size) return;

        const webSessionId = webSessionService.getWebSessionId();
        let totalDenied = 0;

        for (const [approvalId, approvalData] of this.pendingApprovals) {
            if (webSessionId) {
                window.serviceClient?.post(ServiceName.g8ed, ApiPaths.approval.respond(), {
                    approval_id: approvalId,
                    approved: false,
                    reason: reason,
                    case_id: approvalData.case_id,
                    investigation_id: approvalData.investigation_id,
                    task_id: approvalData.task_id,
                }).catch(error => {
                    console.error(`[TERMINAL] Failed to deny approval ${approvalId}:`, error);
                });
            }

            const approvalEl = this.outputContainer?.querySelector(`[data-approval-id="${approvalId}"]`);
            if (approvalEl) {
                const actionsDiv = approvalEl.querySelector('.approval-compact__actions');
                if (actionsDiv) {
                    actionsDiv.innerHTML = `
                        <div class="approval-compact__status approval-compact__status--denied">
                            <span class="material-symbols-outlined">close</span>
                            ${statusMessage}
                        </div>
                    `;
                }
            }
            totalDenied++;
        }

        this.pendingApprovals.clear();

        if (totalDenied > 0) {
            console.log(`[TERMINAL] Denied ${totalDenied} pending approval(s) - ${reason}`);
        }
    }

    restoreCommandResult(executionId, data) {
        if (!data) return;

        const container = executionId ? this.executionResultsContainers.get(executionId) : null;

        if (container) {
            const command = data.command;
            const content = data.content;
            const output = this._extractOutputFromContent(content, command);

            this._appendResultToContainer(container, {
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
            this.restoreCommandExecution(data);
        }
    }
}
