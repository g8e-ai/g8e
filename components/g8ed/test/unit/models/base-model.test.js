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
import { G8eBaseModel, G8eIdentifiableModel, F, now, addSeconds, toISOString, secondsBetween, daysBetween } from '@g8ed/models/base.js';

class StringModel extends G8eBaseModel {
    static fields = {
        name:  { type: F.string, required: true },
        label: { type: F.string, default: null },
        tag:   { type: F.string, minLength: 2, default: null },
    };
}

class BooleanModel extends G8eBaseModel {
    static fields = {
        active:  { type: F.boolean, required: true },
        coerced: { type: F.boolean, coerce: true, default: null },
    };
}

class NumberModel extends G8eBaseModel {
    static fields = {
        count:   { type: F.number, required: true },
        limited: { type: F.number, min: 1, max: 100, default: null },
        coerced: { type: F.number, coerce: true, default: null },
    };
}

class DateModel extends G8eBaseModel {
    static fields = {
        ts: { type: F.date, required: true },
    };
}

class NestedItem extends G8eBaseModel {
    static fields = {
        value: { type: F.string, required: true },
    };
}

class ArrayModel extends G8eBaseModel {
    static fields = {
        tags:  { type: F.array, default: () => [] },
        items: { type: F.array, items: NestedItem, default: () => [] },
    };
}

class ObjectModel extends G8eBaseModel {
    static fields = {
        meta:   { type: F.object, default: () => ({}) },
        nested: { type: F.object, model: NestedItem, default: null },
    };
}

class AbsentModel extends G8eBaseModel {
    static fields = {
        required_field: { type: F.string, required: true },
        absent_field:   { type: F.string, absent: true },
    };
}

class SecretModel extends G8eBaseModel {
    static fields = {
        id:      { type: F.string, required: true },
        api_key: { type: F.string, default: null },
        label:   { type: F.string, default: null },
    };

    forClient() {
        const obj = this.forDB();
        delete obj.api_key;
        return obj;
    }
}

describe('Time helpers [UNIT - PURE LOGIC]', () => {
    describe('now()', () => {
        it('returns a Date instance', () => {
            expect(now()).toBeInstanceOf(Date);
        });

        it('returns approximately the current time', () => {
            const before = Date.now();
            const d = now();
            const after = Date.now();
            expect(d.getTime()).toBeGreaterThanOrEqual(before);
            expect(d.getTime()).toBeLessThanOrEqual(after);
        });
    });

    describe('addSeconds()', () => {
        it('adds seconds to a Date', () => {
            const base = new Date('2026-01-01T00:00:00.000Z');
            const result = addSeconds(base, 60);
            expect(result.toISOString()).toBe('2026-01-01T00:01:00.000Z');
        });

        it('supports negative seconds', () => {
            const base = new Date('2026-01-01T00:01:00.000Z');
            const result = addSeconds(base, -60);
            expect(result.toISOString()).toBe('2026-01-01T00:00:00.000Z');
        });

        it('returns a new Date instance', () => {
            const base = new Date();
            const result = addSeconds(base, 10);
            expect(result).not.toBe(base);
        });
    });

    describe('secondsBetween()', () => {
        it('computes positive difference', () => {
            const a = new Date('2026-01-01T00:00:00.000Z');
            const b = new Date('2026-01-01T00:01:30.000Z');
            expect(secondsBetween(a, b)).toBe(90);
        });

        it('floors fractional seconds', () => {
            const a = new Date('2026-01-01T00:00:00.000Z');
            const b = new Date('2026-01-01T00:00:01.999Z');
            expect(secondsBetween(a, b)).toBe(1);
        });
    });

    describe('daysBetween()', () => {
        it('computes positive day difference', () => {
            const a = new Date('2026-01-01T00:00:00.000Z');
            const b = new Date('2026-01-04T00:00:00.000Z');
            expect(daysBetween(a, b)).toBe(3);
        });

        it('floors fractional days', () => {
            const a = new Date('2026-01-01T00:00:00.000Z');
            const b = new Date('2026-01-01T23:59:59.000Z');
            expect(daysBetween(a, b)).toBe(0);
        });
    });

    describe('toISOString()', () => {
        it('returns null for falsy input', () => {
            expect(toISOString(null)).toBeNull();
            expect(toISOString(undefined)).toBeNull();
            expect(toISOString('')).toBeNull();
        });

        it('returns ISO string for a Date instance', () => {
            const d = new Date('2026-03-01T12:00:00.000Z');
            expect(toISOString(d)).toBe('2026-03-01T12:00:00.000Z');
        });

        it('parses an ISO string and returns ISO string', () => {
            expect(toISOString('2026-03-01T12:00:00.000Z')).toBe('2026-03-01T12:00:00.000Z');
        });
    });
});

describe('G8eBaseModel.parse() — F.string strict type enforcement [UNIT - PURE LOGIC]', () => {
    it('accepts a valid string value', () => {
        const m = StringModel.parse({ name: 'alice' });
        expect(m.name).toBe('alice');
    });

    it('rejects a number where a string is expected — no coercion', () => {
        expect(() => StringModel.parse({ name: 42 }))
            .toThrow('must be a string');
    });

    it('rejects a boolean where a string is expected — no coercion', () => {
        expect(() => StringModel.parse({ name: true }))
            .toThrow('must be a string');
    });

    it('rejects an object where a string is expected — no coercion', () => {
        expect(() => StringModel.parse({ name: {} }))
            .toThrow('must be a string');
    });

    it('rejects an array where a string is expected — no coercion', () => {
        expect(() => StringModel.parse({ name: ['a'] }))
            .toThrow('must be a string');
    });

    it('error is attached to validationErrors array', () => {
        let caught;
        try {
            StringModel.parse({ name: 99 });
        } catch (e) {
            caught = e;
        }
        expect(caught).toBeDefined();
        expect(Array.isArray(caught.validationErrors)).toBe(true);
        expect(caught.validationErrors.some(e => e.includes('name'))).toBe(true);
    });

    it('applies minLength check on valid string', () => {
        expect(() => StringModel.parse({ name: 'x', tag: 'a' }))
            .toThrow('at least 2 character(s)');
    });

    it('passes minLength check when string is long enough', () => {
        const m = StringModel.parse({ name: 'x', tag: 'ab' });
        expect(m.tag).toBe('ab');
    });

    it('minLength check trims before comparison', () => {
        expect(() => StringModel.parse({ name: 'x', tag: ' ' }))
            .toThrow('at least 2 character(s)');
    });

    it('optional string field defaults to null when absent', () => {
        const m = StringModel.parse({ name: 'x' });
        expect(m.label).toBeNull();
    });

    it('required string field throws when missing', () => {
        expect(() => StringModel.parse({})).toThrow('is required');
    });
});

describe('G8eBaseModel.parse() — F.array strict type enforcement [UNIT - PURE LOGIC]', () => {
    it('accepts a valid array value', () => {
        const m = ArrayModel.parse({ tags: ['a', 'b'] });
        expect(m.tags).toEqual(['a', 'b']);
    });

    it('rejects a string where an array is expected — no silent fallback', () => {
        expect(() => ArrayModel.parse({ tags: 'not-an-array' }))
            .toThrow('must be an array');
    });

    it('rejects a number where an array is expected', () => {
        expect(() => ArrayModel.parse({ tags: 42 }))
            .toThrow('must be an array');
    });

    it('rejects a plain object where an array is expected', () => {
        expect(() => ArrayModel.parse({ tags: {} }))
            .toThrow('must be an array');
    });

    it('rejects a boolean where an array is expected', () => {
        expect(() => ArrayModel.parse({ tags: false }))
            .toThrow('must be an array');
    });

    it('error is attached to validationErrors array', () => {
        let caught;
        try {
            ArrayModel.parse({ tags: 'bad' });
        } catch (e) {
            caught = e;
        }
        expect(caught).toBeDefined();
        expect(Array.isArray(caught.validationErrors)).toBe(true);
        expect(caught.validationErrors.some(e => e.includes('tags'))).toBe(true);
    });

    it('does NOT silently substitute default when non-array value is present', () => {
        let caught;
        try {
            ArrayModel.parse({ tags: 'wrong' });
        } catch (e) {
            caught = e;
        }
        expect(caught).toBeDefined();
    });

    it('empty array is accepted', () => {
        const m = ArrayModel.parse({ tags: [] });
        expect(m.tags).toEqual([]);
    });

    it('uses default when field is absent', () => {
        const m = ArrayModel.parse({});
        expect(m.tags).toEqual([]);
    });

    it('parses each element via items.parse() when items is defined', () => {
        const m = ArrayModel.parse({ items: [{ value: 'hello' }] });
        expect(m.items).toHaveLength(1);
        expect(m.items[0]).toBeInstanceOf(NestedItem);
        expect(m.items[0].value).toBe('hello');
    });

    it('does not re-wrap elements already of the correct type', () => {
        const item = new NestedItem({ value: 'already' });
        const m = ArrayModel.parse({ items: [item] });
        expect(m.items[0]).toBe(item);
    });
});

describe('G8eBaseModel.parse() — F.boolean [UNIT - PURE LOGIC]', () => {
    it('accepts a boolean true', () => {
        const m = BooleanModel.parse({ active: true });
        expect(m.active).toBe(true);
    });

    it('accepts a boolean false', () => {
        const m = BooleanModel.parse({ active: false });
        expect(m.active).toBe(false);
    });

    it('rejects a non-boolean without coerce', () => {
        expect(() => BooleanModel.parse({ active: 'true' }))
            .toThrow('must be a boolean');
    });

    it('coerces string "true" to boolean when coerce is set', () => {
        const m = BooleanModel.parse({ active: true, coerced: 'true' });
        expect(m.coerced).toBe(true);
    });

    it('coerces string "false" to boolean when coerce is set', () => {
        const m = BooleanModel.parse({ active: true, coerced: 'false' });
        expect(m.coerced).toBe(false);
    });

    it('rejects arbitrary string even with coerce', () => {
        expect(() => BooleanModel.parse({ active: true, coerced: 'yes' }))
            .toThrow('must be a boolean');
    });
});

describe('G8eBaseModel.parse() — F.number [UNIT - PURE LOGIC]', () => {
    it('accepts a valid number', () => {
        const m = NumberModel.parse({ count: 5 });
        expect(m.count).toBe(5);
    });

    it('rejects a non-number without coerce', () => {
        expect(() => NumberModel.parse({ count: '5' }))
            .toThrow('must be a number');
    });

    it('coerces numeric string when coerce is set', () => {
        const m = NumberModel.parse({ count: 1, coerced: '42' });
        expect(m.coerced).toBe(42);
    });

    it('rejects non-finite coerced value', () => {
        expect(() => NumberModel.parse({ count: 1, coerced: 'abc' }))
            .toThrow('must be a number');
    });

    it('enforces min constraint', () => {
        expect(() => NumberModel.parse({ count: 1, limited: 0 }))
            .toThrow('>= 1');
    });

    it('enforces max constraint', () => {
        expect(() => NumberModel.parse({ count: 1, limited: 101 }))
            .toThrow('<= 100');
    });

    it('accepts boundary values for min and max', () => {
        const m = NumberModel.parse({ count: 1, limited: 1 });
        expect(m.limited).toBe(1);
        const m2 = NumberModel.parse({ count: 1, limited: 100 });
        expect(m2.limited).toBe(100);
    });
});

describe('G8eBaseModel.parse() — F.date [UNIT - PURE LOGIC]', () => {
    it('accepts a Date instance', () => {
        const d = new Date('2026-01-01T00:00:00.000Z');
        const m = DateModel.parse({ ts: d });
        expect(m.ts).toBeInstanceOf(Date);
        expect(m.ts).toBe(d);
    });

    it('parses an ISO string into a Date', () => {
        const m = DateModel.parse({ ts: '2026-01-01T00:00:00.000Z' });
        expect(m.ts).toBeInstanceOf(Date);
        expect(m.ts.toISOString()).toBe('2026-01-01T00:00:00.000Z');
    });

    it('forDB serializes Date to ISO string', () => {
        const m = DateModel.parse({ ts: '2026-03-14T10:00:00.000Z' });
        const json = m.forDB();
        expect(typeof json.ts).toBe('string');
        expect(json.ts).toBe('2026-03-14T10:00:00.000Z');
    });
});

describe('G8eBaseModel.parse() — F.object [UNIT - PURE LOGIC]', () => {
    it('accepts a plain object', () => {
        const m = ObjectModel.parse({ meta: { x: 1 } });
        expect(m.meta).toEqual({ x: 1 });
    });

    it('uses default when meta is absent', () => {
        const m = ObjectModel.parse({});
        expect(m.meta).toEqual({});
    });

    it('parses nested model via model.parse() when model is defined', () => {
        const m = ObjectModel.parse({ nested: { value: 'hi' } });
        expect(m.nested).toBeInstanceOf(NestedItem);
        expect(m.nested.value).toBe('hi');
    });

    it('does not re-wrap an instance already of the correct model type', () => {
        const item = new NestedItem({ value: 'existing' });
        const m = ObjectModel.parse({ nested: item });
        expect(m.nested).toBe(item);
    });
});

describe('G8eBaseModel.parse() — absent fields [UNIT - PURE LOGIC]', () => {
    it('absent field is omitted when not in raw input', () => {
        const m = AbsentModel.parse({ required_field: 'x' });
        expect('absent_field' in m).toBe(false);
    });

    it('absent field is set when provided in raw input', () => {
        const m = AbsentModel.parse({ required_field: 'x', absent_field: 'present' });
        expect(m.absent_field).toBe('present');
    });
});

describe('G8eBaseModel constructor [UNIT - PURE LOGIC]', () => {
    it('throws when required string field is a non-string', () => {
        expect(() => new StringModel({ name: 42 }))
            .toThrow('StringModel requires name');
    });

    it('throws when required boolean field is missing', () => {
        expect(() => new BooleanModel({}))
            .toThrow('BooleanModel requires active');
    });

    it('constructs successfully with valid data', () => {
        const m = new StringModel({ name: 'valid' });
        expect(m.name).toBe('valid');
    });

    it('coerces date strings to Date in constructor', () => {
        const m = new DateModel({ ts: '2026-01-01T00:00:00.000Z' });
        expect(m.ts).toBeInstanceOf(Date);
    });
});

describe('G8eBaseModel.parse() — input guard [UNIT - PURE LOGIC]', () => {
    it('throws when raw is null', () => {
        expect(() => StringModel.parse(null)).toThrow('requires a plain object');
    });

    it('throws when raw is a string', () => {
        expect(() => StringModel.parse('bad')).toThrow('requires a plain object');
    });

    it('throws when raw is a number', () => {
        expect(() => StringModel.parse(42)).toThrow('requires a plain object');
    });

    it('accepts empty object', () => {
        expect(() => StringModel.parse({})).toThrow('is required');
    });
});

describe('G8eBaseModel boundary serialization [UNIT - PURE LOGIC]', () => {
    it('forDB() returns a plain object, not a model instance', () => {
        const m = new StringModel({ name: 'test' });
        const db = m.forDB();
        expect(db instanceof StringModel).toBe(false);
        expect(typeof db).toBe('object');
        expect(db.name).toBe('test');
    });

    it('forDB() serializes nested G8eBaseModel instances recursively', () => {
        const item = new NestedItem({ value: 'nested' });
        const m = new ObjectModel({ nested: item });
        const db = m.forDB();
        expect(db.nested instanceof NestedItem).toBe(false);
        expect(db.nested.value).toBe('nested');
    });

    it('forDB() serializes Date fields to ISO strings', () => {
        const m = new DateModel({ ts: new Date('2026-01-01T00:00:00.000Z') });
        expect(typeof m.forDB().ts).toBe('string');
    });

    it('forWire() delegates to forDB() by default', () => {
        const m = new StringModel({ name: 'x' });
        expect(m.forWire()).toEqual(m.forDB());
    });

    it('forClient() delegates to forWire() by default', () => {
        const m = new StringModel({ name: 'x' });
        expect(m.forClient()).toEqual(m.forWire());
    });

    it('forClient() can strip secrets via override', () => {
        const m = new SecretModel({ id: 'abc', api_key: 'secret', label: 'test' });
        const client = m.forClient();
        expect(client.id).toBe('abc');
        expect(client.label).toBe('test');
        expect('api_key' in client).toBe(false);
    });

    it('forKV() returns a plain object identical to forDB()', () => {
        const m = new StringModel({ name: 'kv-test' });
        expect(m.forKV()).toEqual(m.forDB());
        expect(typeof m.forKV()).toBe('object');
        expect(m.forKV().name).toBe('kv-test');
    });

    it('forDB() omits undefined values', () => {
        const m = AbsentModel.parse({ required_field: 'x' });
        const db = m.forDB();
        expect('absent_field' in db).toBe(false);
    });
});

describe('G8eBaseModel field inheritance [UNIT - PURE LOGIC]', () => {
    class ParentModel extends G8eBaseModel {
        static fields = {
            parent_field: { type: F.string, required: true },
        };
    }

    class ChildModel extends ParentModel {
        static fields = {
            child_field: { type: F.string, default: 'default' },
        };
    }

    it('child inherits parent fields', () => {
        const m = ChildModel.parse({ parent_field: 'pval' });
        expect(m.parent_field).toBe('pval');
        expect(m.child_field).toBe('default');
    });

    it('child field can override parent default', () => {
        const m = ChildModel.parse({ parent_field: 'pval', child_field: 'cval' });
        expect(m.child_field).toBe('cval');
    });

    it('missing parent required field still throws', () => {
        expect(() => ChildModel.parse({ child_field: 'x' }))
            .toThrow('is required');
    });
});

describe('G8eIdentifiableModel [UNIT - PURE LOGIC]', () => {
    class IdentifiableDoc extends G8eIdentifiableModel {
        static fields = {
            title: { type: F.string, required: true },
        };
    }

    it('auto-generates a uuid id when not provided', () => {
        const m = new IdentifiableDoc({ title: 'test' });
        expect(typeof m.id).toBe('string');
        expect(m.id.length).toBeGreaterThan(0);
    });

    it('preserves provided id', () => {
        const m = new IdentifiableDoc({ title: 'test', id: 'my-id' });
        expect(m.id).toBe('my-id');
    });

    it('sets created_at to a Date', () => {
        const m = new IdentifiableDoc({ title: 'test' });
        expect(m.created_at).toBeInstanceOf(Date);
    });

    it('defaults updated_at to null', () => {
        const m = new IdentifiableDoc({ title: 'test' });
        expect(m.updated_at).toBeNull();
    });

    it('updateTimestamp() sets updated_at to a Date', () => {
        const m = new IdentifiableDoc({ title: 'test' });
        m.updateTimestamp();
        expect(m.updated_at).toBeInstanceOf(Date);
    });

    it('generateId() returns a uuid string', () => {
        const id = G8eIdentifiableModel.generateId();
        expect(typeof id).toBe('string');
        expect(id.length).toBeGreaterThan(0);
    });

    it('generateId(prefix) prefixes the uuid', () => {
        const id = G8eIdentifiableModel.generateId('doc');
        expect(id.startsWith('doc-')).toBe(true);
    });

    it('forDB() serializes id, created_at as ISO string, updated_at as null', () => {
        const m = new IdentifiableDoc({ title: 'test' });
        const db = m.forDB();
        expect(typeof db.id).toBe('string');
        expect(typeof db.created_at).toBe('string');
        expect(db.updated_at).toBeNull();
    });

    it('round-trips through parse()', () => {
        const original = new IdentifiableDoc({ title: 'round-trip' });
        const parsed = IdentifiableDoc.parse(original.forDB());
        expect(parsed.id).toBe(original.id);
        expect(parsed.title).toBe('round-trip');
        expect(parsed.created_at).toBeInstanceOf(Date);
    });
});
