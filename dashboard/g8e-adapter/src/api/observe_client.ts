// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Observe API client. Credentialed reads against the allowlisted observe GET
// endpoints with stable cursor pagination and typed response validation.
// Every fetch uses the credentialed fetch helper, so credentials: 'include'
// and the endpoint allowlist are enforced. This module adds typed response
// validation so a malformed or disclosure-violating response cannot reach the
// state stores.

import type { CredentialedFetch, GatewayResponse } from './fetch';
import {
  isAgentLifecycleStatus,
  isDownloadPrivacyClassification,
  isEvalVerificationStatus,
  isMeasurementStatus,
  isRunKind,
  isRunLifecycleStatus,
  isSnapshotFreshness,
} from '../types/enums';
import type {
  AgentStateProjection,
  DownloadArtifact,
  EvalDetail,
  EvalMetricSummary,
  EvalSummary,
  EvidenceSafeLink,
  ObservedMeasurement,
  ObserveBootstrapSnapshot,
  ObservePage,
  OverviewCounters,
  OverviewMeasurements,
  RunDetail,
  RunSummary,
  RunTask,
  SuccessRateMeasurement,
} from '../types/observe';

export class ObserveResponseValidationError extends Error {
  constructor(
    public readonly endpoint: string,
    message: string,
  ) {
    super(`observe ${endpoint}: ${message}`);
    this.name = 'ObserveResponseValidationError';
  }
}

export interface PageOptions {
  cursor?: string;
  limit?: number;
}

export interface ObserveClient {
  getBootstrap(): Promise<ObserveBootstrapSnapshot>;
  getRuns(opts?: PageOptions): Promise<ObservePage<RunSummary>>;
  getRunDetail(runId: string): Promise<RunDetail>;
  getEvals(opts?: PageOptions): Promise<ObservePage<EvalSummary>>;
  getEvalDetail(runId: string): Promise<EvalDetail>;
  getDownloads(opts?: PageOptions): Promise<ObservePage<DownloadArtifact>>;
  getDownloadDetail(artifactId: string): Promise<DownloadArtifact>;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function requireString(obj: Record<string, unknown>, field: string, endpoint: string): string {
  const v = obj[field];
  if (typeof v !== 'string') {
    throw new ObserveResponseValidationError(endpoint, `missing or non-string field: ${field}`);
  }
  return v;
}

function requireNumber(obj: Record<string, unknown>, field: string, endpoint: string): number {
  const v = obj[field];
  if (typeof v !== 'number' || !Number.isFinite(v)) {
    throw new ObserveResponseValidationError(endpoint, `missing or non-finite number field: ${field}`);
  }
  return v;
}

function requireBoolean(obj: Record<string, unknown>, field: string, endpoint: string): boolean {
  const v = obj[field];
  if (typeof v !== 'boolean') {
    throw new ObserveResponseValidationError(endpoint, `missing or non-boolean field: ${field}`);
  }
  return v;
}

function optionalString(obj: Record<string, unknown>, field: string): string | undefined {
  const v = obj[field];
  return typeof v === 'string' ? v : undefined;
}

function optionalNumber(obj: Record<string, unknown>, field: string): number | undefined {
  const v = obj[field];
  return typeof v === 'number' && Number.isFinite(v) ? v : undefined;
}

function requireObject(value: unknown, endpoint: string, label: string): Record<string, unknown> {
  if (!isObject(value)) {
    throw new ObserveResponseValidationError(endpoint, `${label} is not an object`);
  }
  return value;
}

function requireArray(value: unknown, endpoint: string, label: string): unknown[] {
  if (!Array.isArray(value)) {
    throw new ObserveResponseValidationError(endpoint, `${label} is not an array`);
  }
  return value;
}

function requireEnum<T extends string>(
  obj: Record<string, unknown>,
  field: string,
  endpoint: string,
  guard: (v: unknown) => v is T,
): T {
  const v = obj[field];
  if (!guard(v)) {
    throw new ObserveResponseValidationError(endpoint, `missing or invalid enum field: ${field}`);
  }
  return v;
}

// Validate that a response body is an object and return it, or throw.
function requireBody(res: GatewayResponse<unknown>, endpoint: string): Record<string, unknown> {
  if (!res.ok) {
    throw new ObserveResponseValidationError(endpoint, `HTTP ${res.status}`);
  }
  if (res.body === undefined) {
    throw new ObserveResponseValidationError(endpoint, 'empty response body');
  }
  return requireObject(res.body, endpoint, 'response body');
}

// Build a cursor query string from page options. The cursor is opaque to the
// client; it is passed through unchanged.
function pageQuery(opts: PageOptions | undefined): string {
  const params = new URLSearchParams();
  if (opts?.cursor) params.set('cursor', opts.cursor);
  if (opts?.limit !== undefined) params.set('limit', String(opts.limit));
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export function createObserveClient(fetchImpl: CredentialedFetch): ObserveClient {
  return {
    async getBootstrap(): Promise<ObserveBootstrapSnapshot> {
      const res = await fetchImpl('GET', '/api/v1/observe/bootstrap');
      const body = requireBody(res, 'bootstrap');
      return validateBootstrap(body);
    },

    async getRuns(opts?: PageOptions): Promise<ObservePage<RunSummary>> {
      const res = await fetchImpl('GET', `/api/v1/observe/runs${pageQuery(opts)}`);
      const body = requireBody(res, 'runs');
      return validateRunPage(body);
    },

    async getRunDetail(runId: string): Promise<RunDetail> {
      const res = await fetchImpl('GET', `/api/v1/observe/runs/${encodeURIComponent(runId)}`);
      const body = requireBody(res, 'run_detail');
      return validateRunDetail(body);
    },

    async getEvals(opts?: PageOptions): Promise<ObservePage<EvalSummary>> {
      const res = await fetchImpl('GET', `/api/v1/observe/evals${pageQuery(opts)}`);
      const body = requireBody(res, 'evals');
      return validateEvalPage(body);
    },

    async getEvalDetail(runId: string): Promise<EvalDetail> {
      const res = await fetchImpl('GET', `/api/v1/observe/evals/${encodeURIComponent(runId)}`);
      const body = requireBody(res, 'eval_detail');
      return validateEvalDetail(body);
    },

    async getDownloads(opts?: PageOptions): Promise<ObservePage<DownloadArtifact>> {
      const res = await fetchImpl('GET', `/api/v1/observe/downloads${pageQuery(opts)}`);
      const body = requireBody(res, 'downloads');
      return validateDownloadPage(body);
    },

    async getDownloadDetail(artifactId: string): Promise<DownloadArtifact> {
      const res = await fetchImpl('GET', `/api/v1/observe/downloads/${encodeURIComponent(artifactId)}`);
      const body = requireBody(res, 'download_detail');
      return validateDownloadArtifact(body, 'download_detail');
    },
  };
}

// --- Validators ---

function validateBootstrap(body: Record<string, unknown>): ObserveBootstrapSnapshot {
  requireString(body, 'schema_version', 'bootstrap');
  requireString(body, 'generated_at', 'bootstrap');
  const agents = requireArray(body.agents, 'bootstrap', 'agents').map((a) =>
    validateAgentState(a, 'bootstrap'),
  );
  const overview = validateOverviewCounters(body.overview, 'bootstrap');
  const measurements = validateOverviewMeasurements(body.measurements, 'bootstrap');
  const recentRuns = requireArray(body.recent_runs, 'bootstrap', 'recent_runs').map((r) =>
    validateRunSummary(r, 'bootstrap'),
  );
  const latestEvals = requireArray(body.latest_evals, 'bootstrap', 'latest_evals').map((e) =>
    validateEvalSummary(e, 'bootstrap'),
  );
  const downloads = requireArray(body.downloads, 'bootstrap', 'downloads').map((d) =>
    validateDownloadArtifact(d, 'bootstrap'),
  );
  const activeRun = body.active_run !== undefined && body.active_run !== null
    ? validateRunSummary(body.active_run, 'bootstrap')
    : undefined;
  return {
    schema_version: body.schema_version as string,
    agents,
    active_run: activeRun,
    overview,
    measurements,
    recent_runs: recentRuns,
    latest_evals: latestEvals,
    downloads,
    generated_at: body.generated_at as string,
  };
}

function validateOverviewCounters(value: unknown, endpoint: string): OverviewCounters {
  const obj = requireObject(value, endpoint, 'overview');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    agents_running: requireNumber(obj, 'agents_running', endpoint),
    agents_running_freshness: requireEnum(obj, 'agents_running_freshness', endpoint, isSnapshotFreshness),
    tasks_in_queue: requireNumber(obj, 'tasks_in_queue', endpoint),
    tasks_in_queue_freshness: requireEnum(obj, 'tasks_in_queue_freshness', endpoint, isSnapshotFreshness),
    success_rate: obj.success_rate !== undefined && obj.success_rate !== null
      ? validateSuccessRate(obj.success_rate, endpoint)
      : undefined,
    generated_at: requireString(obj, 'generated_at', endpoint),
  };
}

function validateSuccessRate(value: unknown, endpoint: string): SuccessRateMeasurement {
  const obj = requireObject(value, endpoint, 'success_rate');
  return {
    metric_id: requireString(obj, 'metric_id', endpoint),
    metric_version: requireString(obj, 'metric_version', endpoint),
    value: requireNumber(obj, 'value', endpoint),
    unit: requireString(obj, 'unit', endpoint),
    denominator: requireNumber(obj, 'denominator', endpoint),
    verification_status: requireEnum(obj, 'verification_status', endpoint, isEvalVerificationStatus),
  };
}

function validateOverviewMeasurements(value: unknown, endpoint: string): OverviewMeasurements {
  const obj = requireObject(value, endpoint, 'measurements');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    total_throughput: obj.total_throughput !== undefined && obj.total_throughput !== null
      ? validateObservedMeasurement(obj.total_throughput, endpoint)
      : undefined,
    cpu: obj.cpu !== undefined && obj.cpu !== null ? validateObservedMeasurement(obj.cpu, endpoint) : undefined,
    ram: obj.ram !== undefined && obj.ram !== null ? validateObservedMeasurement(obj.ram, endpoint) : undefined,
    vram: obj.vram !== undefined && obj.vram !== null ? validateObservedMeasurement(obj.vram, endpoint) : undefined,
    disk: obj.disk !== undefined && obj.disk !== null ? validateObservedMeasurement(obj.disk, endpoint) : undefined,
  };
}

function validateObservedMeasurement(value: unknown, endpoint: string): ObservedMeasurement {
  const obj = requireObject(value, endpoint, 'measurement');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    metric_id: requireString(obj, 'metric_id', endpoint),
    value: requireNumber(obj, 'value', endpoint),
    unit: requireString(obj, 'unit', endpoint),
    source_component: requireString(obj, 'source_component', endpoint),
    observed_at: requireString(obj, 'observed_at', endpoint),
    window_seconds: optionalNumber(obj, 'window_seconds'),
    status: requireEnum(obj, 'status', endpoint, isMeasurementStatus),
  };
}

function validateAgentState(value: unknown, endpoint: string): AgentStateProjection {
  const obj = requireObject(value, endpoint, 'agent');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    agent_id: requireString(obj, 'agent_id', endpoint),
    display_name: requireString(obj, 'display_name', endpoint),
    role: requireString(obj, 'role', endpoint),
    status: requireEnum(obj, 'status', endpoint, isAgentLifecycleStatus),
    run_id: optionalString(obj, 'run_id'),
    task_id: optionalString(obj, 'task_id'),
    model: optionalString(obj, 'model'),
    throughput: obj.throughput !== undefined && obj.throughput !== null
      ? validateObservedMeasurement(obj.throughput, endpoint)
      : undefined,
    freshness: requireEnum(obj, 'freshness', endpoint, isSnapshotFreshness),
    observed_at: requireString(obj, 'observed_at', endpoint),
  };
}

function validateRunSummary(value: unknown, endpoint: string): RunSummary {
  const obj = requireObject(value, endpoint, 'run_summary');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    run_id: requireString(obj, 'run_id', endpoint),
    run_kind: requireEnum(obj, 'run_kind', endpoint, isRunKind),
    display_name: requireString(obj, 'display_name', endpoint),
    status: requireEnum(obj, 'status', endpoint, isRunLifecycleStatus),
    active_task_id: optionalString(obj, 'active_task_id'),
    completed_tasks: requireNumber(obj, 'completed_tasks', endpoint),
    total_tasks: requireNumber(obj, 'total_tasks', endpoint),
    started_at: optionalString(obj, 'started_at'),
    ended_at: optionalString(obj, 'ended_at'),
    has_receipts: requireBoolean(obj, 'has_receipts', endpoint),
    evidence_count: requireNumber(obj, 'evidence_count', endpoint),
    observed_at: requireString(obj, 'observed_at', endpoint),
  };
}

function validateRunPage(body: Record<string, unknown>): ObservePage<RunSummary> {
  requireString(body, 'schema_version', 'runs');
  const items = requireArray(body.items, 'runs', 'items').map((r) => validateRunSummary(r, 'runs'));
  return {
    schema_version: body.schema_version as string,
    items,
    cursor: optionalString(body, 'cursor'),
    has_more: requireBoolean(body, 'has_more', 'runs'),
    limit: requireNumber(body, 'limit', 'runs'),
  };
}

function validateRunDetail(body: Record<string, unknown>): RunDetail {
  requireString(body, 'schema_version', 'run_detail');
  requireString(body, 'run_id', 'run_detail');
  requireString(body, 'display_name', 'run_detail');
  requireString(body, 'observed_at', 'run_detail');
  requireNumber(body, 'completed_tasks', 'run_detail');
  requireNumber(body, 'total_tasks', 'run_detail');
  const tasks = requireArray(body.tasks, 'run_detail', 'tasks').map((t) => validateRunTask(t, 'run_detail'));
  const evidence = requireArray(body.evidence_safe_links, 'run_detail', 'evidence_safe_links').map((e) =>
    validateEvidenceSafeLink(e, 'run_detail'),
  );
  return {
    schema_version: body.schema_version as string,
    run_id: body.run_id as string,
    run_kind: requireEnum(body, 'run_kind', 'run_detail', isRunKind),
    display_name: body.display_name as string,
    status: requireEnum(body, 'status', 'run_detail', isRunLifecycleStatus),
    active_task_id: optionalString(body, 'active_task_id'),
    completed_tasks: body.completed_tasks as number,
    total_tasks: body.total_tasks as number,
    tasks,
    evidence_safe_links: evidence,
    started_at: optionalString(body, 'started_at'),
    ended_at: optionalString(body, 'ended_at'),
    observed_at: body.observed_at as string,
  };
}

function validateRunTask(value: unknown, endpoint: string): RunTask {
  const obj = requireObject(value, endpoint, 'task');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    task_id: requireString(obj, 'task_id', endpoint),
    display_name: optionalString(obj, 'display_name'),
    status: requireEnum(obj, 'status', endpoint, isRunLifecycleStatus),
    owning_agent: optionalString(obj, 'owning_agent'),
    model: optionalString(obj, 'model'),
    started_at: optionalString(obj, 'started_at'),
    ended_at: optionalString(obj, 'ended_at'),
  };
}

function validateEvidenceSafeLink(value: unknown, endpoint: string): EvidenceSafeLink {
  const obj = requireObject(value, endpoint, 'evidence_safe_link');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    artifact_id: requireString(obj, 'artifact_id', endpoint),
    label: requireString(obj, 'label', endpoint),
    media_type: requireString(obj, 'media_type', endpoint),
  };
}

function validateEvalSummary(value: unknown, endpoint: string): EvalSummary {
  const obj = requireObject(value, endpoint, 'eval_summary');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    run_id: requireString(obj, 'run_id', endpoint),
    suite_id: requireString(obj, 'suite_id', endpoint),
    suite_version: requireString(obj, 'suite_version', endpoint),
    arm_id: requireString(obj, 'arm_id', endpoint),
    status: requireEnum(obj, 'status', endpoint, isRunLifecycleStatus),
    verification_status: requireEnum(obj, 'verification_status', endpoint, isEvalVerificationStatus),
    receipt_count: requireNumber(obj, 'receipt_count', endpoint),
    metric_count: requireNumber(obj, 'metric_count', endpoint),
    published_projection_sha256: optionalString(obj, 'published_projection_sha256'),
    completed_at: optionalString(obj, 'completed_at'),
    observed_at: requireString(obj, 'observed_at', endpoint),
  };
}

function validateEvalPage(body: Record<string, unknown>): ObservePage<EvalSummary> {
  requireString(body, 'schema_version', 'evals');
  const items = requireArray(body.items, 'evals', 'items').map((e) => validateEvalSummary(e, 'evals'));
  return {
    schema_version: body.schema_version as string,
    items,
    cursor: optionalString(body, 'cursor'),
    has_more: requireBoolean(body, 'has_more', 'evals'),
    limit: requireNumber(body, 'limit', 'evals'),
  };
}

function validateEvalDetail(body: Record<string, unknown>): EvalDetail {
  requireString(body, 'schema_version', 'eval_detail');
  requireString(body, 'run_id', 'eval_detail');
  requireString(body, 'suite_id', 'eval_detail');
  requireString(body, 'suite_version', 'eval_detail');
  requireString(body, 'arm_id', 'eval_detail');
  requireString(body, 'observed_at', 'eval_detail');
  requireNumber(body, 'receipt_count', 'eval_detail');
  requireNumber(body, 'assigned_tasks', 'eval_detail');
  requireNumber(body, 'terminal_attempts', 'eval_detail');
  const metrics = requireArray(body.metrics, 'eval_detail', 'metrics').map((m) => validateEvalMetric(m, 'eval_detail'));
  return {
    schema_version: body.schema_version as string,
    run_id: body.run_id as string,
    suite_id: body.suite_id as string,
    suite_version: body.suite_version as string,
    arm_id: body.arm_id as string,
    model_id: optionalString(body, 'model_id'),
    model_provider: optionalString(body, 'model_provider'),
    status: requireEnum(body, 'status', 'eval_detail', isRunLifecycleStatus),
    verification_status: requireEnum(body, 'verification_status', 'eval_detail', isEvalVerificationStatus),
    receipt_count: body.receipt_count as number,
    assigned_tasks: body.assigned_tasks as number,
    terminal_attempts: body.terminal_attempts as number,
    metrics,
    published_projection_sha256: optionalString(body, 'published_projection_sha256'),
    completed_at: optionalString(body, 'completed_at'),
    observed_at: body.observed_at as string,
  };
}

function validateEvalMetric(value: unknown, endpoint: string): EvalMetricSummary {
  const obj = requireObject(value, endpoint, 'eval_metric');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    metric_id: requireString(obj, 'metric_id', endpoint),
    metric_version: requireString(obj, 'metric_version', endpoint),
    value: optionalNumber(obj, 'value'),
    unit: requireString(obj, 'unit', endpoint),
    eligible: requireNumber(obj, 'eligible', endpoint),
    denominator: requireNumber(obj, 'denominator', endpoint),
    verification_status: requireEnum(obj, 'verification_status', endpoint, isEvalVerificationStatus),
    recorded_at: optionalString(obj, 'recorded_at'),
  };
}

function validateDownloadArtifact(value: unknown, endpoint: string): DownloadArtifact {
  const obj = requireObject(value, endpoint, 'download');
  return {
    schema_version: requireString(obj, 'schema_version', endpoint),
    artifact_id: requireString(obj, 'artifact_id', endpoint),
    filename: requireString(obj, 'filename', endpoint),
    media_type: requireString(obj, 'media_type', endpoint),
    byte_size: requireNumber(obj, 'byte_size', endpoint),
    sha256: requireString(obj, 'sha256', endpoint),
    privacy_classification: requireEnum(obj, 'privacy_classification', endpoint, isDownloadPrivacyClassification),
    source_run_id: optionalString(obj, 'source_run_id'),
    download_url: requireString(obj, 'download_url', endpoint),
    generated_at: requireString(obj, 'generated_at', endpoint),
  };
}

function validateDownloadPage(body: Record<string, unknown>): ObservePage<DownloadArtifact> {
  requireString(body, 'schema_version', 'downloads');
  const items = requireArray(body.items, 'downloads', 'items').map((d) => validateDownloadArtifact(d, 'downloads'));
  return {
    schema_version: body.schema_version as string,
    items,
    cursor: optionalString(body, 'cursor'),
    has_more: requireBoolean(body, 'has_more', 'downloads'),
    limit: requireNumber(body, 'limit', 'downloads'),
  };
}
