// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { _EVENTS } from './shared.js';

/**
 * Wire Event Type Constants
 * Canonical values loaded from protocol/constants/events.json.
 * That file is the single source of truth shared across VSE, VSA, and g8ed.
 *
 * Mirrors: components/vse/app/constants/events.py EventType (flat naming)
 */

/**
 * Build a flat UPPER_SNAKE_CASE constant map from the nested _EVENTS object.
 *
 * Naming conventions:
 * - app.*           → drop "app", uppercase rest (e.g. app.case.created → CASE_CREATED)
 * - ai.*            → drop "ai", uppercase rest (e.g. ai.llm.config.requested → LLM_CONFIG_REQUESTED)
 * - platform.auth.* → drop "platform", uppercase rest (e.g. platform.auth.login.requested → AUTH_LOGIN_REQUESTED)
 * - platform.*      → keep all, uppercase (e.g. platform.sse.keepalive.sent → PLATFORM_SSE_KEEPALIVE_SENT)
 * - source.*        → drop "source", prefix EVENT_SOURCE_, uppercase rest (e.g. source.user.chat → EVENT_SOURCE_USER_CHAT)
 * - everything else → keep all, uppercase (e.g. operator.heartbeat.sent → OPERATOR_HEARTBEAT_SENT)
 */
function buildEventType(events) {
    const result = {};

    function flatten(obj, parts) {
        for (const [key, val] of Object.entries(obj)) {
            const path = [...parts, key];
            if (typeof val === 'object' && val !== null && !Array.isArray(val)) {
                flatten(val, path);
            } else {
                const segments = path.slice();

                let prefix = '';
                if (segments[0] === 'app' || segments[0] === 'ai') {
                    segments.shift();
                } else if (segments[0] === 'platform' && segments[1] === 'auth') {
                    segments.shift();
                } else if (segments[0] === 'source') {
                    segments.shift();
                    prefix = 'EVENT_SOURCE_';
                }

                result[prefix + segments.join('_').toUpperCase()] = val;
            }
        }
    }

    flatten(events, []);
    return result;
}

export const EventType = Object.freeze(buildEventType(_EVENTS));

export const SSE_KEEPALIVE_INTERVAL_MS = 20_000;

export const ConnectionState = Object.freeze({
    DISCONNECTED: 'disconnected',
    CONNECTING:   'connecting',
    CONNECTED:    'connected',
    RECONNECTING: 'reconnecting',
    CLOSED:       'closed',
    ERROR:        'error',
});
