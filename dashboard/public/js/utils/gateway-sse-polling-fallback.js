// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { ApiPaths } from '../constants/api-paths.js';
import { gatewayUrl } from './gateway-url.js';
import { normalizeGatewayEvent } from './gateway-sse-normalizer.js';

/**
 * Poll stored Gateway SSE events when the EventSource stream is unavailable.
 * Uses GET /api/v1/sse/events?since_id=<id>&limit=<n> with credentials.
 */
export class GatewaySsePollingFallback {
    constructor({ onEvent, onUnauthenticated, pollIntervalMs = 5000, limit = 100, fetchImpl = fetch }) {
        this.onEvent = onEvent;
        this.onUnauthenticated = onUnauthenticated;
        this.pollIntervalMs = pollIntervalMs;
        this.limit = limit;
        this.fetchImpl = fetchImpl;
        this.timer = null;
        this.running = false;
        this.lastEventId = 0;
    }

    start() {
        if (this.running) return;
        this.running = true;
        this.poll();
    }

    stop() {
        this.running = false;
        if (this.timer) {
            clearTimeout(this.timer);
            this.timer = null;
        }
    }

    setLastEventId(id) {
        if (typeof id === 'number' && id > this.lastEventId) {
            this.lastEventId = id;
        }
    }

    getLastEventId() {
        return this.lastEventId;
    }

    async poll() {
        if (!this.running) return;

        const params = new URLSearchParams();
        if (this.lastEventId) params.set('since_id', String(this.lastEventId));
        params.set('limit', String(this.limit));
        const url = gatewayUrl(`${ApiPaths.sse.events()}?${params.toString()}`);

        try {
            const res = await this.fetchImpl(url, {
                method: 'GET',
                credentials: 'include',
                headers: { Accept: 'application/json' },
            });

            if (res.status === 401) {
                this.stop();
                this.onUnauthenticated?.();
                return;
            }

            if (res.ok) {
                const text = await res.text();
                if (text) this.parseEventsResponse(text);
            }
        } catch {
            // Network errors are non-fatal in polling mode.
        }

        this.scheduleNext();
    }

    parseEventsResponse(text) {
        let items;
        try {
            const parsed = JSON.parse(text);
            if (Array.isArray(parsed)) {
                items = parsed;
            } else if (parsed && typeof parsed === 'object' && Array.isArray(parsed.events)) {
                items = parsed.events;
            } else {
                items = [parsed];
            }
        } catch {
            return;
        }

        for (const item of items) {
            if (typeof item !== 'string') continue;
            try {
                const event = normalizeGatewayEvent(item, '');
                if (event.id > this.lastEventId) {
                    this.lastEventId = event.id;
                }
                this.onEvent?.(event);
            } catch {
                // Invalid events are silently dropped.
            }
        }
    }

    scheduleNext() {
        if (!this.running) return;
        this.timer = setTimeout(() => {
            this.timer = null;
            this.poll();
        }, this.pollIntervalMs);
    }
}
