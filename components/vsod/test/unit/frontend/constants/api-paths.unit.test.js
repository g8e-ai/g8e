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
import { ApiPaths } from '@vsod/public/js/constants/api-paths.js';

describe('ApiPaths [UNIT]', () => {

    describe('chat paths', () => {
        it('send() returns /api/chat/send', () => {
            expect(ApiPaths.chat.send()).toBe('/api/chat/send');
        });

        it('send() includes the /send segment (regression: was /api/chat)', () => {
            expect(ApiPaths.chat.send()).not.toBe('/api/chat');
        });

        it('send() ends with /send', () => {
            expect(ApiPaths.chat.send().endsWith('/send')).toBe(true);
        });

        it('investigations() returns /api/chat/investigations', () => {
            expect(ApiPaths.chat.investigations()).toBe('/api/chat/investigations');
        });

        it('investigation(id) returns /api/chat/investigations/:id', () => {
            expect(ApiPaths.chat.investigation('inv-abc')).toBe('/api/chat/investigations/inv-abc');
        });

        it('investigation(id) interpolates the id correctly', () => {
            const id = 'inv-xyz-123';
            expect(ApiPaths.chat.investigation(id)).toBe(`/api/chat/investigations/${id}`);
        });

        it('stop() returns /api/chat/stop', () => {
            expect(ApiPaths.chat.stop()).toBe('/api/chat/stop');
        });

        it('all chat paths share the /api/chat base', () => {
            expect(ApiPaths.chat.send()).toMatch(/^\/api\/chat\//);
            expect(ApiPaths.chat.investigations()).toMatch(/^\/api\/chat\//);
            expect(ApiPaths.chat.investigation('x')).toMatch(/^\/api\/chat\//);
            expect(ApiPaths.chat.stop()).toMatch(/^\/api\/chat\//);
        });

        it('send() and stop() are distinct paths', () => {
            expect(ApiPaths.chat.send()).not.toBe(ApiPaths.chat.stop());
        });

        it('send() and investigations() are distinct paths', () => {
            expect(ApiPaths.chat.send()).not.toBe(ApiPaths.chat.investigations());
        });

        it('investigations() and investigation(id) are distinct paths', () => {
            expect(ApiPaths.chat.investigations()).not.toBe(ApiPaths.chat.investigation('inv-1'));
        });

        it('investigation(id) is a sub-path of investigations()', () => {
            expect(ApiPaths.chat.investigation('inv-1').startsWith(ApiPaths.chat.investigations())).toBe(true);
        });

        it('all path builders are functions', () => {
            expect(typeof ApiPaths.chat.send).toBe('function');
            expect(typeof ApiPaths.chat.investigations).toBe('function');
            expect(typeof ApiPaths.chat.investigation).toBe('function');
            expect(typeof ApiPaths.chat.stop).toBe('function');
        });

        it('all paths start with /', () => {
            expect(ApiPaths.chat.send().startsWith('/')).toBe(true);
            expect(ApiPaths.chat.investigations().startsWith('/')).toBe(true);
            expect(ApiPaths.chat.investigation('x').startsWith('/')).toBe(true);
            expect(ApiPaths.chat.stop().startsWith('/')).toBe(true);
        });
    });

    describe('operator paths', () => {
        it('bind() returns /api/operators/bind', () => {
            expect(ApiPaths.operator.bind()).toBe('/api/operators/bind');
        });

        it('unbind() returns /api/operators/unbind', () => {
            expect(ApiPaths.operator.unbind()).toBe('/api/operators/unbind');
        });

        it('list() returns /api/operators', () => {
            expect(ApiPaths.operator.list()).toBe('/api/operators');
        });

        it('details(id) interpolates operator id', () => {
            expect(ApiPaths.operator.details('op-1')).toBe('/api/operators/op-1/details');
        });

        it('stop(id) interpolates operator id', () => {
            expect(ApiPaths.operator.stop('op-1')).toBe('/api/operators/op-1/stop');
        });
    });

    describe('auth paths', () => {
        it('webSession() returns /api/auth/web-session', () => {
            expect(ApiPaths.auth.webSession()).toBe('/api/auth/web-session');
        });

        it('logout() returns /api/auth/logout', () => {
            expect(ApiPaths.auth.logout()).toBe('/api/auth/logout');
        });

        it('register() returns /api/auth/register', () => {
            expect(ApiPaths.auth.register()).toBe('/api/auth/register');
        });

        it('passkey.authVerify() returns /api/auth/passkey/auth-verify', () => {
            expect(ApiPaths.auth.passkey.authVerify()).toBe('/api/auth/passkey/auth-verify');
        });
    });

    describe('user paths', () => {
        it('me() returns /api/user/me', () => {
            expect(ApiPaths.user.me()).toBe('/api/user/me');
        });
    });

    describe('deviceLink paths', () => {
        it('list() returns /api/device-links', () => {
            expect(ApiPaths.deviceLink.list()).toBe('/api/device-links');
        });

        it('revoke(tokenId) interpolates the token', () => {
            expect(ApiPaths.deviceLink.revoke('tok-1')).toBe('/api/device-links/tok-1');
        });
    });

    describe('sse paths', () => {
        it('events() returns /sse/events', () => {
            expect(ApiPaths.sse.events()).toBe('/sse/events');
        });
    });

    describe('approval paths', () => {
        it('respond() returns /api/operator/approval/respond', () => {
            expect(ApiPaths.approval.respond()).toBe('/api/operator/approval/respond');
        });

        it('directCommand() returns /api/operator/approval/direct-command', () => {
            expect(ApiPaths.approval.directCommand()).toBe('/api/operator/approval/direct-command');
        });
    });

    describe('server-side mirror contract', () => {
        it('chat.send() matches the server-side ApiPaths.chat.send()', async () => {
            const { ApiPaths: ServerApiPaths } = await import('@vsod/constants/api_paths.js');
            expect(ApiPaths.chat.send()).toBe(ServerApiPaths.chat.send());
        });

        it('chat.investigations() matches server-side', async () => {
            const { ApiPaths: ServerApiPaths } = await import('@vsod/constants/api_paths.js');
            expect(ApiPaths.chat.investigations()).toBe(ServerApiPaths.chat.investigations());
        });

        it('chat.investigation(id) matches server-side', async () => {
            const { ApiPaths: ServerApiPaths } = await import('@vsod/constants/api_paths.js');
            expect(ApiPaths.chat.investigation('inv-test')).toBe(ServerApiPaths.chat.investigation('inv-test'));
        });

        it('chat.stop() matches server-side', async () => {
            const { ApiPaths: ServerApiPaths } = await import('@vsod/constants/api_paths.js');
            expect(ApiPaths.chat.stop()).toBe(ServerApiPaths.chat.stop());
        });
    });
});
