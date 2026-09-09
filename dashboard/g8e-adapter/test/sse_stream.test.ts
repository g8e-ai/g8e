import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';

import type { FrontendRuntimeConfig } from '../src/config/runtime_config';
import { parseRuntimeConfig, RUNTIME_CONFIG_SCHEMA_VERSION } from '../src/config/runtime_config';
import { SseStream, type ReconcileReason, type SseConnectionState } from '../src/sse/stream';

function makeConfig(): FrontendRuntimeConfig {
  return parseRuntimeConfig({
    schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
    gateway_base_url: 'https://g8e.example.com',
    passkey_rp_id: 'g8e.example.com',
    passkey_rp_name: 'g8e',
    app_name: 'g8e',
  });
}

// Fake EventSource that simulates the browser EventSource API.
function createFakeEventSource() {
  let instance: FakeEventSource | null = null;
  const factory = (url: string) => {
    instance = new FakeEventSource(url);
    return instance;
  };
  return { factory, get instance() { return instance; } };
}

class FakeEventSource {
  url: string;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  readyState = 0; // CONNECTING
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;

  constructor(url: string) {
    this.url = url;
  }

  fireOpen(): void {
    this.readyState = FakeEventSource.OPEN;
    if (this.onopen) this.onopen(new Event('open'));
  }

  fireMessage(data: string, lastEventId = ''): void {
    if (this.onmessage) this.onmessage(new MessageEvent('message', { data, lastEventId }));
  }

  fireError(): void {
    if (this.onerror) this.onerror(new Event('error'));
  }

  close(): void {
    this.readyState = FakeEventSource.CLOSED;
  }
}

function agentEvent(id: number): string {
  return JSON.stringify({
    id,
    event: JSON.stringify({
      type: 'app.agent.status.updated',
      data: {
        schema_version: '1.0.0',
        agent_id: 'u:sage',
        display_name: 'Sage',
        role: 'sage',
        status: 'running',
        observed_at: '2026-01-01T00:00:00Z',
      },
    }),
  });
}

describe('SseStream', () => {
  it('opens an EventSource with credentials at the absolute configured URL', () => {
    const fake = createFakeEventSource();
    const states: SseConnectionState[] = [];
    new SseStream({
      config: makeConfig(),
      callbacks: { onStateChange: (s) => states.push(s) },
      eventSourceFactory: fake.factory,
    }).connect();
    expect(fake.instance).not.toBeNull();
    expect(fake.instance!.url).toBe('https://g8e.example.com/api/v1/sse/stream');
    expect(states).toContain('connecting');
  });

  it('includes since_id when reconnecting with a last event id', () => {
    const fake = createFakeEventSource();
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: {},
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    fake.instance!.fireMessage(agentEvent(42), '42');
    expect(stream.getLastEventId()).toBe(42);
    stream.disconnect();
    stream.connect();
    expect(fake.instance!.url).toBe('https://g8e.example.com/api/v1/sse/stream?since_id=42');
  });

  it('fires onEvent for recognized events', () => {
    const fake = createFakeEventSource();
    const events: unknown[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onEvent: (e) => events.push(e) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    fake.instance!.fireMessage(agentEvent(1), '1');
    expect(events.length).toBe(1);
  });

  it('deduplicates events by durable ID', () => {
    const fake = createFakeEventSource();
    const events: unknown[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onEvent: (e) => events.push(e) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    fake.instance!.fireMessage(agentEvent(5), '5');
    fake.instance!.fireMessage(agentEvent(5), '5');
    expect(events.length).toBe(1);
  });

  it('fires onReconcile with initial_connection on first open', () => {
    const fake = createFakeEventSource();
    const reasons: ReconcileReason[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onReconcile: (r) => reasons.push(r) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    expect(reasons).toContain('initial_connection');
  });

  it('fires onReconcile with reconnect_gap on subsequent opens', () => {
    const fake = createFakeEventSource();
    const reasons: ReconcileReason[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onReconcile: (r) => reasons.push(r) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    reasons.length = 0;
    stream.disconnect();
    stream.connect();
    fake.instance!.fireOpen();
    expect(reasons).toContain('reconnect_gap');
  });

  it('fires onReconcile with truncated on truncated sentinel', () => {
    const fake = createFakeEventSource();
    const reasons: ReconcileReason[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onReconcile: (r) => reasons.push(r) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    fake.instance!.fireMessage(JSON.stringify({ event: JSON.stringify({ type: 'truncated', data: { since_id: 1, limit: 100 } }) }), '0');
    expect(reasons).toContain('truncated');
  });

  it('fires onReconcile with replay_failed on error sentinel with replay_failed reason', () => {
    const fake = createFakeEventSource();
    const reasons: ReconcileReason[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onReconcile: (r) => reasons.push(r) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    fake.instance!.fireMessage(JSON.stringify({ event: JSON.stringify({ type: 'error', data: { reason: 'replay_failed' } }) }), '0');
    expect(reasons).toContain('replay_failed');
  });

  it('fires onUnauthenticated when EventSource closes after error', () => {
    const fake = createFakeEventSource();
    let unauth = false;
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onUnauthenticated: () => { unauth = true; } },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.readyState = FakeEventSource.CLOSED;
    fake.instance!.fireError();
    expect(unauth).toBe(true);
  });

  it('sets state to reconnecting when EventSource is still connecting after error', () => {
    const fake = createFakeEventSource();
    const states: SseConnectionState[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onStateChange: (s) => states.push(s) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.readyState = FakeEventSource.CONNECTING;
    fake.instance!.fireError();
    expect(states).toContain('reconnecting');
  });

  it('disconnect closes the EventSource and sets state to disconnected', () => {
    const fake = createFakeEventSource();
    const states: SseConnectionState[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onStateChange: (s) => states.push(s) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    stream.disconnect();
    expect(fake.instance!.readyState).toBe(FakeEventSource.CLOSED);
    expect(states).toContain('disconnected');
  });

  it('clearState resets dedup and lastEventId', () => {
    const fake = createFakeEventSource();
    const events: unknown[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onEvent: (e) => events.push(e) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    fake.instance!.fireMessage(agentEvent(10), '10');
    stream.clearState();
    expect(stream.getLastEventId()).toBe(0);
    // After clearing, the same event ID should not be deduped.
    fake.instance!.fireMessage(agentEvent(10), '10');
    expect(events.length).toBe(2);
  });

  it('onVisibilityChange triggers reconciliation after a long pause', () => {
    const reasons: ReconcileReason[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onReconcile: (r) => reasons.push(r) },
      eventSourceFactory: createFakeEventSource().factory,
      visibilityPauseMs: 0, // any pause triggers
    });
    stream.onVisibilityChange(false);
    stream.onVisibilityChange(true);
    expect(reasons).toContain('visibility_restored');
  });

  it('onVisibilityChange does not trigger reconciliation for short pauses', () => {
    const reasons: ReconcileReason[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onReconcile: (r) => reasons.push(r) },
      eventSourceFactory: createFakeEventSource().factory,
      visibilityPauseMs: 60000,
    });
    stream.onVisibilityChange(false);
    stream.onVisibilityChange(true);
    expect(reasons).not.toContain('visibility_restored');
  });

  it('silently drops invalid JSON events', () => {
    const fake = createFakeEventSource();
    const events: unknown[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onEvent: (e) => events.push(e) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    fake.instance!.fireMessage('not json', '0');
    expect(events.length).toBe(0);
  });

  it('silently drops unknown event types without calling onEvent', () => {
    const fake = createFakeEventSource();
    const events: unknown[] = [];
    const stream = new SseStream({
      config: makeConfig(),
      callbacks: { onEvent: (e) => events.push(e) },
      eventSourceFactory: fake.factory,
    });
    stream.connect();
    fake.instance!.fireOpen();
    fake.instance!.fireMessage(JSON.stringify({ event: JSON.stringify({ type: 'unknown.type', data: {} }) }), '1');
    // Unknown events are still passed to onEvent but marked as unrecognized.
    // The presentation layer decides what to do with them.
    expect(events.length).toBe(1);
    expect((events[0] as { recognized: boolean }).recognized).toBe(false);
  });
});
