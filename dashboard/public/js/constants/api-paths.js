// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8ed API Path Builders — Frontend (Gateway-direct)
 *
 * All browser API calls route through the g8e Gateway at window.G8E_GATEWAY_URL.
 */

const BASE = {
    GATEWAY_API:      '/api/v1',
    GATEWAY_AUTH:     '/api/v1/auth',
    GATEWAY_USERS:    '/api/v1/users',
    GATEWAY_PASSKEYS: '/api/v1/auth/passkeys',
    GATEWAY_SESSIONS: '/api/v1/auth/sessions',
    GATEWAY_SSE:      '/api/v1/sse',
    GATEWAY_AUDIT:    '/api/v1/audit',
    OPERATORS:        '/api/v1/operators',
    CHAT:             '/api/v1/chat',
    SETTINGS:         '/api/v1/settings',
    CASES:            '/api/v1/cases',
    INVESTIGATIONS:   '/api/v1/investigations',
    OPERATOR:         '/api/v1/operator',
    OPERATOR_APPROVAL:'/api/v1/operator/approval',
    WELL_KNOWN_BIN:   '/.well-known/g8e/bin',
};

const Seg = {
    operator: {
        BIND:   'bind',
        UNBIND: 'unbind',
        STOP:   'stop',
    },
    auth: {
        LOGOUT:       'logout',
        BOOTSTRAP:    'bootstrap',
        STATUS:       'status',
        CONSOLE:      'console',
        REGISTER:     'register',
        AUTHENTICATE: 'authenticate',
        CHALLENGE:    'challenge',
        VERIFY:       'verify',
    },
    user: {
        ME:       'me',
        DEV_LOGS: 'dev-logs',
    },
    chat: {
        STOP: 'stop',
    },
    approval: {
        RESPOND: 'respond',
    },
    sse: {
        STREAM: 'stream',
        EVENTS: 'events',
    },
    audit: {
        EVENTS: 'events',
        VERIFY: 'verify',
    },
};

export const ApiPaths = {
    operator: {
        bind:   () => `${BASE.OPERATORS}/${Seg.operator.BIND}`,
        unbind: () => `${BASE.OPERATORS}/${Seg.operator.UNBIND}`,
        list:   () => `${BASE.OPERATORS}`,
        get:    (operatorId) => `${BASE.OPERATORS}/${operatorId}`,
        stop:   (operatorId) => `${BASE.OPERATORS}/${operatorId}/${Seg.operator.STOP}`,
    },
    auth: {
        bootstrapStatus: () => `${BASE.GATEWAY_AUTH}/${Seg.auth.BOOTSTRAP}/${Seg.auth.STATUS}`,
        logout:          () => `${BASE.GATEWAY_AUTH}/${Seg.auth.LOGOUT}`,
        sessionsMe:      () => `${BASE.GATEWAY_SESSIONS}/${Seg.user.ME}`,
        passkeys: {
            list:                  () => `${BASE.GATEWAY_PASSKEYS}`,
            registerChallenge:     () => `${BASE.GATEWAY_PASSKEYS}/${Seg.auth.CONSOLE}/${Seg.auth.REGISTER}/${Seg.auth.CHALLENGE}`,
            registerVerify:        () => `${BASE.GATEWAY_PASSKEYS}/${Seg.auth.CONSOLE}/${Seg.auth.REGISTER}/${Seg.auth.VERIFY}`,
            authenticateChallenge: () => `${BASE.GATEWAY_PASSKEYS}/${Seg.auth.CONSOLE}/${Seg.auth.AUTHENTICATE}/${Seg.auth.CHALLENGE}`,
            authenticateVerify:    () => `${BASE.GATEWAY_PASSKEYS}/${Seg.auth.CONSOLE}/${Seg.auth.AUTHENTICATE}/${Seg.auth.VERIFY}`,
            revoke:                (credentialId) => `${BASE.GATEWAY_PASSKEYS}/${credentialId}`,
        },
    },
    user: {
        me:      () => `${BASE.GATEWAY_USERS}/${Seg.user.ME}`,
        devLogs: () => `${BASE.GATEWAY_USERS}/${Seg.user.ME}/${Seg.user.DEV_LOGS}`,
    },
    settings: {
        list: () => BASE.SETTINGS,
        save: () => BASE.SETTINGS,
        user: () => `${BASE.SETTINGS}/user`,
        sync: () => `${BASE.SETTINGS}/sync`,
    },
    chat: {
        send:           () => BASE.CHAT,
        investigations: () => BASE.INVESTIGATIONS,
        investigation:  (investigationId) => `${BASE.INVESTIGATIONS}/${investigationId}`,
        stop:           () => `${BASE.CHAT}/${Seg.chat.STOP}`,
        cases:          (caseId) => `${BASE.CASES}/${caseId}`,
    },
    approval: {
        respond:       () => `${BASE.OPERATOR_APPROVAL}/${Seg.approval.RESPOND}`,
        directCommand: () => `${BASE.OPERATOR}/direct-command`,
    },
    sse: {
        stream: () => `${BASE.GATEWAY_SSE}/${Seg.sse.STREAM}`,
        events: () => `${BASE.GATEWAY_SSE}/${Seg.sse.EVENTS}`,
    },
    audit: {
        events: () => `${BASE.GATEWAY_AUDIT}/${Seg.audit.EVENTS}`,
        verify: () => `${BASE.GATEWAY_AUDIT}/${Seg.audit.VERIFY}`,
    },
    gateway: {
        approvals: () => `${BASE.GATEWAY_API}/approvals`,
        operatorBinary: (os, arch) => `${BASE.WELL_KNOWN_BIN}/${os}/${arch}`,
        operatorBinaryChecksum: (os, arch) => `${BASE.WELL_KNOWN_BIN}/${os}/${arch}/sha256`,
    },
};
