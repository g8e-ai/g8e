import { describe, expect, it } from 'vitest';

import {
  PublicSseStream,
  applyPublicRecord,
  createPublicMirrorState,
  parsePublicRuntimeConfig,
  reconcilePublicSnapshot,
} from '../src/public';

class FakeEventSource {
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readyState = 0;
  private readonly listeners = new Map<string, (event: MessageEvent) => void>();

  close(): void {
    this.readyState = 2;
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
    this.listeners.set(type, listener as (event: MessageEvent) => void);
  }

  emit(type: string, data: unknown, lastEventId = ''): void {
    const event = new MessageEvent(type, { data: typeof data === 'string' ? data : JSON.stringify(data), lastEventId });
    this.listeners.get(type)?.(event);
  }
}

describe('public spectator SSE boundary', () => {
  it('opens the mirror stream without credentials and resumes from the observed sequence', () => {
    const created: Array<{ url: string; init: EventSourceInit | undefined; source: FakeEventSource }> = [];
    const stream = new PublicSseStream({
      config: parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: 'https://feed.example.com' }),
      source_id: 'deployment-a',
      since_sequence: 7,
      callbacks: {},
      eventSourceFactory: (url, init) => {
        const source = new FakeEventSource();
        created.push({ url, init, source });
        return source as unknown as EventSource;
      },
    });

    stream.connect();

    expect(created[0].url).toBe('https://feed.example.com/stream?source=deployment-a&since_id=7');
    expect(created[0].init).toEqual({ withCredentials: false });
  });

  it('reconciles records and seals their chain position with a snapshot', () => {
    let state = createPublicMirrorState();
    const source = new FakeEventSource();
    const stream = new PublicSseStream({
      config: parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: 'https://feed.example.com' }),
      source_id: 'deployment-a',
      callbacks: {
        onRecord: (record) => { state = applyPublicRecord(state, record); },
        onSnapshot: (snapshot) => { state = reconcilePublicSnapshot(state, snapshot); },
      },
      eventSourceFactory: () => source as unknown as EventSource,
    });
    stream.connect();

    source.emit('projection', '{"campaign_id":"campaign-a"}', '1');
    source.emit('snapshot', {
      protocol_version: '1.0.0',
      source_id: 'deployment-a',
      high_water_sequence: 1,
      feed_chain_hash: '1'.repeat(64),
      batch_count: 1,
      generated_at: '2026-09-13T00:00:00Z',
      freshness: 'active',
    });

    expect(state.observed_sequence).toBe(1);
    expect(state.high_water_sequence).toBe(1);
    expect(state.feed_chain_hash).toBe('1'.repeat(64));
  });
});
