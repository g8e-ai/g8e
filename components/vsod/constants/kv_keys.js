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
 * VSODB KV Key Constants
 * Single source of truth for ALL KV store keys used across the platform (VSOD + g8ee).
 *
 * All keys are built via KVKey.* functions. Never construct key strings manually.
 *
 * --- CACHE ---   KVKey.doc(collection, id)  /  KVKey.query(collection, hash)
 * --- SESSION ---  KVKey.webSessionKey(id)  /  KVKey.operatorSessionKey(id)
 *                  KVKey.sessionBindOperators(operatorSessionId)  /  KVKey.sessionWebBind(webSessionId)
 * --- OPERATOR --- KVKey.operatorFirstDeployed(id)  /  KVKey.operatorTrackedStatus(id)
 * --- USER ---     KVKey.userOperators(id)  /  KVKey.userWebSessions(id)  /  KVKey.userMemories(id)
 * --- INVESTIGATION --- KVKey.attachment(invId, attId)  /  KVKey.attachmentIndex(invId)
 * --- AUTH ---     KVKey.nonce(n)  /  KVKey.downloadToken(t)  /  KVKey.deviceLink(t)
 *                  KVKey.deviceLinkUses(t)  /  KVKey.deviceLinkFingerprints(t)  /  KVKey.deviceLinkRegistrationLock(t)
 *                  KVKey.deviceLinkList(userId)  /  KVKey.loginFailed(id)  /  KVKey.loginLock(id)  /  KVKey.loginIpAccounts(ip)
 *                  KVKey.passkeyPendingRegistration(token)
 * --- EXECUTION -- KVKey.pendingCmd(executionId)
 */

import { _KV } from './shared.js';

export const CACHE_PREFIX = _KV['cache.prefix'];

// --- CACHE domain ---
const Cache = {
    DOMAIN:     'cache',
    DOC:        'doc',
    QUERY:      'query',
};

// --- SESSION domain ---
const SessionDomain = {
    DOMAIN:     'session',
    OPERATOR:   'operator',
    WEB:        'web',
    BIND:       'bind',
};

// --- OPERATOR domain ---
const Operator = {
    DOMAIN:         'operator',
    FIRST_DEPLOYED: 'first.deployed',
    TRACKED_STATUS: 'tracked.status',
};

// --- USER domain ---
const User = {
    DOMAIN:       'user',
    OPERATORS:    'operators',
    WEB_SESSIONS: 'web_sessions',
    MEMORIES:     'memories',
};

// --- INVESTIGATION domain ---
const Investigation = {
    DOMAIN:           'investigation',
    ATTACHMENT:       'attachment',
    ATTACHMENT_INDEX: 'attachment.index',
};

// --- AUTH domain ---
const Auth = {
    DOMAIN:            'auth',
    NONCE:             'nonce',
    TOKEN:             'token',
    DOWNLOAD:          'download',
    DEVICE:            'device',
    USES:              'uses',
    FINGERPRINTS:      'fingerprints',
    REG_LOCK:          'reg.lock',
    DEVICE_LIST:       'device.list',
    LOGIN:             'login',
    IP:                'ip',
    FAILED:            'failed',
    LOCK:              'lock',
    ACCOUNTS:          'accounts',
    PASSKEY:           'passkey',
    CHALLENGE:         'challenge',
    PENDING:           'pending',
};

// --- EXECUTION domain ---
const Execution = {
    DOMAIN:      'execution',
    PENDING_CMD: 'pending.cmd',
};

const V = CACHE_PREFIX;

export const KVKeyPrefix = {
    // Cache
    CACHE_DOC:                     `${V}:${Cache.DOMAIN}:${Cache.DOC}:`,
    CACHE_QUERY:                   `${V}:${Cache.DOMAIN}:${Cache.QUERY}:`,
    // Session
    SESSION:                       `${V}:${SessionDomain.DOMAIN}:`,
    SESSION_OPERATOR:              `${V}:${SessionDomain.DOMAIN}:${SessionDomain.OPERATOR}:`,
    SESSION_WEB:                   `${V}:${SessionDomain.DOMAIN}:${SessionDomain.WEB}:`,
    // Operator
    OPERATOR:                      `${V}:${Operator.DOMAIN}:`,
    // User
    USER:                          `${V}:${User.DOMAIN}:`,
    // Investigation
    INVESTIGATION:                 `${V}:${Investigation.DOMAIN}:`,
    // Auth
    AUTH_TOKEN_DEVICE:             `${V}:${Auth.DOMAIN}:${Auth.TOKEN}:${Auth.DEVICE}:`,
    AUTH_TOKEN_DOWNLOAD:           `${V}:${Auth.DOMAIN}:${Auth.TOKEN}:${Auth.DOWNLOAD}:`,
    AUTH_DEVICE_LIST:              `${V}:${Auth.DOMAIN}:${Auth.DEVICE_LIST}:`,
    AUTH_LOGIN:                    `${V}:${Auth.DOMAIN}:${Auth.LOGIN}:`,
    AUTH_LOGIN_IP:                 `${V}:${Auth.DOMAIN}:${Auth.LOGIN}:${Auth.IP}:`,
    // Execution
    EXECUTION:                     `${V}:${Execution.DOMAIN}:`,
};

export const KVKey = {
    // Cache
    doc:   (collection, id)   => `${V}:${Cache.DOMAIN}:${Cache.DOC}:${collection}:${id}`,
    query: (collection, hash) => `${V}:${Cache.DOMAIN}:${Cache.QUERY}:${collection}:${hash}`,

    // Session — explicitly typed, never generic
    webSessionKey:        (webSessionId)             => `${V}:${SessionDomain.DOMAIN}:${SessionDomain.WEB}:${webSessionId}`,
    operatorSessionKey:   (operatorSessionId)        => `${V}:${SessionDomain.DOMAIN}:${SessionDomain.OPERATOR}:${operatorSessionId}`,
    sessionBindOperators:  (operatorSessionId)        => `${V}:${SessionDomain.DOMAIN}:${SessionDomain.OPERATOR}:${operatorSessionId}:${SessionDomain.BIND}`,
    sessionWebBind:       (webSessionId)             => `${V}:${SessionDomain.DOMAIN}:${SessionDomain.WEB}:${webSessionId}:${SessionDomain.BIND}`,

    // Operator
    operatorFirstDeployed: (operatorId) => `${V}:${Operator.DOMAIN}:${operatorId}:${Operator.FIRST_DEPLOYED}`,
    operatorTrackedStatus: (operatorId) => `${V}:${Operator.DOMAIN}:${operatorId}:${Operator.TRACKED_STATUS}`,

    // User
    userOperators:   (userId) => `${V}:${User.DOMAIN}:${userId}:${User.OPERATORS}`,
    userWebSessions: (userId) => `${V}:${User.DOMAIN}:${userId}:${User.WEB_SESSIONS}`,
    userMemories:    (userId) => `${V}:${User.DOMAIN}:${userId}:${User.MEMORIES}`,

    // Investigation
    attachment:      (investigationId, attachmentId) => `${V}:${Investigation.DOMAIN}:${investigationId}:${Investigation.ATTACHMENT}:${attachmentId}`,
    attachmentIndex: (investigationId)               => `${V}:${Investigation.DOMAIN}:${investigationId}:${Investigation.ATTACHMENT_INDEX}`,

    // Auth
    passkeyChallenge:            (userId) => `${V}:${Auth.DOMAIN}:${Auth.PASSKEY}:${Auth.CHALLENGE}:${userId}`,
    passkeyPendingRegistration:  (token)  => `${V}:${Auth.DOMAIN}:${Auth.PASSKEY}:${Auth.PENDING}:${token}`,
    nonce:                    (nonce)       => `${V}:${Auth.DOMAIN}:${Auth.NONCE}:${nonce}`,
    downloadToken:            (token)       => `${V}:${Auth.DOMAIN}:${Auth.TOKEN}:${Auth.DOWNLOAD}:${token}`,
    deviceLink:               (token)       => `${V}:${Auth.DOMAIN}:${Auth.TOKEN}:${Auth.DEVICE}:${token}`,
    deviceLinkUses:           (token)       => `${V}:${Auth.DOMAIN}:${Auth.TOKEN}:${Auth.DEVICE}:${token}:${Auth.USES}`,
    deviceLinkFingerprints:   (token)       => `${V}:${Auth.DOMAIN}:${Auth.TOKEN}:${Auth.DEVICE}:${token}:${Auth.FINGERPRINTS}`,
    deviceLinkRegistrationLock: (token)     => `${V}:${Auth.DOMAIN}:${Auth.TOKEN}:${Auth.DEVICE}:${token}:${Auth.REG_LOCK}`,
    deviceLinkList:           (userId)      => `${V}:${Auth.DOMAIN}:${Auth.DEVICE_LIST}:${userId}`,
    loginFailed:              (identifier)  => `${V}:${Auth.DOMAIN}:${Auth.LOGIN}:${identifier}:${Auth.FAILED}`,
    loginLock:                (identifier)  => `${V}:${Auth.DOMAIN}:${Auth.LOGIN}:${identifier}:${Auth.LOCK}`,
    loginIpAccounts:          (ip)          => `${V}:${Auth.DOMAIN}:${Auth.LOGIN}:${Auth.IP}:${ip}:${Auth.ACCOUNTS}`,

    // Execution
    pendingCmd: (executionId) => `${V}:${Execution.DOMAIN}:${executionId}:${Execution.PENDING_CMD}`,
};

export const KVScanPattern = {
    scanWebSessions:                   () => `${V}:${SessionDomain.DOMAIN}:${SessionDomain.WEB}:*`,
    scanOperatorSessions:              () => `${V}:${SessionDomain.DOMAIN}:${SessionDomain.OPERATOR}:*`,
    allSessionBindOperatorss:           () => `${V}:${SessionDomain.DOMAIN}:${SessionDomain.OPERATOR}:*:${SessionDomain.BIND}`,
    allSessionWebBinds:                () => `${V}:${SessionDomain.DOMAIN}:${SessionDomain.WEB}:*:${SessionDomain.BIND}`,
    allAccountLocks:                   () => `${V}:${Auth.DOMAIN}:${Auth.LOGIN}:*:${Auth.LOCK}`,
    allFailedLogins:                   () => `${V}:${Auth.DOMAIN}:${Auth.LOGIN}:*:${Auth.FAILED}`,
    allIpAccounts:                     () => `${V}:${Auth.DOMAIN}:${Auth.LOGIN}:${Auth.IP}:*:${Auth.ACCOUNTS}`,
};
