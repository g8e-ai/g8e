// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export type PublicFreshness = 'active' | 'delayed' | 'stale' | 'intentionally_stopped' | 'safety_stopped' | 'source_offline';
export type PublicDisplayState = 'active' | 'delayed' | 'stale' | 'stopped' | 'safety-stopped' | 'offline';

export interface PublicSnapshot {
  readonly protocol_version: string;
  readonly source_id: string;
  readonly high_water_sequence: number;
  readonly feed_chain_hash: string;
  readonly batch_count: number;
  readonly generated_at: string;
  readonly freshness: string;
}

export interface PublicItem {
  readonly sequence: number;
  readonly record_type: string;
  readonly [field: string]: unknown;
}

export interface PublicRecentProjection {
  readonly sequence: number;
  readonly [field: string]: unknown;
}

export interface PublicBootstrap {
  readonly protocol_version: string;
  readonly snapshot: PublicSnapshot;
  readonly source_freshness: string;
  readonly recent_projections: readonly PublicRecentProjection[];
  readonly proof_catalog_summary: {
    readonly artifact_count: number;
    readonly total_byte_size: number;
    readonly last_generated_at?: string;
  };
  readonly generated_at: string;
}

export interface PublicHistory {
  readonly protocol_version: string;
  readonly items: readonly PublicItem[];
  readonly cursor?: string;
  readonly has_more: boolean;
  readonly limit: number;
}

export interface PublicProofCatalogEntry {
  readonly artifact_id: string;
  readonly filename: string;
  readonly media_type: string;
  readonly byte_size: number;
  readonly sha256: string;
  readonly classification: string;
  readonly campaign_id: string;
  readonly source_run_id?: string;
  readonly generated_at: string;
  readonly verification_command: string;
  readonly immutable_url: string;
}

export interface PublicProofCatalog {
  readonly schema_version: string;
  readonly entries: readonly PublicProofCatalogEntry[];
  readonly generated_at: string;
}

export interface PublicRecord {
  readonly sequence: number;
  readonly record_type: string;
  readonly record_hash: string;
  readonly record_bytes: string;
}

export interface PublicMirrorState {
  readonly protocol_version: string;
  readonly source_id: string;
  readonly observed_sequence: number;
  readonly high_water_sequence: number;
  readonly feed_chain_hash: string;
  readonly batch_count: number;
  readonly freshness: PublicFreshness;
  readonly display_state: PublicDisplayState;
  readonly proof_artifact_count: number;
  readonly proof_total_byte_size: number;
  readonly items: readonly PublicItem[];
  readonly max_items: number;
}

const ZERO_HASH = '0'.repeat(64);
const DEFAULT_MAX_ITEMS = 1000;

export class PublicReconciliationError extends Error {
  constructor(message: string) {
    super(`public reconciliation failed: ${message}`);
    this.name = 'PublicReconciliationError';
  }
}

function displayState(freshness: string): { freshness: PublicFreshness; display: PublicDisplayState } {
  switch (freshness) {
    case 'active': return { freshness, display: 'active' };
    case 'delayed': return { freshness, display: 'delayed' };
    case 'stale': return { freshness, display: 'stale' };
    case 'intentionally_stopped': return { freshness, display: 'stopped' };
    case 'safety_stopped': return { freshness, display: 'safety-stopped' };
    case 'source_offline': return { freshness, display: 'offline' };
    default: throw new PublicReconciliationError(`unknown freshness ${freshness}`);
  }
}

function validateSequence(sequence: number): void {
  if (!Number.isSafeInteger(sequence) || sequence < 0) throw new PublicReconciliationError(`invalid sequence ${sequence}`);
}

function retain(items: readonly PublicItem[], maxItems: number): readonly PublicItem[] {
  return items.slice(Math.max(0, items.length - maxItems));
}

function appendItems(state: PublicMirrorState, incoming: readonly PublicItem[]): PublicMirrorState {
  if (incoming.length === 0) return state;
  let expected = state.observed_sequence + 1;
  for (const item of incoming) {
    validateSequence(item.sequence);
    if (item.sequence !== expected) throw new PublicReconciliationError(`expected sequence ${expected}, received ${item.sequence}`);
    expected++;
  }
  const last = incoming[incoming.length - 1]!;
  return {
    ...state,
    observed_sequence: last.sequence,
    items: retain([...state.items, ...incoming], state.max_items),
  };
}

export function createPublicMirrorState(maxItems = DEFAULT_MAX_ITEMS): PublicMirrorState {
  if (!Number.isSafeInteger(maxItems) || maxItems < 1) throw new PublicReconciliationError(`invalid retention limit ${maxItems}`);
  return {
    protocol_version: '',
    source_id: '',
    observed_sequence: 0,
    high_water_sequence: 0,
    feed_chain_hash: ZERO_HASH,
    batch_count: 0,
    freshness: 'source_offline',
    display_state: 'offline',
    proof_artifact_count: 0,
    proof_total_byte_size: 0,
    items: [],
    max_items: maxItems,
  };
}

export function reconcilePublicSnapshot(state: PublicMirrorState, snapshot: PublicSnapshot): PublicMirrorState {
  validateSequence(snapshot.high_water_sequence);
  if (snapshot.high_water_sequence < state.high_water_sequence || snapshot.high_water_sequence < state.observed_sequence) {
    throw new PublicReconciliationError('snapshot sequence regressed');
  }
  if (snapshot.high_water_sequence === state.high_water_sequence && snapshot.feed_chain_hash !== state.feed_chain_hash) {
    throw new PublicReconciliationError('snapshot equivocation');
  }
  if (state.source_id !== '' && snapshot.source_id !== state.source_id) throw new PublicReconciliationError('snapshot source changed');
  if (state.protocol_version !== '' && snapshot.protocol_version !== state.protocol_version) {
    throw new PublicReconciliationError('snapshot protocol changed');
  }
  const mapped = displayState(snapshot.freshness);
  return {
    ...state,
    protocol_version: snapshot.protocol_version,
    source_id: snapshot.source_id,
    high_water_sequence: snapshot.high_water_sequence,
    feed_chain_hash: snapshot.feed_chain_hash,
    batch_count: snapshot.batch_count,
    freshness: mapped.freshness,
    display_state: mapped.display,
  };
}

export function reconcilePublicBootstrap(state: PublicMirrorState, bootstrap: PublicBootstrap): PublicMirrorState {
  if (bootstrap.protocol_version !== bootstrap.snapshot.protocol_version) throw new PublicReconciliationError('bootstrap protocol mismatch');
  if (bootstrap.source_freshness !== bootstrap.snapshot.freshness) throw new PublicReconciliationError('bootstrap freshness mismatch');
  let reconciled = reconcilePublicSnapshot(state, bootstrap.snapshot);
  const recent = [...bootstrap.recent_projections].sort((left, right) => left.sequence - right.sequence);
  if (recent.length > 0) {
    if (recent[recent.length - 1]!.sequence !== bootstrap.snapshot.high_water_sequence) {
      throw new PublicReconciliationError('bootstrap projections do not reach snapshot high-water sequence');
    }
    reconciled = {
      ...reconciled,
      observed_sequence: bootstrap.snapshot.high_water_sequence,
      items: retain(recent.map((item) => ({ ...item, record_type: 'projection' })), state.max_items),
    };
  } else {
    reconciled = { ...reconciled, observed_sequence: bootstrap.snapshot.high_water_sequence };
  }
  return {
    ...reconciled,
    proof_artifact_count: bootstrap.proof_catalog_summary.artifact_count,
    proof_total_byte_size: bootstrap.proof_catalog_summary.total_byte_size,
  };
}

export function applyPublicHistory(state: PublicMirrorState, history: PublicHistory): PublicMirrorState {
  if (state.protocol_version !== '' && history.protocol_version !== state.protocol_version) {
    throw new PublicReconciliationError('history protocol changed');
  }
  return appendItems({ ...state, protocol_version: history.protocol_version }, history.items);
}

export function applyPublicRecord(state: PublicMirrorState, record: PublicRecord): PublicMirrorState {
  let payload: unknown;
  try {
    payload = JSON.parse(record.record_bytes);
  } catch {
    throw new PublicReconciliationError('record bytes are not JSON');
  }
  if (typeof payload !== 'object' || payload === null || Array.isArray(payload)) {
    throw new PublicReconciliationError('record bytes must contain an object');
  }
  return appendItems(state, [{ ...(payload as Record<string, unknown>), sequence: record.sequence, record_type: record.record_type }]);
}
