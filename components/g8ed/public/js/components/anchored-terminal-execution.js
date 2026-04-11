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
import { nowISOString } from '../utils/timestamp.js';
import { templateLoader } from '../utils/template-loader.js';
import { escapeHtml } from '../utils/html.js';
import { webSessionService } from '../utils/web-session-service.js';
import { ServiceName } from '../constants/service-client-constants.js';
import { ApiPaths } from '../constants/api-paths.js';

export class TerminalExecutionMixin {
    initExecutionState() {
        this.pendingApprovals = new Map();
        this.activeExecutions = new Map();
        this.executionResultsContainers = new Map();
    }

    async showExecutingIndicator(command) {
        if (!this.outputContainer) return null;

        if (this._execCounter === undefined) this._execCounter = 0;
        const id = `exec-${Date.now()}-${++this._execCounter}`;

        const indicator = document.createElement('div');
        indicator.className = 'anchored-terminal__executing';
        indicator.id = id;
        await templateLoader.renderTo(indicator, 'executing-indicator', { command });

        this.outputContainer.appendChild(indicator);
        this.scrollToBottom();

        return id;
    }

    async showPreparingIndicator(command) {
        if (!this.outputContainer) return null;

        if (this._execCounter === undefined) this._execCounter = 0;
        const id = `exec-${Date.now()}-${++this._execCounter}`;

        const indicator = document.createElement('div');
        indicator.className = 'anchored-terminal__executing';
        indicator.id = id;
        await templateLoader.renderTo(indicator, 'preparing-indicator', { command });

        this.outputContainer.appendChild(indicator);
        this.scrollToBottom();

        return id;
    }

    async _showExecutingIndicatorInContainer(container, command) {
        if (!container || typeof container.querySelector !== 'function') return null;

        const id = `exec-${Date.now()}`;
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

    async handleApprovalRequest(data) {
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
        approval.setAttribute('data-approval-id', approvalId);

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

        await templateLoader.renderTo(approval, 'approval-card', {
            cardModifier,
            icon,
            iconModifier,
            headerText,
            riskBadgeHtml,
            promptHtml: isFileEdit ? '' : '<span class="approval-compact__prompt">$</span>',
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

        this.outputContainer.appendChild(approval);
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

            if (approvalEl) {
                const actionsDiv = approvalEl.querySelector('.approval-compact__actions');
                if (actionsDiv) {
                    await templateLoader.renderTo(actionsDiv, 'approval-status', {
                        statusClass: approved ? 'approved' : 'denied',
                        statusIcon: approved ? 'check' : 'close',
                        statusText: approved ? 'Approved' : 'Denied'
                    });
                }

                if (approved) {
                    const resultsContainer = this._createResultsContainer(approvalId, approvalEl);
                    this._pendingExecutingIndicator = await this._showExecutingIndicatorInContainer(
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

    async _createResultsContainer(executionId, approvalEl) {
        if (!this.outputContainer) return null;

        if (this.executionResultsContainers.has(executionId)) {
            return this.executionResultsContainers.get(executionId);
        }

        const container = document.createElement('div');
        container.className = 'anchored-terminal__results-group';
        container.setAttribute('data-execution-id', executionId);

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

    async _appendResultToContainer(container, resultData) {
        if (!container || typeof container.querySelector !== 'function') return;

        const body = container.querySelector('.anchored-terminal__results-body');
        if (!body) return;

        const { command, stdout, stderr, exitCode, status, timestamp, operatorId, hostname } = resultData;

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
            outputContent += stdout;
        }
        if (stderr) {
            outputContent += (outputContent ? '\n' : '') + stderr;
        }
        if (!outputContent) {
            outputContent = '(No output)';
        }

        const hostnameHtml = hostname
            ? `<span class="anchored-terminal__result-hostname"><span class="material-symbols-outlined icon-12">computer</span>${hostname}</span>`
            : '';

        await templateLoader.renderTo(entry, 'command-result', {
            statusClass,
            statusIcon,
            hostnameHtml: hostnameHtml || '',
            command: escapeHtml(command),
            displayTime,
            outputContent: escapeHtml(outputContent),
            exitCodeHtml: exitCode !== undefined
                ? `<div class="anchored-terminal__result-exit anchored-terminal__result-exit--${exitCode === 0 ? 'success' : 'error'}">Exit code: ${exitCode}</div>`
                : '',
            hostnameHtml_raw: hostnameHtml // for trusted rendering
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
            if (execId && !this.activeExecutions.has(execId)) {
                const indicatorId = await this.showPreparingIndicator(command);
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

            let resultsContainer =
                (approvalId ? this.executionResultsContainers.get(approvalId) : null) ||
                (execId ? this.executionResultsContainers.get(execId) : null);

            if (!resultsContainer) {
                const containerId = approvalId || execId;
                if (!containerId) {
                    console.error('[TERMINAL] Received final command event with no execution_id or approval_id — cannot render result', data);
                    return;
                }
                resultsContainer = await this._createResultsContainer(containerId);
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

        const containerId = data.approval_id || data.execution_id;
        if (!containerId) {
            console.error('[TERMINAL] Received intent result with no approval_id or execution_id — cannot render result', data);
            return;
        }
        const container = await this._createResultsContainer(containerId);
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
        const container = await this._createResultsContainer(containerId);
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
            ? new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
            : '';

        const statusClass = wasApproved ? 'approved' : 'denied';
        const statusIcon = wasApproved ? 'check' : 'close';
        const statusText = wasApproved ? 'Approved' : 'Denied';

        const entry = document.createElement('div');
        entry.className = 'anchored-terminal__approval restored';
        if (executionId) {
            entry.setAttribute('data-approval-id', executionId);
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
            timeHtml_raw: displayTime ? `<span class="approval-compact__time">${displayTime}</span>` : '' // for trusted rendering
        });

        this.outputContainer.appendChild(entry);

        if (wasApproved && executionId) {
            await this._createResultsContainer(executionId, entry);
        }

        return entry;
    }

    async denyAllPendingApprovals(reason, statusMessage = 'Cancelled') {
        if (!this.pendingApprovals?.size) return;

        const webSessionId = webSessionService.getWebSessionId();
        let totalDenied = 0;

        for (const [approvalId, approvalData] of this.pendingApprovals) {
            if (webSessionId) {
                window.serviceClient?.post(ComponentName.G8ED, ApiPaths.approval.respond(), {
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
                    await templateLoader.renderTo(actionsDiv, 'approval-status', {
                        statusClass: 'denied',
                        statusIcon: 'close',
                        statusText: statusMessage
                    });
                }
            }
            totalDenied++;
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
