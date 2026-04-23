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

import { EventType, ToolDisplayCategory } from '../constants/events.js';
import { ApiPaths } from '../constants/api-paths.js';
import { decodeHtmlEntities } from '../utils/html.js';
import { notificationService } from '../utils/notification-service.js';

export const ChatSSEHandlersMixin = {
    shouldProcessEvent(data) {
        if (!data || !data.investigation_id) return false;
        const currentId = this.casesManager?.getCurrentInvestigationId();
        if (!currentId) return false;
        return data.investigation_id === currentId;
    },
    setupSSEListeners() {
        if (this._sseListenersRegistered) {
            console.warn('[CHAT] SSE listeners already registered, skipping duplicate registration');
            return;
        }

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_STARTED, (data) => {
            this.handleIterationStarted(data);
        });

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, (data) => {
            this.handleAITextChunk(data);
        });

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_COMPLETED, (data) => {
            this.handleTurnComplete(data);
        });

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED, (data) => {
            this.handleResponseComplete(data);
        });

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_FAILED, (data) => {
            this.handleChatError(data);
            this._handleLLMChatIterationFailed(data);
        });

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_RETRY, (data) => {
            this.handleChatRetry(data);
        });

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_TOOL_CALL_STARTED, (data) => {
            this.handleToolCallStarted(data);
        });

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_TOOL_CALL_COMPLETED, (data) => {
            this.handleToolCallCompleted(data);
        });

        this.eventBus.on(EventType.LLM_CHAT_ITERATION_STOPPED, (data) => {
            this.handleChatStopped(data);
        });

        // IMPORTANT: Do NOT subscribe to OPERATOR_NETWORK_PORT_CHECK_REQUESTED for UI
        // indicator creation. STARTED owns the indicator lifecycle.
        //
        // OPERATOR_NETWORK_PORT_CHECK_REQUESTED serves two distinct roles:
        //   1. MCP tool-call event dispatched to the operator (g8eo).
        //   2. A frontend notification emitted by agent_sse when the LLM's TOOL_CALL
        //      stream chunk is processed (see agent_sse.py).
        //
        // Although the execution_id on REQUESTED now matches the one used by
        // port_service for STARTED/COMPLETED/FAILED (both originate from
        // orchestrate_tool_execution's generate_command_execution_id), the TOOL_CALL
        // chunk is yielded by execute_turn_tool_calls AFTER it awaits the full
        // port-check, so REQUESTED arrives AFTER STARTED/COMPLETED. An indicator
        // created from REQUESTED would therefore never be completed.
        // Only STARTED/COMPLETED/FAILED drive the UI indicator lifecycle.
        this.eventBus.on(EventType.OPERATOR_NETWORK_PORT_CHECK_STARTED, (data) => {
            this.handleNetworkPortCheckIndicator(data);
        });

        this.eventBus.on(EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED, (data) => {
            this.handleNetworkPortCheckCompleted(data);
        });

        this.eventBus.on(EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED, (data) => {
            this.handleNetworkPortCheckFailed(data);
        });

        this.eventBus.on(EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED, (data) => {
            this.approvalPending = true;
            this.showAIStopButton();
            this.handleOperatorExecutionRequest(data);
        });

        this.eventBus.on(EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED, (data) => {
            this.approvalPending = true;
            this.showAIStopButton();
            this.handleOperatorExecutionRequest(data);
        });

        this.eventBus.on(EventType.OPERATOR_INTENT_APPROVAL_REQUESTED, (data) => {
            this.approvalPending = true;
            this.showAIStopButton();
            this.handleOperatorExecutionRequest(data);
        });

        this.eventBus.on(EventType.AI_AGENT_CONTINUE_APPROVAL_REQUESTED, (data) => {
            this.approvalPending = true;
            this.showAIStopButton();
            this.handleOperatorExecutionRequest(data);
        });

        this.eventBus.on(EventType.OPERATOR_COMMAND_APPROVAL_PREPARING, () => {
            this.approvalPending = true;
            this.showAIStopButton();
        });

        this.eventBus.on(EventType.OPERATOR_COMMAND_STARTED, () => {
            this.approvalPending = false;
            this.executionActive = true;
            this.showAIStopButton();
        });

        this.eventBus.on(EventType.OPERATOR_COMMAND_COMPLETED, (data) => {
            this.executionActive = false;
            this.approvalPending = false;
            this.hideAIStopButton();
            if (data.execution_id && this.anchoredTerminal) {
                this.anchoredTerminal.completeActivityIndicator(`fn-${data.execution_id}`);
            }
        });

        this.eventBus.on(EventType.OPERATOR_COMMAND_FAILED, (data) => {
            this.executionActive = false;
            this.approvalPending = false;
            this.hideAIStopButton();
            if (data.execution_id && this.anchoredTerminal) {
                this.anchoredTerminal.completeActivityIndicator(`fn-${data.execution_id}`);
            }
        });

        this.eventBus.on(EventType.OPERATOR_COMMAND_CANCELLED, (data) => {
            this.handleCommandCancelled(data);
        });

        this.eventBus.on(EventType.OPERATOR_FILE_EDIT_STARTED, () => {
            this.approvalPending = false;
            this.executionActive = true;
            this.showAIStopButton();
        });

        this.eventBus.on(EventType.OPERATOR_FILE_EDIT_COMPLETED, (data) => {
            this.executionActive = false;
            this.approvalPending = false;
            this.hideAIStopButton();
            if (data.execution_id && this.anchoredTerminal) {
                this.anchoredTerminal.completeActivityIndicator(`fn-${data.execution_id}`);
            }
        });

        this.eventBus.on(EventType.OPERATOR_FILE_EDIT_FAILED, (data) => {
            this.executionActive = false;
            this.approvalPending = false;
            this.hideAIStopButton();
            if (data.execution_id && this.anchoredTerminal) {
                this.anchoredTerminal.completeActivityIndicator(`fn-${data.execution_id}`);
            }
        });

        this.eventBus.on(EventType.TRIBUNAL_SESSION_STARTED, (data) => {
            this.handleTribunalStarted(data);
        });

        this.eventBus.on(EventType.TRIBUNAL_VOTING_PASS_COMPLETED, (data) => {
            this.handleTribunalPassCompleted(data);
        });

        this.eventBus.on(EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED, (data) => {
            this.handleTribunalVotingCompleted(data);
        });

        this.eventBus.on(EventType.TRIBUNAL_VOTING_REVIEW_STARTED, (data) => {
            this.handleTribunalAuditorStarted(data);
        });

        this.eventBus.on(EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED, (data) => {
            this.handleTribunalAuditorCompleted(data);
        });

        this.eventBus.on(EventType.TRIBUNAL_SESSION_COMPLETED, (data) => {
            this.handleTribunalCompleted(data);
        });

        const TRIBUNAL_TERMINAL_FAILURE_EVENTS = [
            EventType.TRIBUNAL_SESSION_DISABLED,
            EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED,
            EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
            EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
            EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
            EventType.TRIBUNAL_SESSION_VERIFIER_FAILED,
        ];
        for (const failureEvent of TRIBUNAL_TERMINAL_FAILURE_EVENTS) {
            this.eventBus.on(failureEvent, (data) => {
                this.handleTribunalSessionFailed(failureEvent, data);
            });
        }

        this.eventBus.on(EventType.LLM_CHAT_SUBMITTED, (payload) => {
            this.submitChatMessage(payload.message, {
                attachments: payload.attachments
            }).catch((error) => {
                console.error('[CHAT] Unhandled error from LLM_CHAT_SUBMITTED:', error);
            });
        });

        this.eventBus.on(EventType.LLM_CHAT_STOP_SHOW, () => this.showAIStopButton());
        this.eventBus.on(EventType.LLM_CHAT_STOP_HIDE, () => this.hideAIStopButton());

        this.eventBus.on(EventType.OPERATOR_TERMINAL_THINKING_APPEND, ({ webSessionId, text }) => {
            if (this.anchoredTerminal) {
                this.anchoredTerminal.appendThinkingContent(webSessionId, text);
            }
        });

        this.eventBus.on(EventType.OPERATOR_TERMINAL_THINKING_COMPLETE, ({ webSessionId }) => {
            if (this.anchoredTerminal) {
                this.anchoredTerminal.completeThinkingEntry(webSessionId);
            }
        });

        this.eventBus.on(EventType.CASE_SELECTED, (data) => {
            this.clearChat();
            this.handleCaseSelected(data);
        });

        this.eventBus.on(EventType.CASE_CREATED, (data) => {
            this.handleCaseCreated(data);
        });

        this.eventBus.on(EventType.CASE_CLEARED, () => {
            this.handleCaseCleared();
        });

        // SSE connection event listeners for reconnection handling
        this.eventBus.on(EventType.PLATFORM_SSE_CONNECTION_CLOSED, (data) => {
            this._handleSSEConnectionClosed(data);
        });

        this.eventBus.on(EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED, (data) => {
            this._handleSSEConnectionEstablished(data);
        });

        this._sseListenersRegistered = true;
    },

    async submitChatMessage(message, {
        attachments = []
    } = {}) {
        if (!this.currentUser) return;

        if (typeof message !== 'string') {
            throw new Error('[CHAT] submitChatMessage: message must be a string');
        }
        const trimmedMessage = message.trim();
        if (!trimmedMessage) return;

        const webSessionId = this.currentWebSessionId;
        if (!webSessionId) {
            throw new Error('[CHAT] Unable to determine authenticated session ID');
        }

        if (this.anchoredTerminal) {
            this.anchoredTerminal.appendUserMessage(trimmedMessage);
            this.anchoredTerminal.showWaitingIndicator(webSessionId);
        }

        if (!Array.isArray(attachments)) {
            throw new Error('[CHAT] submitChatMessage: attachments must be an array');
        }

        const chatPayload = {
            message: trimmedMessage,
            case_id: this.casesManager.getCurrentCaseId(),
            investigation_id: this.casesManager.getCurrentInvestigationId(),
            attachments,
            llm_primary_provider: this.llmModelManager.getPrimaryProvider(),
            llm_assistant_provider: this.llmModelManager.getAssistantProvider(),
            llm_lite_provider: this.llmModelManager.getLiteProvider(),
            llm_primary_model: this.llmModelManager.getPrimaryModel(),
            llm_assistant_model: this.llmModelManager.getAssistantModel(),
            llm_lite_model: this.llmModelManager.getLiteModel(),
        };

        try {
            const response = await this.serviceClient.post('g8ed', ApiPaths.chat.send(), chatPayload);

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

        } catch (error) {
            console.error('[CHAT] Error sending message:', error);
            if (this.thinkingManager) this.thinkingManager.hideThinkingIndicator(webSessionId);
            if (this.anchoredTerminal) {
                this.anchoredTerminal.hideWaitingIndicator();
                this.anchoredTerminal.appendErrorMessage(`Failed to send message: ${error.message}`);
            }
            throw error;
        } finally {
            if (this.anchoredTerminal) {
                this.anchoredTerminal.focus();
            }
        }
    },

    handleCaseCreated(data) {
        if (!data.case_id) {
            console.error('[CHAT] case.created event missing case_id:', data);
            notificationService.error('Case creation failed: missing case_id');
            return;
        }

        notificationService.success(`Case created: ${data.case_id}`);
    },

    handleCaseCleared() {
        this.clearChat();
    },

    handleIterationStarted(data) {
        if (!data.web_session_id) {
            return;
        }
        if (!this.shouldProcessEvent(data)) return;

        if (this.anchoredTerminal) {
            this.anchoredTerminal.showWaitingIndicator(data.web_session_id);
        }
    },

    handleAITextChunk(data) {
        if (!data.web_session_id) {
            return;
        }

        const textContent = data.content;
        if (!textContent) {
            return;
        }

        if (this.thinkingManager) {
            this.thinkingManager.hideThinkingIndicator(data.web_session_id);
        }

        this.streamingActive = true;
        this.showAIStopButton();

        if (!this.anchoredTerminal) return;

        // Reset auto-scroll exactly once per session on first chunk
        if (!this._hasResetAutoScrollForSession) {
            this._hasResetAutoScrollForSession = new Set();
        }
        if (!this._hasResetAutoScrollForSession.has(data.web_session_id)) {
            this.anchoredTerminal.resetAutoScroll();
            this._hasResetAutoScrollForSession.add(data.web_session_id);
        }

        this.anchoredTerminal.appendStreamingTextChunk(data.web_session_id, textContent);
    },

    handleNetworkPortCheckIndicator(data) {
        if (!this.shouldProcessEvent(data)) return;
        const webSessionId = data.web_session_id;
        if (!webSessionId || !this.anchoredTerminal) return;
        const executionId = data.execution_id;
        if (!executionId) return;
        const indicatorId = `port-check-${executionId}`;
        if (!this._portCheckIndicators) this._portCheckIndicators = new Map();
        this._portCheckIndicators.set(executionId, indicatorId);
        const port = data.port;
        if (port === undefined || port === null) {
            console.error('[CHAT] handleNetworkPortCheckIndicator: missing port in payload', data);
            return;
        }
        this.anchoredTerminal.appendActivityIndicator({
            id: indicatorId,
            icon: 'lan',
            label: 'Checking port',
            detail: port,
            category: ToolDisplayCategory.NETWORK,
        });
    },

    handleNetworkPortCheckCompleted(data) {
        if (!this.shouldProcessEvent(data)) return;
        if (!this.anchoredTerminal || !this._portCheckIndicators) return;
        const indicatorId = this._portCheckIndicators.get(data.execution_id);
        if (!indicatorId) return;
        this.anchoredTerminal.completeActivityIndicator(indicatorId);
        this._portCheckIndicators.delete(data.execution_id);
    },

    handleNetworkPortCheckFailed(data) {
        if (!this.shouldProcessEvent(data)) return;
        if (!this.anchoredTerminal || !this._portCheckIndicators) return;
        const indicatorId = this._portCheckIndicators.get(data.execution_id);
        if (!indicatorId) return;
        this.anchoredTerminal.completeActivityIndicator(indicatorId);
        this._portCheckIndicators.delete(data.execution_id);
    },

    handleTurnComplete(data) {
        if (!this.shouldProcessEvent(data)) return;
        const webSessionId = data.web_session_id;
        if (!webSessionId) return;

        if (this.streamingActive && this.anchoredTerminal) {
            this.anchoredTerminal.sealStreamingResponse(webSessionId);
            this.streamingActive = false;
        }
    },

    handleTribunalStarted(data) {
        if (!this.anchoredTerminal) return;

        const widgetId = `tribunal-${data.web_session_id}-${Date.now()}`;

        if (!this._tribunalWidgetIds) this._tribunalWidgetIds = new Map();
        this._tribunalWidgetIds.set(data.web_session_id, widgetId);

        this.anchoredTerminal.showTribunal({
            id: widgetId,
            model: data.model,
            numPasses: data.num_passes,
            request: data.request,
            guidelines: data.guidelines,
            webSessionId: data.web_session_id,
        });

        const webSessionId = data.web_session_id;
        if (webSessionId && this.streamingActive) {
            this.anchoredTerminal?.sealStreamingResponse(webSessionId);
            this.streamingActive = false;
            this.anchoredTerminal?.clearActivityIndicators();
            this._portCheckIndicators?.clear();
            this._hasResetAutoScrollForSession?.delete(webSessionId);
        }
    },

    handleTribunalPassCompleted(data) {
        console.log('[TRIBUNAL] Pass completed event received:', data);
        if (!this.anchoredTerminal || !this._tribunalWidgetIds) {
            console.log('[TRIBUNAL] Missing anchoredTerminal or _tribunalWidgetIds');
            return;
        }

        const widgetId = this._tribunalWidgetIds.get(data.web_session_id);
        if (!widgetId) {
            console.log('[TRIBUNAL] No widget ID found for web_session_id:', data.web_session_id);
            console.log('[TRIBUNAL] Available widget IDs:', Array.from(this._tribunalWidgetIds.entries()));
            return;
        }

        console.log('[TRIBUNAL] Updating pass:', widgetId, data.pass_index, data.success);
        this.anchoredTerminal.updateTribunalPass(widgetId, {
            passIndex: data.pass_index,
            success: data.success,
            candidate: data.candidate,
        });
    },

    handleTribunalVotingCompleted(data) {
        if (!this.anchoredTerminal || !this._tribunalWidgetIds) return;

        const widgetId = this._tribunalWidgetIds.get(data.web_session_id);
        if (!widgetId) return;

        this.anchoredTerminal.updateTribunalStatus(widgetId, 'Verifying command\u2026');
    },

    handleTribunalAuditorStarted(data) {
        if (!this.anchoredTerminal || !this._tribunalWidgetIds) return;

        const widgetId = this._tribunalWidgetIds.get(data.web_session_id);
        if (!widgetId) return;

        this.anchoredTerminal.updateTribunalStatus(widgetId, 'Verifying\u2026');
    },

    handleTribunalAuditorCompleted(data) {
        if (!this.anchoredTerminal || !this._tribunalWidgetIds) return;

        const widgetId = this._tribunalWidgetIds.get(data.web_session_id);
        if (!widgetId) return;

        const label = data.passed
            ? 'Command verified'
            : (data.reason === 'revised' ? 'Command revised' : 'Verification complete');
        this.anchoredTerminal.updateTribunalStatus(widgetId, label);
    },

    handleTribunalCompleted(data) {
        if (!this.anchoredTerminal || !this._tribunalWidgetIds) return;

        const widgetId = this._tribunalWidgetIds.get(data.web_session_id);
        if (!widgetId) return;

        this.anchoredTerminal.completeTribunal({
            id: widgetId,
            finalCommand: data.final_command,
            outcome: data.outcome,
        });

        this._tribunalWidgetIds.delete(data.web_session_id);
    },

    handleTribunalSessionFailed(eventType, data) {
        if (!this.anchoredTerminal || !this._tribunalWidgetIds) return;

        const widgetId = this._tribunalWidgetIds.get(data.web_session_id);
        if (!widgetId) return;

        this.anchoredTerminal.failTribunal({
            id: widgetId,
            eventType,
        });

        this._tribunalWidgetIds.delete(data.web_session_id);
    },

    handleResponseComplete(data) {
        if (!data || !data.web_session_id) return;
        if (!this.shouldProcessEvent(data)) return;

        const webSessionId = data.web_session_id;

        this.anchoredTerminal?.hideWaitingIndicator();
        this.anchoredTerminal?.clearActivityIndicators();
        this._portCheckIndicators?.clear();
        this._searchWebIndicators?.clear();
        this._hasResetAutoScrollForSession?.delete(webSessionId);

        if (this.anchoredTerminal && data.content) {
            if (!this.markdownRenderer) {
                throw new Error('[CHAT] markdownRenderer is not initialized');
            }
            const decoded = decodeHtmlEntities(data.content);
            const html = this.markdownRenderer.parseMarkdown(decoded, false);
            const groundingMetadata = (data.grounding_metadata?.grounding_used) ? data.grounding_metadata : null;
            this.anchoredTerminal.finalizeAIResponseChunk(webSessionId, html, groundingMetadata);
        }

        this.streamingActive = false;

        const thinkingActive = this.thinkingManager?.thinkingActive ?? false;
        if (!this.executionActive && !this.approvalPending && !thinkingActive) {
            this.hideAIStopButton();
        }
    },

    handleChatStopped(data = {}) {
        if (!this.shouldProcessEvent(data)) return;
        const webSessionId = data.web_session_id;

        if (this.anchoredTerminal) {
            this.anchoredTerminal.hideWaitingIndicator();
            if (webSessionId) {
                const entry = document.getElementById(`ai-response-${webSessionId}`);
                if (entry) {
                    entry.classList.remove('streaming');
                    entry.querySelectorAll('.streaming-cursor').forEach(c => c.remove());
                }
            }
        }

        this.streamingActive = false;
        this.approvalPending = false;
        if (!this.executionActive) {
            this.hideAIStopButton();
        }
    },

    handleChatError(data = {}) {
        if (!this.shouldProcessEvent(data)) return;
        const webSessionId = data.web_session_id;

        if (this.anchoredTerminal && webSessionId) {
            const entry = document.getElementById(`ai-response-${webSessionId}`);
            if (entry) {
                entry.classList.remove('streaming');
                entry.querySelectorAll('.streaming-cursor').forEach(c => c.remove());
            }
        }

        if (!data.event_type) {
            data.event_type = EventType.LLM_CHAT_ITERATION_FAILED;
        }

        const errorText = typeof data.error === 'string' && data.error.trim() ? data.error.trim() : 'The AI session encountered an error.';
        const rawError = typeof data.raw_error === 'string' && data.raw_error.trim() ? data.raw_error.trim() : null;
        const attemptsMeta = data.metadata && typeof data.metadata === 'object' ? data.metadata : null;

        const lines = [errorText];
        if (rawError && rawError !== errorText) {
            lines.push(`Details: ${rawError}`);
        }
        if (attemptsMeta && typeof attemptsMeta.attempts === 'number') {
            const attempts = attemptsMeta.attempts;
            const maxAttempts = typeof attemptsMeta.max_attempts === 'number' ? attemptsMeta.max_attempts : null;
            const attemptLine = maxAttempts ? `Retries attempted: ${attempts} of ${maxAttempts}` : `Retries attempted: ${attempts}`;
            lines.push(attemptLine);

            if (typeof attemptsMeta.initial_backoff_seconds === 'number' && typeof attemptsMeta.backoff_multiplier === 'number') {
                lines.push(`Retry backoff started at ${attemptsMeta.initial_backoff_seconds}s (x${attemptsMeta.backoff_multiplier} multiplier).`);
            }
        }

        const message = lines.join('\n');

        if (this.anchoredTerminal && this.currentUser) {
            this.anchoredTerminal.appendErrorMessage(message);
        } else {
            this.addSystemMessage(message, 'error');
        }
    },

    handleChatRetry(data) {
        if (!this.shouldProcessEvent(data)) return;
        if (!this.anchoredTerminal) return;

        const attempt = data.attempt || 0;
        const maxAttempts = data.max_attempts || 0;
        const message = `Retrying (attempt ${attempt}/${maxAttempts})...`;

        this.anchoredTerminal.appendSystemMessage(message);
    },

    handleToolCallStarted(data) {
        if (!this.shouldProcessEvent(data)) return;
        if (!this.anchoredTerminal) return;

        const executionId = data.execution_id;
        if (!executionId) return;

        // check_port_status has a dedicated sidecar indicator driven by
        // OPERATOR_NETWORK_PORT_CHECK_STARTED/COMPLETED/FAILED from port_service
        // (which fires in the correct order relative to this generic STARTED event).
        const toolName = data.tool_name;
        if (toolName === 'check_port_status') {
            return;
        }

        const indicatorId = `tool-${executionId}`;
        if (!this._toolCallIndicators) this._toolCallIndicators = new Map();
        this._toolCallIndicators.set(executionId, indicatorId);

        this.anchoredTerminal.appendActivityIndicator({
            id: indicatorId,
            icon: data.display_icon || 'tool',
            label: data.display_label || 'Processing',
            detail: data.display_detail,
            category: data.category,
        });
    },

    handleToolCallCompleted(data) {
        if (!this.shouldProcessEvent(data)) return;
        if (!this.anchoredTerminal || !this._toolCallIndicators) return;

        const indicatorId = this._toolCallIndicators.get(data.execution_id);
        if (!indicatorId) return;

        this.anchoredTerminal.completeActivityIndicator(indicatorId);
        this._toolCallIndicators.delete(data.execution_id);
    },

    handleCommandCancelled(data) {
        if (!this.shouldProcessEvent(data)) return;

        this.executionActive = false;
        this.approvalPending = false;
        this.hideAIStopButton();

        if (data.execution_id && this.anchoredTerminal) {
            const execId = data.execution_id;
            this.anchoredTerminal.completeActivityIndicator(`fn-${execId}`);
            this.anchoredTerminal.completeActivityIndicator(`tool-${execId}`);
        }
    },

};

export const ChatOperatorExecutionMixin = {
    handleOperatorExecutionRequest(data) {
        if (!this.shouldProcessEvent(data)) return;
        if (!data) return;

        const webSessionId = data.web_session_id;

        if (this.thinkingManager) {
            this.thinkingManager.hideThinkingIndicator(webSessionId);
        }
    },
};
