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

// Live campaign observability enums (O2-live). Values match the Go wire
// shapes in internal/models/observe.go and the protocol JSON in
// protocol/models/observe_event_payloads.json.

export type CampaignFreshness =
  | 'active'
  | 'delayed'
  | 'stale'
  | 'intentionally_stopped'
  | 'safety_stopped'
  | 'source_offline';

export const CAMPAIGN_FRESHNESS_VALUES: readonly CampaignFreshness[] = [
  'active', 'delayed', 'stale', 'intentionally_stopped', 'safety_stopped', 'source_offline',
] as const;

export type CampaignCycleStatus =
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'superseded';

export const CAMPAIGN_CYCLE_STATUSES: readonly CampaignCycleStatus[] = [
  'queued', 'running', 'completed', 'failed', 'superseded',
] as const;

export type AssignmentProgressStatus =
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'superseded'
  | 'unavailable';

export const ASSIGNMENT_PROGRESS_STATUSES: readonly AssignmentProgressStatus[] = [
  'queued', 'running', 'completed', 'failed', 'superseded', 'unavailable',
] as const;

export type TerminalOutcomeStatus =
  | 'completed'
  | 'failed'
  | 'superseded'
  | 'unavailable'
  | 'qualification';

export const TERMINAL_OUTCOME_STATUSES: readonly TerminalOutcomeStatus[] = [
  'completed', 'failed', 'superseded', 'unavailable', 'qualification',
] as const;

export type SupervisorStatus =
  | 'idle'
  | 'running'
  | 'stopped'
  | 'safety_stopped';

export const SUPERVISOR_STATUSES: readonly SupervisorStatus[] = [
  'idle', 'running', 'stopped', 'safety_stopped',
] as const;

export type StopReason =
  | 'graceful'
  | 'safety_budget'
  | 'safety_disk'
  | 'safety_hardware_drift'
  | 'safety_credential'
  | 'safety_verifier'
  | 'safety_disclosure'
  | 'safety_publication';

export const STOP_REASONS: readonly StopReason[] = [
  'graceful', 'safety_budget', 'safety_disk', 'safety_hardware_drift',
  'safety_credential', 'safety_verifier', 'safety_disclosure', 'safety_publication',
] as const;

export type StopScope = 'cycle' | 'supervisor';

export const STOP_SCOPES: readonly StopScope[] = [
  'cycle', 'supervisor',
] as const;

export type ModelRole = 'primary' | 'assistant' | 'lite';

export const MODEL_ROLES: readonly ModelRole[] = [
  'primary', 'assistant', 'lite',
] as const;

export type PublicationStatus = 'pending' | 'published' | 'failed';

export const PUBLICATION_STATUSES: readonly PublicationStatus[] = [
  'pending', 'published', 'failed',
] as const;

export type MeasurementScope = 'process' | 'system' | 'accelerator';

export const MEASUREMENT_SCOPES: readonly MeasurementScope[] = [
  'process', 'system', 'accelerator',
] as const;

// Public feed enums (O3-public-feed). Values match the Go wire shapes in
// internal/models/public_feed.go and the protocol JSON in
// protocol/models/public_feed.json.

export type PublicFeedRecordType =
  | 'projection'
  | 'event'
  | 'proof_manifest'
  | 'key_revocation';

export const PUBLIC_FEED_RECORD_TYPES: readonly PublicFeedRecordType[] = [
  'projection', 'event', 'proof_manifest', 'key_revocation',
] as const;

export type PublicFeedOutboxStatus =
  | 'pending'
  | 'sent'
  | 'acknowledged'
  | 'failed';

export const PUBLIC_FEED_OUTBOX_STATUSES: readonly PublicFeedOutboxStatus[] = [
  'pending', 'sent', 'acknowledged', 'failed',
] as const;

export type PublicFeedIngestRejectionReason =
  | 'signature_invalid'
  | 'sequence_out_of_order'
  | 'hash_chain_mismatch'
  | 'duplicate_sequence'
  | 'oversized_batch'
  | 'revoked_key'
  | 'unknown_key';

export const PUBLIC_FEED_INGEST_REJECTION_REASONS: readonly PublicFeedIngestRejectionReason[] = [
  'signature_invalid', 'sequence_out_of_order', 'hash_chain_mismatch',
  'duplicate_sequence', 'oversized_batch', 'revoked_key', 'unknown_key',
] as const;

export type PublicFeedProofClassification = 'public_safe';

export const PUBLIC_FEED_PROOF_CLASSIFICATIONS: readonly PublicFeedProofClassification[] = [
  'public_safe',
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

export function isCampaignFreshness(value: unknown): value is CampaignFreshness {
  return typeof value === 'string' && (CAMPAIGN_FRESHNESS_VALUES as readonly string[]).includes(value);
}

export function isCampaignCycleStatus(value: unknown): value is CampaignCycleStatus {
  return typeof value === 'string' && (CAMPAIGN_CYCLE_STATUSES as readonly string[]).includes(value);
}

export function isAssignmentProgressStatus(value: unknown): value is AssignmentProgressStatus {
  return typeof value === 'string' && (ASSIGNMENT_PROGRESS_STATUSES as readonly string[]).includes(value);
}

export function isTerminalOutcomeStatus(value: unknown): value is TerminalOutcomeStatus {
  return typeof value === 'string' && (TERMINAL_OUTCOME_STATUSES as readonly string[]).includes(value);
}

export function isSupervisorStatus(value: unknown): value is SupervisorStatus {
  return typeof value === 'string' && (SUPERVISOR_STATUSES as readonly string[]).includes(value);
}

export function isStopReason(value: unknown): value is StopReason {
  return typeof value === 'string' && (STOP_REASONS as readonly string[]).includes(value);
}

export function isStopScope(value: unknown): value is StopScope {
  return typeof value === 'string' && (STOP_SCOPES as readonly string[]).includes(value);
}

export function isModelRole(value: unknown): value is ModelRole {
  return typeof value === 'string' && (MODEL_ROLES as readonly string[]).includes(value);
}

export function isPublicationStatus(value: unknown): value is PublicationStatus {
  return typeof value === 'string' && (PUBLICATION_STATUSES as readonly string[]).includes(value);
}

export function isMeasurementScope(value: unknown): value is MeasurementScope {
  return typeof value === 'string' && (MEASUREMENT_SCOPES as readonly string[]).includes(value);
}

export function isPublicFeedRecordType(value: unknown): value is PublicFeedRecordType {
  return typeof value === 'string' && (PUBLIC_FEED_RECORD_TYPES as readonly string[]).includes(value);
}

export function isPublicFeedOutboxStatus(value: unknown): value is PublicFeedOutboxStatus {
  return typeof value === 'string' && (PUBLIC_FEED_OUTBOX_STATUSES as readonly string[]).includes(value);
}

export function isPublicFeedIngestRejectionReason(value: unknown): value is PublicFeedIngestRejectionReason {
  return typeof value === 'string' && (PUBLIC_FEED_INGEST_REJECTION_REASONS as readonly string[]).includes(value);
}

export function isPublicFeedProofClassification(value: unknown): value is PublicFeedProofClassification {
  return typeof value === 'string' && (PUBLIC_FEED_PROOF_CLASSIFICATIONS as readonly string[]).includes(value);
}
