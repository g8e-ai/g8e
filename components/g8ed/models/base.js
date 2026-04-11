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

import { v4 as uuidv4 } from 'uuid';

// ---------------------------------------------------------------------------
// Time helpers — Date objects only inside the application boundary
// ---------------------------------------------------------------------------

export function now() {
    return new Date();
}

export function addSeconds(date, seconds) {
    return new Date(date.getTime() + seconds * 1000);
}

export function secondsBetween(a, b) {
    return Math.floor((b.getTime() - a.getTime()) / 1000);
}

export function daysBetween(a, b) {
    return Math.floor((b.getTime() - a.getTime()) / (1000 * 60 * 60 * 24));
}

export function toISOString(value) {
    if (!value) return null;
    if (value instanceof Date) return value.toISOString();
    return new Date(value).toISOString();
}

// ---------------------------------------------------------------------------
// Internal boundary serializer — Date → ISO string, models → plain objects.
// Only called by forDB() / forWire() / forClient(). Never called directly at
// call sites in service or route code.
// ---------------------------------------------------------------------------

function _flatten(value) {
    if (value instanceof G8eBaseModel) return value.forDB();
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
// Field type tokens — used in static fields declarations on subclasses
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
// G8eBaseModel
//
// The base class for all g8ed domain objects.
//
// Subclasses declare a static `fields` object describing every field:
//
//   static fields = {
//       operator_id:  { type: F.string,  required: true },
//       name:         { type: F.string,  required: true, minLength: 1 },
//       status:       { type: F.string,  default: OperatorStatus.AVAILABLE },
//       sentinel_mode:{ type: F.boolean, default: true },
//       limit:        { type: F.number,  default: 20, min: 1, max: 100 },
//       created_at:   { type: F.date,    default: () => now() },
//       system_info:  { type: F.object,  model: SystemInfo, default: () => new SystemInfo() },
//       history:      { type: F.array,   items: HistoryEntry, default: () => [] },
//   };
//
// Field definition options:
//   required:  true           — field is required; missing/null throws validation error
//   default:   value|fn       — value or factory function for missing fields
//   absent:    true           — field is omitted (undefined) when not present; never coerced to null
//                               use for optional fields that must be absent vs null-distinguished
//   coerce:    true           — for F.boolean: accept 'true'/'false' strings (default: strict)
//                             — for F.number: accept numeric strings (default: strict)
//   minLength: N              — F.string: reject strings shorter than N after trim
//   min:       N              — F.number: reject values below N
//   max:       N              — F.number: reject values above N
//   model:     ClassName      — F.object: parse nested object via model.parse()
//   items:     ClassName      — F.array:  parse each element via items.parse()
//
// Construction:
//   ModelClass.parse(raw)     — the only public entry point; validates, coerces, strips unknown
//                               fields, throws structured errors. Use at every inbound boundary.
//
// Boundary serialization:
//   model.forDB()             — before dbClient.setDocument / updateDocument
//   model.forWire()           — before outbound HTTP fetch or pub-sub publish
//   model.forClient()         — before res.json() to the browser
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

export class G8eBaseModel {
    static fields = {};

    constructor(fields = {}) {
        const fieldDefinitions = _collectFields(this.constructor);
        const errors = [];
        for (const [key, def] of Object.entries(fieldDefinitions)) {
            const val = fields[key];
            if (val === undefined || val === null) {
                if (def.required) {
                    errors.push(`${this.constructor.name} requires ${key}`);
                    continue;
                }
                if (def.absent) continue;
                this[key] = typeof def.default === 'function' ? def.default() : (def.default ?? null);
            } else {
                if (def.type === F.string && typeof val !== 'string') {
                    const msg = def.required
                        ? `${this.constructor.name} requires ${key}`
                        : `${key} must be a string`;
                    errors.push(msg);
                    continue;
                }
                if (def.type === F.boolean && typeof val !== 'boolean') {
                    if (!def.coerce || (val !== 'true' && val !== 'false')) {
                        const msg = def.required
                            ? `${this.constructor.name} requires ${key}`
                            : `${key} must be a boolean`;
                        errors.push(msg);
                        continue;
                    }
                    this[key] = val === 'true';
                    continue;
                }
                if (def.type === F.date && !(val instanceof Date)) {
                    this[key] = new Date(val);
                    continue;
                }
                if (def.type === F.object && def.model && !(val instanceof def.model)) {
                    this[key] = def.model.parse(val);
                    continue;
                }
                this[key] = val;
            }
        }
        if (errors.length > 0) {
            const err = new Error(`${this.constructor.name} validation failed: ${errors.join(', ')}`);
            err.validationErrors = errors;
            throw err;
        }
        this._validate();
    }

    _validate() {}

    static parse(raw = {}) {
        if (!raw || typeof raw !== 'object') {
            throw new Error(`${this.name}.parse() requires a plain object, got ${typeof raw}`);
        }

        const fieldDefinitions = _collectFields(this);
        const parsed = {};
        const errors = [];

        for (const [key, def] of Object.entries(fieldDefinitions)) {
            const raw_val = raw[key];
            const missing = raw_val === undefined || raw_val === null;

            if (missing) {
                if (def.required) {
                    errors.push(`${key} is required`);
                    continue;
                }
                if (def.absent) continue;
                parsed[key] = typeof def.default === 'function' ? def.default() : (def.default ?? null);
                continue;
            }

            switch (def.type) {
                case F.string: {
                    if (typeof raw_val !== 'string') {
                        errors.push(`${key} must be a string`);
                        continue;
                    }
                    if (def.minLength !== undefined && raw_val.trim().length < def.minLength) {
                        errors.push(`${key} must be at least ${def.minLength} character(s)`);
                        continue;
                    }
                    parsed[key] = raw_val;
                    break;
                }
                case F.boolean: {
                    if (typeof raw_val !== 'boolean') {
                        if (def.coerce && (raw_val === 'true' || raw_val === 'false')) {
                            parsed[key] = raw_val === 'true';
                        } else {
                            errors.push(`${key} must be a boolean`);
                            continue;
                        }
                    } else {
                        parsed[key] = raw_val;
                    }
                    break;
                }
                case F.number: {
                    let n;
                    if (typeof raw_val === 'number') {
                        n = raw_val;
                    } else if (def.coerce && typeof raw_val === 'string' && raw_val.trim() !== '') {
                        n = Number(raw_val);
                    } else {
                        errors.push(`${key} must be a number`);
                        continue;
                    }
                    if (!isFinite(n)) {
                        errors.push(`${key} must be a number`);
                        continue;
                    }
                    if (def.min !== undefined && n < def.min) {
                        errors.push(`${key} must be >= ${def.min}`);
                        continue;
                    }
                    if (def.max !== undefined && n > def.max) {
                        errors.push(`${key} must be <= ${def.max}`);
                        continue;
                    }
                    parsed[key] = n;
                    break;
                }
                case F.date:
                    if (raw_val instanceof Date) {
                        parsed[key] = raw_val;
                    } else {
                        const d = new Date(raw_val);
                        if (isNaN(d.getTime())) {
                            errors.push(`${key} must be a valid date`);
                            continue;
                        }
                        parsed[key] = d;
                    }
                    break;
                case F.object:
                    if (def.model) {
                        parsed[key] = raw_val instanceof def.model ? raw_val : def.model.parse(raw_val);
                    } else {
                        parsed[key] = raw_val && typeof raw_val === 'object' ? raw_val : {};
                    }
                    break;
                case F.array:
                    if (!Array.isArray(raw_val)) {
                        errors.push(`${key} must be an array`);
                        continue;
                    }
                    if (def.items) {
                        parsed[key] = raw_val.map(item =>
                            item instanceof def.items ? item : def.items.parse(item)
                        );
                    } else {
                        parsed[key] = raw_val;
                    }
                    break;
                case F.any:
                default:
                    parsed[key] = raw_val;
            }
        }

        if (errors.length > 0) {
            const err = new Error(`${this.name} validation failed: ${errors.join(', ')}`);
            err.validationErrors = errors;
            throw err;
        }

        return new this(parsed);
    }

    forDB() {
        return _flatten({ ...this });
    }

    forWire() {
        return this.forDB();
    }

    forClient() {
        return this.forWire();
    }

    forKV() {
        return this.forDB();
    }

}

// ---------------------------------------------------------------------------
// G8eIdentifiableModel
//
// Base class for all persistent documents. Provides:
//   - id          — auto-generated UUID, can be overridden by subclass
//   - created_at  — set on construction
//   - updated_at  — null until updateTimestamp() is called
//   - updateTimestamp() — sets updated_at to now()
//   - generateId(prefix?) — static UUID factory
// ---------------------------------------------------------------------------

export class G8eIdentifiableModel extends G8eBaseModel {
    static fields = {
        id:         { type: F.string, default: () => uuidv4() },
        created_at: { type: F.date,   default: () => now() },
        updated_at: { type: F.date,   default: null },
    };

    updateTimestamp() {
        this.updated_at = now();
    }

    static generateId(prefix) {
        const base = uuidv4();
        return prefix ? `${prefix}-${base}` : base;
    }
}

