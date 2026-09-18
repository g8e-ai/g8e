// Startup orchestrator. Implements the required boot order:
//   1. load runtime config
//   2. fetch bootstrap (snapshot seal target + bounded recent projections)
//   3. index recent projections for first paint
//   4. page history forward from the first retained record to the
//      snapshot's high-water sequence (history items arrive in ascending
//      sequence order; the store enforces strict contiguity)
//   5. seal against feed-chain state once observed == high-water
//   6. render once after history replay (batched store updates)
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
  loadRuntimeConfig,
  normalizeRecentProjection,
  openStream,
  type StreamHandle,
} from './feed';
import { normalizeHistoryItem } from './feed';
import type { FeedSnapshot } from '../contract/types';

const HISTORY_LIMIT = 100;
const MAX_HISTORY_ROUNDS = 10;
const MAX_HISTORY_PAGES = 100;

let activeStream: StreamHandle | null = null;
// Generation token: a superseded or stopped startFeed must not connect a
// stream after a newer run (React StrictMode mounts effects twice).
let generation = 0;

export interface StartOptions {
  /** Inject a fetch implementation for tests. */
  fetchImpl?: typeof fetch;
}

export async function startFeed(opts: StartOptions = {}): Promise<void> {
  const gen = ++generation;
  const fetchImpl = opts.fetchImpl ?? fetch.bind(globalThis);
  try {
    evalStore.setConnection('connecting', 'Connecting to the public mirror.');
    const runtime = await loadRuntimeConfig(fetchImpl);
    const bootstrap = await fetchBootstrap(runtime.mirror_origin, fetchImpl);
    if (gen !== generation) return;
    const snapshot = bootstrap.snapshot;
    const recent = bootstrap.recent_projections.map(normalizeRecentProjection);
    evalStore.initBootstrap(snapshot, recent, bootstrap.proof_catalog_summary.artifact_count);
    await reconcileHistory(runtime.mirror_origin, snapshot.source_id, fetchImpl, gen);
    if (gen !== generation) return;
    connectStream(runtime.mirror_origin, snapshot.source_id, evalStore.getState().observedSequence, gen);
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
): Promise<void> {
  evalStore.beginBatch();
  try {
    for (let round = 0; round < MAX_HISTORY_ROUNDS; round++) {
      for (let page = 0; page < MAX_HISTORY_PAGES; page++) {
        const cursor = evalStore.getState().observedSequence;
        const pageData = await fetchHistoryPage(mirrorOrigin, sourceId, cursor, HISTORY_LIMIT, fetchImpl);
        for (const item of pageData.items) {
          evalStore.acceptProjection(normalizeHistoryItem(item));
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

function connectStream(mirrorOrigin: string, sourceId: string, sinceId: number, gen: number): void {
  activeStream?.close();
  activeStream = openStream(mirrorOrigin, sourceId, sinceId, {
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
