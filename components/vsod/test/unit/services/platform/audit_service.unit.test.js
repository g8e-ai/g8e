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

import { describe, it, expect } from 'vitest';
import { AuditService } from '@vsod/services/platform/audit_service.js';
import { SourceComponent } from '@vsod/constants/ai.js';
import { EventType } from '@vsod/constants/chat.js';

describe('AuditService [UNIT]', () => {
    const service = new AuditService();

    const mockInvestigation = {
        investigation_id: 'inv_123',
        case_id: 'case_456',
        case_title: 'Test Case',
        operator_id: 'op_789',
        operator_name: 'Test Operator',
        created_at: '2026-03-01T10:00:00Z',
        history_trail: [
            {
                event_type: 'command_executed',
                summary: 'Executed ls -la',
                timestamp: '2026-03-01T10:05:00Z',
                details: { command: 'ls -la' }
            }
        ],
        conversation_history: [
            {
                sender: EventType.EVENT_SOURCE_USER_CHAT,
                content: 'Hello AI',
                timestamp: '2026-03-01T10:10:00Z'
            },
            {
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                content: 'Hello human',
                timestamp: '2026-03-01T10:11:00Z'
            }
        ]
    };

    describe('flattenInvestigationEvents', () => {
        it('flattens history trail and conversation history', () => {
            const events = service.flattenInvestigationEvents([mockInvestigation]);
            
            expect(events).toHaveLength(3);
            
            // History trail event
            expect(events[0].summary).toBe('Executed ls -la');
            expect(events[0].actor).toBe(SourceComponent.VSOD);
            
            // Conversation events
            expect(events[1].actor).toBe(EventType.EVENT_SOURCE_USER_CHAT);
            expect(events[1].content).toBe('Hello AI');
            expect(events[1].details.sender).toBe(EventType.EVENT_SOURCE_USER_CHAT);
            expect(events[2].actor).toBe(EventType.EVENT_SOURCE_AI_PRIMARY);
            expect(events[2].content).toBe('Hello human');
            expect(events[2].details.sender).toBe(EventType.EVENT_SOURCE_AI_PRIMARY);
        });

        it('filters by date range', () => {
            const fromDate = '2026-03-01T10:08:00Z';
            const events = service.flattenInvestigationEvents([mockInvestigation], { fromDate });
            
            expect(events).toHaveLength(2);
            expect(events[0].actor).toBe(EventType.EVENT_SOURCE_USER_CHAT);
        });

        it('sorts events by timestamp', () => {
            const outOfOrder = {
                ...mockInvestigation,
                history_trail: [
                    { summary: 'Later', timestamp: '2026-03-01T12:00:00Z' },
                    { summary: 'Earlier', timestamp: '2026-03-01T11:00:00Z' }
                ],
                conversation_history: []
            };
            const events = service.flattenInvestigationEvents([outOfOrder]);
            expect(events[0].summary).toBe('Earlier');
            expect(events[1].summary).toBe('Later');
        });
    });

    describe('buildCsvFromEvents', () => {
        it('generates a CSV with headers', () => {
            const events = [
                {
                    timestamp: '2026-03-01T10:00:00Z',
                    event_type: 'test',
                    actor: EventType.EVENT_SOURCE_USER_CHAT,
                    summary: 'Simple summary',
                    case_id: 'C1'
                }
            ];
            const csv = service.buildCsvFromEvents(events);
            const lines = csv.split('\n');
            
            expect(lines[0]).toBe('timestamp,event_type,actor,summary,case_id,case_title,investigation_id,source,attempt_number,phase');
            expect(lines[1]).toContain(`2026-03-01T10:00:00Z,test,${EventType.EVENT_SOURCE_USER_CHAT},Simple summary,C1`);
        });

        it('escapes quotes and wraps values with commas', () => {
            const events = [
                {
                    summary: 'Summary with "quotes" and , commas',
                    actor: EventType.EVENT_SOURCE_USER_CHAT
                }
            ];
            const csv = service.buildCsvFromEvents(events);
            expect(csv).toContain('"Summary with ""quotes"" and , commas"');
        });
    });
});
