// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Runtime contract for the Go campaign projection envelopes. This boundary is
// deliberately separate from the explorer view contract: protobuf enum names,
// uint64 protojson strings, and version-specific wire fields are validated
// before any adapter conversion is allowed.

import { parseWireActivityFamily, WIRE_UNAVAILABLE_REASONS, type WireActivityFamily } from './activity-family';
import { PUBLIC_UNAVAILABLE_REASONS } from './types';
import { ValidationError } from './validators';

export const CAMPAIGN_LIFECYCLE_SCHEMA_VERSION = '1.0.0' as const;
export const CAMPAIGN_RESULT_SCHEMA_VERSIONS = ['1.0.0', '1.1.0'] as const;
export const CAMPAIGN_MESSAGE_TYPES = ['PublicAssignmentLifecycleRecord', 'PublicAssignmentResultProjection'] as const;
export type CampaignMessageType = (typeof CAMPAIGN_MESSAGE_TYPES)[number];
export type CampaignEnvelopeVersion = typeof CAMPAIGN_LIFECYCLE_SCHEMA_VERSION | (typeof CAMPAIGN_RESULT_SCHEMA_VERSIONS)[number];

const LIFECYCLE_STATUSES = [
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_ESCALATED',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_STOPPED',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_GRADER_FAILED',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_POLICY_REJECTED',
  'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNAVAILABLE',
] as const;
const SCENARIO_CATEGORIES = [
  'EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE',
  'EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION',
  'EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT',
  'EVALUATION_SCENARIO_CATEGORY_TECHNICAL_ANALYSIS',
  'EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION',
  'EVALUATION_SCENARIO_CATEGORY_VERIFICATION',
  'EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY',
  'EVALUATION_SCENARIO_CATEGORY_RECOVERY',
  'EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE',
] as const;
const LANES = ['EVALUATION_LANE_PLATFORM', 'EVALUATION_LANE_MODEL_ROLE', 'EVALUATION_LANE_SYSTEM'] as const;
const ROLES = ['MODEL_CAMPAIGN_ROLE_PRIMARY', 'MODEL_CAMPAIGN_ROLE_ASSISTANT', 'MODEL_CAMPAIGN_ROLE_LITE'] as const;
const VERDICT_STATUSES = [
  'EVALUATION_VERDICT_STATUS_PASS',
  'EVALUATION_VERDICT_STATUS_FAIL',
  'EVALUATION_VERDICT_STATUS_UNAVAILABLE',
  'EVALUATION_VERDICT_STATUS_UNSUPPORTED',
  'EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE',
] as const;
const VERIFICATION_STATUSES = ['verified', 'unverified', 'failed', 'invalid'] as const;
const GRADING_METHODS = ['EVALUATION_GRADING_METHOD_DETERMINISTIC', 'EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE'] as const;
const FINISH_STATES = [
  'PUBLIC_FINISH_STATE_STOP',
  'PUBLIC_FINISH_STATE_LENGTH',
  'PUBLIC_FINISH_STATE_TOOL_CALL',
  'PUBLIC_FINISH_STATE_ERROR',
  'PUBLIC_FINISH_STATE_UNAVAILABLE',
] as const;
const LOAD_STATES = ['EVALUATION_LOAD_STATE_COLD', 'EVALUATION_LOAD_STATE_WARM', 'EVALUATION_LOAD_STATE_UNAVAILABLE'] as const;
const USAGE_AVAILABILITIES = ['EVALUATION_USAGE_AVAILABILITY_REPORTED', 'EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE'] as const;
const POLICY_OUTCOMES = [
  'EVALUATION_POLICY_DECISION_OUTCOME_ALLOW',
  'EVALUATION_POLICY_DECISION_OUTCOME_DENY',
  'EVALUATION_POLICY_DECISION_OUTCOME_REFUSED',
] as const;
const EXECUTION_OUTCOMES = [
  'EVALUATION_VERDICT_STATUS_PASS',
  'EVALUATION_VERDICT_STATUS_FAIL',
  'EVALUATION_VERDICT_STATUS_UNAVAILABLE',
  'EVALUATION_VERDICT_STATUS_UNSUPPORTED',
  'EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE',
] as const;
const SEMANTIC_OUTCOMES = EXECUTION_OUTCOMES;
const RECEIPT_STATUSES = ['PUBLIC_RECEIPT_STATUS_UNAVAILABLE', 'PUBLIC_RECEIPT_STATUS_REPORTED'] as const;
const EVIDENCE_SOURCES = ['PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED', 'PUBLIC_EVIDENCE_SOURCE_BOUND_PUBLIC_PROOF'] as const;
const EXPLANATION_CODES = [
  'PUBLIC_GRADE_EXPLANATION_CODE_CRITERION_PASSED',
  'PUBLIC_GRADE_EXPLANATION_CODE_CRITERION_FAILED',
  'PUBLIC_GRADE_EXPLANATION_CODE_EVIDENCE_UNAVAILABLE',
  'PUBLIC_GRADE_EXPLANATION_CODE_UNSUPPORTED',
  'PUBLIC_GRADE_EXPLANATION_CODE_INVALID_EVIDENCE',
  'PUBLIC_GRADE_EXPLANATION_CODE_GRADER_UNAVAILABLE',
] as const;
const EVIDENCE_KINDS = [
  'campaign_profile',
  'model_registry',
  'verification_report',
  'evaluation_projection',
  'comparison_row',
  'efficiency_observation',
  'statistical_analysis',
  'source_manifest',
  'assignment_audit_slice',
  'assignment_audit_vault_key',
] as const;
const TOOL_DIMENSIONS = [
  'PUBLIC_TOOL_SCORE_DIMENSION_TOOL_RECOGNITION',
  'PUBLIC_TOOL_SCORE_DIMENSION_TOOL_SELECTION',
  'PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SCHEMA',
  'PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SEMANTICS',
  'PUBLIC_TOOL_SCORE_DIMENSION_PERMISSION_COMPLIANCE',
  'PUBLIC_TOOL_SCORE_DIMENSION_RESULT_INTERPRETATION',
  'PUBLIC_TOOL_SCORE_DIMENSION_FOLLOW_UP_DECISION',
  'PUBLIC_TOOL_SCORE_DIMENSION_UNNECESSARY_TOOL_CALLS',
  'PUBLIC_TOOL_SCORE_DIMENSION_LOOPING',
  'PUBLIC_TOOL_SCORE_DIMENSION_RECOVERY',
  'TOOL_SCORE_DIMENSION_TOOL_RECOGNITION',
  'TOOL_SCORE_DIMENSION_TOOL_SELECTION',
  'TOOL_SCORE_DIMENSION_ARGUMENT_SCHEMA',
  'TOOL_SCORE_DIMENSION_ARGUMENT_SEMANTICS',
  'TOOL_SCORE_DIMENSION_PERMISSION_COMPLIANCE',
  'TOOL_SCORE_DIMENSION_RESULT_INTERPRETATION',
  'TOOL_SCORE_DIMENSION_FOLLOW_UP_DECISION',
  'TOOL_SCORE_DIMENSION_UNNECESSARY_TOOL_CALLS',
  'TOOL_SCORE_DIMENSION_LOOPING',
  'TOOL_SCORE_DIMENSION_RECOVERY',
] as const;

type WireMetricUnavailableReason = (typeof WIRE_UNAVAILABLE_REASONS)[number] | (typeof PUBLIC_UNAVAILABLE_REASONS)[number];

interface WireMetric {
  value?: number;
  unavailable_reason?: WireMetricUnavailableReason;
}

interface WireScore {
  score_id: string;
  dimension?: string;
  value?: number;
  unit?: string;
  direction?: string;
  missing_data_policy?: string;
}

interface WireScenarioCriterion {
  criterion_id: string;
  public_label: string;
  public_description: string;
  grading_method: (typeof GRADING_METHODS)[number];
  required: boolean;
}

interface WireScenarioSummary {
  scenario_id: string;
  scenario_version: string;
  category: (typeof SCENARIO_CATEGORIES)[number];
  public_description: string;
  grading_method: (typeof GRADING_METHODS)[number];
  allowed_tools: string[];
  expected_tools: string[];
  forbidden_tools: string[];
  criteria: WireScenarioCriterion[];
  tool_score_dimensions: Array<{ dimension: (typeof TOOL_DIMENSIONS)[number]; required: boolean }>;
}

interface WireSemanticGradeSummary {
  criterion_id: string;
  status: (typeof VERDICT_STATUSES)[number];
  grading_method: (typeof GRADING_METHODS)[number];
  judge_variant_id?: string;
  explanation_code: (typeof EXPLANATION_CODES)[number];
}

interface WireModelActivityRecord {
  model_role: (typeof ROLES)[number];
  agent_persona?: string;
  variant_id: string;
  usage_availability: (typeof USAGE_AVAILABILITIES)[number];
  input_tokens?: string;
  output_tokens?: string;
  thinking_tokens?: string;
  cache_tokens?: string;
  total_duration_nanos?: string;
  generation_duration_nanos?: string;
  retry_count?: number;
  finish_state: (typeof FINISH_STATES)[number];
  load_state: (typeof LOAD_STATES)[number];
}

interface WireToolDecisionActivityRecord {
  tool_label: string;
  recognized?: boolean;
  selected?: boolean;
  permission_compliant?: boolean;
  unnecessary?: boolean;
  outcome: (typeof SEMANTIC_OUTCOMES)[number];
  evidence_source: (typeof EVIDENCE_SOURCES)[number];
}

interface WireToolCallActivityRecord {
  tool_label: string;
  execution_outcome: (typeof EXECUTION_OUTCOMES)[number];
  semantic_outcome: (typeof SEMANTIC_OUTCOMES)[number];
  evidence_source: (typeof EVIDENCE_SOURCES)[number];
}

interface WirePolicyDecisionActivityRecord {
  tool_label: string;
  outcome: (typeof POLICY_OUTCOMES)[number];
  evidence_source: (typeof EVIDENCE_SOURCES)[number];
}

interface WireGovernedActionActivityRecord {
  action_label: 'governed action';
  reported_policy_outcome: (typeof POLICY_OUTCOMES)[number];
  receipt_status: (typeof RECEIPT_STATUSES)[number];
  evidence_source: (typeof EVIDENCE_SOURCES)[number];
}

interface WireActivitySummary {
  model_activity: WireActivityFamily<WireModelActivityRecord>;
  tool_decisions: WireActivityFamily<WireToolDecisionActivityRecord>;
  tool_calls: WireActivityFamily<WireToolCallActivityRecord>;
  policy_decisions: WireActivityFamily<WirePolicyDecisionActivityRecord>;
  governed_actions: WireActivityFamily<WireGovernedActionActivityRecord>;
}

interface WireEvidenceBinding {
  sha256: string;
  schema_ref: string;
  kind: (typeof EVIDENCE_KINDS)[number];
}

interface WireVerificationMetadata {
  provenance: 'PUBLIC_VERIFICATION_PROVENANCE_BOUND' | 'PUBLIC_VERIFICATION_PROVENANCE_LEGACY_UNBOUND';
  verifier_state: (typeof VERDICT_STATUSES)[number];
  verifier_release_version?: string;
  verifier_contract_version?: string;
  report_digest?: string;
  population_digest?: string;
}

export interface CampaignLifecycleRecord {
  assignment_id: string;
  run_id: string;
  scenario_id: string;
  scenario_category?: (typeof SCENARIO_CATEGORIES)[number];
  lane?: (typeof LANES)[number];
  designated_role?: (typeof ROLES)[number];
  variant_id?: string;
  stack_id?: string;
  lifecycle_status: (typeof LIFECYCLE_STATUSES)[number];
  repetition?: number;
  observed_at: string;
}

export interface CampaignResultRecord {
  assignment_id: string;
  run_id: string;
  scenario_id: string;
  scenario_category?: (typeof SCENARIO_CATEGORIES)[number];
  lane?: (typeof LANES)[number];
  designated_role?: (typeof ROLES)[number];
  variant_id?: string;
  lifecycle_status: (typeof LIFECYCLE_STATUSES)[number];
  summary_status: (typeof VERDICT_STATUSES)[number];
  decomposed_scores?: WireScore[];
  result_digest: string;
  verification_status: (typeof VERIFICATION_STATUSES)[number];
  unavailable_metric_reasons?: string[];
  completed_at: string;
  scenario_summary?: WireScenarioSummary;
  semantic_grade_summaries?: WireSemanticGradeSummary[];
  activity_summary?: WireActivitySummary;
  evidence_bindings?: WireEvidenceBinding[];
  verification_metadata?: WireVerificationMetadata;
  benchmark_observations?: Record<string, unknown>;
  resource_summary?: Record<string, WireMetric>;
}

export interface CampaignProjectionEnvelope {
  schema_version: CampaignEnvelopeVersion;
  message_type: CampaignMessageType;
  idempotency_key: string;
  record: CampaignLifecycleRecord | CampaignResultRecord;
}

const ENVELOPE_FIELDS = ['schema_version', 'message_type', 'idempotency_key', 'record'] as const;
const LIFECYCLE_FIELDS = [
  'assignment_id', 'run_id', 'scenario_id', 'scenario_category', 'lane', 'designated_role', 'variant_id',
  'stack_id', 'lifecycle_status', 'repetition', 'observed_at',
] as const;
const RESULT_FIELDS = [
  'assignment_id', 'run_id', 'scenario_id', 'scenario_category', 'lane', 'designated_role', 'variant_id',
  'lifecycle_status', 'summary_status', 'decomposed_scores', 'result_digest', 'verification_status',
  'unavailable_metric_reasons', 'completed_at', 'scenario_summary', 'semantic_grade_summaries',
  'activity_summary', 'evidence_bindings', 'verification_metadata', 'benchmark_observations', 'resource_summary',
] as const;

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function fail(path: string, message: string): never {
  throw new ValidationError(message, path);
}

function assert(condition: unknown, path: string, message: string): asserts condition {
  if (!condition) fail(path, message);
}

function assertObject(value: unknown, path: string): asserts value is Record<string, unknown> {
  assert(isObject(value), path, 'expected object');
}

function rejectUnknown(value: Record<string, unknown>, fields: readonly string[], path: string): void {
  for (const key of Object.keys(value)) {
    if (!fields.includes(key)) fail(`${path}.${key}`, `unknown field "${key}"`);
  }
}

function assertString(value: unknown, path: string, maxBytes = 128): asserts value is string {
  assert(typeof value === 'string', path, 'expected string');
  const bytes = new TextEncoder().encode(value).byteLength;
  assert(bytes >= 1 && bytes <= maxBytes, path, `expected 1-${maxBytes} UTF-8 bytes`);
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0;
    assert(code > 0x1f && code !== 0x7f, path, 'control characters are not allowed');
  }
}

function assertOptionalString(value: unknown, path: string, maxBytes = 128): void {
  if (value !== undefined) assertString(value, path, maxBytes);
}

function assertEnum<T extends string>(value: unknown, values: readonly T[], path: string): asserts value is T {
  assert(typeof value === 'string' && (values as readonly string[]).includes(value), path, 'unknown enum value');
}

function assertOptionalEnum<T extends string>(value: unknown, values: readonly T[], path: string): void {
  if (value !== undefined) assertEnum(value, values, path);
}

function assertInteger(value: unknown, path: string, max = Number.MAX_SAFE_INTEGER): asserts value is number {
  assert(typeof value === 'number' && Number.isSafeInteger(value), path, 'expected safe integer');
  assert(value >= 0 && value <= max, path, 'expected a bounded nonnegative integer');
}

function assertTimestamp(value: unknown, path: string): asserts value is string {
  assertString(value, path);
  assert(Number.isFinite(Date.parse(value)), path, 'expected an RFC3339 timestamp');
}

function assertHash(value: unknown, path: string): asserts value is string {
  assert(typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value), path, 'expected lowercase SHA-256');
}

function assertMetric(value: unknown, path: string): asserts value is WireMetric {
  assertObject(value, path);
  rejectUnknown(value, ['value', 'unavailable_reason'], path);
  const hasValue = value.value !== undefined;
  const hasReason = value.unavailable_reason !== undefined;
  assert(hasValue !== hasReason, path, 'metric requires exactly one value or unavailable_reason');
  if (hasValue) {
    assert(typeof value.value === 'number' && Number.isFinite(value.value), `${path}.value`, 'expected finite number');
    assert(value.value >= 0, `${path}.value`, 'must be nonnegative');
  }
  if (hasReason) assertEnum(value.unavailable_reason, [...WIRE_UNAVAILABLE_REASONS, ...PUBLIC_UNAVAILABLE_REASONS], `${path}.unavailable_reason`);
}

function assertStringArray(value: unknown, path: string, maxItems: number, maxBytes = 128): asserts value is string[] {
  assert(Array.isArray(value), path, 'expected array');
  assert(value.length <= maxItems, path, `expected at most ${maxItems} entries`);
  for (let index = 0; index < value.length; index++) assertString(value[index], `${path}[${index}]`, maxBytes);
}

function assertBoolean(value: unknown, path: string): asserts value is boolean {
  assert(typeof value === 'boolean', path, 'expected boolean');
}

function assertOptionalBoolean(value: unknown, path: string): asserts value is boolean | undefined {
  if (value !== undefined) assertBoolean(value, path);
}

function assertLifecycleRecord(value: unknown, path: string): asserts value is CampaignLifecycleRecord {
  assertObject(value, path);
  rejectUnknown(value, LIFECYCLE_FIELDS, path);
  assertString(value.assignment_id, `${path}.assignment_id`);
  assertString(value.run_id, `${path}.run_id`);
  assertString(value.scenario_id, `${path}.scenario_id`);
  assertOptionalEnum(value.scenario_category, SCENARIO_CATEGORIES, `${path}.scenario_category`);
  assertOptionalEnum(value.lane, LANES, `${path}.lane`);
  assertOptionalEnum(value.designated_role, ROLES, `${path}.designated_role`);
  assertOptionalString(value.variant_id, `${path}.variant_id`);
  assertOptionalString(value.stack_id, `${path}.stack_id`);
  assertEnum(value.lifecycle_status, LIFECYCLE_STATUSES, `${path}.lifecycle_status`);
  if (value.repetition !== undefined) assertInteger(value.repetition, `${path}.repetition`, 1000000);
  assertTimestamp(value.observed_at, `${path}.observed_at`);
}

function assertScore(value: unknown, path: string): asserts value is WireScore {
  assertObject(value, path);
  rejectUnknown(value, ['score_id', 'dimension', 'value', 'unit', 'direction', 'missing_data_policy'], path);
  assertString(value.score_id, `${path}.score_id`);
  assertOptionalString(value.dimension, `${path}.dimension`);
  if (value.value !== undefined) {
    assert(typeof value.value === 'number' && Number.isFinite(value.value), `${path}.value`, 'expected finite number');
    assert(value.value >= 0, `${path}.value`, 'must be nonnegative');
  }
  assertOptionalString(value.unit, `${path}.unit`);
  assertOptionalString(value.direction, `${path}.direction`);
  assertOptionalString(value.missing_data_policy, `${path}.missing_data_policy`);
}

function assertScenarioSummary(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['scenario_id', 'scenario_version', 'category', 'public_description', 'grading_method', 'allowed_tools', 'expected_tools', 'forbidden_tools', 'criteria', 'tool_score_dimensions'], path);
  assertString(value.scenario_id, `${path}.scenario_id`);
  assertString(value.scenario_version, `${path}.scenario_version`);
  assertEnum(value.category, SCENARIO_CATEGORIES, `${path}.category`);
  assertString(value.public_description, `${path}.public_description`, 512);
  assertEnum(value.grading_method, GRADING_METHODS, `${path}.grading_method`);
  for (const field of ['allowed_tools', 'expected_tools', 'forbidden_tools'] as const) assertStringArray(value[field], `${path}.${field}`, 64);
  assert(Array.isArray(value.criteria), `${path}.criteria`, 'expected array');
  assert(value.criteria.length <= 64, `${path}.criteria`, 'expected at most 64 entries');
  const criterionIds = new Set<string>();
  for (let index = 0; index < value.criteria.length; index++) {
    const criterionPath = `${path}.criteria[${index}]`;
    const criterion: unknown = value.criteria[index];
    assertObject(criterion, criterionPath);
    rejectUnknown(criterion, ['criterion_id', 'public_label', 'public_description', 'grading_method', 'required'], criterionPath);
    assertString(criterion.criterion_id, `${criterionPath}.criterion_id`);
    assert(!criterionIds.has(criterion.criterion_id), `${criterionPath}.criterion_id`, 'duplicate criterion id');
    criterionIds.add(criterion.criterion_id);
    assertString(criterion.public_label, `${criterionPath}.public_label`);
    assertString(criterion.public_description, `${criterionPath}.public_description`, 512);
    assertEnum(criterion.grading_method, GRADING_METHODS, `${criterionPath}.grading_method`);
    assertBoolean(criterion.required, `${criterionPath}.required`);
  }
  assert(Array.isArray(value.tool_score_dimensions), `${path}.tool_score_dimensions`, 'expected array');
  assert(value.tool_score_dimensions.length <= 64, `${path}.tool_score_dimensions`, 'expected at most 64 entries');
  for (let index = 0; index < value.tool_score_dimensions.length; index++) {
    const dimensionPath = `${path}.tool_score_dimensions[${index}]`;
    const dimension: unknown = value.tool_score_dimensions[index];
    assertObject(dimension, dimensionPath);
    rejectUnknown(dimension, ['dimension', 'required'], dimensionPath);
    assertEnum(dimension.dimension, TOOL_DIMENSIONS, `${dimensionPath}.dimension`);
    assertBoolean(dimension.required, `${dimensionPath}.required`);
  }
}

function assertGradeSummaries(value: unknown, path: string): void {
  assert(Array.isArray(value), path, 'expected array');
  assert(value.length <= 64, path, 'expected at most 64 entries');
  for (let index = 0; index < value.length; index++) {
    const gradePath = `${path}[${index}]`;
    const grade: unknown = value[index];
    assertObject(grade, gradePath);
    rejectUnknown(grade, ['criterion_id', 'status', 'grading_method', 'judge_variant_id', 'explanation_code'], gradePath);
    assertString(grade.criterion_id, `${gradePath}.criterion_id`);
    assertEnum(grade.status, VERDICT_STATUSES, `${gradePath}.status`);
    assertEnum(grade.grading_method, GRADING_METHODS, `${gradePath}.grading_method`);
    assertOptionalString(grade.judge_variant_id, `${gradePath}.judge_variant_id`);
    assertEnum(grade.explanation_code, EXPLANATION_CODES, `${gradePath}.explanation_code`);
  }
}

function assertUint64String(value: unknown, path: string): void {
  assert(typeof value === 'string' && /^(0|[1-9][0-9]*)$/u.test(value), path, 'expected decimal uint64 string');
  assert(BigInt(value) <= BigInt(Number.MAX_SAFE_INTEGER), path, 'numeric value exceeds browser-safe range');
}

function assertActivitySummary(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['model_activity', 'tool_decisions', 'tool_calls', 'policy_decisions', 'governed_actions'], path);
  parseWireActivityFamily(value.model_activity, `${path}.model_activity`, assertModelActivityRecord);
  parseWireActivityFamily(value.tool_decisions, `${path}.tool_decisions`, assertToolDecisionRecord);
  parseWireActivityFamily(value.tool_calls, `${path}.tool_calls`, assertToolCallRecord);
  parseWireActivityFamily(value.policy_decisions, `${path}.policy_decisions`, assertPolicyDecisionRecord);
  parseWireActivityFamily(value.governed_actions, `${path}.governed_actions`, assertGovernedActionRecord);
}

function assertModelActivityRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['model_role', 'agent_persona', 'variant_id', 'usage_availability', 'input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'total_duration_nanos', 'generation_duration_nanos', 'retry_count', 'finish_state', 'load_state'], path);
  assertEnum(value.model_role, ROLES, `${path}.model_role`);
  assertOptionalString(value.agent_persona, `${path}.agent_persona`);
  assertString(value.variant_id, `${path}.variant_id`);
  assertEnum(value.usage_availability, USAGE_AVAILABILITIES, `${path}.usage_availability`);
  for (const field of ['input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'total_duration_nanos', 'generation_duration_nanos'] as const) {
    if (value[field] !== undefined) assertUint64String(value[field], `${path}.${field}`);
  }
  if (value.retry_count !== undefined) assertInteger(value.retry_count, `${path}.retry_count`, 1000);
  assertEnum(value.finish_state, FINISH_STATES, `${path}.finish_state`);
  assertEnum(value.load_state, LOAD_STATES, `${path}.load_state`);
}

function assertToolDecisionRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['tool_label', 'recognized', 'selected', 'permission_compliant', 'unnecessary', 'outcome', 'evidence_source'], path);
  assertString(value.tool_label, `${path}.tool_label`);
  assertOptionalBoolean(value.recognized, `${path}.recognized`);
  assertOptionalBoolean(value.selected, `${path}.selected`);
  assertOptionalBoolean(value.permission_compliant, `${path}.permission_compliant`);
  assertOptionalBoolean(value.unnecessary, `${path}.unnecessary`);
  assertEnum(value.outcome, SEMANTIC_OUTCOMES, `${path}.outcome`);
  assert(value.evidence_source === 'PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED', `${path}.evidence_source`, 'tool decisions require application-reported evidence');
}

function assertToolCallRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['tool_label', 'execution_outcome', 'semantic_outcome', 'evidence_source'], path);
  assertString(value.tool_label, `${path}.tool_label`);
  assertEnum(value.execution_outcome, EXECUTION_OUTCOMES, `${path}.execution_outcome`);
  assertEnum(value.semantic_outcome, SEMANTIC_OUTCOMES, `${path}.semantic_outcome`);
  assert(value.evidence_source === 'PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED', `${path}.evidence_source`, 'tool calls require application-reported evidence');
}

function assertPolicyDecisionRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['tool_label', 'outcome', 'evidence_source'], path);
  assertString(value.tool_label, `${path}.tool_label`);
  assertEnum(value.outcome, POLICY_OUTCOMES, `${path}.outcome`);
  assert(value.evidence_source === 'PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED', `${path}.evidence_source`, 'policy decisions require application-reported evidence');
}

function assertGovernedActionRecord(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['action_label', 'reported_policy_outcome', 'receipt_status', 'evidence_source'], path);
  assert(value.action_label === 'governed action', `${path}.action_label`, 'unexpected governed action label');
  assertEnum(value.reported_policy_outcome, POLICY_OUTCOMES, `${path}.reported_policy_outcome`);
  assertEnum(value.receipt_status, RECEIPT_STATUSES, `${path}.receipt_status`);
  assertEnum(value.evidence_source, EVIDENCE_SOURCES, `${path}.evidence_source`);
  if (value.receipt_status === 'PUBLIC_RECEIPT_STATUS_REPORTED') assert(value.evidence_source === 'PUBLIC_EVIDENCE_SOURCE_BOUND_PUBLIC_PROOF', `${path}.evidence_source`, 'reported receipt status requires bound public proof');
}

function assertEvidenceBindings(value: unknown, path: string): void {
  assert(Array.isArray(value), path, 'expected array');
  assert(value.length <= 32, path, 'expected at most 32 entries');
  const hashes = new Set<string>();
  for (let index = 0; index < value.length; index++) {
    const bindingPath = `${path}[${index}]`;
    const binding: unknown = value[index];
    assertObject(binding, bindingPath);
    rejectUnknown(binding, ['sha256', 'schema_ref', 'kind'], bindingPath);
    assertHash(binding.sha256, `${bindingPath}.sha256`);
    assertString(binding.schema_ref, `${bindingPath}.schema_ref`);
    assertEnum(binding.kind, EVIDENCE_KINDS, `${bindingPath}.kind`);
    assert(!hashes.has(binding.sha256), `${bindingPath}.sha256`, 'duplicate evidence identity');
    hashes.add(binding.sha256);
  }
}

function assertVerificationMetadata(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['provenance', 'verifier_state', 'verifier_release_version', 'verifier_contract_version', 'report_digest', 'population_digest'], path);
  assertEnum(value.provenance, ['PUBLIC_VERIFICATION_PROVENANCE_BOUND', 'PUBLIC_VERIFICATION_PROVENANCE_LEGACY_UNBOUND'], `${path}.provenance`);
  assertEnum(value.verifier_state, VERDICT_STATUSES, `${path}.verifier_state`);
  assertOptionalString(value.verifier_release_version, `${path}.verifier_release_version`);
  assertOptionalString(value.verifier_contract_version, `${path}.verifier_contract_version`);
  for (const field of ['report_digest', 'population_digest'] as const) {
    if (value[field] !== undefined) assertHash(value[field], `${path}.${field}`);
  }
  if (value.provenance === 'PUBLIC_VERIFICATION_PROVENANCE_BOUND') {
    assertString(value.verifier_release_version, `${path}.verifier_release_version`);
    assertString(value.verifier_contract_version, `${path}.verifier_contract_version`);
    assertHash(value.report_digest, `${path}.report_digest`);
    assertHash(value.population_digest, `${path}.population_digest`);
  }
}

function assertBenchmarkObservations(value: unknown, path: string): void {
  assertObject(value, path);
  rejectUnknown(value, ['grade_summaries', 'tool_scorecard', 'timing', 'gpu', 'unavailable_reasons'], path);
  if (value.grade_summaries !== undefined) {
    assert(Array.isArray(value.grade_summaries), `${path}.grade_summaries`, 'expected array');
    assert(value.grade_summaries.length <= 64, `${path}.grade_summaries`, 'expected at most 64 entries');
    for (let index = 0; index < value.grade_summaries.length; index++) {
      const gradePath = `${path}.grade_summaries[${index}]`;
      const grade: unknown = value.grade_summaries[index];
      assertObject(grade, gradePath);
      rejectUnknown(grade, ['criterion_id', 'status'], gradePath);
      assertString(grade.criterion_id, `${gradePath}.criterion_id`);
      assertString(grade.status, `${gradePath}.status`);
    }
  }
  if (value.tool_scorecard !== undefined) {
    assertObject(value.tool_scorecard, `${path}.tool_scorecard`);
    rejectUnknown(value.tool_scorecard, ['tool_recognition', 'tool_selection', 'argument_schema', 'argument_semantics', 'permission_compliance', 'result_interpretation', 'follow_up_decision', 'unnecessary_tool_calls', 'looping', 'recovery'], `${path}.tool_scorecard`);
    for (const [key, metric] of Object.entries(value.tool_scorecard)) assertMetric(metric, `${path}.tool_scorecard.${key}`);
  }
  for (const [family, fields] of Object.entries({
    timing: ['model_load_ms', 'time_to_first_token_ms', 'generation_ms', 'whole_task_ms'],
    gpu: ['vram_before_bytes', 'vram_peak_bytes', 'system_ram_peak_bytes', 'utilization_percent', 'temperature_celsius', 'power_watts', 'clock_mhz'],
  })) {
    if (value[family] === undefined) continue;
    assertObject(value[family], `${path}.${family}`);
    rejectUnknown(value[family], fields, `${path}.${family}`);
    for (const [key, metric] of Object.entries(value[family])) assertMetric(metric, `${path}.${family}.${key}`);
  }
  if (value.unavailable_reasons !== undefined && value.unavailable_reasons !== null) assertStringArray(value.unavailable_reasons, `${path}.unavailable_reasons`, 16, 512);
}

function assertExtensions(value: Record<string, unknown>, path: string): void {
  if (value.scenario_summary !== undefined) assertScenarioSummary(value.scenario_summary, `${path}.scenario_summary`);
  if (value.semantic_grade_summaries !== undefined) assertGradeSummaries(value.semantic_grade_summaries, `${path}.semantic_grade_summaries`);
  if (value.activity_summary !== undefined) assertActivitySummary(value.activity_summary, `${path}.activity_summary`);
  if (value.evidence_bindings !== undefined) assertEvidenceBindings(value.evidence_bindings, `${path}.evidence_bindings`);
  if (value.verification_metadata !== undefined) assertVerificationMetadata(value.verification_metadata, `${path}.verification_metadata`);
  if (value.benchmark_observations !== undefined) assertBenchmarkObservations(value.benchmark_observations, `${path}.benchmark_observations`);
  if (value.resource_summary !== undefined) {
    assertObject(value.resource_summary, `${path}.resource_summary`);
    rejectUnknown(value.resource_summary, ['latency_ms', 'input_tokens', 'output_tokens', 'thinking_tokens', 'cache_tokens', 'retries'], `${path}.resource_summary`);
    for (const [key, metric] of Object.entries(value.resource_summary)) assertMetric(metric, `${path}.resource_summary.${key}`);
  }
}

function assertResultRecord(value: unknown, path: string, version: CampaignEnvelopeVersion): asserts value is CampaignResultRecord {
  assertObject(value, path);
  rejectUnknown(value, RESULT_FIELDS, path);
  assertString(value.assignment_id, `${path}.assignment_id`);
  assertString(value.run_id, `${path}.run_id`);
  assertString(value.scenario_id, `${path}.scenario_id`);
  assertOptionalEnum(value.scenario_category, SCENARIO_CATEGORIES, `${path}.scenario_category`);
  assertOptionalEnum(value.lane, LANES, `${path}.lane`);
  assertOptionalEnum(value.designated_role, ROLES, `${path}.designated_role`);
  assertOptionalString(value.variant_id, `${path}.variant_id`);
  assertEnum(value.lifecycle_status, LIFECYCLE_STATUSES, `${path}.lifecycle_status`);
  assertEnum(value.summary_status, VERDICT_STATUSES, `${path}.summary_status`);
  if (value.decomposed_scores !== undefined) {
    assert(Array.isArray(value.decomposed_scores), `${path}.decomposed_scores`, 'expected array');
    assert(value.decomposed_scores.length <= 64, `${path}.decomposed_scores`, 'expected at most 64 entries');
    for (let index = 0; index < value.decomposed_scores.length; index++) assertScore(value.decomposed_scores[index], `${path}.decomposed_scores[${index}]`);
  }
  assertHash(value.result_digest, `${path}.result_digest`);
  assertEnum(value.verification_status, VERIFICATION_STATUSES, `${path}.verification_status`);
  if (value.unavailable_metric_reasons !== undefined) assertStringArray(value.unavailable_metric_reasons, `${path}.unavailable_metric_reasons`, 16);
  assertTimestamp(value.completed_at, `${path}.completed_at`);
  if (version === '1.0.0') {
    for (const field of ['scenario_summary', 'semantic_grade_summaries', 'activity_summary', 'evidence_bindings', 'verification_metadata', 'benchmark_observations', 'resource_summary'] as const) {
      assert(value[field] === undefined, `${path}.${field}`, 'field requires campaign envelope 1.1.0');
    }
  } else {
    assertExtensions(value, path);
  }
}

export function decodeCampaignProjectionEnvelope(value: unknown): CampaignProjectionEnvelope {
  const path = 'campaign_envelope';
  assertObject(value, path);
  rejectUnknown(value, ENVELOPE_FIELDS, path);
  assertString(value.schema_version, `${path}.schema_version`);
  assertEnum(value.message_type, CAMPAIGN_MESSAGE_TYPES, `${path}.message_type`);
  assertString(value.idempotency_key, `${path}.idempotency_key`, 512);
  assertObject(value.record, `${path}.record`);
  if (value.message_type === 'PublicAssignmentLifecycleRecord') {
    assert(value.schema_version === CAMPAIGN_LIFECYCLE_SCHEMA_VERSION, `${path}.schema_version`, 'lifecycle records require envelope 1.0.0');
    assertLifecycleRecord(value.record, `${path}.record`);
    return value as unknown as CampaignProjectionEnvelope;
  }
  assert((CAMPAIGN_RESULT_SCHEMA_VERSIONS as readonly string[]).includes(value.schema_version), `${path}.schema_version`, 'result records require envelope 1.0.0 or 1.1.0');
  assertResultRecord(value.record, `${path}.record`, value.schema_version as CampaignEnvelopeVersion);
  return value as unknown as CampaignProjectionEnvelope;
}

export function isCampaignProjectionEnvelope(value: unknown): value is CampaignProjectionEnvelope {
  try {
    decodeCampaignProjectionEnvelope(value);
    return true;
  } catch {
    return false;
  }
}
