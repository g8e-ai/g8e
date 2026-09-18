// Public mirror feed client. Runtime validation, anonymous fetch construction,
// endpoint allowlisting, and SSE transport come from the audited g8e adapter.
// Explorer-specific validators normalize the mirror wire shapes into the
// complete typed evaluation corpus store without reaching a private service.

import {
  PublicSseStream,
  createPublicClient,
  parsePublicRuntimeConfig,
  type PublicRuntimeConfig,
  type PublicSseConnectionState,
} from '../../../src/public';
import type {
  FeedBootstrap,
  FeedHistoryPage,
  FeedSnapshot,
  ProjectionRecord,
} from '../contract/types';
import {
  isFeedBootstrap,
  isFeedHistoryPage,
  isProjectionRecord,
  isFeedSnapshot,
} from '../contract/validators';
import { evalStore } from './store';

export type RuntimeConfig = PublicRuntimeConfig;

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** Normalize a /history item ({...payload, sequence, record_type}). */
export function normalizeHistoryItem(item: unknown): ProjectionRecord {
  if (!isObject(item)) throw new Error('invalid public history record');
  const { sequence, record_type, ...payload } = item;
  const record: ProjectionRecord = {
    sequence: Number(sequence),
    record_type: record_type as ProjectionRecord['record_type'],
    record_bytes: JSON.stringify(payload),
  };
  isProjectionRecord(record);
  return record;
}

/** Normalize a /bootstrap recent_projections item ({...payload, sequence}). */
export function normalizeRecentProjection(item: unknown): ProjectionRecord {
  if (!isObject(item)) throw new Error('invalid public bootstrap record');
  const { sequence, ...payload } = item;
  const record: ProjectionRecord = {
    sequence: Number(sequence),
    record_type: 'projection',
    record_bytes: JSON.stringify(payload),
  };
  isProjectionRecord(record);
  return record;
}

/** Normalize an SSE record event (id: sequence, event: type, data: bytes). */
export function normalizeStreamRecord(recordType: string, lastEventId: string, data: string): ProjectionRecord {
  const record: ProjectionRecord = {
    sequence: Number(lastEventId),
    record_type: recordType as ProjectionRecord['record_type'],
    record_bytes: data,
  };
  isProjectionRecord(record);
  return record;
}

export async function loadRuntimeConfig(fetchImpl: typeof fetch = fetch.bind(globalThis)): Promise<RuntimeConfig> {
  const response = await fetchImpl('/runtime.json', {
    method: 'GET',
    credentials: 'omit',
    headers: { Accept: 'application/json' },
  });
  if (!response.ok) throw new Error(`runtime config returned ${response.status}`);
  return parsePublicRuntimeConfig(await response.json());
}

function client(mirrorOrigin: string, fetchImpl: typeof fetch) {
  return createPublicClient(parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: mirrorOrigin }), fetchImpl);
}

export async function fetchBootstrap(
  mirrorOrigin: string,
  fetchImpl: typeof fetch = fetch.bind(globalThis),
): Promise<FeedBootstrap> {
  const bootstrap = await client(mirrorOrigin, fetchImpl).bootstrap();
  isFeedBootstrap(bootstrap);
  return bootstrap;
}

export async function fetchSnapshot(
  mirrorOrigin: string,
  sourceId: string,
  fetchImpl: typeof fetch = fetch.bind(globalThis),
): Promise<FeedSnapshot> {
  const snapshot = await client(mirrorOrigin, fetchImpl).snapshot(sourceId);
  isFeedSnapshot(snapshot);
  return snapshot;
}

export async function fetchHistoryPage(
  mirrorOrigin: string,
  sourceId: string,
  cursor: number,
  limit: number,
  fetchImpl: typeof fetch = fetch.bind(globalThis),
): Promise<FeedHistoryPage> {
  const history = await client(mirrorOrigin, fetchImpl).history(sourceId, cursor, limit);
  isFeedHistoryPage(history);
  return history;
}

export interface StreamHandlers {
  onSnapshot: (snapshot: FeedSnapshot) => void;
  onProjection: (record: ProjectionRecord) => void;
  onTruncated: () => void;
  onError: () => void;
  onStateChange?: (state: PublicSseConnectionState) => void;
}

export interface StreamHandle {
  close: () => void;
}

export function openStream(
  mirrorOrigin: string,
  sourceId: string,
  sinceId: number,
  handlers: StreamHandlers,
): StreamHandle {
  const stream = new PublicSseStream({
    config: parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: mirrorOrigin }),
    source_id: sourceId,
    since_sequence: sinceId,
    callbacks: {
      onSnapshot: (snapshot) => {
        try {
          isFeedSnapshot(snapshot);
          handlers.onSnapshot(snapshot);
        } catch {
          handlers.onError();
        }
      },
      onRecord: (record) => {
        if (record.sequence <= evalStore.getState().observedSequence) return;
        try {
          handlers.onProjection(normalizeStreamRecord(record.record_type, String(record.sequence), record.record_bytes));
        } catch {
          handlers.onError();
        }
      },
      onTruncated: handlers.onTruncated,
      onError: handlers.onError,
      onStateChange: handlers.onStateChange,
    },
  });
  stream.connect();
  return { close: () => stream.disconnect() };
}
