// Maps Go campaign publication envelopes (Phase 8) into frozen explorer view
// records. The mirror carries CampaignProjectionEnvelope payloads inside
// record_bytes; this adapter is the only decode path for those envelopes.

import {
  VIEW_SCHEMA_VERSION,
  type AssignmentResult,
  type EvaluationSummary,
  type EvaluationUnit,
  type LifecycleStatus,
  type LiveEvent,
  type LiveEventKind,
  type MetricValue,
  type ModelRole,
  type QualityState,
  type ScenarioCategory,
  type SnapshotRecord,
  type TerminalStatus,
  type VerifierState,
} from '../contract/types';

export const CAMPAIGN_SOURCE_REVISION = 'g8e-eval-campaign';

export interface CampaignProjectionEnvelope {
  schema_version: string;
  message_type: string;
  idempotency_key: string;
  record: Record<string, unknown>;
}

export interface CampaignAdaptContext {
  assignmentMeta: Map<string, AssignmentMeta>;
  runTotals: Map<string, RunProgress>;
}

interface AssignmentMeta {
  repetition: number;
  scenarioId: string;
  scenarioCategory?: ScenarioCategory;
  variantId?: string;
  role?: ModelRole;
  evaluationUnit?: EvaluationUnit;
  stackId?: string;
}

interface RunProgress {
  total: number;
  completed: number;
  failed: number;
}

export function createCampaignAdaptContext(): CampaignAdaptContext {
  return {
    assignmentMeta: new Map(),
    runTotals: new Map(),
  };
}

export function isCampaignProjectionEnvelope(value: unknown): value is CampaignProjectionEnvelope {
  if (typeof value !== 'object' || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate.schema_version === 'string' &&
    typeof candidate.message_type === 'string' &&
    typeof candidate.idempotency_key === 'string' &&
    typeof candidate.record === 'object' &&
    candidate.record !== null
  );
}

export function campaignDatasetId(runId: string): string {
  return `ds-live-${runId}`;
}

export function adaptCampaignProjectionEnvelope(
  envelope: CampaignProjectionEnvelope,
  context: CampaignAdaptContext,
): Array<SnapshotRecord | LiveEvent> {
  switch (envelope.message_type) {
    case 'PublicAssignmentLifecycleRecord':
      return adaptLifecycleRecord(envelope, context);
    case 'PublicAssignmentResultProjection':
      return adaptResultProjection(envelope, context);
    default:
      throw new Error(`unsupported campaign projection message_type ${envelope.message_type}`);
  }
}

function adaptLifecycleRecord(
  envelope: CampaignProjectionEnvelope,
  context: CampaignAdaptContext,
): Array<SnapshotRecord | LiveEvent> {
  const record = envelope.record;
  const runId = requiredString(record, 'run_id');
  const assignmentId = requiredString(record, 'assignment_id');
  const datasetId = campaignDatasetId(runId);
  const observedAt = timestampString(record.observed_at) ?? new Date().toISOString();
  const lifecycle = mapLifecycleStatus(requiredString(record, 'lifecycle_status'));
  const scenarioCategory = mapScenarioCategory(optionalString(record.scenario_category));
  const role = mapModelRole(optionalString(record.designated_role));
  const evaluationUnit = mapEvaluationUnit(optionalString(record.lane));
  const repetition = optionalInteger(record.repetition) ?? 1;

  context.assignmentMeta.set(metaKey(runId, assignmentId), {
    repetition,
    scenarioId: optionalString(record.scenario_id) ?? assignmentId,
    scenarioCategory,
    variantId: optionalString(record.variant_id),
    role,
    evaluationUnit,
    stackId: optionalString(record.stack_id),
  });

  const records: Array<SnapshotRecord | LiveEvent> = [];
  if (!context.runTotals.has(runId)) {
    records.push(buildInitialEvaluationSummary(runId, datasetId, observedAt, evaluationUnit));
  }

  const eventKind = lifecycleEventKind(lifecycle);
  if (eventKind) {
    records.push({
      schema_version: VIEW_SCHEMA_VERSION,
      kind: eventKind,
      dataset_id: datasetId,
      quality_state: lifecycleQualityState(lifecycle),
      observed_at: observedAt,
      source_revision_label: CAMPAIGN_SOURCE_REVISION,
      event_id: envelope.idempotency_key,
      run_id: runId,
      assignment_id: assignmentId,
      task_id: optionalString(record.scenario_id),
      variant_id: optionalString(record.variant_id),
      lifecycle_status: lifecycle,
      completed: context.runTotals.get(runId)?.completed ?? 0,
      total: context.runTotals.get(runId)?.total ?? 0,
      stage_label: buildStageLabel(lifecycle, scenarioCategory, optionalString(record.scenario_id)),
    });
  }

  return records;
}

function adaptResultProjection(
  envelope: CampaignProjectionEnvelope,
  context: CampaignAdaptContext,
): Array<SnapshotRecord | LiveEvent> {
  const record = envelope.record;
  const runId = requiredString(record, 'run_id');
  const assignmentId = requiredString(record, 'assignment_id');
  const datasetId = campaignDatasetId(runId);
  const observedAt = timestampString(record.completed_at) ?? new Date().toISOString();
  const meta = context.assignmentMeta.get(metaKey(runId, assignmentId));
  const lifecycle = mapLifecycleStatus(requiredString(record, 'lifecycle_status'));
  const summaryStatus = optionalString(record.summary_status);
  const terminalStatus = mapTerminalStatus(lifecycle, summaryStatus);
  const qualityState: QualityState = terminalStatus === 'completed' ? 'live_in_progress' : 'terminal_failed';

  const hadRunProgress = context.runTotals.has(runId);
  const progress = context.runTotals.get(runId) ?? { total: 0, completed: 0, failed: 0 };
  if (terminalStatus === 'completed') {
    progress.completed += 1;
  } else {
    progress.failed += 1;
  }
  progress.total = Math.max(progress.total, progress.completed + progress.failed);
  context.runTotals.set(runId, progress);

  const assignment: AssignmentResult = {
    schema_version: VIEW_SCHEMA_VERSION,
    kind: 'assignment_result',
    dataset_id: datasetId,
    quality_state: qualityState,
    observed_at: observedAt,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    assignment_id: assignmentId,
    run_id: runId,
    task_id: meta?.scenarioId ?? optionalString(record.scenario_id) ?? assignmentId,
    variant_id: optionalString(record.variant_id) ?? meta?.variantId ?? 'unknown',
    role: meta?.role ?? mapModelRole(optionalString(record.designated_role)) ?? 'primary',
    repetition: meta?.repetition ?? 1,
    scenario_category: meta?.scenarioCategory ?? mapScenarioCategory(optionalString(record.scenario_category)),
    evaluation_unit: meta?.evaluationUnit ?? mapEvaluationUnit(optionalString(record.lane)),
    stack_id: meta?.stackId ?? optionalString(record.stack_id),
    terminal_status: terminalStatus,
    metric_values: mapDecomposedScores(record.decomposed_scores),
    missingness_reason: unavailableMetricReason(record.unavailable_metric_reasons),
    stage_summary: [],
    verification_disposition: mapVerificationDisposition(optionalString(record.verification_status)),
  };

  const records: Array<SnapshotRecord | LiveEvent> = [assignment];
  if (!hadRunProgress) {
    records.unshift(buildInitialEvaluationSummary(runId, datasetId, observedAt, assignment.evaluation_unit));
  }

  const eventKind: LiveEventKind = terminalStatus === 'completed' ? 'assignment_completed' : 'assignment_failed';
  records.push({
    schema_version: VIEW_SCHEMA_VERSION,
    kind: eventKind,
    dataset_id: datasetId,
    quality_state: qualityState,
    observed_at: observedAt,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    event_id: `${envelope.idempotency_key}:event`,
    run_id: runId,
    assignment_id: assignmentId,
    task_id: assignment.task_id,
    variant_id: assignment.variant_id,
    lifecycle_status: lifecycle === 'completed' ? 'completed' : 'failed',
    completed: progress.completed,
    total: progress.total,
    stage_label: buildStageLabel(lifecycle, assignment.scenario_category, assignment.task_id),
    metric_delta: assignment.metric_values.pass ? { pass_rate: assignment.metric_values.pass } : undefined,
  });

  records.push(buildUpdatedEvaluationSummary(runId, datasetId, observedAt, progress, assignment.evaluation_unit));
  return records;
}

function buildInitialEvaluationSummary(
  runId: string,
  datasetId: string,
  observedAt: string,
  evaluationUnit?: EvaluationUnit,
): EvaluationSummary {
  return {
    schema_version: VIEW_SCHEMA_VERSION,
    kind: 'evaluation_summary',
    dataset_id: datasetId,
    quality_state: 'live_in_progress',
    observed_at: observedAt,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    run_id: runId,
    suite_id: 'north-star-25',
    arm: evaluationUnit === 'system' ? 'heterogeneous-system' : 'homogeneous-model-role',
    evaluation_unit: evaluationUnit ?? 'model',
    lifecycle_state: 'running',
    assignment_total: 0,
    assignment_completed: 0,
    assignment_failed: 0,
    terminal_outcomes: {
      completed: 0,
      model_failed: 0,
      grader_failed: 0,
      invalid_evidence: 0,
      stopped: 0,
    },
    verifier_state: 'not_applicable',
    headline_metrics: {},
  };
}

function buildUpdatedEvaluationSummary(
  runId: string,
  datasetId: string,
  observedAt: string,
  progress: RunProgress,
  evaluationUnit?: EvaluationUnit,
): EvaluationSummary {
  const passRate: MetricValue<number> | undefined =
    progress.completed + progress.failed > 0
      ? { value: progress.completed / (progress.completed + progress.failed) }
      : undefined;
  return {
    schema_version: VIEW_SCHEMA_VERSION,
    kind: 'evaluation_summary',
    dataset_id: datasetId,
    quality_state: 'live_in_progress',
    observed_at: observedAt,
    source_revision_label: CAMPAIGN_SOURCE_REVISION,
    run_id: runId,
    suite_id: 'north-star-25',
    arm: evaluationUnit === 'system' ? 'heterogeneous-system' : 'homogeneous-model-role',
    evaluation_unit: evaluationUnit ?? 'model',
    lifecycle_state: 'running',
    assignment_total: progress.total,
    assignment_completed: progress.completed,
    assignment_failed: progress.failed,
    terminal_outcomes: {
      completed: progress.completed,
      model_failed: progress.failed,
      grader_failed: 0,
      invalid_evidence: 0,
      stopped: 0,
    },
    verifier_state: 'not_applicable',
    headline_metrics: passRate ? { pass_rate: passRate } : {},
  };
}

function lifecycleEventKind(lifecycle: LifecycleStatus): LiveEventKind | undefined {
  switch (lifecycle) {
    case 'queued':
      return 'stage_updated';
    case 'running':
      return 'assignment_started';
    default:
      return undefined;
  }
}

function lifecycleQualityState(lifecycle: LifecycleStatus): QualityState {
  return lifecycle === 'queued' || lifecycle === 'running' ? 'live_in_progress' : 'terminal_failed';
}

function buildStageLabel(
  lifecycle: LifecycleStatus | string,
  scenarioCategory?: ScenarioCategory,
  scenarioId?: string,
): string {
  const parts = [String(lifecycle).replace(/_/g, ' ')];
  if (scenarioCategory) parts.push(scenarioCategory.replace(/_/g, ' '));
  if (scenarioId) parts.push(scenarioId);
  return parts.join(' · ');
}

function mapDecomposedScores(value: unknown): Record<string, MetricValue> {
  if (!Array.isArray(value)) {
    return { pass: { unavailable_reason: 'decomposed scores not published' } };
  }
  const metrics: Record<string, MetricValue> = {};
  for (const entry of value) {
    if (typeof entry !== 'object' || entry === null) continue;
    const score = entry as Record<string, unknown>;
    const scoreId = optionalString(score.score_id);
    if (!scoreId) continue;
    if (typeof score.value === 'number' && Number.isFinite(score.value)) {
      metrics[scoreId] = { value: score.value };
    } else {
      metrics[scoreId] = { unavailable_reason: 'score value unavailable' };
    }
  }
  if (metrics.task_score) {
    metrics.pass = metrics.task_score;
  } else if (!metrics.pass) {
    metrics.pass = { unavailable_reason: 'task score not published' };
  }
  return metrics;
}

function mapVerificationDisposition(status?: string): VerifierState {
  switch ((status ?? '').toLowerCase()) {
    case 'verified':
    case 'passed':
      return 'passed';
    case 'failed':
    case 'invalid':
      return 'failed';
    default:
      return 'not_applicable';
  }
}

function mapTerminalStatus(lifecycle: LifecycleStatus, summaryStatus?: string): TerminalStatus {
  const summary = normalizeEnumToken(summaryStatus);
  const verdict = summary.startsWith('VERDICT_STATUS_')
    ? summary.slice('VERDICT_STATUS_'.length)
    : summary;
  if (lifecycle === 'stopped') return 'stopped';
  if (verdict.includes('GRADER') || lifecycle === 'failed' && summary.includes('GRADER')) return 'grader_failed';
  if (verdict.includes('POLICY') || summary.includes('POLICY') || verdict.includes('INVALID')) return 'invalid_evidence';
  if (lifecycle === 'completed' && verdict.includes('PASS')) return 'completed';
  if (lifecycle === 'completed') return 'model_failed';
  if (verdict.includes('UNAVAILABLE')) return 'model_failed';
  return 'model_failed';
}

function mapLifecycleStatus(value: string): LifecycleStatus {
  const normalized = normalizeEnumToken(value);
  const token = normalized.startsWith('ASSIGNMENT_LIFECYCLE_STATUS_')
    ? normalized.slice('ASSIGNMENT_LIFECYCLE_STATUS_'.length)
    : normalized;
  switch (token) {
    case 'QUEUED':
      return 'queued';
    case 'RUNNING':
      return 'running';
    case 'COMPLETED':
      return 'completed';
    case 'STOPPED':
      return 'stopped';
    default:
      return 'failed';
  }
}

function mapScenarioCategory(value?: string): ScenarioCategory | undefined {
  if (!value) return undefined;
  const normalized = normalizeEnumToken(value);
  if (normalized.startsWith('SCENARIO_CATEGORY_')) {
    return mapScenarioCategoryToken(normalized.slice('SCENARIO_CATEGORY_'.length));
  }
  return mapScenarioCategoryToken(normalized);
}

function mapScenarioCategoryToken(token: string): ScenarioCategory | undefined {
  switch (token) {
    case 'INSTRUCTION_ADHERENCE':
      return 'instruction_adherence';
    case 'TOOL_SELECTION':
      return 'tool_selection';
    case 'TOOL_ARGUMENT':
      return 'tool_arguments';
    case 'TECHNICAL_ANALYSIS':
      return 'technical_analysis';
    case 'ROUTING_DELEGATION':
      return 'routing_delegation';
    case 'VERIFICATION':
      return 'verification';
    case 'SECURITY_POLICY':
      return 'security_policy';
    case 'RECOVERY':
      return 'recovery';
    case 'FINAL_RESPONSE':
      return 'final_response';
    default:
      return undefined;
  }
}

function mapModelRole(value?: string): ModelRole | undefined {
  if (!value) return undefined;
  const normalized = normalizeEnumToken(value);
  if (normalized.startsWith('ROLE_')) {
    const role = normalized.slice('ROLE_'.length).toLowerCase();
    if (role === 'primary' || role === 'assistant' || role === 'lite') return role;
  }
  return undefined;
}

function mapEvaluationUnit(value?: string): EvaluationUnit | undefined {
  if (!value) return undefined;
  const normalized = normalizeEnumToken(value);
  if (normalized === 'LANE_MODEL_ROLE') return 'model';
  if (normalized === 'LANE_SYSTEM') return 'system';
  return undefined;
}

function unavailableMetricReason(value: unknown): string | undefined {
  if (!Array.isArray(value) || value.length === 0) return undefined;
  return value.map(String).join('; ');
}

function metaKey(runId: string, assignmentId: string): string {
  return `${runId}:${assignmentId}`;
}

function normalizeEnumToken(value?: string): string {
  if (!value) return '';
  if (value.startsWith('EVALUATION_')) {
    return value.slice('EVALUATION_'.length);
  }
  if (value.startsWith('MODEL_CAMPAIGN_')) {
    return value.slice('MODEL_CAMPAIGN_'.length);
  }
  return value;
}

function requiredString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  if (typeof value !== 'string' || value.length === 0) {
    throw new Error(`campaign projection missing ${key}`);
  }
  return value;
}

function optionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 ? value : undefined;
}

function optionalInteger(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isInteger(value) ? value : undefined;
}

function timestampString(value: unknown): string | undefined {
  if (typeof value !== 'string' || value.length === 0) return undefined;
  const parsed = Date.parse(value);
  if (Number.isNaN(parsed)) return undefined;
  return new Date(parsed).toISOString();
}
