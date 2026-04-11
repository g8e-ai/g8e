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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { OperatorStatus } from '@g8ed/public/js/constants/operator-constants.js';

let operatorSessionService;

beforeEach(async () => {
    vi.resetModules();
    ({ operatorSessionService } = await import('@g8ed/public/js/utils/operator-session-service.js'));
});

function makeOperator(overrides = {}) {
    return {
        operator_id: 'op-abc',
        web_session_id: 'ws-xyz',
        status: OperatorStatus.BOUND,
        ...overrides,
    };
}

describe('OperatorSessionService — initial state [UNIT]', () => {
    it('getBoundOperatorId() returns null before any operators are set', () => {
        expect(operatorSessionService.getBoundOperatorId()).toBeNull();
    });

    it('getBoundOperators() returns an empty array before any operators are set', () => {
        expect(operatorSessionService.getBoundOperators()).toEqual([]);
    });

    it('isBound() returns false with no operators set', () => {
        expect(operatorSessionService.isBound()).toBe(false);
    });

    it('getBoundOperatorForSession() returns null with no operators set', () => {
        expect(operatorSessionService.getBoundOperatorForSession('ws-xyz')).toBeNull();
    });
});

describe('OperatorSessionService — setBoundOperators() [UNIT]', () => {
    it('throws when passed a non-array', () => {
        expect(() => operatorSessionService.setBoundOperators(null)).toThrow(
            'OperatorSessionService.setBoundOperators requires an array'
        );
    });

    it('throws when passed an object', () => {
        expect(() => operatorSessionService.setBoundOperators({ operator_id: 'op-1' })).toThrow(
            'OperatorSessionService.setBoundOperators requires an array'
        );
    });

    it('throws when passed a string', () => {
        expect(() => operatorSessionService.setBoundOperators('op-1')).toThrow(
            'OperatorSessionService.setBoundOperators requires an array'
        );
    });

    it('accepts an empty array without throwing', () => {
        expect(() => operatorSessionService.setBoundOperators([])).not.toThrow();
    });

    it('stores the operators array', () => {
        const ops = [makeOperator()];
        operatorSessionService.setBoundOperators(ops);
        expect(operatorSessionService.getBoundOperators()).toBe(ops);
    });

    it('sets _boundOperatorId to the operator_id of the first BOUND operator', () => {
        operatorSessionService.setBoundOperators([makeOperator({ operator_id: 'op-1' })]);
        expect(operatorSessionService.getBoundOperatorId()).toBe('op-1');
    });

    it('ignores non-BOUND operators when deriving _boundOperatorId', () => {
        operatorSessionService.setBoundOperators([
            makeOperator({ operator_id: 'op-active', status: OperatorStatus.ACTIVE }),
            makeOperator({ operator_id: 'op-offline', status: OperatorStatus.OFFLINE }),
        ]);
        expect(operatorSessionService.getBoundOperatorId()).toBeNull();
    });

    it('picks the first BOUND operator when multiple are present', () => {
        operatorSessionService.setBoundOperators([
            makeOperator({ operator_id: 'op-1', status: OperatorStatus.BOUND }),
            makeOperator({ operator_id: 'op-2', status: OperatorStatus.BOUND }),
        ]);
        expect(operatorSessionService.getBoundOperatorId()).toBe('op-1');
    });

    it('sets _boundOperatorId to null when no BOUND operator exists in the list', () => {
        operatorSessionService.setBoundOperators([
            makeOperator({ status: OperatorStatus.ACTIVE }),
        ]);
        expect(operatorSessionService.getBoundOperatorId()).toBeNull();
    });

    it('sets _boundOperatorId to null for an empty array', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        operatorSessionService.setBoundOperators([]);
        expect(operatorSessionService.getBoundOperatorId()).toBeNull();
    });

    it('replaces the previous operator list on a second call', () => {
        operatorSessionService.setBoundOperators([makeOperator({ operator_id: 'op-old' })]);
        operatorSessionService.setBoundOperators([makeOperator({ operator_id: 'op-new' })]);
        expect(operatorSessionService.getBoundOperatorId()).toBe('op-new');
        expect(operatorSessionService.getBoundOperators().length).toBe(1);
    });
});

describe('OperatorSessionService — clearBoundOperators() [UNIT]', () => {
    it('resets getBoundOperators() to an empty array', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        operatorSessionService.clearBoundOperators();
        expect(operatorSessionService.getBoundOperators()).toEqual([]);
    });

    it('resets getBoundOperatorId() to null', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        operatorSessionService.clearBoundOperators();
        expect(operatorSessionService.getBoundOperatorId()).toBeNull();
    });

    it('is idempotent — calling twice does not throw', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        operatorSessionService.clearBoundOperators();
        expect(() => operatorSessionService.clearBoundOperators()).not.toThrow();
    });

    it('is safe to call with no operators ever set', () => {
        expect(() => operatorSessionService.clearBoundOperators()).not.toThrow();
        expect(operatorSessionService.getBoundOperators()).toEqual([]);
    });
});

describe('OperatorSessionService — getBoundOperatorId() [UNIT]', () => {
    it('returns the operator_id of the BOUND operator', () => {
        operatorSessionService.setBoundOperators([makeOperator({ operator_id: 'op-xyz' })]);
        expect(operatorSessionService.getBoundOperatorId()).toBe('op-xyz');
    });

    it('returns null after clearBoundOperators()', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        operatorSessionService.clearBoundOperators();
        expect(operatorSessionService.getBoundOperatorId()).toBeNull();
    });
});

describe('OperatorSessionService — getBoundOperators() [UNIT]', () => {
    it('returns the exact same array reference that was set', () => {
        const ops = [makeOperator()];
        operatorSessionService.setBoundOperators(ops);
        expect(operatorSessionService.getBoundOperators()).toBe(ops);
    });

    it('returns an empty array after clearBoundOperators()', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        operatorSessionService.clearBoundOperators();
        expect(operatorSessionService.getBoundOperators()).toEqual([]);
    });

    it('returns all operators regardless of status', () => {
        const ops = [
            makeOperator({ operator_id: 'op-1', status: OperatorStatus.BOUND }),
            makeOperator({ operator_id: 'op-2', status: OperatorStatus.ACTIVE }),
            makeOperator({ operator_id: 'op-3', status: OperatorStatus.OFFLINE }),
        ];
        operatorSessionService.setBoundOperators(ops);
        expect(operatorSessionService.getBoundOperators().length).toBe(3);
    });
});

describe('OperatorSessionService — isBound() [UNIT]', () => {
    it('returns true when a BOUND operator is set', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        expect(operatorSessionService.isBound()).toBe(true);
    });

    it('returns false when only non-BOUND operators are set', () => {
        operatorSessionService.setBoundOperators([
            makeOperator({ status: OperatorStatus.ACTIVE }),
        ]);
        expect(operatorSessionService.isBound()).toBe(false);
    });

    it('returns false after clearBoundOperators()', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        operatorSessionService.clearBoundOperators();
        expect(operatorSessionService.isBound()).toBe(false);
    });

    it('returns false when setBoundOperators is called with an empty array', () => {
        operatorSessionService.setBoundOperators([]);
        expect(operatorSessionService.isBound()).toBe(false);
    });

    it('reflects updated state when operators are replaced', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        expect(operatorSessionService.isBound()).toBe(true);
        operatorSessionService.setBoundOperators([makeOperator({ status: OperatorStatus.STALE })]);
        expect(operatorSessionService.isBound()).toBe(false);
    });
});

describe('OperatorSessionService — getBoundOperatorForSession() [UNIT]', () => {
    it('returns the matching BOUND operator for the given webSessionId', () => {
        const op = makeOperator({ web_session_id: 'ws-1' });
        operatorSessionService.setBoundOperators([op]);
        expect(operatorSessionService.getBoundOperatorForSession('ws-1')).toBe(op);
    });

    it('returns null when no operator matches the webSessionId', () => {
        operatorSessionService.setBoundOperators([makeOperator({ web_session_id: 'ws-other' })]);
        expect(operatorSessionService.getBoundOperatorForSession('ws-no-match')).toBeNull();
    });

    it('returns null when the matching operator is not BOUND', () => {
        operatorSessionService.setBoundOperators([
            makeOperator({ web_session_id: 'ws-1', status: OperatorStatus.STALE }),
        ]);
        expect(operatorSessionService.getBoundOperatorForSession('ws-1')).toBeNull();
    });

    it('returns null when webSessionId is null', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        expect(operatorSessionService.getBoundOperatorForSession(null)).toBeNull();
    });

    it('returns null when webSessionId is undefined', () => {
        operatorSessionService.setBoundOperators([makeOperator()]);
        expect(operatorSessionService.getBoundOperatorForSession(undefined)).toBeNull();
    });

    it('returns null when webSessionId is an empty string', () => {
        operatorSessionService.setBoundOperators([makeOperator({ web_session_id: '' })]);
        expect(operatorSessionService.getBoundOperatorForSession('')).toBeNull();
    });

    it('returns null with no operators set', () => {
        expect(operatorSessionService.getBoundOperatorForSession('ws-1')).toBeNull();
    });

    it('matches only by exact webSessionId — does not return a different session', () => {
        const op1 = makeOperator({ operator_id: 'op-1', web_session_id: 'ws-1' });
        const op2 = makeOperator({ operator_id: 'op-2', web_session_id: 'ws-2' });
        operatorSessionService.setBoundOperators([op1, op2]);
        expect(operatorSessionService.getBoundOperatorForSession('ws-2')).toBe(op2);
        expect(operatorSessionService.getBoundOperatorForSession('ws-1')).toBe(op1);
    });

    it('returns null after clearBoundOperators()', () => {
        operatorSessionService.setBoundOperators([makeOperator({ web_session_id: 'ws-1' })]);
        operatorSessionService.clearBoundOperators();
        expect(operatorSessionService.getBoundOperatorForSession('ws-1')).toBeNull();
    });
});
