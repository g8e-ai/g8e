// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Strict runtime validation for public records and feed transport payloads.
// Every guard fails closed: an unknown field, wrong type, or out-of-enum
// value throws a typed ValidationError that the store surfaces as a
// fail-closed error state rather than rendering partial data.

import {
  type CatalogSnapshot,
  type ConfidenceInterval,
  DATASET_KINDS,
  ENVIRONMENT_SOURCES,
  ESCALATION_DISPOSITIONS,
  EVALUATION_UNITS,
  type FeedBootstrap,
  type FeedHistoryPage,
  FEED_RECORD_TYPES,
  type FeedSnapshot,
  FRESHNESS_STATES,
  LIFECYCLE_STATUSES,
  type LiveEvent,
  LIVE_EVENT_KINDS,
  type MetricValue,
  type ModelSummary,
  MODEL_ROLES,
  NATIVE_LANES,
  NATIVE_METRIC_UNITS,
  NATIVE_POSTURES,
  NATIVE_RESULT_STATUSES,
  NATIVE_SCENARIO_STATUSES,
  type ProjectionRecord,
  QUALITY_STATES,
  SCENARIO_CATEGORIES,
  SECURITY_PRIVACY_EVENTS,
  type SnapshotRecord,
  SNAPSHOT_KINDS,
  type SuiteSummary,
  type EvaluationSummary,
  type AssignmentResult,
  type MethodologySnapshot,
  TERMINAL_STATUSES,
  TOOL_SCORE_DIMENSIONS,
  VERIFIER_STATES,
} from './types';

export class ValidationError extends Error {
  constructor(
    message: string,
    readonly path: string,
  ) {
    super(message);
    this.name = 'ValidationError';
  }
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === 'string';
}

function isNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

function isInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value);
}

function isBoolean(value: unknown): value is boolean {
  return typeof value === 'boolean';
}

function isEnum<T extends string>(value: unknown, set: readonly T[]): value is T {
  return isString(value) && (set as readonly string[]).includes(value);
}

function assert(condition: unknown, path: string, message: string): asserts condition {
  if (!condition) throw new ValidationError(message, path);
}

function assertString(value: unknown, path: string): asserts value is string {
  assert(isString(value), path, `expected string`);
}

function assertNumber(value: unknown, path: string): asserts value is number {
  assert(isNumber(value), path, `expected finite number`);
}

function assertInteger(value: unknown, path: string): asserts value is number {
  assert(isInteger(value), path, `expected safe integer`);
}

function assertBoolean(value: unknown, path: string): asserts value is boolean {
  assert(isBoolean(value), path, `expected boolean`);
}

function assertObject(value: unknown, path: string): asserts value is Record<string, unknown> {
  assert(isObject(value), path, `expected object`);
}

function assertEnum<T extends string>(
  value: unknown,
  set: readonly T[],
  path: string,
): asserts value is T {
  assert(isEnum(value, set), path, `expected one of ${(set as readonly string[]).join(', ')}`);
}

function assertOptional<T>(
  value: unknown,
  path: string,
  check: (v: unknown, p: string) => asserts v is T,
): asserts value is T | undefined {
  if (value === undefined) return;
  check(value, path);
}

/** Legacy mirror records published verifier_failure_summary as string[]. */
export function normalizeVerifierFailureSummary(value: unknown): string | undefined {
  if (value === undefined || value === null) return undefined;
  if (isString(value)) return value;
  if (!Array.isArray(value)) return undefined;
  const parts = value.filter((item): item is string => isString(item));
  if (parts.length === 0) return undefined;
  if (parts.length === 1) return parts[0];
  const maxDetail = 3;
  const detail = parts.slice(0, maxDetail).join('; ');
  const suffix = parts.length > maxDetail ? `; ... and ${parts.length - maxDetail} more` : '';
  const combined = `${parts.length} verification failure(s): ${detail}${suffix}`;
  return combined.length > 500 ? `${combined.slice(0, 497)}...` : combined;
}

function assertStringArray(value: unknown, path: string): asserts value is string[] {
  assert(Array.isArray(value), path, 'expected array');
  for (let i = 0; i < value.length; i++) assertString(value[i], `${path}[${i}]`);
}

/** Reject unknown fields on a record to prevent silent shape drift. */
function rejectUnknown(value: Record<string, unknown>, allowed: readonly string[], path: string): void {
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) {
      throw new ValidationError(`unknown field "${key}"`, path);
    }
  }
}

const ENVELOPE_FIELDS = [
  'schema_version',
  'kind',
  'dataset_id',
  'quality_state',
  'observed_at',
  'source_revision_label',
] as const;

function assertEnvelope(
  value: Record<string, unknown>,
  path: string,
  kindSet: readonly string[],
): void {
  assertString(value.schema_version, `${path}.schema_version`);
  assert(['1.0.0', '1.1.0', '1.2.0', '1.3.0'].includes(value.schema_version), `${path}.schema_version`, `expected a supported schema version`);
  assertEnum(value.kind, kindSet, `${path}.kind`);
  assertString(value.dataset_id, `${path}.dataset_id`);
  assertEnum(value.quality_state, QUALITY_STATES, `${path}.quality_state`);
  assertString(value.observed_at, `${path}.observed_at`);
  assertOptional(value.source_revision_label, `${path}.source_revision_label`, assertString);
}

function assertMetricValue(value: unknown, path: string): asserts value is MetricValue {
  assertObject(value, path);
  rejectUnknown(value, ['value', 'unavailable_reason'], path);
  assertOptional(value.value, `${path}.value`, assertNumber);
  assertOptional(value.unavailable_reason, `${path}.unavailable_reason`, assertString);
  if (value.value === undefined && value.unavailable_reason === undefined) {
    throw new ValidationError('metric requires value or unavailable_reason', path);
  }
}

function assertOptionalMetricValue(
  value: unknown,
  path: string,
): asserts value is MetricValue | undefined {
  if (value === undefined) return;
  assertMetricValue(value, path);
}

function assertConfidenceInterval(value: unknown, path: string): asserts value is ConfidenceInterval {
  assertObject(value, path);
  rejectUnknown(value, ['estimate', 'lower', 'upper', 'denominator'], path);
  assertNumber(value.estimate, `${path}.estimate`);
  assertNumber(value.lower, `${path}.lower`);
  assertNumber(value.upper, `${path}.upper`);
  assertInteger(value.denominator, `${path}.denominator`);
  assert(value.denominator >= 0, `${path}.denominator`, 'must be >= 0');
  assert(value.lower <= value.estimate && value.estimate <= value.upper, path, 'lower <= estimate <= upper');
}

function assertOptionalConfidenceInterval(
  value: unknown,
  path: string,
): asserts value is ConfidenceInterval | undefined {
  if (value === undefined) return;
  assertConfidenceInterval(value, path);
}

function assertMetricRecord(value: unknown, allowed: readonly string[], path: string): void {
  assertObject(value, path);
  rejectUnknown(value, allowed, path);
  for (const [key, metric] of Object.entries(value)) assertMetricValue(metric, `${path}.${key}`);
}

function assertGradeSummaries(value: unknown, path: string): void {
  assert(Array.isArray(value), path, 'expected array');
  for (let i = 0; i < value.length; i++) {
    const summary = value[i];
    assertObject(summary, `${path}[${i}]`);
    rejectUnknown(summary, ['criterion_id', 'status', 'detail'], `${path}[${i}]`);
    assertString(summary.criterion_id, `${path}[${i}].criterion_id`);
    assertString(summary.status, `${path}[${i}].status`);
    assertOptional(summary.detail, `${path}[${i}].detail`, assertString);
  }
}

function assertBenchmarkObservations(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['grade_summaries', 'escalation_disposition', 'tool_scorecard', 'security_privacy_events', 'timing', 'gpu', 'correlated_failure', 'unavailable_reasons'], path);
  if (value.grade_summaries !== undefined) assertGradeSummaries(value.grade_summaries, `${path}.grade_summaries`);
  assertOptional(value.escalation_disposition, `${path}.escalation_disposition`, (v, p) => assertEnum(v, ESCALATION_DISPOSITIONS, p));
  if (value.tool_scorecard !== undefined) assertMetricRecord(value.tool_scorecard, TOOL_SCORE_DIMENSIONS, `${path}.tool_scorecard`);
  if (value.security_privacy_events !== undefined) {
    assertObject(value.security_privacy_events, `${path}.security_privacy_events`);
    rejectUnknown(value.security_privacy_events, SECURITY_PRIVACY_EVENTS, `${path}.security_privacy_events`);
    for (const [key, count] of Object.entries(value.security_privacy_events)) {
      assertInteger(count, `${path}.security_privacy_events.${key}`);
      assert(count >= 0, `${path}.security_privacy_events.${key}`, 'must be >= 0');
    }
  }
  if (value.timing !== undefined) assertMetricRecord(value.timing, ['model_load_ms', 'time_to_first_token_ms', 'generation_ms', 'whole_task_ms'], `${path}.timing`);
  if (value.gpu !== undefined) assertMetricRecord(value.gpu, ['vram_before_bytes', 'vram_peak_bytes', 'system_ram_peak_bytes', 'utilization_percent', 'temperature_celsius', 'power_watts', 'clock_mhz'], `${path}.gpu`);
  if (value.correlated_failure !== undefined) {
    assertObject(value.correlated_failure, `${path}.correlated_failure`);
    rejectUnknown(value.correlated_failure, ['cluster_id', 'semantic_error_code', 'affected_roles'], `${path}.correlated_failure`);
    assertString(value.correlated_failure.cluster_id, `${path}.correlated_failure.cluster_id`);
    assertString(value.correlated_failure.semantic_error_code, `${path}.correlated_failure.semantic_error_code`);
    assert(Array.isArray(value.correlated_failure.affected_roles), `${path}.correlated_failure.affected_roles`, 'expected array');
    for (let i = 0; i < value.correlated_failure.affected_roles.length; i++) {
      assertEnum(value.correlated_failure.affected_roles[i], MODEL_ROLES, `${path}.correlated_failure.affected_roles[${i}]`);
    }
  }
  assertStringArray(value.unavailable_reasons, `${path}.unavailable_reasons`);
}

const PROVIDER_ENVIRONMENT_FIELDS = ['source', 'processor', 'memory', 'graphics', 'storage', 'system_type'] as const;

function assertProviderEnvironment(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, PROVIDER_ENVIRONMENT_FIELDS, path);
  const env = value as Record<string, unknown>;
  assertEnum(env.source, ENVIRONMENT_SOURCES, `${path}.source`);
  for (const field of PROVIDER_ENVIRONMENT_FIELDS) {
    if (field === 'source') continue;
    assertOptional(env[field], `${path}.${field}`, assertString);
  }
}

export function isCatalogSnapshot(value: unknown): asserts value is CatalogSnapshot {
  assertObject(value, 'catalog_snapshot');
  rejectUnknown(value, [...ENVELOPE_FIELDS, 'dataset_kind', 'title', 'description', 'limitations', 'model_count', 'evaluated_count', 'suite_count', 'run_count', 'assignment_count', 'provider_request_count', 'provider_token_count', 'retry_count', 'verifier_passed_count', 'verifier_failed_count', 'generated_at', 'provider_environment'], 'catalog_snapshot');
  assertEnvelope(value, 'catalog_snapshot', ['catalog_snapshot']);
  assertEnum(value.dataset_kind, DATASET_KINDS, 'catalog_snapshot.dataset_kind');
  assertString(value.title, 'catalog_snapshot.title');
  assertString(value.description, 'catalog_snapshot.description');
  assert(Array.isArray(value.limitations), 'catalog_snapshot.limitations', 'expected array');
  for (let i = 0; i < value.limitations.length; i++) {
    assertString((value.limitations as unknown[])[i], `catalog_snapshot.limitations[${i}]`);
  }
  assertInteger(value.model_count, 'catalog_snapshot.model_count');
  assert(value.model_count >= 0, 'catalog_snapshot.model_count', 'must be >= 0');
  assertInteger(value.evaluated_count, 'catalog_snapshot.evaluated_count');
  assert(value.evaluated_count >= 0, 'catalog_snapshot.evaluated_count', 'must be >= 0');
  assertInteger(value.suite_count, 'catalog_snapshot.suite_count');
  assert(value.suite_count >= 0, 'catalog_snapshot.suite_count', 'must be >= 0');
  assertInteger(value.run_count, 'catalog_snapshot.run_count');
  assert(value.run_count >= 0, 'catalog_snapshot.run_count', 'must be >= 0');
  assertInteger(value.assignment_count, 'catalog_snapshot.assignment_count');
  assert(value.assignment_count >= 0, 'catalog_snapshot.assignment_count', 'must be >= 0');
  assertInteger(value.provider_request_count, 'catalog_snapshot.provider_request_count');
  assert(value.provider_request_count >= 0, 'catalog_snapshot.provider_request_count', 'must be >= 0');
  assertInteger(value.provider_token_count, 'catalog_snapshot.provider_token_count');
  assert(value.provider_token_count >= 0, 'catalog_snapshot.provider_token_count', 'must be >= 0');
  assertInteger(value.retry_count, 'catalog_snapshot.retry_count');
  assert(value.retry_count >= 0, 'catalog_snapshot.retry_count', 'must be >= 0');
  assertInteger(value.verifier_passed_count, 'catalog_snapshot.verifier_passed_count');
  assert(value.verifier_passed_count >= 0, 'catalog_snapshot.verifier_passed_count', 'must be >= 0');
  assertInteger(value.verifier_failed_count, 'catalog_snapshot.verifier_failed_count');
  assert(value.verifier_failed_count >= 0, 'catalog_snapshot.verifier_failed_count', 'must be >= 0');
  assertString(value.generated_at, 'catalog_snapshot.generated_at');
  if (value.provider_environment !== undefined) {
    assertProviderEnvironment(value.provider_environment, 'catalog_snapshot.provider_environment');
  }
}

export function isModelSummary(value: unknown): asserts value is ModelSummary {
  assertObject(value, 'model_summary');
  assertEnvelope(value, 'model_summary', ['model_summary']);
  assertString(value.variant_id, 'model_summary.variant_id');
  assertString(value.display_name, 'model_summary.display_name');
  assertOptional(value.served_model_tag, 'model_summary.served_model_tag', assertString);
  assertEnum(value.role, MODEL_ROLES, 'model_summary.role');
  assertOptional(value.backend_provider_class, 'model_summary.backend_provider_class', assertString);
  assertOptional(value.quantization_weight_class, 'model_summary.quantization_weight_class', assertString);
  assertBoolean(value.inventory_only, 'model_summary.inventory_only');
  assertNumber(value.evaluation_coverage, 'model_summary.evaluation_coverage');
  assertOptionalConfidenceInterval(value.pass_rate, 'model_summary.pass_rate');
  assertOptionalMetricValue(value.agreement_pairwise, 'model_summary.agreement_pairwise');
  assertOptionalMetricValue(value.agreement_all_five, 'model_summary.agreement_all_five');
  if (value.repeatability !== undefined) {
    assertObject(value.repeatability, 'model_summary.repeatability');
    rejectUnknown(value.repeatability, ['consistently_correct', 'consistently_wrong', 'inconsistent', 'insufficient'], 'model_summary.repeatability');
    assertInteger(value.repeatability.consistently_correct, 'model_summary.repeatability.consistently_correct');
    assertInteger(value.repeatability.consistently_wrong, 'model_summary.repeatability.consistently_wrong');
    assertInteger(value.repeatability.inconsistent, 'model_summary.repeatability.inconsistent');
    assertInteger(value.repeatability.insufficient, 'model_summary.repeatability.insufficient');
  }
  assertOptionalMetricValue(value.latency_p50_ms, 'model_summary.latency_p50_ms');
  assertOptionalMetricValue(value.latency_p95_ms, 'model_summary.latency_p95_ms');
  assertOptionalMetricValue(value.output_throughput_p50, 'model_summary.output_throughput_p50');
  assertOptionalMetricValue(value.output_throughput_p95, 'model_summary.output_throughput_p95');
  assertOptionalMetricValue(value.input_tokens, 'model_summary.input_tokens');
  assertOptionalMetricValue(value.output_tokens, 'model_summary.output_tokens');
  assertOptionalMetricValue(value.thinking_tokens, 'model_summary.thinking_tokens');
  assertOptionalMetricValue(value.cache_tokens, 'model_summary.cache_tokens');
  if (value.terminal_outcomes !== undefined) {
    assertObject(value.terminal_outcomes, 'model_summary.terminal_outcomes');
  }
  if (value.unavailable_reasons !== undefined) {
    assert(Array.isArray(value.unavailable_reasons), 'model_summary.unavailable_reasons', 'expected array');
  }
}

export function isSuiteSummary(value: unknown): asserts value is SuiteSummary {
  assertObject(value, 'suite_summary');
  assertEnvelope(value, 'suite_summary', ['suite_summary']);
  assertString(value.suite_id, 'suite_summary.suite_id');
  assertString(value.display_name, 'suite_summary.display_name');
  assertInteger(value.task_count, 'suite_summary.task_count');
  assertInteger(value.assignment_count, 'suite_summary.assignment_count');
  assertEnum(value.status, LIFECYCLE_STATUSES, 'suite_summary.status');
  assertEnum(value.verifier_state, VERIFIER_STATES, 'suite_summary.verifier_state');
  assertOptional(value.verifier_failure_summary, 'suite_summary.verifier_failure_summary', assertString);
  assert(Array.isArray(value.model_coverage), 'suite_summary.model_coverage', 'expected array');
  assertObject(value.metric_summaries, 'suite_summary.metric_summaries');
  assert(Array.isArray(value.limitations), 'suite_summary.limitations', 'expected array');
}

function assertNativeResult(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['active_posture', 'lane', 'summary_status', 'summary', 'required_verdict_count', 'passed_verdict_count', 'verification_valid', 'verification_failure_count', 'scenarios', 'metrics'], path);
  assertEnum(value.active_posture, NATIVE_POSTURES, `${path}.active_posture`);
  assertEnum(value.lane, NATIVE_LANES, `${path}.lane`);
  assertEnum(value.summary_status, NATIVE_RESULT_STATUSES, `${path}.summary_status`);
  assertString(value.summary, `${path}.summary`);
  assertInteger(value.required_verdict_count, `${path}.required_verdict_count`);
  assertInteger(value.passed_verdict_count, `${path}.passed_verdict_count`);
  assert(value.required_verdict_count >= 0, `${path}.required_verdict_count`, 'must be >= 0');
  assert(value.passed_verdict_count >= 0 && value.passed_verdict_count <= value.required_verdict_count, `${path}.passed_verdict_count`, 'must be between zero and required_verdict_count');
  assertBoolean(value.verification_valid, `${path}.verification_valid`);
  assertInteger(value.verification_failure_count, `${path}.verification_failure_count`);
  assert(value.verification_failure_count >= 0, `${path}.verification_failure_count`, 'must be >= 0');
  assert(value.verification_valid === (value.verification_failure_count === 0), path, 'verification validity and failure count disagree');
  assert(Array.isArray(value.scenarios), `${path}.scenarios`, 'expected array');
  let verdictCount = 0;
  let passedCount = 0;
  for (let scenarioIndex = 0; scenarioIndex < value.scenarios.length; scenarioIndex++) {
    const scenarioPath = `${path}.scenarios[${scenarioIndex}]`;
    const scenario: unknown = value.scenarios[scenarioIndex];
    assertObject(scenario, scenarioPath);
    rejectUnknown(scenario, ['scenario_id', 'scenario_version', 'status', 'verdicts'], scenarioPath);
    assertString(scenario.scenario_id, `${scenarioPath}.scenario_id`);
    assertString(scenario.scenario_version, `${scenarioPath}.scenario_version`);
    assertEnum(scenario.status, NATIVE_SCENARIO_STATUSES, `${scenarioPath}.status`);
    assert(Array.isArray(scenario.verdicts), `${scenarioPath}.verdicts`, 'expected array');
    for (let verdictIndex = 0; verdictIndex < scenario.verdicts.length; verdictIndex++) {
      const verdictPath = `${scenarioPath}.verdicts[${verdictIndex}]`;
      const verdict: unknown = scenario.verdicts[verdictIndex];
      assertObject(verdict, verdictPath);
      rejectUnknown(verdict, ['assertion_id', 'assertion_version', 'status'], verdictPath);
      assertString(verdict.assertion_id, `${verdictPath}.assertion_id`);
      assertString(verdict.assertion_version, `${verdictPath}.assertion_version`);
      assertEnum(verdict.status, NATIVE_RESULT_STATUSES, `${verdictPath}.status`);
      verdictCount++;
      if (verdict.status === 'pass') passedCount++;
    }
  }
  assert(verdictCount === value.required_verdict_count, path, 'scenario verdict count must equal required_verdict_count');
  assert(passedCount === value.passed_verdict_count, path, 'passing scenario verdict count must equal passed_verdict_count');
  assert((value.summary_status === 'pass') === (passedCount === verdictCount), path, 'summary status and verdict outcomes disagree');
  assert(Array.isArray(value.metrics), `${path}.metrics`, 'expected array');
  for (let index = 0; index < value.metrics.length; index++) {
    const metricPath = `${path}.metrics[${index}]`;
    const metric: unknown = value.metrics[index];
    assertObject(metric, metricPath);
    rejectUnknown(metric, ['metric_id', 'metric_version', 'numerator', 'denominator', 'value', 'unit'], metricPath);
    assertString(metric.metric_id, `${metricPath}.metric_id`);
    assertString(metric.metric_version, `${metricPath}.metric_version`);
    assertInteger(metric.numerator, `${metricPath}.numerator`);
    assertInteger(metric.denominator, `${metricPath}.denominator`);
    assert(metric.numerator >= 0 && metric.numerator <= metric.denominator, metricPath, 'metric counts are inconsistent');
    assertNumber(metric.value, `${metricPath}.value`);
    assertEnum(metric.unit, NATIVE_METRIC_UNITS, `${metricPath}.unit`);
  }
}

export function isEvaluationSummary(value: unknown): asserts value is EvaluationSummary {
  assertObject(value, 'evaluation_summary');
  rejectUnknown(value, [...ENVELOPE_FIELDS, 'run_id', 'campaign_id', 'suite_id', 'arm', 'evaluation_unit', 'stack_id', 'primary_invocation_share', 'correlated_failure_rate', 'benchmark_unavailable_reasons', 'model_role_mapping', 'lifecycle_state', 'assignment_total', 'assignment_completed', 'assignment_failed', 'terminal_outcomes', 'started_at', 'ended_at', 'elapsed_seconds', 'verifier_state', 'verifier_failure_summary', 'headline_metrics', 'evidence_link', 'native_result'], 'evaluation_summary');
  assertEnvelope(value, 'evaluation_summary', ['evaluation_summary']);
  assertString(value.run_id, 'evaluation_summary.run_id');
  assertOptional(value.campaign_id, 'evaluation_summary.campaign_id', assertString);
  assertString(value.suite_id, 'evaluation_summary.suite_id');
  assertString(value.arm, 'evaluation_summary.arm');
  assertOptional(value.evaluation_unit, 'evaluation_summary.evaluation_unit', (v, p) => assertEnum(v, EVALUATION_UNITS, p));
  assertOptional(value.stack_id, 'evaluation_summary.stack_id', assertString);
  assertOptionalMetricValue(value.primary_invocation_share, 'evaluation_summary.primary_invocation_share');
  assertOptionalMetricValue(value.correlated_failure_rate, 'evaluation_summary.correlated_failure_rate');
  assertOptional(value.benchmark_unavailable_reasons, 'evaluation_summary.benchmark_unavailable_reasons', assertStringArray);
  if (value.model_role_mapping !== undefined) {
    assertObject(value.model_role_mapping, 'evaluation_summary.model_role_mapping');
    rejectUnknown(value.model_role_mapping, MODEL_ROLES, 'evaluation_summary.model_role_mapping');
    for (const [role, variant] of Object.entries(value.model_role_mapping)) assertString(variant, `evaluation_summary.model_role_mapping.${role}`);
  }
  assertEnum(value.lifecycle_state, LIFECYCLE_STATUSES, 'evaluation_summary.lifecycle_state');
  assertInteger(value.assignment_total, 'evaluation_summary.assignment_total');
  assertInteger(value.assignment_completed, 'evaluation_summary.assignment_completed');
  assertInteger(value.assignment_failed, 'evaluation_summary.assignment_failed');
  assert(value.assignment_total >= 0 && value.assignment_completed >= 0 && value.assignment_failed >= 0, 'evaluation_summary', 'assignment counts must be >= 0');
  assert(value.assignment_completed + value.assignment_failed <= value.assignment_total, 'evaluation_summary', 'assignment counts are inconsistent');
  assertObject(value.terminal_outcomes, 'evaluation_summary.terminal_outcomes');
  assertOptional(value.started_at, 'evaluation_summary.started_at', assertString);
  assertOptional(value.ended_at, 'evaluation_summary.ended_at', assertString);
  assertOptional(value.elapsed_seconds, 'evaluation_summary.elapsed_seconds', assertNumber);
  assertEnum(value.verifier_state, VERIFIER_STATES, 'evaluation_summary.verifier_state');
  assertOptional(value.verifier_failure_summary, 'evaluation_summary.verifier_failure_summary', assertString);
  assertObject(value.headline_metrics, 'evaluation_summary.headline_metrics');
  assertOptional(value.evidence_link, 'evaluation_summary.evidence_link', assertString);
  if (value.native_result !== undefined) {
    assert(value.schema_version === '1.3.0', 'evaluation_summary.schema_version', 'native results require schema 1.3.0');
    assert(value.evaluation_unit === 'system', 'evaluation_summary.evaluation_unit', 'native results require system evaluation');
    assert(value.model_role_mapping === undefined || Object.keys(value.model_role_mapping).length === 0, 'evaluation_summary.model_role_mapping', 'native results cannot claim model roles');
    assertNativeResult(value.native_result, 'evaluation_summary.native_result');
  } else {
    assertObject(value.model_role_mapping, 'evaluation_summary.model_role_mapping');
  }
}

export function isAssignmentResult(value: unknown): asserts value is AssignmentResult {
  assertObject(value, 'assignment_result');
  rejectUnknown(value, [...ENVELOPE_FIELDS, 'assignment_id', 'run_id', 'task_id', 'variant_id', 'role', 'repetition', 'scenario_category', 'evaluation_unit', 'stack_id', 'benchmark_observations', 'terminal_status', 'metric_values', 'missingness_reason', 'stage_summary', 'resource_summary', 'verification_disposition'], 'assignment_result');
  assertEnvelope(value, 'assignment_result', ['assignment_result']);
  assertString(value.assignment_id, 'assignment_result.assignment_id');
  assertString(value.run_id, 'assignment_result.run_id');
  assertString(value.task_id, 'assignment_result.task_id');
  assertString(value.variant_id, 'assignment_result.variant_id');
  assertEnum(value.role, MODEL_ROLES, 'assignment_result.role');
  assertInteger(value.repetition, 'assignment_result.repetition');
  assertOptional(value.scenario_category, 'assignment_result.scenario_category', (v, p) => assertEnum(v, SCENARIO_CATEGORIES, p));
  assertOptional(value.evaluation_unit, 'assignment_result.evaluation_unit', (v, p) => assertEnum(v, EVALUATION_UNITS, p));
  assertOptional(value.stack_id, 'assignment_result.stack_id', assertString);
  if (value.benchmark_observations !== undefined) assertBenchmarkObservations(value.benchmark_observations, 'assignment_result.benchmark_observations');
  assertEnum(value.terminal_status, [...TERMINAL_STATUSES, 'running', 'queued'], 'assignment_result.terminal_status');
  assertObject(value.metric_values, 'assignment_result.metric_values');
  for (const [key, metric] of Object.entries(value.metric_values)) assertMetricValue(metric, `assignment_result.metric_values.${key}`);
  assertOptional(value.missingness_reason, 'assignment_result.missingness_reason', assertString);
  assert(Array.isArray(value.stage_summary), 'assignment_result.stage_summary', 'expected array');
  if (value.resource_summary !== undefined) {
    assertObject(value.resource_summary, 'assignment_result.resource_summary');
  }
  assertOptional(value.verification_disposition, 'assignment_result.verification_disposition', (v, p) =>
    assertEnum(v, VERIFIER_STATES, p),
  );
}

export function isMethodologySnapshot(value: unknown): asserts value is MethodologySnapshot {
  assertObject(value, 'methodology_snapshot');
  assertEnvelope(value, 'methodology_snapshot', ['methodology_snapshot']);
  assert(Array.isArray(value.metric_definitions), 'methodology_snapshot.metric_definitions', 'expected array');
  assert(Array.isArray(value.suite_definitions), 'methodology_snapshot.suite_definitions', 'expected array');
  assert(Array.isArray(value.limitations), 'methodology_snapshot.limitations', 'expected array');
}

export function isLiveEvent(value: unknown): asserts value is LiveEvent {
  assertObject(value, 'live_event');
  assertEnvelope(value, 'live_event', LIVE_EVENT_KINDS as unknown as string[]);
  assertString(value.event_id, 'live_event.event_id');
  assertString(value.run_id, 'live_event.run_id');
  assertOptional(value.assignment_id, 'live_event.assignment_id', assertString);
  assertOptional(value.task_id, 'live_event.task_id', assertString);
  assertOptional(value.variant_id, 'live_event.variant_id', assertString);
  assertOptional(value.role, 'live_event.role', (role) => assertEnum(role, MODEL_ROLES, 'live_event.role'));
  assertEnum(value.lifecycle_status, LIFECYCLE_STATUSES, 'live_event.lifecycle_status');
  assertInteger(value.completed, 'live_event.completed');
  assertInteger(value.total, 'live_event.total');
  assertOptional(value.stage_label, 'live_event.stage_label', assertString);
  if (value.metric_delta !== undefined) assertObject(value.metric_delta, 'live_event.metric_delta');
  assertString(value.observed_at, 'live_event.observed_at');
}

/** Decode and validate a snapshot record or live event from a projection. */
export function decodeViewRecord(kind: string, payload: unknown): SnapshotRecord | LiveEvent {
  switch (kind) {
    case 'catalog_snapshot':
      isCatalogSnapshot(payload);
      return payload as CatalogSnapshot;
    case 'model_summary':
      isModelSummary(payload);
      return payload as ModelSummary;
    case 'suite_summary':
      isSuiteSummary(payload);
      return payload as SuiteSummary;
    case 'evaluation_summary':
      if (isObject(payload)) {
        payload = {
          ...payload,
          verifier_failure_summary: normalizeVerifierFailureSummary(payload.verifier_failure_summary),
        };
      }
      isEvaluationSummary(payload);
      return payload as EvaluationSummary;
    case 'assignment_result':
      isAssignmentResult(payload);
      return payload as AssignmentResult;
    case 'methodology_snapshot':
      isMethodologySnapshot(payload);
      return payload as MethodologySnapshot;
    default:
      if ((LIVE_EVENT_KINDS as readonly string[]).includes(kind)) {
        isLiveEvent(payload);
        return payload as LiveEvent;
      }
      throw new ValidationError(`unknown record kind "${kind}"`, 'decode');
  }
}

export function isProjectionRecord(value: unknown): asserts value is ProjectionRecord {
  assertObject(value, 'projection');
  rejectUnknown(value, ['sequence', 'record_type', 'record_hash', 'record_bytes', 'decoded'], 'projection');
  assertInteger(value.sequence, 'projection.sequence');
  assert(value.sequence >= 1, 'projection.sequence', 'must be >= 1');
  assertEnum(value.record_type, FEED_RECORD_TYPES, 'projection.record_type');
  assertOptional(value.record_hash, 'projection.record_hash', assertString);
  assertString(value.record_bytes, 'projection.record_bytes');
  if (value.decoded !== undefined) {
    assertObject(value.decoded, 'projection.decoded');
  }
}

export function isFeedSnapshot(value: unknown): asserts value is FeedSnapshot {
  assertObject(value, 'feed_snapshot');
  rejectUnknown(value, ['protocol_version', 'source_id', 'high_water_sequence', 'feed_chain_hash', 'batch_count', 'generated_at', 'freshness'], 'feed_snapshot');
  assertString(value.protocol_version, 'feed_snapshot.protocol_version');
  assert(value.protocol_version === '1.0.0', 'feed_snapshot.protocol_version', 'expected 1.0.0');
  assertString(value.source_id, 'feed_snapshot.source_id');
  assertInteger(value.high_water_sequence, 'feed_snapshot.high_water_sequence');
  assert(value.high_water_sequence >= 0, 'feed_snapshot.high_water_sequence', 'must be >= 0');
  assertString(value.feed_chain_hash, 'feed_snapshot.feed_chain_hash');
  assert(/^[0-9a-f]{64}$/.test(value.feed_chain_hash), 'feed_snapshot.feed_chain_hash', 'expected sha256 hex');
  assertInteger(value.batch_count, 'feed_snapshot.batch_count');
  assert(value.batch_count >= 0, 'feed_snapshot.batch_count', 'must be >= 0');
  assertString(value.generated_at, 'feed_snapshot.generated_at');
  assertEnum(value.freshness, FRESHNESS_STATES, 'feed_snapshot.freshness');
}

export function isFeedBootstrap(value: unknown): asserts value is FeedBootstrap {
  assertObject(value, 'feed_bootstrap');
  assertString(value.protocol_version, 'feed_bootstrap.protocol_version');
  assert(value.protocol_version === '1.0.0', 'feed_bootstrap.protocol_version', 'expected 1.0.0');
  isFeedSnapshot(value.snapshot);
  assertEnum(value.source_freshness, FRESHNESS_STATES, 'feed_bootstrap.source_freshness');
  assert(value.source_freshness === value.snapshot.freshness, 'feed_bootstrap.source_freshness', 'must match snapshot.freshness');
  assert(Array.isArray(value.recent_projections), 'feed_bootstrap.recent_projections', 'expected array');
  assertObject(value.proof_catalog_summary, 'feed_bootstrap.proof_catalog_summary');
  assertInteger(value.proof_catalog_summary.artifact_count, 'feed_bootstrap.proof_catalog_summary.artifact_count');
  assert(value.proof_catalog_summary.artifact_count >= 0, 'feed_bootstrap.proof_catalog_summary.artifact_count', 'must be >= 0');
  assertString(value.generated_at, 'feed_bootstrap.generated_at');
}

export function isFeedHistoryPage(value: unknown): asserts value is FeedHistoryPage {
  assertObject(value, 'feed_history');
  assertString(value.protocol_version, 'feed_history.protocol_version');
  assert(Array.isArray(value.items), 'feed_history.items', 'expected array');
  assertBoolean(value.has_more, 'feed_history.has_more');
  assertInteger(value.limit, 'feed_history.limit');
  if (value.cursor !== undefined) assertString(value.cursor, 'feed_history.cursor');
}

/** Validate a snapshot record kind against the closed set. */
export function isSnapshotKind(value: unknown): value is SnapshotRecord['kind'] {
  return isEnum(value, SNAPSHOT_KINDS);
}
