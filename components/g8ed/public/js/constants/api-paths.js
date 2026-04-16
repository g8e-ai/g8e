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
 * g8ed API Path Builders — Frontend
 *
 * Single source of truth for all g8ed endpoint paths consumed by the frontend.
 * Mirrors components/g8ed/constants/api_paths.js (server-side).
 *
 * All path strings MUST be defined here — no inline string literals elsewhere.
 *
 * Usage:
 *   import { ApiPaths } from '../constants/api-paths.js';
 *   serviceClient.get(ComponentName.G8ED, ApiPaths.operator.details(operatorId));
 */

const BASE = {
    OPERATORS:    '/api/operators',
    AUTH:         '/api/auth',
    AUTH_PASSKEY: '/api/auth/passkey',
    APPROVAL:     '/api/operator/approval',
    USER:         '/api/user',
    CHAT:         '/api/chat',
    DEVICE_LINKS: '/api/device-links',
    SSE:          '/sse',
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
        G8E_POD:        'g8ep',
        REAUTH:          'reauth',
    },
    auth: {
        WEB_SESSION:    'web-session',
        REGISTER:       'register',
        LOGOUT:         'logout',
        LINK:           'link',
        GENERATE:       'generate',
        AUTHORIZE:      'authorize',
        REJECT:         'reject',
        PASSKEY:        'passkey',
        REGISTER_CHALLENGE: 'register-challenge',
        REGISTER_VERIFY:    'register-verify',
        REGISTER_VERIFY_SETUP: 'register-verify-setup',
        AUTH_CHALLENGE:     'auth-challenge',
        AUTH_VERIFY:        'auth-verify',
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
        g8eNodeReauth: () => `${BASE.OPERATORS}/${Seg.operator.G8E_POD}/${Seg.operator.REAUTH}`,
    },
    auth: {
        webSession:     () => `${BASE.AUTH}/${Seg.auth.WEB_SESSION}`,
        register:       () => `${BASE.AUTH}/${Seg.auth.REGISTER}`,
        logout:         () => `${BASE.AUTH}/${Seg.auth.LOGOUT}`,
        linkGenerate:   () => `${BASE.AUTH}/${Seg.auth.LINK}/${Seg.auth.GENERATE}`,
        linkAuthorize:  (token) => `${BASE.AUTH}/${Seg.auth.LINK}/${token}/${Seg.auth.AUTHORIZE}`,
        linkReject:     (token) => `${BASE.AUTH}/${Seg.auth.LINK}/${token}/${Seg.auth.REJECT}`,
        passkey: {
            registerChallenge: () => `${BASE.AUTH_PASSKEY}/${Seg.auth.REGISTER_CHALLENGE}`,
            registerVerify:    () => `${BASE.AUTH_PASSKEY}/${Seg.auth.REGISTER_VERIFY}`,
            registerVerifySetup: () => `${BASE.AUTH_PASSKEY}/${Seg.auth.REGISTER_VERIFY_SETUP}`,
            authChallenge:     () => `${BASE.AUTH_PASSKEY}/${Seg.auth.AUTH_CHALLENGE}`,
            authVerify:        () => `${BASE.AUTH_PASSKEY}/${Seg.auth.AUTH_VERIFY}`,
        },
    },
    deviceLink: {
        list:   () => BASE.DEVICE_LINKS,
        create: () => BASE.DEVICE_LINKS,
        revoke: (tokenId) => `${BASE.DEVICE_LINKS}/${tokenId}`,
        delete: (tokenId) => `${BASE.DEVICE_LINKS}/${tokenId}?action=${Seg.deviceLink.DELETE}`,
    },
    user: {
        me:      () => `${BASE.USER}/${Seg.user.ME}`,
        devLogs: () => `${BASE.USER}/${Seg.user.ME}/${Seg.user.DEV_LOGS}`,
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
        events: () => `${BASE.SSE}/${Seg.sse.EVENTS}`,
        health: () => `${BASE.SSE}/${Seg.sse.HEALTH}`,
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
