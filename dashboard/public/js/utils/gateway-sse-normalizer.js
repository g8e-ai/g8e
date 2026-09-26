// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Parse Gateway SSE push envelopes for the legacy SPA.
 * Mirrors dashboard/g8e-adapter/src/sse/normalizer.ts (nested `event` field).
 */
export function normalizeGatewayEvent(rawData, lastEventId = '') {
    let outer;
    try {
        outer = JSON.parse(rawData);
    } catch {
        throw new Error('invalid outer JSON');
    }

    if (!outer || typeof outer !== 'object' || Array.isArray(outer)) {
        throw new Error('outer envelope is not an object');
    }

    let inner = outer;
    if (typeof outer.event === 'string') {
        try {
            inner = JSON.parse(outer.event);
        } catch {
            throw new Error('invalid nested event JSON');
        }
    }

    if (!inner || typeof inner !== 'object' || Array.isArray(inner)) {
        throw new Error('inner event is not an object');
    }

    let id = 0;
    if (typeof outer.id === 'number') {
        id = outer.id;
    } else if (typeof outer.id === 'string') {
        const parsed = parseInt(outer.id, 10);
        if (!Number.isNaN(parsed)) id = parsed;
    }
    if (id === 0 && typeof inner.id === 'number') {
        id = inner.id;
    }
    if (id === 0 && lastEventId) {
        const parsed = parseInt(lastEventId, 10);
        if (!Number.isNaN(parsed)) id = parsed;
    }

    const type = typeof inner.type === 'string' ? inner.type : 'message';
    const timestamp = typeof inner.timestamp === 'string' ? inner.timestamp : new Date().toISOString();
    const payload = inner.data !== undefined ? inner.data : inner;

    return { id, type, timestamp, payload, raw: rawData };
}
