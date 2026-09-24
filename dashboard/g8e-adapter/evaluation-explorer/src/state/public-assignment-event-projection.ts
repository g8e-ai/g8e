// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Disclosure-safe projection from campaign-side assignment signals to public
// live events. Provider-boundary observe fields (served_model_tag, backend_name,
// exact quantization) are intentionally excluded; terminal assignment_result
// projections carry categorical activity summaries instead.

import {
  VIEW_SCHEMA_VERSION,
  type LiveEvent,
  type MetricValue,
  type ModelRole,
} from '../contract/types';
import { CAMPAIGN_SOURCE_REVISION, campaignDatasetId } from './campaign-adapter';

/** Public-safe model-role invocation signal (no provider-boundary fields). */
export interface PublicModelRoleInvocationSignal {
  run_id: string;
  assignment_id: string;
  variant_id: string;
  role: ModelRole;
  task_id?: string;
  observed_at: string;
  event_id: string;
  completed: number;
  total: number;
}

/** Public-safe per-assignment metric availability signal. */
export interface PublicMetricAvailabilitySignal {
  run_id: string;
  assignment_id: string;
  variant_id: string;
  metric_id: string;
  numerator: number;
  denominator: number;
  rate?: number;
  observed_at: string;
  event_id: string;
  completed: number;
  total: number;
}

const HEADLINE_METRIC_IDS = new Set(['pass_rate', 'latency_p50_ms', 'output_throughput_p50_tokens_per_second']);

export function isHeadlineMetricId(metricId: string): boolean {
  return HEADLINE_METRIC_IDS.has(metricId);
}

export function projectModelRoleInvocationToLiveEvent(signal: PublicModelRoleInvocationSignal): LiveEvent {
  const stageParts = ['model role invoked', signal.role, signal.variant_id];
  if (signal.task_id) stageParts.push(signal.task_id);
  return {
    schema_version: VIEW_SCHEMA_VERSION,
    kind: 'stage_updated',
    dataset_id: campaignDatasetId(signal.run_id),
    quality_state: 'live_in_progress',
    observed_at: signal.observed_at,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    event_id: signal.event_id,
    run_id: signal.run_id,
    assignment_id: signal.assignment_id,
    task_id: signal.task_id,
    variant_id: signal.variant_id,
    role: signal.role,
    lifecycle_status: 'running',
    completed: signal.completed,
    total: signal.total,
    stage_label: stageParts.join(' · '),
  };
}

export function projectMetricAvailabilityToLiveEvent(signal: PublicMetricAvailabilitySignal): LiveEvent {
  const metricKey = signal.metric_id;
  let metricValue: MetricValue;
  if (signal.denominator <= 0) {
    metricValue = { unavailable_reason: 'no_scored_calls' };
  } else if (signal.rate !== undefined && Number.isFinite(signal.rate)) {
    metricValue = { value: signal.rate };
  } else if (signal.denominator > 0) {
    metricValue = { value: signal.numerator / signal.denominator };
  } else {
    metricValue = { unavailable_reason: 'no_scored_calls' };
  }

  return {
    schema_version: VIEW_SCHEMA_VERSION,
    kind: 'metric_updated',
    dataset_id: campaignDatasetId(signal.run_id),
    quality_state: 'live_in_progress',
    observed_at: signal.observed_at,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    event_id: signal.event_id,
    run_id: signal.run_id,
    assignment_id: signal.assignment_id,
    variant_id: signal.variant_id,
    lifecycle_status: 'running',
    completed: signal.completed,
    total: signal.total,
    metric_delta: { [metricKey]: metricValue },
  };
}
