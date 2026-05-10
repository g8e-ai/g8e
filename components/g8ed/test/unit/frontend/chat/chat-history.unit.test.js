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

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { EventType } from '@g8ed/public/js/constants/events.js';

vi.mock('@g8ed/public/js/utils/notification-service.js', () => ({
    notificationService: {
        info: vi.fn(),
        error: vi.fn(),
        show: vi.fn(),
    }
}));

vi.mock('@g8ed/public/js/models/investigation-models.js', () => {
    class MockInvestigationHistoryEntry {
        constructor(data) {
            this.timestamp = data.timestamp;
            this.event_type = data.event_type;
            this.actor = data.actor;
            this.summary = data.summary;
            this.content = data.content;
            this.metadata = data.metadata || {};
            this.context = data.context || {};
            this.grounding_metadata = data.grounding_metadata || null;
        }

        isUserMessage() {
            const sender = this.metadata?.sender;
            if (!sender) return false;
            return sender.includes('user.chat') || sender.includes('user.terminal');
        }

        isAIResponse() {
            const sender = this.metadata?.sender;
            if (!sender) return false;
            return sender.includes('ai.primary') || sender.includes('ai.assistant');
        }

        isSystemMessage() {
            const sender = this.metadata?.sender;
            if (!sender) return false;
            return sender.includes('system');
        }

        static parse(data) {
            return new MockInvestigationHistoryEntry(data);
        }
    }

    return {
        InvestigationFactory: {
            parseConversationHistory: vi.fn((history) => history.map(msg => MockInvestigationHistoryEntry.parse(msg))),
        },
        InvestigationHistoryEntry: MockInvestigationHistoryEntry,
    };
});

import { notificationService } from '@g8ed/public/js/utils/notification-service.js';
import { InvestigationFactory, InvestigationHistoryEntry } from '@g8ed/public/js/models/investigation-models.js';
import { ChatHistoryMixin } from '@g8ed/public/js/components/chat-history.js';

const INVESTIGATION_ID = 'inv-test-history123';
const EXECUTION_ID = 'cmd-test-execution123';
const APPROVAL_ID = 'approval-test123';

function createTestComponent() {
    const component = Object.assign({}, ChatHistoryMixin);
    
    component.currentUser = { id: 'user-test123', email: 'test@example.com' };
    component.messagesContainer = document.createElement('div');
    component.anchoredTerminal = {
        resetAutoScroll: vi.fn(),
        scrollToBottom: vi.fn(),
        restoreCommandExecution: vi.fn(),
        restoreApprovalRequest: vi.fn(),
        restoreCommandResult: vi.fn(),
        appendUserMessage: vi.fn(() => document.createElement('div')),
        appendDirectHtmlResponse: vi.fn(() => document.createElement('div')),
        appendSystemMessage: vi.fn(() => document.createElement('div')),
    };
    component.messageRenderer = {
        renderContent: vi.fn((content) => content),
    };
    
    return component;
}

function makeConversationMessage(overrides = {}) {
    return {
        timestamp: new Date('2026-01-15T10:30:00Z').toISOString(),
        actor: EventType.EVENT_SOURCE_USER_CHAT,
        event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER,
        content: 'Test message',
        summary: 'Test message',
        metadata: {
            sender: EventType.EVENT_SOURCE_USER_CHAT,
            event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER,
        },
        context: {
            investigation_id: INVESTIGATION_ID,
            case_id: 'case-test123',
            event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER,
        },
        ...overrides,
    };
}

function makeOperatorMessage(overrides = {}) {
    return {
        timestamp: new Date('2026-01-15T10:31:00Z').toISOString(),
        actor: EventType.EVENT_SOURCE_USER_TERMINAL,
        event_type: EventType.OPERATOR_COMMAND_EXECUTION,
        content: 'ls -la',
        summary: 'Command execution',
        metadata: {
            sender: EventType.EVENT_SOURCE_USER_TERMINAL,
            event_type: EventType.OPERATOR_COMMAND_EXECUTION,
            execution_id: EXECUTION_ID,
            command: 'ls -la',
            status: 'completed',
            exit_code: 0,
            hostname: 'test-host',
        },
        context: {
            investigation_id: INVESTIGATION_ID,
            event_type: EventType.OPERATOR_COMMAND_EXECUTION,
        },
        ...overrides,
    };
}

function makeApprovalRequestMessage(overrides = {}) {
    return {
        timestamp: new Date('2026-01-15T10:32:00Z').toISOString(),
        actor: EventType.EVENT_SOURCE_USER_TERMINAL,
        event_type: EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
        content: 'Approval required for command',
        summary: 'Approval requested',
        metadata: {
            sender: EventType.EVENT_SOURCE_USER_TERMINAL,
            event_type: EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
            execution_id: EXECUTION_ID,
            approval_id: APPROVAL_ID,
            command: 'rm -rf /tmp/test',
            hostname: 'test-host',
        },
        context: {
            investigation_id: INVESTIGATION_ID,
            event_type: EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
        },
        ...overrides,
    };
}

function makeApprovalGrantedMessage(overrides = {}) {
    return {
        timestamp: new Date('2026-01-15T10:33:00Z').toISOString(),
        actor: EventType.EVENT_SOURCE_USER_TERMINAL,
        event_type: EventType.OPERATOR_COMMAND_APPROVAL_GRANTED,
        content: 'Command approved',
        summary: 'Approval granted',
        metadata: {
            sender: EventType.EVENT_SOURCE_USER_TERMINAL,
            event_type: EventType.OPERATOR_COMMAND_APPROVAL_GRANTED,
            execution_id: EXECUTION_ID,
            approval_id: APPROVAL_ID,
        },
        context: {
            investigation_id: INVESTIGATION_ID,
            event_type: EventType.OPERATOR_COMMAND_APPROVAL_GRANTED,
        },
        ...overrides,
    };
}

function makeApprovalRejectedMessage(overrides = {}) {
    return {
        timestamp: new Date('2026-01-15T10:33:00Z').toISOString(),
        actor: EventType.EVENT_SOURCE_USER_TERMINAL,
        event_type: EventType.OPERATOR_COMMAND_APPROVAL_REJECTED,
        content: 'Command rejected',
        summary: 'Approval rejected',
        metadata: {
            sender: EventType.EVENT_SOURCE_USER_TERMINAL,
            event_type: EventType.OPERATOR_COMMAND_APPROVAL_REJECTED,
            execution_id: EXECUTION_ID,
            approval_id: APPROVAL_ID,
            reason: 'Unsafe command',
        },
        context: {
            investigation_id: INVESTIGATION_ID,
            event_type: EventType.OPERATOR_COMMAND_APPROVAL_REJECTED,
        },
        ...overrides,
    };
}

function makeCommandResultMessage(overrides = {}) {
    return {
        timestamp: new Date('2026-01-15T10:34:00Z').toISOString(),
        actor: EventType.EVENT_SOURCE_USER_TERMINAL,
        event_type: EventType.OPERATOR_COMMAND_RESULT,
        content: 'Command output',
        summary: 'Command result',
        metadata: {
            sender: EventType.EVENT_SOURCE_USER_TERMINAL,
            event_type: EventType.OPERATOR_COMMAND_RESULT,
            execution_id: EXECUTION_ID,
            command: 'ls -la',
            status: 'completed',
            exit_code: 0,
            hostname: 'test-host',
            operator_id: 'op-test123',
        },
        context: {
            investigation_id: INVESTIGATION_ID,
            event_type: EventType.OPERATOR_COMMAND_RESULT,
        },
        ...overrides,
    };
}

describe('ChatHistoryMixin', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        global.requestAnimationFrame = vi.fn((cb) => cb());
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    describe('handleCaseSelected', () => {
        it('delegates to loadConversationHistoryFromData with correct parameters', () => {
            const component = createTestComponent();
            component.loadConversationHistoryFromData = vi.fn();
            
            const data = {
                conversationHistory: [makeConversationMessage()],
                investigationId: INVESTIGATION_ID,
            };
            
            component.handleCaseSelected(data);
            
            expect(component.loadConversationHistoryFromData).toHaveBeenCalledWith(
                data.conversationHistory,
                data.investigationId
            );
        });
    });

    describe('loadConversationHistoryFromData', () => {
        it('returns early when investigationId is missing', () => {
            const component = createTestComponent();
            component.restoreConversationHistory = vi.fn();
            
            component.loadConversationHistoryFromData([makeConversationMessage()], null);
            
            expect(component.restoreConversationHistory).not.toHaveBeenCalled();
        });

        it('returns early when currentUser is missing', () => {
            const component = createTestComponent();
            component.currentUser = null;
            component.restoreConversationHistory = vi.fn();
            
            component.loadConversationHistoryFromData([makeConversationMessage()], INVESTIGATION_ID);
            
            expect(component.restoreConversationHistory).not.toHaveBeenCalled();
        });

        it('parses and restores conversation history when data is valid', () => {
            const component = createTestComponent();
            component.restoreConversationHistory = vi.fn();
            const history = [makeConversationMessage()];
            
            component.loadConversationHistoryFromData(history, INVESTIGATION_ID);
            
            expect(InvestigationFactory.parseConversationHistory).toHaveBeenCalledWith(history);
            expect(component.restoreConversationHistory).toHaveBeenCalled();
        });

        it('shows info notification when conversation history is empty', () => {
            const component = createTestComponent();
            
            component.loadConversationHistoryFromData([], INVESTIGATION_ID);
            
            expect(notificationService.info).toHaveBeenCalledWith(
                'No previous conversation history found for this investigation'
            );
        });

        it('shows info notification when conversation history is null', () => {
            const component = createTestComponent();
            
            component.loadConversationHistoryFromData(null, INVESTIGATION_ID);
            
            expect(notificationService.info).toHaveBeenCalledWith(
                'No previous conversation history found for this investigation'
            );
        });

        it('logs error and shows notification when parsing fails', () => {
            const component = createTestComponent();
            const testError = new Error('Parse failed');
            InvestigationFactory.parseConversationHistory.mockImplementation(() => {
                throw testError;
            });
            
            component.loadConversationHistoryFromData([makeConversationMessage()], INVESTIGATION_ID);
            
            expect(notificationService.error).toHaveBeenCalledWith(
                'Failed to load conversation history: Parse failed'
            );
        });

        it('handles non-error objects in error notification', () => {
            const component = createTestComponent();
            InvestigationFactory.parseConversationHistory.mockImplementation(() => {
                throw 'String error';
            });
            
            component.loadConversationHistoryFromData([makeConversationMessage()], INVESTIGATION_ID);
            
            expect(notificationService.error).toHaveBeenCalledWith(
                'Failed to load conversation history: Unknown error'
            );
        });
    });

    describe('restoreConversationHistory', () => {
        it('returns early when messagesContainer is missing', () => {
            const component = createTestComponent();
            component.messagesContainer = null;
            
            component.restoreConversationHistory([makeConversationMessage()], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.resetAutoScroll).not.toHaveBeenCalled();
        });

        it('returns early when conversationHistory is missing', () => {
            const component = createTestComponent();
            
            component.restoreConversationHistory(null, INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.resetAutoScroll).not.toHaveBeenCalled();
        });

        it('resets auto scroll when anchoredTerminal exists', () => {
            const component = createTestComponent();
            
            component.restoreConversationHistory([makeConversationMessage()], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.resetAutoScroll).toHaveBeenCalled();
        });

        it('skips thinking events (context.event_type)', () => {
            const component = createTestComponent();
            const thinkingMessage = makeConversationMessage({
                context: {
                    event_type: 'thinking_event',
                    investigation_id: INVESTIGATION_ID,
                },
            });
            
            component.restoreConversationHistory([thinkingMessage], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.appendUserMessage).not.toHaveBeenCalled();
            expect(component.anchoredTerminal.appendDirectHtmlResponse).not.toHaveBeenCalled();
            expect(component.anchoredTerminal.appendSystemMessage).not.toHaveBeenCalled();
        });

        it('skips thinking events (metadata.is_thinking)', () => {
            const component = createTestComponent();
            const thinkingMessage = makeConversationMessage({
                metadata: {
                    sender: EventType.EVENT_SOURCE_USER_CHAT,
                    event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER,
                    is_thinking: true,
                },
            });
            
            component.restoreConversationHistory([thinkingMessage], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.appendUserMessage).not.toHaveBeenCalled();
            expect(component.anchoredTerminal.appendDirectHtmlResponse).not.toHaveBeenCalled();
            expect(component.anchoredTerminal.appendSystemMessage).not.toHaveBeenCalled();
        });

        it('skips thinking events (metadata.source.includes thinking)', () => {
            const component = createTestComponent();
            const thinkingMessage = makeConversationMessage({
                metadata: {
                    sender: EventType.EVENT_SOURCE_USER_CHAT,
                    event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER,
                    source: 'thinking_process',
                },
            });
            
            component.restoreConversationHistory([thinkingMessage], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.appendUserMessage).not.toHaveBeenCalled();
            expect(component.anchoredTerminal.appendDirectHtmlResponse).not.toHaveBeenCalled();
            expect(component.anchoredTerminal.appendSystemMessage).not.toHaveBeenCalled();
        });

        it('restores user chat messages', () => {
            const component = createTestComponent();
            const userMessage = makeConversationMessage({
                metadata: {
                    sender: EventType.EVENT_SOURCE_USER_CHAT,
                    event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER,
                },
            });
            
            component.restoreConversationHistory([userMessage], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.appendUserMessage).toHaveBeenCalledWith(
                userMessage.content,
                expect.any(String)
            );
        });

        it('restores AI response messages', () => {
            const component = createTestComponent();
            const aiMessage = makeConversationMessage({
                metadata: {
                    sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                    event_type: EventType.INVESTIGATION_CHAT_MESSAGE_AI,
                },
                grounding_metadata: { citations: ['cite1'] },
            });
            
            component.restoreConversationHistory([aiMessage], INVESTIGATION_ID);
            
            expect(component.messageRenderer.renderContent).toHaveBeenCalledWith(aiMessage.content);
            expect(component.anchoredTerminal.appendDirectHtmlResponse).toHaveBeenCalledWith(
                aiMessage.content,
                expect.any(String),
                { citations: ['cite1'] }
            );
        });

        it('restores system messages', () => {
            const component = createTestComponent();
            const systemMessage = makeConversationMessage({
                metadata: {
                    sender: EventType.EVENT_SOURCE_SYSTEM,
                    event_type: EventType.INVESTIGATION_CHAT_MESSAGE_SYSTEM,
                },
            });
            
            component.restoreConversationHistory([systemMessage], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.appendSystemMessage).toHaveBeenCalledWith(systemMessage.content);
        });

        it('calls scrollToBottom after restoration when anchoredTerminal exists', () => {
            const component = createTestComponent();
            
            component.restoreConversationHistory([makeConversationMessage()], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.scrollToBottom).toHaveBeenCalled();
        });

        it('does not call scrollToBottom when anchoredTerminal is missing', () => {
            const component = createTestComponent();
            component.anchoredTerminal = null;
            
            component.restoreConversationHistory([makeConversationMessage()], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal).toBeNull();
        });
    });

    describe('restoreConversationHistory - operator command handling', () => {
        it('groups operator messages by execution_id', () => {
            const component = createTestComponent();
            const executionId1 = 'cmd-exec1';
            const executionId2 = 'cmd-exec2';
            
            const messages = [
                makeOperatorMessage({ metadata: { ...makeOperatorMessage().metadata, execution_id: executionId1 } }),
                makeOperatorMessage({ metadata: { ...makeOperatorMessage().metadata, execution_id: executionId2 } }),
                makeOperatorMessage({ metadata: { ...makeOperatorMessage().metadata, execution_id: executionId1 } }),
            ];
            
            component.restoreConversationHistory(messages, INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreCommandExecution).toHaveBeenCalled();
        });

        it('restores direct execution command', () => {
            const component = createTestComponent();
            const directCommand = makeOperatorMessage({
                metadata: {
                    ...makeOperatorMessage().metadata,
                    event_type: EventType.OPERATOR_COMMAND_EXECUTION,
                    direct_execution: true,
                },
            });
            
            component.restoreConversationHistory([directCommand], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreCommandExecution).toHaveBeenCalledWith(
                expect.objectContaining({
                    execution_id: EXECUTION_ID,
                    command: directCommand.metadata.command,
                    direct_execution: true,
                })
            );
        });

        it('filters out approval requested events during restoration', () => {
            const component = createTestComponent();
            const approvalRequest = makeApprovalRequestMessage();
            
            component.restoreConversationHistory([approvalRequest], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
        });

        it('filters out approval granted events during restoration', async () => {
            const component = createTestComponent();
            const messages = [
                makeApprovalRequestMessage(),
                makeApprovalGrantedMessage(),
                makeCommandResultMessage(),
            ];
            
            await component.restoreConversationHistory(messages, INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
            expect(component.anchoredTerminal.restoreCommandResult).not.toHaveBeenCalled();
        });

        it('filters out approval rejected events during restoration', () => {
            const component = createTestComponent();
            const messages = [
                makeApprovalRequestMessage(),
                makeApprovalRejectedMessage(),
            ];
            
            component.restoreConversationHistory(messages, INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
            expect(component.anchoredTerminal.restoreCommandResult).not.toHaveBeenCalled();
        });

        it('restores direct command for execution without approval request', () => {
            const component = createTestComponent();
            const executionMessage = makeOperatorMessage({
                metadata: {
                    ...makeOperatorMessage().metadata,
                    event_type: EventType.OPERATOR_COMMAND_EXECUTION,
                },
            });
            
            component.restoreConversationHistory([executionMessage], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreCommandExecution).toHaveBeenCalled();
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
        });

        it('restores direct command for result without approval request', () => {
            const component = createTestComponent();
            const resultMessage = makeCommandResultMessage({
                metadata: {
                    ...makeCommandResultMessage().metadata,
                    event_type: EventType.OPERATOR_COMMAND_RESULT,
                },
            });
            
            component.restoreConversationHistory([resultMessage], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreCommandExecution).toHaveBeenCalled();
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
        });

        it('skips restoring execution if already restored', () => {
            const component = createTestComponent();
            const messages = [
                makeOperatorMessage(),
                makeOperatorMessage(),
            ];
            
            component.restoreConversationHistory(messages, INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreCommandExecution).toHaveBeenCalledTimes(1);
        });

        it('filters out approval preparing events during restoration', () => {
            const component = createTestComponent();
            const approvalPreparing = makeOperatorMessage({
                metadata: {
                    ...makeOperatorMessage().metadata,
                    event_type: EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
                },
            });
            
            component.restoreConversationHistory([approvalPreparing], INVESTIGATION_ID);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
        });
    });

    describe('restoreDirectCommand', () => {
        it('returns early when anchoredTerminal is missing', () => {
            const component = createTestComponent();
            component.anchoredTerminal = null;
            
            component.restoreDirectCommand(makeOperatorMessage());
            
            expect(component.anchoredTerminal).toBeNull();
        });

        it('calls restoreCommandExecution with correct parameters', () => {
            const component = createTestComponent();
            const msgInstance = makeOperatorMessage();
            
            component.restoreDirectCommand(msgInstance);
            
            expect(component.anchoredTerminal.restoreCommandExecution).toHaveBeenCalledWith({
                execution_id: EXECUTION_ID,
                command: msgInstance.metadata.command,
                content: msgInstance.content,
                status: msgInstance.metadata.status,
                exit_code: msgInstance.metadata.exit_code,
                direct_execution: msgInstance.metadata.direct_execution,
                timestamp: msgInstance.timestamp,
                hostname: msgInstance.metadata.hostname,
            });
        });

        it('uses default status when metadata.status is missing', () => {
            const component = createTestComponent();
            const msgInstance = makeOperatorMessage({
                metadata: {
                    ...makeOperatorMessage().metadata,
                    status: undefined,
                },
            });
            
            component.restoreDirectCommand(msgInstance);
            
            const callArgs = component.anchoredTerminal.restoreCommandExecution.mock.calls[0][0];
            expect(callArgs.status).toBe('completed');
        });

        it('handles missing metadata gracefully', () => {
            const component = createTestComponent();
            const msgInstance = {
                content: 'test',
                timestamp: new Date().toISOString(),
                metadata: undefined,
            };
            
            component.restoreDirectCommand(msgInstance);
            
            expect(component.anchoredTerminal.restoreCommandExecution).toHaveBeenCalledWith({
                execution_id: undefined,
                command: undefined,
                content: 'test',
                status: 'completed',
                exit_code: undefined,
                direct_execution: undefined,
                timestamp: msgInstance.timestamp,
                hostname: undefined,
            });
        });
    });

    describe('restoreOperatorActivity', () => {
        it('returns early when executionId is missing', () => {
            const component = createTestComponent();
            
            component.restoreOperatorActivity(null, [makeOperatorMessage()]);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
        });

        it('returns early when operatorMessages is missing', () => {
            const component = createTestComponent();
            
            component.restoreOperatorActivity(EXECUTION_ID, null);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
        });

        it('returns early when operatorMessages is empty', () => {
            const component = createTestComponent();
            
            component.restoreOperatorActivity(EXECUTION_ID, []);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).not.toHaveBeenCalled();
        });

        it('returns early when anchoredTerminal is missing', () => {
            const component = createTestComponent();
            component.anchoredTerminal = null;
            
            component.restoreOperatorActivity(EXECUTION_ID, [makeOperatorMessage()]);
            
            expect(component.anchoredTerminal).toBeNull();
        });

        it('sorts operator messages by timestamp', () => {
            const component = createTestComponent();
            const msg1 = makeApprovalRequestMessage({
                timestamp: new Date('2026-01-15T10:35:00Z').toISOString(),
            });
            const msg2 = makeApprovalGrantedMessage({
                timestamp: new Date('2026-01-15T10:33:00Z').toISOString(),
            });
            
            component.restoreOperatorActivity(EXECUTION_ID, [msg1, msg2]);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).toHaveBeenCalled();
        });

        it('restores approval request with approval_id as container key', () => {
            const component = createTestComponent();
            const approvalRequest = makeApprovalRequestMessage();
            
            component.restoreOperatorActivity(EXECUTION_ID, [approvalRequest]);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).toHaveBeenCalledWith(
                expect.objectContaining({
                    approval_id: APPROVAL_ID,
                }),
                false,
                APPROVAL_ID
            );
        });

        it('uses executionId as container key when approval_id is missing', () => {
            const component = createTestComponent();
            const approvalRequest = makeApprovalRequestMessage({
                metadata: {
                    ...makeApprovalRequestMessage().metadata,
                    approval_id: undefined,
                },
            });
            
            component.restoreOperatorActivity(EXECUTION_ID, [approvalRequest]);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).toHaveBeenCalledWith(
                expect.any(Object),
                false,
                EXECUTION_ID
            );
        });

        it('restores command results when approval was granted', async () => {
            const component = createTestComponent();
            const messages = [
                makeApprovalRequestMessage(),
                makeApprovalGrantedMessage(),
                makeCommandResultMessage(),
            ];
            
            await component.restoreOperatorActivity(EXECUTION_ID, messages);
            
            expect(component.anchoredTerminal.restoreCommandResult).toHaveBeenCalledWith(
                APPROVAL_ID,
                expect.objectContaining({
                    command: messages[2].metadata.command,
                    content: messages[2].content,
                })
            );
        });

        it('does not restore command results when approval was rejected', () => {
            const component = createTestComponent();
            const messages = [
                makeApprovalRequestMessage(),
                makeApprovalRejectedMessage(),
                makeCommandResultMessage(),
            ];
            
            component.restoreOperatorActivity(EXECUTION_ID, messages);
            
            expect(component.anchoredTerminal.restoreCommandResult).not.toHaveBeenCalled();
        });

        it('does not restore command results when no approval decision', () => {
            const component = createTestComponent();
            const messages = [
                makeApprovalRequestMessage(),
                makeCommandResultMessage(),
            ];
            
            component.restoreOperatorActivity(EXECUTION_ID, messages);
            
            expect(component.anchoredTerminal.restoreCommandResult).not.toHaveBeenCalled();
        });

        it('handles multiple command results', async () => {
            const component = createTestComponent();
            const messages = [
                makeApprovalRequestMessage(),
                makeApprovalGrantedMessage(),
                makeCommandResultMessage(),
                makeCommandResultMessage({
                    content: 'Second result',
                    timestamp: new Date('2026-01-15T10:35:00Z').toISOString(),
                }),
            ];
            
            await component.restoreOperatorActivity(EXECUTION_ID, messages);
            
            expect(component.anchoredTerminal.restoreCommandResult).toHaveBeenCalledTimes(2);
        });

        it('uses approval request command when result command is missing', async () => {
            const component = createTestComponent();
            const approvalRequest = makeApprovalRequestMessage({
                metadata: {
                    ...makeApprovalRequestMessage().metadata,
                    command: 'rm -rf /tmp/test',
                },
            });
            const resultMessage = makeCommandResultMessage({
                metadata: {
                    ...makeCommandResultMessage().metadata,
                    command: undefined,
                },
            });
            const messages = [approvalRequest, makeApprovalGrantedMessage(), resultMessage];
            
            await component.restoreOperatorActivity(EXECUTION_ID, messages);
            
            const callArgs = component.anchoredTerminal.restoreCommandResult.mock.calls[0];
            expect(callArgs[1].command).toBe('rm -rf /tmp/test');
        });

        it('sets wasApproved to true for granted approval', () => {
            const component = createTestComponent();
            const messages = [
                makeApprovalRequestMessage(),
                makeApprovalGrantedMessage(),
            ];
            
            component.restoreOperatorActivity(EXECUTION_ID, messages);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).toHaveBeenCalledWith(
                expect.any(Object),
                true,
                expect.any(String)
            );
        });

        it('sets wasApproved to false for rejected approval', () => {
            const component = createTestComponent();
            const messages = [
                makeApprovalRequestMessage(),
                makeApprovalRejectedMessage(),
            ];
            
            component.restoreOperatorActivity(EXECUTION_ID, messages);
            
            expect(component.anchoredTerminal.restoreApprovalRequest).toHaveBeenCalledWith(
                expect.any(Object),
                false,
                expect.any(String)
            );
        });
    });

    describe('addRestoredMessage', () => {
        it('returns early when anchoredTerminal is missing', () => {
            const component = createTestComponent();
            component.anchoredTerminal = null;
            
            component.addRestoredMessage('test', EventType.EVENT_SOURCE_USER_CHAT, new Date());
            
            expect(component.anchoredTerminal).toBeNull();
        });

        it('formats timestamp for display when provided', () => {
            const component = createTestComponent();
            const timestamp = new Date('2026-01-15T10:30:45Z');
            
            component.addRestoredMessage('test', EventType.EVENT_SOURCE_USER_CHAT, timestamp);
            
            expect(component.anchoredTerminal.appendUserMessage).toHaveBeenCalledWith(
                'test',
                expect.stringMatching(/^\d{1,2}:\d{2}:\d{2}\s?[AP]M$/)
            );
        });

        it('passes null display time when timestamp is not provided', () => {
            const component = createTestComponent();
            
            component.addRestoredMessage('test', EventType.EVENT_SOURCE_USER_CHAT, null);
            
            expect(component.anchoredTerminal.appendUserMessage).toHaveBeenCalledWith(
                'test',
                null
            );
        });

        it('calls appendUserMessage for user chat sender', () => {
            const component = createTestComponent();
            const timestamp = new Date();
            
            component.addRestoredMessage('test message', EventType.EVENT_SOURCE_USER_CHAT, timestamp);
            
            expect(component.anchoredTerminal.appendUserMessage).toHaveBeenCalledWith(
                'test message',
                expect.any(String)
            );
        });

        it('calls appendDirectHtmlResponse for AI primary sender', () => {
            const component = createTestComponent();
            const timestamp = new Date();
            const groundingMetadata = { citations: ['cite1'] };
            
            component.addRestoredMessage('AI response', EventType.EVENT_SOURCE_AI_PRIMARY, timestamp, null, groundingMetadata);
            
            expect(component.messageRenderer.renderContent).toHaveBeenCalledWith('AI response');
            expect(component.anchoredTerminal.appendDirectHtmlResponse).toHaveBeenCalledWith(
                'AI response',
                expect.any(String),
                groundingMetadata
            );
        });

        it('calls appendSystemMessage for system sender', () => {
            const component = createTestComponent();
            const timestamp = new Date();
            
            component.addRestoredMessage('System message', EventType.EVENT_SOURCE_SYSTEM, timestamp);
            
            expect(component.anchoredTerminal.appendSystemMessage).toHaveBeenCalledWith('System message');
        });

        it('passes context info to terminal methods', () => {
            const component = createTestComponent();
            const contextInfo = {
                investigation_id: INVESTIGATION_ID,
                case_id: 'case123',
                event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER,
            };
            
            component.addRestoredMessage('test', EventType.EVENT_SOURCE_USER_CHAT, new Date(), contextInfo);
            
            expect(component.anchoredTerminal.appendUserMessage).toHaveBeenCalled();
        });
    });
});
