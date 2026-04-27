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
import { ApiPaths } from '../constants/api-paths.js';
import { AnchoredOperatorTerminal } from './anchored-terminal.js';
import { LlmModelManager } from './llm-model-manager.js';
import { AttachmentsManager, CompactAttachmentsUI } from '../utils/attachments.js';
import { ThinkingManager } from './thinking.js';
import { ScrollDelegation } from '../utils/scroll-delegation.js';
import { notificationService } from '../utils/notification-service.js';
import { templateLoader } from '../utils/template-loader.js';
import { MarkdownRenderer } from '../utils/markdown.js';
import { MessageRenderer } from './message-renderer.js';
import { CasesManager } from './cases-manager.js';
import { TriageQuestionsPopup } from './triage-questions-popup.js';
import { ChatAuthMixin } from './chat-auth.js';
import { ChatSSEHandlersMixin, ChatOperatorExecutionMixin } from './chat-sse-handlers.js';
import { ChatHistoryMixin } from './chat-history.js';
import { debounce } from '../utils/debounce.js';

export class ChatComponent {
    constructor(eventBus) {
        this.eventBus = eventBus;

        this.serviceClient = window.serviceClient;

        this.authStateUnsubscribe = null;
        this.webSessionModel = window.authState.getWebSessionModel();
        this.currentUser = this.webSessionModel;
        this.currentWebSessionId = this.webSessionModel?.id || null;

        this.streamingActive = false;
        this.executionActive = false;
        this.approvalPending = false;

        // Streaming content management
        this.pendingCitations = new Map();
        this.streamingContent = new Map();
        this._debouncedRenderChunk = debounce(this._renderChunk.bind(this), 100);
        this._hasResetAutoScrollForSession = new Set();

        this.casesManager = null;

        this.llmModelManager = new LlmModelManager(this.eventBus);
        window.llmModelManager = this.llmModelManager;

        this.attachments = new AttachmentsManager({
            maxFileSize: 10 * 1024 * 1024,
            maxFiles: 10
        });
        this.attachmentsUI = null;

        this.thinkingManager = null;
        this.anchoredTerminal = null;

        this.container = null;
        this.messagesContainer = null;

        this.scrollDelegation = null;

        this.markdownRenderer = new MarkdownRenderer();
        this.messageRenderer = new MessageRenderer(this.markdownRenderer);

        this.triageQuestionsPopup = new TriageQuestionsPopup(this.eventBus, document.body);

        // Reconnection state
        this._reconnectAttempts = 0;
        this._maxReconnectAttempts = 5;
        this._reconnectDelay = 1000; // Start with 1 second
        this._reconnectTimer = null;
    }

    init() {
        this.messageRenderer.setupCopyButtonListeners();
        this.subscribeToAuthState();
        this.waitForAuthStateInitialization();
    }

    showAIStopButton() {
        if (this.aiStopBtn) {
            this.aiStopBtn.disabled = false;
            this.aiStopBtn.title = 'Stop AI Response';
        }
    }

    hideAIStopButton() {
        if (this.aiStopBtn) {
            this.aiStopBtn.disabled = true;
            this.aiStopBtn.title = 'No active AI response to stop';
        }
    }

    async stopAIProcessing(options = {}) {
        const { silent = false, reason = 'User requested stop' } = options;
        const investigationId = this.casesManager.getCurrentInvestigationId();

        if (!investigationId) {
            if (!silent) console.warn('[CHAT] No active investigation to stop');
            return false;
        }

        try {
            const response = await this.serviceClient.post('g8ed', ApiPaths.chat.stop(), {
                investigation_id: investigationId,
                reason: reason
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            await response.json();

            if (this.approvalPending) {
                this.eventBus.emit(EventType.OPERATOR_TERMINAL_APPROVAL_DENIED, { reason, statusMessage: 'Cancelled' });
                this.approvalPending = false;
            }

            this.streamingActive = false;
            this.executionActive = false;

            if (this.thinkingManager) {
                this.thinkingManager.hideThinkingIndicator(this.currentWebSessionId);
            }

            this.anchoredTerminal?.clearActivityIndicators();
            this._portCheckIndicators?.clear();
            this._searchWebIndicators?.clear();
            this.pendingCitations.delete(this.currentWebSessionId);
            this.hideAIStopButton();

            if (!silent) {
                notificationService.info('AI response stopped by user');
            }

            return true;

        } catch (error) {
            console.error('[CHAT] Failed to stop AI processing:', error);
            if (!silent) {
                const msg = error instanceof Error ? error.message : 'Unknown error';
                notificationService.error(`Failed to stop AI: ${msg}`);
            }
            return false;
        }
    }

    async render() {
        const existingContainer = document.querySelector('[data-component="chat"]');
        if (existingContainer) {
            this.container = existingContainer;
            this.container.id = 'chat-container';
            this.container.className = 'chat-container';
        } else {
            this.container = document.createElement('div');
            this.container.id = 'chat-container';
            this.container.className = 'chat-container';
            document.querySelector('.main-content')?.appendChild(this.container);
        }

        if (!this.currentUser) {
            return;
        }

        const chatContainerHTML = await templateLoader.load('chat-container');
        this.container.innerHTML = chatContainerHTML;

        this.bindDOMEvents();

        this.thinkingManager = new ThinkingManager(this.eventBus, this.messagesContainer, this.markdownRenderer);

        this.anchoredTerminal = new AnchoredOperatorTerminal(this.eventBus);
        this.anchoredTerminal.markdownRenderer = this.markdownRenderer;
        this.anchoredTerminal.citationsHandler = this.messageRenderer.citationsHandler;
        this.anchoredTerminal.init();

        const attachmentsDisplay = document.getElementById('anchored-terminal-attachments');
        if (attachmentsDisplay) {
            this.attachmentsUI = new CompactAttachmentsUI(this.attachments, attachmentsDisplay);
            const attachmentButton = document.getElementById('anchored-terminal-attach');
            if (attachmentButton) {
                this.attachmentsUI.createAttachButton(attachmentButton);
            }
            this.anchoredTerminal.setAttachmentsUI(this.attachmentsUI);
        }

        this.anchoredTerminal.disable();
    }

    bindDOMEvents() {
        this.messagesContainer = document.getElementById('messages-container');
        this.aiStopBtn = document.getElementById('ai-stop-btn');

        if (this.aiStopBtn) {
            this.aiStopBtn.addEventListener('click', () => this.stopAIProcessing());
        }

        this.llmModelManager.init();

        const scrollContainer = document.getElementById('anchored-terminal-body');
        if (scrollContainer) {
            this.scrollDelegation = new ScrollDelegation(scrollContainer);
            this.scrollDelegation.enable();
        }
    }

    clearChat() {
        if (this.messagesContainer) {
            this.messagesContainer.innerHTML = '';
        }

        if (this.thinkingManager) {
            this.thinkingManager.clearAllThinkingData();
        }

        this.streamingActive = false;
        this.executionActive = false;
        this.approvalPending = false;
        this.hideAIStopButton();
        this._portCheckIndicators?.clear();
        this._searchWebIndicators?.clear();
        this._processingIndicators?.clear();
        this.pendingCitations.clear();
        this.streamingContent.clear();
        this._hasResetAutoScrollForSession.clear();

        if (this.anchoredTerminal) {
            this.anchoredTerminal.clearOutput();
        }
    }

    destroy() {
        if (this.thinkingManager) {
            this.thinkingManager.destroy();
        }

        if (this.container && this.container.parentNode) {
            this.container.parentNode.removeChild(this.container);
        }
    }

    addSystemMessage(message, type = 'info') {
        notificationService.show(message, type);
    }

    initCasesManager() {
        this.casesManager = new CasesManager(this.eventBus);
        this.casesManager.init();
        window.casesManager = this.casesManager;
    }

    attemptReconnect() {
        if (this._reconnectAttempts >= this._maxReconnectAttempts) {
            console.warn('[CHAT] Max reconnection attempts reached');
            return;
        }

        this._reconnectAttempts++;
        console.log(`[CHAT] Reconnection attempt ${this._reconnectAttempts}/${this._maxReconnectAttempts}`);

        // Clear any existing timer
        if (this._reconnectTimer) {
            clearTimeout(this._reconnectTimer);
        }

        // Schedule reconnection attempt
        this._reconnectTimer = setTimeout(() => {
            // Emit reconnection attempt event (this would be handled by SSE client)
            this.eventBus.emit('CHAT_RECONNECTION_ATTEMPTED', {
                web_session_id: this.currentWebSessionId,
                attempt: this._reconnectAttempts
            });
        }, this._reconnectDelay);

        // Exponential backoff
        this._reconnectDelay = Math.min(this._reconnectDelay * 2, 30000); // Max 30 seconds
    }

    _handleSSEConnectionClosed(data) {
        if (!data || !data.web_session_id || data.web_session_id !== this.currentWebSessionId) {
            return;
        }

        console.log('[CHAT] SSE connection closed, attempting reconnection');
        
        // Show error message to user
        if (this.anchoredTerminal) {
            this.anchoredTerminal.appendErrorMessage('Connection lost. Attempting to reconnect...');
        }

        // Start reconnection attempts
        this.attemptReconnect();
    }

    _handleSSEConnectionEstablished(data) {
        if (!data || !data.web_session_id || data.web_session_id !== this.currentWebSessionId) {
            return;
        }

        console.log('[CHAT] SSE connection re-established');
        
        // Reset reconnection state
        this._reconnectAttempts = 0;
        this._reconnectDelay = 1000;
        if (this._reconnectTimer) {
            clearTimeout(this._reconnectTimer);
            this._reconnectTimer = null;
        }

        // Show success message to user
        if (this.anchoredTerminal) {
            this.anchoredTerminal.appendSystemMessage('Connection re-established');
        }
    }

    _handleLLMChatIterationFailed(data) {
        if (!data || !data.web_session_id || data.web_session_id !== this.currentWebSessionId) {
            return;
        }

        console.error('[CHAT] LLM chat iteration failed:', data.error);

        if (this.anchoredTerminal) {
            this.anchoredTerminal.hideWaitingIndicator();
        }
        if (this.thinkingManager) {
            this.thinkingManager.hideThinkingIndicator(data.web_session_id);
        }

        this._debouncedRenderChunk?.cancel();
        this.streamingContent.delete(this.currentWebSessionId);
        this._hasResetAutoScrollForSession.delete(this.currentWebSessionId);
        this.streamingActive = false;
        this.hideAIStopButton();
    }

    _renderChunk() {
        // Placeholder for debounced render chunk logic
        // This can be implemented when actual debounced rendering is needed
    }

}

Object.assign(ChatComponent.prototype, ChatAuthMixin, ChatSSEHandlersMixin, ChatHistoryMixin);

const _operatorBase = Object.create(Object.getPrototypeOf(ChatComponent.prototype));
Object.assign(_operatorBase, ChatOperatorExecutionMixin);
Object.setPrototypeOf(ChatComponent.prototype, _operatorBase);
