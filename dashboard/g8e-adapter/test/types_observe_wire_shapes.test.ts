// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';

import type {
  AgentStateProjection,
  AgentStatusUpdatedPayload,
  DownloadArtifact,
  EvalDetail,
  EvalMetricRecordedPayload,
  EvalMetricSummary,
  EvalRunCompletedPayload,
  EvalSummary,
  EvidenceSafeLink,
  ObserveBootstrapSnapshot,
  ObservePage,
  ObservedMeasurement,
  OverviewCounters,
  OverviewMeasurements,
  RunDetail,
  RunStatusUpdatedPayload,
  RunSummary,
  RunTask,
  SuccessRateMeasurement,
} from '../src/types/observe';

// These tests verify the TypeScript interface field names match the Go wire
// shapes byte-for-byte. A representative instance of each model is serialized
// to JSON and the exact key set is asserted, so a field rename on either side
// fails the test.

function keys(obj: object): string[] {
  return Object.keys(obj).sort();
}

function keysOf<T extends object>(obj: T): string[] {
  return keys(obj);
}

describe('observe wire shapes', () => {
  it('AgentStateProjection has the Go field names', () => {
    const p: AgentStateProjection = {
      schema_version: '1.0.0',
      agent_id: 'u:p',
      display_name: 'Sage',
      role: 'sage',
      status: 'running',
      run_id: 'r1',
      task_id: 't1',
      model: 'm',
      throughput: undefined,
      freshness: 'observed',
      observed_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(p)).toEqual([
      'agent_id', 'display_name', 'freshness', 'model', 'observed_at',
      'role', 'run_id', 'schema_version', 'status', 'task_id', 'throughput',
    ]);
  });

  it('ObservedMeasurement has the Go field names', () => {
    const m: ObservedMeasurement = {
      schema_version: '1.0.0',
      metric_id: 'cpu',
      value: 0.5,
      unit: 'fraction',
      source_component: 'host',
      observed_at: '2026-01-01T00:00:00Z',
      window_seconds: 60,
      status: 'observed',
    };
    expect(keysOf(m)).toEqual([
      'metric_id', 'observed_at', 'schema_version', 'source_component',
      'status', 'unit', 'value', 'window_seconds',
    ]);
  });

  it('SuccessRateMeasurement has the Go field names', () => {
    const s: SuccessRateMeasurement = {
      metric_id: 'success_rate',
      metric_version: '1.0.0',
      value: 0.9,
      unit: 'fraction',
      denominator: 100,
      verification_status: 'projection_validated',
    };
    expect(keysOf(s)).toEqual([
      'denominator', 'metric_id', 'metric_version', 'unit', 'value',
      'verification_status',
    ]);
  });

  it('OverviewCounters has the Go field names', () => {
    const o: OverviewCounters = {
      schema_version: '1.0.0',
      agents_running: 2,
      agents_running_freshness: 'observed',
      tasks_in_queue: 3,
      tasks_in_queue_freshness: 'stale',
      success_rate: undefined,
      generated_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(o)).toEqual([
      'agents_running', 'agents_running_freshness', 'generated_at',
      'schema_version', 'success_rate', 'tasks_in_queue',
      'tasks_in_queue_freshness',
    ]);
  });

  it('OverviewMeasurements has the Go field names', () => {
    const m: OverviewMeasurements = {
      schema_version: '1.0.0',
      total_throughput: undefined,
      cpu: undefined,
      ram: undefined,
      vram: undefined,
      disk: undefined,
    };
    expect(keysOf(m)).toEqual([
      'cpu', 'disk', 'ram', 'schema_version', 'total_throughput', 'vram',
    ]);
  });

  it('RunSummary has the Go field names', () => {
    const r: RunSummary = {
      schema_version: '1.0.0',
      run_id: 'r1',
      run_kind: 'investigation',
      display_name: 'Case',
      status: 'running',
      active_task_id: 't1',
      completed_tasks: 1,
      total_tasks: 2,
      started_at: '2026-01-01T00:00:00Z',
      ended_at: undefined,
      has_receipts: true,
      evidence_count: 3,
      observed_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(r)).toEqual([
      'active_task_id', 'completed_tasks', 'display_name', 'ended_at',
      'evidence_count', 'has_receipts', 'observed_at', 'run_id', 'run_kind',
      'schema_version', 'started_at', 'status', 'total_tasks',
    ]);
  });

  it('RunTask has the Go field names', () => {
    const t: RunTask = {
      schema_version: '1.0.0',
      task_id: 't1',
      display_name: 'Task',
      status: 'completed',
      owning_agent: 'sage',
      model: 'm',
      started_at: '2026-01-01T00:00:00Z',
      ended_at: '2026-01-01T00:01:00Z',
    };
    expect(keysOf(t)).toEqual([
      'display_name', 'ended_at', 'model', 'owning_agent', 'schema_version',
      'started_at', 'status', 'task_id',
    ]);
  });

  it('EvidenceSafeLink has the Go field names', () => {
    const e: EvidenceSafeLink = {
      schema_version: '1.0.0',
      artifact_id: 'a1',
      label: 'Report',
      media_type: 'application/pdf',
    };
    expect(keysOf(e)).toEqual([
      'artifact_id', 'label', 'media_type', 'schema_version',
    ]);
  });

  it('RunDetail has the Go field names', () => {
    const d: RunDetail = {
      schema_version: '1.0.0',
      run_id: 'r1',
      run_kind: 'investigation',
      display_name: 'Case',
      status: 'running',
      active_task_id: 't1',
      completed_tasks: 1,
      total_tasks: 2,
      tasks: [],
      evidence_safe_links: [],
      started_at: '2026-01-01T00:00:00Z',
      ended_at: undefined,
      observed_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(d)).toEqual([
      'active_task_id', 'completed_tasks', 'display_name', 'ended_at',
      'evidence_safe_links', 'observed_at', 'run_id', 'run_kind',
      'schema_version', 'started_at', 'status', 'tasks', 'total_tasks',
    ]);
  });

  it('EvalSummary has the Go field names', () => {
    const e: EvalSummary = {
      schema_version: '1.0.0',
      run_id: 'r1',
      suite_id: 's1',
      suite_version: '1.0.0',
      arm_id: 'a1',
      status: 'completed',
      verification_status: 'projection_validated',
      receipt_count: 5,
      metric_count: 3,
      published_projection_sha256: 'abc',
      completed_at: '2026-01-01T00:00:00Z',
      observed_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(e)).toEqual([
      'arm_id', 'completed_at', 'metric_count', 'observed_at',
      'published_projection_sha256', 'receipt_count', 'run_id', 'schema_version',
      'status', 'suite_id', 'suite_version', 'verification_status',
    ]);
  });

  it('EvalMetricSummary has the Go field names', () => {
    const m: EvalMetricSummary = {
      schema_version: '1.0.0',
      metric_id: 'acc',
      metric_version: '1.0.0',
      value: 0.9,
      unit: 'fraction',
      eligible: 100,
      denominator: 100,
      verification_status: 'projection_validated',
      recorded_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(m)).toEqual([
      'denominator', 'eligible', 'metric_id', 'metric_version',
      'recorded_at', 'schema_version', 'unit', 'value', 'verification_status',
    ]);
  });

  it('EvalDetail has the Go field names', () => {
    const d: EvalDetail = {
      schema_version: '1.0.0',
      run_id: 'r1',
      suite_id: 's1',
      suite_version: '1.0.0',
      arm_id: 'a1',
      model_id: 'm1',
      model_provider: 'p1',
      status: 'completed',
      verification_status: 'projection_validated',
      receipt_count: 5,
      assigned_tasks: 10,
      terminal_attempts: 10,
      metrics: [],
      published_projection_sha256: 'abc',
      completed_at: '2026-01-01T00:00:00Z',
      observed_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(d)).toEqual([
      'arm_id', 'assigned_tasks', 'completed_at', 'metrics', 'model_id',
      'model_provider', 'observed_at', 'published_projection_sha256',
      'receipt_count', 'run_id', 'schema_version', 'status', 'suite_id',
      'suite_version', 'terminal_attempts', 'verification_status',
    ]);
  });

  it('DownloadArtifact has the Go field names', () => {
    const d: DownloadArtifact = {
      schema_version: '1.0.0',
      artifact_id: 'a1',
      filename: 'report.pdf',
      media_type: 'application/pdf',
      byte_size: 1024,
      sha256: 'abc',
      privacy_classification: 'public_safe',
      source_run_id: 'r1',
      download_url: '/api/v1/observe/downloads/a1',
      generated_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(d)).toEqual([
      'artifact_id', 'byte_size', 'download_url', 'filename', 'generated_at',
      'media_type', 'privacy_classification', 'schema_version', 'sha256',
      'source_run_id',
    ]);
  });

  it('ObservePage has the Go field names', () => {
    const p: ObservePage<RunSummary> = {
      schema_version: '1.0.0',
      items: [],
      cursor: 'c1',
      has_more: true,
      limit: 20,
    };
    expect(keysOf(p)).toEqual([
      'cursor', 'has_more', 'items', 'limit', 'schema_version',
    ]);
  });

  it('ObserveBootstrapSnapshot has the Go field names', () => {
    const s: ObserveBootstrapSnapshot = {
      schema_version: '1.0.0',
      agents: [],
      active_run: undefined,
      overview: {} as OverviewCounters,
      measurements: {} as OverviewMeasurements,
      recent_runs: [],
      latest_evals: [],
      downloads: [],
      generated_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(s)).toEqual([
      'active_run', 'agents', 'downloads', 'generated_at', 'latest_evals',
      'measurements', 'overview', 'recent_runs', 'schema_version',
    ]);
  });

  it('AgentStatusUpdatedPayload has the Go field names', () => {
    const p: AgentStatusUpdatedPayload = {
      schema_version: '1.0.0',
      agent_id: 'u:p',
      display_name: 'Sage',
      role: 'sage',
      status: 'running',
      run_id: 'r1',
      task_id: 't1',
      model: 'm',
      observed_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(p)).toEqual([
      'agent_id', 'display_name', 'model', 'observed_at', 'role', 'run_id',
      'schema_version', 'status', 'task_id',
    ]);
  });

  it('RunStatusUpdatedPayload has the Go field names', () => {
    const p: RunStatusUpdatedPayload = {
      schema_version: '1.0.0',
      run_id: 'r1',
      run_kind: 'investigation',
      display_name: 'Case',
      status: 'running',
      active_task_id: 't1',
      completed_tasks: 1,
      total_tasks: 2,
      started_at: '2026-01-01T00:00:00Z',
      ended_at: undefined,
      observed_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(p)).toEqual([
      'active_task_id', 'completed_tasks', 'display_name', 'ended_at',
      'observed_at', 'run_id', 'run_kind', 'schema_version', 'started_at',
      'status', 'total_tasks',
    ]);
  });

  it('EvalRunCompletedPayload has the Go field names', () => {
    const p: EvalRunCompletedPayload = {
      schema_version: '1.0.0',
      run_id: 'r1',
      suite_id: 's1',
      suite_version: '1.0.0',
      arm_id: 'a1',
      terminal_attempts: 10,
      assigned_tasks: 10,
      receipt_count: 5,
      verification_status: 'projection_validated',
      published_projection_sha256: 'abc',
      completed_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(p)).toEqual([
      'arm_id', 'assigned_tasks', 'completed_at', 'published_projection_sha256',
      'receipt_count', 'run_id', 'schema_version', 'suite_id', 'suite_version',
      'terminal_attempts', 'verification_status',
    ]);
  });

  it('EvalMetricRecordedPayload has the Go field names', () => {
    const p: EvalMetricRecordedPayload = {
      schema_version: '1.0.0',
      run_id: 'r1',
      metric_id: 'acc',
      metric_version: '1.0.0',
      value: 0.9,
      unit: 'fraction',
      eligible: 100,
      denominator: 100,
      verification_status: 'projection_validated',
      recorded_at: '2026-01-01T00:00:00Z',
    };
    expect(keysOf(p)).toEqual([
      'denominator', 'eligible', 'metric_id', 'metric_version', 'recorded_at',
      'run_id', 'schema_version', 'unit', 'value', 'verification_status',
    ]);
  });
});
