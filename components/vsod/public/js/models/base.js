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

// ---------------------------------------------------------------------------
// Field type tokens
// ---------------------------------------------------------------------------

export const F = Object.freeze({
    string:  'string',
    boolean: 'boolean',
    number:  'number',
    date:    'date',
    object:  'object',
    array:   'array',
    any:     'any',
});

// ---------------------------------------------------------------------------
// Internal boundary serializer — Date -> ISO string, models -> plain objects.
// Only invoked by forWire(). Never called directly at call sites.
// ---------------------------------------------------------------------------

function _flatten(value) {
    if (value instanceof FrontendBaseModel) return value.forWire();
    if (value instanceof Date) return value.toISOString();
    if (Array.isArray(value)) return value.map(_flatten);
    if (value !== null && typeof value === 'object') {
        const out = {};
        for (const [k, v] of Object.entries(value)) {
            if (v !== undefined) out[k] = _flatten(v);
        }
        return out;
    }
    return value;
}

// ---------------------------------------------------------------------------
// Field inheritance — merges static fields up the prototype chain
// ---------------------------------------------------------------------------

function _collectFields(cls) {
    const chain = [];
    let cur = cls;
    while (cur && cur !== Object) {
        if (Object.prototype.hasOwnProperty.call(cur, 'fields')) {
            chain.unshift(cur.fields);
        }
        cur = Object.getPrototypeOf(cur);
    }
    return Object.assign({}, ...chain);
}

// ---------------------------------------------------------------------------
// FrontendBaseModel
//
// Base class for all frontend domain objects that cross the wire boundary.
//
// - Date objects live inside the application boundary.
// - forWire() serializes to plain JSON (Date -> ISO string) for outbound fetch.
// - parse(raw) is the only entry point for data arriving from the wire.
//
// Field definition options mirror the backend VSOBaseModel exactly:
//   required:  true           - field is required; missing/null throws
//   default:   value|fn       - value or factory for missing fields
//   absent:    true           - omitted when not present
//   coerce:    true           - for F.boolean/F.number: accept string input
//   minLength: N              - F.string: reject strings shorter than N
//   min/max:   N              - F.number: range enforcement
//   model:     ClassName      - F.object: parse nested object via model.parse()
//   items:     ClassName      - F.array: parse each element via items.parse()
// ---------------------------------------------------------------------------

export class FrontendBaseModel {
    static fields = {};

    constructor(data = {}) {
        const fields = _collectFields(this.constructor);
        for (const [key, def] of Object.entries(fields)) {
            const val = data[key];
            if (val === undefined || val === null) {
                if (def.absent) continue;
                this[key] = typeof def.default === 'function' ? def.default() : (def.default ?? null);
            } else {
                this[key] = val;
            }
        }
    }

    static parse(raw = {}) {
        if (!raw || typeof raw !== 'object') {
            throw new Error(`${this.name}.parse() requires a plain object, got ${typeof raw}`);
        }

        const fields = _collectFields(this);
        const parsed = {};
        const errors = [];

        for (const [key, def] of Object.entries(fields)) {
            const rawVal = raw[key];
            const missing = rawVal === undefined || rawVal === null;

            if (missing) {
                if (def.required) { errors.push(`${key} is required`); continue; }
                if (def.absent) continue;
                parsed[key] = typeof def.default === 'function' ? def.default() : (def.default ?? null);
                continue;
            }

            switch (def.type) {
                case F.string: {
                    const s = String(rawVal);
                    if (def.minLength !== undefined && s.trim().length < def.minLength) {
                        errors.push(`${key} must be at least ${def.minLength} character(s)`);
                        continue;
                    }
                    parsed[key] = s;
                    break;
                }
                case F.boolean: {
                    if (typeof rawVal !== 'boolean') {
                        if (def.coerce && (rawVal === 'true' || rawVal === 'false')) {
                            parsed[key] = rawVal === 'true';
                        } else {
                            errors.push(`${key} must be a boolean`);
                            continue;
                        }
                    } else {
                        parsed[key] = rawVal;
                    }
                    break;
                }
                case F.number: {
                    let n;
                    if (typeof rawVal === 'number') {
                        n = rawVal;
                    } else if (def.coerce && typeof rawVal === 'string' && rawVal.trim() !== '') {
                        n = Number(rawVal);
                    } else {
                        errors.push(`${key} must be a number`);
                        continue;
                    }
                    if (!isFinite(n)) { errors.push(`${key} must be a finite number`); continue; }
                    if (def.min !== undefined && n < def.min) { errors.push(`${key} must be >= ${def.min}`); continue; }
                    if (def.max !== undefined && n > def.max) { errors.push(`${key} must be <= ${def.max}`); continue; }
                    parsed[key] = n;
                    break;
                }
                case F.date:
                    parsed[key] = rawVal instanceof Date ? rawVal : new Date(rawVal);
                    break;
                case F.object:
                    if (def.model) {
                        parsed[key] = rawVal instanceof def.model ? rawVal : def.model.parse(rawVal);
                    } else {
                        parsed[key] = rawVal && typeof rawVal === 'object' ? rawVal : {};
                    }
                    break;
                case F.array:
                    if (!Array.isArray(rawVal)) {
                        parsed[key] = typeof def.default === 'function' ? def.default() : [];
                        break;
                    }
                    parsed[key] = def.items
                        ? rawVal.map(item => item instanceof def.items ? item : def.items.parse(item))
                        : rawVal;
                    break;
                case F.any:
                default:
                    parsed[key] = rawVal;
            }
        }

        if (errors.length > 0) {
            const err = new Error('Validation failed');
            err.validationErrors = errors;
            throw err;
        }

        const instance = new this(parsed);
        instance._validate();
        return instance;
    }

    _validate() {}

    forWire() {
        return _flatten({ ...this });
    }
}

// ---------------------------------------------------------------------------
// FrontendIdentifiableModel
//
// Extends FrontendBaseModel with id, created_at, updated_at.
// created_at is a Date inside the boundary; serialized to ISO string by forWire().
// ---------------------------------------------------------------------------

export class FrontendIdentifiableModel extends FrontendBaseModel {
    static fields = {
        id:         { type: F.string, default: () => crypto.randomUUID() },
        created_at: { type: F.date,   default: () => new Date() },
        updated_at: { type: F.date,   default: null },
    };

    updateTimestamp() {
        this.updated_at = new Date();
    }
}
