// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// The events the console reacts to, named from the generated registry
// (src/generated/events.ts, produced by `make constants` from
// protocol/constants/events.json). Unknown types are ignored, never guessed at.

import { EventType as E } from '../generated/events';

export const Ev = {
  IterationStarted: E.AI_LLM_CHAT_ITERATION_STARTED,
  IterationCompleted: E.AI_LLM_CHAT_ITERATION_COMPLETED,
  IterationFailed: E.AI_LLM_CHAT_ITERATION_FAILED,
  IterationStopped: E.AI_LLM_CHAT_ITERATION_STOPPED,
  TextChunk: E.AI_LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
  TextCompleted: E.AI_LLM_CHAT_ITERATION_TEXT_COMPLETED,
  ThinkingStarted: E.AI_LLM_CHAT_ITERATION_THINKING_STARTED,
  ThinkingEnd: E.AI_LLM_CHAT_ITERATION_THINKING_END,

  ConsensusStarted: E.AI_CONSENSUS_SESSION_STARTED,
  ConsensusCompleted: E.AI_CONSENSUS_SESSION_COMPLETED,

  CommandRequested: E.OPERATOR_COMMAND_REQUESTED,
  CommandStarted: E.OPERATOR_COMMAND_STARTED,
  CommandCompleted: E.OPERATOR_COMMAND_COMPLETED,
  CommandFailed: E.OPERATOR_COMMAND_FAILED,

  CommandApprovalRequested: E.OPERATOR_COMMAND_APPROVAL_REQUESTED,
  FileEditApprovalRequested: E.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED,
  IntentApprovalRequested: E.OPERATOR_INTENT_APPROVAL_REQUESTED,
  AgentContinueApprovalRequested: E.AI_AGENT_CONTINUE_APPROVAL_REQUESTED,

  ApprovalsChanged: E.PLATFORM_APPROVALS_CHANGED,

  CaseCreated: E.APP_CASE_CREATED,
  CaseUpdated: E.APP_CASE_UPDATED,

  /** Prefix of g8e.v1.operator.status.updated.<state>. */
  OperatorStatusPrefix: E.OPERATOR_STATUS_UPDATED_ACTIVE.slice(0, -'active'.length),
} as const;

export const APPROVAL_REQUEST_TYPES: ReadonlySet<string> = new Set([
  Ev.CommandApprovalRequested,
  Ev.FileEditApprovalRequested,
  Ev.IntentApprovalRequested,
  Ev.AgentContinueApprovalRequested,
]);

// Conversation-history senders (ensemble/app/constants/message_sender.py).
export const Sender = {
  UserChat: 'g8e.v1.source.user.chat',
  UserTerminal: 'g8e.v1.source.user.terminal',
  AiPrimary: 'g8e.v1.source.ai.primary',
  AiAssistant: 'g8e.v1.source.ai.assistant',
  AiTriage: 'g8e.v1.source.ai.triage',
  System: 'g8e.v1.source.system',
} as const;
