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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { JSDOM } from 'jsdom';
import { AuthManager } from '@g8ed/public/js/components/auth.js';
import { WebSessionModel } from '@g8ed/public/js/models/session-model.js';
import { UserRole, OperatorSessionRole } from '@g8ed/constants/auth.js';

function createUserSession({ roles = [], name = 'Demo User', email = 'demo@g8e.ai' } = {}) {
    return new WebSessionModel({
        id: 'session-test-123',
        user_id: 'user-test-456',
        is_active: true,
        expires_at: new Date(Date.now() + 86400000).toISOString(),
        user_data: { name, email, roles }
    });
}

describe('AuthManager renderUserProfile dropdown actions', () => {
    beforeEach(() => {
        const dom = new JSDOM('<!DOCTYPE html><html><body><div id="auth-button-container"></div></body></html>');
        global.window = dom.window;
        global.document = dom.window.document;

        vi.spyOn(AuthManager.prototype, 'init').mockImplementation(() => {});

        global.window.APP_CONFIG = {};
        global.window.sseConnectionManager = {
            isConnectionActiveFor: () => false,
            initializeConnection: () => {}
        };
    });

    afterEach(() => {
        vi.restoreAllMocks();
        delete global.window;
        delete global.document;
    });

    it('renders Settings link for all users above Console', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({}));

        const dropdown = document.querySelector('.profile-dropdown');
        expect(dropdown).toBeTruthy();

        const settingsLink = dropdown.querySelector('a.settings-link');
        expect(settingsLink).toBeTruthy();
        expect(settingsLink.textContent).toBe('Settings');
        expect(settingsLink.href).toContain('/settings');
        expect(settingsLink.className).toContain('profile-dropdown-action-link');
        expect(settingsLink.className).not.toContain('btn');
    });

    it('renders Settings link before Console for superadmin users', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({ roles: [UserRole.SUPERADMIN] }));

        const dropdown = document.querySelector('.profile-dropdown');
        const links = [...dropdown.querySelectorAll('.profile-dropdown-action-link')];
        const settingsIdx = links.findIndex(el => el.classList.contains('settings-link'));
        const consoleIdx = links.findIndex(el => el.classList.contains('console-link'));

        expect(settingsIdx).toBeGreaterThanOrEqual(0);
        expect(consoleIdx).toBeGreaterThan(settingsIdx);
    });

    it('renders plain-text Audit Log and Logout actions without button styling', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({}));

        const dropdown = document.querySelector('.profile-dropdown');
        expect(dropdown).toBeTruthy();

        const auditLink = dropdown.querySelector('a.audit-log-link');
        expect(auditLink).toBeTruthy();
        expect(auditLink.textContent).toBe('Audit Log');
        expect(auditLink.className).toContain('profile-dropdown-action-link');
        expect(auditLink.className).not.toContain('btn');

        const logoutButton = dropdown.querySelector('button.logout-link');
        expect(logoutButton).toBeTruthy();
        expect(logoutButton.textContent).toBe('Logout');
        expect(logoutButton.className).toContain('profile-dropdown-action-link');
        expect(logoutButton.className).not.toContain('btn');

        const dropdownButtons = dropdown.querySelectorAll('.btn');
        expect(dropdownButtons.length).toBe(0);
    });

    it('does not render SaaS-era Upgrade or Support links', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({}));

        const upgradeLink = document.querySelector('.profile-dropdown a.upgrade-plan-link');
        expect(upgradeLink).toBeNull();

        const supportLink = document.querySelector('.profile-dropdown a.support-link');
        expect(supportLink).toBeNull();
    });

    it('renders Console link only for superadmin users', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({ roles: [UserRole.SUPERADMIN] }));

        const consoleLink = document.querySelector('.profile-dropdown a.console-link');
        expect(consoleLink).toBeTruthy();
        expect(consoleLink.textContent).toBe('Console');
    });

    it('does not render Console link for non-superadmin users', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({}));

        const consoleLink = document.querySelector('.profile-dropdown a.console-link');
        expect(consoleLink).toBeNull();
    });
});

describe('AuthManager renderUserProfile — email display via getEmail()', () => {
    beforeEach(() => {
        const dom = new JSDOM('<!DOCTYPE html><html><body><div id="auth-button-container"></div></body></html>');
        global.window = dom.window;
        global.document = dom.window.document;

        vi.spyOn(AuthManager.prototype, 'init').mockImplementation(() => {});

        global.window.APP_CONFIG = {};
        global.window.sseConnectionManager = {
            isConnectionActiveFor: () => false,
            initializeConnection: () => {}
        };
    });

    afterEach(() => {
        vi.restoreAllMocks();
        delete global.window;
        delete global.document;
    });

    it('renders email element in dropdown header when getEmail() returns a value', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({ email: 'demo@g8e.ai' }));

        const emailEl = document.querySelector('.profile-dropdown .profile-email');
        expect(emailEl).toBeTruthy();
        expect(emailEl.textContent).toBe('demo@g8e.ai');
    });

    it('does not render email element when getEmail() returns null', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({ email: null }));

        const emailEl = document.querySelector('.profile-dropdown .profile-email');
        expect(emailEl).toBeNull();
    });

    it('renders display name using getDisplayName()', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({ name: 'Alice', email: 'alice@example.com' }));

        const nameEl = document.querySelector('.profile-dropdown .profile-name');
        expect(nameEl).toBeTruthy();
        expect(nameEl.textContent).toBe('Alice');
    });
});

describe('AuthManager getUserRole() — uses session model methods', () => {
    beforeEach(() => {
        const dom = new JSDOM('<!DOCTYPE html><html><body><div id="auth-button-container"></div></body></html>');
        global.window = dom.window;
        global.document = dom.window.document;

        vi.spyOn(AuthManager.prototype, 'init').mockImplementation(() => {});

        global.window.APP_CONFIG = {};
        global.window.sseConnectionManager = {
            isConnectionActiveFor: () => false,
            initializeConnection: () => {}
        };
    });

    afterEach(() => {
        vi.restoreAllMocks();
        delete global.window;
        delete global.document;
    });

    it('returns "Admin" for admin role', () => {
        const auth = new AuthManager(null);
        const session = createUserSession({ roles: [UserRole.ADMIN] });
        expect(auth.getUserRole(session)).toBe('Admin');
    });

    it('returns "Admin" for superadmin role', () => {
        const auth = new AuthManager(null);
        const session = createUserSession({ roles: [UserRole.SUPERADMIN] });
        expect(auth.getUserRole(session)).toBe('Admin');
    });

    it('returns "Operator" for operator session role', () => {
        const auth = new AuthManager(null);
        const session = createUserSession({ roles: [OperatorSessionRole.OPERATOR] });
        expect(auth.getUserRole(session)).toBe('Operator');
    });

    it('returns "User" for plain user role', () => {
        const auth = new AuthManager(null);
        const session = createUserSession({ roles: [UserRole.USER] });
        expect(auth.getUserRole(session)).toBe('User');
    });

    it('returns "User" for empty roles', () => {
        const auth = new AuthManager(null);
        const session = createUserSession({ roles: [] });
        expect(auth.getUserRole(session)).toBe('User');
    });

    it('renders correct role label in membership section', () => {
        const auth = new AuthManager(null);

        auth.renderUserProfile(createUserSession({ roles: [UserRole.ADMIN] }));

        const membershipValue = document.querySelector('.profile-dropdown .membership-value');
        expect(membershipValue).toBeTruthy();
        expect(membershipValue.textContent).toBe('Admin');
    });
});
