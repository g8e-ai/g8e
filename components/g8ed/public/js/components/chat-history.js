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

import { InvestigationFactory, InvestigationHistoryEntry } from '../models/investigation-models.js';
import { notificationService } from '../utils/notification-service.js';
import { EventType } from '../constants/events.js';

export const ChatHistoryMixin = {
    handleCaseSelected(data) {
        this.loadConversationHistoryFromData(data.conversationHistory, data.investigationId);
    },

    loadConversationHistoryFromData(conversationHistory, investigationId) {
        if (!investigationId || !this.currentUser) {
            return;
        }

        try {
            if (conversationHistory && conversationHistory.length > 0) {
                const parsedHistory = InvestigationFactory.parseConversationHistory(conversationHistory);
                this.restoreConversationHistory(parsedHistory, investigationId);
            } else {
                notificationService.info('No previous conversation history found for this investigation');
            }

        } catch (error) {
            console.error('[CHAT] Failed to load conversation history:', error);
            const msg = error instanceof Error ? error.message : 'Unknown error';
            notificationService.error(`Failed to load conversation history: ${msg}`);
        }
    },

    async restoreConversationHistory(conversationHistory, investigationId = null) {
        if (!this.messagesContainer || !conversationHistory) return;

        if (this.anchoredTerminal) {
            this.anchoredTerminal.resetAutoScroll();
        }

        const operatorMessagesByExecution = new Map();
        const restoredExecutions = new Set();

        for (const message of conversationHistory) {
            const msgInstance = message instanceof InvestigationHistoryEntry ? message : InvestigationHistoryEntry.parse(message);
            const metadata = msgInstance.metadata || {};

            if (metadata.sender === EventType.EVENT_SOURCE_USER_TERMINAL) {
                const executionId = metadata.execution_id;

                if (executionId) {
                    if (!operatorMessagesByExecution.has(executionId)) {
                        operatorMessagesByExecution.set(executionId, []);
                    }
                    operatorMessagesByExecution.get(executionId).push(msgInstance);
                }
            }
        }

        const allMessages = conversationHistory.map(msg =>
            msg instanceof InvestigationHistoryEntry ? msg : InvestigationHistoryEntry.parse(msg)
        );

        for (const msgInstance of allMessages) {
            const context = msgInstance.context || {};
            const metadata = msgInstance.metadata || {};

            if (context.event_type === 'thinking_event' ||
                metadata.is_thinking ||
                metadata.source?.includes('thinking')) {
                continue;
            }

            if (metadata.event_type === EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED ||
                metadata.event_type === EventType.OPERATOR_COMMAND_APPROVAL_GRANTED ||
                metadata.event_type === EventType.OPERATOR_COMMAND_APPROVAL_REJECTED ||
                metadata.event_type === EventType.OPERATOR_COMMAND_APPROVAL_PREPARING) {
                continue;
            }

            if (metadata.sender === EventType.EVENT_SOURCE_USER_TERMINAL) {
                const executionId = metadata.execution_id;
                const eventType = metadata.event_type;

                if (executionId && restoredExecutions.has(executionId)) {
                    continue;
                }

                if (eventType === EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED) {
                    const relatedMessages = operatorMessagesByExecution.get(executionId);
                    await this.restoreOperatorActivity(executionId, relatedMessages);
                    if (executionId) restoredExecutions.add(executionId);
                    continue;
                }

                if (eventType === EventType.OPERATOR_COMMAND_EXECUTION && metadata.direct_execution) {
                    await this.restoreDirectCommand(msgInstance);
                    if (executionId) restoredExecutions.add(executionId);
                    continue;
                }

                if (eventType === EventType.OPERATOR_COMMAND_EXECUTION || eventType === EventType.OPERATOR_COMMAND_RESULT) {
                    const relatedMessages = operatorMessagesByExecution.get(executionId);
                    const hasApprovalRequest = relatedMessages?.some(m =>
                        (m.metadata?.event_type) === EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED
                    ) ?? false;

                    if (!hasApprovalRequest) {
                        await this.restoreDirectCommand(msgInstance);
                        if (executionId) restoredExecutions.add(executionId);
                    }
                    continue;
                }

                continue;
            }

            let senderType;
            if (msgInstance.isUserMessage()) {
                senderType = EventType.EVENT_SOURCE_USER_CHAT;
            } else if (msgInstance.isAIResponse()) {
                senderType = EventType.EVENT_SOURCE_AI_PRIMARY;
            } else {
                senderType = EventType.EVENT_SOURCE_SYSTEM;
            }

            this.addRestoredMessage(
                msgInstance.content,
                senderType,
                msgInstance.timestamp,
                {
                    investigation_id: context.investigation_id,
                    case_id: context.case_id,
                    event_type: context.event_type
                },
                msgInstance.grounding_metadata
            );
        }

        if (this.anchoredTerminal) {
            this.anchoredTerminal.resetAutoScroll();
            requestAnimationFrame(() => this.anchoredTerminal.scrollToBottom());
        }
    },

    async restoreDirectCommand(msgInstance) {
        if (!this.anchoredTerminal) return;

        const metadata = msgInstance.metadata || {};

        await this.anchoredTerminal.restoreCommandExecution({
            execution_id: metadata.execution_id,
            command: metadata.command,
            content: msgInstance.content,
            status: metadata.status || 'completed',
            exit_code: metadata.exit_code,
            direct_execution: metadata.direct_execution,
            timestamp: msgInstance.timestamp,
            hostname: metadata.hostname
        });
    },

    async restoreOperatorActivity(executionId, operatorMessages) {
        if (!executionId || !operatorMessages || operatorMessages.length === 0) return;
        if (!this.anchoredTerminal) return;

        operatorMessages.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));

        let approvalRequest = null;
        let approvalDecision = null;
        const commandResults = [];
        let wasApproved = false;

        for (const msg of operatorMessages) {
            const metadata = msg.metadata || {};
            const eventType = metadata.event_type;

            if (eventType === EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED) {
                approvalRequest = { ...metadata, timestamp: msg.timestamp, content: msg.content };
            } else if (eventType === EventType.OPERATOR_COMMAND_APPROVAL_GRANTED) {
                approvalDecision = metadata;
                wasApproved = true;
            } else if (eventType === EventType.OPERATOR_COMMAND_APPROVAL_REJECTED) {
                approvalDecision = metadata;
                wasApproved = false;
            } else if (eventType === EventType.OPERATOR_COMMAND_RESULT || eventType === EventType.OPERATOR_COMMAND_EXECUTION) {
                commandResults.push({ ...metadata, content: msg.content, timestamp: msg.timestamp });
            }
        }

        const containerKey = approvalRequest?.approval_id || executionId;

        if (approvalRequest) {
            await this.anchoredTerminal.restoreApprovalRequest(approvalRequest, wasApproved, containerKey);
        }

        if (wasApproved && commandResults.length > 0) {
            for (const result of commandResults) {
                await this.anchoredTerminal.restoreCommandResult(containerKey, {
                    command: result.command || approvalRequest?.command,
                    content: result.content,
                    status: result.status || 'completed',
                    exit_code: result.exit_code,
                    direct_execution: false,
                    timestamp: result.timestamp,
                    operator_id: result.operator_id,
                    hostname: result.hostname
                });
            }
        }
    },

    addRestoredMessage(content, sender, originalTimestamp, contextInfo = null, groundingMetadata = null) {
        if (!this.anchoredTerminal) return;

        const displayTime = originalTimestamp
            ? new Date(originalTimestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
            : null;

        if (sender === EventType.EVENT_SOURCE_USER_CHAT) {
            this.anchoredTerminal.appendUserMessage(content, displayTime);
        } else if (sender === EventType.EVENT_SOURCE_AI_PRIMARY) {
            const formattedContent = this.messageRenderer.renderContent(content);
            this.anchoredTerminal.appendDirectHtmlResponse(formattedContent, displayTime, groundingMetadata);
        } else if (sender === EventType.EVENT_SOURCE_SYSTEM) {
            this.anchoredTerminal.appendSystemMessage(content);
        }
    },
};
