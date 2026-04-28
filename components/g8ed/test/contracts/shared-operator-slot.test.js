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
 * OperatorSlot Wire Protocol Contract Tests
 *
 * Verifies that g8ed's OperatorSlot model fields match the canonical
 * wire shape defined in shared/models/wire/operator_slot.json.
 *
 * Prevents desynchronization between g8ed's OperatorSlot serialization
 * and the canonical wire format consumed by frontend components.
 */

import { describe, it, expect } from 'vitest';
import { OperatorSlot } from '@g8ed/models/operator_model.js';
import { readFileSync } from 'fs';
import { join } from 'path';

const wire = JSON.parse(
    readFileSync(join(process.cwd(), '../../shared/models/wire/operator_slot.json'), 'utf-8')
);

describe('g8ed OperatorSlot matches shared/models/wire/operator_slot.json', () => {
    const wireFields = wire.operator_slot.fields;
    const modelFields = OperatorSlot.fields;

    it('operator_id field exists and is required', () => {
        expect(wireFields.operator_id).toBeDefined();
        expect(wireFields.operator_id.required).toBe(true);
        expect(modelFields.operator_id).toBeDefined();
        expect(modelFields.operator_id.required).toBe(true);
    });

    it('name field exists and is optional', () => {
        expect(wireFields.name).toBeDefined();
        expect(wireFields.name.required).toBe(false);
        expect(modelFields.name).toBeDefined();
        expect(modelFields.name.required).toBeUndefined();
    });

    it('status field exists and is optional', () => {
        expect(wireFields.status).toBeDefined();
        expect(wireFields.status.required).toBe(false);
        expect(modelFields.status).toBeDefined();
        expect(modelFields.status.required).toBeUndefined();
    });

    it('status_display field exists and is optional', () => {
        expect(wireFields.status_display).toBeDefined();
        expect(wireFields.status_display.required).toBe(false);
        expect(modelFields.status_display).toBeDefined();
        expect(modelFields.status_display.required).toBeUndefined();
    });

    it('status_class field exists and is optional', () => {
        expect(wireFields.status_class).toBeDefined();
        expect(wireFields.status_class.required).toBe(false);
        expect(wireFields.status_class.default).toBe('inactive');
        expect(modelFields.status_class).toBeDefined();
        expect(modelFields.status_class.default).toBe('inactive');
    });

    it('web_session_id field exists and is optional', () => {
        expect(wireFields.web_session_id).toBeDefined();
        expect(wireFields.web_session_id.required).toBe(false);
        expect(modelFields.bound_web_session_id).toBeDefined();
        expect(modelFields.bound_web_session_id.required).toBeUndefined();
    });

    it('is_g8ep field exists and is optional', () => {
        expect(wireFields.is_g8ep).toBeDefined();
        expect(wireFields.is_g8ep.required).toBe(false);
        expect(wireFields.is_g8ep.default).toBe(false);
        expect(modelFields.is_g8ep).toBeDefined();
        expect(modelFields.is_g8ep.default).toBe(false);
    });

    it('first_deployed field exists and is optional', () => {
        expect(wireFields.first_deployed).toBeDefined();
        expect(wireFields.first_deployed.required).toBe(false);
        expect(modelFields.first_deployed).toBeDefined();
        expect(modelFields.first_deployed.required).toBeUndefined();
    });

    it('claimed_at field exists and is optional', () => {
        expect(wireFields.claimed_at).toBeDefined();
        expect(wireFields.claimed_at.required).toBe(false);
        expect(modelFields.claimed_at).toBeDefined();
        expect(modelFields.claimed_at.required).toBeUndefined();
    });

    it('last_heartbeat field exists and is optional', () => {
        expect(wireFields.last_heartbeat).toBeDefined();
        expect(wireFields.last_heartbeat.required).toBe(false);
        expect(modelFields.last_heartbeat).toBeDefined();
        expect(modelFields.last_heartbeat.required).toBeUndefined();
    });

    it('latest_heartbeat_snapshot field exists and is optional', () => {
        expect(wireFields.latest_heartbeat_snapshot).toBeDefined();
        expect(wireFields.latest_heartbeat_snapshot.required).toBe(false);
        expect(modelFields.latest_heartbeat_snapshot).toBeDefined();
        expect(modelFields.latest_heartbeat_snapshot.required).toBeUndefined();
    });

    it('system_info field does not exist in wire schema', () => {
        expect(wireFields.system_info).toBeUndefined();
    });

    it('all JSON fields exist in g8ed model', () => {
        const jsonKeys = Object.keys(wireFields);
        for (const field of jsonKeys) {
            const mappedField = field === 'web_session_id' ? 'bound_web_session_id' : field;
            expect(modelFields[mappedField]).toBeDefined(
                `shared JSON defines field '${field}' but g8ed OperatorSlot does not have it (mapped to '${mappedField}')`
            );
        }
    });

    it('all g8ed model fields exist in JSON', () => {
        const modelKeys = Object.keys(modelFields);
        const jsonKeys = Object.keys(wireFields);
        const extraFields = modelKeys.filter(key => {
            const mappedKey = key === 'bound_web_session_id' ? 'web_session_id' : key;
            return !jsonKeys.includes(mappedKey);
        });

        expect(extraFields.length).toBe(0,
            `g8ed OperatorSlot has fields not in shared JSON: ${extraFields.join(', ')}. ` +
            'Add them to shared/models/wire/operator_slot.json first.'
        );
    });
});
