// Feed-state classification. The UI must clearly distinguish:
//   - Feed offline: the mirror transport is unreachable
//   - No data: the feed is live but no records have been published
//   - No matching filters: records exist but none match the active filters
//   - Run failed: a terminal failure state for a specific run
//   - Metric unavailable: a specific metric was not observed or not applicable
// These are distinct product states, never collapsed into a single "empty".

import type { FreshnessState, LifecycleStatus, QualityState } from '../contract/types';

export type FeedConnectionState =
  | 'connecting'
  | 'live'
  | 'offline'
  | 'error';

/** SSE transport state from the public mirror stream. */
export type StreamConnectionState =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'reconnecting';

/** Mirror freshness windows — kept in sync with PublicFeedFreshness*Seconds. */
export const FRESHNESS_DELAYED_MS = 60_000;
export const FRESHNESS_STALE_MS = 300_000;
export const FRESHNESS_OFFLINE_MS = 900_000;

export interface FeedStatus {
  connection: FeedConnectionState;
  freshness: FreshnessState | 'unknown';
  /** Snapshot-pinned freshness that must not be overridden by age heuristics. */
  pinnedFreshness?: FreshnessState;
  highWaterSequence: number;
  lastAcceptedAt: string | undefined;
  message: string;
}

/** Derive mirror freshness from the last accepted record timestamp. */
export function deriveFreshness(
  lastAcceptedAt: string | undefined,
  pinnedFreshness?: FreshnessState,
  nowMs: number = Date.now(),
): FreshnessState | 'unknown' {
  if (pinnedFreshness === 'intentionally_stopped' || pinnedFreshness === 'safety_stopped') {
    return pinnedFreshness;
  }
  if (!lastAcceptedAt) return 'unknown';
  const acceptedAt = new Date(lastAcceptedAt).getTime();
  if (!Number.isFinite(acceptedAt)) return 'unknown';
  const age = nowMs - acceptedAt;
  if (age >= FRESHNESS_OFFLINE_MS) return 'source_offline';
  if (age >= FRESHNESS_STALE_MS) return 'stale';
  if (age >= FRESHNESS_DELAYED_MS) return 'delayed';
  return 'active';
}

export function effectiveFeedFreshness(
  feedStatus: FeedStatus,
  nowMs: number = Date.now(),
): FreshnessState | 'unknown' {
  return deriveFreshness(feedStatus.lastAcceptedAt, feedStatus.pinnedFreshness, nowMs);
}

export function classifyStreamConnection(
  streamConnection: StreamConnectionState,
  feedConnection: FeedConnectionState,
): { label: string; tone: 'ok' | 'warn' | 'critical'; description: string } {
  if (feedConnection === 'offline' || feedConnection === 'error') {
    return {
      label: 'Offline',
      tone: 'warn',
      description: 'The mirror stream is unreachable. Last accepted data remains visible.',
    };
  }
  switch (streamConnection) {
    case 'connected':
      return {
        label: 'Live',
        tone: 'ok',
        description: 'Streaming live updates via SSE.',
      };
    case 'connecting':
      return {
        label: 'Connecting',
        tone: 'warn',
        description: 'Opening the SSE stream.',
      };
    case 'reconnecting':
      return {
        label: 'Reconnecting',
        tone: 'warn',
        description: 'The SSE stream is reconnecting.',
      };
    case 'disconnected':
      return feedConnection === 'connecting' || feedConnection === 'live'
        ? {
            label: 'Connecting',
            tone: 'warn',
            description: 'Opening the SSE stream.',
          }
        : {
            label: 'Offline',
            tone: 'warn',
            description: 'The SSE stream is not connected.',
          };
  }
}

export function classifyFreshness(freshness: FreshnessState | 'unknown'): {
  label: string;
  tone: 'ok' | 'warn' | 'critical';
  description: string;
} {
  switch (freshness) {
    case 'active':
      return { label: 'Active', tone: 'ok', description: 'The mirror is delivering public records.' };
    case 'delayed':
      return { label: 'Delayed', tone: 'warn', description: 'Records are arriving but slower than expected.' };
    case 'stale':
      return { label: 'Inactive', tone: 'warn', description: 'No recent updates; last accepted data remains visible.' };
    case 'intentionally_stopped':
      return { label: 'Intentionally stopped', tone: 'warn', description: 'The feed was stopped by the source.' };
    case 'safety_stopped':
      return { label: 'Safety stopped', tone: 'critical', description: 'The feed was stopped for a safety reason.' };
    case 'source_offline':
      return { label: 'Source offline', tone: 'critical', description: 'The mirror source is unreachable.' };
    case 'unknown':
      return { label: 'Connecting', tone: 'warn', description: 'Connecting to the public mirror.' };
  }
}

export function isTerminalFailure(status: LifecycleStatus): boolean {
  return status === 'failed' || status === 'stopped';
}

export function qualityStateLabel(state: QualityState): string {
  switch (state) {
    case 'verified_public':
      return 'Verified public';
    case 'exploratory_verified':
      return 'Exploratory · verifier passed';
    case 'exploratory_partial':
      return 'Exploratory · partial';
    case 'live_in_progress':
      return 'Live · in progress';
    case 'terminal_failed':
      return 'Terminal · failed';
    case 'dead_evidence':
      return 'Dead evidence';
    case 'not_evaluated':
      return 'Not evaluated';
    case 'unavailable':
      return 'Unavailable';
  }
}

export function qualityStateTone(state: QualityState): 'ok' | 'info' | 'warn' | 'critical' | 'neutral' {
  switch (state) {
    case 'verified_public':
      return 'ok';
    case 'exploratory_verified':
      return 'info';
    case 'exploratory_partial':
      return 'warn';
    case 'live_in_progress':
      return 'info';
    case 'terminal_failed':
      return 'critical';
    case 'dead_evidence':
      return 'critical';
    case 'not_evaluated':
      return 'neutral';
    case 'unavailable':
      return 'neutral';
  }
}

export function emptyStateReason(
  hasRecords: boolean,
  hasFilters: boolean,
  connection: FeedConnectionState,
): { kind: 'no-data' | 'no-matching-filters' | 'feed-offline'; message: string } {
  if (connection === 'offline' || connection === 'error') {
    return {
      kind: 'feed-offline',
      message: 'The public mirror is unreachable. Last accepted data remains visible below.',
    };
  }
  if (!hasRecords) {
    return { kind: 'no-data', message: 'No public records have been published for this dataset yet.' };
  }
  if (hasFilters) {
    return { kind: 'no-matching-filters', message: 'No records match the active filters. Adjust filters to see more.' };
  }
  return { kind: 'no-data', message: 'No records available.' };
}
