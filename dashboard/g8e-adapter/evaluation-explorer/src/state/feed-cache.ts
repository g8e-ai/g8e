// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { FeedSnapshot, ProjectionRecord } from '../contract/types';
import { isFeedSnapshot, isProjectionRecord } from '../contract/validators';

const CACHE_SCHEMA_VERSION = '1.0.0';
const CACHE_DATABASE = 'g8e-evaluation-explorer';
const CACHE_STORE = 'feeds';

export interface CachedFeed {
  schema_version: typeof CACHE_SCHEMA_VERSION;
  mirror_origin: string;
  snapshot: FeedSnapshot;
  records: ProjectionRecord[];
}

export interface FeedCacheStorage {
  load(mirrorOrigin: string): Promise<CachedFeed | null>;
  save(feed: CachedFeed): Promise<void>;
  clear(mirrorOrigin: string): Promise<void>;
}

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function validateCachedFeed(value: unknown, mirrorOrigin: string): CachedFeed | null {
  if (!object(value) || value.schema_version !== CACHE_SCHEMA_VERSION || value.mirror_origin !== mirrorOrigin) return null;
  try {
    isFeedSnapshot(value.snapshot);
    if (!Array.isArray(value.records)) return null;
    const records = value.records as ProjectionRecord[];
    for (const record of records) isProjectionRecord(record);
    if (value.snapshot.high_water_sequence === 0) return records.length === 0 ? value as unknown as CachedFeed : null;
    if (records.length === 0 || records[records.length - 1]?.sequence !== value.snapshot.high_water_sequence) return null;
    for (let index = 1; index < records.length; index++) {
      if (records[index]!.sequence <= records[index - 1]!.sequence) return null;
    }
    return value as unknown as CachedFeed;
  } catch {
    return null;
  }
}

export function createCachedFeed(mirrorOrigin: string, snapshot: FeedSnapshot, records: ProjectionRecord[]): CachedFeed {
  return { schema_version: CACHE_SCHEMA_VERSION, mirror_origin: mirrorOrigin, snapshot, records };
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve();
    transaction.onerror = () => reject(transaction.error);
    transaction.onabort = () => reject(transaction.error);
  });
}

async function openDatabase(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return null;
  const request = indexedDB.open(CACHE_DATABASE, 1);
  request.onupgradeneeded = () => {
    if (!request.result.objectStoreNames.contains(CACHE_STORE)) request.result.createObjectStore(CACHE_STORE, { keyPath: 'mirror_origin' });
  };
  return requestResult(request);
}

export class IndexedDBFeedCache implements FeedCacheStorage {
  async load(mirrorOrigin: string): Promise<CachedFeed | null> {
    const database = await openDatabase();
    if (!database) return null;
    try {
      const transaction = database.transaction(CACHE_STORE, 'readonly');
      const completed = transactionDone(transaction);
      const value = await requestResult(transaction.objectStore(CACHE_STORE).get(mirrorOrigin) as IDBRequest<unknown>);
      await completed;
      return validateCachedFeed(value, mirrorOrigin);
    } finally {
      database.close();
    }
  }

  async save(feed: CachedFeed): Promise<void> {
    const database = await openDatabase();
    if (!database) return;
    try {
      const transaction = database.transaction(CACHE_STORE, 'readwrite');
      const completed = transactionDone(transaction);
      transaction.objectStore(CACHE_STORE).put(feed);
      await completed;
    } finally {
      database.close();
    }
  }

  async clear(mirrorOrigin: string): Promise<void> {
    const database = await openDatabase();
    if (!database) return;
    try {
      const transaction = database.transaction(CACHE_STORE, 'readwrite');
      const completed = transactionDone(transaction);
      transaction.objectStore(CACHE_STORE).delete(mirrorOrigin);
      await completed;
    } finally {
      database.close();
    }
  }
}
