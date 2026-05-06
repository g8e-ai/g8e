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

import { SourceComponent } from '../../constants/ai.js';

/**
 * Event categories for frontend filtering and stats.
 */
export const AuditEventCategory = {
    CHAT: 'chat',
    COMMAND: 'command',
    APPROVAL: 'approval',
    FILE_EDIT: 'file_edit',
    SYSTEM: 'system',
    OTHER: 'other'
};

export class AuditService {

    /**
     * Map a raw EventType string to a high-level AuditEventCategory.
     * @param {string} eventType 
     * @returns {string} One of AuditEventCategory values
     */
    categorizeEvent(eventType) {
        if (!eventType) return AuditEventCategory.OTHER;

        const et = eventType.toLowerCase();

        if (et.includes('.chat.') || et.includes('.source.user.chat') || et.includes('.source.ai.')) {
            return AuditEventCategory.CHAT;
        }

        if (et.includes('.command.approval.') || et.includes('.file.edit.approval.') || et.includes('.intent.approval.') || et.includes('.stream.approval.')) {
            return AuditEventCategory.APPROVAL;
        }

        if (et.includes('.command.')) {
            return AuditEventCategory.COMMAND;
        }

        if (et.includes('.file.edit.')) {
            return AuditEventCategory.FILE_EDIT;
        }

        if (et.includes('.platform.') || et.includes('.system.')) {
            return AuditEventCategory.SYSTEM;
        }

        return AuditEventCategory.OTHER;
    }

    flattenInvestigationEvents(investigations, { fromDate, toDate } = {}) {
        const investigationsArray = Array.isArray(investigations) ? investigations : [];
        const allEvents = [];

        for (const investigation of investigationsArray) {
            const historyTrail = investigation.history_trail || [];
            const conversationHistory = investigation.conversation_history || [];
            const operatorId = investigation.operator_id || investigation.bound_operator_id || null;
            const operatorName = investigation.operator_name || investigation.bound_operator_name || null;

            // Process history trail (mostly actions and state changes)
            for (const entry of historyTrail) {
                const eventType = entry.event_type;
                allEvents.push({
                    investigation_id: investigation.investigation_id || investigation.id,
                    case_id: investigation.case_id,
                    case_title: investigation.case_title,
                    event_type: eventType,
                    category: this.categorizeEvent(eventType),
                    actor: entry.actor || SourceComponent.G8ED,
                    summary: entry.summary,
                    timestamp: entry.timestamp || investigation.created_at,
                    details: entry.details || {},
                    content: entry.content || entry.summary,
                    attempt_number: entry.attempt_number,
                    phase: entry.phase,
                    operator_id: entry.operator_id || operatorId,
                    operator_name: entry.operator_name || operatorName
                });
            }

            // Process conversation history (chat messages, including embedded tool calls/results)
            for (const message of conversationHistory) {
                const sender = message.metadata?.sender || message.sender || null;
                const eventType = message.event_type || message.metadata?.event_type || (sender?.includes('user') ? 'g8e.v1.source.user.chat' : 'g8e.v1.source.ai.primary');
                
                allEvents.push({
                    investigation_id: investigation.investigation_id || investigation.id,
                    case_id: investigation.case_id,
                    case_title: investigation.case_title,
                    event_type: eventType,
                    category: this.categorizeEvent(eventType),
                    actor: sender,
                    summary: message.content?.substring(0, 500),
                    content: message.content,
                    timestamp: message.timestamp || message.created_at,
                    details: {
                        sender: sender,
                        full_content: message.content,
                        has_attachments: !!(message.attachments?.length),
                        has_citations: !!(message.metadata?.grounding_metadata),
                        ...message.metadata
                    },
                    operator_id: message.operator_id || operatorId,
                    operator_name: message.operator_name || operatorName
                });
            }
        }

        // Deduplicate events that might appear in both trails (if any)
        // This is a safety measure against overlapping data
        const seen = new Set();
        const uniqueEvents = [];
        
        // Sort by timestamp first
        allEvents.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));

        for (const event of allEvents) {
            // Create a unique key based on investigation, type, and content/timestamp
            const key = `${event.investigation_id}:${event.event_type}:${event.timestamp}:${event.summary?.substring(0, 50)}`;
            if (!seen.has(key)) {
                seen.add(key);
                uniqueEvents.push(event);
            }
        }

        let filtered = uniqueEvents;
        if (fromDate) {
            const from = new Date(fromDate);
            if (!isNaN(from)) {
                filtered = filtered.filter(e => new Date(e.timestamp) >= from);
            }
        }
        if (toDate) {
            const to = new Date(toDate);
            if (!isNaN(to)) {
                filtered = filtered.filter(e => new Date(e.timestamp) <= to);
            }
        }

        return filtered;
    }

    buildCsvFromEvents(events) {
        const headers = [
            'timestamp',
            'event_type',
            'actor',
            'summary',
            'case_id',
            'case_title',
            'investigation_id',
            'source',
            'attempt_number',
            'phase'
        ];

        let csv = headers.join(',') + '\n';
        for (const event of events) {
            const row = headers.map(h => {
                const value = event[h];
                const stringValue = String(value == null ? '' : value).replace(/"/g, '""');
                if (stringValue.includes(',') || stringValue.includes('\n') || stringValue.includes('"')) {
                    return `"${stringValue}"`;
                }
                return stringValue;
            });
            csv += row.join(',') + '\n';
        }

        return csv;
    }
}
