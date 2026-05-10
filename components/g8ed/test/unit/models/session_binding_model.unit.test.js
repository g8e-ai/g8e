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
import { BoundSessionsDocument } from '@g8ed/models/session_binding_model.js';

describe('BoundSessionsDocument Model [UNIT]', () => {
    it('should have correct default values', () => {
        const doc = new BoundSessionsDocument({
            web_session_id: 'ws_123',
            user_id: 'u_123'
        });

        expect(doc.web_session_id).toBe('ws_123');
        expect(doc.user_id).toBe('u_123');
        expect(doc.operator_session_ids).toEqual([]);
        expect(doc.bound_at).toBeInstanceOf(Date);
        expect(doc.last_updated_at).toBeInstanceOf(Date);
    });

    it('should require mandatory fields', () => {
        expect(() => new BoundSessionsDocument({ web_session_id: 'ws_1' })).toThrow();
        expect(() => new BoundSessionsDocument({ user_id: 'u_1' })).toThrow();
    });

    it('should correctly parse from DB', () => {
        const dbData = {
            id: 'ws_123',
            web_session_id: 'ws_123',
            user_id: 'u_123',
            operator_session_ids: ['os_1', 'os_2'],
            bound_at: new Date().toISOString(),
            last_updated_at: new Date().toISOString()
        };

        const doc = BoundSessionsDocument.parse(dbData);
        expect(doc.web_session_id).toBe('ws_123');
        expect(doc.operator_session_ids).toHaveLength(2);
        expect(doc.operator_session_ids).toContain('os_1');
    });
});
