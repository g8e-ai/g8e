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
import { PostLoginService } from '@g8ed/services/auth/post_login_service.js';

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}));

vi.mock('@g8ed/utils/security.js', () => ({
    getCookieDomain: vi.fn().mockReturnValue('localhost'),
}));

const SESSION_ID = 'sess_abc123';
const USER_ID = 'user-1';
const ORG_ID = 'org-1';
const DOWNLOAD_KEY = 'dl_key_abc';

function makeUser(overrides = {}) {
    return {
        id:              USER_ID,
        email:           'user@example.com',
        name:            'Test User',
        organization_id: ORG_ID,
        roles:           ['user'],
        ...overrides,
    };
}

function makeSession(overrides = {}) {
    return { id: SESSION_ID, user_id: USER_ID, ...overrides };
}

function makeReq(overrides = {}) {
    return {
        ip:      '127.0.0.1',
        headers: { 'user-agent': 'test-agent', 'x-forwarded-for': undefined },
        ...overrides,
    };
}

function makeRes() {
    const res = { cookie: vi.fn() };
    return res;
}

function makeService(overrides = {}) {
    return new PostLoginService({
        webSessionService: {
            createWebSession: vi.fn().mockResolvedValue(makeSession()),
        },
        apiKeyService: {},
        userService: {
            getUserG8eKey:    vi.fn().mockResolvedValue(DOWNLOAD_KEY),
            createUserG8eKey: vi.fn(),
        },
        operatorService: {
            initializeOperatorSlots: vi.fn().mockResolvedValue([]),
        },
        g8eNodeOperatorService: {
            activateG8ENodeOperatorForUser: vi.fn().mockResolvedValue(undefined),
        },
        ...overrides,
    });
}

describe('PostLoginService [UNIT]', () => {
    let service;

    beforeEach(() => {
        vi.clearAllMocks();
        service = makeService();
    });

    // -------------------------------------------------------------------------
    // createSessionAndSetCookie
    // -------------------------------------------------------------------------

    describe('createSessionAndSetCookie', () => {
        it('creates a web session and returns it', async () => {
            const req = makeReq();
            const res = makeRes();

            const session = await service.createSessionAndSetCookie(req, res, makeUser());

            expect(session.id).toBe(SESSION_ID);
            expect(service.webSessionService.createWebSession).toHaveBeenCalledOnce();
        });

        it('sets a session cookie on the response', async () => {
            const req = makeReq();
            const res = makeRes();

            await service.createSessionAndSetCookie(req, res, makeUser());

            expect(res.cookie).toHaveBeenCalledOnce();
            const [cookieName, cookieValue] = res.cookie.mock.calls[0];
            expect(cookieName).toMatch(/session/i);
            expect(cookieValue).toBe(SESSION_ID);
        });

        it('cookie is httpOnly, secure, and has a maxAge', async () => {
            const req = makeReq();
            const res = makeRes();

            await service.createSessionAndSetCookie(req, res, makeUser());

            const [, , opts] = res.cookie.mock.calls[0];
            expect(opts.httpOnly).toBe(true);
            expect(opts.secure).toBe(true);
            expect(opts.maxAge).toBeGreaterThan(0);
        });

        it('uses existing download API key when present', async () => {
            const req = makeReq();
            const res = makeRes();

            await service.createSessionAndSetCookie(req, res, makeUser());

            expect(service.userService.getUserG8eKey).toHaveBeenCalledWith(USER_ID);
            expect(service.userService.createUserG8eKey).not.toHaveBeenCalled();
        });

        it('creates a download API key when none exists', async () => {
            service.userService.getUserG8eKey = vi.fn().mockResolvedValue(null);
            service.userService.createUserG8eKey = vi.fn().mockResolvedValue({
                success: true,
                api_key: 'dl_new_key',
            });

            const req = makeReq();
            const res = makeRes();

            await service.createSessionAndSetCookie(req, res, makeUser());

            expect(service.userService.createUserG8eKey).toHaveBeenCalledWith(
                USER_ID,
                ORG_ID
            );
        });

        it('falls back to user.id as org when organization_id is absent', async () => {
            service.userService.getUserG8eKey = vi.fn().mockResolvedValue(null);
            service.userService.createUserG8eKey = vi.fn().mockResolvedValue({
                success: true,
                api_key: 'dl_new_key',
            });

            const req = makeReq();
            const res = makeRes();
            const user = makeUser({ organization_id: null });

            await service.createSessionAndSetCookie(req, res, user);

            expect(service.userService.createUserG8eKey).toHaveBeenCalledWith(
                USER_ID,
                USER_ID
            );
        });

        it('passes null api_key to createWebSession when key creation fails', async () => {
            service.userService.getUserG8eKey = vi.fn().mockResolvedValue(null);
            service.userService.createUserG8eKey = vi.fn().mockResolvedValue({ success: false });

            const req = makeReq();
            const res = makeRes();

            await service.createSessionAndSetCookie(req, res, makeUser());

            const sessionArgs = service.webSessionService.createWebSession.mock.calls[0][0];
            expect(sessionArgs.api_key).toBeNull();
        });

        it('passes ip and user-agent from request to createWebSession', async () => {
            const req = makeReq({ ip: '10.0.0.1', headers: { 'user-agent': 'Mozilla/5.0' } });
            const res = makeRes();

            await service.createSessionAndSetCookie(req, res, makeUser());

            const [, requestContext] = service.webSessionService.createWebSession.mock.calls[0];
            expect(requestContext.ip).toBe('10.0.0.1');
            expect(requestContext.userAgent).toBe('Mozilla/5.0');
        });
    });

    // -------------------------------------------------------------------------
    // onSuccessfulLogin
    // -------------------------------------------------------------------------

    describe('onSuccessfulLogin', () => {
        it('calls activateG8ENodeOperatorForUser with user_id, org_id, and session_id', async () => {
            await service.onSuccessfulLogin(makeUser(), makeSession());

            await vi.waitFor(() =>
                expect(service.g8eNodeOperatorService.activateG8ENodeOperatorForUser)
                    .toHaveBeenCalledWith(USER_ID, ORG_ID, SESSION_ID)
            );
        });

        it('calls initializeOperatorSlots with user_id and org_id', async () => {
            await service.onSuccessfulLogin(makeUser(), makeSession());

            await vi.waitFor(() =>
                expect(service.operatorService.initializeOperatorSlots)
                    .toHaveBeenCalledWith(USER_ID, ORG_ID)
            );
        });

        it('passes user.id as org when organization_id is absent', async () => {
            const user = makeUser({ organization_id: null });

            await service.onSuccessfulLogin(user, makeSession());

            await vi.waitFor(() => {
                expect(service.g8eNodeOperatorService.activateG8ENodeOperatorForUser)
                    .toHaveBeenCalledWith(USER_ID, null, SESSION_ID);
                expect(service.operatorService.initializeOperatorSlots)
                    .toHaveBeenCalledWith(USER_ID, USER_ID);
            });
        });

        it('resolves even when activateG8ENodeOperatorForUser rejects', async () => {
            service.g8eNodeOperatorService.activateG8ENodeOperatorForUser =
                vi.fn().mockRejectedValue(new Error('docker exec failed'));

            await expect(service.onSuccessfulLogin(makeUser(), makeSession()))
                .resolves.toBeUndefined();
        });

        it('resolves even when initializeOperatorSlots rejects', async () => {
            service.operatorService.initializeOperatorSlots =
                vi.fn().mockRejectedValue(new Error('g8es unavailable'));

            await expect(service.onSuccessfulLogin(makeUser(), makeSession()))
                .resolves.toBeUndefined();
        });
    });

    // -------------------------------------------------------------------------
    // onSuccessfulRegistration
    // -------------------------------------------------------------------------

    describe('onSuccessfulRegistration', () => {
        it('calls activateG8ENodeOperatorForUser with user_id, org_id, and session_id', async () => {
            await service.onSuccessfulRegistration(makeUser(), makeSession());

            await vi.waitFor(() =>
                expect(service.g8eNodeOperatorService.activateG8ENodeOperatorForUser)
                    .toHaveBeenCalledWith(USER_ID, ORG_ID, SESSION_ID)
            );
        });

        it('calls initializeOperatorSlots with user_id and org_id', async () => {
            await service.onSuccessfulRegistration(makeUser(), makeSession());

            await vi.waitFor(() =>
                expect(service.operatorService.initializeOperatorSlots)
                    .toHaveBeenCalledWith(USER_ID, ORG_ID)
            );
        });

        it('passes null org to activateG8ENodeOperatorForUser when organization_id is absent', async () => {
            const user = makeUser({ organization_id: null });

            await service.onSuccessfulRegistration(user, makeSession());

            await vi.waitFor(() =>
                expect(service.g8eNodeOperatorService.activateG8ENodeOperatorForUser)
                    .toHaveBeenCalledWith(USER_ID, null, SESSION_ID)
            );
        });

        it('resolves even when activateG8ENodeOperatorForUser rejects', async () => {
            service.g8eNodeOperatorService.activateG8ENodeOperatorForUser =
                vi.fn().mockRejectedValue(new Error('container not running'));

            await expect(service.onSuccessfulRegistration(makeUser(), makeSession()))
                .resolves.toBeUndefined();
        });

        it('resolves even when initializeOperatorSlots rejects', async () => {
            service.operatorService.initializeOperatorSlots =
                vi.fn().mockRejectedValue(new Error('slot init failed'));

            await expect(service.onSuccessfulRegistration(makeUser(), makeSession()))
                .resolves.toBeUndefined();
        });
    });

    // -------------------------------------------------------------------------
    // _initializeSlotsAndActivateG8eNode (sequential ordering)
    // -------------------------------------------------------------------------

    describe('_initializeSlotsAndActivateG8eNode (ordering)', () => {
        it('awaits initializeOperatorSlots before calling activateG8ENodeOperatorForUser', async () => {
            const callOrder = [];

            service.operatorService.initializeOperatorSlots = vi.fn().mockImplementation(async () => {
                callOrder.push('initializeOperatorSlots:start');
                await new Promise(r => setTimeout(r, 10));
                callOrder.push('initializeOperatorSlots:end');
                return [];
            });

            service.g8eNodeOperatorService.activateG8ENodeOperatorForUser = vi.fn().mockImplementation(async () => {
                callOrder.push('activateG8eNode:start');
            });

            await service._initializeSlotsAndActivateG8eNode(makeUser(), makeSession(), 'login');

            expect(callOrder).toEqual([
                'initializeOperatorSlots:start',
                'initializeOperatorSlots:end',
                'activateG8eNode:start',
            ]);
        });

        it('does not call activateG8ENodeOperatorForUser when initializeOperatorSlots rejects', async () => {
            service.operatorService.initializeOperatorSlots =
                vi.fn().mockRejectedValue(new Error('g8es unavailable'));

            await expect(
                service._initializeSlotsAndActivateG8eNode(makeUser(), makeSession(), 'login')
            ).rejects.toThrow('g8es unavailable');

            expect(service.g8eNodeOperatorService.activateG8ENodeOperatorForUser).not.toHaveBeenCalled();
        });

        it('propagates activateG8ENodeOperatorForUser errors to the caller', async () => {
            service.g8eNodeOperatorService.activateG8ENodeOperatorForUser =
                vi.fn().mockRejectedValue(new Error('supervisor unreachable'));

            await expect(
                service._initializeSlotsAndActivateG8eNode(makeUser(), makeSession(), 'login')
            ).rejects.toThrow('supervisor unreachable');
        });
    });
});
