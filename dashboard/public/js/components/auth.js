// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { EventType } from '../constants/events.js';
import { AuthResponseModel } from '../models/auth-response-model.js';
import { WebSessionModel } from '../models/session-model.js';
import { webSessionService } from '../utils/web-session-service.js';
import { UserRole, OperatorSessionRole } from '../constants/auth-constants.js';
import { AppPaths } from '../constants/app-constants.js';
import { notificationService } from '../utils/notification-service.js';
import { ServiceName } from '../constants/service-client-constants.js';
import { ApiPaths } from '../constants/api-paths.js';

const CLI_SESSION_ID = 'browser';

function _base64urlToBuffer(base64url) {
    const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
    const binary = atob(base64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes.buffer;
}

function _decodeRegistrationOptions(options) {
    return {
        ...options,
        challenge: _base64urlToBuffer(options.challenge),
        user: {
            ...options.user,
            id: _base64urlToBuffer(options.user.id),
        },
        excludeCredentials: (options.excludeCredentials || []).map(c => ({
            ...c,
            id: _base64urlToBuffer(c.id),
        })),
    };
}

function _decodeAuthenticationOptions(options) {
    return {
        ...options,
        challenge: _base64urlToBuffer(options.challenge),
        allowCredentials: (options.allowCredentials || []).map(c => ({
            ...c,
            id: _base64urlToBuffer(c.id),
        })),
    };
}

function _bufferToBase64url(buffer) {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (const byte of bytes) binary += String.fromCharCode(byte);
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

function _serializeCredential(credential) {
    const r = credential.response;
    const response = {};

    if (r.clientDataJSON    != null) response.clientDataJSON    = _bufferToBase64url(r.clientDataJSON);
    if (r.attestationObject != null) response.attestationObject = _bufferToBase64url(r.attestationObject);
    if (r.authenticatorData != null) response.authenticatorData = _bufferToBase64url(r.authenticatorData);
    if (r.signature         != null) response.signature         = _bufferToBase64url(r.signature);
    if (r.userHandle        != null) response.userHandle        = _bufferToBase64url(r.userHandle);
    if (r.publicKey         != null) response.publicKey         = _bufferToBase64url(r.publicKey);
    if (r.publicKeyAlgorithm != null) response.publicKeyAlgorithm = r.publicKeyAlgorithm;

    const transports = typeof r.getTransports === 'function' ? r.getTransports() : (r.transports ?? []);
    response.transports = transports;

    return {
        id:                     credential.id,
        rawId:                  _bufferToBase64url(credential.rawId),
        type:                   credential.type,
        clientExtensionResults: credential.getClientExtensionResults?.() ?? {},
        response,
    };
}

/**
 * AuthManager — gateway-direct WebAuthn authentication.
 *
 * The browser authenticates directly against the g8e Gateway at
 * window.G8E_GATEWAY_URL (ServiceName.GATEWAY) over HTTPS with
 * credentials: 'include'. The gateway issues an HttpOnly
 * g8e_web_session_cookie after a successful passkey ceremony; the cookie is
 * not JS-readable and is sent automatically on every subsequent request.
 *
 * External interface (used by app.js, chat-auth.js, and other components):
 * - getWebSessionId(), getWebSessionModel(), getState()
 * - isAuthenticated(), hasRole(), isAdmin()
 * - subscribe(callback), logout()
 * - showInfo(), showError()
 */
export class AuthManager {
    constructor(eventBus) {
        this.eventBus = eventBus;
        this.session = null;
        this.bootstrapped = null;
        this.subscribers = new Set();
        this.initialized = false;

        Object.defineProperty(this, 'loading', {
            get: () => !this.initialized,
            enumerable: true
        });
    }

    async init() {
        try {
            await this.validateSession();
        } catch (error) {
            console.error('[AUTH] Initialization error:', error.message);
        }

        const onLoginPage = window.location.pathname === '/' || window.location.pathname === '/login';
        if (!this.isAuthenticated() && onLoginPage) {
            this._handleUnauthenticatedInit();
        }
    }

    async _handleUnauthenticatedInit() {
        const bootstrapped = await this._fetchBootstrapStatus();
        if (bootstrapped === false) {
            this.showPasskeyRegistrationModal();
        } else {
            this.showPasskeyLoginModal();
        }
    }

    // =========================================================================
    // Session validation (gateway-direct)
    // =========================================================================

    async _fetchBootstrapStatus() {
        try {
            const response = await window.serviceClient.get(ServiceName.GATEWAY, ApiPaths.auth.bootstrapStatus());
            if (!response.ok) return null;
            const data = await response.json();
            this.bootstrapped = data?.bootstrapped === true;
            return this.bootstrapped;
        } catch (error) {
            console.warn('[AUTH] Bootstrap status check failed:', error.message);
            return null;
        }
    }

    async _fetchUser() {
        const response = await window.serviceClient.get(ServiceName.GATEWAY, ApiPaths.user.me());
        const data = await response.json();
        return { response, data };
    }

    async _fetchSessionId() {
        try {
            const response = await window.serviceClient.get(ServiceName.GATEWAY, ApiPaths.auth.sessionsMe());
            if (!response.ok) return null;
            const data = await response.json();
            return data?.web_session_id ?? data?.session_id ?? null;
        } catch (error) {
            console.warn('[AUTH] Session id fetch failed:', error.message);
            return null;
        }
    }

    async validateSession() {
        try {
            const { response, data } = await this._fetchUser();
            if (response.ok && data?.success && data?.user) {
                const userModel = AuthResponseModel.parse(data).session;
                if (userModel) {
                    const webSessionId = await this._fetchSessionId();
                    if (webSessionId) {
                        userModel.web_session_id = webSessionId;
                    }
                    this.setSession(userModel);
                    return;
                }
            }
            this.clearSession();
        } catch (error) {
            console.warn('[AUTH] Session validation failed:', error.message);
            this.clearSession();
        } finally {
            this.completeInitialization();
        }
    }

    completeInitialization() {
        this.initialized = true;
        const state = this.getState();
        this.notifySubscribers(EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE, state);
        if (this.eventBus) {
            this.eventBus.emit(EventType.AUTH_COMPONENT_INITIALIZED_AUTHSTATE, state);
        }
    }

    setSession(sessionModel) {
        const wasAuthenticated = this.isAuthenticated();
        this.session = sessionModel;
        webSessionService.setSession(sessionModel);

        if (sessionModel) {
            this._onSessionEstablished(sessionModel, wasAuthenticated);
        }
    }

    _onSessionEstablished(sessionModel, wasAuthenticated) {
        document.body.classList.add('user-authenticated');
        this.renderUserProfile(sessionModel);
        this._subscribeToSSEFailed();

        if (!wasAuthenticated) {
            const payload = {
                webSessionModel: sessionModel,
                webSessionId: sessionModel.getWebSessionId?.() ?? sessionModel.web_session_id,
                isAuthenticated: true
            };
            this.notifySubscribers(EventType.AUTH_USER_AUTHENTICATED, payload);
            if (this.eventBus) {
                this.eventBus.emit(EventType.AUTH_USER_AUTHENTICATED, payload);
            }

            const currentPath = window.location.pathname;
            if (currentPath === '/' || currentPath === '/login') {
                this._navigate(AppPaths.CHAT);
            }
        }
    }

    _navigate(url) {
        window.location.href = url;
    }

    clearSession() {
        const wasAuthenticated = this.isAuthenticated();
        this.session = null;
        webSessionService.clearSession();
        this._onSessionCleared(wasAuthenticated);
    }

    _onSessionCleared(wasAuthenticated) {
        document.body.classList.remove('user-authenticated');
        this.renderSignInButton();

        const banner = document.getElementById('unauthenticated-banner');
        if (banner) banner.classList.remove('hidden');

        if (wasAuthenticated) {
            const payload = {
                isAuthenticated: false,
                user: null,
                webSessionId: null,
                webSessionModel: null
            };
            this.notifySubscribers(EventType.AUTH_USER_UNAUTHENTICATED, payload);
            if (this.eventBus) {
                this.eventBus.emit(EventType.AUTH_USER_UNAUTHENTICATED, payload);
            }
        }
    }

    // =========================================================================
    // Passkey registration (first-time setup / bootstrap)
    // =========================================================================

    async startPasskeyRegistration(userName) {
        try {
            const challengeRes = await window.serviceClient.post(
                ServiceName.GATEWAY,
                ApiPaths.auth.passkeys.registerChallenge(),
                { user_name: userName, cli_session_id: CLI_SESSION_ID },
            );
            const challengeData = await challengeRes.json();
            if (!challengeRes.ok || !challengeData.success) {
                return { success: false, message: challengeData.error || 'Failed to get registration challenge' };
            }

            // user_id for the verify step comes from the challenge response
            // (options.user.id). The gateway creates the user record during the
            // challenge phase when user_id is omitted to bootstrap.
            const userId = challengeData.options?.user?.id;
            if (!userId) {
                return { success: false, message: 'Registration challenge missing user id' };
            }

            const attestation = await navigator.credentials.create({
                publicKey: _decodeRegistrationOptions(challengeData.options),
            });

            const verifyRes = await window.serviceClient.post(
                ServiceName.GATEWAY,
                ApiPaths.auth.passkeys.registerVerify(),
                {
                    user_id: userId,
                    cli_session_id: CLI_SESSION_ID,
                    attestation_response: _serializeCredential(attestation),
                },
            );
            const verifyData = await verifyRes.json();

            if (!verifyRes.ok || !verifyData.success) {
                return { success: false, message: verifyData.error || 'Passkey registration failed' };
            }

            const session = AuthResponseModel.parse(verifyData).session;
            if (session) {
                const webSessionId = await this._fetchSessionId();
                if (webSessionId) session.web_session_id = webSessionId;
                this.setSession(session);
            }

            return { success: true };
        } catch (error) {
            console.error('[AUTH] Passkey registration error:', error.message);
            return { success: false, message: error.name === 'NotAllowedError' ? 'Registration cancelled.' : 'Passkey registration failed.' };
        }
    }

    // =========================================================================
    // Passkey authentication (returning user)
    // =========================================================================

    async passkeyLogin() {
        try {
            // Discoverable-credential flow: omit user_id so the gateway returns
            // a challenge without allowCredentials, letting the browser pick
            // any resident passkey. The user_id is recovered from the
            // assertion's userHandle during verification.
            const challengeRes = await window.serviceClient.post(
                ServiceName.GATEWAY,
                ApiPaths.auth.passkeys.authenticateChallenge(),
                {},
            );
            const challengeData = await challengeRes.json();
            if (!challengeRes.ok || !challengeData.success) {
                if (challengeData.needs_setup) {
                    return { success: false, needs_setup: true };
                }
                return { success: false, message: challengeData.error || 'Failed to get authentication challenge' };
            }

            const assertion = await navigator.credentials.get({
                publicKey: _decodeAuthenticationOptions(challengeData.options),
            });

            // Recover user_id from the assertion's userHandle if the gateway
            // did not provide it in the challenge response.
            const serialized = _serializeCredential(assertion);
            const userId = challengeData.user_id || challengeData.options?.user_id || serialized.response.userHandle;

            const verifyRes = await window.serviceClient.post(
                ServiceName.GATEWAY,
                ApiPaths.auth.passkeys.authenticateVerify(),
                {
                    user_id: userId,
                    assertion_response: serialized,
                },
            );
            const verifyData = await verifyRes.json();

            if (!verifyRes.ok || !verifyData.success) {
                return { success: false, message: verifyData.error || 'Authentication failed' };
            }

            const session = AuthResponseModel.parse(verifyData).session;
            if (session) {
                const webSessionId = await this._fetchSessionId();
                if (webSessionId) session.web_session_id = webSessionId;
                this.setSession(session);
            }
            return { success: true };
        } catch (error) {
            console.error('[AUTH] Passkey login error:', error.message);
            return { success: false, message: error.name === 'NotAllowedError' ? 'Authentication cancelled.' : 'Passkey authentication failed.' };
        }
    }

    // =========================================================================
    // SSE-driven session invalidation
    // =========================================================================

    _subscribeToSSEFailed() {
        if (this._sseFailedUnsubscribe || !this.eventBus) return;
        this._sseFailedUnsubscribe = this.eventBus.on(EventType.PLATFORM_SSE_CONNECTION_FAILED, () => {
            if (this.isAuthenticated()) {
                this.handleSessionExpired();
            }
        });
    }

    handleSessionExpired() {
        console.log('[AUTH] WebSession expired');
        this.clearSession();
        this.notifySubscribers(EventType.AUTH_SESSION_EXPIRED, {
            message: 'Your session has expired. Please sign in again.'
        });
    }

    // =========================================================================
    // Logout
    // =========================================================================

    async logout() {
        try {
            await window.serviceClient.post(ServiceName.GATEWAY, ApiPaths.auth.logout());
        } catch (error) {
            console.error('[AUTH] Logout error:', error.message);
        }

        if (window.sseConnectionManager) {
            window.sseConnectionManager.disconnect();
        }

        this.clearSession();
        this._navigate(AppPaths.HOME);
    }

    // =========================================================================
    // Public API
    // =========================================================================

    isAuthenticated() {
        return this.session?.isValid() ?? false;
    }

    getWebSessionId() {
        return this.session?.web_session_id ?? null;
    }

    getWebSessionModel() {
        return this.session;
    }

    hasRole(role) {
        return this.session?.hasRole(role) ?? false;
    }

    isAdmin() {
        return this.session?.isAdmin() ?? false;
    }

    getState() {
        return {
            isAuthenticated: this.isAuthenticated(),
            webSessionModel: this.session,
            webSessionId: this.getWebSessionId(),
            loading: !this.initialized
        };
    }

    subscribe(callback) {
        this.subscribers.add(callback);
        return () => this.subscribers.delete(callback);
    }

    notifySubscribers(event, data) {
        this.subscribers.forEach(callback => {
            try {
                callback(event, data);
            } catch (error) {
                console.warn('[AUTH] Subscriber error:', error.message);
            }
        });
    }

    // =========================================================================
    // Notifications
    // =========================================================================

    showInfo(message) {
        console.log('[AUTH] Info:', message);
        notificationService.info(message);
        if (this.eventBus) {
            this.eventBus.emit(EventType.AUTH_INFO, { message });
        }
    }

    showError(message) {
        console.error('[AUTH] Error:', message);
        notificationService.error(message);
    }

    // =========================================================================
    // UI Rendering
    // =========================================================================

    renderSignInButton() {
        const container = document.getElementById('auth-button-container');
        if (!container) return;

        container.innerHTML = '';

        const wrapper = document.createElement('div');
        wrapper.className = 'auth-controls-wrapper';

        const button = document.createElement('button');
        button.className = 'local-signin-btn';
        button.setAttribute('aria-label', 'Sign in');
        button.innerHTML = `
            <span class="signin-text">Sign in</span>
            <span class="material-symbols-outlined lock-icon">lock</span>
        `;
        button.addEventListener('click', () => this.showPasskeyLoginModal());
        wrapper.appendChild(button);

        container.appendChild(wrapper);
    }

    renderUserProfile(session) {
        const banner = document.getElementById('unauthenticated-banner');
        if (banner) banner.classList.add('hidden');

        const container = document.getElementById('auth-button-container');
        if (!container) return;

        container.innerHTML = '';

        const wrapper = document.createElement('div');
        wrapper.className = 'auth-controls-wrapper';

        const profile = document.createElement('div');
        profile.id = 'user-profile-display';
        profile.className = 'user-profile-display';

        const avatar = document.createElement('img');
        avatar.src = session.getAvatar();
        avatar.alt = 'Profile';
        avatar.className = 'profile-avatar';
        avatar.onerror = function() {
            this.src = '/media/default-avatar.png';
            this.onerror = null;
        };
        profile.appendChild(avatar);

        const dropdown = this.createProfileDropdown(session);
        profile.appendChild(dropdown);

        profile.addEventListener('click', (e) => {
            e.stopPropagation();
            dropdown.classList.toggle('show');
        });

        document.addEventListener('click', () => dropdown.classList.remove('show'));

        wrapper.appendChild(profile);
        container.appendChild(wrapper);
    }

    createProfileDropdown(session) {
        const dropdown = document.createElement('div');
        dropdown.className = 'profile-dropdown';

        const header = document.createElement('div');
        header.className = 'profile-dropdown-header';

        const userInfo = document.createElement('div');
        userInfo.className = 'profile-info';

        const name = document.createElement('div');
        name.className = 'profile-name';
        name.textContent = session.getDisplayName();
        userInfo.appendChild(name);

        header.appendChild(userInfo);
        dropdown.appendChild(header);
        dropdown.appendChild(this.createMembershipSection(session));
        dropdown.appendChild(this.createDropdownActions(session));

        return dropdown;
    }

    createMembershipSection(session) {
        const section = document.createElement('div');
        section.className = 'profile-membership-info';

        const label = document.createElement('div');
        label.className = 'membership-label';
        label.textContent = 'Role';
        section.appendChild(label);

        const role = this.getUserRole(session);

        const value = document.createElement('div');
        value.className = 'membership-value';
        value.textContent = role;

        section.appendChild(value);
        return section;
    }

    createDropdownActions(session) {
        const actions = document.createElement('div');
        actions.className = 'profile-dropdown-actions';

        actions.appendChild(this.createActionLink('/settings', 'Settings', null, 'settings-link'));

        if (session.hasRole(UserRole.SUPERADMIN)) {
            actions.appendChild(this.createActionLink('/console', 'Console', null, 'console-link'));
        }

        actions.appendChild(this.createActionLink('/audit', 'Audit Log', null, 'audit-log-link'));

        const logoutBtn = document.createElement('button');
        logoutBtn.type = 'button';
        logoutBtn.className = 'profile-dropdown-action-link profile-dropdown-action-button logout-link';
        logoutBtn.textContent = 'Logout';
        logoutBtn.title = 'Sign Out';
        logoutBtn.addEventListener('click', () => this.logout());
        actions.appendChild(logoutBtn);

        return actions;
    }

    createActionLink(href, text, icon, className) {
        const link = document.createElement('a');
        link.href = href;
        link.className = `profile-dropdown-action-link ${className}`;
        link.title = text;
        link.addEventListener('click', (e) => e.stopPropagation());

        if (icon) {
            link.innerHTML = `<span class="action-label">${text}</span><span class="material-symbols-outlined">${icon}</span>`;
        } else {
            link.textContent = text;
        }

        return link;
    }

    // =========================================================================
    // Utility methods
    // =========================================================================

    getUserRole(session) {
        if (session.isAdmin()) return 'Admin';
        if (session.hasRole(OperatorSessionRole.OPERATOR)) return 'Operator';
        return 'User';
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // =========================================================================
    // Passkey modals
    // =========================================================================

    showPasskeyLoginModal() {
        let modal = document.getElementById('auth-modal');
        if (modal) modal.remove();

        modal = document.createElement('div');
        modal.id = 'auth-modal';
        modal.className = 'auth-modal-overlay';
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.remove();
        });

        const card = document.createElement('div');
        card.className = 'auth-modal-card';
        modal.appendChild(card);

        this._renderPasskeyLoginForm(card);
        document.body.appendChild(modal);
    }

    _renderPasskeyLoginForm(card) {
        card.innerHTML = '';

        const title = document.createElement('h2');
        title.className = 'auth-modal-title';
        title.textContent = 'Sign In';
        card.appendChild(title);

        const desc = document.createElement('p');
        desc.className = 'auth-modal-description';
        desc.textContent = 'Use a passkey to sign in to g8e.';
        card.appendChild(desc);

        const errorEl = document.createElement('div');
        errorEl.className = 'auth-modal-error hidden';
        card.appendChild(errorEl);

        const submitBtn = document.createElement('button');
        submitBtn.type = 'button';
        submitBtn.className = 'auth-modal-submit';
        submitBtn.textContent = 'Sign in with passkey';
        submitBtn.addEventListener('click', async () => {
            submitBtn.disabled = true;
            submitBtn.textContent = 'Signing in...';
            errorEl.classList.add('hidden');

            const result = await this.passkeyLogin();

            if (result.success) {
                const modal = document.getElementById('auth-modal');
                if (modal) modal.remove();
                this._navigate(AppPaths.CHAT);
            } else if (result.needs_setup) {
                this._renderPasskeyRegisterForm(card);
            } else {
                errorEl.textContent = result.message;
                errorEl.classList.remove('hidden');
                submitBtn.disabled = false;
                submitBtn.textContent = 'Sign in with passkey';
            }
        });
        card.appendChild(submitBtn);
    }

    showPasskeyRegistrationModal() {
        let modal = document.getElementById('auth-modal');
        if (modal) modal.remove();

        modal = document.createElement('div');
        modal.id = 'auth-modal';
        modal.className = 'auth-modal-overlay';
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.remove();
        });

        const card = document.createElement('div');
        card.className = 'auth-modal-card';
        modal.appendChild(card);

        this._renderPasskeyRegisterForm(card);
        document.body.appendChild(modal);
    }

    _renderPasskeyRegisterForm(card) {
        card.innerHTML = '';

        const title = document.createElement('h2');
        title.className = 'auth-modal-title';
        title.textContent = 'Set Up Passkey';
        card.appendChild(title);

        const desc = document.createElement('p');
        desc.className = 'auth-modal-description';
        desc.textContent = 'No passkey is registered yet. Create one to sign in to g8e.';
        card.appendChild(desc);

        const form = document.createElement('form');
        form.className = 'auth-modal-form';
        form.addEventListener('submit', (e) => e.preventDefault());

        const nameGroup = document.createElement('div');
        nameGroup.className = 'auth-input-group';
        const nameLabel = document.createElement('label');
        nameLabel.setAttribute('for', 'passkey-register-name');
        nameLabel.textContent = 'Display name (optional)';
        nameGroup.appendChild(nameLabel);
        const nameInput = document.createElement('input');
        nameInput.type = 'text';
        nameInput.id = 'passkey-register-name';
        nameInput.name = 'passkey-register-name';
        nameInput.placeholder = 'Your name';
        nameGroup.appendChild(nameInput);
        form.appendChild(nameGroup);

        const errorEl = document.createElement('div');
        errorEl.className = 'auth-modal-error hidden';
        form.appendChild(errorEl);

        const submitBtn = document.createElement('button');
        submitBtn.type = 'submit';
        submitBtn.className = 'auth-modal-submit';
        submitBtn.textContent = 'Create passkey';
        submitBtn.addEventListener('click', async (e) => {
            e.preventDefault();
            const userName = nameInput.value.trim() || 'browser';

            submitBtn.disabled = true;
            submitBtn.textContent = 'Creating passkey...';
            errorEl.classList.add('hidden');

            const result = await this.startPasskeyRegistration(userName);

            if (result.success) {
                const modal = document.getElementById('auth-modal');
                if (modal) modal.remove();
                this._navigate(AppPaths.CHAT);
            } else {
                errorEl.textContent = result.message;
                errorEl.classList.remove('hidden');
                submitBtn.disabled = false;
                submitBtn.textContent = 'Create passkey';
            }
        });
        form.appendChild(submitBtn);

        card.appendChild(form);
    }

    // =========================================================================
    // Theme
    // =========================================================================

    createThemeToggleButton() {
        const button = document.createElement('button');
        button.className = 'profile-theme-toggle-standalone';
        button.setAttribute('aria-label', 'Toggle theme');
        button.innerHTML = '<span class="material-symbols-outlined"></span>';

        button.addEventListener('click', (e) => {
            e.stopPropagation();
            const newTheme = window.ThemeManager.toggle();
            this.updateThemeToggleIcon(button, newTheme);
        });

        const currentTheme = window.ThemeManager ? window.ThemeManager.getTheme() : 'dark';
        this.updateThemeToggleIcon(button, currentTheme);

        if (window.ThemeManager) {
            window.ThemeManager.onChange((theme) => this.updateThemeToggleIcon(button, theme));
        }

        return button;
    }

    updateThemeToggleIcon(button, theme) {
        const icon = button.querySelector('.material-symbols-outlined');
        if (icon) {
            icon.textContent = theme === 'dark' ? 'dark_mode' : 'light_mode';
        }
    }
}
