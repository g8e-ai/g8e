// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Gateway SSE consumer (docs/architecture/sse.md, docs/guides/build_frontend.md).
// The stream URL carries no routing identity — the Gateway derives user and web
// session from the cookie (INV-FE-SSE-01). Only the non-secret since_id cursor
// is sent on reconnect.

import { Paths } from './paths';

export interface StreamEvent {
  /** Durable Gateway event ID (0 when the producer supplied none). */
  id: number;
  type: string;
  timestamp: string;
  data: Record<string, unknown>;
}

function asObject(v: unknown): Record<string, unknown> | null {
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
}

/**
 * Parses one SSE `message` payload: the outer push envelope carries user_id,
 * one session ID, and `event`, which producers encode either as an object or
 * as a JSON string. Returns null for anything that is not a typed event.
 */
export function normalizeEvent(raw: string, lastEventId = ''): StreamEvent | null {
  let outer: Record<string, unknown> | null;
  try {
    outer = asObject(JSON.parse(raw));
  } catch {
    return null;
  }
  if (!outer) return null;

  let inner: Record<string, unknown> | null = outer;
  if (typeof outer.event === 'string') {
    try {
      inner = asObject(JSON.parse(outer.event));
    } catch {
      return null;
    }
  } else if (asObject(outer.event)) {
    inner = asObject(outer.event);
  }
  if (!inner || typeof inner.type !== 'string' || !inner.type) return null;

  let id = 0;
  const parsedLast = parseInt(lastEventId, 10);
  if (!Number.isNaN(parsedLast)) id = parsedLast;
  else if (typeof outer.id === 'number') id = outer.id;

  const data = asObject(inner.data) ?? {};
  const timestamp =
    typeof inner.timestamp === 'string' ? inner.timestamp : typeof data.timestamp === 'string' ? data.timestamp : new Date().toISOString();

  return { id, type: inner.type, timestamp, data };
}

export type ConnectionState = 'connecting' | 'open' | 'closed';

export interface StreamOptions {
  onEvent: (ev: StreamEvent) => void;
  onState: (state: ConnectionState) => void;
  /** Factory seam for tests. */
  createSource?: (url: string) => EventSource;
}

const OPEN_TIMEOUT_MS = 10_000;
const MAX_BACKOFF_MS = 30_000;
const SEEN_LIMIT = 1000;

export function backoffDelay(attempt: number, jitter = Math.random()): number {
  return Math.min(MAX_BACKOFF_MS, 1000 * 2 ** attempt) + jitter * 500;
}

/**
 * Owns one EventSource with exponential-backoff reconnect, an open-timeout
 * guard, resume from the last durable ID, and bounded de-duplication.
 */
export class GatewayStream {
  private source: EventSource | null = null;
  private attempt = 0;
  private lastId = 0;
  private seen = new Set<number>();
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private openTimer: ReturnType<typeof setTimeout> | null = null;
  private stopped = true;
  private readonly opts: StreamOptions;

  constructor(opts: StreamOptions) {
    this.opts = opts;
  }

  start(): void {
    this.stopped = false;
    this.connect();
  }

  stop(): void {
    this.stopped = true;
    this.clearTimers();
    this.source?.close();
    this.source = null;
    this.opts.onState('closed');
  }

  /** Reconnects immediately if the stream is down (e.g. on tab refocus). */
  nudge(): void {
    if (!this.stopped && !this.source) {
      this.clearTimers();
      this.connect();
    }
  }

  private clearTimers(): void {
    if (this.retryTimer) clearTimeout(this.retryTimer);
    if (this.openTimer) clearTimeout(this.openTimer);
    this.retryTimer = null;
    this.openTimer = null;
  }

  private connect(): void {
    if (this.stopped || this.source) return;
    this.opts.onState('connecting');
    // Without since_id the Gateway replays stored history; the console loads
    // history over HTTP, so a first connect asks for live events only.
    const url = `${Paths.sseStream}?since_id=${this.lastId}`;
    const make = this.opts.createSource ?? ((u: string) => new EventSource(u, { withCredentials: true }));
    const es = make(url);
    this.source = es;

    this.openTimer = setTimeout(() => this.fail(), OPEN_TIMEOUT_MS);
    es.onopen = () => {
      if (this.openTimer) clearTimeout(this.openTimer);
      this.openTimer = null;
      this.attempt = 0;
      this.opts.onState('open');
    };
    es.onerror = () => this.fail();
    es.onmessage = (msg: MessageEvent<string>) => this.handle(msg.data, msg.lastEventId);
  }

  private fail(): void {
    if (this.openTimer) clearTimeout(this.openTimer);
    this.openTimer = null;
    this.source?.close();
    this.source = null;
    if (this.stopped) return;
    this.opts.onState('closed');
    const delay = backoffDelay(this.attempt);
    this.attempt++;
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null;
      this.connect();
    }, delay);
  }

  private handle(raw: string, lastEventId: string): void {
    const ev = normalizeEvent(raw, lastEventId);
    if (!ev) return;
    if (ev.id) {
      if (this.seen.has(ev.id)) return;
      this.seen.add(ev.id);
      if (this.seen.size > SEEN_LIMIT) {
        const oldest = this.seen.values().next().value;
        if (oldest !== undefined) this.seen.delete(oldest);
      }
      if (ev.id > this.lastId) this.lastId = ev.id;
    }
    this.opts.onEvent(ev);
  }
}
