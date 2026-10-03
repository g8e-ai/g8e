// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Feed-state classification. The UI must clearly distinguish:
//   - Feed offline: the mirror transport is unreachable
//   - No data: the feed is live but no records have been published
//   - No matching filters: records exist but none match the active filters
//   - Run failed: a terminal failure state for a specific run
//   - Metric unavailable: a specific metric was not observed or not applicable
// These are distinct product states, never collapsed into a single "empty".

import type { FreshnessState, LifecycleStatus, LiveEventKind, QualityState } from '../contract/types';

/** Run-level live events may advance evaluation_summary quality; assignment events must not. */
const RUN_LEVEL_QUALITY_EVENT_KINDS = new Set<LiveEventKind>([
  'evaluation_queued',
  'evaluation_started',
  'evaluation_completed',
  'evaluation_failed',
  'evaluation_stopped',
]);

const QUALITY_STATE_RANK: Record<QualityState, number> = {
  verified_public: 7,
  exploratory_verified: 6,
  exploratory_partial: 5,
  legacy_unverified: 4,
  live_in_progress: 3,
  terminal_failed: 2,
  dead_evidence: 1,
  not_evaluated: 0,
  unavailable: -1,
};

export function isRunLevelQualityEvent(kind: LiveEventKind): boolean {
  return RUN_LEVEL_QUALITY_EVENT_KINDS.has(kind);
}

/** Keep the higher-trust quality label when reconciling live event updates. */
export function mergeQualityState(current: QualityState, candidate: QualityState): QualityState {
  return QUALITY_STATE_RANK[candidate] > QUALITY_STATE_RANK[current] ? candidate : current;
}

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
      return { label: 'Offline', tone: 'critical', description: 'The mirror source is unreachable.' };
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
      return 'Current-standard verified';
    case 'exploratory_verified':
      return 'Run-scoped verification passed';
    case 'exploratory_partial':
      return 'Not fully verified';
    case 'legacy_unverified':
      return 'Legacy · not current-standard verified';
    case 'live_in_progress':
      return 'In progress · unverified';
    case 'terminal_failed':
      return 'Failed · unverified';
    case 'dead_evidence':
      return 'Evidence invalid';
    case 'not_evaluated':
      return 'Not evaluated';
    case 'unavailable':
      return 'Unavailable';
  }
}

export interface QualityFilterOption {
  value: QualityState;
  label: string;
}

/** Quality states exposed by the Models view filter, in preferred display order. */
export const MODEL_QUALITY_FILTER_STATES: readonly QualityState[] = [
  'verified_public',
  'exploratory_verified',
  'exploratory_partial',
  'legacy_unverified',
  'live_in_progress',
  'not_evaluated',
];

/** Quality states exposed by the Evaluations view filter, in preferred display order. */
export const EVALUATION_QUALITY_FILTER_STATES: readonly QualityState[] = [
  'verified_public',
  'exploratory_verified',
  'exploratory_partial',
  'legacy_unverified',
  'live_in_progress',
  'terminal_failed',
  'dead_evidence',
  'not_evaluated',
  'unavailable',
];

/** Returns filter options for quality states that appear in the current records. */
export function availableQualityFilterOptions(
  records: Array<{ quality_state: QualityState }>,
  candidates: readonly QualityState[],
): QualityFilterOption[] {
  const present = new Set(records.map((record) => record.quality_state));
  const rank = (state: QualityState) => QUALITY_STATE_RANK[state] ?? 0;
  return candidates
    .filter((state) => present.has(state))
    .sort((left, right) => rank(right) - rank(left))
    .map((state) => ({ value: state, label: qualityStateLabel(state) }));
}

/** Drops a stale quality selection when the current dataset no longer contains it. */
export function normalizeQualityFilter(selected: string, options: QualityFilterOption[]): string {
  if (selected === 'all') return 'all';
  return options.some((option) => option.value === selected) ? selected : 'all';
}

export function qualityStateTone(state: QualityState): 'ok' | 'info' | 'warn' | 'critical' | 'neutral' {
  switch (state) {
    case 'verified_public':
      return 'ok';
    case 'exploratory_verified':
      return 'info';
    case 'exploratory_partial':
      return 'warn';
    case 'legacy_unverified':
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
