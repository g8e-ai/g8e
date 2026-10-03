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

export interface Operator {
  id: string;
  user_id: string;
  name?: string;
  status: OperatorStatus;
  operator_type?: string;
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

export interface Investigation {
  id: string;
  case_id: string;
  case_title: string;
  case_description?: string;
  user_id: string;
  status: string;
  priority?: string;
  severity?: string;
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

// A case as the console presents it: g8ee has no case list endpoint, so cases
// are grouped from the caller's investigations.
export interface CaseSummary {
  id: string;
  title: string;
  status: string;
  priority?: string;
  updatedAt: string;
  investigations: Investigation[];
}
