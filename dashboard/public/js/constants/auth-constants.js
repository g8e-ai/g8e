// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export const UserRole = Object.freeze({
    USER:       'user',
    ADMIN:      'admin',
    SUPERADMIN: 'superadmin',
});

export const OperatorSessionRole = Object.freeze({
    OPERATOR: 'operator',
});

export const AuthProvider = Object.freeze({
    LOCAL: 'local',
});

export const DeviceLinkStatus = Object.freeze({
    ACTIVE:    'active',
    EXHAUSTED: 'exhausted',
    EXPIRED:   'expired',
    REVOKED:   'revoked',
});

export const IntentStatus = Object.freeze({
    GRANTED: 'granted',
    FAILED:  'failed',
});
