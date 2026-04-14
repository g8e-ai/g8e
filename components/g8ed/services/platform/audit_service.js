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

export class AuditService {

    flattenInvestigationEvents(investigations, { fromDate, toDate } = {}) {
        const investigationsArray = Array.isArray(investigations) ? investigations : [];
        const allEvents = [];

        for (const investigation of investigationsArray) {
            const historyTrail = investigation.history_trail || [];
            const conversationHistory = investigation.conversation_history || [];
            const operatorId = investigation.operator_id || investigation.bound_operator_id || null;
            const operatorName = investigation.operator_name || investigation.bound_operator_name || null;

            for (const entry of historyTrail) {
                allEvents.push({
                    investigation_id: investigation.investigation_id || investigation.id,
                    case_id: investigation.case_id,
                    case_title: investigation.case_title,
                    event_type: entry.event_type,
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

            for (const message of conversationHistory) {
                const sender = message.metadata?.sender || message.sender || null;
                allEvents.push({
                    investigation_id: investigation.investigation_id || investigation.id,
                    case_id: investigation.case_id,
                    case_title: investigation.case_title,
                    event_type: message.event_type || null,
                    actor: sender,
                    summary: message.content?.substring(0, 500),
                    content: message.content,
                    timestamp: message.timestamp || message.created_at,
                    details: {
                        sender: sender,
                        full_content: message.content,
                        has_attachments: !!(message.attachments?.length),
                        has_citations: !!(message.metadata?.grounding_metadata)
                    },
                    operator_id: message.operator_id || operatorId,
                    operator_name: message.operator_name || operatorName
                });
            }
        }

        allEvents.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));

        let filtered = allEvents;
        if (fromDate) {
            const from = new Date(fromDate);
            filtered = filtered.filter(e => new Date(e.timestamp) >= from);
        }
        if (toDate) {
            const to = new Date(toDate);
            filtered = filtered.filter(e => new Date(e.timestamp) <= to);
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
