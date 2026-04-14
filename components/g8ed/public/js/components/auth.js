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

import { EventType } from '../constants/events.js';
import { AuthResponseModel } from '../models/auth-response-model.js';
import { WebSessionModel } from '../models/session-model.js';
import { webSessionService } from '../utils/web-session-service.js';
import { UserRole, OperatorSessionRole } from '../constants/auth-constants.js';
import { AppPaths } from '../constants/app-constants.js';
import { notificationService } from '../utils/notification-service.js';
import { ComponentName } from '../constants/service-client-constants.js';
import { ApiPaths } from '../constants/api-paths.js';

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
 * AuthManager - WebSession-Centric Authentication
 *
 * Architecture:
 * - Single source of truth: `session` (WebSessionModel from g8es KV via backend)
 * - All auth state derived from session object
 * - Passkey (FIDO2/WebAuthn) authentication via /api/auth/passkey/*
 * - HttpOnly cookie sessions managed by server
 *
 * External Interface (used by other components):
 * - getWebSessionId(), getWebSessionModel(), getState()
 * - getApiKey(), isAuthenticated(), hasRole(), isAdmin()
 * - subscribe(callback), logout()
 * - showInfo(), showError()
 */
export class AuthManager {
    constructor(eventBus) {
        this.eventBus = eventBus;
        this.session = null;
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

        const onLoginPage = window.location.pathname === '/';
        if (!this.isAuthenticated() && onLoginPage) {
            this._handleUnauthenticatedInit();
        }
    }

    _handleUnauthenticatedInit() {
        this.showPasskeyLoginModal();
    }

    // =========================================================================
    // WebSession Management
    // =========================================================================

    async _fetchSession() {
        const response = await window.serviceClient.get(ComponentName.G8ED, ApiPaths.auth.webSession());
        const data = await response.json();
        return { response, data };
    }

    _applySessionData(response, data) {
        if (response.ok && data.success && data.session) {
            this.setSession(AuthResponseModel.parse(data).session);
        } else {
            this.clearSession();
        }
    }

    async validateSession() {
        try {
            const { response, data } = await this._fetchSession();
            this._applySessionData(response, data);
        } catch (error) {
            console.warn('[AUTH] WebSession validation failed:', error.message);
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
                webSessionId: sessionModel.id,
                isAuthenticated: true
            };
            this.notifySubscribers(EventType.AUTH_USER_AUTHENTICATED, payload);
            if (this.eventBus) {
                this.eventBus.emit(EventType.AUTH_USER_AUTHENTICATED, payload);
            }

            // Redirect to /chat after successful login if on root or login page
            const currentPath = window.location.pathname;
            if (currentPath === '/' || currentPath === '/login') {
                this._navigate('/chat');
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
    // Passkey Authentication
    // =========================================================================

    async startPasskeyRegistration(userId) {
        try {
            const challengeRes = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.passkey.registerChallenge(), { user_id: userId });
            const challengeData = await challengeRes.json();
            if (!challengeRes.ok || !challengeData.success) {
                return { success: false, message: challengeData.error || 'Failed to get registration challenge' };
            }

            const attestation = await navigator.credentials.create({ publicKey: _decodeRegistrationOptions(challengeData.options) });

            const verifyRes = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerify(), {
                user_id: userId,
                attestation_response: _serializeCredential(attestation)
            });
            const verifyData = await verifyRes.json();

            if (!verifyRes.ok || !verifyData.success) {
                return { success: false, message: verifyData.error || 'Passkey registration failed' };
            }

            if (verifyData.session) {
                this.setSession(AuthResponseModel.parse(verifyData).session);
            }

            return { success: true };
        } catch (error) {
            console.error('[AUTH] Passkey registration error:', error.message);
            return { success: false, message: error.name === 'NotAllowedError' ? 'Registration cancelled.' : 'Passkey registration failed.' };
        }
    }

    async passkeyLogin(email) {
        try {
            const challengeRes = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.passkey.authChallenge(), { email });
            const challengeData = await challengeRes.json();
            if (!challengeRes.ok || !challengeData.success) {
                if (challengeData.needs_setup && challengeData.user_id) {
                    return { success: false, needs_setup: true, userId: challengeData.user_id };
                }
                return { success: false, message: challengeData.error || 'Failed to get authentication challenge' };
            }

            const assertion = await navigator.credentials.get({ publicKey: _decodeAuthenticationOptions(challengeData.options) });

            const verifyRes = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.passkey.authVerify(), {
                email,
                assertion_response: _serializeCredential(assertion),
            });
            const verifyData = await verifyRes.json();

            if (!verifyRes.ok || !verifyData.success) {
                return { success: false, message: verifyData.error || 'Authentication failed' };
            }

            this.setSession(AuthResponseModel.parse(verifyData).session);
            return { success: true };
        } catch (error) {
            console.error('[AUTH] Passkey login error:', error.message);
            return { success: false, message: error.name === 'NotAllowedError' ? 'Authentication cancelled.' : 'Passkey authentication failed.' };
        }
    }

    // =========================================================================
    // SSE-Driven Session Invalidation
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
            await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.logout());
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
        return this.session?.id ?? null;
    }

    getWebSessionModel() {
        return this.session;
    }

    getApiKey() {
        return this.session?.getApiKey() ?? null;
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
        const text = document.createElement('span');
        text.className = 'signin-text';
        text.textContent = 'Sign in';
        button.appendChild(text);
        const icon = document.createElement('span');
        icon.className = 'material-symbols-outlined lock-icon';
        icon.textContent = 'lock';
        button.appendChild(icon);
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

        // Avatar
        const avatar = document.createElement('span');
        avatar.className = 'material-symbols-outlined profile-avatar';
        avatar.textContent = 'person';
        profile.appendChild(avatar);

        // Dropdown
        const dropdown = this.createProfileDropdown(session);
        profile.appendChild(dropdown);

        // Toggle dropdown on click
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

        if (session.getEmail()) {
            const email = document.createElement('div');
            email.className = 'profile-email';
            email.textContent = session.getEmail();
            userInfo.appendChild(email);
        }

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
            const label = document.createElement('span');
            label.className = 'action-label';
            label.textContent = text;
            link.appendChild(label);
            const iconSpan = document.createElement('span');
            iconSpan.className = 'material-symbols-outlined';
            iconSpan.textContent = icon;
            link.appendChild(iconSpan);
        } else {
            link.textContent = text;
        }

        return link;
    }

    // =========================================================================
    // Utility Methods
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
    // Passkey Modals
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

        const form = document.createElement('form');
        form.className = 'auth-modal-form';
        form.addEventListener('submit', (e) => e.preventDefault());

        const emailGroup = document.createElement('div');
        emailGroup.className = 'auth-input-group';
        const emailLabel = document.createElement('label');
        emailLabel.setAttribute('for', 'passkey-login-email');
        emailLabel.textContent = 'Email';
        emailGroup.appendChild(emailLabel);
        const emailInput = document.createElement('input');
        emailInput.type = 'email';
        emailInput.id = 'passkey-login-email';
        emailInput.name = 'passkey-login-email';
        emailInput.placeholder = 'you@example.com';
        emailInput.required = true;
        emailGroup.appendChild(emailInput);
        form.appendChild(emailGroup);

        const errorEl = document.createElement('div');
        errorEl.className = 'auth-modal-error hidden';
        form.appendChild(errorEl);

        const submitBtn = document.createElement('button');
        submitBtn.type = 'submit';
        submitBtn.className = 'auth-modal-submit';
        submitBtn.textContent = 'Sign In with Passkey';
        submitBtn.addEventListener('click', async () => {
            const email = emailInput.value.trim();
            if (!email) {
                errorEl.textContent = 'Email is required.';
                errorEl.classList.remove('hidden');
                return;
            }

            submitBtn.disabled = true;
            submitBtn.textContent = 'Signing in...';
            errorEl.classList.add('hidden');

            const result = await this.passkeyLogin(email);

            if (result.success) {
                const modal = document.getElementById('auth-modal');
                if (modal) modal.remove();
                this._navigate(AppPaths.CHAT);
            } else if (result.needs_setup) {
                this._renderPasskeyResetSetupForm(card, result.userId);
            } else {
                errorEl.textContent = result.message;
                errorEl.classList.remove('hidden');
                submitBtn.disabled = false;
                submitBtn.textContent = 'Sign In with Passkey';
            }
        });
        form.appendChild(submitBtn);

        card.appendChild(form);
    }

    _renderPasskeyResetSetupForm(card, userId) {
        card.innerHTML = '';

        const title = document.createElement('h2');
        title.className = 'auth-modal-title';
        title.textContent = 'Set Up Passkey';
        card.appendChild(title);

        const desc = document.createElement('p');
        desc.className = 'auth-modal-description';
        desc.textContent = 'No passkey found for this account. Set up a new passkey to continue.';
        card.appendChild(desc);

        const errorEl = document.createElement('div');
        errorEl.className = 'auth-modal-error hidden';
        card.appendChild(errorEl);

        const submitBtn = document.createElement('button');
        submitBtn.type = 'button';
        submitBtn.className = 'auth-modal-submit';
        submitBtn.textContent = 'Set Up Passkey';
        submitBtn.addEventListener('click', async () => {
            submitBtn.disabled = true;
            submitBtn.textContent = 'Setting up passkey...';
            errorEl.classList.add('hidden');

            const result = await this.startPasskeyRegistration(userId);

            if (result.success) {
                const modal = document.getElementById('auth-modal');
                if (modal) modal.remove();
                this._navigate(AppPaths.CHAT);
            } else {
                errorEl.textContent = result.message;
                errorEl.classList.remove('hidden');
                submitBtn.disabled = false;
                submitBtn.textContent = 'Set Up Passkey';
            }
        });
        card.appendChild(submitBtn);
    }

    _renderPasskeyRegisterForm(card) {
        card.innerHTML = '';

        const title = document.createElement('h2');
        title.className = 'auth-modal-title';
        title.textContent = 'Create Account';
        card.appendChild(title);

        const form = document.createElement('form');
        form.className = 'auth-modal-form';
        form.addEventListener('submit', (e) => e.preventDefault());

        const nameGroup = document.createElement('div');
        nameGroup.className = 'auth-input-group';
        const nameLabel = document.createElement('label');
        nameLabel.setAttribute('for', 'passkey-register-name');
        nameLabel.textContent = 'Name';
        nameGroup.appendChild(nameLabel);
        const nameInput = document.createElement('input');
        nameInput.type = 'text';
        nameInput.id = 'passkey-register-name';
        nameInput.name = 'passkey-register-name';
        nameInput.placeholder = 'Your name';
        nameInput.required = true;
        nameGroup.appendChild(nameInput);
        form.appendChild(nameGroup);

        const emailGroup = document.createElement('div');
        emailGroup.className = 'auth-input-group';
        const emailLabel = document.createElement('label');
        emailLabel.setAttribute('for', 'passkey-register-email');
        emailLabel.textContent = 'Email';
        emailGroup.appendChild(emailLabel);
        const emailInput = document.createElement('input');
        emailInput.type = 'email';
        emailInput.id = 'passkey-register-email';
        emailInput.name = 'passkey-register-email';
        emailInput.placeholder = 'you@example.com';
        emailInput.required = true;
        emailGroup.appendChild(emailInput);
        form.appendChild(emailGroup);

        const errorEl = document.createElement('div');
        errorEl.className = 'auth-modal-error hidden';
        form.appendChild(errorEl);

        const submitBtn = document.createElement('button');
        submitBtn.type = 'submit';
        submitBtn.className = 'auth-modal-submit';
        submitBtn.textContent = 'Create Account';

        let createdUserId = null;

        submitBtn.addEventListener('click', async () => {
            errorEl.classList.add('hidden');

            if (createdUserId) {
                submitBtn.disabled = true;
                submitBtn.textContent = 'Setting up passkey...';

                const result = await this.startPasskeyRegistration(createdUserId);

                if (result.success) {
                    const modal = document.getElementById('auth-modal');
                    if (modal) modal.remove();
                    this._navigate(AppPaths.CHAT);
                } else {
                    errorEl.textContent = result.message;
                    errorEl.classList.remove('hidden');
                    submitBtn.disabled = false;
                    submitBtn.textContent = 'Set Up Passkey';
                }
                return;
            }

            const name = nameInput.value.trim();
            const email = emailInput.value.trim();

            if (!name) {
                errorEl.textContent = 'Name is required.';
                errorEl.classList.remove('hidden');
                return;
            }
            if (!email) {
                errorEl.textContent = 'Email is required.';
                errorEl.classList.remove('hidden');
                return;
            }

            submitBtn.disabled = true;
            submitBtn.textContent = 'Creating account...';

            const response = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.register(), { name, email });

            if (!response.ok) {
                const body = await response.json().catch(() => ({}));
                errorEl.textContent = body.error || 'Account creation failed.';
                errorEl.classList.remove('hidden');
                submitBtn.disabled = false;
                submitBtn.textContent = 'Create Account';
                return;
            }

            const body = await response.json();
            createdUserId = body.user_id;
            const challengeOptions = body.challenge_options;

            nameInput.disabled = true;
            emailInput.disabled = true;
            submitBtn.disabled = true;
            submitBtn.textContent = 'Setting up passkey...';

            let result;
            if (challengeOptions) {
                // Atomic flow: use the challenge provided in the registration response
                try {
                    const attestation = await navigator.credentials.create({ 
                        publicKey: _decodeRegistrationOptions(challengeOptions) 
                    });
                    const verifyRes = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerify(), {
                        user_id: createdUserId,
                        attestation_response: _serializeCredential(attestation)
                    });
                    const verifyData = await verifyRes.json();
                    result = { 
                        success: verifyRes.ok && verifyData.success, 
                        message: verifyData.error || 'Passkey registration failed' 
                    };
                    if (result.success && verifyData.session) {
                        this.setSession(AuthResponseModel.parse(verifyData).session);
                    }
                } catch (error) {
                    result = { success: false, message: error.name === 'NotAllowedError' ? 'Registration cancelled.' : 'Passkey registration failed.' };
                }
            } else {
                // Fallback (should not happen with atomic backend)
                result = await this.startPasskeyRegistration(createdUserId);
            }

            if (result.success) {
                const modal = document.getElementById('auth-modal');
                if (modal) modal.remove();
                this._navigate(AppPaths.CHAT);
            } else {
                errorEl.textContent = result.message;
                errorEl.classList.remove('hidden');
                submitBtn.disabled = false;
                submitBtn.textContent = 'Set Up Passkey';
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
