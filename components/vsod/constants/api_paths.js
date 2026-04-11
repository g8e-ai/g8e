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
 * VSOD API Path Definitions
 *
 * Single source of truth for all VSOD HTTP endpoint paths.
 *
 * Route templates (Express registration):  router.get(OperatorPaths.DETAILS, ...)
 * Path builders (client URL construction): ApiPaths.g8e.details(operatorId)
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const _SHARED_DIR = path.resolve(__dirname, '../../../shared/constants');
let _sharedApiPaths = {};
try {
    const apiPathsFile = path.join(_SHARED_DIR, 'api_paths.json');
    _sharedApiPaths = JSON.parse(fs.readFileSync(apiPathsFile, 'utf8'));
} catch (err) {
    // Fallback for tests or environments where shared file isn't available
    _sharedApiPaths = {
        internal_prefix: '/api/internal',
        vse: {
            chat: '/chat',
            chat_stop: '/chat/stop',
            investigations: '/investigations',
            investigation: '/investigations/{investigation_id}',
            cases: '/cases',
            case: '/cases/{case_id}',
            operators_stop: '/operators/stop',
            operators_register_session: '/operators/register-operator-session',
            operators_deregister_session: '/operators/deregister-operator-session',
            operator_direct_command: '/operator/direct-command',
            operator_approval_respond: '/operator/approval/respond',
            health: '/health',
            settings_user: '/settings/user',
            mcp_tools_list: '/mcp/tools/list',
            mcp_tools_call: '/mcp/tools/call'
        },
        vsod: {
            sse_push: '/sse/push',
            grant_intent: '/operators/{operator_id}/grant-intent',
            revoke_intent: '/operators/{operator_id}/revoke-intent',
            health: '/health'
        }
    };
}

export const InternalApiPaths = Object.freeze({
    PREFIX: _sharedApiPaths.internal_prefix,
    vse: Object.fromEntries(
        Object.entries(_sharedApiPaths.vse).map(([k, v]) => [k, _sharedApiPaths.internal_prefix + v])
    ),
    vsod: Object.fromEntries(
        Object.entries(_sharedApiPaths.vsod).map(([k, v]) => [k, _sharedApiPaths.internal_prefix + v])
    )
});

// ---------------------------------------------------------------------------
// Domain base paths — exported for use in server.js route mounting and tests
// ---------------------------------------------------------------------------

export const BasePaths = Object.freeze({
    OPERATOR:         '/operator',
    OPERATOR_API:     '/api/operators',
    OPERATOR_APPROVAL:'/api/operator/approval',
    AUTH:             '/api/auth',
    AUTH_PASSKEY:     '/api/auth/passkey',
    AUTH_LINK:        '/auth/link',
    DEVICE_LINKS:     '/api/device-links',
    USER:             '/api/user',
    CHAT:             '/api/chat',
    SSE:              '/sse',
    HEALTH:           '/health',
    METRICS:          '/api/metrics',
    AUDIT:            '/api/audit',
    CONSOLE:          '/api/console',
    SETTINGS:         '/api/settings',
    SYSTEM:           '/api/system',
    INTERNAL:         '/api/internal',
    INTERNAL_SSE:     '/api/internal/sse',
    INTERNAL_OPERATORS: '/api/internal/operators',
    INTERNAL_USERS:    '/api/internal/users',
    INTERNAL_SETTINGS: '/api/internal/settings',
    INTERNAL_SESSION:  '/api/internal/session',
    DOCS:             '/api/docs',
    SETUP:            '/api/setup',
    MCP:              '/mcp',
});

// ---------------------------------------------------------------------------
// Domain segments — private building blocks, never used directly outside this file
// ---------------------------------------------------------------------------

// --- OPERATOR domain ---
const Operator = {
    BASE:              '/api/operators',
    BIND:              'bind',
    UNBIND:            'unbind',
    BIND_ALL:          'bind-all',
    UNBIND_ALL:        'unbind-all',
    DETAILS:           'details',
    STOP:              'stop',
    API_KEY:           'api-key',
    REFRESH_API_KEY:   'refresh-api-key',
    DOWNLOAD:          'download',
    HEALTH:            'health',
    G8E_POD:          'g8e-pod',
    REAUTH:            'reauth',
    PARAM:             ':operatorId',
};

// --- OPERATOR download/health (mounted at /operator, not /api/operators) ---
const OperatorBin = {
    BASE: BasePaths.OPERATOR,
};

// --- AUTH domain ---
const Auth = {
    BASE:             BasePaths.AUTH,
    WEB_SESSION:      'web-session',
    LOGOUT:           'logout',
    REGISTER:         'register',
    OPERATOR:         'operator',
    REFRESH:          'refresh',
    LINK:             'link',
    GENERATE:         'generate',
    ADMIN:            'admin',
    LOCKED_ACCOUNTS:  'locked-accounts',
    UNLOCK_ACCOUNT:   'unlock-account',
    ACCOUNT_STATUS:   'account-status',
    PARAM_USER_ID: ':userId',
    PARAM_TOKEN:      ':token',
    REGISTER_TOKEN:   '/auth/link',
    AUTHORIZE:        'authorize',
    REJECT:           'reject',
};

// --- PASSKEY domain (mounted under /api/auth/passkey) ---
const Passkey = {
    BASE:               BasePaths.AUTH_PASSKEY,
    REGISTER_CHALLENGE: 'register-challenge',
    REGISTER_VERIFY:    'register-verify',
    AUTH_CHALLENGE:     'auth-challenge',
    AUTH_VERIFY:        'auth-verify',
};

// --- DEVICE LINK domain ---
const DeviceLink = {
    BASE:     BasePaths.DEVICE_LINKS,
    PARAM:    ':token',
    DELETE:   'delete',
    REGISTER: 'register',
};

// --- USER domain ---
const User = {
    BASE:            BasePaths.USER,
    ME:              'me',
    DEV_LOGS:        'dev-logs',
    REFRESH_G8E_KEY: 'refresh-g8e-key',
};

// --- CHAT domain ---
const Chat = {
    BASE:            BasePaths.CHAT,
    SEND:            'send',
    INVESTIGATIONS:  'investigations',
    STOP:            'stop',
    CASES:           'cases',
    HEALTH:          'health',
    PARAM:           ':investigationId',
    CASE_PARAM:      ':caseId',
};

// --- OPERATOR APPROVAL domain ---
const Approval = {
    BASE:            BasePaths.OPERATOR_APPROVAL,
    RESPOND:         'respond',
    DIRECT_COMMAND:  'direct-command',
    PENDING:         'pending',
};

// --- SSE domain ---
const SSE = {
    BASE:   BasePaths.SSE,
    EVENTS: 'events',
    HEALTH: 'health',
};

// --- AUDIT domain ---
const Audit = {
    BASE:     BasePaths.AUDIT,
    EVENTS:   'events',
    DOWNLOAD: 'download',
};

// --- HEALTH domain ---
const Health = {
    BASE:        BasePaths.HEALTH,
    LIVE:        'live',
    STORE:       'store',
    DETAILS:     'details',
    CACHE_STATS: 'cache-stats',
};

// --- CONSOLE domain ---
const Console = {
    BASE:         BasePaths.CONSOLE,
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
};

// --- DOCS domain ---
const Docs = {
    BASE: BasePaths.DOCS,
    TREE: 'tree',
    FILE: 'file',
};

// --- SYSTEM domain ---
const System = {
    BASE:               BasePaths.SYSTEM,
    NETWORK_INTERFACES: 'network-interfaces',
};

// --- SETTINGS domain ---
const Settings = {
    BASE: BasePaths.SETTINGS,
};

// --- INTERNAL DEVICE LINK domain ---
const InternalDeviceLink = {
    BASE:       '/api/internal/device-links',
    USER:       'user',
    PARAM_USER: ':userId',
    PARAM_TOKEN: ':token',
    DELETE:     'delete',
};

// --- INTERNAL domain ---
const Internal = {
    BASE:               BasePaths.INTERNAL,
    HEALTH:             'health',
    SSE:                'sse',
    PUSH:               'push',
    OPERATORS:          'operators',
    USER:               'user',
    USERS:              'users',
    STATS:              'stats',
    EMAIL:              'email',
    SESSION:            'session',
    REAUTH:             'reauth',
    INITIALIZE_SLOTS:   'initialize-slots',
    STATUS:             'status',
    WITH_SESSION:       'with-session-context',
    HEARTBEAT:          'heartbeat',
    CONTEXT:            'context',
    RESET_CACHE:        'reset-cache',
    REFRESH_KEY:        'refresh-key',
    PASSKEYS:           'passkeys',
    ROLES:              'roles',
    PARAM_OPERATOR:     ':operatorId',
    PARAM_USER:         ':userId',
    PARAM_SESSION:      ':sessionId',
};

// ---------------------------------------------------------------------------
// Route templates — used in Express router.get/post/etc handlers
// ---------------------------------------------------------------------------

export const OperatorPaths = Object.freeze({
    BIND:              `/${Operator.BIND}`,
    UNBIND:            `/${Operator.UNBIND}`,
    BIND_ALL:          `/${Operator.BIND_ALL}`,
    UNBIND_ALL:        `/${Operator.UNBIND_ALL}`,
    LIST:              '/',
    DETAILS:           `/${Operator.PARAM}/${Operator.DETAILS}`,
    STOP:              `/${Operator.PARAM}/${Operator.STOP}`,
    API_KEY:           `/${Operator.PARAM}/${Operator.API_KEY}`,
    REFRESH_API_KEY:   `/${Operator.PARAM}/${Operator.REFRESH_API_KEY}`,
    DOWNLOAD:          `/${Operator.DOWNLOAD}/${':os'}/${':arch'}`,
    DOWNLOAD_SHA256:   `/${Operator.DOWNLOAD}/${':os'}/${':arch'}/sha256`,
    HEALTH:            `/${Operator.HEALTH}`,
    G8E_GATEWAY_REAUTH:   `/${Operator.G8E_POD}/${Operator.REAUTH}`,
});

export const AuthPaths = Object.freeze({
    WEB_SESSION:      `/${Auth.WEB_SESSION}`,
    LOGOUT:           `/${Auth.LOGOUT}`,
    REGISTER:         `/${Auth.REGISTER}`,
    OPERATOR_AUTH:    `/${Auth.OPERATOR}`,
    OPERATOR_REFRESH: `/${Auth.OPERATOR}/${Auth.REFRESH}`,
    LINK_GENERATE:    `/${Auth.LINK}/${Auth.GENERATE}`,
    ADMIN_LOCKED_ACCOUNTS: `/${Auth.ADMIN}/${Auth.LOCKED_ACCOUNTS}`,
    ADMIN_UNLOCK_ACCOUNT:  `/${Auth.ADMIN}/${Auth.UNLOCK_ACCOUNT}`,
    ADMIN_ACCOUNT_STATUS:  `/${Auth.ADMIN}/${Auth.ACCOUNT_STATUS}/${Auth.PARAM_USER_ID}`,
});

export const PasskeyPaths = Object.freeze({
    REGISTER_CHALLENGE: `/${Passkey.REGISTER_CHALLENGE}`,
    REGISTER_VERIFY:    `/${Passkey.REGISTER_VERIFY}`,
    AUTH_CHALLENGE:     `/${Passkey.AUTH_CHALLENGE}`,
    AUTH_VERIFY:        `/${Passkey.AUTH_VERIFY}`,
});

export const DeviceLinkPaths = Object.freeze({
    LIST:     '/',
    CREATE:   '/',
    REVOKE:   `/${DeviceLink.PARAM}`,
    REGISTER: `/${DeviceLink.PARAM}/${DeviceLink.REGISTER}`,
});

export const UserPaths = Object.freeze({
    ME:             `/${User.ME}`,
    DEV_LOGS:       `/${User.ME}/${User.DEV_LOGS}`,
    REFRESH_G8E_KEY: `/${User.ME}/${User.REFRESH_G8E_KEY}`,
});

export const ChatPaths = Object.freeze({
    SEND:           `/${Chat.SEND}`,
    INVESTIGATIONS: `/${Chat.INVESTIGATIONS}`,
    INVESTIGATION:  `/${Chat.INVESTIGATIONS}/${Chat.PARAM}`,
    STOP:           `/${Chat.STOP}`,
    CASES:          `/${Chat.CASES}/${Chat.CASE_PARAM}`,
    HEALTH:         `/${Chat.HEALTH}`,
});

export const OperatorApprovalPaths = Object.freeze({
    RESPOND:        `/${Approval.RESPOND}`,
    DIRECT_COMMAND: `/${Approval.DIRECT_COMMAND}`,
    PENDING:        `/${Approval.PENDING}`,
});

export const SSEPaths = Object.freeze({
    EVENTS: `/${SSE.EVENTS}`,
    HEALTH: `/${SSE.HEALTH}`,
});

export const AuditPaths = Object.freeze({
    EVENTS:   `/${Audit.EVENTS}`,
    DOWNLOAD: `/${Audit.DOWNLOAD}`,
});

export const HealthPaths = Object.freeze({
    ROOT:        '/',
    LIVE:        `/${Health.LIVE}`,
    STORE:       `/${Health.STORE}`,
    DETAILS:     `/${Health.DETAILS}`,
    CACHE_STATS: `/${Health.CACHE_STATS}`,
});

export const ConsolePaths = Object.freeze({
    OVERVIEW:            `/${Console.OVERVIEW}`,
    METRICS_USERS:       `/${Console.METRICS}/${Console.USERS}`,
    METRICS_OPERATORS:   `/${Console.METRICS}/${Console.OPERATORS}`,
    METRICS_SESSIONS:    `/${Console.METRICS}/${Console.SESSIONS}`,
    METRICS_AI:          `/${Console.METRICS}/${Console.AI}`,
    METRICS_LOGIN_AUDIT: `/${Console.METRICS}/${Console.LOGIN_AUDIT}`,
    METRICS_REALTIME:    `/${Console.METRICS}/${Console.REALTIME}`,
    CACHE_CLEAR:         `/${Console.CACHE}/${Console.CLEAR}`,
    COMPONENTS_HEALTH:   `/${Console.COMPONENTS}/${Console.HEALTH}`,
    KV_SCAN:             `/${Console.KV}/${Console.SCAN}`,
    KV_KEY:              `/${Console.KV}/${Console.KEY}`,
    DB_QUERY:            `/${Console.DB}/${Console.QUERY}`,
    DB_COLLECTIONS:      `/${Console.DB}/${Console.COLLECTIONS}`,
    LOGS_STREAM:         `/${Console.LOGS}/${Console.STREAM}`,
});

export const DocsPaths = Object.freeze({
    TREE: `/${Docs.TREE}`,
    FILE: `/${Docs.FILE}`,
});

export const SystemPaths = Object.freeze({
    NETWORK_INTERFACES: `/${System.NETWORK_INTERFACES}`,
});

// --- METRICS domain ---
const Metrics = {
    BASE:   BasePaths.METRICS,
    HEALTH: 'health',
};

export const MetricsPaths = Object.freeze({
    HEALTH: `/${Metrics.HEALTH}`,
});

export const SettingsPaths = Object.freeze({
    ROOT: '/',
});

export const MCPPaths = Object.freeze({
    ROOT: '/',
});

// --- SETUP domain ---
const Setup = {
    BASE:   BasePaths.SETUP,
};

export const SetupPaths = Object.freeze({
    WIZARD:  '/setup',
});

export const InternalDeviceLinkPaths = Object.freeze({
    LIST_FOR_USER:   `/${InternalDeviceLink.USER}/${InternalDeviceLink.PARAM_USER}`,
    CREATE_FOR_USER: `/${InternalDeviceLink.USER}/${InternalDeviceLink.PARAM_USER}`,
    REVOKE:          `/${InternalDeviceLink.PARAM_TOKEN}`,
});

export const InternalPaths = Object.freeze({
    HEALTH:                    InternalApiPaths.vsod.health,
    SSE_PUSH:                  InternalApiPaths.vsod.sse_push,
    OPERATORS_USER:            `/${Internal.OPERATORS}/${Internal.USER}/${Internal.PARAM_USER}`,
    OPERATORS_USER_REAUTH:     `/${Internal.OPERATORS}/${Internal.USER}/${Internal.PARAM_USER}/${Internal.REAUTH}`,
    OPERATORS_USER_INIT_SLOTS: `/${Internal.OPERATORS}/${Internal.USER}/${Internal.PARAM_USER}/${Internal.INITIALIZE_SLOTS}`,
    OPERATOR:                  `/${Internal.OPERATORS}/${Internal.PARAM_OPERATOR}`,
    OPERATOR_STATUS:           `/${Internal.OPERATORS}/${Internal.PARAM_OPERATOR}/${Internal.STATUS}`,
    OPERATOR_WITH_SESSION:     `/${Internal.OPERATORS}/${Internal.PARAM_OPERATOR}/${Internal.WITH_SESSION}`,
    OPERATOR_HEARTBEAT:        `/${Internal.OPERATORS}/${Internal.PARAM_OPERATOR}/${Internal.HEARTBEAT}`,
    OPERATOR_CONTEXT:          `/${Internal.OPERATORS}/${Internal.PARAM_OPERATOR}/${Internal.CONTEXT}`,
    OPERATOR_RESET_CACHE:      `/${Internal.OPERATORS}/${Internal.PARAM_OPERATOR}/${Internal.RESET_CACHE}`,
    OPERATOR_REFRESH_KEY:      `/${Internal.OPERATORS}/${Internal.PARAM_OPERATOR}/${Internal.REFRESH_KEY}`,
    OPERATOR_GRANT_INTENT:     InternalApiPaths.vsod.grant_intent.replace('{operator_id}', Internal.PARAM_OPERATOR),
    OPERATOR_REVOKE_INTENT:    InternalApiPaths.vsod.revoke_intent.replace('{operator_id}', Internal.PARAM_OPERATOR),
    USERS:                     `/${Internal.USERS}`,
    USERS_STATS:               `/${Internal.USERS}/${Internal.STATS}`,
    USER:                      `/${Internal.USERS}/${Internal.PARAM_USER}`,
    USER_EMAIL:                `/${Internal.USERS}/${Internal.EMAIL}/${':email'}`,
    USER_PASSKEYS:             `/${Internal.USERS}/${Internal.PARAM_USER}/${Internal.PASSKEYS}`,
    USER_PASSKEY:              `/${Internal.USERS}/${Internal.PARAM_USER}/${Internal.PASSKEYS}/:credentialId`,
    USER_ROLES:                `/${Internal.USERS}/${Internal.PARAM_USER}/${Internal.ROLES}`,
    SESSION:                   `/${Internal.SESSION}/${Internal.PARAM_SESSION}`,
    SETTINGS:                  '/settings',
});

// ---------------------------------------------------------------------------
// Fully-qualified path builders — used by client code to construct URLs
// ---------------------------------------------------------------------------

export const apiPaths = {
    vse: {
        chat:                      () => InternalApiPaths.vse.chat,
        chatStop:                  () => InternalApiPaths.vse.chat_stop,
        investigations:            () => InternalApiPaths.vse.investigations,
        investigation:             (id) => InternalApiPaths.vse.investigation.replace('{investigation_id}', id),
        cases:                     () => InternalApiPaths.vse.cases,
        case:                      (id) => InternalApiPaths.vse.case.replace('{case_id}', id),
        operatorsStop:             () => InternalApiPaths.vse.operators_stop,
        operatorsRegisterSession:   () => InternalApiPaths.vse.operators_register_session,
        operatorsDeregisterSession: () => InternalApiPaths.vse.operators_deregister_session,
        operatorDirectCommand:     () => InternalApiPaths.vse.operator_direct_command,
        operatorApprovalRespond:   () => InternalApiPaths.vse.operator_approval_respond,
        operatorApprovalPending:   () => InternalApiPaths.vse.operator_approval_pending,
        health:                    () => InternalApiPaths.vse.health,
        settingsUser:              () => InternalApiPaths.vse.settings_user,
        mcpToolsList:              () => InternalApiPaths.vse.mcp_tools_list,
        mcpToolsCall:              () => InternalApiPaths.vse.mcp_tools_call,
    },
    operator: {
        bind:           () => `${Operator.BASE}/${Operator.BIND}`,
        unbind:         () => `${Operator.BASE}/${Operator.UNBIND}`,
        bindAll:        () => `${Operator.BASE}/${Operator.BIND_ALL}`,
        unbindAll:      () => `${Operator.BASE}/${Operator.UNBIND_ALL}`,
        list:           () => `${Operator.BASE}`,
        details:        (operatorId) => `${Operator.BASE}/${operatorId}/${Operator.DETAILS}`,
        stop:           (operatorId) => `${Operator.BASE}/${operatorId}/${Operator.STOP}`,
        apiKey:         (operatorId) => `${Operator.BASE}/${operatorId}/${Operator.API_KEY}`,
        refreshApiKey:  (operatorId) => `${Operator.BASE}/${operatorId}/${Operator.REFRESH_API_KEY}`,
        download:       (os, arch)   => `${OperatorBin.BASE}/${Operator.DOWNLOAD}/${os}/${arch}`,
        downloadSha256: (os, arch)   => `${OperatorBin.BASE}/${Operator.DOWNLOAD}/${os}/${arch}/sha256`,
        health:         () => `${OperatorBin.BASE}/${Operator.HEALTH}`,
        g8eNodeReauth:  () => `${Operator.BASE}/${Operator.G8E_POD}/${Operator.REAUTH}`,
    },
    auth: {
        webSession:          () => `${Auth.BASE}/${Auth.WEB_SESSION}`,
        logout:              () => `${Auth.BASE}/${Auth.LOGOUT}`,
        register:            () => `${Auth.BASE}/${Auth.REGISTER}`,
        operatorAuth:        () => `${Auth.BASE}/${Auth.OPERATOR}`,
        operatorRefresh:     () => `${Auth.BASE}/${Auth.OPERATOR}/${Auth.REFRESH}`,
        linkGenerate:        () => `${Auth.BASE}/${Auth.LINK}/${Auth.GENERATE}`,
        linkAuthorize:       (token) => `${Auth.BASE}/${Auth.LINK}/${token}/${Auth.AUTHORIZE}`,
        linkReject:          (token) => `${Auth.BASE}/${Auth.LINK}/${token}/${Auth.REJECT}`,
        adminLockedAccounts: () => `${Auth.BASE}/${Auth.ADMIN}/${Auth.LOCKED_ACCOUNTS}`,
        adminUnlockAccount:  () => `${Auth.BASE}/${Auth.ADMIN}/${Auth.UNLOCK_ACCOUNT}`,
        adminAccountStatus:  (userId) => `${Auth.BASE}/${Auth.ADMIN}/${Auth.ACCOUNT_STATUS}/${userId}`,
    },
    passkey: {
        registerChallenge: () => `${Passkey.BASE}/${Passkey.REGISTER_CHALLENGE}`,
        registerVerify:    () => `${Passkey.BASE}/${Passkey.REGISTER_VERIFY}`,
        authChallenge:     () => `${Passkey.BASE}/${Passkey.AUTH_CHALLENGE}`,
        authVerify:        () => `${Passkey.BASE}/${Passkey.AUTH_VERIFY}`,
    },
    deviceLink: {
        list:     () => DeviceLink.BASE,
        create:   () => DeviceLink.BASE,
        revoke:   (token) => `${DeviceLink.BASE}/${token}`,
        delete:   (token) => `${DeviceLink.BASE}/${token}?action=${DeviceLink.DELETE}`,
        register: (token) => `${Auth.REGISTER_TOKEN}/${token}/${DeviceLink.REGISTER}`,
    },
    user: {
        me:              () => `${User.BASE}/${User.ME}`,
        devLogs:         () => `${User.BASE}/${User.ME}/${User.DEV_LOGS}`,
        refreshG8eKey:  () => `${User.BASE}/${User.ME}/${User.REFRESH_G8E_KEY}`,
    },
    chat: {
        send:           () => `${Chat.BASE}/${Chat.SEND}`,
        investigations: () => `${Chat.BASE}/${Chat.INVESTIGATIONS}`,
        investigation:  (investigationId) => `${Chat.BASE}/${Chat.INVESTIGATIONS}/${investigationId}`,
        stop:           () => `${Chat.BASE}/${Chat.STOP}`,
        deleteCase:     (caseId) => `${Chat.BASE}/${Chat.CASES}/${caseId}`,
        health:         () => `${Chat.BASE}/${Chat.HEALTH}`,
    },
    approval: {
        respond:       () => `${Approval.BASE}/${Approval.RESPOND}`,
        directCommand: () => `${Approval.BASE}/${Approval.DIRECT_COMMAND}`,
    },
    sse: {
        events: () => `${SSE.BASE}/${SSE.EVENTS}`,
        health: () => `${SSE.BASE}/${SSE.HEALTH}`,
    },
    audit: {
        events:   () => `${Audit.BASE}/${Audit.EVENTS}`,
        download: () => `${Audit.BASE}/${Audit.DOWNLOAD}`,
    },
    health: {
        root:       () => `${Health.BASE}`,
        live:       () => `${Health.BASE}/${Health.LIVE}`,
        store:      () => `${Health.BASE}/${Health.STORE}`,
        details:    () => `${Health.BASE}/${Health.DETAILS}`,
        cacheStats: () => `${Health.BASE}/${Health.CACHE_STATS}`,
    },
    console: {
        overview:          () => `${Console.BASE}/${Console.OVERVIEW}`,
        metricsUsers:      () => `${Console.BASE}/${Console.METRICS}/${Console.USERS}`,
        metricsOperators:  () => `${Console.BASE}/${Console.METRICS}/${Console.OPERATORS}`,
        metricsSessions:   () => `${Console.BASE}/${Console.METRICS}/${Console.SESSIONS}`,
        metricsAI:         () => `${Console.BASE}/${Console.METRICS}/${Console.AI}`,
        metricsLoginAudit: () => `${Console.BASE}/${Console.METRICS}/${Console.LOGIN_AUDIT}`,
        metricsRealtime:   () => `${Console.BASE}/${Console.METRICS}/${Console.REALTIME}`,
        cacheClear:        () => `${Console.BASE}/${Console.CACHE}/${Console.CLEAR}`,
        componentsHealth:  () => `${Console.BASE}/${Console.COMPONENTS}/${Console.HEALTH}`,
        kvScan:            () => `${Console.BASE}/${Console.KV}/${Console.SCAN}`,
        kvKey:             () => `${Console.BASE}/${Console.KV}/${Console.KEY}`,
        dbQuery:           () => `${Console.BASE}/${Console.DB}/${Console.QUERY}`,
        dbCollections:     () => `${Console.BASE}/${Console.DB}/${Console.COLLECTIONS}`,
        logsStream:        () => `${Console.BASE}/${Console.LOGS}/${Console.STREAM}`,
    },
    docs: {
        tree: () => `${Docs.BASE}/${Docs.TREE}`,
        file: () => `${Docs.BASE}/${Docs.FILE}`,
    },
    system: {
        networkInterfaces: () => `${System.BASE}/${System.NETWORK_INTERFACES}`,
    },
    settings: {
        root: () => `${Settings.BASE}`,
    },
    setup: {
        wizard: () => '/setup',
    },
    metrics: {
        health: () => `${Metrics.BASE}/${Metrics.HEALTH}`,
    },
    internal: {
        health:              () => `${Internal.BASE}/${Internal.HEALTH}`,
        ssePush:             () => `${Internal.BASE}/${Internal.SSE}/${Internal.PUSH}`,
        operatorsForUser:    (userId) => `${Internal.BASE}/${Internal.OPERATORS}/${Internal.USER}/${userId}`,
        operatorReauth:      (userId) => `${Internal.BASE}/${Internal.OPERATORS}/${Internal.USER}/${userId}/${Internal.REAUTH}`,
        operatorInitSlots:   (userId) => `${Internal.BASE}/${Internal.OPERATORS}/${Internal.USER}/${userId}/${Internal.INITIALIZE_SLOTS}`,
        operator:            (operatorId) => `${Internal.BASE}/${Internal.OPERATORS}/${operatorId}`,
        operatorStatus:      (operatorId) => `${Internal.BASE}/${Internal.OPERATORS}/${operatorId}/${Internal.STATUS}`,
        operatorWithSession: (operatorId) => `${Internal.BASE}/${Internal.OPERATORS}/${operatorId}/${Internal.WITH_SESSION}`,
        operatorHeartbeat:   (operatorId) => `${Internal.BASE}/${Internal.OPERATORS}/${operatorId}/${Internal.HEARTBEAT}`,
        operatorContext:     (operatorId) => `${Internal.BASE}/${Internal.OPERATORS}/${operatorId}/${Internal.CONTEXT}`,
        operatorResetCache:  (operatorId) => `${Internal.BASE}/${Internal.OPERATORS}/${operatorId}/${Internal.RESET_CACHE}`,
        operatorRefreshKey:  (operatorId) => `${Internal.BASE}/${Internal.OPERATORS}/${operatorId}/${Internal.REFRESH_KEY}`,
        operatorGrantIntent: (operatorId) => (InternalApiPaths.vsod?.grant_intent || '/operators/{operator_id}/grant-intent').replace('{operator_id}', operatorId),
        operatorRevokeIntent: (operatorId) => (InternalApiPaths.vsod?.revoke_intent || '/operators/{operator_id}/revoke-intent').replace('{operator_id}', operatorId),
        users:               () => `${Internal.BASE}/${Internal.USERS}`,
        usersStats:          () => `${Internal.BASE}/${Internal.USERS}/${Internal.STATS}`,
        user:                (userId) => `${Internal.BASE}/${Internal.USERS}/${userId}`,
        userPasskeys:        (userId) => `${Internal.BASE}/${Internal.USERS}/${userId}/${Internal.PASSKEYS}`,
        userPasskey:         (userId, credentialId) => `${Internal.BASE}/${Internal.USERS}/${userId}/${Internal.PASSKEYS}/${credentialId}`,
        userByEmail:         (email)  => `${Internal.BASE}/${Internal.USERS}/${Internal.EMAIL}/${email}`,
        userResetPassword:   (userId) => `${Internal.BASE}/${Internal.USERS}/${userId}/${Internal.RESET_PASSWORD}`,
        userRoles:           (userId) => `${Internal.BASE}/${Internal.USERS}/${userId}/${Internal.ROLES}`,
        session:             (sessionId) => `${Internal.BASE}/${Internal.SESSION}/${sessionId}`,
        settings:            () => '/api/internal/settings',
    },
    internalDeviceLink: {
        listForUser:   (userId) => `${InternalDeviceLink.BASE}/${InternalDeviceLink.USER}/${userId}`,
        createForUser: (userId) => `${InternalDeviceLink.BASE}/${InternalDeviceLink.USER}/${userId}`,
        revoke:        (token)  => `${InternalDeviceLink.BASE}/${token}`,
        delete:        (token)  => `${InternalDeviceLink.BASE}/${token}?action=${InternalDeviceLink.DELETE}`,
    },
};

// Aliased as ApiPaths for legacy/mirror contract support
export const ApiPaths = apiPaths;
