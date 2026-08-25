// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export const SSEClientConfig = Object.freeze({
    MAX_RECONNECT_ATTEMPTS:      10,
    BASE_RECONNECT_DELAY_MS:     1000,
    MAX_RECONNECT_DELAY_MS:      30000,
    MIN_RECONNECT_DELAY_MS:      1000,
    KEEPALIVE_TIMEOUT_MS:        120000,
    QUICK_FAILURE_THRESHOLD_MS:  5000,
    QUICK_FAILURE_BACKOFF_COUNT: 3,
    RECONNECT_FAILURE_REASON:    'max_attempts_exceeded',
});
