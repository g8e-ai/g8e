// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export const OperatorStatus = Object.freeze({
    AVAILABLE:   'available',
    UNAVAILABLE: 'unavailable',
    OFFLINE:     'offline',
    BOUND:       'bound',
    STALE:       'stale',
    ACTIVE:      'active',
    STOPPED:     'stopped',
    TERMINATED:  'terminated',
});

export const OperatorType = Object.freeze({
    OPERATOR: 'system',
    CLOUD:    'cloud',
});
