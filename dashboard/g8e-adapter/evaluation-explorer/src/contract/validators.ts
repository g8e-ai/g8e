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
  ACTIVITY_AVAILABILITIES,
  PUBLIC_ACTIVITY_EVIDENCE_SOURCES,
  PUBLIC_EVIDENCE_KINDS,
  PUBLIC_FINISH_STATES,
  PUBLIC_GRADE_EXPLANATION_CODES,
  PUBLIC_LOAD_STATES,
  PUBLIC_RECEIPT_STATUSES,
  PUBLIC_SEMANTIC_OUTCOMES,
  PUBLIC_TOOL_EXECUTION_OUTCOMES,
  PUBLIC_TOOL_OUTCOMES,
  PUBLIC_USAGE_AVAILABILITIES,
  PUBLIC_UNAVAILABLE_REASONS,
  SUPPORTED_VIEW_SCHEMA_VERSIONS,
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

function assertPublicIdentifier(value: unknown, path: string): asserts value is string {
  assertString(value, path);
  assert(new TextEncoder().encode(value).byteLength <= 128, path, 'expected at most 128 UTF-8 bytes');
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

function assertStringArray(value: unknown, path: string, maxItems = 128, maxBytes = 512): asserts value is string[] {
  assert(Array.isArray(value), path, 'expected array');
  assert(value.length <= maxItems, path, `expected at most ${maxItems} entries`);
  for (let i = 0; i < value.length; i++) {
    const itemPath = `${path}[${i}]`;
    assertString(value[i], itemPath);
    assert(new TextEncoder().encode(value[i]).byteLength <= maxBytes, itemPath, `expected at most ${maxBytes} UTF-8 bytes`);
    for (const character of value[i]) {
      const code = character.codePointAt(0) ?? 0;
      assert(code > 0x1f && code !== 0x7f, itemPath, 'control characters are not allowed');
    }
  }
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
  assert((SUPPORTED_VIEW_SCHEMA_VERSIONS as readonly string[]).includes(value.schema_version), `${path}.schema_version`, `expected a supported schema version`);
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
  const hasValue = value.value !== undefined;
  const hasReason = value.unavailable_reason !== undefined;
  if (hasValue === hasReason) {
    throw new ValidationError('metric requires exactly one value or unavailable_reason', path);
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
    const summary: unknown = value[i];
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
  rejectUnknown(value, [...ENVELOPE_FIELDS, 'variant_id', 'display_name', 'served_model_tag', 'role', 'backend_provider_class', 'quantization_weight_class', 'inventory_only', 'evaluation_coverage', 'pass_rate', 'agreement_pairwise', 'agreement_all_five', 'repeatability', 'latency_p50_ms', 'latency_p95_ms', 'output_throughput_p50', 'output_throughput_p95', 'input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'terminal_outcomes', 'unavailable_reasons'], 'model_summary');
  assertEnvelope(value, 'model_summary', ['model_summary']);
  assertString(value.variant_id, 'model_summary.variant_id');
  assertString(value.display_name, 'model_summary.display_name');
  assertOptional(value.served_model_tag, 'model_summary.served_model_tag', assertString);
  assertEnum(value.role, MODEL_ROLES, 'model_summary.role');
  assertOptional(value.backend_provider_class, 'model_summary.backend_provider_class', assertString);
  assertOptional(value.quantization_weight_class, 'model_summary.quantization_weight_class', assertString);
  assertBoolean(value.inventory_only, 'model_summary.inventory_only');
  assertNumber(value.evaluation_coverage, 'model_summary.evaluation_coverage');
  assert(value.evaluation_coverage >= 0 && value.evaluation_coverage <= 1, 'model_summary.evaluation_coverage', 'must be between zero and one');
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
    for (const key of ['consistently_correct', 'consistently_wrong', 'inconsistent', 'insufficient']) {
      assert((value.repeatability[key] as number) >= 0, `model_summary.repeatability.${key}`, 'must be >= 0');
    }
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
    rejectUnknown(value.terminal_outcomes, TERMINAL_STATUSES, 'model_summary.terminal_outcomes');
    for (const [key, count] of Object.entries(value.terminal_outcomes)) {
      assertInteger(count, `model_summary.terminal_outcomes.${key}`);
      assert(count >= 0, `model_summary.terminal_outcomes.${key}`, 'must be >= 0');
    }
  }
  if (value.unavailable_reasons !== undefined) assertStringArray(value.unavailable_reasons, 'model_summary.unavailable_reasons');
}

export function isSuiteSummary(value: unknown): asserts value is SuiteSummary {
  assertObject(value, 'suite_summary');
  rejectUnknown(value, [...ENVELOPE_FIELDS, 'suite_id', 'display_name', 'task_count', 'assignment_count', 'status', 'verifier_state', 'verifier_failure_summary', 'model_coverage', 'metric_summaries', 'limitations'], 'suite_summary');
  assertEnvelope(value, 'suite_summary', ['suite_summary']);
  assertString(value.suite_id, 'suite_summary.suite_id');
  assertString(value.display_name, 'suite_summary.display_name');
  assertInteger(value.task_count, 'suite_summary.task_count');
  assert(value.task_count >= 0, 'suite_summary.task_count', 'must be >= 0');
  assertInteger(value.assignment_count, 'suite_summary.assignment_count');
  assert(value.assignment_count >= 0, 'suite_summary.assignment_count', 'must be >= 0');
  assertEnum(value.status, LIFECYCLE_STATUSES, 'suite_summary.status');
  assertEnum(value.verifier_state, VERIFIER_STATES, 'suite_summary.verifier_state');
  assertOptional(value.verifier_failure_summary, 'suite_summary.verifier_failure_summary', assertString);
  assertStringArray(value.model_coverage, 'suite_summary.model_coverage');
  assertObject(value.metric_summaries, 'suite_summary.metric_summaries');
  for (const [key, metric] of Object.entries(value.metric_summaries)) assertMetricValue(metric, `suite_summary.metric_summaries.${key}`);
  assertStringArray(value.limitations, 'suite_summary.limitations');
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

function assertPublicMetricValue(value: unknown, path: string): void {
  assertMetricValue(value, path);
  if (isObject(value) && value.unavailable_reason !== undefined) {
    assertEnum(value.unavailable_reason, PUBLIC_UNAVAILABLE_REASONS, `${path}.unavailable_reason`);
  }
}

function assertPublicStringArray(value: unknown, path: string, maxItems: number, maxBytes = 128): void {
  assert(Array.isArray(value), path, 'expected array');
  assert(value.length <= maxItems, path, `expected at most ${maxItems} entries`);
  for (let index = 0; index < value.length; index++) {
    assertString(value[index], `${path}[${index}]`);
    const item = value[index] as string;
    assert(new TextEncoder().encode(item).byteLength <= maxBytes, `${path}[${index}]`, `expected at most ${maxBytes} UTF-8 bytes`);
    assert(!Array.from(item).some((character) => {
      const codePoint = character.codePointAt(0) ?? 0;
      return codePoint <= 0x1f || codePoint === 0x7f;
    }), `${path}[${index}]`, 'control characters are not allowed');
  }
}

function assertPublicScenarioSummary(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['scenario_id', 'scenario_version', 'category', 'public_description', 'grading_method', 'allowed_tools', 'expected_tools', 'forbidden_tools', 'criteria', 'tool_score_dimensions'], path);
  assertPublicIdentifier(value.scenario_id, `${path}.scenario_id`);
  assertPublicIdentifier(value.scenario_version, `${path}.scenario_version`);
  assertEnum(value.category, SCENARIO_CATEGORIES, `${path}.category`);
  assertString(value.public_description, `${path}.public_description`);
  assert(new TextEncoder().encode(value.public_description).byteLength <= 512, `${path}.public_description`, 'expected at most 512 UTF-8 bytes');
  assertEnum(value.grading_method, ['deterministic', 'semantic_judge'], `${path}.grading_method`);
  for (const field of ['allowed_tools', 'expected_tools', 'forbidden_tools'] as const) {
    assertPublicStringArray(value[field], `${path}.${field}`, 64);
  }
  assert(Array.isArray(value.criteria), `${path}.criteria`, 'expected array');
  assert(value.criteria.length <= 64, `${path}.criteria`, 'expected at most 64 entries');
  const criterionIds = new Set<string>();
  for (let index = 0; index < value.criteria.length; index++) {
    const criterionPath = `${path}.criteria[${index}]`;
    const criterion: unknown = value.criteria[index];
    assertObject(criterion, criterionPath);
    rejectUnknown(criterion, ['criterion_id', 'public_label', 'public_description', 'grading_method', 'required'], criterionPath);
    assertPublicIdentifier(criterion.criterion_id, `${criterionPath}.criterion_id`);
    assert(!criterionIds.has(criterion.criterion_id), `${criterionPath}.criterion_id`, 'duplicate criterion id');
    criterionIds.add(criterion.criterion_id);
    assertPublicIdentifier(criterion.public_label, `${criterionPath}.public_label`);
    assertString(criterion.public_description, `${criterionPath}.public_description`);
    assert(new TextEncoder().encode(criterion.public_description).byteLength <= 512, `${criterionPath}.public_description`, 'expected at most 512 UTF-8 bytes');
    assertEnum(criterion.grading_method, ['deterministic', 'semantic_judge'], `${criterionPath}.grading_method`);
    assertBoolean(criterion.required, `${criterionPath}.required`);
  }
  assert(Array.isArray(value.tool_score_dimensions), `${path}.tool_score_dimensions`, 'expected array');
  assert(value.tool_score_dimensions.length <= 64, `${path}.tool_score_dimensions`, 'expected at most 64 entries');
  for (let index = 0; index < value.tool_score_dimensions.length; index++) {
    const dimensionPath = `${path}.tool_score_dimensions[${index}]`;
    const dimension: unknown = value.tool_score_dimensions[index];
    assertObject(dimension, dimensionPath);
    rejectUnknown(dimension, ['dimension', 'required'], dimensionPath);
    assertEnum(dimension.dimension, TOOL_SCORE_DIMENSIONS, `${dimensionPath}.dimension`);
    assertBoolean(dimension.required, `${dimensionPath}.required`);
  }
}

function assertPublicActivityFamily(value: unknown, path: string, validateRecord: (value: unknown, path: string) => void): void {
  assertObject(value, path);
  rejectUnknown(value, ['availability', 'unavailable_reason', 'records'], path);
  assertEnum(value.availability, ACTIVITY_AVAILABILITIES, `${path}.availability`);
  if (value.availability === 'observed') {
    assert(value.unavailable_reason === undefined, `${path}.unavailable_reason`, 'observed activity cannot have unavailable_reason');
  } else {
    assertEnum(value.unavailable_reason, PUBLIC_UNAVAILABLE_REASONS, `${path}.unavailable_reason`);
    if (value.availability === 'not_applicable') assert(value.unavailable_reason === 'scenario_not_applicable', `${path}.unavailable_reason`, 'not_applicable activity requires scenario_not_applicable');
  }
  assert(Array.isArray(value.records), `${path}.records`, 'expected array');
  assert(value.records.length <= 128, `${path}.records`, 'expected at most 128 entries');
  for (let index = 0; index < value.records.length; index++) validateRecord(value.records[index], `${path}.records[${index}]`);
  if (value.availability !== 'observed') assert(value.records.length === 0, `${path}.records`, 'unavailable activity cannot contain records');
}

function assertPublicModelActivityRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['model_role', 'agent_persona', 'variant_id', 'usage_availability', 'input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'total_duration_nanos', 'generation_duration_nanos', 'retry_count', 'finish_state', 'load_state'], path);
  assertEnum(value.model_role, MODEL_ROLES, `${path}.model_role`);
  assertOptional(value.agent_persona, `${path}.agent_persona`, assertPublicIdentifier);
  assertPublicIdentifier(value.variant_id, `${path}.variant_id`);
  assertEnum(value.usage_availability, PUBLIC_USAGE_AVAILABILITIES, `${path}.usage_availability`);
  for (const field of ['input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'total_duration_nanos', 'generation_duration_nanos'] as const) {
    if (value[field] !== undefined) {
      assertPublicMetricValue(value[field], `${path}.${field}`);
      if (isObject(value[field]) && typeof value[field].value === 'number' && value[field].value !== undefined) {
        assert(Number.isSafeInteger(value[field].value), `${path}.${field}.value`, 'token and duration values must be safe integers');
      }
    }
  }
  if (value.retry_count !== undefined) {
    assertPublicMetricValue(value.retry_count, `${path}.retry_count`);
    if (isObject(value.retry_count) && typeof value.retry_count.value === 'number') assert(value.retry_count.value <= 1000, `${path}.retry_count.value`, 'must be <= 1000');
  }
  assertEnum(value.finish_state, PUBLIC_FINISH_STATES, `${path}.finish_state`);
  assertEnum(value.load_state, PUBLIC_LOAD_STATES, `${path}.load_state`);
}

function assertPublicToolDecisionRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['tool_label', 'recognized', 'selected', 'permission_compliant', 'unnecessary', 'outcome', 'evidence_source'], path);
  assertPublicIdentifier(value.tool_label, `${path}.tool_label`);
  assertBoolean(value.recognized, `${path}.recognized`);
  assertBoolean(value.selected, `${path}.selected`);
  assertBoolean(value.permission_compliant, `${path}.permission_compliant`);
  assertBoolean(value.unnecessary, `${path}.unnecessary`);
  assertEnum(value.outcome, PUBLIC_SEMANTIC_OUTCOMES, `${path}.outcome`);
  assert(value.evidence_source === 'application_reported', `${path}.evidence_source`, 'tool decisions require application-reported evidence');
}

function assertPublicToolCallRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['tool_label', 'execution_outcome', 'semantic_outcome', 'evidence_source'], path);
  assertPublicIdentifier(value.tool_label, `${path}.tool_label`);
  assertEnum(value.execution_outcome, PUBLIC_TOOL_EXECUTION_OUTCOMES, `${path}.execution_outcome`);
  assertEnum(value.semantic_outcome, PUBLIC_SEMANTIC_OUTCOMES, `${path}.semantic_outcome`);
  assert(value.evidence_source === 'application_reported', `${path}.evidence_source`, 'tool calls require application-reported evidence');
}

function assertPublicPolicyDecisionRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['tool_label', 'outcome', 'evidence_source'], path);
  assertPublicIdentifier(value.tool_label, `${path}.tool_label`);
  assertEnum(value.outcome, PUBLIC_TOOL_OUTCOMES, `${path}.outcome`);
  assert(value.evidence_source === 'application_reported', `${path}.evidence_source`, 'policy decisions require application-reported evidence');
}

function assertPublicGovernedActionRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['action_label', 'reported_policy_outcome', 'receipt_status', 'evidence_source'], path);
  assert(value.action_label === 'governed action', `${path}.action_label`, 'unexpected governed action label');
  assertEnum(value.reported_policy_outcome, PUBLIC_TOOL_OUTCOMES, `${path}.reported_policy_outcome`);
  assertEnum(value.receipt_status, PUBLIC_RECEIPT_STATUSES, `${path}.receipt_status`);
  assertEnum(value.evidence_source, PUBLIC_ACTIVITY_EVIDENCE_SOURCES, `${path}.evidence_source`);
  if (value.receipt_status === 'reported') assert(value.evidence_source === 'bound_public_proof', `${path}.evidence_source`, 'reported receipt status requires bound public proof');
}

function assertPublicActivitySummary(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['model_activity', 'tool_decisions', 'tool_calls', 'policy_decisions', 'governed_actions'], path);
  assertPublicActivityFamily(value.model_activity, `${path}.model_activity`, assertPublicModelActivityRecord);
  assertPublicActivityFamily(value.tool_decisions, `${path}.tool_decisions`, assertPublicToolDecisionRecord);
  assertPublicActivityFamily(value.tool_calls, `${path}.tool_calls`, assertPublicToolCallRecord);
  assertPublicActivityFamily(value.policy_decisions, `${path}.policy_decisions`, assertPublicPolicyDecisionRecord);
  assertPublicActivityFamily(value.governed_actions, `${path}.governed_actions`, assertPublicGovernedActionRecord);
}

function assertPublicEvidenceBindings(value: unknown, path: string): void {
  assert(Array.isArray(value), path, 'expected array');
  assert(value.length <= 32, path, 'expected at most 32 entries');
  const identities = new Set<string>();
  for (let index = 0; index < value.length; index++) {
    const bindingPath = `${path}[${index}]`;
    const binding: unknown = value[index];
    assertObject(binding, bindingPath);
    rejectUnknown(binding, ['sha256', 'schema_ref', 'kind'], bindingPath);
    assertString(binding.sha256, `${bindingPath}.sha256`);
    assert(/^[0-9a-f]{64}$/u.test(binding.sha256), `${bindingPath}.sha256`, 'expected lowercase SHA-256');
    assertString(binding.schema_ref, `${bindingPath}.schema_ref`);
    assert(new TextEncoder().encode(binding.schema_ref).byteLength <= 128, `${bindingPath}.schema_ref`, 'expected at most 128 UTF-8 bytes');
    assertEnum(binding.kind, PUBLIC_EVIDENCE_KINDS, `${bindingPath}.kind`);
    assert(!identities.has(binding.sha256), `${bindingPath}.sha256`, 'duplicate evidence identity');
    identities.add(binding.sha256);
  }
}

function assertPublicVerificationMetadata(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['provenance', 'verifier_state', 'verifier_release_version', 'verifier_contract_version', 'report_digest', 'population_digest'], path);
  assertEnum(value.provenance, ['bound', 'legacy_unbound'], `${path}.provenance`);
  assertEnum(value.verifier_state, VERIFIER_STATES, `${path}.verifier_state`);
  assertOptional(value.verifier_release_version, `${path}.verifier_release_version`, assertString);
  assertOptional(value.verifier_contract_version, `${path}.verifier_contract_version`, assertString);
  for (const field of ['report_digest', 'population_digest'] as const) {
    if (value[field] !== undefined) {
      assertString(value[field], `${path}.${field}`);
      assert(/^[0-9a-f]{64}$/u.test(value[field]), `${path}.${field}`, 'expected lowercase SHA-256');
    }
  }
  if (value.provenance === 'bound') {
    assertString(value.verifier_release_version, `${path}.verifier_release_version`);
    assertString(value.verifier_contract_version, `${path}.verifier_contract_version`);
    assertString(value.report_digest, `${path}.report_digest`);
    assertString(value.population_digest, `${path}.population_digest`);
  }
}

function assertPublicSemanticGrades(value: unknown, path: string): void {
  assert(Array.isArray(value), path, 'expected array');
  assert(value.length <= 64, path, 'expected at most 64 entries');
  const criterionIds = new Set<string>();
  for (let index = 0; index < value.length; index++) {
    const gradePath = `${path}[${index}]`;
    const grade: unknown = value[index];
    assertObject(grade, gradePath);
    rejectUnknown(grade, ['criterion_id', 'status', 'grading_method', 'judge_variant_id', 'explanation_code'], gradePath);
    assertPublicIdentifier(grade.criterion_id, `${gradePath}.criterion_id`);
    assert(!criterionIds.has(grade.criterion_id), `${gradePath}.criterion_id`, 'duplicate criterion id');
    criterionIds.add(grade.criterion_id);
    assertEnum(grade.status, NATIVE_RESULT_STATUSES, `${gradePath}.status`);
    assertEnum(grade.grading_method, ['deterministic', 'semantic_judge'], `${gradePath}.grading_method`);
    assertOptional(grade.judge_variant_id, `${gradePath}.judge_variant_id`, assertPublicIdentifier);
    assertEnum(grade.explanation_code, PUBLIC_GRADE_EXPLANATION_CODES, `${gradePath}.explanation_code`);
  }
}

function assertPublicResourceSummary(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['latency_ms', 'input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'retries'], path);
  for (const [key, metric] of Object.entries(value)) {
    assertPublicMetricValue(metric, `${path}.${key}`);
    if (['input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'retries'].includes(key) && isObject(metric) && typeof metric.value === 'number' && metric.value !== undefined) {
      assert(Number.isSafeInteger(metric.value), `${path}.${key}.value`, 'token and retry values must be safe integers');
    }
    if (key === 'retries' && isObject(metric) && typeof metric.value === 'number' && metric.value !== undefined) {
      assert(metric.value <= 1000, `${path}.${key}.value`, 'must be <= 1000');
    }
  }
}

export function isAssignmentResult(value: unknown): asserts value is AssignmentResult {
  assertObject(value, 'assignment_result');
  rejectUnknown(value, [...ENVELOPE_FIELDS, 'assignment_id', 'run_id', 'task_id', 'scenario_id', 'variant_id', 'role', 'repetition', 'scenario_category', 'evaluation_unit', 'stack_id', 'scenario_summary', 'semantic_grade_summaries', 'activity_summary', 'evidence_bindings', 'benchmark_observations', 'terminal_status', 'metric_values', 'missingness_reason', 'stage_summary', 'resource_summary', 'verification_disposition', 'verification_metadata'], 'assignment_result');
  assertEnvelope(value, 'assignment_result', ['assignment_result']);
  assertString(value.assignment_id, 'assignment_result.assignment_id');
  assertString(value.run_id, 'assignment_result.run_id');
  assertString(value.task_id, 'assignment_result.task_id');
  if (value.scenario_id !== undefined) {
    assertPublicIdentifier(value.scenario_id, 'assignment_result.scenario_id');
    assert(value.scenario_id === value.task_id, 'assignment_result.scenario_id', 'must equal task_id when both scenario identity fields are present');
  }
  assertString(value.variant_id, 'assignment_result.variant_id');
  assertEnum(value.role, MODEL_ROLES, 'assignment_result.role');
  assertInteger(value.repetition, 'assignment_result.repetition');
  assertOptional(value.scenario_category, 'assignment_result.scenario_category', (v, p) => assertEnum(v, SCENARIO_CATEGORIES, p));
  assertOptional(value.evaluation_unit, 'assignment_result.evaluation_unit', (v, p) => assertEnum(v, EVALUATION_UNITS, p));
  assertOptional(value.stack_id, 'assignment_result.stack_id', assertString);
  if (value.benchmark_observations !== undefined) assertBenchmarkObservations(value.benchmark_observations, 'assignment_result.benchmark_observations');
  if (value.schema_version !== '1.4.0' && value.schema_version !== '1.5.0') {
    for (const field of ['scenario_summary', 'semantic_grade_summaries', 'activity_summary', 'evidence_bindings', 'verification_metadata'] as const) {
      assert(value[field] === undefined, `assignment_result.${field}`, 'field requires schema 1.4.0 or later');
    }
  } else {
    if (value.scenario_summary !== undefined) assertPublicScenarioSummary(value.scenario_summary, 'assignment_result.scenario_summary');
    if (value.semantic_grade_summaries !== undefined) assertPublicSemanticGrades(value.semantic_grade_summaries, 'assignment_result.semantic_grade_summaries');
    if (value.activity_summary !== undefined) assertPublicActivitySummary(value.activity_summary, 'assignment_result.activity_summary');
    if (value.evidence_bindings !== undefined) assertPublicEvidenceBindings(value.evidence_bindings, 'assignment_result.evidence_bindings');
    if (value.verification_metadata !== undefined) assertPublicVerificationMetadata(value.verification_metadata, 'assignment_result.verification_metadata');
  }
  assertEnum(value.terminal_status, [...TERMINAL_STATUSES, 'running', 'queued'], 'assignment_result.terminal_status');
  assertObject(value.metric_values, 'assignment_result.metric_values');
  for (const [key, metric] of Object.entries(value.metric_values)) assertMetricValue(metric, `assignment_result.metric_values.${key}`);
  assertOptional(value.missingness_reason, 'assignment_result.missingness_reason', assertString);
  assert(Array.isArray(value.stage_summary), 'assignment_result.stage_summary', 'expected array');
  assert(value.stage_summary.length <= 128, 'assignment_result.stage_summary', 'expected at most 128 entries');
  for (let index = 0; index < value.stage_summary.length; index++) {
    const stagePath = `assignment_result.stage_summary[${index}]`;
    const stage = value.stage_summary[index];
    assertObject(stage, stagePath);
    rejectUnknown(stage, ['name', 'duration_seconds'], stagePath);
    assertString(stage.name, `${stagePath}.name`);
    assertNumber(stage.duration_seconds, `${stagePath}.duration_seconds`);
    assert(stage.duration_seconds >= 0 && stage.duration_seconds <= 604800, `${stagePath}.duration_seconds`, 'must be within seven days');
  }
  if (value.resource_summary !== undefined) assertPublicResourceSummary(value.resource_summary, 'assignment_result.resource_summary');
  assertOptional(value.verification_disposition, 'assignment_result.verification_disposition', (v, p) =>
    assertEnum(v, VERIFIER_STATES, p),
  );
}

export function isMethodologySnapshot(value: unknown): asserts value is MethodologySnapshot {
  assertObject(value, 'methodology_snapshot');
  rejectUnknown(value, [...ENVELOPE_FIELDS, 'metric_definitions', 'suite_definitions', 'limitations'], 'methodology_snapshot');
  assertEnvelope(value, 'methodology_snapshot', ['methodology_snapshot']);
  assert(Array.isArray(value.metric_definitions), 'methodology_snapshot.metric_definitions', 'expected array');
  assert(value.metric_definitions.length <= 128, 'methodology_snapshot.metric_definitions', 'expected at most 128 entries');
  for (let index = 0; index < value.metric_definitions.length; index++) {
    const path = `methodology_snapshot.metric_definitions[${index}]`;
    const definition: unknown = value.metric_definitions[index];
    assertObject(definition, path);
    rejectUnknown(definition, ['key', 'name', 'unit', 'direction', 'denominator', 'missing_value_behavior', 'aggregation', 'uncertainty_method', 'explanation'], path);
    for (const field of ['key', 'name', 'unit', 'denominator', 'missing_value_behavior', 'aggregation', 'uncertainty_method', 'explanation'] as const) {
      assertString(definition[field], `${path}.${field}`);
    }
    assertEnum(definition.direction, ['higher_is_better', 'lower_is_better'], `${path}.direction`);
  }
  assert(Array.isArray(value.suite_definitions), 'methodology_snapshot.suite_definitions', 'expected array');
  assert(value.suite_definitions.length <= 128, 'methodology_snapshot.suite_definitions', 'expected at most 128 entries');
  for (let index = 0; index < value.suite_definitions.length; index++) {
    const path = `methodology_snapshot.suite_definitions[${index}]`;
    const definition: unknown = value.suite_definitions[index];
    assertObject(definition, path);
    rejectUnknown(definition, ['suite_id', 'display_name', 'task_count', 'description'], path);
    assertString(definition.suite_id, `${path}.suite_id`);
    assertString(definition.display_name, `${path}.display_name`);
    assertInteger(definition.task_count, `${path}.task_count`);
    assert(definition.task_count >= 0, `${path}.task_count`, 'must be >= 0');
    assertString(definition.description, `${path}.description`);
  }
  assertStringArray(value.limitations, 'methodology_snapshot.limitations');
}

export function isLiveEvent(value: unknown): asserts value is LiveEvent {
  assertObject(value, 'live_event');
  rejectUnknown(value, [...ENVELOPE_FIELDS, 'event_id', 'run_id', 'assignment_id', 'task_id', 'variant_id', 'role', 'lifecycle_status', 'completed', 'total', 'stage_label', 'metric_delta', 'feed_sequence'], 'live_event');
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
  assert(value.completed >= 0, 'live_event.completed', 'must be >= 0');
  assert(value.total >= 0, 'live_event.total', 'must be >= 0');
  assert(value.completed <= value.total, 'live_event', 'completed cannot exceed total');
  assertOptional(value.stage_label, 'live_event.stage_label', assertString);
  if (value.metric_delta !== undefined) {
    assertObject(value.metric_delta, 'live_event.metric_delta');
    for (const [key, metric] of Object.entries(value.metric_delta)) assertMetricValue(metric, `live_event.metric_delta.${key}`);
  }
  assertOptional(value.feed_sequence, 'live_event.feed_sequence', assertInteger);
  if (value.feed_sequence !== undefined) assert(value.feed_sequence >= 0, 'live_event.feed_sequence', 'must be >= 0');
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
  if (value.record_hash !== undefined) {
    assertString(value.record_hash, 'projection.record_hash');
    assert(/^[0-9a-f]{64}$/u.test(value.record_hash), 'projection.record_hash', 'expected sha256 hex');
  }
  assertString(value.record_bytes, 'projection.record_bytes');
  if (value.decoded !== undefined) {
    assertObject(value.decoded, 'projection.decoded');
    assertString(value.decoded.kind, 'projection.decoded.kind');
    decodeViewRecord(value.decoded.kind, value.decoded);
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
  rejectUnknown(value, ['protocol_version', 'snapshot', 'source_freshness', 'recent_projections', 'proof_catalog_summary', 'generated_at'], 'feed_bootstrap');
  assertString(value.protocol_version, 'feed_bootstrap.protocol_version');
  assert(value.protocol_version === '1.0.0', 'feed_bootstrap.protocol_version', 'expected 1.0.0');
  isFeedSnapshot(value.snapshot);
  assertEnum(value.source_freshness, FRESHNESS_STATES, 'feed_bootstrap.source_freshness');
  assert(value.source_freshness === value.snapshot.freshness, 'feed_bootstrap.source_freshness', 'must match snapshot.freshness');
  assert(Array.isArray(value.recent_projections), 'feed_bootstrap.recent_projections', 'expected array');
  for (let index = 0; index < value.recent_projections.length; index++) {
    const item: unknown = value.recent_projections[index];
    assertObject(item, `feed_bootstrap.recent_projections[${index}]`);
    assertInteger(item.sequence, `feed_bootstrap.recent_projections[${index}].sequence`);
    assert(item.sequence >= 1, `feed_bootstrap.recent_projections[${index}].sequence`, 'must be >= 1');
  }
  assertObject(value.proof_catalog_summary, 'feed_bootstrap.proof_catalog_summary');
  rejectUnknown(value.proof_catalog_summary, ['artifact_count', 'total_byte_size', 'last_generated_at'], 'feed_bootstrap.proof_catalog_summary');
  assertInteger(value.proof_catalog_summary.artifact_count, 'feed_bootstrap.proof_catalog_summary.artifact_count');
  assert(value.proof_catalog_summary.artifact_count >= 0, 'feed_bootstrap.proof_catalog_summary.artifact_count', 'must be >= 0');
  assertInteger(value.proof_catalog_summary.total_byte_size, 'feed_bootstrap.proof_catalog_summary.total_byte_size');
  assert(value.proof_catalog_summary.total_byte_size >= 0, 'feed_bootstrap.proof_catalog_summary.total_byte_size', 'must be >= 0');
  assertOptional(value.proof_catalog_summary.last_generated_at, 'feed_bootstrap.proof_catalog_summary.last_generated_at', assertString);
  assertString(value.generated_at, 'feed_bootstrap.generated_at');
}

export function isFeedHistoryPage(value: unknown): asserts value is FeedHistoryPage {
  assertObject(value, 'feed_history');
  rejectUnknown(value, ['protocol_version', 'items', 'cursor', 'has_more', 'limit'], 'feed_history');
  assertString(value.protocol_version, 'feed_history.protocol_version');
  assert(value.protocol_version === '1.0.0', 'feed_history.protocol_version', 'expected 1.0.0');
  assert(Array.isArray(value.items), 'feed_history.items', 'expected array');
  for (let index = 0; index < value.items.length; index++) {
    const item: unknown = value.items[index];
    assertObject(item, `feed_history.items[${index}]`);
    assertInteger(item.sequence, `feed_history.items[${index}].sequence`);
    assert(item.sequence >= 1, `feed_history.items[${index}].sequence`, 'must be >= 1');
    assertEnum(item.record_type, FEED_RECORD_TYPES, `feed_history.items[${index}].record_type`);
  }
  assertBoolean(value.has_more, 'feed_history.has_more');
  assertInteger(value.limit, 'feed_history.limit');
  assert(value.limit >= 0, 'feed_history.limit', 'must be >= 0');
  if (value.cursor !== undefined) assertString(value.cursor, 'feed_history.cursor');
}

/** Validate a snapshot record kind against the closed set. */
export function isSnapshotKind(value: unknown): value is SnapshotRecord['kind'] {
  return isEnum(value, SNAPSHOT_KINDS);
}
