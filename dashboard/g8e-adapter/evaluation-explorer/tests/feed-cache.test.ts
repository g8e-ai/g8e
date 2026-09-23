// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { afterEach, describe, expect, it, vi } from 'vitest';
import type { FeedSnapshot, ProjectionRecord } from '../src/contract/types';
import { fixtureCatalogExploratory, fixtureModelSummaries } from '../src/fixtures/fixtures';
import { createCachedFeed, type CachedFeed, type FeedCacheStorage, validateCachedFeed } from '../src/state/feed-cache';
import { startFeed, stopFeed } from '../src/state/startup';
import type { openStream } from '../src/state/feed';

const mirrorOrigin = 'https://mirror.example';
const firstHash = '1'.repeat(64);
const latestHash = '2'.repeat(64);

function snapshot(sequence: number, hash: string): FeedSnapshot {
  return {
    protocol_version: '1.0.0',
    source_id: 'public-source',
    high_water_sequence: sequence,
    feed_chain_hash: hash,
    batch_count: sequence,
    generated_at: '2026-09-21T08:00:00Z',
    freshness: 'active',
  };
}

function record(sequence: number, value: object): ProjectionRecord {
  return { sequence, record_type: 'projection', record_bytes: JSON.stringify(value) };
}

class MemoryFeedCache implements FeedCacheStorage {
  saved: CachedFeed | null = null;
  cleared = false;

  constructor(private readonly cached: CachedFeed | null) {}

  async load(): Promise<CachedFeed | null> {
    return this.cached;
  }

  async save(feed: CachedFeed): Promise<void> {
    this.saved = feed;
  }

  async clear(): Promise<void> {
    this.cleared = true;
  }
}

function jsonResponse(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } });
}

afterEach(() => stopFeed());

describe('feed cache', () => {
  it('rejects a cache with a sequence gap', () => {
    const cached = createCachedFeed(mirrorOrigin, snapshot(3, latestHash), [
      record(1, fixtureCatalogExploratory),
      record(3, fixtureModelSummaries[0]!),
    ]);
    expect(validateCachedFeed(cached, mirrorOrigin)).toBeNull();
  });

  it('resumes history from a retained cache checkpoint', async () => {
    const cached = createCachedFeed(mirrorOrigin, snapshot(2, firstHash), [
      record(1, fixtureCatalogExploratory),
      record(2, fixtureModelSummaries[0]!),
    ]);
    const storage = new MemoryFeedCache(cached);
    const requests: string[] = [];
    const fetchImpl = vi.fn(async (input: string | URL | Request) => {
      const target = String(input);
      requests.push(target);
      if (target === '/runtime.json') return jsonResponse({ schema_version: '1.0.0', mirror_origin: mirrorOrigin });
      const url = new URL(target);
      if (url.pathname === '/bootstrap') {
        return jsonResponse({
          protocol_version: '1.0.0',
          snapshot: snapshot(3, latestHash),
          source_freshness: 'active',
          recent_projections: [],
          proof_catalog_summary: { artifact_count: 0, total_byte_size: 0 },
          generated_at: '2026-09-21T08:00:00Z',
        });
      }
      if (url.pathname === '/snapshot' && url.searchParams.get('sequence') === '2') return jsonResponse(snapshot(2, firstHash));
      if (url.pathname === '/history') {
        return jsonResponse({
          protocol_version: '1.0.0',
          items: [{ ...fixtureModelSummaries[1]!, sequence: 3, record_type: 'projection' }],
          has_more: false,
          limit: 500,
        });
      }
      if (url.pathname === '/snapshot') return jsonResponse(snapshot(3, latestHash));
      return jsonResponse({}, 404);
    }) as typeof fetch;
    let streamCursor = 0;
    const streamFactory: typeof openStream = (_origin, _source, sinceSequence) => {
      streamCursor = sinceSequence;
      return { close: () => undefined };
    };

    await startFeed({ fetchImpl, cacheStorage: storage, streamFactory });

    expect(requests.some((request) => request.includes('/history?source=public-source&cursor=2&limit=500'))).toBe(true);
    expect(storage.saved?.records).toHaveLength(3);
    expect(storage.saved?.snapshot.feed_chain_hash).toBe(latestHash);
    expect(streamCursor).toBe(3);
  });

  it('clears an incompatible cache and replays retained history', async () => {
    const cached = createCachedFeed(mirrorOrigin, snapshot(2, firstHash), [
      record(1, fixtureCatalogExploratory),
      record(2, fixtureModelSummaries[0]!),
    ]);
    const storage = new MemoryFeedCache(cached);
    const requests: string[] = [];
    const fetchImpl = vi.fn(async (input: string | URL | Request) => {
      const target = String(input);
      requests.push(target);
      if (target === '/runtime.json') return jsonResponse({ schema_version: '1.0.0', mirror_origin: mirrorOrigin });
      const url = new URL(target);
      if (url.pathname === '/bootstrap') {
        return jsonResponse({
          protocol_version: '1.0.0',
          snapshot: snapshot(3, latestHash),
          source_freshness: 'active',
          recent_projections: [],
          proof_catalog_summary: { artifact_count: 0, total_byte_size: 0 },
          generated_at: '2026-09-21T08:00:00Z',
        });
      }
      if (url.pathname === '/snapshot' && url.searchParams.get('sequence') === '2') return jsonResponse(snapshot(2, '9'.repeat(64)));
      if (url.pathname === '/history') {
        return jsonResponse({
          protocol_version: '1.0.0',
          items: [
            { ...fixtureCatalogExploratory, sequence: 1, record_type: 'projection' },
            { ...fixtureModelSummaries[0]!, sequence: 2, record_type: 'projection' },
            { ...fixtureModelSummaries[1]!, sequence: 3, record_type: 'projection' },
          ],
          has_more: false,
          limit: 500,
        });
      }
      if (url.pathname === '/snapshot') return jsonResponse(snapshot(3, latestHash));
      return jsonResponse({}, 404);
    }) as typeof fetch;
    const streamFactory: typeof openStream = () => ({ close: () => undefined });

    await startFeed({ fetchImpl, cacheStorage: storage, streamFactory });

    expect(storage.cleared).toBe(true);
    expect(requests.some((request) => request.includes('/history?source=public-source&cursor=0&limit=500'))).toBe(true);
  });
});
