// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Wire shapes consumed by the console. Sources of truth:
//   Gateway: internal/models/auth.go, internal/models/gateway.go
//   g8ee:    ensemble/app/models/investigations.py, protocol/python/g8e/models

export interface User {
  id: string;
  status?: string;
  roles?: string[];
}

export interface PasskeyCredential {
  /** Standard base64 (Go []byte JSON encoding). */
  id: string;
  attestation_type?: string;
  created_at_unix_ms?: number;
  last_used_at_unix_ms?: number;
}

export type OperatorStatus =
  | 'active'
  | 'available'
  | 'bound'
  | 'offline'
  | 'stale'
  | 'stopped'
  | 'terminated'
  | 'unavailable';

export type OperatorRole = 'inference' | 'provenance' | 'observer' | 'data';

export interface Operator {
  id: string;
  user_id: string;
  name?: string;
  status: OperatorStatus;
  operator_type?: string;
  operator_role?: OperatorRole;
  operator_session_id?: string;
  bound_web_session_id?: string;
  current_hostname?: string;
  system_fingerprint?: string;
  last_heartbeat_at?: string;
  started_at?: string;
  created_at: string;
  updated_at: string;
}

export interface BindResponse {
  success: boolean;
  bound_count?: number;
  failed_count?: number;
  unbound_count?: number;
  failed_operator_ids?: string[];
  error?: string;
}

export interface SuspendedTransaction {
  transaction_hash: string;
  tool_name?: string;
  created_at?: string;
  expires_at?: string;
}

export interface PlatformEnrollmentRequest {
  request_id: string;
  component_kind?: string;
  component_name?: string;
  instance_id?: string;
  hostname?: string;
  system_fingerprint?: string;
  state: string;
  created_at?: string;
  expires_at?: string;
  fingerprints?: { app?: string; operator?: string; cli?: string };
}

export interface ConversationMessage {
  id?: string;
  sender: string;
  content: string;
  timestamp: string;
  metadata?: {
    event_type?: string | null;
    execution_id?: string | null;
    command?: string | null;
    status?: string | null;
    approval_id?: string | null;
    hostname?: string | null;
    justification?: string | null;
    approved?: boolean | null;
  };
}

export const InvestigationStatus = {
  Open: 'Open',
  Closed: 'Closed',
  Escalated: 'Escalated',
  Resolved: 'Resolved',
} as const;
export type InvestigationStatusValue = (typeof InvestigationStatus)[keyof typeof InvestigationStatus];

export const CaseStatus = {
  New: 'New',
  Triage: 'Triage',
  InProgress: 'InProgress',
  HumanReview: 'HumanReview',
  WaitingForCustomer: 'WaitingForCustomer',
  Escalated: 'Escalated',
  Resolved: 'Resolved',
  Closed: 'Closed',
} as const;
export type CaseStatusValue = (typeof CaseStatus)[keyof typeof CaseStatus];

export const Priority = {
  Critical: 'CRITICAL',
  High: 'HIGH',
  Medium: 'MEDIUM',
  Low: 'LOW',
} as const;
export type PriorityValue = (typeof Priority)[keyof typeof Priority];

export const Severity = {
  Critical: 'CRITICAL',
  High: 'HIGH',
  Medium: 'MEDIUM',
  Low: 'LOW',
} as const;
export type SeverityValue = (typeof Severity)[keyof typeof Severity];

export interface Investigation {
  id: string;
  case_id: string;
  case_title: string;
  case_description?: string;
  user_id: string;
  status: InvestigationStatusValue | string;
  priority?: PriorityValue | string;
  severity?: SeverityValue | string;
  sentinel_mode?: boolean;
  created_with_case?: boolean;
  conversation_history?: ConversationMessage[];
  created_at: string;
  updated_at?: string | null;
}

export interface ChatStartedResponse {
  success: boolean;
  case_id: string;
  investigation_id: string;
}

export interface ChatStopResponse {
  success: boolean;
  investigation_id: string;
  /** False when no turn was running, so no stopped event will follow. */
  was_active: boolean;
}

// A case as the console presents it: g8ee has no case list endpoint, so cases
// are grouped from the caller's investigations.
export interface CaseSummary {
  id: string;
  title: string;
  status: InvestigationStatusValue | CaseStatusValue | string;
  priority?: PriorityValue | string;
  updatedAt: string;
  investigations: Investigation[];
}

export type LlmRole = 'primary' | 'assistant' | 'lite';
export type FieldRequirement = 'required' | 'optional' | 'none';

/** A provider the ensemble accepts for a role, with the fields it needs. */
export interface LlmProviderOption {
  provider: string;
  label: string;
  endpoint: FieldRequirement;
  api_key: FieldRequirement;
  default_endpoint?: string | null;
  configured_endpoint?: string | null;
  api_key_set?: boolean;
  lists_models: boolean;
}

/** One role's stored provider and model selection. */
export interface LlmRoleView {
  provider: string | null;
  model: string | null;
}

/** /settings/llm/models: the models a saved provider connection serves. */
export interface LlmModelList {
  models: string[];
}

export interface LlmSettings {
  providers: LlmProviderOption[];
  primary: LlmRoleView;
  assistant: LlmRoleView;
  lite: LlmRoleView;
}
