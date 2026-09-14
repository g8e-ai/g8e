import { createPublicFetch, type PublicFetch } from './fetch';
import type { PublicRuntimeConfig } from './runtime_config';
import type { PublicBootstrap, PublicHistory, PublicItem, PublicSnapshot } from './state';

export class PublicClientError extends Error {
  constructor(public readonly operation: string, public readonly status: number, message: string) {
    super(`public client ${operation}: ${message}`);
    this.name = 'PublicClientError';
  }
}

export interface PublicClient {
  bootstrap(sourceID?: string, signal?: AbortSignal): Promise<PublicBootstrap>;
  snapshot(sourceID?: string, signal?: AbortSignal): Promise<PublicSnapshot>;
  history(sourceID?: string, cursor?: number, limit?: number, signal?: AbortSignal): Promise<PublicHistory>;
}

function object(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function string(value: unknown): value is string {
  return typeof value === 'string';
}

function integer(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
}

function snapshot(value: unknown): value is PublicSnapshot {
  const candidate = object(value);
  return candidate !== null
    && string(candidate.protocol_version)
    && string(candidate.source_id)
    && integer(candidate.high_water_sequence)
    && string(candidate.feed_chain_hash)
    && /^[0-9a-f]{64}$/.test(candidate.feed_chain_hash)
    && integer(candidate.batch_count)
    && string(candidate.generated_at)
    && ['active', 'delayed', 'stale', 'intentionally_stopped', 'safety_stopped', 'source_offline'].includes(String(candidate.freshness));
}

function item(value: unknown): value is PublicItem {
  const candidate = object(value);
  return candidate !== null && integer(candidate.sequence) && string(candidate.record_type);
}

function recentProjection(value: unknown): boolean {
  const candidate = object(value);
  return candidate !== null && integer(candidate.sequence) && candidate.record_type === undefined;
}

function bootstrap(value: unknown): value is PublicBootstrap {
  const candidate = object(value);
  const summary = object(candidate?.proof_catalog_summary);
  return candidate !== null
    && string(candidate.protocol_version)
    && snapshot(candidate.snapshot)
    && string(candidate.source_freshness)
    && Array.isArray(candidate.recent_projections)
    && candidate.recent_projections.every(recentProjection)
    && summary !== null
    && integer(summary.artifact_count)
    && integer(summary.total_byte_size)
    && string(candidate.generated_at);
}

function history(value: unknown): value is PublicHistory {
  const candidate = object(value);
  return candidate !== null
    && string(candidate.protocol_version)
    && Array.isArray(candidate.items)
    && candidate.items.every(item)
    && (candidate.cursor === undefined || string(candidate.cursor))
    && typeof candidate.has_more === 'boolean'
    && integer(candidate.limit);
}

function query(values: Record<string, string | number | undefined>): string {
  const params = new URLSearchParams();
  for (const [name, value] of Object.entries(values)) if (value !== undefined && value !== '') params.set(name, String(value));
  const encoded = params.toString();
  return encoded === '' ? '' : `?${encoded}`;
}

async function read<T>(fetchPublic: PublicFetch, operation: 'bootstrap' | 'snapshot' | 'history', suffix: string, validator: (value: unknown) => value is T, signal?: AbortSignal): Promise<T> {
  const response = await fetchPublic(operation, suffix, signal);
  if (!response.ok) throw new PublicClientError(operation, response.status, 'mirror request failed');
  if (!validator(response.body)) throw new PublicClientError(operation, response.status, 'invalid response');
  return response.body;
}

export function createPublicClient(config: PublicRuntimeConfig, fetchImpl: typeof fetch = fetch): PublicClient {
  const fetchPublic = createPublicFetch(config, fetchImpl);
  return {
    bootstrap: (sourceID, signal) => read(fetchPublic, 'bootstrap', query({ source: sourceID }), bootstrap, signal),
    snapshot: (sourceID, signal) => read(fetchPublic, 'snapshot', query({ source: sourceID }), snapshot, signal),
    history: (sourceID, cursor, limit, signal) => read(fetchPublic, 'history', query({ source: sourceID, cursor, limit }), history, signal),
  };
}
