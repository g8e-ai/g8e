// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Maps Go campaign publication envelopes (Phase 8) into explorer view records
// for assignment results and live events. evaluation_summary, catalog, model,
// and methodology snapshots are published only by the Go projector.

import {
  VIEW_SCHEMA_VERSION,
  type AssignmentResult,
  type BenchmarkObservations,
  type EvaluationUnit,
  type LifecycleStatus,
  type LiveEvent,
  type LiveEventKind,
  type MetricValue,
  type ModelRole,
  type PublicActivityFamily,
  type PublicGovernedActionActivityRecord,
  type PublicModelActivityRecord,
  type PublicPolicyDecisionActivityRecord,
  type PublicToolCallActivityRecord,
  type PublicToolDecisionActivityRecord,
  type PublicUnavailableReason,
  type QualityState,
  type ScenarioCategory,
  type SnapshotRecord,
  type TerminalStatus,
  type VerifierState,
} from '../contract/types';
import { normalizeActivityFamily, parseWireActivityFamily } from '../contract/activity-family';
import { decodeCampaignProjectionEnvelope } from '../contract/campaign-wire';

export const CAMPAIGN_SOURCE_REVISION = 'g8e-eval-campaign';

export interface CampaignProjectionEnvelope {
  schema_version: string;
  message_type: string;
  idempotency_key: string;
  record: Record<string, unknown>;
}

export function isCampaignProjectionEnvelope(value: unknown): value is CampaignProjectionEnvelope {
  try {
    decodeCampaignProjectionEnvelope(value);
    return true;
  } catch {
    return false;
  }
}

function validateCampaignEnvelope(value: unknown): CampaignProjectionEnvelope {
  return decodeCampaignProjectionEnvelope(value) as unknown as CampaignProjectionEnvelope;
}

export interface CampaignAdaptContext {
  assignmentMeta: Map<string, AssignmentMeta>;
  runTotals: Map<string, RunProgress>;
  scheduledAssignments: Map<string, Set<string>>;
  terminalAssignments: Map<string, Set<string>>;
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
  /** Distinct assignments observed in queued lifecycle projections. */
  scheduled: number;
  /** Full campaign matrix size once known from queued lifecycle or catalog snapshots. */
  matrixTotal: number;
  /** Assignments with a terminal public result projection. */
  terminal: number;
  /** Terminal assignments with a passing verdict. */
  passed: number;
  /** Terminal assignments with a failing verdict or provider failure. */
  failed: number;
}

export function createCampaignAdaptContext(): CampaignAdaptContext {
  return {
    assignmentMeta: new Map(),
    runTotals: new Map(),
    scheduledAssignments: new Map(),
    terminalAssignments: new Map(),
  };
}

export function cloneCampaignAdaptContext(context: CampaignAdaptContext): CampaignAdaptContext {
  return {
    assignmentMeta: new Map(Array.from(context.assignmentMeta, ([key, value]) => [key, { ...value }])),
    runTotals: new Map(Array.from(context.runTotals, ([key, value]) => [key, { ...value }])),
    scheduledAssignments: new Map(Array.from(context.scheduledAssignments, ([key, value]) => [key, new Set(value)])),
    terminalAssignments: new Map(Array.from(context.terminalAssignments, ([key, value]) => [key, new Set(value)])),
  };
}

function ensureRunProgress(context: CampaignAdaptContext, runId: string): RunProgress {
  const existing = context.runTotals.get(runId);
  if (existing) return existing;
  const progress: RunProgress = { scheduled: 0, matrixTotal: 0, terminal: 0, passed: 0, failed: 0 };
  context.runTotals.set(runId, progress);
  return progress;
}

function trackScheduledAssignment(context: CampaignAdaptContext, runId: string, assignmentId: string): RunProgress {
  let seen = context.scheduledAssignments.get(runId);
  if (!seen) {
    seen = new Set();
    context.scheduledAssignments.set(runId, seen);
  }
  seen.add(assignmentId);
  const progress = ensureRunProgress(context, runId);
  progress.scheduled = seen.size;
  return progress;
}

function recordMatrixTotal(context: CampaignAdaptContext, runId: string, candidate: number): RunProgress {
  const progress = ensureRunProgress(context, runId);
  if (candidate > progress.matrixTotal) {
    progress.matrixTotal = candidate;
  }
  return progress;
}

/** Records the authoritative campaign matrix size from a published catalog snapshot. */
export function recordCampaignMatrixTotal(context: CampaignAdaptContext, runId: string, assignmentCount: number): void {
  if (assignmentCount > 0) {
    recordMatrixTotal(context, runId, assignmentCount);
  }
}

export function campaignRunIdFromDatasetId(datasetId: string): string | undefined {
  const prefix = 'ds-live-';
  return datasetId.startsWith(prefix) ? datasetId.slice(prefix.length) : undefined;
}

function markTerminalAssignment(
  context: CampaignAdaptContext,
  runId: string,
  assignmentId: string,
  terminalStatus: TerminalStatus,
): { progress: RunProgress; isNew: boolean } {
  const progress = ensureRunProgress(context, runId);
  let seen = context.terminalAssignments.get(runId);
  if (!seen) {
    seen = new Set();
    context.terminalAssignments.set(runId, seen);
  }
  if (seen.has(assignmentId)) {
    return { progress, isNew: false };
  }
  seen.add(assignmentId);
  progress.terminal += 1;
  if (terminalStatus === 'completed') {
    progress.passed += 1;
  } else {
    progress.failed += 1;
  }
  return { progress, isNew: true };
}

/** Live-event progress: terminal assignments finished vs the full campaign matrix size. */
export function campaignProgressCounts(progress: RunProgress): { completed: number; total: number } {
  const total =
    progress.matrixTotal > 0
      ? progress.matrixTotal
      : progress.scheduled > 0
        ? progress.scheduled
        : progress.terminal;
  return { completed: progress.terminal, total };
}

/** Point-in-time progress stamped onto a live event. */
export function liveEventProgressCounts(
  progress: RunProgress,
  eventKind?: LiveEventKind,
): { completed: number; total: number } {
  const { completed: terminal, total } = campaignProgressCounts(progress);
  if (eventKind === 'assignment_started') {
    // A running assignment is the next unit of work after finished terminals.
    const eventTotal = total > 0 ? total : terminal + 1;
    return { completed: Math.min(terminal + 1, eventTotal), total: eventTotal };
  }
  return { completed: terminal, total };
}

export function campaignDatasetId(runId: string): string {
  return `ds-live-${runId}`;
}

export function adaptCampaignProjectionEnvelope(
  envelope: unknown,
  context: CampaignAdaptContext,
): Array<SnapshotRecord | LiveEvent> {
  const validatedEnvelope = validateCampaignEnvelope(envelope);
  const adaptedEnvelope = validatedEnvelope;
  switch (adaptedEnvelope.message_type) {
    case 'PublicAssignmentLifecycleRecord':
      return adaptLifecycleRecord(adaptedEnvelope, context);
    case 'PublicAssignmentResultProjection':
      return adaptResultProjection(adaptedEnvelope, context);
    default:
      throw new Error(`unsupported campaign projection message_type ${adaptedEnvelope.message_type}`);
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
  let progress = ensureRunProgress(context, runId);
  if (lifecycle === 'queued') {
    progress = trackScheduledAssignment(context, runId, assignmentId);
    recordMatrixTotal(context, runId, progress.scheduled);
  }

  const eventKind = lifecycleEventKind(lifecycle);
  const { completed, total } = liveEventProgressCounts(progress, eventKind);
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
      role,
      lifecycle_status: lifecycle,
      completed,
      total,
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
  const variantId = optionalString(record.variant_id) ?? meta?.variantId ?? 'unknown';
  const role = meta?.role ?? mapModelRole(optionalString(record.designated_role)) ?? 'primary';

  const { progress } = markTerminalAssignment(context, runId, assignmentId, terminalStatus);
  const { completed, total } = campaignProgressCounts(progress);

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
    variant_id: variantId,
    role,
    repetition: meta?.repetition ?? 1,
    scenario_category: meta?.scenarioCategory ?? mapScenarioCategory(optionalString(record.scenario_category)),
    evaluation_unit: meta?.evaluationUnit ?? mapEvaluationUnit(optionalString(record.lane)),
    stack_id: meta?.stackId ?? optionalString(record.stack_id),
    scenario_summary: mapScenarioSummary(record.scenario_summary),
    semantic_grade_summaries: mapSemanticGradeSummaries(record.semantic_grade_summaries),
    activity_summary: mapActivitySummary(record.activity_summary),
    evidence_bindings: mapEvidenceBindings(record.evidence_bindings),
    terminal_status: terminalStatus,
    metric_values: mapDecomposedScores(record.decomposed_scores),
    missingness_reason: unavailableMetricReason(record.unavailable_metric_reasons),
    benchmark_observations: mapBenchmarkObservations(record.benchmark_observations),
    stage_summary: [],
    resource_summary: mapResourceSummary(record.resource_summary),
    verification_disposition: mapVerificationDisposition(optionalString(record.verification_status)),
    verification_metadata: mapVerificationMetadata(record.verification_metadata),
  };

  const records: Array<SnapshotRecord | LiveEvent> = [assignment];

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
    role: assignment.role,
    lifecycle_status: lifecycle === 'completed' ? 'completed' : 'failed',
    completed,
    total,
    stage_label: buildStageLabel(lifecycle, assignment.scenario_category, assignment.task_id),
    metric_delta: buildTerminalMetricDelta(assignment.metric_values),
  });

  return records;
}

function buildTerminalMetricDelta(metricValues: Record<string, MetricValue>): LiveEvent['metric_delta'] {
  const delta: Record<string, MetricValue> = {};
  if (metricValues.pass) delta.pass = metricValues.pass;
  if (metricValues.deterministic_pass_rate) delta.deterministic_pass_rate = metricValues.deterministic_pass_rate;
  return Object.keys(delta).length > 0 ? delta : undefined;
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

function mapScenarioSummary(value: unknown): AssignmentResult['scenario_summary'] {
  if (!isRecord(value)) return undefined;
  return {
    scenario_id: requiredString(value, 'scenario_id'),
    scenario_version: requiredString(value, 'scenario_version'),
    category: mapScenarioCategory(requiredString(value, 'category')) ?? 'instruction_adherence',
    public_description: requiredString(value, 'public_description'),
    grading_method: mapGradingMethod(requiredString(value, 'grading_method')),
    allowed_tools: stringArray(value.allowed_tools),
    expected_tools: stringArray(value.expected_tools),
    forbidden_tools: stringArray(value.forbidden_tools),
    criteria: Array.isArray(value.criteria) ? value.criteria.map(mapScenarioCriterion) : [],
    tool_score_dimensions: Array.isArray(value.tool_score_dimensions)
      ? value.tool_score_dimensions.map(mapToolScoreDimension)
      : [],
  };
}

function mapScenarioCriterion(value: unknown): NonNullable<AssignmentResult['scenario_summary']>['criteria'][number] {
  const criterion = asRecord(value);
  return {
    criterion_id: requiredString(criterion, 'criterion_id'),
    public_label: requiredString(criterion, 'public_label'),
    public_description: requiredString(criterion, 'public_description'),
    grading_method: mapGradingMethod(requiredString(criterion, 'grading_method')),
    required: criterion.required === true,
  };
}

function mapToolScoreDimension(value: unknown): NonNullable<AssignmentResult['scenario_summary']>['tool_score_dimensions'][number] {
  const dimension = asRecord(value);
  return {
    dimension: mapToolScoreDimensionName(requiredString(dimension, 'dimension')),
    required: dimension.required === true,
  };
}

function mapSemanticGradeSummaries(value: unknown): AssignmentResult['semantic_grade_summaries'] {
  if (!Array.isArray(value)) return undefined;
  return value.map((entry) => {
    const grade = asRecord(entry);
    return {
      criterion_id: requiredString(grade, 'criterion_id'),
      status: mapNativeResultStatus(requiredString(grade, 'status')),
      grading_method: mapGradingMethod(requiredString(grade, 'grading_method')),
      judge_variant_id: optionalString(grade.judge_variant_id),
      explanation_code: mapExplanationCode(requiredString(grade, 'explanation_code')),
    };
  });
}

function mapActivitySummary(value: unknown): AssignmentResult['activity_summary'] {
  if (!isRecord(value)) return undefined;
  const path = 'assignment_result.activity_summary';
  return {
    model_activity: mapActivityFamily(value.model_activity, mapModelActivityRecord, `${path}.model_activity`),
    tool_decisions: mapActivityFamily(value.tool_decisions, mapToolDecisionRecord, `${path}.tool_decisions`),
    tool_calls: mapActivityFamily(value.tool_calls, mapToolCallRecord, `${path}.tool_calls`),
    policy_decisions: mapActivityFamily(value.policy_decisions, mapPolicyDecisionRecord, `${path}.policy_decisions`),
    governed_actions: mapActivityFamily(value.governed_actions, mapGovernedActionRecord, `${path}.governed_actions`),
  };
}

function mapActivityFamily<T>(value: unknown, mapper: (value: unknown, path: string) => T, path: string): PublicActivityFamily<T> {
  const wire = parseWireActivityFamily(value, path, mapper);
  return normalizeActivityFamily({ ...wire, records: wire.records });
}

function mapModelActivityRecord(value: unknown, _path: string): PublicModelActivityRecord {
  const record = asRecord(value);
  return {
    model_role: mapModelRole(requiredString(record, 'model_role')) ?? 'primary',
    agent_persona: optionalString(record.agent_persona),
    variant_id: requiredString(record, 'variant_id'),
    usage_availability: mapUsageAvailability(requiredString(record, 'usage_availability')),
    input_tokens: mapWireMetric(record.input_tokens, true),
    output_tokens: mapWireMetric(record.output_tokens, true),
    thinking_tokens: mapWireMetric(record.thinking_tokens, true),
    cache_tokens: mapWireMetric(record.cache_tokens, true),
    total_duration_nanos: mapWireMetric(record.total_duration_nanos, true),
    generation_duration_nanos: mapWireMetric(record.generation_duration_nanos, true),
    retry_count: mapWireMetric(record.retry_count),
    finish_state: mapFinishState(requiredString(record, 'finish_state')),
    load_state: mapLoadState(requiredString(record, 'load_state')),
  };
}

function mapToolDecisionRecord(value: unknown, _path: string): PublicToolDecisionActivityRecord {
  const record = asRecord(value);
  return { tool_label: requiredString(record, 'tool_label'), recognized: record.recognized === true, selected: record.selected === true, permission_compliant: record.permission_compliant === true, unnecessary: record.unnecessary === true, outcome: mapSemanticOutcome(requiredString(record, 'outcome')), evidence_source: 'application_reported' };
}

function mapToolCallRecord(value: unknown, _path: string): PublicToolCallActivityRecord {
  const record = asRecord(value);
  return { tool_label: requiredString(record, 'tool_label'), execution_outcome: mapExecutionOutcome(requiredString(record, 'execution_outcome')), semantic_outcome: mapSemanticOutcome(requiredString(record, 'semantic_outcome')), evidence_source: 'application_reported' };
}

function mapPolicyDecisionRecord(value: unknown, _path: string): PublicPolicyDecisionActivityRecord {
  const record = asRecord(value);
  return { tool_label: requiredString(record, 'tool_label'), outcome: mapToolOutcome(requiredString(record, 'outcome')), evidence_source: 'application_reported' };
}

function mapGovernedActionRecord(value: unknown, _path: string): PublicGovernedActionActivityRecord {
  const record = asRecord(value);
  return { action_label: 'governed action', reported_policy_outcome: mapReportedPolicyOutcome(requiredString(record, 'reported_policy_outcome')), receipt_status: mapReceiptStatus(requiredString(record, 'receipt_status')), evidence_source: mapEvidenceSource(requiredString(record, 'evidence_source')) };
}

function mapEvidenceBindings(value: unknown): AssignmentResult['evidence_bindings'] {
  if (!Array.isArray(value)) return undefined;
  return value.map((entry) => {
    const binding = asRecord(entry);
    return { sha256: requiredString(binding, 'sha256'), schema_ref: requiredString(binding, 'schema_ref'), kind: requiredString(binding, 'kind') as NonNullable<AssignmentResult['evidence_bindings']>[number]['kind'] };
  });
}

function mapResourceSummary(value: unknown): AssignmentResult['resource_summary'] {
  if (!isRecord(value)) return undefined;
  const result: NonNullable<AssignmentResult['resource_summary']> = {};
  for (const [key, metric] of Object.entries(value)) {
    if (['latency_ms', 'input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'retries'].includes(key)) {
      result[key as keyof typeof result] = mapWireMetric(metric);
    }
  }
  return result;
}

function mapVerificationMetadata(value: unknown): AssignmentResult['verification_metadata'] {
  if (!isRecord(value)) return undefined;
  return {
    provenance: requiredString(value, 'provenance') === 'PUBLIC_VERIFICATION_PROVENANCE_BOUND' ? 'bound' : 'legacy_unbound',
    verifier_state: mapVerificationState(requiredString(value, 'verifier_state')),
    verifier_release_version: optionalString(value.verifier_release_version),
    verifier_contract_version: optionalString(value.verifier_contract_version),
    report_digest: optionalString(value.report_digest),
    population_digest: optionalString(value.population_digest),
  };
}

function mapWireMetric(value: unknown, decimalString = false): MetricValue<number> | undefined {
  if (value === undefined) return undefined;
  if (decimalString && typeof value === 'string') return { value: Number(value) };
  if (typeof value === 'number' && Number.isFinite(value)) return { value };
  if (isRecord(value) && typeof value.value === 'number' && Number.isFinite(value.value)) return { value: value.value };
  if (isRecord(value) && typeof value.unavailable_reason === 'string') return { unavailable_reason: mapUnavailableReason(value.unavailable_reason) };
  return undefined;
}

function mapGradingMethod(value: string): 'deterministic' | 'semantic_judge' { return value.endsWith('SEMANTIC_JUDGE') ? 'semantic_judge' : 'deterministic'; }
function mapNativeResultStatus(value: string): 'pass' | 'fail' | 'unavailable' | 'unsupported' | 'invalid_evidence' { return normalizeEnumToken(value).replace('VERDICT_STATUS_', '').toLowerCase() as ReturnType<typeof mapNativeResultStatus>; }
function mapExplanationCode(value: string): NonNullable<AssignmentResult['semantic_grade_summaries']>[number]['explanation_code'] { return normalizeEnumToken(value).replace(/^PUBLIC_/, '').replace(/^GRADE_EXPLANATION_CODE_/, '').toLowerCase() as NonNullable<AssignmentResult['semantic_grade_summaries']>[number]['explanation_code']; }
function mapToolScoreDimensionName(value: string): NonNullable<AssignmentResult['scenario_summary']>['tool_score_dimensions'][number]['dimension'] { return normalizeEnumToken(value).replace(/^PUBLIC_/, '').replace(/^TOOL_SCORE_DIMENSION_/, '').toLowerCase() as NonNullable<AssignmentResult['scenario_summary']>['tool_score_dimensions'][number]['dimension']; }
function mapUnavailableReason(value: string): PublicUnavailableReason {
  return normalizeEnumToken(value).replace(/^PUBLIC_/, '').replace(/^UNAVAILABLE_REASON_/, '').toLowerCase() as PublicUnavailableReason;
}
function mapUsageAvailability(value: string): 'reported' | 'unavailable' { return normalizeEnumToken(value).replace(/^EVALUATION_/, '').replace(/^USAGE_AVAILABILITY_/, '').toLowerCase() as ReturnType<typeof mapUsageAvailability>; }
function mapFinishState(value: string): PublicModelActivityRecord['finish_state'] { return normalizeEnumToken(value).replace(/^PUBLIC_/, '').replace(/^FINISH_STATE_/, '').toLowerCase() as PublicModelActivityRecord['finish_state']; }
function mapLoadState(value: string): PublicModelActivityRecord['load_state'] { return normalizeEnumToken(value).replace(/^EVALUATION_/, '').replace(/^LOAD_STATE_/, '').toLowerCase() as PublicModelActivityRecord['load_state']; }
function mapSemanticOutcome(value: string): PublicToolDecisionActivityRecord['outcome'] { return mapNativeResultStatus(value); }
function mapExecutionOutcome(value: string): PublicToolCallActivityRecord['execution_outcome'] { return mapNativeResultStatus(value); }
function mapToolOutcome(value: string): 'allow' | 'deny' | 'refused' { return normalizeEnumToken(value).replace(/^EVALUATION_/, '').replace(/^POLICY_DECISION_OUTCOME_/, '').toLowerCase() as ReturnType<typeof mapToolOutcome>; }
function mapReportedPolicyOutcome(value: string): 'allow' | 'deny' | 'refused' { return mapToolOutcome(value); }
function mapReceiptStatus(value: string): 'unavailable' | 'reported' { return normalizeEnumToken(value).replace(/^PUBLIC_/, '').replace(/^RECEIPT_STATUS_/, '').toLowerCase() as ReturnType<typeof mapReceiptStatus>; }
function mapEvidenceSource(value: string): 'application_reported' | 'bound_public_proof' { return normalizeEnumToken(value).replace(/^PUBLIC_/, '').replace(/^EVIDENCE_SOURCE_/, '').toLowerCase() as ReturnType<typeof mapEvidenceSource>; }
function mapVerificationState(value: string): VerifierState {
  const status = mapNativeResultStatus(value);
  if (status === 'pass') return 'passed';
  if (status === 'fail' || status === 'invalid_evidence') return 'failed';
  return 'not_applicable';
}

function isRecord(value: unknown): value is Record<string, unknown> { return typeof value === 'object' && value !== null && !Array.isArray(value); }
function asRecord(value: unknown): Record<string, unknown> { if (!isRecord(value)) throw new Error('campaign projection expected object'); return value; }
function stringArray(value: unknown): string[] { return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []; }

function mapBenchmarkObservations(value: unknown): BenchmarkObservations | undefined {
  if (typeof value !== 'object' || value === null) return undefined;
  const observations = value as Record<string, unknown>;
  const unavailableReasons = Array.isArray(observations.unavailable_reasons)
    ? observations.unavailable_reasons.filter((reason): reason is string => typeof reason === 'string')
    : [];
  const gradeSummaries = mapGradeSummaries(observations.grade_summaries);
  const toolScorecard = mapToolScorecard(observations.tool_scorecard);
  const escalationDisposition = mapEscalationDisposition(observations.escalation_disposition);
  const securityPrivacyEvents = mapSecurityPrivacyEvents(observations.security_privacy_events);
  const timing = mapBenchmarkTiming(observations.timing);
  const gpu = mapGPUObservation(observations.gpu);
  const correlatedFailure = mapCorrelatedFailure(observations.correlated_failure);
  if (
    !gradeSummaries &&
    !toolScorecard &&
    !escalationDisposition &&
    !securityPrivacyEvents &&
    !timing &&
    !gpu &&
    !correlatedFailure &&
    unavailableReasons.length === 0
  ) {
    return undefined;
  }
  return {
    grade_summaries: gradeSummaries,
    tool_scorecard: toolScorecard,
    escalation_disposition: escalationDisposition,
    security_privacy_events: securityPrivacyEvents,
    timing,
    gpu,
    correlated_failure: correlatedFailure,
    unavailable_reasons: unavailableReasons,
  };
}

function mapGradeSummaries(value: unknown): BenchmarkObservations['grade_summaries'] {
  if (!Array.isArray(value)) return undefined;
  const summaries = value
    .map((entry) => {
      if (typeof entry !== 'object' || entry === null) return undefined;
      const summary = entry as Record<string, unknown>;
      const criterionId = optionalString(summary.criterion_id);
      const status = optionalString(summary.status);
      if (!criterionId || !status) return undefined;
      return {
        criterion_id: criterionId,
        status,
        detail: optionalString(summary.detail),
      };
    })
    .filter((summary): summary is NonNullable<typeof summary> => summary !== undefined);
  return summaries.length > 0 ? summaries : undefined;
}

function mapToolScorecard(value: unknown): BenchmarkObservations['tool_scorecard'] {
  if (typeof value !== 'object' || value === null) return undefined;
  const scorecard = value as Record<string, unknown>;
  const mapped = compactMetricRecord(
    Object.fromEntries(Object.entries(scorecard).map(([key, metric]) => [key, mapMetricValue(metric)])) as Record<
      string,
      MetricValue | undefined
    >,
  );
  return Object.keys(mapped).length > 0 ? mapped : undefined;
}

function mapEscalationDisposition(value: unknown): BenchmarkObservations['escalation_disposition'] {
  if (typeof value !== 'string') return undefined;
  const normalized = value.replace(/-/g, '_');
  if (
    normalized === 'correct_autonomous_completion' ||
    normalized === 'correct_escalation' ||
    normalized === 'false_escalation' ||
    normalized === 'missed_escalation'
  ) {
    return normalized;
  }
  return undefined;
}

function mapSecurityPrivacyEvents(value: unknown): BenchmarkObservations['security_privacy_events'] {
  if (typeof value !== 'object' || value === null) return undefined;
  const events = value as Record<string, unknown>;
  const mapped: NonNullable<BenchmarkObservations['security_privacy_events']> = {};
  for (const [key, count] of Object.entries(events)) {
    if (typeof count === 'number' && Number.isFinite(count)) {
      mapped[key as keyof typeof mapped] = count;
    }
  }
  return Object.keys(mapped).length > 0 ? mapped : undefined;
}

function mapCorrelatedFailure(value: unknown): BenchmarkObservations['correlated_failure'] {
  if (typeof value !== 'object' || value === null) return undefined;
  const failure = value as Record<string, unknown>;
  const clusterId = optionalString(failure.cluster_id);
  const semanticErrorCode = optionalString(failure.semantic_error_code);
  if (!clusterId || !semanticErrorCode) return undefined;
  const affectedRoles = Array.isArray(failure.affected_roles)
    ? failure.affected_roles.filter((role): role is 'primary' | 'assistant' | 'lite' => role === 'primary' || role === 'assistant' || role === 'lite')
    : [];
  return {
    cluster_id: clusterId,
    semantic_error_code: semanticErrorCode,
    affected_roles: affectedRoles,
  };
}

function mapBenchmarkTiming(value: unknown): BenchmarkObservations['timing'] {
  if (typeof value !== 'object' || value === null) return undefined;
  const timing = value as Record<string, unknown>;
  const mapped = compactMetricRecord({
    model_load_ms: mapMetricValue(timing.model_load_ms),
    time_to_first_token_ms: mapMetricValue(timing.time_to_first_token_ms),
    generation_ms: mapMetricValue(timing.generation_ms),
    whole_task_ms: mapMetricValue(timing.whole_task_ms),
  });
  return Object.keys(mapped).length > 0 ? mapped : undefined;
}

function mapGPUObservation(value: unknown): BenchmarkObservations['gpu'] {
  if (typeof value !== 'object' || value === null) return undefined;
  const gpu = value as Record<string, unknown>;
  const mapped = compactMetricRecord({
    vram_before_bytes: mapMetricValue(gpu.vram_before_bytes),
    vram_peak_bytes: mapMetricValue(gpu.vram_peak_bytes),
    system_ram_peak_bytes: mapMetricValue(gpu.system_ram_peak_bytes),
    utilization_percent: mapMetricValue(gpu.utilization_percent),
    temperature_celsius: mapMetricValue(gpu.temperature_celsius),
    power_watts: mapMetricValue(gpu.power_watts),
    clock_mhz: mapMetricValue(gpu.clock_mhz),
  });
  return Object.keys(mapped).length > 0 ? mapped : undefined;
}

function compactMetricRecord<T extends Record<string, MetricValue | undefined>>(value: T): Partial<Record<keyof T, MetricValue>> {
  const mapped: Partial<Record<keyof T, MetricValue>> = {};
  for (const [key, metric] of Object.entries(value)) {
    if (metric !== undefined) {
      mapped[key as keyof T] = metric;
    }
  }
  return mapped;
}

function mapMetricValue(value: unknown): MetricValue | undefined {
  if (typeof value !== 'object' || value === null) return undefined;
  const metric = value as Record<string, unknown>;
  if (typeof metric.value === 'number' && Number.isFinite(metric.value)) {
    return { value: metric.value };
  }
  if (typeof metric.unavailable_reason === 'string') {
    return { unavailable_reason: metric.unavailable_reason };
  }
  return undefined;
}

function decomposedScoreKey(score: Record<string, unknown>): string | undefined {
  const dimension = optionalString(score.dimension);
  if (dimension) return dimension;
  const scoreId = optionalString(score.score_id);
  if (!scoreId) return undefined;
  if (!scoreId.includes(':')) return scoreId;
  const suffix = scoreId.slice(scoreId.lastIndexOf(':') + 1).replace(/-/g, '_');
  return suffix || undefined;
}

function synthesizePassMetric(metrics: Record<string, MetricValue>): void {
  if (metrics.task_score) {
    metrics.pass = metrics.task_score;
    return;
  }
  const rate = metrics.deterministic_pass_rate;
  if (rate?.value !== undefined) {
    metrics.pass = { value: rate.value >= 1 ? 1 : 0 };
    return;
  }
  if (!metrics.pass) {
    metrics.pass = { unavailable_reason: 'task score not published' };
  }
}

function mapDecomposedScores(value: unknown): Record<string, MetricValue> {
  if (!Array.isArray(value)) {
    return { pass: { unavailable_reason: 'decomposed scores not published' } };
  }
  const metrics: Record<string, MetricValue> = {};
  for (const entry of value) {
    if (typeof entry !== 'object' || entry === null) continue;
    const score = entry as Record<string, unknown>;
    const key = decomposedScoreKey(score);
    if (!key) continue;
    if (typeof score.value === 'number' && Number.isFinite(score.value)) {
      metrics[key] = { value: score.value };
    } else {
      metrics[key] = { unavailable_reason: 'score value unavailable' };
    }
  }
  synthesizePassMetric(metrics);
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
    case 'unverified':
      return 'not_run';
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
