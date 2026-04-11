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

/**
 * ThinkingManager - Handles AI thinking process visualization
 * 
 * Features:
 * - Real-time thinking indicators during AI processing
 * - Persistent Analysis Process blocks with collapsible content
 * - WebSession-based thinking data management
 * - Markdown formatting for thinking content
 * - UX-optimized display (only shows when there are actual thoughts)
 */

import { EventType, ThinkingActionType } from '../constants/events.js';
import { ThinkingEvent } from '../models/ai-event-models.js';
import { decodeHtmlEntities } from '../utils/html.js';

export class ThinkingManager {
    constructor(eventBus, messagesContainer, markdownRenderer) {
        this.eventBus = eventBus;
        this.messagesContainer = messagesContainer;
        this.markdownRenderer = markdownRenderer;

        this.thinkingActive = false;
        this.activeSessions = new Set();

        this.setupEventListeners();
    }

    setupEventListeners() {
        this.eventBus.on(EventType.LLM_CHAT_ITERATION_THINKING_STARTED, (raw) => {
            if (this._isFiltered()) return;
            const data = ThinkingEvent.parse(raw);

            if (data.action_type === ThinkingActionType.START) {
                this.handleThinkingStart(data);
            } else if (data.action_type === ThinkingActionType.END) {
                this.handleThinkingEnd(data);
            } else {
                this.handleThinkingUpdate(data);
            }
        });
    }

    _isFiltered() {
        let rejected = false;
        this.eventBus.emit(EventType.LLM_CHAT_FILTER_EVENT, { reject: () => { rejected = true; } });
        return rejected;
    }

    handleThinkingStart(data) {
        if (!data.web_session_id) {
            console.warn('[THINKING] Invalid thinking start data - missing web_session_id');
            return;
        }

        const webSessionId = data.web_session_id;
        const text = this.getThinkingText(data);

        this.activeSessions.add(webSessionId);
        this.thinkingActive = true;

        this.eventBus.emit(EventType.LLM_CHAT_STOP_SHOW);
        if (text) {
            this.eventBus.emit(EventType.OPERATOR_TERMINAL_THINKING_APPEND, { webSessionId, text });
        }
    }

    handleThinkingUpdate(data) {
        if (!data.web_session_id) {
            console.warn('[THINKING] Invalid thinking update data - missing web_session_id');
            return;
        }

        const webSessionId = data.web_session_id;
        const text = this.getThinkingText(data);

        this.activeSessions.add(webSessionId);
        this.thinkingActive = true;

        this.eventBus.emit(EventType.LLM_CHAT_STOP_SHOW);
        if (text) {
            this.eventBus.emit(EventType.OPERATOR_TERMINAL_THINKING_APPEND, { webSessionId, text });
        }
    }

    handleThinkingEnd(data) {
        if (!data.web_session_id) {
            console.warn('[THINKING] Invalid thinking end data - missing web_session_id');
            return;
        }

        const webSessionId = data.web_session_id;
        const text = this.getThinkingText(data);

        if (text) {
            this.eventBus.emit(EventType.OPERATOR_TERMINAL_THINKING_APPEND, { webSessionId, text });
        }
        this.eventBus.emit(EventType.OPERATOR_TERMINAL_THINKING_COMPLETE, { webSessionId });

        this.activeSessions.delete(webSessionId);
        this.thinkingActive = this.activeSessions.size > 0;

        this.eventBus.emit(EventType.LLM_CHAT_STOP_HIDE);
    }

    hideThinkingIndicator(webSessionId = null) {
        if (webSessionId) {
            this.activeSessions.delete(webSessionId);
            this.eventBus.emit(EventType.OPERATOR_TERMINAL_THINKING_COMPLETE, { webSessionId });
        } else {
            this.activeSessions.forEach(sessionId => {
                this.eventBus.emit(EventType.OPERATOR_TERMINAL_THINKING_COMPLETE, { webSessionId: sessionId });
            });
            this.activeSessions.clear();
        }

        this.thinkingActive = this.activeSessions.size > 0;
        this.eventBus.emit(EventType.LLM_CHAT_STOP_HIDE);
    }

    getThinkingText(data) {
        if (!data || !data.thinking) {
            return '';
        }

        return decodeHtmlEntities(data.thinking);
    }

    clearAllThinkingData() {
        this.thinkingActive = false;
        this.activeSessions.clear();
    }
}

