// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8ed API Path Builders — Frontend
 *
 * Single source of truth for all g8ed endpoint paths consumed by the frontend.
 * Mirrors components/g8ed/constants/api_paths.js (server-side).
 *
 * All path strings MUST be defined here — no inline string literals elsewhere.
 *
 * Usage:
 *   import { ApiPaths } from '../constants/api-paths.js';
 *   serviceClient.get(ServiceName.g8ed, ApiPaths.operator.details(operatorId));
 */

const BASE = {
    // Gateway-direct (g8e) API surface — browser-authenticated via the HttpOnly
    // g8e_web_session_cookie. All auth/session/user/passkey paths route to the
    // gateway at window.G8E_GATEWAY_URL (ServiceName.GATEWAY).
    GATEWAY_API:      '/api/v1',
    GATEWAY_AUTH:     '/api/v1/auth',
    GATEWAY_USERS:    '/api/v1/users',
    GATEWAY_PASSKEYS: '/api/v1/auth/passkeys',
    GATEWAY_SESSIONS: '/api/v1/auth/sessions',
    GATEWAY_SSE:      '/api/v1/sse',
    // Legacy g8ed surface — retained for out-of-scope features (operator, chat,
    // settings, console, audit) that have not yet migrated to the gateway.
    OPERATORS:    '/api/operators',
    APPROVAL:     '/api/operator/approval',
    CHAT:         '/api/chat',
    DEVICE_LINKS: '/api/device-links',
    AUDIT:        '/api/audit',
    SETTINGS:     '/api/settings',
    SETUP:        '/api/setup',
    HEALTH:       '/health',
    METRICS:      '/api/metrics',
    CONSOLE:      '/api/console',
    SYSTEM:       '/api/system',
    DOCS:         '/api/docs',
};

const Seg = {
    operator: {
        BIND:            'bind',
        UNBIND:          'unbind',
        BIND_ALL:        'bind-all',
        UNBIND_ALL:      'unbind-all',
        DETAILS:         'details',
        STOP:            'stop',
        API_KEY:         'api-key',
        REFRESH_API_KEY: 'refresh-api-key',
        DROP_POD:        'drop-pod',
        REAUTH:          'reauth',
    },
    auth: {
        LOGOUT:     'logout',
        BOOTSTRAP:  'bootstrap',
        STATUS:     'status',
        CONSOLE:    'console',
        REGISTER:   'register',
        AUTHENTICATE: 'authenticate',
        CHALLENGE:  'challenge',
        VERIFY:     'verify',
        // Legacy g8ed auth segments retained for out-of-scope device-link flows.
        LINK:      'link',
        GENERATE:  'generate',
        AUTHORIZE: 'authorize',
        REJECT:    'reject',
    },
    deviceLink: {
        DELETE:   'delete',
    },
    user: {
        ME:       'me',
        DEV_LOGS: 'dev-logs',
    },
    chat: {
        SEND:           'send',
        INVESTIGATIONS: 'investigations',
        STOP:           'stop',
        CASES:          'cases',
        HEALTH:         'health',
    },
    approval: {
        RESPOND:        'respond',
        DIRECT_COMMAND: 'direct-command',
    },
    sse: {
        STREAM: 'stream',
        EVENTS: 'events',
        HEALTH: 'health',
    },
    audit: {
        EVENTS:   'events',
        DOWNLOAD: 'download',
    },
    setup: {
        CONFIG: 'config',
        USER:   'user',
    },
    health: {
        LIVE:        'live',
        STORE:       'store',
        DETAILS:     'details',
        CACHE_STATS: 'cache-stats',
    },
    console: {
        OVERVIEW:     'overview',
        METRICS:      'metrics',
        USERS:        'users',
        OPERATORS:    'operators',
        SESSIONS:     'sessions',
        AI:           'ai',
        LOGIN_AUDIT:  'login-audit',
        REALTIME:     'realtime',
        CACHE:        'cache',
        CLEAR:        'clear',
        COMPONENTS:   'components',
        HEALTH:       'health',
        KV:           'kv',
        SCAN:         'scan',
        KEY:          'key',
        DB:           'db',
        QUERY:        'query',
        COLLECTIONS:  'collections',
        LOGS:         'logs',
        STREAM:       'stream',
    },
    metrics: {
        HEALTH: 'health',
    },
    system: {
        NETWORK_INTERFACES: 'network-interfaces',
    },
    docs: {
        TREE: 'tree',
        FILE: 'file',
    },
};

export const ApiPaths = {
    operator: {
        bind:          () => `${BASE.OPERATORS}/${Seg.operator.BIND}`,
        unbind:        () => `${BASE.OPERATORS}/${Seg.operator.UNBIND}`,
        bindAll:       () => `${BASE.OPERATORS}/${Seg.operator.BIND_ALL}`,
        unbindAll:     () => `${BASE.OPERATORS}/${Seg.operator.UNBIND_ALL}`,
        list:          () => `${BASE.OPERATORS}`,
        details:       (operatorId) => `${BASE.OPERATORS}/${operatorId}/${Seg.operator.DETAILS}`,
        stop:          (operatorId) => `${BASE.OPERATORS}/${operatorId}/${Seg.operator.STOP}`,
        apiKey:        (operatorId) => `${BASE.OPERATORS}/${operatorId}/${Seg.operator.API_KEY}`,
        refreshApiKey: (operatorId) => `${BASE.OPERATORS}/${operatorId}/${Seg.operator.REFRESH_API_KEY}`,
        dropPodReauth: () => `${BASE.OPERATORS}/${Seg.operator.DROP_POD}/${Seg.operator.REAUTH}`,
    },
    auth: {
        // Gateway-direct auth paths (ServiceName.GATEWAY).
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
        // Legacy g8ed device-link auth paths (ServiceName.g8ed) — out of scope.
        linkGenerate:  () => `${BASE.GATEWAY_AUTH}/${Seg.auth.LINK}/${Seg.auth.GENERATE}`,
        linkAuthorize: (token) => `${BASE.GATEWAY_AUTH}/${Seg.auth.LINK}/${token}/${Seg.auth.AUTHORIZE}`,
        linkReject:    (token) => `${BASE.GATEWAY_AUTH}/${Seg.auth.LINK}/${token}/${Seg.auth.REJECT}`,
    },
    deviceLink: {
        list:   () => BASE.DEVICE_LINKS,
        create: () => BASE.DEVICE_LINKS,
        revoke: (tokenId) => `${BASE.DEVICE_LINKS}/${tokenId}`,
        delete: (tokenId) => `${BASE.DEVICE_LINKS}/${tokenId}?action=${Seg.deviceLink.DELETE}`,
    },
    user: {
        me:      () => `${BASE.GATEWAY_USERS}/${Seg.user.ME}`,
        devLogs: () => `${BASE.GATEWAY_USERS}/${Seg.user.ME}/${Seg.user.DEV_LOGS}`,
    },
    settings: {
        list: () => BASE.SETTINGS,
        save: () => BASE.SETTINGS,
    },
    setup: {
        config: () => `${BASE.SETUP}/${Seg.setup.CONFIG}`,
        user:   () => `${BASE.SETUP}/${Seg.setup.USER}`,
    },
    chat: {
        send:           () => `${BASE.CHAT}/${Seg.chat.SEND}`,
        investigations: () => `${BASE.CHAT}/${Seg.chat.INVESTIGATIONS}`,
        investigation:  (investigationId) => `${BASE.CHAT}/${Seg.chat.INVESTIGATIONS}/${investigationId}`,
        stop:           () => `${BASE.CHAT}/${Seg.chat.STOP}`,
        cases:          (caseId) => `${BASE.CHAT}/${Seg.chat.CASES}/${caseId}`,
        health:         () => `${BASE.CHAT}/${Seg.chat.HEALTH}`,
    },
    approval: {
        respond:       () => `${BASE.APPROVAL}/${Seg.approval.RESPOND}`,
        directCommand: () => `${BASE.APPROVAL}/${Seg.approval.DIRECT_COMMAND}`,
    },
    sse: {
        stream: () => `${BASE.GATEWAY_SSE}/${Seg.sse.STREAM}`,
        events: () => `${BASE.GATEWAY_SSE}/${Seg.sse.EVENTS}`,
    },
    audit: {
        events:   () => `${BASE.AUDIT}/${Seg.audit.EVENTS}`,
        download: () => `${BASE.AUDIT}/${Seg.audit.DOWNLOAD}`,
    },
    health: {
        root:       () => `${BASE.HEALTH}`,
        live:       () => `${BASE.HEALTH}/${Seg.health.LIVE}`,
        store:      () => `${BASE.HEALTH}/${Seg.health.STORE}`,
        details:    () => `${BASE.HEALTH}/${Seg.health.DETAILS}`,
        cacheStats: () => `${BASE.HEALTH}/${Seg.health.CACHE_STATS}`,
    },
    console: {
        overview:          () => `${BASE.CONSOLE}/${Seg.console.OVERVIEW}`,
        metricsUsers:      () => `${BASE.CONSOLE}/${Seg.console.METRICS}/${Seg.console.USERS}`,
        metricsOperators:  () => `${BASE.CONSOLE}/${Seg.console.METRICS}/${Seg.console.OPERATORS}`,
        metricsSessions:   () => `${BASE.CONSOLE}/${Seg.console.METRICS}/${Seg.console.SESSIONS}`,
        metricsAI:         () => `${BASE.CONSOLE}/${Seg.console.METRICS}/${Seg.console.AI}`,
        metricsLoginAudit: () => `${BASE.CONSOLE}/${Seg.console.METRICS}/${Seg.console.LOGIN_AUDIT}`,
        metricsRealtime:   () => `${BASE.CONSOLE}/${Seg.console.METRICS}/${Seg.console.REALTIME}`,
        cacheClear:        () => `${BASE.CONSOLE}/${Seg.console.CACHE}/${Seg.console.CLEAR}`,
        componentsHealth:   () => `${BASE.CONSOLE}/${Seg.console.COMPONENTS}/${Seg.console.HEALTH}`,
        kvScan:            () => `${BASE.CONSOLE}/${Seg.console.KV}/${Seg.console.SCAN}`,
        kvKey:             () => `${BASE.CONSOLE}/${Seg.console.KV}/${Seg.console.KEY}`,
        dbQuery:           () => `${BASE.CONSOLE}/${Seg.console.DB}/${Seg.console.QUERY}`,
        dbCollections:     () => `${BASE.CONSOLE}/${Seg.console.DB}/${Seg.console.COLLECTIONS}`,
        logsStream:        () => `${BASE.CONSOLE}/${Seg.console.LOGS}/${Seg.console.STREAM}`,
    },
    metrics: {
        health: () => `${BASE.METRICS}/${Seg.metrics.HEALTH}`,
    },
    system: {
        networkInterfaces: () => `${BASE.SYSTEM}/${Seg.system.NETWORK_INTERFACES}`,
    },
    docs: {
        tree: () => `${BASE.DOCS}/${Seg.docs.TREE}`,
        file: () => `${BASE.DOCS}/${Seg.docs.FILE}`,
    },
};
