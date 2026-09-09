// Polling fallback for SSE. When EventSource is unavailable or fails
// repeatedly, this module polls the stored-events HTTP endpoint
// `GET /api/v1/sse/events?since_id=<id>&limit=<n>` to fetch retained events.
// This is NOT the SSE stream endpoint (`/api/v1/sse/stream`) — SSE is
// server-push via EventSource. The events endpoint returns stored push
// envelopes as JSON for polling clients.
//
// Per the original plan (line 345 of v2.1.8_prompt_built_observability_frontend_plan.md):
// "Polling fallback uses `GET /api/v1/sse/events?since_id=<id>&limit=<n>`."

import type { FrontendRuntimeConfig } from '../config/runtime_config';
import type { NormalizedGatewayEvent } from './normalizer';
import { normalizeGatewayEvent } from './normalizer';

export interface SsePollingOptions {
  config: FrontendRuntimeConfig;
  onEvent(event: NormalizedGatewayEvent): void;
  onUnauthenticated?(): void;
  fetchImpl?: typeof fetch;
  pollIntervalMs?: number;
  limit?: number;
}

export class SsePollingFallback {
  private timer: ReturnType<typeof setTimeout> | null = null;
  private running = false;
  private lastEventId = 0;
  private readonly fetchImpl: typeof fetch;
  private readonly pollInterval: number;
  private readonly limit: number;
  private readonly baseUrl: string;

  constructor(private readonly opts: SsePollingOptions) {
    this.fetchImpl = opts.fetchImpl ?? fetch;
    this.pollInterval = opts.pollIntervalMs ?? 5000;
    this.limit = opts.limit ?? 100;
    this.baseUrl = opts.config.gateway_base_url.replace(/\/$/, '');
  }

  start(): Promise<void> {
    if (this.running) return Promise.resolve();
    this.running = true;
    return this.poll();
  }

  stop(): void {
    this.running = false;
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }

  setLastEventId(id: number): void {
    this.lastEventId = id;
  }

  getLastEventId(): number {
    return this.lastEventId;
  }

  private async poll(): Promise<void> {
    if (!this.running) return;

    const params = new URLSearchParams();
    if (this.lastEventId) params.set('since_id', String(this.lastEventId));
    params.set('limit', String(this.limit));
    const url = `${this.baseUrl}/api/v1/sse/events?${params.toString()}`;

    try {
      const res = await this.fetchImpl(url, {
        method: 'GET',
        credentials: 'include',
        headers: { Accept: 'application/json' },
      });

      if (res.status === 401) {
        this.running = false;
        this.opts.onUnauthenticated?.();
        return;
      }

      if (!res.ok) {
        this.scheduleNext();
        return;
      }

      const text = await res.text();
      if (text) {
        this.parseEventsResponse(text);
      }
    } catch {
      // Network errors are non-fatal in polling mode; retry on next interval.
    }

    this.scheduleNext();
  }

  // Parse the JSON response from /api/v1/sse/events. The response is a JSON
  // array of stored push envelopes, each with the same nested structure as
  // the SSE stream's data field (outer envelope with id and event fields).
  parseEventsResponse(text: string): void {
    let items: unknown[];
    try {
      const parsed = JSON.parse(text);
      if (Array.isArray(parsed)) {
        items = parsed;
      } else if (parsed && typeof parsed === 'object' && Array.isArray((parsed as Record<string, unknown>).events)) {
        items = (parsed as Record<string, unknown[]>).events;
      } else {
        items = [parsed];
      }
    } catch {
      return;
    }

    for (const item of items) {
      if (typeof item !== 'string') {
        continue;
      }
      try {
        const event = normalizeGatewayEvent(item, '');
        if (event.id > this.lastEventId) {
          this.lastEventId = event.id;
        }
        this.opts.onEvent(event);
      } catch {
        // Invalid events are silently dropped.
      }
    }
  }

  private scheduleNext(): void {
    if (!this.running) return;
    this.timer = setTimeout(() => {
      this.timer = null;
      this.poll();
    }, this.pollInterval);
  }
}
