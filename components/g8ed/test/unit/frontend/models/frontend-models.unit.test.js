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
import { FrontendBaseModel, FrontendIdentifiableModel, F } from '@g8ed/public/js/models/base.js';
import {
    InvestigationHistoryEntry,
    InvestigationModel,
    InvestigationFactory,
    Priority,
    Severity,
    EventType, // Import the augmented EventType from here
} from '@g8ed/public/js/models/investigation-models.js';
import { InvestigationStatus } from '@g8ed/public/js/constants/investigation-constants.js';
import { EventType } from '@g8ed/public/js/constants/events.js';

describe('FrontendBaseModel — parse() [UNIT]', () => {
    class Item extends FrontendBaseModel {
        static fields = {
            name:  { type: F.string, required: true },
            count: { type: F.number, default: 0 },
            ts:    { type: F.date,   default: () => new Date() },
        };
    }

    it('parse() returns an instance of the subclass', () => {
        expect(Item.parse({ name: 'x' })).toBeInstanceOf(Item);
    });

    it('parse() coerces F.date string to Date', () => {
        const m = Item.parse({ name: 'x', ts: '2026-01-01T00:00:00.000Z' });
        expect(m.ts).toBeInstanceOf(Date);
        expect(m.ts.toISOString()).toBe('2026-01-01T00:00:00.000Z');
    });

    it('parse() passes through a Date unchanged', () => {
        const d = new Date('2026-06-01T00:00:00.000Z');
        expect(Item.parse({ name: 'x', ts: d }).ts.getTime()).toBe(d.getTime());
    });

    it('parse() applies default for missing optional field', () => {
        const m = Item.parse({ name: 'x' });
        expect(m.count).toBe(0);
    });

    it('parse() applies default factory for missing F.date field', () => {
        const before = Date.now();
        const m = Item.parse({ name: 'x' });
        expect(m.ts).toBeInstanceOf(Date);
        expect(m.ts.getTime()).toBeGreaterThanOrEqual(before);
    });

    it('parse() throws with validationErrors for missing required field', () => {
        let err;
        try { Item.parse({}); } catch (e) { err = e; }
        expect(err).toBeDefined();
        expect(Array.isArray(err.validationErrors)).toBe(true);
        expect(err.validationErrors.some(e => e.includes('name'))).toBe(true);
    });

    it('parse() strips invalid fields', () => {
        expect(Item.parse({ name: 'x', unknown: 'drop' })).not.toHaveProperty('unknown');
    });

    it('parse() throws for non-object input', () => {
        expect(() => Item.parse('bad')).toThrow();
    });

    it('F.boolean strict rejects string', () => {
        class M extends FrontendBaseModel {
            static fields = { flag: { type: F.boolean, required: true } };
        }
        expect(() => M.parse({ flag: 'true' })).toThrow();
    });

    it('F.boolean coerce accepts string', () => {
        class M extends FrontendBaseModel {
            static fields = { flag: { type: F.boolean, required: true, coerce: true } };
        }
        expect(M.parse({ flag: 'true' }).flag).toBe(true);
        expect(M.parse({ flag: 'false' }).flag).toBe(false);
    });

    it('F.number strict rejects string', () => {
        class M extends FrontendBaseModel {
            static fields = { n: { type: F.number, required: true } };
        }
        expect(() => M.parse({ n: '42' })).toThrow();
    });

    it('F.number coerce accepts numeric string', () => {
        class M extends FrontendBaseModel {
            static fields = { n: { type: F.number, required: true, coerce: true } };
        }
        expect(M.parse({ n: '42' }).n).toBe(42);
    });

    it('F.number enforces min/max', () => {
        class M extends FrontendBaseModel {
            static fields = { n: { type: F.number, required: true, min: 1, max: 10 } };
        }
        expect(() => M.parse({ n: 0 })).toThrow();
        expect(() => M.parse({ n: 11 })).toThrow();
        expect(M.parse({ n: 5 }).n).toBe(5);
    });

    it('F.array with items parses each element', () => {
        class Tag extends FrontendBaseModel {
            static fields = { label: { type: F.string, required: true } };
        }
        class M extends FrontendBaseModel {
            static fields = { tags: { type: F.array, items: Tag, default: () => [] } };
        }
        const m = M.parse({ tags: [{ label: 'a' }, { label: 'b' }] });
        expect(m.tags[0]).toBeInstanceOf(Tag);
        expect(m.tags[1].label).toBe('b');
    });
});

describe('FrontendBaseModel — forWire() [UNIT]', () => {
    it('serializes Date fields to ISO strings', () => {
        class M extends FrontendBaseModel {
            static fields = { ts: { type: F.date, default: null } };
        }
        const m = M.parse({ ts: new Date('2026-01-01T00:00:00.000Z') });
        const wire = m.forWire();
        expect(typeof wire.ts).toBe('string');
        expect(wire.ts).toBe('2026-01-01T00:00:00.000Z');
    });

    it('no Date instance survives forWire()', () => {
        class M extends FrontendBaseModel {
            static fields = { ts: { type: F.date, default: null }, nested: { type: F.any, default: null } };
        }
        const m = M.parse({ ts: new Date(), nested: { inner: new Date() } });

        function findDates(obj) {
            if (obj instanceof Date) return true;
            if (Array.isArray(obj)) return obj.some(findDates);
            if (obj !== null && typeof obj === 'object') return Object.values(obj).some(findDates);
            return false;
        }

        expect(findDates(m.forWire())).toBe(false);
    });

    it('flattens nested FrontendBaseModel instances', () => {
        class Inner extends FrontendBaseModel {
            static fields = { ts: { type: F.date, default: null } };
        }
        class Outer extends FrontendBaseModel {
            static fields = { inner: { type: F.object, model: Inner, default: null } };
        }
        const outer = Outer.parse({ inner: { ts: '2026-06-01T00:00:00.000Z' } });
        const wire = outer.forWire();
        expect(wire.inner).not.toBeInstanceOf(FrontendBaseModel);
        expect(wire.inner.ts).toBe('2026-06-01T00:00:00.000Z');
    });
});

describe('FrontendIdentifiableModel [UNIT]', () => {
    it('id defaults to a UUID string', () => {
        const m = FrontendIdentifiableModel.parse({});
        expect(typeof m.id).toBe('string');
        expect(m.id.length).toBeGreaterThan(0);
    });

    it('accepts explicit id', () => {
        expect(FrontendIdentifiableModel.parse({ id: 'custom' }).id).toBe('custom');
    });

    it('created_at is a Date inside the boundary', () => {
        expect(FrontendIdentifiableModel.parse({}).created_at).toBeInstanceOf(Date);
    });

    it('updated_at is null by default', () => {
        expect(FrontendIdentifiableModel.parse({}).updated_at).toBeNull();
    });

    it('updateTimestamp() sets updated_at to a Date >= created_at', () => {
        const m = FrontendIdentifiableModel.parse({});
        m.updateTimestamp();
        expect(m.updated_at).toBeInstanceOf(Date);
        expect(m.updated_at.getTime()).toBeGreaterThanOrEqual(m.created_at.getTime());
    });

    it('forWire() serializes created_at and updated_at to ISO strings', () => {
        const m = FrontendIdentifiableModel.parse({});
        m.updateTimestamp();
        const wire = m.forWire();
        expect(typeof wire.created_at).toBe('string');
        expect(typeof wire.updated_at).toBe('string');
    });

    it('forWire() emits null for updated_at when not set', () => {
        expect(FrontendIdentifiableModel.parse({}).forWire().updated_at).toBeNull();
    });

    it('subclass inherits id/created_at/updated_at without redeclaring', () => {
        class Doc extends FrontendIdentifiableModel {
            static fields = { name: { type: F.string, required: true } };
        }
        const m = Doc.parse({ name: 'test' });
        expect(typeof m.id).toBe('string');
        expect(m.created_at).toBeInstanceOf(Date);
        expect(m.name).toBe('test');
    });
});

describe('InvestigationHistoryEntry [UNIT]', () => {
    const minimal = { 
        event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER, 
        actor: EventType.EVENT_SOURCE_USER_CHAT, 
        summary: 'Created' 
    };

    it('parse() returns an instance', () => {
        expect(InvestigationHistoryEntry.parse(minimal)).toBeInstanceOf(InvestigationHistoryEntry);
    });

    it('timestamp defaults to a Date', () => {
        expect(InvestigationHistoryEntry.parse(minimal).timestamp).toBeInstanceOf(Date);
    });

    it('timestamp coerced from ISO string', () => {
        const m = InvestigationHistoryEntry.parse({ ...minimal, timestamp: '2026-01-01T00:00:00.000Z' });
        expect(m.timestamp).toBeInstanceOf(Date);
        expect(m.timestamp.toISOString()).toBe('2026-01-01T00:00:00.000Z');
    });

    it('forWire() serializes timestamp to ISO string', () => {
        const wire = InvestigationHistoryEntry.parse(minimal).forWire();
        expect(typeof wire.timestamp).toBe('string');
    });

    it('throws for missing required fields', () => {
        expect(() => InvestigationHistoryEntry.parse({})).toThrow();
    });

    it('backfills event_type from context if missing at root', () => {
        const m = InvestigationHistoryEntry.parse({
            actor: EventType.EVENT_SOURCE_USER_CHAT,
            summary: 'hello',
            context: { event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER }
        });
        expect(m.event_type).toBe(EventType.INVESTIGATION_CHAT_MESSAGE_USER);
    });

    it('id defaults to a string', () => {
        expect(typeof InvestigationHistoryEntry.parse(minimal).id).toBe('string');
    });

    it('isUserMessage() returns true for USER_CHAT actor', () => {
        expect(InvestigationHistoryEntry.parse(minimal).isUserMessage()).toBe(true);
    });

    it('isAIResponse() returns true for AI_PRIMARY actor', () => {
        const m = InvestigationHistoryEntry.parse({ 
            event_type: EventType.INVESTIGATION_CHAT_MESSAGE_AI,
            actor: EventType.EVENT_SOURCE_AI_PRIMARY,
            summary: 'hi'
        });
        expect(m.isAIResponse()).toBe(true);
    });

    it('getSenderDisplayName() returns "You" for user', () => {
        expect(InvestigationHistoryEntry.parse(minimal).getSenderDisplayName()).toBe('You');
    });

    it('getSenderDisplayName() returns "g8e.local" for AI', () => {
        const m = InvestigationHistoryEntry.parse({ 
            event_type: EventType.INVESTIGATION_CHAT_MESSAGE_AI,
            actor: EventType.EVENT_SOURCE_AI_PRIMARY,
            summary: 'hi'
        });
        expect(m.getSenderDisplayName()).toBe('g8e');
    });
});

describe('InvestigationModel [UNIT]', () => {
    const minimal = {
        case_id: 'case-1',
        case_title: 'Test Case',
        case_description: 'A test',
    };

    it('parse() returns an instance', () => {
        expect(InvestigationModel.parse(minimal)).toBeInstanceOf(InvestigationModel);
    });

    it('status defaults to OPEN', () => {
        expect(InvestigationModel.parse(minimal).status).toBe(InvestigationStatus.OPEN);
    });

    it('priority defaults to MEDIUM integer', () => {
        expect(InvestigationModel.parse(minimal).priority).toBe(Priority.MEDIUM);
    });

    it('coerces string priority to integer', () => {
        expect(InvestigationModel.parse({ ...minimal, priority: 'high' }).priority).toBe(Priority.HIGH);
    });

    it('coerces string severity to integer', () => {
        expect(InvestigationModel.parse({ ...minimal, severity: 'critical' }).severity).toBe(Severity.CRITICAL);
    });

    it('history_trail defaults to empty array', () => {
        expect(InvestigationModel.parse(minimal).history_trail).toEqual([]);
    });

    it('history_trail entries are InvestigationHistoryEntry instances', () => {
        const m = InvestigationModel.parse({
            ...minimal,
            history_trail: [{ event_type: EventType.CASE_CREATED, actor: 'g8ed', summary: 'Init' }],
        });
        expect(m.history_trail[0]).toBeInstanceOf(InvestigationHistoryEntry);
    });

    it('created_at is a Date inside the boundary', () => {
        expect(InvestigationModel.parse(minimal).created_at).toBeInstanceOf(Date);
    });

    it('forWire() has no Date instances', () => {
        function findDates(obj) {
            if (obj instanceof Date) return true;
            if (Array.isArray(obj)) return obj.some(findDates);
            if (obj !== null && typeof obj === 'object') return Object.values(obj).some(findDates);
            return false;
        }
        const m = InvestigationModel.parse({
            ...minimal,
            history_trail: [{ event_type: EventType.CASE_CREATED, actor: 'g8ed', summary: 'Init' }],
        });
        expect(findDates(m.forWire())).toBe(false);
    });

    it('addHistoryEntry() pushes a parsed entry and calls updateTimestamp', () => {
        const m = InvestigationModel.parse(minimal);
        m.addHistoryEntry('status_changed', 'g8ed', 'Changed to closed');
        expect(m.history_trail).toHaveLength(1);
        expect(m.history_trail[0]).toBeInstanceOf(InvestigationHistoryEntry);
        expect(m.updated_at).toBeInstanceOf(Date);
    });

    it('updateStatus() updates status and records history entry', () => {
        const m = InvestigationModel.parse(minimal);
        m.updateStatus('Closed', 'g8ed', 'Closing');
        expect(m.status).toBe('Closed');
        expect(m.history_trail).toHaveLength(1);
        expect(m.history_trail[0].event_type).toBe('status_changed');
    });

    it('hasConversationHistory() returns false when null', () => {
        expect(InvestigationModel.parse(minimal).hasConversationHistory()).toBe(false);
    });

    it('throws for missing required fields', () => {
        expect(() => InvestigationModel.parse({})).toThrow();
    });
});

describe('InvestigationHistoryEntry — g8ee wire shape compat [UNIT]', () => {
    it('parses g8ee ConversationHistoryMessage wire shape (user chat)', () => {
        const wire = {
            id: 'msg-1',
            sender: EventType.EVENT_SOURCE_USER_CHAT,
            content: 'Help me with docker',
            timestamp: '2026-04-06T15:00:00.000Z',
            metadata: {},
        };
        const entry = InvestigationHistoryEntry.parse(wire);
        expect(entry).toBeInstanceOf(InvestigationHistoryEntry);
        expect(entry.actor).toBe(EventType.EVENT_SOURCE_USER_CHAT);
        expect(entry.event_type).toBe(EventType.INVESTIGATION_CHAT_MESSAGE_USER);
        expect(entry.summary).toBe('Help me with docker');
        expect(entry.content).toBe('Help me with docker');
    });

    it('parses g8ee ConversationHistoryMessage wire shape (AI response)', () => {
        const wire = {
            id: 'msg-2',
            sender: EventType.EVENT_SOURCE_AI_PRIMARY,
            content: 'Docker version is 24.0.7',
            timestamp: '2026-04-06T15:01:00.000Z',
            metadata: {},
        };
        const entry = InvestigationHistoryEntry.parse(wire);
        expect(entry.actor).toBe(EventType.EVENT_SOURCE_AI_PRIMARY);
        expect(entry.event_type).toBe(EventType.INVESTIGATION_CHAT_MESSAGE_AI);
        expect(entry.summary).toBe('Docker version is 24.0.7');
    });

    it('parses g8ee ConversationHistoryMessage wire shape (system message)', () => {
        const wire = {
            id: 'msg-3',
            sender: EventType.EVENT_SOURCE_SYSTEM,
            content: 'Approval requested',
            timestamp: '2026-04-06T15:02:00.000Z',
            metadata: { approval_id: 'appr-1' },
        };
        const entry = InvestigationHistoryEntry.parse(wire);
        expect(entry.actor).toBe(EventType.EVENT_SOURCE_SYSTEM);
        expect(entry.event_type).toBe(EventType.INVESTIGATION_CHAT_MESSAGE_SYSTEM);
    });

    it('truncates summary to 500 chars when derived from content', () => {
        const wire = {
            sender: EventType.EVENT_SOURCE_AI_PRIMARY,
            content: 'x'.repeat(600),
            metadata: {},
        };
        const entry = InvestigationHistoryEntry.parse(wire);
        expect(entry.summary).toHaveLength(500);
        expect(entry.content).toHaveLength(600);
    });

    it('derives event_type for ai.assistant sender', () => {
        const wire = {
            sender: EventType.EVENT_SOURCE_AI_ASSISTANT,
            content: 'Quick answer',
            metadata: {},
        };
        const entry = InvestigationHistoryEntry.parse(wire);
        expect(entry.event_type).toBe(EventType.INVESTIGATION_CHAT_MESSAGE_AI);
    });

    it('derives event_type for user.terminal sender', () => {
        const wire = {
            sender: EventType.EVENT_SOURCE_USER_TERMINAL,
            content: 'ls -la',
            metadata: {},
        };
        const entry = InvestigationHistoryEntry.parse(wire);
        expect(entry.event_type).toBe(EventType.INVESTIGATION_CHAT_MESSAGE_USER);
    });

    it('parseConversationHistory handles array of g8ee wire messages', () => {
        const history = [
            { sender: EventType.EVENT_SOURCE_USER_CHAT, content: 'hello', metadata: {} },
            { sender: EventType.EVENT_SOURCE_AI_PRIMARY, content: 'hi there', metadata: {} },
        ];
        const result = InvestigationFactory.parseConversationHistory(history);
        expect(result).toHaveLength(2);
        expect(result[0].event_type).toBe(EventType.INVESTIGATION_CHAT_MESSAGE_USER);
        expect(result[1].event_type).toBe(EventType.INVESTIGATION_CHAT_MESSAGE_AI);
    });
});

describe('InvestigationFactory [UNIT]', () => {
    const apiResponse = {
        case_id: 'case-1',
        case_title: 'Test',
        case_description: 'Desc',
        status: 'Open',
    };

    it('fromAPIResponse() returns an InvestigationModel', () => {
        expect(InvestigationFactory.fromAPIResponse(apiResponse)).toBeInstanceOf(InvestigationModel);
    });

    it('createConversationMessage() returns an InvestigationHistoryEntry', () => {
        const m = InvestigationFactory.createConversationMessage('hello', EventType.INVESTIGATION_CHAT_MESSAGE_USER, 'inv-1');
        expect(m).toBeInstanceOf(InvestigationHistoryEntry);
        expect(m.content).toBe('hello');
        expect(m.context.investigation_id).toBe('inv-1');
    });

    it('parseConversationHistory() returns array of InvestigationHistoryEntry', () => {
        const history = [
            { content: 'msg1', actor: EventType.EVENT_SOURCE_USER_CHAT, event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER },
            { content: 'msg2', actor: EventType.EVENT_SOURCE_AI_PRIMARY, event_type: EventType.INVESTIGATION_CHAT_MESSAGE_AI },
        ];
        const result = InvestigationFactory.parseConversationHistory(history);
        expect(result).toHaveLength(2);
        result.forEach(m => expect(m).toBeInstanceOf(InvestigationHistoryEntry));
    });

    it('parseConversationHistory() returns [] for non-array input', () => {
        expect(InvestigationFactory.parseConversationHistory(null)).toEqual([]);
        expect(InvestigationFactory.parseConversationHistory(undefined)).toEqual([]);
    });
});

describe('Wire boundary discipline [UNIT]', () => {
    function findDates(obj) {
        if (obj instanceof Date) return true;
        if (Array.isArray(obj)) return obj.some(findDates);
        if (obj !== null && typeof obj === 'object') return Object.values(obj).some(findDates);
        return false;
    }

    it('InvestigationModel with history_trail has no Date in forWire()', () => {
        const m = InvestigationModel.parse({
            case_id: 'c', case_title: 't', case_description: 'd',
            history_trail: [{ event_type: EventType.CASE_CREATED, actor: 'g8ed', summary: 'Init' }],
        });
        expect(findDates(m.forWire())).toBe(false);
    });

    it('InvestigationHistoryEntry has no Date in forWire()', () => {
        const m = InvestigationHistoryEntry.parse({ event_type: EventType.INVESTIGATION_CHAT_MESSAGE_USER, actor: EventType.EVENT_SOURCE_USER_CHAT, summary: 's' });
        expect(findDates(m.forWire())).toBe(false);
    });
});
