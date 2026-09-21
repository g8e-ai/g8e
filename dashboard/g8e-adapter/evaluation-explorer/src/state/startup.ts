// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Startup orchestrator. Implements the required boot order:
//   1. load runtime config
//   2. fetch bootstrap (snapshot seal target + bounded recent projections)
//   3. index recent projections for first paint
//   4. page history forward from the first retained record to the
//      snapshot's high-water sequence (history items arrive in ascending
//      sequence order; the store enforces strict contiguity)
//   5. seal against feed-chain state once observed == high-water
//   6. publish reconciled history in one batched store update
//   7. connect SSE from the accepted cursor
//
// On any transport failure the store fails closed: last accepted data
// remains visible with an offline label, and a cold start remains empty.
// The orchestrator never reaches the private listener (8081), Gateway,
// the provider, or report paths.

import { evalStore } from './store';
import {
  fetchBootstrap,
  fetchHistoryPage,
  fetchSnapshot,
  fetchSnapshotAt,
  loadRuntimeConfig,
  normalizeRecentProjection,
  openStream,
  type StreamHandle,
} from './feed';
import { normalizeHistoryItem } from './feed';
import { createCachedFeed, IndexedDBFeedCache, type CachedFeed, type FeedCacheStorage } from './feed-cache';
import type { FeedSnapshot, ProjectionRecord } from '../contract/types';

const HISTORY_LIMIT = 500;
const MAX_HISTORY_ROUNDS = 10;
const MAX_HISTORY_PAGES = 100;

let activeStream: StreamHandle | null = null;
// Generation token: a superseded or stopped startFeed must not connect a
// stream after a newer run (React StrictMode mounts effects twice).
let generation = 0;

export interface StartOptions {
  /** Inject a fetch implementation for tests. */
  fetchImpl?: typeof fetch;
  cacheStorage?: FeedCacheStorage;
  streamFactory?: typeof openStream;
}

async function loadCompatibleCache(
  storage: FeedCacheStorage,
  mirrorOrigin: string,
  snapshot: FeedSnapshot,
  fetchImpl: typeof fetch,
): Promise<CachedFeed | null> {
  let cached: CachedFeed | null;
  try {
    cached = await storage.load(mirrorOrigin);
  } catch {
    return null;
  }
  if (!cached) return null;
  const cachedSnapshot = cached.snapshot;
  if (
    cachedSnapshot.source_id !== snapshot.source_id ||
    cachedSnapshot.protocol_version !== snapshot.protocol_version ||
    cachedSnapshot.high_water_sequence > snapshot.high_water_sequence
  ) {
    await storage.clear(mirrorOrigin).catch(() => undefined);
    return null;
  }
  if (cachedSnapshot.high_water_sequence === snapshot.high_water_sequence) {
    if (cachedSnapshot.feed_chain_hash === snapshot.feed_chain_hash) return cached;
    await storage.clear(mirrorOrigin).catch(() => undefined);
    return null;
  }
  try {
    const anchor = await fetchSnapshotAt(
      mirrorOrigin,
      snapshot.source_id,
      cachedSnapshot.high_water_sequence,
      fetchImpl,
    );
    if (
      anchor.source_id === cachedSnapshot.source_id &&
      anchor.protocol_version === cachedSnapshot.protocol_version &&
      anchor.high_water_sequence === cachedSnapshot.high_water_sequence &&
      anchor.feed_chain_hash === cachedSnapshot.feed_chain_hash
    ) return cached;
  } catch {
    await storage.clear(mirrorOrigin).catch(() => undefined);
    return null;
  }
  await storage.clear(mirrorOrigin).catch(() => undefined);
  return null;
}

async function saveCache(
  storage: FeedCacheStorage,
  mirrorOrigin: string,
  records: ProjectionRecord[],
): Promise<void> {
  const snapshot = evalStore.getState().currentSnapshot;
  if (!snapshot || snapshot.high_water_sequence !== evalStore.getState().observedSequence) return;
  await storage.save(createCachedFeed(mirrorOrigin, snapshot, records)).catch(() => undefined);
}

export async function startFeed(opts: StartOptions = {}): Promise<void> {
  const gen = ++generation;
  const fetchImpl = opts.fetchImpl ?? fetch.bind(globalThis);
  const cacheStorage = opts.cacheStorage ?? new IndexedDBFeedCache();
  const streamFactory = opts.streamFactory ?? openStream;
  try {
    evalStore.setConnection('connecting', 'Connecting to the public mirror.');
    const runtime = await loadRuntimeConfig(fetchImpl);
    const bootstrap = await fetchBootstrap(runtime.mirror_origin, fetchImpl);
    if (gen !== generation) return;
    const snapshot = bootstrap.snapshot;
    const recent = bootstrap.recent_projections.map(normalizeRecentProjection);
    evalStore.initBootstrap(snapshot, recent, bootstrap.proof_catalog_summary.artifact_count);
    const cached = await loadCompatibleCache(cacheStorage, runtime.mirror_origin, snapshot, fetchImpl);
    if (gen !== generation) return;
    const records = cached ? [...cached.records] : [];
    if (cached) {
      evalStore.initBootstrap(cached.snapshot, [], bootstrap.proof_catalog_summary.artifact_count);
      for (const record of cached.records) evalStore.acceptProjection(record);
      evalStore.acceptSnapshot(snapshot);
    }
    await reconcileHistory(runtime.mirror_origin, snapshot.source_id, fetchImpl, gen, records);
    if (gen !== generation) return;
    await saveCache(cacheStorage, runtime.mirror_origin, records);
    if (gen !== generation) return;
    connectStream(runtime.mirror_origin, snapshot.source_id, evalStore.getState().observedSequence, gen, streamFactory);
  } catch (error) {
    if (gen !== generation) return;
    const message = error instanceof Error ? error.message : 'public mirror is unreachable';
    evalStore.setOffline(`The public mirror is unreachable: ${message}. Last accepted data remains visible.`);
  }
}

async function reconcileHistory(
  mirrorOrigin: string,
  sourceId: string,
  fetchImpl: typeof fetch,
  gen: number,
  records: ProjectionRecord[],
): Promise<void> {
  evalStore.beginBatch();
  try {
    for (let round = 0; round < MAX_HISTORY_ROUNDS; round++) {
      for (let page = 0; page < MAX_HISTORY_PAGES; page++) {
        const cursor = evalStore.getState().observedSequence;
        const pageData = await fetchHistoryPage(mirrorOrigin, sourceId, cursor, HISTORY_LIMIT, fetchImpl);
        for (const item of pageData.items) {
          const record = normalizeHistoryItem(item);
          records.push(record);
          evalStore.acceptProjection(record);
        }
        if (!pageData.has_more) break;
        if (pageData.items.length === 0 || page === MAX_HISTORY_PAGES - 1) {
          throw new Error('public history did not advance');
        }
      }
      if (gen !== generation) return;
      const snapshot: FeedSnapshot = await fetchSnapshot(mirrorOrigin, sourceId, fetchImpl);
      evalStore.acceptSnapshot(snapshot);
      if (snapshot.high_water_sequence === evalStore.getState().observedSequence) return;
    }
    throw new Error('public history could not reach snapshot');
  } finally {
    evalStore.endBatch();
  }
}

function connectStream(
  mirrorOrigin: string,
  sourceId: string,
  sinceId: number,
  gen: number,
  streamFactory: typeof openStream,
): void {
  activeStream?.close();
  activeStream = streamFactory(mirrorOrigin, sourceId, sinceId, {
    onSnapshot: (snapshot) => evalStore.acceptSnapshot(snapshot),
    onProjection: (record) => {
      evalStore.acceptProjection(record);
    },
    onTruncated: () => {
      if (gen === generation) void startFeed();
    },
    onError: () => {
      evalStore.setOffline('The mirror stream is unreachable. Last accepted data remains visible.');
    },
    onStateChange: (streamState) => {
      if (gen !== generation) return;
      evalStore.setStreamConnection(streamState);
      if (streamState === 'connected') {
        evalStore.setConnection('live', 'Streaming live updates via SSE.');
      }
    },
  });
}

export function stopFeed(): void {
  generation++;
  activeStream?.close();
  activeStream = null;
}
