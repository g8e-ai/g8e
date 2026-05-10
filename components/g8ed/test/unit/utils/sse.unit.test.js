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

import { describe, it, expect, vi } from 'vitest';
import { writeSSEFrame } from '../../../utils/sse.js';
import { G8eBaseModel } from '../../../models/base.js';
import { SSE_FRAME_TERMINATOR } from '../../../constants/service_config.js';

class MockEventModel extends G8eBaseModel {
    constructor(data) {
        super();
        this.data = data;
    }
    forWire() {
        return { content: this.data };
    }
}

describe('sse utils', () => {
    describe('writeSSEFrame', () => {
        it('should write a correctly formatted SSE frame', () => {
            const res = {
                write: vi.fn(),
                flush: vi.fn()
            };
            const eventData = new MockEventModel('hello');
            
            writeSSEFrame(res, eventData);
            
            const expected = `data: ${JSON.stringify({ content: 'hello' })}${SSE_FRAME_TERMINATOR}`;
            expect(res.write).toHaveBeenCalledWith(expected);
            expect(res.flush).toHaveBeenCalled();
        });

        it('should throw if eventData is not a G8eBaseModel', () => {
            const res = { write: vi.fn() };
            expect(() => writeSSEFrame(res, { some: 'object' })).toThrow(/requires a G8eBaseModel instance/);
        });

        it('should handle missing flush function', () => {
            const res = { write: vi.fn() }; // no flush
            const eventData = new MockEventModel('test');
            
            expect(() => writeSSEFrame(res, eventData)).not.toThrow();
            expect(res.write).toHaveBeenCalled();
        });
    });
});
