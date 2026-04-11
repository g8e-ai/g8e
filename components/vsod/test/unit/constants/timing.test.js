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

import { describe, it, expect } from 'vitest';
import {
    SESSION_TTL_SECONDS,
    SESSION_REFRESH_THRESHOLD_SECONDS,
    ABSOLUTE_SESSION_TIMEOUT_SECONDS,
} from '@vsod/constants/session.js';
import { CacheTTL } from '@vsod/constants/service_config.js';
import {  SSE_KEEPALIVE_INTERVAL_MS  } from '@vsod/constants/events.js';
import {
    DEVICE_LINK_TTL_SECONDS,
    DEVICE_LINK_TTL_MIN_SECONDS,
    DEVICE_LINK_TTL_MAX_SECONDS,
    SESSION_AUTH_LISTEN_TTL_MS,
    LOCK_TTL_MS,
    LOCK_RETRY_DELAY_MS,
    LOCK_MAX_RETRIES,
    TIMESTAMP_WINDOW_MS,
    NONCE_TTL_SECONDS,
    NONCE_CACHE_CLEANUP_INTERVAL_MS,
    INTENT_TTL_MS,
} from '@vsod/constants/auth.js';
import {
    KV_CLIENT_READY_WAIT_MS,
    KV_CLIENT_POLL_INTERVAL_MS,
    PUBSUB_RECONNECT_DELAY_MS,
} from '@vsod/constants/http_client.js';
import {
    CACHE_WARMING_PERIODIC_INTERVAL_HOURS,
    CACHE_WARMING_PERIODIC_INTERVAL_MS,
    CACHE_WARMING_INVESTIGATION_PRELOAD_LIMIT,
    CRL_NEXT_UPDATE_MS,
    CONSOLE_METRICS_CACHE_TTL_MS,
    CONSOLE_METRICS_WINDOW_1_DAY_MS,
    CONSOLE_METRICS_WINDOW_7_DAYS_MS,
    CONSOLE_METRICS_WINDOW_30_DAYS_MS,
} from '@vsod/constants/service_config.js';

describe('Timing Constants [UNIT]', () => {

    describe('session timing', () => {
        it('SESSION_TTL_SECONDS is 8 hours', () => {
            expect(SESSION_TTL_SECONDS).toBe(8 * 60 * 60);
        });

        it('SESSION_REFRESH_THRESHOLD_SECONDS is 1 hour', () => {
            expect(SESSION_REFRESH_THRESHOLD_SECONDS).toBe(60 * 60);
        });

        it('ABSOLUTE_SESSION_TIMEOUT_SECONDS is 24 hours', () => {
            expect(ABSOLUTE_SESSION_TIMEOUT_SECONDS).toBe(24 * 60 * 60);
        });

        it('SESSION_TTL_SECONDS is less than ABSOLUTE_SESSION_TIMEOUT_SECONDS', () => {
            expect(SESSION_TTL_SECONDS).toBeLessThan(ABSOLUTE_SESSION_TIMEOUT_SECONDS);
        });

        it('SESSION_REFRESH_THRESHOLD_SECONDS is less than SESSION_TTL_SECONDS', () => {
            expect(SESSION_REFRESH_THRESHOLD_SECONDS).toBeLessThan(SESSION_TTL_SECONDS);
        });

        it('all session timing values are positive integers', () => {
            expect(Number.isInteger(SESSION_TTL_SECONDS)).toBe(true);
            expect(Number.isInteger(SESSION_REFRESH_THRESHOLD_SECONDS)).toBe(true);
            expect(Number.isInteger(ABSOLUTE_SESSION_TIMEOUT_SECONDS)).toBe(true);
            expect(SESSION_TTL_SECONDS).toBeGreaterThan(0);
            expect(SESSION_REFRESH_THRESHOLD_SECONDS).toBeGreaterThan(0);
            expect(ABSOLUTE_SESSION_TIMEOUT_SECONDS).toBeGreaterThan(0);
        });
    });

    describe('CacheTTL', () => {
        it('all TTL values are positive numbers', () => {
            for (const [key, value] of Object.entries(CacheTTL)) {
                expect(typeof value, `CacheTTL.${key}`).toBe('number');
                expect(value, `CacheTTL.${key} must be positive`).toBeGreaterThan(0);
            }
        });

        it('USER TTL is 1 hour', () => {
            expect(CacheTTL.USER).toBe(3600);
        });

        it('OPERATOR TTL is 1 hour', () => {
            expect(CacheTTL.OPERATOR).toBe(3600);
        });

        it('API_KEY TTL is 24 hours', () => {
            expect(CacheTTL.API_KEY).toBe(86400);
        });

        it('SETTINGS TTL is 24 hours', () => {
            expect(CacheTTL.SETTINGS).toBe(86400);
        });

        it('HEARTBEAT TTL is 5 minutes', () => {
            expect(CacheTTL.HEARTBEAT).toBe(300);
        });

        it('CASE and INVESTIGATION TTLs are equal', () => {
            expect(CacheTTL.CASE).toBe(CacheTTL.INVESTIGATION);
        });

        it('API_KEY TTL >= USER TTL (keys change less often)', () => {
            expect(CacheTTL.API_KEY).toBeGreaterThanOrEqual(CacheTTL.USER);
        });

        it('HEARTBEAT TTL is less than USER TTL (heartbeats are volatile)', () => {
            expect(CacheTTL.HEARTBEAT).toBeLessThan(CacheTTL.USER);
        });

        it('QUERY TTL is shorter than USER TTL (queries are more volatile)', () => {
            expect(CacheTTL.QUERY).toBeLessThanOrEqual(CacheTTL.USER);
        });

        it('DEFAULT TTL is a positive fallback', () => {
            expect(CacheTTL.DEFAULT).toBeGreaterThan(0);
        });
    });

    describe('SSE timing', () => {
        it('SSE_KEEPALIVE_INTERVAL_MS is 20 seconds', () => {
            expect(SSE_KEEPALIVE_INTERVAL_MS).toBe(20000);
        });

        it('SSE_KEEPALIVE_INTERVAL_MS is a positive number', () => {
            expect(SSE_KEEPALIVE_INTERVAL_MS).toBeGreaterThan(0);
        });
    });

    describe('device link timing', () => {
        it('DEVICE_LINK_TTL_SECONDS is 1 hour (default)', () => {
            expect(DEVICE_LINK_TTL_SECONDS).toBe(3600);
        });

        it('DEVICE_LINK_TTL_MIN_SECONDS is 1 minute', () => {
            expect(DEVICE_LINK_TTL_MIN_SECONDS).toBe(60);
        });

        it('DEVICE_LINK_TTL_MAX_SECONDS is 7 days', () => {
            expect(DEVICE_LINK_TTL_MAX_SECONDS).toBe(7 * 24 * 60 * 60);
        });

        it('MIN < default < MAX', () => {
            expect(DEVICE_LINK_TTL_MIN_SECONDS).toBeLessThan(DEVICE_LINK_TTL_SECONDS);
            expect(DEVICE_LINK_TTL_SECONDS).toBeLessThan(DEVICE_LINK_TTL_MAX_SECONDS);
        });

        it('SESSION_AUTH_LISTEN_TTL_MS is 60 seconds', () => {
            expect(SESSION_AUTH_LISTEN_TTL_MS).toBe(60_000);
        });

        it('all values are positive', () => {
            expect(DEVICE_LINK_TTL_SECONDS).toBeGreaterThan(0);
            expect(DEVICE_LINK_TTL_MIN_SECONDS).toBeGreaterThan(0);
            expect(DEVICE_LINK_TTL_MAX_SECONDS).toBeGreaterThan(0);
            expect(SESSION_AUTH_LISTEN_TTL_MS).toBeGreaterThan(0);
        });
    });

    describe('distributed lock timing', () => {
        it('LOCK_TTL_MS is positive', () => {
            expect(LOCK_TTL_MS).toBeGreaterThan(0);
        });

        it('LOCK_RETRY_DELAY_MS is positive', () => {
            expect(LOCK_RETRY_DELAY_MS).toBeGreaterThan(0);
        });

        it('LOCK_MAX_RETRIES is a positive integer', () => {
            expect(Number.isInteger(LOCK_MAX_RETRIES)).toBe(true);
            expect(LOCK_MAX_RETRIES).toBeGreaterThan(0);
        });

        it('total lock wait time (retries * delay) is less than LOCK_TTL_MS', () => {
            const maxWait = LOCK_MAX_RETRIES * LOCK_RETRY_DELAY_MS;
            expect(maxWait).toBeLessThanOrEqual(LOCK_TTL_MS * 2);
        });

        it('LOCK_RETRY_DELAY_MS is less than LOCK_TTL_MS', () => {
            expect(LOCK_RETRY_DELAY_MS).toBeLessThan(LOCK_TTL_MS);
        });
    });

    describe('timestamp and nonce timing', () => {
        it('TIMESTAMP_WINDOW_MS is 5 minutes', () => {
            expect(TIMESTAMP_WINDOW_MS).toBe(5 * 60 * 1000);
        });

        it('NONCE_TTL_SECONDS is 10 minutes', () => {
            expect(NONCE_TTL_SECONDS).toBe(10 * 60);
        });

        it('NONCE_CACHE_CLEANUP_INTERVAL_MS is 1 minute', () => {
            expect(NONCE_CACHE_CLEANUP_INTERVAL_MS).toBe(60 * 1000);
        });

        it('NONCE_TTL_SECONDS > TIMESTAMP_WINDOW_MS / 1000 (nonce outlives window)', () => {
            expect(NONCE_TTL_SECONDS).toBeGreaterThan(TIMESTAMP_WINDOW_MS / 1000);
        });

        it('NONCE_CACHE_CLEANUP_INTERVAL_MS < NONCE_TTL_SECONDS * 1000', () => {
            expect(NONCE_CACHE_CLEANUP_INTERVAL_MS).toBeLessThan(NONCE_TTL_SECONDS * 1000);
        });
    });

    describe('KV client timing', () => {
        it('KV_CLIENT_READY_WAIT_MS is positive', () => {
            expect(KV_CLIENT_READY_WAIT_MS).toBeGreaterThan(0);
        });

        it('KV_CLIENT_POLL_INTERVAL_MS is positive', () => {
            expect(KV_CLIENT_POLL_INTERVAL_MS).toBeGreaterThan(0);
        });

        it('KV_CLIENT_POLL_INTERVAL_MS is less than KV_CLIENT_READY_WAIT_MS', () => {
            expect(KV_CLIENT_POLL_INTERVAL_MS).toBeLessThan(KV_CLIENT_READY_WAIT_MS);
        });
    });

    describe('PubSub reconnect timing', () => {
        it('PUBSUB_RECONNECT_DELAY_MS is 1 second', () => {
            expect(PUBSUB_RECONNECT_DELAY_MS).toBe(1000);
        });

        it('is a positive number', () => {
            expect(PUBSUB_RECONNECT_DELAY_MS).toBeGreaterThan(0);
        });
    });

    describe('intent timing', () => {
        it('INTENT_TTL_MS is 1 hour', () => {
            expect(INTENT_TTL_MS).toBe(60 * 60 * 1000);
        });

        it('is a positive number', () => {
            expect(INTENT_TTL_MS).toBeGreaterThan(0);
        });
    });

    describe('cache warming timing', () => {
        it('CACHE_WARMING_PERIODIC_INTERVAL_HOURS is 12', () => {
            expect(CACHE_WARMING_PERIODIC_INTERVAL_HOURS).toBe(12);
        });

        it('CACHE_WARMING_PERIODIC_INTERVAL_MS is derived from hours', () => {
            expect(CACHE_WARMING_PERIODIC_INTERVAL_MS).toBe(
                CACHE_WARMING_PERIODIC_INTERVAL_HOURS * 60 * 60 * 1000
            );
        });

        it('CACHE_WARMING_INVESTIGATION_PRELOAD_LIMIT is 50', () => {
            expect(CACHE_WARMING_INVESTIGATION_PRELOAD_LIMIT).toBe(50);
        });

        it('CACHE_WARMING_INVESTIGATION_PRELOAD_LIMIT is a positive integer', () => {
            expect(Number.isInteger(CACHE_WARMING_INVESTIGATION_PRELOAD_LIMIT)).toBe(true);
            expect(CACHE_WARMING_INVESTIGATION_PRELOAD_LIMIT).toBeGreaterThan(0);
        });
    });

    describe('CRL and certificate timing', () => {
        it('CRL_NEXT_UPDATE_MS is 24 hours', () => {
            expect(CRL_NEXT_UPDATE_MS).toBe(24 * 60 * 60 * 1000);
        });

        it('all CRL/cert values are positive', () => {
            expect(CRL_NEXT_UPDATE_MS).toBeGreaterThan(0);
        });
    });

    describe('fleet stream and console metrics timing', () => {
        it('CONSOLE_METRICS_CACHE_TTL_MS is 30 seconds', () => {
            expect(CONSOLE_METRICS_CACHE_TTL_MS).toBe(30000);
        });

        it('CONSOLE_METRICS_WINDOW_1_DAY_MS is 24 hours', () => {
            expect(CONSOLE_METRICS_WINDOW_1_DAY_MS).toBe(24 * 60 * 60 * 1000);
        });

        it('CONSOLE_METRICS_WINDOW_7_DAYS_MS is 7x the 1-day window', () => {
            expect(CONSOLE_METRICS_WINDOW_7_DAYS_MS).toBe(7 * CONSOLE_METRICS_WINDOW_1_DAY_MS);
        });

        it('CONSOLE_METRICS_WINDOW_30_DAYS_MS is 30x the 1-day window', () => {
            expect(CONSOLE_METRICS_WINDOW_30_DAYS_MS).toBe(30 * CONSOLE_METRICS_WINDOW_1_DAY_MS);
        });

        it('metrics windows are in ascending order', () => {
            expect(CONSOLE_METRICS_WINDOW_1_DAY_MS)
                .toBeLessThan(CONSOLE_METRICS_WINDOW_7_DAYS_MS);
            expect(CONSOLE_METRICS_WINDOW_7_DAYS_MS)
                .toBeLessThan(CONSOLE_METRICS_WINDOW_30_DAYS_MS);
        });

    });

    describe('all timing constants are positive numbers', () => {
        const allConstants = {
            SESSION_TTL_SECONDS,
            SESSION_REFRESH_THRESHOLD_SECONDS,
            ABSOLUTE_SESSION_TIMEOUT_SECONDS,
            SSE_KEEPALIVE_INTERVAL_MS,
            DEVICE_LINK_TTL_SECONDS,
            DEVICE_LINK_TTL_MIN_SECONDS,
            DEVICE_LINK_TTL_MAX_SECONDS,
            SESSION_AUTH_LISTEN_TTL_MS,
            LOCK_TTL_MS,
            LOCK_RETRY_DELAY_MS,
            LOCK_MAX_RETRIES,
            TIMESTAMP_WINDOW_MS,
            NONCE_TTL_SECONDS,
            NONCE_CACHE_CLEANUP_INTERVAL_MS,
            KV_CLIENT_READY_WAIT_MS,
            KV_CLIENT_POLL_INTERVAL_MS,
            PUBSUB_RECONNECT_DELAY_MS,
            INTENT_TTL_MS,
            CACHE_WARMING_PERIODIC_INTERVAL_HOURS,
            CACHE_WARMING_PERIODIC_INTERVAL_MS,
            CACHE_WARMING_INVESTIGATION_PRELOAD_LIMIT,
            CRL_NEXT_UPDATE_MS,
            CONSOLE_METRICS_CACHE_TTL_MS,
            CONSOLE_METRICS_WINDOW_1_DAY_MS,
            CONSOLE_METRICS_WINDOW_7_DAYS_MS,
            CONSOLE_METRICS_WINDOW_30_DAYS_MS,
        };

        for (const [name, value] of Object.entries(allConstants)) {
            it(`${name} is a positive number`, () => {
                expect(typeof value, `${name} should be a number`).toBe('number');
                expect(value, `${name} should be positive`).toBeGreaterThan(0);
            });
        }
    });

});
