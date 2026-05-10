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

import { OperatorDeployment } from './operator-deployment.js';
import { nowISOString, formatForDisplay } from '../utils/timestamp.js';
import { TerminalScrollMixin } from './anchored-terminal-scroll.js';
import { TerminalOperatorMixin } from './anchored-terminal-operator.js';
import { TerminalOutputMixin } from './anchored-terminal-output.js';
import { TerminalExecutionMixin } from './anchored-terminal-execution.js';
import { EventType } from '../constants/events.js';
import { ApiPaths } from '../constants/api-paths.js';

function applyMixins(target, ...mixins) {
    for (const mixin of mixins) {
        for (const key of Object.getOwnPropertyNames(mixin.prototype)) {
            if (key !== 'constructor') {
                Object.defineProperty(
                    target.prototype,
                    key,
                    Object.getOwnPropertyDescriptor(mixin.prototype, key)
                );
            }
        }
    }
}

export class AnchoredOperatorTerminal {
    constructor(eventBus) {
        this.eventBus = eventBus;

        this.container = null;
        this.terminal = null;
        this.outputContainer = null;
        this.inputElement = null;
        this.sendButton = null;
        this.hostnameElement = null;
        this.promptElement = null;

        this.commandHistory = [];
        this.historyIndex = -1;
        this.activeStreamingResponses = new Map();

        this.currentUser = null;
        this.isAuthenticated = false;

        this.attachments = null;
        this.attachmentsUI = null;

        this.thinkingContentRaw = new Map();

        this._escapeDiv = document.createElement('div');
        this._eventsBound = false;
        this._initialized = false;

        this.initScrollState();
        this.initOperatorState();
        this.initExecutionState();
    }

    init() {
        if (this._initialized) return;

        this.cacheDOMReferences();
        if (!this.container) {
            console.warn('[ANCHORED TERMINAL] Container not found, skipping initialization');
            return;
        }

        this.bindDOMEvents();
        this.bindScrollListener();
        this.bindEventBusListeners();
        this.showWelcomeMessage();

        this._initialized = true;
    }

    cacheDOMReferences() {
        this.container = document.getElementById('anchored-terminal-container');
        this.terminal = document.getElementById('anchored-terminal');
        this.outputContainer = document.getElementById('anchored-terminal-output');
        this.inputElement = document.getElementById('anchored-terminal-input');
        this.sendButton = document.getElementById('anchored-terminal-send');
        this.hostnameElement = document.getElementById('anchored-terminal-hostname');
        this.promptElement = document.getElementById('anchored-terminal-prompt');
        this.attachmentButton = document.getElementById('anchored-terminal-attach');
        this.attachmentsDisplay = document.getElementById('anchored-terminal-attachments');
        this.modeIndicator = document.getElementById('anchored-terminal-mode');
        this.resizeHandle = document.getElementById('panel-resize-handle');
        this.maximizeButton = document.getElementById('anchored-terminal-maximize');
        this.inputArea = this.terminal?.querySelector('.anchored-terminal__input-area');
        this.scrollContainer = document.getElementById('anchored-terminal-body');
    }

    bindDOMEvents() {
        if (!this.inputElement) return;

        this.inputElement.addEventListener('input', () => {
            this.updateModeIndicator();
        });

        this.inputElement.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.executeCommand();
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                this.navigateHistory(-1);
            } else if (e.key === 'ArrowDown') {
                e.preventDefault();
                this.navigateHistory(1);
            }
        });

        if (this.sendButton) {
            this.sendButton.addEventListener('click', () => {
                this.executeCommand();
            });
        }

        if (this.resizeHandle) {
            this.resizeHandle.addEventListener('mousedown', (e) => {
                this.startResize(e);
            });
        }

        if (this.maximizeButton) {
            this.maximizeButton.addEventListener('click', () => {
                this.toggleMaximize();
            });
        }
    }

    toggleMaximize() {
        if (!this.inputArea || !this.maximizeButton) return;

        const isMaximized = this.inputArea.classList.toggle('maximized');
        this.maximizeButton.classList.toggle('maximized', isMaximized);
        this.maximizeButton.title = isMaximized ? 'Collapse input area' : 'Expand input area';

        if (this.inputElement) {
            this.inputElement.focus();
        }
    }

    openFilePicker() {
        if (this.attachmentsUI?.fileInput) {
            this.attachmentsUI.fileInput.click();
        }
    }

    async executeCommand() {
        if (!this.inputElement) return;

        const input = this.inputElement.value.trim();
        if (!input) return;

        this.resetAutoScroll();

        this.commandHistory.push(input);
        this.historyIndex = this.commandHistory.length;

        this.inputElement.value = '';
        this.updateModeIndicator();

        if (input.startsWith('/run ')) {
            const command = input.substring(5).trim();
            if (command) {
                await this.executeDirectCommand(command);
            }
            return;
        }

        await this.sendChatMessage(input);
    }

    async sendChatMessage(message) {
        if (!this.currentUser) {
            this.appendSystemMessage('Please sign in to send messages');
            return;
        }

        const primaryModel = window.llmModelManager?.getPrimaryModel() || '';
        const assistantModel = window.llmModelManager?.getAssistantModel() || '';

        if (!primaryModel || !assistantModel) {
            if (!primaryModel && !assistantModel) {
                this.appendSystemMessage('Please select both a primary and assistant model before sending a message');
            } else if (!primaryModel) {
                this.appendSystemMessage('Please select a primary model before sending a message');
            } else {
                this.appendSystemMessage('Please select an assistant model before sending a message');
            }
            return;
        }

        try {
            const attachments = this.attachmentsUI?.manager
                ? this.attachmentsUI.manager.getFormattedForBackend()
                : [];

            this.eventBus.emit(EventType.LLM_CHAT_SUBMITTED, {
                message,
                attachments
            });

            if (this.attachmentsUI?.manager) {
                this.attachmentsUI.manager.clearAll();
            }

        } catch (error) {
            console.warn('[ANCHORED TERMINAL] Failed to send chat message:', error.message);
            this.appendSystemMessage(`Error: ${error.message}`);
        }
    }

    async executeDirectCommand(command) {
        if (!this.isOperatorBound) {
            this.appendSystemMessage('No Operator bound. Use /run only when an Operator is connected.');
            return;
        }

        try {
            const webSessionId = window.authState?.getWebSessionId();
            if (!webSessionId) {
                this.appendSystemMessage('Error: No active session. Please sign in.');
                return;
            }

            const executingId = this.showExecutingIndicator(command);
            const executionId = `direct_${Date.now()}_${Math.random().toString(36).substring(2, 9)}`;

            const response = await window.serviceClient.post('g8ed', ApiPaths.approval.directCommand(), {
                command: command,
                execution_id: executionId,
                operator_id: this.boundOperator?.operator_id,
                web_session_id: webSessionId,
                hostname: this.boundOperator?.latest_heartbeat_snapshot?.system_identity?.hostname || this.boundOperator?.name || null
            });

            const result = await response.json();

            this.hideExecutingIndicator(executingId);

            if (result.success) {
                if (result.execution_id) {
                    this.activeExecutions.set(result.execution_id, {
                        command,
                        startedAt: Date.now()
                    });
                }
            } else {
                const containerId = `direct-${Date.now()}`;
                const container = this._createResultsContainer(containerId);
                if (container) {
                    this._appendResultToContainer(container, {
                        command,
                        stdout: '',
                        stderr: result.error || 'Command execution failed',
                        exitCode: 1,
                        status: 'failed',
                        timestamp: nowISOString()
                    });
                }
            }

        } catch (error) {
            console.warn('[ANCHORED TERMINAL] Command execution failed:', error.message);
            const containerId = `direct-${Date.now()}`;
            const container = this._createResultsContainer(containerId);
            if (container) {
                this._appendResultToContainer(container, {
                    command,
                    stdout: '',
                    stderr: error.message,
                    exitCode: 1,
                    status: 'failed',
                    timestamp: nowISOString()
                });
            }
        }
    }

    navigateHistory(direction) {
        if (this.commandHistory.length === 0) return;

        this.historyIndex += direction;

        if (this.historyIndex < 0) {
            this.historyIndex = 0;
        } else if (this.historyIndex >= this.commandHistory.length) {
            this.historyIndex = this.commandHistory.length;
            if (this.inputElement) {
                this.inputElement.value = '';
            }
            return;
        }

        if (this.inputElement && this.commandHistory[this.historyIndex]) {
            this.inputElement.value = this.commandHistory[this.historyIndex];
            this.inputElement.selectionStart = this.inputElement.value.length;
        }
    }

    async showWelcomeMessage() {
        if (!this.outputContainer) return;

        const welcome = document.createElement('div');
        welcome.className = 'anchored-terminal__welcome';
        this.outputContainer.appendChild(welcome);

        this._deploymentComponent = new OperatorDeployment({
            onCommandReady: (cmd) => {}
        });
        await this._deploymentComponent.mount(welcome, this.currentUser);
    }

    formatTimestamp() {
        return formatForDisplay(new Date());
    }

    escapeHtml(text) {
        if (!text) return '';
        this._escapeDiv.textContent = text;
        return this._escapeDiv.innerHTML;
    }

    show() {
        if (this.container) {
            this.container.classList.remove('hidden');
        }
    }

    hide() {
        if (this.container) {
            this.container.classList.add('hidden');
        }
    }

    focus() {
        if (this.inputElement && !this.inputElement.disabled) {
            this.inputElement.focus();
        }
    }

    setUser(user) {
        this.currentUser = user;
        this.isAuthenticated = !!user;
        this.updateInputState();
        if (this._deploymentComponent) {
            this._deploymentComponent.setUser(user);
        }
    }

    setAttachmentsUI(attachmentsUI) {
        this.attachmentsUI = attachmentsUI;
    }

    updateInputState() {
        if (!this.inputElement) return;

        if (this.isAuthenticated) {
            this.inputElement.disabled = false;
            this.inputElement.placeholder = 'Start a conversation or use /run <command> for direct execution.';
            if (this.sendButton) {
                this.sendButton.disabled = false;
            }
            if (this.attachmentButton) {
                this.attachmentButton.disabled = false;
            }
        } else {
            this.inputElement.disabled = true;
            this.inputElement.placeholder = 'Sign in to start chatting...';
            if (this.sendButton) {
                this.sendButton.disabled = true;
            }
            if (this.attachmentButton) {
                this.attachmentButton.disabled = true;
            }
        }
    }

    updateModeIndicator() {
        if (!this.modeIndicator || !this.inputElement) return;

        const value = this.inputElement.value;

        if (value.startsWith('/run')) {
            this.modeIndicator.textContent = '/run';
            this.modeIndicator.classList.add('cli-mode');
            this.modeIndicator.classList.remove('chat-mode');
            this.inputElement.classList.add('cli-mode');
        } else {
            this.modeIndicator.textContent = 'Chat';
            this.modeIndicator.classList.add('chat-mode');
            this.modeIndicator.classList.remove('cli-mode');
            this.inputElement.classList.remove('cli-mode');
        }
    }

    enable() {
        this.updateInputState();
    }

    disable() {
        if (this.inputElement) {
            this.inputElement.disabled = true;
            this.inputElement.placeholder = 'Sign in to start chatting...';
        }
        if (this.sendButton) {
            this.sendButton.disabled = true;
        }
    }

    getValue() {
        return this.inputElement?.value;
    }

    setValue(value) {
        if (this.inputElement) {
            this.inputElement.value = value;
            this.updateModeIndicator();
        }
    }

    clear() {
        if (this.inputElement) {
            this.inputElement.value = '';
            this.updateModeIndicator();
        }
        if (this.attachmentsUI?.manager) {
            this.attachmentsUI.manager.clearAll();
        }
    }

    clearOutput() {
        this._cancelPendingTimers();

        if (this._deploymentComponent) {
            this._deploymentComponent.destroy();
            this._deploymentComponent = null;
        }

        if (this.outputContainer) {
            this.outputContainer.innerHTML = '';
        }

        this.activeStreamingResponses.clear();
        this.pendingApprovals.clear();
        this.activeExecutions.clear();
        this.thinkingContentRaw.clear();
        this.executionResultsContainers.clear();
        this.clearStreamingAccumulator();

        if (this.currentUser) {
            this.showWelcomeMessage();
        }
    }
}

applyMixins(
    AnchoredOperatorTerminal,
    TerminalScrollMixin,
    TerminalOperatorMixin,
    TerminalOutputMixin,
    TerminalExecutionMixin
);
