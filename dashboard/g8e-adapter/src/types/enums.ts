// Typed observe/enum models mirroring the Go contracts in
// internal/models/observe.go and the protocol JSON in
// protocol/models/observe_event_payloads.json. Field names and enum values
// match the Go wire shapes byte-for-byte.

export type AgentLifecycleStatus =
  | 'idle'
  | 'queued'
  | 'running'
  | 'waiting'
  | 'completed'
  | 'failed'
  | 'offline';

export const AGENT_LIFECYCLE_STATUSES: readonly AgentLifecycleStatus[] = [
  'idle', 'queued', 'running', 'waiting', 'completed', 'failed', 'offline',
] as const;

export type RunLifecycleStatus =
  | 'queued'
  | 'running'
  | 'waiting'
  | 'completed'
  | 'failed'
  | 'cancelled';

export const RUN_LIFECYCLE_STATUSES: readonly RunLifecycleStatus[] = [
  'queued', 'running', 'waiting', 'completed', 'failed', 'cancelled',
] as const;

export type RunKind = 'investigation' | 'eval' | 'demo' | 'workflow';

export const RUN_KINDS: readonly RunKind[] = [
  'investigation', 'eval', 'demo', 'workflow',
] as const;

export type EvalVerificationStatus =
  | 'projection_validated'
  | 'receipt_verification_not_applicable'
  | 'verified';

export const EVAL_VERIFICATION_STATUSES: readonly EvalVerificationStatus[] = [
  'projection_validated', 'receipt_verification_not_applicable', 'verified',
] as const;

export type MeasurementStatus =
  | 'observed'
  | 'stale'
  | 'unavailable'
  | 'unsupported';

export const MEASUREMENT_STATUSES: readonly MeasurementStatus[] = [
  'observed', 'stale', 'unavailable', 'unsupported',
] as const;

export type SnapshotFreshness = 'observed' | 'stale' | 'unavailable';

export const SNAPSHOT_FRESHNESS_VALUES: readonly SnapshotFreshness[] = [
  'observed', 'stale', 'unavailable',
] as const;

export type DownloadPrivacyClassification = 'public_safe' | 'restricted';

export const DOWNLOAD_PRIVACY_CLASSIFICATIONS: readonly DownloadPrivacyClassification[] = [
  'public_safe', 'restricted',
] as const;

export function isAgentLifecycleStatus(value: unknown): value is AgentLifecycleStatus {
  return typeof value === 'string' && (AGENT_LIFECYCLE_STATUSES as readonly string[]).includes(value);
}

export function isRunLifecycleStatus(value: unknown): value is RunLifecycleStatus {
  return typeof value === 'string' && (RUN_LIFECYCLE_STATUSES as readonly string[]).includes(value);
}

export function isRunKind(value: unknown): value is RunKind {
  return typeof value === 'string' && (RUN_KINDS as readonly string[]).includes(value);
}

export function isEvalVerificationStatus(value: unknown): value is EvalVerificationStatus {
  return typeof value === 'string' && (EVAL_VERIFICATION_STATUSES as readonly string[]).includes(value);
}

export function isMeasurementStatus(value: unknown): value is MeasurementStatus {
  return typeof value === 'string' && (MEASUREMENT_STATUSES as readonly string[]).includes(value);
}

export function isSnapshotFreshness(value: unknown): value is SnapshotFreshness {
  return typeof value === 'string' && (SNAPSHOT_FRESHNESS_VALUES as readonly string[]).includes(value);
}

export function isDownloadPrivacyClassification(value: unknown): value is DownloadPrivacyClassification {
  return typeof value === 'string' && (DOWNLOAD_PRIVACY_CLASSIFICATIONS as readonly string[]).includes(value);
}
