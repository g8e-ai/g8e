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
 * Route unit test fixtures
 *
 * Provides standard Express app builders for route unit tests.
 * All route unit tests should use these instead of ad-hoc buildApp() closures.
 *
 * Usage:
 *
 *   import { buildUnauthenticatedApp, buildAuthenticatedApp } from '@test/fixtures/route.fixture.js';
 *
 *   // Unauthenticated — for public/setup endpoints (requireFirstRun path)
 *   app = buildUnauthenticatedApp(myRouter);
 *
 *   // Authenticated — for session-required endpoints (requireAuth path)
 *   app = buildAuthenticatedApp(myRouter, { userId: 'user-1' });
 *   app = buildAuthenticatedApp(myRouter, { userId: 'admin-1', roles: [UserRole.SUPERADMIN] });
 */

import express from 'express';
import cookieParser from 'cookie-parser';
import { UserRole } from '@vsod/constants/auth.js';

const DEFAULT_SESSION = {
    id:              'web_session_test_abc123',
    user_id:         'user-1',
    organization_id: 'org-1',
    user_data: {
        email: 'user@example.com',
        name:  'Test User',
        roles: [UserRole.USER],
    },
    is_active:    true,
    session_type: 'web',
};

/**
 * Build an Express app with no session attached.
 * Use for unauthenticated endpoints (setup flow, public routes).
 *
 * @param {express.Router} router
 * @param {string} [mountPath='/']
 * @returns {express.Application}
 */
export function buildUnauthenticatedApp(router, mountPath = '/') {
    const app = express();
    app.use(cookieParser());
    app.use(express.json());
    app.use(mountPath, router);
    return app;
}

/**
 * Build an Express app with a fully authenticated session pre-attached.
 * Simulates what requireAuth does after successful session validation.
 *
 * @param {express.Router} router
 * @param {{ userId?: string, webSessionId?: string, roles?: string[], sessionOverrides?: object }} [opts]
 * @param {string} [mountPath='/']
 * @returns {express.Application}
 */
export function buildAuthenticatedApp(router, opts = {}, mountPath = '/') {
    const {
        userId         = DEFAULT_SESSION.user_id,
        webSessionId   = DEFAULT_SESSION.id,
        roles          = DEFAULT_SESSION.user_data.roles,
        sessionOverrides = {},
    } = opts;

    const app = express();
    app.use(cookieParser());
    app.use(express.json());
    app.use((req, _res, next) => {
        req.userId      = userId;
        req.webSessionId = webSessionId;
        req.session     = {
            ...DEFAULT_SESSION,
            id:      webSessionId,
            user_id: userId,
            user_data: {
                ...DEFAULT_SESSION.user_data,
                roles,
            },
            ...sessionOverrides,
        };
        next();
    });
    app.use(mountPath, router);
    return app;
}
