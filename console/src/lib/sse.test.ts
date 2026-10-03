// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { GatewayStream, backoffDelay, normalizeEvent, type ConnectionState, type StreamEvent } from './sse';

describe('normalizeEvent', () => {
  it('parses a push envelope whose event is a JSON string', () => {
    const raw = JSON.stringify({
      user_id: 'u',
      web_session_id: 'w',
      event: JSON.stringify({ type: 'g8e.v1.app.case.created', data: { case_id: 'c1', timestamp: '2026-01-01T00:00:00Z' } }),
    });
    const ev = normalizeEvent(raw, '42');
    expect(ev).toEqual({ id: 42, type: 'g8e.v1.app.case.created', timestamp: '2026-01-01T00:00:00Z', data: { case_id: 'c1', timestamp: '2026-01-01T00:00:00Z' } });
  });

  it('parses a push envelope whose event is an object', () => {
    const raw = JSON.stringify({ user_id: 'u', web_session_id: 'w', event: { type: 't', data: { a: 1 } } });
    const ev = normalizeEvent(raw, '');
    expect(ev?.type).toBe('t');
    expect(ev?.data).toEqual({ a: 1 });
    expect(ev?.id).toBe(0);
  });

  it('rejects malformed or untyped payloads', () => {
    expect(normalizeEvent('not json')).toBeNull();
    expect(normalizeEvent('[1,2]')).toBeNull();
    expect(normalizeEvent(JSON.stringify({ event: '{bad' }))).toBeNull();
    expect(normalizeEvent(JSON.stringify({ event: { data: {} } }))).toBeNull();
  });
});

describe('backoffDelay', () => {
  it('doubles from one second and caps at thirty seconds plus jitter', () => {
    expect(backoffDelay(0, 0)).toBe(1000);
    expect(backoffDelay(3, 0)).toBe(8000);
    expect(backoffDelay(10, 0)).toBe(30000);
    expect(backoffDelay(10, 1)).toBe(30500);
  });
});

class FakeSource {
  static instances: FakeSource[] = [];
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((m: MessageEvent<string>) => void) | null = null;
  closed = false;
  constructor(public url: string) {
    FakeSource.instances.push(this);
  }
  close() {
    this.closed = true;
  }
  emit(id: number, type: string, data: Record<string, unknown> = {}) {
    this.onmessage?.({ data: JSON.stringify({ event: { type, data } }), lastEventId: String(id) } as MessageEvent<string>);
  }
}

describe('GatewayStream', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeSource.instances = [];
  });
  afterEach(() => vi.useRealTimers());

  function start() {
    const events: StreamEvent[] = [];
    const states: ConnectionState[] = [];
    const stream = new GatewayStream({
      onEvent: (e) => events.push(e),
      onState: (s) => states.push(s),
      createSource: (url) => new FakeSource(url) as unknown as EventSource,
    });
    stream.start();
    return { stream, events, states };
  }

  it('connects live-only, de-duplicates, and resumes from the last durable ID', () => {
    const { events, states } = start();
    const first = FakeSource.instances[0]!;
    expect(first.url).toBe('/api/v1/sse/stream?since_id=0');
    first.onopen?.();
    expect(states).toEqual(['connecting', 'open']);

    first.emit(5, 'a');
    first.emit(5, 'a');
    first.emit(7, 'b');
    expect(events.map((e) => e.id)).toEqual([5, 7]);

    first.onerror?.();
    expect(first.closed).toBe(true);
    vi.advanceTimersByTime(1500);
    expect(FakeSource.instances[1]!.url).toBe('/api/v1/sse/stream?since_id=7');
  });

  it('treats a source that never opens as failed and retries', () => {
    start();
    vi.advanceTimersByTime(10_000);
    expect(FakeSource.instances[0]!.closed).toBe(true);
    vi.advanceTimersByTime(1500);
    expect(FakeSource.instances).toHaveLength(2);
  });

  it('stops reconnecting after stop()', () => {
    const { stream } = start();
    stream.stop();
    FakeSource.instances[0]!.onerror?.();
    vi.advanceTimersByTime(60_000);
    expect(FakeSource.instances).toHaveLength(1);
  });
});
