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
import { OperatorStatus } from '../constants/operator-constants.js';

export class TerminalOperatorMixin {
    initOperatorState() {
        this.isOperatorBound = false;
        this.boundOperator = null;
    }

    bindEventBusListeners() {
        if (this._eventsBound || !this.eventBus) return;

        this.eventBus.on(EventType.OPERATOR_STATUS_UPDATED_BOUND, (data) => {
            if (data?.operator) this.setOperatorBound(data.operator);
        });

        const unboundStatuses = [
            EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
            EventType.OPERATOR_STATUS_UPDATED_AVAILABLE,
            EventType.OPERATOR_STATUS_UPDATED_UNAVAILABLE,
            EventType.OPERATOR_STATUS_UPDATED_OFFLINE,
            EventType.OPERATOR_STATUS_UPDATED_STALE,
            EventType.OPERATOR_STATUS_UPDATED_STOPPED,
            EventType.OPERATOR_STATUS_UPDATED_TERMINATED,
        ];
        for (const eventType of unboundStatuses) {
            this.eventBus.on(eventType, () => this.setOperatorUnbound());
        }

        this.eventBus.on(EventType.OPERATOR_PANEL_LIST_UPDATED, (data) => {
            this.handleOperatorListUpdate(data);
        });

        this.eventBus.on(EventType.OPERATOR_BOUND, (data) => {
            this.handleOperatorBound(data);
        });

        this.eventBus.on(EventType.OPERATOR_UNBOUND, (data) => {
            this.handleOperatorUnbound(data);
        });

        this.eventBus.on(EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED, async (data) => {
            await this.handleApprovalRequest(data);
        });

        this.eventBus.on(EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED, async (data) => {
            await this.handleApprovalRequest(data);
        });

        this.eventBus.on(EventType.OPERATOR_INTENT_APPROVAL_REQUESTED, async (data) => {
            await this.handleApprovalRequest(data);
        });

        this.eventBus.on(EventType.OPERATOR_STREAM_APPROVAL_REQUESTED, async (data) => {
            await this.handleApprovalRequest(data);
        });

        this.eventBus.on(EventType.AI_AGENT_CONTINUE_APPROVAL_REQUESTED, async (data) => {
            await this.handleApprovalRequest(data);
        });

        for (const eventType of [
            EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
            EventType.OPERATOR_COMMAND_STARTED,
            EventType.OPERATOR_COMMAND_OUTPUT_RECEIVED,
            EventType.OPERATOR_COMMAND_COMPLETED,
            EventType.OPERATOR_COMMAND_FAILED,
            EventType.OPERATOR_COMMAND_APPROVAL_GRANTED,
            EventType.OPERATOR_COMMAND_APPROVAL_REJECTED,
            EventType.OPERATOR_FILE_EDIT_STARTED,
            EventType.OPERATOR_FILE_EDIT_COMPLETED,
            EventType.OPERATOR_FILE_EDIT_FAILED,
            EventType.OPERATOR_FILE_EDIT_APPROVAL_GRANTED,
            EventType.OPERATOR_FILE_EDIT_APPROVAL_REJECTED,
        ]) {
            this.eventBus.on(eventType, async (data) => {
                await this.handleCommandExecutionEvent({ ...data, eventType });
            });
        }

        for (const eventType of [
            EventType.OPERATOR_INTENT_GRANTED,
            EventType.OPERATOR_INTENT_DENIED,
            EventType.OPERATOR_INTENT_REVOKED,
            EventType.OPERATOR_INTENT_APPROVAL_GRANTED,
            EventType.OPERATOR_INTENT_APPROVAL_REJECTED,
        ]) {
            this.eventBus.on(eventType, async (data) => {
                await this.handleIntentResult({ ...data, eventType });
            });
        }

        this.eventBus.on(EventType.OPERATOR_TERMINAL_APPROVAL_DENIED, async ({ reason, statusMessage }) => {
            await this.denyAllPendingApprovals(reason, statusMessage);
        });

        this.eventBus.on(EventType.OPERATOR_TERMINAL_AUTH_STATE_CHANGED, ({ isAuthenticated, user }) => {
            if (isAuthenticated && user) {
                this.setUser(user);
                this.enable();
                this.focus();
            } else {
                this.setUser(null);
                this.disable();
            }
        });

        this._eventsBound = true;
    }

    handleOperatorListUpdate(data) {
        if (!data || !Array.isArray(data.operators)) return;

        const boundOperator = data.operators.find(op =>
            op.status === OperatorStatus.BOUND || op.is_bound === true
        );

        if (boundOperator) {
            this.setOperatorBound(boundOperator);
        } else {
            this.setOperatorUnbound();
        }
    }

    handleOperatorBound(data) {
        if (data && data.operator) {
            this.setOperatorBound(data.operator);
        }
    }

    handleOperatorUnbound() {
        this.setOperatorUnbound();
    }

    setOperatorBound(operator) {
        if (this.isOperatorBound && this.boundOperator?.operator_id === operator?.operator_id) {
            return;
        }

        this.isOperatorBound = true;
        this.boundOperator = operator;

        const heartbeatSnapshot = operator.latest_heartbeat_snapshot || {};
        const systemIdentity = heartbeatSnapshot.system_identity || {};

        if (this.hostnameElement) {
            const hostname = systemIdentity.hostname || operator.name || 'operator';
            this.hostnameElement.textContent = hostname;
        }

        if (this.promptElement) {
            const user = systemIdentity.current_user || '$';
            this.promptElement.textContent = user === '$' ? '$' : `${user}$`;
        }

        this.updateInputState();
        this.appendSystemMessage(`Connected to ${operator.name || 'operator'}`);
    }

    setOperatorUnbound() {
        this.isOperatorBound = false;
        this.boundOperator = null;

        if (this.hostnameElement) {
            this.hostnameElement.textContent = '';
        }

        if (this.promptElement) {
            this.promptElement.textContent = '$';
        }

        this.updateInputState();
    }
}
