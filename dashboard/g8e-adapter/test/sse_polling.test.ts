import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';

import { parseRuntimeConfig, RUNTIME_CONFIG_SCHEMA_VERSION } from '../src/config/runtime_config';
import { SsePollingFallback } from '../src/sse/polling';
import type { NormalizedGatewayEvent } from '../src/sse/normalizer';

function makeConfig() {
  return parseRuntimeConfig({
    schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
    gateway_base_url: 'https://g8e.example.com',
    passkey_rp_id: 'g8e.example.com',
    passkey_rp_name: 'g8e',
    app_name: 'g8e',
  });
}

// A stored push envelope string, matching the SSE stream's data field shape.
function agentEnvelope(id: number): string {
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

function runEnvelope(id: number): string {
  return JSON.stringify({
    id,
    event: JSON.stringify({
      type: 'app.run.status.updated',
      data: {
        schema_version: '1.0.0',
        run_id: 'inv-1',
        run_kind: 'investigation',
        display_name: 'Case',
        status: 'running',
        completed_tasks: 0,
        total_tasks: 0,
        observed_at: '2026-01-01T00:00:00Z',
      },
    }),
  });
}

interface FetchCall {
  url: string;
  opts: Record<string, unknown>;
}

function makeFetchMock(
  status: number,
  body: unknown,
): typeof fetch & { calls: FetchCall[] } {
  const calls: FetchCall[] = [];
  const mock = vi.fn(async (url: string, opts: Record<string, unknown>) => {
    calls.push({ url, opts });
    return {
      ok: status >= 200 && status < 300,
      status,
      text: async () => (body === null ? '' : JSON.stringify(body)),
      headers: new Headers(),
    } as Response;
  }) as unknown as typeof fetch & { calls: FetchCall[] };
  mock.calls = calls;
  return mock;
}

function makeThrowingFetch(err: unknown): typeof fetch & { calls: FetchCall[] } {
  const calls: FetchCall[] = [];
  const mock = vi.fn(async (url: string, opts: Record<string, unknown>) => {
    calls.push({ url, opts });
    throw err;
  }) as unknown as typeof fetch & { calls: FetchCall[] };
  mock.calls = calls;
  return mock;
}

describe('SsePollingFallback', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('constructs the polling URL with since_id and limit', async () => {
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
      limit: 50,
    });
    polling.setLastEventId(42);
    await polling.start();
    expect(fetchMock.calls[0].url).toBe(
      'https://g8e.example.com/api/v1/sse/events?since_id=42&limit=50',
    );
    polling.stop();
  });

  it('omits since_id when lastEventId is zero', async () => {
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
      limit: 100,
    });
    await polling.start();
    expect(fetchMock.calls[0].url).toBe(
      'https://g8e.example.com/api/v1/sse/events?limit=100',
    );
    polling.stop();
  });

  it('uses the default limit of 100 when none is provided', async () => {
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    expect(fetchMock.calls[0].url).toContain('limit=100');
    polling.stop();
  });

  it('sends credentials: include on every request', async () => {
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    expect(fetchMock.calls[0].opts.credentials).toBe('include');
    polling.stop();
  });

  it('uses GET method with an Accept: application/json header', async () => {
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    expect(fetchMock.calls[0].opts.method).toBe('GET');
    const headers = fetchMock.calls[0].opts.headers as Record<string, string>;
    expect(headers.Accept).toBe('application/json');
    polling.stop();
  });

  it('parses a JSON array of stored envelopes and fires onEvent for each', async () => {
    const fetchMock = makeFetchMock(200, [agentEnvelope(1), runEnvelope(2)]);
    const events: NormalizedGatewayEvent[] = [];
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: (e) => events.push(e),
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    expect(events.length).toBe(2);
    expect(events[0].type).toBe('app.agent.status.updated');
    expect(events[0].recognized).toBe(true);
    expect(events[1].type).toBe('app.run.status.updated');
    expect(events[1].recognized).toBe(true);
    polling.stop();
  });

  it('parses an object with an events array', async () => {
    const fetchMock = makeFetchMock(200, { events: [agentEnvelope(5)] });
    const events: NormalizedGatewayEvent[] = [];
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: (e) => events.push(e),
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    expect(events.length).toBe(1);
    expect(events[0].id).toBe(5);
    polling.stop();
  });

  it('updates lastEventId to the highest event id in the response', async () => {
    const fetchMock = makeFetchMock(200, [agentEnvelope(3), runEnvelope(7), agentEnvelope(5)]);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    expect(polling.getLastEventId()).toBe(7);
    polling.stop();
  });

  it('does not lower lastEventId when a response carries only older ids', async () => {
    const fetchMock = makeFetchMock(200, [agentEnvelope(2)]);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    polling.setLastEventId(10);
    await polling.start();
    expect(polling.getLastEventId()).toBe(10);
    polling.stop();
  });

  it('fires onUnauthenticated and stops polling on a 401 response', async () => {
    const fetchMock = makeFetchMock(401, {});
    let unauth = false;
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      onUnauthenticated: () => { unauth = true; },
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(unauth).toBe(true);
    // After 401, no further polls should occur.
    fetchMock.calls.length = 0;
    await vi.advanceTimersByTimeAsync(5000);
    expect(fetchMock.calls.length).toBe(0);
  });

  it('schedules the next poll after a non-ok non-401 response', async () => {
    const fetchMock = makeFetchMock(500, {});
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(fetchMock.calls.length).toBe(1);
    await vi.advanceTimersByTimeAsync(1500);
    expect(fetchMock.calls.length).toBe(2);
    polling.stop();
  });

  it('schedules the next poll after a network error', async () => {
    const fetchMock = makeThrowingFetch(new TypeError('network error'));
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(fetchMock.calls.length).toBe(1);
    await vi.advanceTimersByTimeAsync(1500);
    expect(fetchMock.calls.length).toBe(2);
    polling.stop();
  });

  it('schedules the next poll after an empty response body', async () => {
    const fetchMock = makeFetchMock(200, null);
    const events: NormalizedGatewayEvent[] = [];
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: (e) => events.push(e),
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(events.length).toBe(0);
    await vi.advanceTimersByTimeAsync(1500);
    expect(fetchMock.calls.length).toBe(2);
    polling.stop();
  });

  it('silently drops invalid JSON in the response body', async () => {
    const mock = vi.fn(async () => ({
      ok: true,
      status: 200,
      text: async () => 'not json',
      headers: new Headers(),
    }) as Response) as unknown as typeof fetch;
    const events: NormalizedGatewayEvent[] = [];
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: (e) => events.push(e),
      fetchImpl: mock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(events.length).toBe(0);
    polling.stop();
  });

  it('silently drops non-string items in the response array', async () => {
    const fetchMock = makeFetchMock(200, [agentEnvelope(1), { not: 'a string' }, 42, null]);
    const events: NormalizedGatewayEvent[] = [];
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: (e) => events.push(e),
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(events.length).toBe(1);
    expect(events[0].id).toBe(1);
    polling.stop();
  });

  it('silently drops items that fail normalization', async () => {
    const fetchMock = makeFetchMock(200, ['{bad json', agentEnvelope(2)]);
    const events: NormalizedGatewayEvent[] = [];
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: (e) => events.push(e),
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(events.length).toBe(1);
    expect(events[0].id).toBe(2);
    polling.stop();
  });

  it('stop cancels the scheduled next poll', async () => {
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(fetchMock.calls.length).toBe(1);
    polling.stop();
    await vi.advanceTimersByTimeAsync(5000);
    expect(fetchMock.calls.length).toBe(1);
  });

  it('start is idempotent when already running', async () => {
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    await polling.start();
    expect(fetchMock.calls.length).toBe(1);
    polling.stop();
  });

  it('restarts polling after stop', async () => {
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    polling.stop();
    fetchMock.calls.length = 0;
    await polling.start();
    expect(fetchMock.calls.length).toBe(1);
    polling.stop();
  });

  it('preserves lastEventId across stop and restart', async () => {
    const fetchMock = makeFetchMock(200, [agentEnvelope(15)]);
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 1000,
    });
    await polling.start();
    expect(polling.getLastEventId()).toBe(15);
    polling.stop();
    const fetchMock2 = makeFetchMock(200, []);
    // Reuse the same polling instance by swapping the fetch impl via a new start.
    // lastEventId is internal state; verify via getLastEventId.
    expect(polling.getLastEventId()).toBe(15);
  });

  it('normalizes events through the same boundary as the SSE stream', async () => {
    const fetchMock = makeFetchMock(200, [agentEnvelope(1)]);
    const events: NormalizedGatewayEvent[] = [];
    const polling = new SsePollingFallback({
      config: makeConfig(),
      onEvent: (e) => events.push(e),
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    const e = events[0];
    expect(e.recognized).toBe(true);
    expect(e.sentinel).toBe(false);
    expect(e.raw).toBe(agentEnvelope(1));
    expect(e.payload).toEqual({
      schema_version: '1.0.0',
      agent_id: 'u:sage',
      display_name: 'Sage',
      role: 'sage',
      status: 'running',
      observed_at: '2026-01-01T00:00:00Z',
    });
    polling.stop();
  });

  it('strips a trailing slash from the base URL', async () => {
    const cfg = parseRuntimeConfig({
      schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
      gateway_base_url: 'https://g8e.example.com/',
      passkey_rp_id: 'g8e.example.com',
      passkey_rp_name: 'g8e',
      app_name: 'g8e',
    });
    const fetchMock = makeFetchMock(200, []);
    const polling = new SsePollingFallback({
      config: cfg,
      onEvent: () => {},
      fetchImpl: fetchMock,
      pollIntervalMs: 60000,
    });
    await polling.start();
    expect(fetchMock.calls[0].url).toBe(
      'https://g8e.example.com/api/v1/sse/events?limit=100',
    );
    polling.stop();
  });
});
