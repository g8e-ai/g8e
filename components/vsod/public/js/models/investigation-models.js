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

import { InvestigationStatus } from '../constants/investigation-constants.js';
import { FrontendIdentifiableModel, F } from './base.js';
import { EventType as AutoEventType } from '../constants/events.js';

export const EventType = AutoEventType;

export const Priority = Object.freeze({
    CRITICAL: 0,
    HIGH: 1,
    MEDIUM: 2,
    LOW: 3,
});

export const Severity = Object.freeze({
    CRITICAL: 0,
    HIGH: 1,
    MEDIUM: 2,
    LOW: 3,
});

export const ComponentName = Object.freeze({
    G8EE: 'g8ee',
    G8EO: 'g8eo',
    VSOD: 'vsod',
});

export const TroubleshootingPhase = Object.freeze({
    PROBLEM_FORMULATION: 'problem_formulation',
    MULTI_AGENT_ANALYSIS: 'multi_agent_analysis',
    RUNTIME_INTEGRATION: 'runtime_integration',
    ITERATIVE_REFINEMENT: 'iterative_refinement',
    COMPLETED: 'completed',
});

export const TroubleshootingMethod = Object.freeze({
    CHAIN_OF_THOUGHT: 'chain_of_thought',
    MULTI_AGENT_COLLABORATIVE: 'multi_agent_collaborative',
    SELF_REFINEMENT: 'self_refinement',
    RUNTIME_EXECUTION: 'runtime_execution',
    HYBRID_APPROACH: 'hybrid_approach',
});

function _priorityToInt(value) {
    if (typeof value === 'number') return value;
    const map = { critical: 0, high: 1, medium: 2, low: 3 };
    return map[value?.toLowerCase()] ?? Priority.MEDIUM;
}

function _severityToInt(value) {
    if (typeof value === 'number') return value;
    const map = { critical: 0, high: 1, medium: 2, low: 3 };
    return map[value?.toLowerCase()] ?? Severity.MEDIUM;
}

export class InvestigationHistoryEntry extends FrontendIdentifiableModel {
    static fields = {
        timestamp:               { type: F.date,   default: () => new Date() },
        event_type:              { type: F.string,  required: true },
        actor:                   { type: F.string,  required: true },
        summary:                 { type: F.string,  required: true },
        content:                 { type: F.string,  default: '' },
        attempt_number:          { type: F.number,  default: 1 },
        investigation_attempt:   { type: F.any,     default: null },
        details:                 { type: F.any,     default: () => ({}) },
        metadata:                { type: F.any,     default: () => ({}) },
        context:                 { type: F.any,     default: () => ({}) },
        grounding_metadata:      { type: F.any,     default: null },
        troubleshooting_step:    { type: F.any,     default: null },
        refinement_cycle:        { type: F.any,     default: null },
        phase:                   { type: F.any,     default: null },
    };

    /**
     * Map wire fields to internal model fields.
     * This is the ONLY place where normalization from external sources happens.
     */
    static _senderToEventType = Object.freeze({
        [EventType.EVENT_SOURCE_USER_CHAT]:     EventType.INVESTIGATION_CHAT_MESSAGE_USER,
        [EventType.EVENT_SOURCE_AI_PRIMARY]:    EventType.INVESTIGATION_CHAT_MESSAGE_AI,
        [EventType.EVENT_SOURCE_AI_ASSISTANT]:  EventType.INVESTIGATION_CHAT_MESSAGE_AI,
        [EventType.EVENT_SOURCE_USER_TERMINAL]: EventType.INVESTIGATION_CHAT_MESSAGE_USER,
        [EventType.EVENT_SOURCE_SYSTEM]:        EventType.INVESTIGATION_CHAT_MESSAGE_SYSTEM,
    });

    static parse(raw = {}) {
        if (!raw || typeof raw !== 'object') return super.parse(raw);

        const coerced = { ...raw };

        const actor = raw.actor || raw.sender || raw.metadata?.sender;
        if (actor) coerced.actor = actor;

        const eventType = raw.event_type
            || raw.context?.event_type
            || InvestigationHistoryEntry._senderToEventType[raw.sender];
        if (eventType) coerced.event_type = eventType;

        if (!raw.summary && raw.content) {
            coerced.summary = raw.content.slice(0, 500);
        }
        if (raw.summary && !raw.content) {
            coerced.content = raw.summary;
        }

        return super.parse(coerced);
    }

    _validate() {
        if (!this.context || typeof this.context !== 'object') {
            this.context = {};
        }

        // Ensure internal metadata is populated from core fields
        this.metadata = {
            ...(this.metadata || {}),
            sender: this.actor,
            event_type: this.event_type,
            attachments_count: this.metadata?.attachments_count || 0,
            message_length:    this.content?.length || 0,
        };

        this.grounding_metadata = this.grounding_metadata || this.metadata?.grounding_metadata || null;
    }

    isUserMessage() {
        return this.event_type === EventType.INVESTIGATION_CHAT_MESSAGE_USER ||
               this.actor === EventType.EVENT_SOURCE_USER_CHAT ||
               this.actor === EventType.EVENT_SOURCE_USER_TERMINAL;
    }

    isAIResponse() {
        return this.event_type === EventType.INVESTIGATION_CHAT_MESSAGE_AI ||
               this.actor === EventType.EVENT_SOURCE_AI_PRIMARY ||
               this.actor === EventType.EVENT_SOURCE_AI_ASSISTANT;
    }

    isSystemMessage() {
        return this.event_type === EventType.INVESTIGATION_CHAT_MESSAGE_SYSTEM ||
               this.actor === EventType.EVENT_SOURCE_SYSTEM;
    }

    getSenderDisplayName() {
        if (this.isUserMessage()) return 'You';
        if (this.isAIResponse()) return 'g8e';
        return 'System';
    }

    getSenderIcon() {
        if (this.isUserMessage()) return 'person';
        if (this.isAIResponse()) return 'smart_toy';
        return 'settings';
    }
}

export class InvestigationCurrentState extends FrontendIdentifiableModel {
    static fields = {
        active_attempt:           { type: F.number,  default: 1 },
        pending_actions:          { type: F.any,     default: null },
        next_deadline:            { type: F.date,    default: null },
        escalation_risk:          { type: F.string,  default: 'low' },
        collaboration_status:     { type: F.any,     default: () => ({}) },
        troubleshooting_context:  { type: F.any,     default: null },
        g8eo_engagement_criteria:  { type: F.any,     default: null },
    };
}

export class InvestigationModel extends FrontendIdentifiableModel {
    static fields = {
        case_id:              { type: F.string,  required: true },
        case_title:           { type: F.string,  required: true },
        case_description:     { type: F.string,  required: true },
        task_id:              { type: F.string,  default: null },
        web_session_id:       { type: F.string,  default: null },
        user_email:           { type: F.string,  default: null },
        user_id:              { type: F.string,  default: null },
        status:               { type: F.string,  default: InvestigationStatus.OPEN },
        priority:             { type: F.number,  default: Priority.MEDIUM },
        severity:             { type: F.number,  default: Severity.MEDIUM },
        metadata:             { type: F.any,     default: () => ({}) },
        conversation_history: { type: F.array,   items: InvestigationHistoryEntry, default: () => [] },
        history_trail:        { type: F.array,   items: InvestigationHistoryEntry, default: () => [] },
        current_state:        { type: F.object,  model: InvestigationCurrentState, default: null },
    };

    static parse(raw = {}) {
        const coerced = { ...raw };
        if (raw.priority !== undefined) coerced.priority = _priorityToInt(raw.priority);
        if (raw.severity !== undefined) coerced.severity = _severityToInt(raw.severity);
        
        // Handle legacy field names in API response
        if (raw.history && !raw.history_trail) coerced.history_trail = raw.history;
        if (raw.messages && !raw.conversation_history) coerced.conversation_history = raw.messages;

        return super.parse(coerced);
    }

    addHistoryEntry(eventType, actor, summary, attemptNumber = null, investigationAttempt = null, details = null) {
        const attempt = attemptNumber || (this.current_state?.active_attempt || 1);
        const entry = InvestigationHistoryEntry.parse({
            event_type: eventType,
            actor,
            summary,
            attempt_number: attempt,
            investigation_attempt: investigationAttempt,
            details: details || {},
            context: {
                event_type: eventType,
                investigation_id: this.id,
                case_id: this.case_id
            }
        });
        this.history_trail.push(entry);
        this.updateTimestamp();
    }

    updateStatus(newStatus, actor, summary) {
        const oldStatus = this.status;
        this.status = newStatus;
        this.addHistoryEntry('status_changed', actor, summary, null, null, {
            old_status: oldStatus,
            new_status: newStatus,
        });
    }

    getLatestMessage() {
        if (!this.conversation_history?.length) return null;
        return this.conversation_history[this.conversation_history.length - 1];
    }

    getMessagesBySender(sender) {
        return (this.conversation_history || []).filter(msg => {
            return (msg.metadata?.sender || msg.sender || msg.actor) === sender;
        });
    }

    getUserMessages() {
        return this.getMessagesBySender(EventType.EVENT_SOURCE_USER_CHAT);
    }

    getAIMessages() {
        return this.getMessagesBySender(EventType.EVENT_SOURCE_AI_PRIMARY);
    }

    hasConversationHistory() {
        return !!(this.conversation_history?.length);
    }
}

export class InvestigationFactory {
    static fromAPIResponse(data) {
        return InvestigationModel.parse(data);
    }

    static createConversationMessage(content, eventType, investigationId, webSessionId = null, caseId = null, sender = EventType.EVENT_SOURCE_USER_CHAT) {
        return InvestigationHistoryEntry.parse({
            content,
            summary: content,
            actor: sender,
            event_type: eventType,
            context: {
                investigation_id: investigationId,
                event_type: eventType,
                web_session_id: webSessionId,
                case_id: caseId,
            },
            metadata: {
                sender,
                event_type: eventType,
                message_length: content.length,
            },
        });
    }

    static parseConversationHistory(conversationHistory) {
        if (!Array.isArray(conversationHistory)) return [];
        return conversationHistory.map(msg => InvestigationHistoryEntry.parse(msg));
    }
}
