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
 * Bound Operator Context Wire Protocol Contract Tests
 *
 * Verifies that g8ed's BoundOperatorContext model fields match the canonical
 * wire shape defined in shared/models/wire/bound_operator_context.json.
 *
 * Prevents desynchronization between g8ed's BoundOperatorContext.forWire()
 * output and g8ee's BoundOperator parsing logic.
 */

import { describe, it, expect } from 'vitest';
import { BoundOperatorContext } from '@g8ed/models/request_models.js';
import { readFileSync } from 'fs';
import { join } from 'path';

const wire = JSON.parse(
    readFileSync(join(process.cwd(), '../../shared/models/wire/bound_operator_context.json'), 'utf-8')
);

describe('g8ed BoundOperatorContext matches shared/models/wire/bound_operator_context.json', () => {
    const wireFields = wire.bound_operator_context.fields;
    const modelFields = BoundOperatorContext.fields;

    it('operator_id field exists and is required', () => {
        expect(wireFields.operator_id).toBeDefined();
        expect(wireFields.operator_id.required).toBe(true);
        expect(modelFields.operator_id).toBeDefined();
        expect(modelFields.operator_id.required).toBe(true);
    });

    it('operator_session_id field exists and is optional', () => {
        expect(wireFields.operator_session_id).toBeDefined();
        expect(wireFields.operator_session_id.required).toBe(false);
        expect(modelFields.operator_session_id).toBeDefined();
        expect(modelFields.operator_session_id.required).toBeUndefined();
    });

    it('status field exists and is optional', () => {
        expect(wireFields.status).toBeDefined();
        expect(wireFields.status.required).toBe(false);
        expect(modelFields.status).toBeDefined();
        expect(modelFields.status.required).toBeUndefined();
    });

    it('all JSON fields exist in g8ed model', () => {
        const jsonKeys = Object.keys(wireFields);
        for (const field of jsonKeys) {
            expect(modelFields[field]).toBeDefined(
                `shared JSON defines field '${field}' but g8ed BoundOperatorContext does not have it`
            );
        }
    });

    it('all g8ed model fields exist in JSON', () => {
        const modelKeys = Object.keys(modelFields);
        const jsonKeys = Object.keys(wireFields);
        const extraFields = modelKeys.filter(key => !jsonKeys.includes(key));
        
        expect(extraFields.length).toBe(0,
            `g8ed BoundOperatorContext has fields not in shared JSON: ${extraFields.join(', ')}. ` +
            'Add them to shared/models/wire/bound_operator_context.json first.'
        );
    });
});
