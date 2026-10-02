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
  type PublicHintArgumentSource,
  type PublicPolicyDecisionActivityRecord,
  type PublicPromptHint,
  type PublicToolCallActivityRecord,
  type PublicToolDecisionActivityRecord,
  type PublicTrajectoryOutcome,
  type PublicTrajectoryPolicy,
  type PublicUnavailableReason,
  type QualityState,
  type ScenarioCategory,
  type SnapshotRecord,
  type TerminalStatus,
  type VerifierState,
} from '../contract/types';
import { normalizeActivityFamily, type WireActivityFamily } from '../contract/activity-family';
import {
  decodeCampaignProjectionEnvelope,
  type CampaignLifecycleEnvelope,
  type CampaignResultEnvelope,
  type WireActivitySummary,
  type WireBenchmarkGradeSummary,
  type WireBenchmarkObservations,
  type WireEvidenceBinding,
  type WireGovernedActionActivityRecord,
  type WireMetric,
  type WireModelActivityRecord,
  type WirePolicyDecisionActivityRecord,
  type WirePromptHint,
  type WireResourceSummary,
  type WireScenarioCriterion,
  type WireScenarioSummary,
  type WireScore,
  type WireSemanticGradeSummary,
  type WireToolCallActivityRecord,
  type WireToolDecisionActivityRecord,
  type WireVerificationMetadata,
} from '../contract/campaign-wire';
import { ValidationError } from '../contract/validators';

export const CAMPAIGN_SOURCE_REVISION = 'g8e-eval-campaign';

export { isCampaignProjectionEnvelope } from '../contract/campaign-wire';

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

/**
 * Live-event progress: terminal assignments finished vs the full campaign matrix size.
 * A restore/catch-up publishes already-terminal assignments before the queued records and
 * catalog snapshot of the remainder, so the scheduled or matrix count seen so far can lag
 * the terminal count; the total is floored at terminal to keep completed <= total.
 */
export function campaignProgressCounts(progress: RunProgress): { completed: number; total: number } {
  const known =
    progress.matrixTotal > 0
      ? progress.matrixTotal
      : progress.scheduled > 0
        ? progress.scheduled
        : progress.terminal;
  return { completed: progress.terminal, total: Math.max(known, progress.terminal) };
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
  const decoded = decodeCampaignProjectionEnvelope(envelope);
  switch (decoded.message_type) {
    case 'PublicAssignmentLifecycleRecord':
      return adaptLifecycleRecord(decoded, context);
    case 'PublicAssignmentResultProjection':
      return adaptResultProjection(decoded, context);
  }
}

function adaptLifecycleRecord(
  envelope: CampaignLifecycleEnvelope,
  context: CampaignAdaptContext,
): Array<SnapshotRecord | LiveEvent> {
  const record = envelope.record;
  const { run_id: runId, assignment_id: assignmentId } = record;
  const datasetId = campaignDatasetId(runId);
  const observedAt = new Date(record.observed_at).toISOString();
  const lifecycle = mapLifecycleStatus(record.lifecycle_status);
  const scenarioCategory = mapScenarioCategory(record.scenario_category);
  const role = mapModelRole(record.designated_role);
  const evaluationUnit = mapEvaluationUnit(record.lane);
  const repetition = record.repetition ?? 1;

  context.assignmentMeta.set(metaKey(runId, assignmentId), {
    repetition,
    scenarioId: record.scenario_id,
    scenarioCategory,
    variantId: record.variant_id,
    role,
    evaluationUnit,
    stackId: record.stack_id,
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
      task_id: record.scenario_id,
      variant_id: record.variant_id,
      role,
      lifecycle_status: lifecycle,
      completed,
      total,
      stage_label: buildStageLabel(lifecycle, scenarioCategory, record.scenario_id),
    });
  }

  return records;
}

function adaptResultProjection(
  envelope: CampaignResultEnvelope,
  context: CampaignAdaptContext,
): Array<SnapshotRecord | LiveEvent> {
  const record = envelope.record;
  const { run_id: runId, assignment_id: assignmentId } = record;
  const datasetId = campaignDatasetId(runId);
  const observedAt = new Date(record.completed_at).toISOString();
  const meta = context.assignmentMeta.get(metaKey(runId, assignmentId));
  const lifecycle = mapLifecycleStatus(record.lifecycle_status);
  const terminalStatus = mapTerminalStatus(record.lifecycle_status, record.summary_status);
  const qualityState: QualityState = terminalStatus === 'completed' ? 'live_in_progress' : 'terminal_failed';
  const variantId = record.variant_id ?? meta?.variantId ?? 'unknown';
  const role = meta?.role ?? mapModelRole(record.designated_role) ?? 'primary';

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
    task_id: meta?.scenarioId ?? record.scenario_id,
    variant_id: variantId,
    role,
    repetition: meta?.repetition ?? 1,
    scenario_category: meta?.scenarioCategory ?? mapScenarioCategory(record.scenario_category),
    evaluation_unit: meta?.evaluationUnit ?? mapEvaluationUnit(record.lane),
    stack_id: meta?.stackId,
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
    verification_disposition: mapVerificationDisposition(record.verification_status),
    verification_metadata: mapVerificationMetadata(record.verification_metadata),
    model_response: record.model_response,
    failure_output: record.failure_output,
    role_transcripts: record.role_transcripts,
    ...(record.trajectory_outcome !== undefined ? { trajectory_outcome: mapTrajectoryOutcome(record.trajectory_outcome) } : {}),
    ...(record.guided_retry_count !== undefined ? { guided_retry_count: record.guided_retry_count } : {}),
    ...(record.failure_reason !== undefined ? { failure_reason: record.failure_reason } : {}),
    ...(record.tools_declared !== undefined ? { tools_declared: record.tools_declared } : {}),
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
    terminal_status: terminalStatus,
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

function mapScenarioSummary(value: WireScenarioSummary | undefined): AssignmentResult['scenario_summary'] {
  if (value === undefined) return undefined;
  return {
    scenario_id: value.scenario_id,
    scenario_version: value.scenario_version,
    category: requiredScenarioCategory(value.category),
    public_description: value.public_description,
    grading_method: mapGradingMethod(value.grading_method),
    allowed_tools: value.allowed_tools ?? [],
    expected_tools: value.expected_tools ?? [],
    forbidden_tools: value.forbidden_tools ?? [],
    ...(value.trajectory_policy !== undefined ? { trajectory_policy: mapTrajectoryPolicy(value.trajectory_policy) } : {}),
    ...(value.prompt_hint !== undefined ? { prompt_hint: mapPromptHint(value.prompt_hint) } : {}),
    criteria: (value.criteria ?? []).map(mapScenarioCriterion),
    tool_score_dimensions: (value.tool_score_dimensions ?? []).map(mapToolScoreDimension),
  };
}

function mapPromptHint(value: WirePromptHint): PublicPromptHint {
  return {
    hinted_tools: value.hinted_tools ?? [],
    arguments: (value.arguments ?? []).map((argument) => ({
      tool_name: argument.tool_name,
      argument_name: argument.argument_name,
      source: mapHintArgumentSource(argument.source),
    })),
  };
}

function mapScenarioCriterion(criterion: WireScenarioCriterion): NonNullable<AssignmentResult['scenario_summary']>['criteria'][number] {
  return {
    criterion_id: criterion.criterion_id,
    public_label: criterion.public_label,
    public_description: criterion.public_description,
    grading_method: mapGradingMethod(criterion.grading_method),
    required: criterion.required,
  };
}

function mapToolScoreDimension(
  dimension: NonNullable<WireScenarioSummary['tool_score_dimensions']>[number],
): NonNullable<AssignmentResult['scenario_summary']>['tool_score_dimensions'][number] {
  return {
    dimension: mapToolScoreDimensionName(dimension.dimension),
    required: dimension.required,
  };
}

function mapSemanticGradeSummaries(value: WireSemanticGradeSummary[] | undefined): AssignmentResult['semantic_grade_summaries'] {
  if (value === undefined) return undefined;
  return value.map((grade) => ({
    criterion_id: grade.criterion_id,
    status: mapNativeResultStatus(grade.status),
    grading_method: mapGradingMethod(grade.grading_method),
    judge_variant_id: grade.judge_variant_id,
    explanation_code: mapExplanationCode(grade.explanation_code),
  }));
}

function mapActivitySummary(value: WireActivitySummary | undefined): AssignmentResult['activity_summary'] {
  if (value === undefined) return undefined;
  return {
    model_activity: mapActivityFamily(value.model_activity, mapModelActivityRecord),
    tool_decisions: mapActivityFamily(value.tool_decisions, mapToolDecisionRecord),
    tool_calls: mapActivityFamily(value.tool_calls, mapToolCallRecord),
    policy_decisions: mapActivityFamily(value.policy_decisions, mapPolicyDecisionRecord),
    governed_actions: mapActivityFamily(value.governed_actions, mapGovernedActionRecord),
  };
}

function mapActivityFamily<W, T>(family: WireActivityFamily<W>, mapRecord: (record: W) => T): PublicActivityFamily<T> {
  return normalizeActivityFamily({
    availability: family.availability,
    unavailable_reason: family.unavailable_reason,
    records: family.records?.map(mapRecord),
  });
}

function mapModelActivityRecord(record: WireModelActivityRecord): PublicModelActivityRecord {
  return {
    model_role: requiredModelRole(record.model_role),
    agent_persona: record.agent_persona,
    variant_id: record.variant_id,
    usage_availability: mapUsageAvailability(record.usage_availability),
    input_tokens: mapDecimalStringMetric(record.input_tokens),
    output_tokens: mapDecimalStringMetric(record.output_tokens),
    thinking_tokens: mapDecimalStringMetric(record.thinking_tokens),
    cache_tokens: mapDecimalStringMetric(record.cache_tokens),
    total_duration_nanos: mapDecimalStringMetric(record.total_duration_nanos),
    generation_duration_nanos: mapDecimalStringMetric(record.generation_duration_nanos),
    retry_count: record.retry_count === undefined ? undefined : { value: record.retry_count },
    finish_state: mapFinishState(record.finish_state),
    load_state: mapLoadState(record.load_state),
  };
}

function mapToolDecisionRecord(record: WireToolDecisionActivityRecord): PublicToolDecisionActivityRecord {
  return {
    tool_label: record.tool_label,
    recognized: record.recognized === true,
    selected: record.selected === true,
    permission_compliant: record.permission_compliant === true,
    unnecessary: record.unnecessary === true,
    outcome: mapSemanticOutcome(record.outcome),
    evidence_source: 'application_reported',
  };
}

function mapToolCallRecord(record: WireToolCallActivityRecord): PublicToolCallActivityRecord {
  return {
    tool_label: record.tool_label,
    execution_outcome: mapExecutionOutcome(record.execution_outcome),
    semantic_outcome: mapSemanticOutcome(record.semantic_outcome),
    evidence_source: 'application_reported',
    ...(record.loop_turn !== undefined ? { loop_turn: record.loop_turn } : {}),
    ...(record.error_type !== undefined ? { error_type: record.error_type } : {}),
    ...(record.guidance_shown === true ? { guidance_shown: true } : {}),
  };
}

function mapPolicyDecisionRecord(record: WirePolicyDecisionActivityRecord): PublicPolicyDecisionActivityRecord {
  return { tool_label: record.tool_label, outcome: mapToolOutcome(record.outcome), evidence_source: 'application_reported' };
}

function mapGovernedActionRecord(record: WireGovernedActionActivityRecord): PublicGovernedActionActivityRecord {
  return {
    action_label: 'governed action',
    reported_policy_outcome: mapReportedPolicyOutcome(record.reported_policy_outcome),
    receipt_status: mapReceiptStatus(record.receipt_status),
    evidence_source: mapEvidenceSource(record.evidence_source),
  };
}

function mapEvidenceBindings(value: WireEvidenceBinding[] | undefined): AssignmentResult['evidence_bindings'] {
  if (value === undefined) return undefined;
  return value.map((binding) => ({ sha256: binding.sha256, schema_ref: binding.schema_ref, kind: binding.kind }));
}

function mapResourceSummary(value: WireResourceSummary | undefined): AssignmentResult['resource_summary'] {
  if (value === undefined) return undefined;
  const result: NonNullable<AssignmentResult['resource_summary']> = {};
  for (const key of Object.keys(value) as Array<keyof WireResourceSummary>) {
    result[key] = mapWireMetric(value[key]);
  }
  return result;
}

function mapVerificationMetadata(value: WireVerificationMetadata | undefined): AssignmentResult['verification_metadata'] {
  if (value === undefined) return undefined;
  return {
    provenance: value.provenance === 'PUBLIC_VERIFICATION_PROVENANCE_BOUND' ? 'bound' : 'legacy_unbound',
    verifier_state: mapVerificationState(value.verifier_state),
    verifier_release_version: value.verifier_release_version,
    verifier_contract_version: value.verifier_contract_version,
    report_digest: value.report_digest,
    population_digest: value.population_digest,
  };
}

function mapWireMetric(metric: WireMetric | undefined): MetricValue<number> | undefined {
  if (metric === undefined) return undefined;
  if (metric.value !== undefined) return { value: metric.value };
  if (metric.unavailable_reason !== undefined) return { unavailable_reason: mapUnavailableReason(metric.unavailable_reason) };
  throw new ValidationError('metric carries neither value nor unavailable_reason', 'resource_summary');
}

/** Activity-record counters are uint64 protojson decimal strings. */
function mapDecimalStringMetric(value: string | undefined): MetricValue<number> | undefined {
  return value === undefined ? undefined : { value: Number(value) };
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
function mapTrajectoryPolicy(value: string): PublicTrajectoryPolicy { return normalizeEnumToken(value).replace(/^TRAJECTORY_POLICY_/, '').toLowerCase() as PublicTrajectoryPolicy; }
function mapTrajectoryOutcome(value: string): PublicTrajectoryOutcome { return normalizeEnumToken(value).replace(/^TRAJECTORY_OUTCOME_/, '').toLowerCase() as PublicTrajectoryOutcome; }
function mapHintArgumentSource(value: string): PublicHintArgumentSource { return normalizeEnumToken(value).replace(/^HINT_ARGUMENT_SOURCE_/, '').toLowerCase() as PublicHintArgumentSource; }
function mapReceiptStatus(value: string): 'unavailable' | 'reported' { return normalizeEnumToken(value).replace(/^PUBLIC_/, '').replace(/^RECEIPT_STATUS_/, '').toLowerCase() as ReturnType<typeof mapReceiptStatus>; }
function mapEvidenceSource(value: string): 'application_reported' | 'bound_public_proof' { return normalizeEnumToken(value).replace(/^PUBLIC_/, '').replace(/^EVIDENCE_SOURCE_/, '').toLowerCase() as ReturnType<typeof mapEvidenceSource>; }
function mapVerificationState(value: string): VerifierState {
  const status = mapNativeResultStatus(value);
  if (status === 'pass') return 'passed';
  if (status === 'fail' || status === 'invalid_evidence') return 'failed';
  return 'not_applicable';
}


// The wire admits only grade summaries, the tool scorecard, timing, GPU, and
// unavailable reasons (`assertBenchmarkObservations`), so those are the only
// fields read here; the view's escalation, security-event, and correlated-failure
// fields are never published on this path.
function mapBenchmarkObservations(value: WireBenchmarkObservations | undefined): BenchmarkObservations | undefined {
  if (value === undefined) return undefined;
  const unavailableReasons = value.unavailable_reasons ?? [];
  const gradeSummaries = mapGradeSummaries(value.grade_summaries);
  const toolScorecard = mapMetricRecord(value.tool_scorecard);
  const timing = mapMetricRecord(value.timing);
  const gpu = mapMetricRecord(value.gpu);
  if (!gradeSummaries && !toolScorecard && !timing && !gpu && unavailableReasons.length === 0) {
    return undefined;
  }
  return {
    grade_summaries: gradeSummaries,
    tool_scorecard: toolScorecard,
    timing,
    gpu,
    unavailable_reasons: unavailableReasons,
  };
}

function mapGradeSummaries(value: WireBenchmarkGradeSummary[] | undefined): BenchmarkObservations['grade_summaries'] {
  if (value === undefined || value.length === 0) return undefined;
  return value.map((summary) => ({
    criterion_id: summary.criterion_id,
    status: summary.status,
    explanation_code: mapExplanationCode(summary.explanation_code),
  }));
}

/** A metric group (tool scorecard, timing, GPU) with each wire metric decoded by
 *  the one metric decoder, so an unavailable reason is normalized the same way
 *  everywhere it is published. */
function mapMetricRecord<K extends string>(value: Partial<Record<K, WireMetric>> | undefined): Partial<Record<K, MetricValue>> | undefined {
  if (value === undefined) return undefined;
  const mapped: Partial<Record<K, MetricValue>> = {};
  for (const key of Object.keys(value) as K[]) {
    const metric = mapWireMetric(value[key]);
    if (metric !== undefined) mapped[key] = metric;
  }
  return Object.keys(mapped).length > 0 ? mapped : undefined;
}

function decomposedScoreKey(score: WireScore): string | undefined {
  if (score.dimension) return score.dimension;
  if (!score.score_id.includes(':')) return score.score_id;
  const suffix = score.score_id.slice(score.score_id.lastIndexOf(':') + 1).replace(/-/g, '_');
  return suffix || undefined;
}

/** A published score set always carries the task score, which is the verdict. A
 *  set without it is a contract violation, never a reason to infer a verdict
 *  from another metric. */
function taskScoreAsPass(metrics: Record<string, MetricValue>): void {
  if (!metrics.task_score) {
    throw new ValidationError('decomposed scores are missing task_score', 'decomposed_scores');
  }
  metrics.pass = metrics.task_score;
}

function mapDecomposedScores(value: WireScore[] | undefined): Record<string, MetricValue> {
  if (value === undefined) {
    return { pass: { unavailable_reason: 'decomposed scores not published' } };
  }
  const metrics: Record<string, MetricValue> = {};
  for (const [index, score] of value.entries()) {
    const key = decomposedScoreKey(score);
    if (!key) {
      throw new ValidationError('score has neither dimension nor score_id', `decomposed_scores[${index}]`);
    }
    // `score.value` is a number: the wire decoder materialized the proto3 default.
    metrics[key] = { value: score.value };
  }
  taskScoreAsPass(metrics);
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

const LIFECYCLE_TOKEN_PREFIX = 'ASSIGNMENT_LIFECYCLE_STATUS_';
const VERDICT_TOKEN_PREFIX = 'VERDICT_STATUS_';

function lifecycleToken(value: string): string {
  const normalized = normalizeEnumToken(value);
  if (!normalized.startsWith(LIFECYCLE_TOKEN_PREFIX)) {
    throw new ValidationError(`unknown assignment lifecycle status: ${value}`, 'lifecycle_status');
  }
  return normalized.slice(LIFECYCLE_TOKEN_PREFIX.length);
}

function verdictToken(value: string | undefined): string {
  const normalized = normalizeEnumToken(value);
  if (!normalized.startsWith(VERDICT_TOKEN_PREFIX)) {
    throw new ValidationError(`unknown verdict status: ${value ?? '(missing)'}`, 'summary_status');
  }
  return normalized.slice(VERDICT_TOKEN_PREFIX.length);
}

/**
 * Maps a terminal result to its outcome. Every wire value is named; a value
 * this function does not know is a contract violation, never a model failure.
 */
function mapTerminalStatus(lifecycleValue: string, summaryStatus: string | undefined): TerminalStatus {
  const lifecycle = lifecycleToken(lifecycleValue);
  const verdict = verdictToken(summaryStatus);
  switch (lifecycle) {
    case 'STOPPED':
      return 'stopped';
    case 'GRADER_FAILED':
      return 'grader_failed';
    case 'POLICY_REJECTED':
      return 'invalid_evidence';
    case 'QUEUED':
    case 'RUNNING':
      throw new ValidationError(`result projection carries a non-terminal lifecycle: ${lifecycle}`, 'lifecycle_status');
    case 'COMPLETED':
    case 'FAILED':
    case 'PARTIAL':
    case 'ESCALATED':
    case 'PROVIDER_FAILED':
    case 'UNAVAILABLE':
      break;
    default:
      throw new ValidationError(`unknown assignment lifecycle status: ${lifecycle}`, 'lifecycle_status');
  }
  switch (verdict) {
    case 'PASS':
      if (lifecycle !== 'COMPLETED') {
        throw new ValidationError(`verdict PASS contradicts lifecycle ${lifecycle}`, 'summary_status');
      }
      return 'completed';
    case 'FAIL':
      return 'model_failed';
    case 'INVALID_EVIDENCE':
    case 'UNAVAILABLE':
    case 'UNSUPPORTED':
      return 'invalid_evidence';
    default:
      throw new ValidationError(`unknown verdict status: ${verdict}`, 'summary_status');
  }
}

function mapLifecycleStatus(value: string): LifecycleStatus {
  switch (lifecycleToken(value)) {
    case 'QUEUED':
      return 'queued';
    case 'RUNNING':
      return 'running';
    case 'COMPLETED':
      return 'completed';
    case 'STOPPED':
      return 'stopped';
    case 'FAILED':
    case 'PARTIAL':
    case 'ESCALATED':
    case 'PROVIDER_FAILED':
    case 'GRADER_FAILED':
    case 'POLICY_REJECTED':
    case 'UNAVAILABLE':
      return 'failed';
    default:
      throw new ValidationError(`unknown assignment lifecycle status: ${value}`, 'lifecycle_status');
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

function requiredScenarioCategory(value: string): ScenarioCategory {
  const category = mapScenarioCategory(value);
  if (category === undefined) throw new ValidationError(`unknown scenario category: ${value}`, 'scenario_category');
  return category;
}

function requiredModelRole(value: string): ModelRole {
  const role = mapModelRole(value);
  if (role === undefined) throw new ValidationError(`unknown model role: ${value}`, 'model_role');
  return role;
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

function unavailableMetricReason(value: string[] | undefined): string | undefined {
  if (value === undefined || value.length === 0) return undefined;
  return value.join('; ');
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

