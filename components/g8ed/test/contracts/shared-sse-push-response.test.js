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
 * SSE Push Response Wire Protocol Contract Tests
 *
 * Verifies that g8ed's SSEPushResponse model fields match the canonical
 * wire shape defined in shared/models/wire/sse_responses.json.
 *
 * Prevents desynchronization between g8ed's SSEPushResponse.forWire()
 * output and g8ee's SSEPushResponse parsing logic.
 */

import { describe, it, expect } from 'vitest';
import { SSEPushResponse } from '@g8ed/models/response_models.js';
import { readFileSync } from 'fs';
import { join } from 'path';

const wire = JSON.parse(
    readFileSync(join(process.cwd(), '../../shared/models/wire/sse_responses.json'), 'utf-8')
);

describe('g8ed SSEPushResponse matches shared/models/wire/sse_responses.json', () => {
    const wireFields = wire.sse_push_response.fields;
    const modelFields = SSEPushResponse.fields;

    it('success field exists and is required', () => {
        expect(wireFields.success).toBeDefined();
        expect(wireFields.success.required).toBe(true);
        expect(modelFields.success).toBeDefined();
        expect(modelFields.success.required).toBe(true);
    });

    it('delivered field exists and is optional with default 0 and min 0', () => {
        expect(wireFields.delivered).toBeDefined();
        expect(wireFields.delivered.required).toBe(false);
        expect(wireFields.delivered.default).toBe(0);
        expect(wireFields.delivered.minimum).toBe(0);
        expect(modelFields.delivered).toBeDefined();
        expect(modelFields.delivered.required).toBeUndefined();
        expect(modelFields.delivered.default).toBe(0);
        expect(modelFields.delivered.min).toBe(0);
    });

    it('error field exists and is optional with default null', () => {
        expect(wireFields.error).toBeDefined();
        expect(wireFields.error.required).toBe(false);
        expect(wireFields.error.default).toBe(null);
        expect(modelFields.error).toBeDefined();
        expect(modelFields.error.required).toBeUndefined();
        expect(modelFields.error.default).toBe(null);
    });

    it('all JSON fields exist in g8ed model', () => {
        const jsonKeys = Object.keys(wireFields);
        for (const field of jsonKeys) {
            expect(modelFields[field]).toBeDefined(
                `shared JSON defines field '${field}' but g8ed SSEPushResponse does not have it`
            );
        }
    });

    it('all g8ed model fields exist in JSON', () => {
        const modelKeys = Object.keys(modelFields);
        const jsonKeys = Object.keys(wireFields);
        const extraFields = modelKeys.filter(key => !jsonKeys.includes(key));
        
        expect(extraFields.length).toBe(0,
            `g8ed SSEPushResponse has fields not in shared JSON: ${extraFields.join(', ')}. ` +
            'Add them to shared/models/wire/sse_responses.json first.'
        );
    });
});
