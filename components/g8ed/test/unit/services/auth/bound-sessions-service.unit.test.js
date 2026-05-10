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
import { BoundSessionsService } from '@g8ed/services/auth/bound_sessions_service.js';
import { KVKey } from '@g8ed/constants/kv_keys.js';
import { OperatorStatus, OperatorType } from '@g8ed/constants/operator.js';

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}));

vi.mock('@g8ed/models/base.js', async (importOriginal) => {
    const actual = await importOriginal();
    return { ...actual, now: () => new Date('2026-01-01T00:00:00Z') };
});

function makeCacheAside() {
    return {
        getDocument: vi.fn().mockResolvedValue(null),
        createDocument: vi.fn().mockResolvedValue(undefined),
        updateDocument: vi.fn().mockResolvedValue(undefined),
        kvKeys: vi.fn().mockResolvedValue([]),
    };
}

function makeOperatorService(overrides = {}) {
    return {
        getOperator: vi.fn().mockResolvedValue(null),
        ...overrides,
    };
}

function makeService({ cacheAsideService, operatorService } = {}) {
    return new BoundSessionsService({
        cacheAsideService: cacheAsideService ?? makeCacheAside(),
        operatorService: operatorService ?? makeOperatorService(),
    });
}

describe('BoundSessionsService constructor', () => {
    it('throws when cacheAsideService is missing', () => {
        expect(() => new BoundSessionsService({
            operatorService: makeOperatorService(),
        })).toThrow('cacheAsideService is required');
    });

    it('throws when operatorService is missing', () => {
        expect(() => new BoundSessionsService({
            cacheAsideService: makeCacheAside(),
        })).toThrow('operatorService is required');
    });
});

describe('BoundSessionsService.bind / unbind / getBoundOperatorSessionIds / getWebSessionForOperator', () => {
    let cacheAside;
    let svc;

    beforeEach(() => {
        cacheAside = makeCacheAside();
        // Add missing KV methods to mock
        cacheAside.kvSet = vi.fn().mockResolvedValue(undefined);
        cacheAside.kvSadd = vi.fn().mockResolvedValue(undefined);
        cacheAside.kvDel = vi.fn().mockResolvedValue(undefined);
        cacheAside.kvSrem = vi.fn().mockResolvedValue(undefined);
        cacheAside.kvScard = vi.fn().mockResolvedValue(0);
        cacheAside.kvSmembers = vi.fn().mockResolvedValue([]);
        cacheAside.kvGet = vi.fn().mockResolvedValue(null);
        
        svc = makeService({ cacheAsideService: cacheAside });
    });

    it('bind writes bidirectional KV entries', async () => {
        await svc.bind('op-sess-1', 'web-sess-1', 'user-1');

        expect(cacheAside.kvSadd).toHaveBeenCalledWith(KVKey.sessionWebBind('web-sess-1'), 'op-sess-1');
        expect(cacheAside.kvSet).toHaveBeenCalledWith(KVKey.sessionBindOperators('op-sess-1'), 'web-sess-1');
    });

    it('bind creates a BoundSessionsDocument when none exists', async () => {
        await svc.bind('op-sess-1', 'web-sess-1', 'user-1');
        expect(cacheAside.createDocument).toHaveBeenCalledOnce();
    });

    it('bind updates existing BoundSessionsDocument', async () => {
        cacheAside.getDocument.mockResolvedValue({
            operator_session_ids: ['op-sess-existing'],
        });
        await svc.bind('op-sess-1', 'web-sess-1', 'user-1');
        expect(cacheAside.updateDocument).toHaveBeenCalledOnce();
    });

    it('unbind removes KV entries', async () => {
        await svc.unbind('op-sess-1', 'web-sess-1');

        expect(cacheAside.kvDel).toHaveBeenCalledWith(KVKey.sessionBindOperators('op-sess-1'));
        expect(cacheAside.kvSrem).toHaveBeenCalledWith(KVKey.sessionWebBind('web-sess-1'), 'op-sess-1');
    });

    it('getBoundOperatorSessionIds returns empty array when nothing bound', async () => {
        const ids = await svc.getBoundOperatorSessionIds('web-sess-1');
        expect(ids).toEqual([]);
    });

    it('getBoundOperatorSessionIds returns all bound operator session IDs', async () => {
        cacheAside.kvSmembers = vi.fn().mockResolvedValueOnce(['op-sess-1', 'op-sess-2']);

        const ids = await svc.getBoundOperatorSessionIds('web-sess-1');
        expect(ids).toContain('op-sess-1');
        expect(ids).toContain('op-sess-2');
    });

    it('getWebSessionForOperator returns null when not bound', async () => {
        const result = await svc.getWebSessionForOperator('op-sess-unknown');
        expect(result).toBeNull();
    });

    it('getWebSessionForOperator returns correct web session ID', async () => {
        cacheAside.kvGet.mockResolvedValueOnce('web-sess-1');
        const result = await svc.getWebSessionForOperator('op-sess-1');
        expect(result).toBe('web-sess-1');
    });
});

describe('BoundSessionsService.resolveBoundOperators', () => {
    let cacheAside;
    let svc;
    let operatorService;

    const WEB_SESSION_ID = 'web-sess-abc123def456';
    const OP_SESSION_ID = 'op-sess-xyz789uvw012';
    const OPERATOR_ID = 'operator-id-001';

    function makeBindingDoc(overrides = {}) {
        return {
            status: 'active',
            operator_ids: [OPERATOR_ID],
            operator_session_ids: [OP_SESSION_ID],
            ...overrides,
        };
    }

    beforeEach(() => {
        cacheAside = makeCacheAside();
        operatorService = makeOperatorService();
        svc = makeService({ cacheAsideService: cacheAside, operatorService });
    });

    it('returns empty array when binding document does not exist', async () => {
        const result = await svc.resolveBoundOperators(WEB_SESSION_ID);
        expect(result).toEqual([]);
    });

    it('returns empty array when binding document status is not active', async () => {
        cacheAside.getDocument.mockResolvedValueOnce(makeBindingDoc({ status: 'ended' }));

        const result = await svc.resolveBoundOperators(WEB_SESSION_ID);
        expect(result).toEqual([]);
    });

    it('returns empty array when operator_ids is empty', async () => {
        cacheAside.getDocument.mockResolvedValueOnce(makeBindingDoc({ operator_ids: [], operator_session_ids: [] }));

        const result = await svc.resolveBoundOperators(WEB_SESSION_ID);
        expect(result).toEqual([]);
    });

    it('skips operator when operator document is not found', async () => {
        cacheAside.getDocument.mockResolvedValueOnce(makeBindingDoc());
        operatorService.getOperator.mockResolvedValueOnce(null);

        const result = await svc.resolveBoundOperators(WEB_SESSION_ID);
        expect(result).toEqual([]);
    });

    it('returns resolved BoundOperatorContext for a valid bound operator', async () => {
        cacheAside.getDocument.mockResolvedValueOnce(makeBindingDoc());
        operatorService.getOperator.mockResolvedValueOnce({
            status: OperatorStatus.BOUND,
            operator_type: OperatorType.SYSTEM,
        });

        const result = await svc.resolveBoundOperators(WEB_SESSION_ID);

        expect(result).toHaveLength(1);
        expect(result[0]).toMatchObject({
            operator_id: OPERATOR_ID,
            operator_session_id: OP_SESSION_ID,
            status: OperatorStatus.BOUND,
        });
        expect(result[0].operator_type).toBeUndefined();
    });

    it('fetches operators in parallel via Promise.all', async () => {
        const OP_SESSION_2 = 'op-sess-second0000000';
        const OPERATOR_ID_2 = 'operator-id-002';

        cacheAside.getDocument.mockResolvedValueOnce(makeBindingDoc({
            operator_ids: [OPERATOR_ID, OPERATOR_ID_2],
            operator_session_ids: [OP_SESSION_ID, OP_SESSION_2],
        }));

        operatorService.getOperator
            .mockResolvedValueOnce({ status: OperatorStatus.BOUND, operator_type: OperatorType.SYSTEM })
            .mockResolvedValueOnce({ status: OperatorStatus.ACTIVE, operator_type: OperatorType.SYSTEM });

        const result = await svc.resolveBoundOperators(WEB_SESSION_ID);

        expect(result).toHaveLength(2);
        expect(result.map(r => r.operator_id)).toContain(OPERATOR_ID);
        expect(result.map(r => r.operator_id)).toContain(OPERATOR_ID_2);
        expect(operatorService.getOperator).toHaveBeenCalledTimes(2);
    });

    it('reads binding document from cacheAside (cache-aside pattern)', async () => {
        cacheAside.getDocument.mockResolvedValueOnce(makeBindingDoc());
        operatorService.getOperator.mockResolvedValueOnce({
            status: OperatorStatus.ACTIVE,
            operator_type: OperatorType.SYSTEM,
        });

        await svc.resolveBoundOperators(WEB_SESSION_ID);

        expect(cacheAside.getDocument).toHaveBeenCalledWith('bound_sessions', WEB_SESSION_ID);
    });

    it('does not call validateSession (regression)', async () => {
        cacheAside.getDocument.mockResolvedValueOnce(makeBindingDoc());
        operatorService.getOperator.mockResolvedValueOnce({
            status: OperatorStatus.ACTIVE,
            operator_type: OperatorType.SYSTEM,
        });

        await svc.resolveBoundOperators(WEB_SESSION_ID);

        expect(svc).not.toHaveProperty('operatorSessionService');
    });

    describe('resolveBoundOperatorsForUser', () => {
        const USER_ID = 'user_123';
        const WEB_SESSION_ID = 'ws_123';
        const WEB_SESSION_ID_2 = 'ws_456';
        const OPERATOR_ID = 'op_123';
        const OPERATOR_ID_2 = 'op_456';
        const OP_SESSION_ID = 'ops_123';
        const OP_SESSION_2 = 'ops_456';

        it('returns empty array when no bound sessions exist for user', async () => {
            cacheAside.kvKeys.mockResolvedValue([]);
            operatorService.getOperator.mockResolvedValue({
                status: OperatorStatus.ACTIVE,
                operator_type: OperatorType.SYSTEM,
            });

            const result = await svc.resolveBoundOperatorsForUser(USER_ID);

            expect(result).toEqual([]);
            expect(cacheAside.kvKeys).toHaveBeenCalledWith('bound_sessions:*');
        });

        it('resolves bound operators from all user sessions', async () => {
            cacheAside.kvKeys.mockResolvedValue([
                'bound_sessions:ws_123',
                'bound_sessions:ws_456',
            ]);
            cacheAside.getDocument
                .mockResolvedValueOnce({
                    id: WEB_SESSION_ID,
                    web_session_id: WEB_SESSION_ID,
                    user_id: USER_ID,
                    status: 'active',
                    operator_ids: [OPERATOR_ID],
                    operator_session_ids: [OP_SESSION_ID],
                })
                .mockResolvedValueOnce({
                    id: WEB_SESSION_ID_2,
                    web_session_id: WEB_SESSION_ID_2,
                    user_id: USER_ID,
                    status: 'active',
                    operator_ids: [OPERATOR_ID_2],
                    operator_session_ids: [OP_SESSION_2],
                });
            operatorService.getOperator
                .mockResolvedValueOnce({
                    id: OPERATOR_ID,
                    status: OperatorStatus.ACTIVE,
                    operator_type: OperatorType.SYSTEM,
                })
                .mockResolvedValueOnce({
                    id: OPERATOR_ID_2,
                    status: OperatorStatus.BOUND,
                    operator_type: OperatorType.SYSTEM,
                });

            const result = await svc.resolveBoundOperatorsForUser(USER_ID);

            expect(result).toHaveLength(2);
            expect(result.map(r => r.operator_id)).toContain(OPERATOR_ID);
            expect(result.map(r => r.operator_id)).toContain(OPERATOR_ID_2);
        });

        it('filters out sessions for different users', async () => {
            cacheAside.kvKeys.mockResolvedValue(['bound_sessions:ws_123', 'bound_sessions:ws_456']);
            cacheAside.getDocument
                .mockResolvedValueOnce({
                    id: WEB_SESSION_ID,
                    web_session_id: WEB_SESSION_ID,
                    user_id: USER_ID,
                    status: 'active',
                    operator_ids: [OPERATOR_ID],
                    operator_session_ids: [OP_SESSION_ID],
                })
                .mockResolvedValueOnce({
                    id: WEB_SESSION_ID_2,
                    web_session_id: WEB_SESSION_ID_2,
                    user_id: 'different_user',
                    status: 'active',
                    operator_ids: [OPERATOR_ID_2],
                    operator_session_ids: [OP_SESSION_2],
                });
            operatorService.getOperator.mockResolvedValue({
                id: OPERATOR_ID,
                status: OperatorStatus.ACTIVE,
                operator_type: OperatorType.SYSTEM,
            });

            const result = await svc.resolveBoundOperatorsForUser(USER_ID);

            expect(result).toHaveLength(1);
            expect(result[0].operator_id).toBe(OPERATOR_ID);
        });

        it('filters out inactive sessions', async () => {
            cacheAside.kvKeys.mockResolvedValue(['bound_sessions:ws_123']);
            cacheAside.getDocument.mockResolvedValue({
                id: WEB_SESSION_ID,
                web_session_id: WEB_SESSION_ID,
                user_id: USER_ID,
                status: 'inactive',
                operator_ids: [OPERATOR_ID],
                operator_session_ids: [OP_SESSION_ID],
            });
            operatorService.getOperator.mockResolvedValue({
                id: OPERATOR_ID,
                status: OperatorStatus.ACTIVE,
                operator_type: OperatorType.SYSTEM,
            });

            const result = await svc.resolveBoundOperatorsForUser(USER_ID);

            expect(result).toEqual([]);
        });

        it('skips operators that are not found', async () => {
            cacheAside.kvKeys.mockResolvedValue(['bound_sessions:ws_123']);
            cacheAside.getDocument.mockResolvedValue({
                id: WEB_SESSION_ID,
                web_session_id: WEB_SESSION_ID,
                user_id: USER_ID,
                status: 'active',
                operator_ids: [OPERATOR_ID, OPERATOR_ID_2],
                operator_session_ids: [OP_SESSION_ID, OP_SESSION_2],
            });
            operatorService.getOperator
                .mockResolvedValueOnce({
                    id: OPERATOR_ID,
                    status: OperatorStatus.ACTIVE,
                    operator_type: OperatorType.SYSTEM,
                })
                .mockResolvedValueOnce(null);

            const result = await svc.resolveBoundOperatorsForUser(USER_ID);

            expect(result).toHaveLength(1);
            expect(result[0].operator_id).toBe(OPERATOR_ID);
        });
    });
});
