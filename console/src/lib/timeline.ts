// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// An investigation's timeline: the persisted conversation history plus the live
// SSE events for that investigation, folded into display items. Pure functions
// so the reducer is testable without a browser.

import { APPROVAL_REQUEST_TYPES, Ev, Sender } from './events';
import type { StreamEvent } from './sse';
import type { ConversationMessage } from './types';

export type ToolStatus = 'requested' | 'running' | 'completed' | 'failed';
export type ApprovalState = 'pending' | 'approved' | 'denied' | 'submitting';
export type ApprovalKind = 'command' | 'file_edit' | 'intent' | 'agent_continue';

export type TimelineItem =
  | { kind: 'user'; key: string; text: string; at: string }
  | { kind: 'assistant'; key: string; text: string; streaming: boolean; at: string }
  | { kind: 'tool'; key: string; tool: string; detail: string; status: ToolStatus; output?: string; error?: string; at: string }
  | {
      kind: 'approval';
      key: string;
      approvalId: string;
      approvalKind: ApprovalKind;
      subject: string;
      justification: string;
      risk?: string;
      state: ApprovalState;
      at: string;
    }
  | { kind: 'notice'; key: string; level: 'info' | 'error'; text: string; at: string };

export interface TimelineState {
  items: TimelineItem[];
  /** True while the ensemble is working on a turn for this investigation. */
  busy: boolean;
  /** Short phase label (e.g. "Thinking", "Tribunal deliberating"); never raw chain-of-thought. */
  phase: string | null;
}

export const emptyTimeline: TimelineState = { items: [], busy: false, phase: null };

const AI_SENDERS: ReadonlySet<string> = new Set([Sender.AiPrimary, Sender.AiAssistant, Sender.AiTriage]);

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

function approvalKind(type: string): ApprovalKind {
  switch (type) {
    case Ev.FileEditApprovalRequested:
      return 'file_edit';
    case Ev.IntentApprovalRequested:
      return 'intent';
    case Ev.AgentContinueApprovalRequested:
      return 'agent_continue';
    default:
      return 'command';
  }
}

function approvalSubject(kind: ApprovalKind, d: Record<string, unknown>): string {
  switch (kind) {
    case 'file_edit':
      return `${str(d.operation) || 'edit'} ${str(d.file_path)}`.trim();
    case 'intent':
      return str(d.intent_question) || str(d.intent_name);
    case 'agent_continue':
      return `Continue past the ${String(d.turn_limit ?? '')}-turn tool budget (${String(d.turns_completed ?? '')} completed)`;
    default:
      return str(d.command);
  }
}

function riskLabel(d: Record<string, unknown>): string | undefined {
  const r = d.risk_analysis;
  if (!r || typeof r !== 'object') return undefined;
  const level = str((r as Record<string, unknown>).risk_level);
  return level || undefined;
}

/** Converts persisted conversation history into timeline items. */
export function fromHistory(history: ConversationMessage[] | undefined): TimelineItem[] {
  const items: TimelineItem[] = [];
  (history ?? []).forEach((m, i) => {
    const key = m.id || `h${i}`;
    const meta = m.metadata ?? {};
    const eventType = meta.event_type ?? '';
    if (m.sender === Sender.UserChat) {
      items.push({ kind: 'user', key, text: m.content, at: m.timestamp });
    } else if (AI_SENDERS.has(m.sender)) {
      if (m.content) items.push({ kind: 'assistant', key, text: m.content, streaming: false, at: m.timestamp });
    } else if (eventType && APPROVAL_REQUEST_TYPES.has(eventType) && meta.approval_id) {
      const kind = approvalKind(eventType);
      items.push({
        kind: 'approval',
        key: `a:${meta.approval_id}`,
        approvalId: meta.approval_id,
        approvalKind: kind,
        subject: meta.command || m.content,
        justification: meta.justification ?? '',
        state: meta.approved === true ? 'approved' : meta.approved === false ? 'denied' : 'pending',
        at: m.timestamp,
      });
    } else if (meta.execution_id && meta.command) {
      const failed = meta.status === 'failed';
      items.push({
        kind: 'tool',
        key: `t:${meta.execution_id}`,
        tool: meta.hostname ? `command on ${meta.hostname}` : 'command',
        detail: meta.command,
        status: failed ? 'failed' : 'completed',
        output: failed ? undefined : m.content,
        error: failed ? m.content : undefined,
        at: m.timestamp,
      });
    } else if (m.content && m.sender === Sender.System) {
      items.push({ kind: 'notice', key, level: 'info', text: m.content, at: m.timestamp });
    }
  });
  return mergeLifecycleRecords(items);
}

// Approval and command lifecycles are recorded as several history rows sharing
// an ID (approval requested, then granted/rejected; command started, then
// finished). Fold each into one item: keep the first row's position and
// subject, take the latest state.
function mergeLifecycleRecords(items: TimelineItem[]): TimelineItem[] {
  const out: TimelineItem[] = [];
  const index = new Map<string, number>();
  for (const item of items) {
    if (item.kind !== 'approval' && item.kind !== 'tool') {
      out.push(item);
      continue;
    }
    const at = index.get(item.key);
    if (at === undefined) {
      index.set(item.key, out.length);
      out.push(item);
    } else if (item.kind === 'approval') {
      const prev = out[at] as Extract<TimelineItem, { kind: 'approval' }>;
      out[at] = { ...prev, state: item.state === 'pending' ? prev.state : item.state };
    } else {
      const prev = out[at] as Extract<TimelineItem, { kind: 'tool' }>;
      out[at] = { ...item, at: prev.at, status: forwardStatus(prev.status, item.status) };
    }
  }
  return out;
}

const TOOL_STATUS_RANK: Record<ToolStatus, number> = { requested: 0, running: 1, completed: 2, failed: 2 };

/** Lifecycle only moves forward: a replayed earlier stage never regresses a terminal item. */
function forwardStatus(prev: ToolStatus | undefined, next: ToolStatus): ToolStatus {
  return prev && TOOL_STATUS_RANK[prev] >= TOOL_STATUS_RANK[next] ? prev : next;
}

function upsert(items: TimelineItem[], key: string, make: (prev: TimelineItem | undefined) => TimelineItem): TimelineItem[] {
  const i = items.findIndex((it) => it.key === key);
  if (i === -1) return [...items, make(undefined)];
  const next = items.slice();
  next[i] = make(items[i]);
  return next;
}

/** The streaming assistant item for the current turn, if one is open. */
function openAssistantKey(items: TimelineItem[]): string | null {
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i]!;
    if (it.kind === 'assistant' && it.streaming) return it.key;
    if (it.kind === 'user') return null;
  }
  return null;
}

function closeStreaming(items: TimelineItem[]): TimelineItem[] {
  return items.map((it) => (it.kind === 'assistant' && it.streaming ? { ...it, streaming: false } : it));
}

/** Folds one live SSE event (already filtered to this investigation) into the timeline. */
export function applyEvent(state: TimelineState, ev: StreamEvent): TimelineState {
  const d = ev.data;
  const at = ev.timestamp;
  switch (ev.type) {
    case Ev.IterationStarted:
      return { ...state, busy: true, phase: 'Working' };
    case Ev.ThinkingStarted:
      return { ...state, busy: true, phase: d.phase === 'end' ? 'Working' : 'Thinking' };
    case Ev.ThinkingEnd:
      return { ...state, phase: 'Working' };
    case Ev.ConsensusStarted:
      return { ...state, busy: true, phase: 'Tribunal deliberating' };
    case Ev.ConsensusCompleted:
      return { ...state, phase: 'Working' };
    case Ev.TextChunk: {
      const chunk = str(d.content);
      if (!chunk) return state;
      const key = openAssistantKey(state.items) ?? `s:${ev.id || at}`;
      const items = upsert(state.items, key, (prev) =>
        prev && prev.kind === 'assistant'
          ? { ...prev, text: prev.text + chunk }
          : { kind: 'assistant', key, text: chunk, streaming: true, at },
      );
      return { ...state, items, busy: true, phase: null };
    }
    case Ev.TextCompleted: {
      const full = str(d.content);
      const key = openAssistantKey(state.items);
      let items = state.items;
      if (key) {
        items = upsert(items, key, (prev) =>
          prev && prev.kind === 'assistant' ? { ...prev, text: full || prev.text, streaming: false } : prev!,
        );
      } else if (full) {
        items = [...items, { kind: 'assistant', key: `s:${ev.id || at}`, text: full, streaming: false, at }];
      }
      // TEXT_COMPLETED is the end-of-turn event: the ensemble emits it once, after
      // the model (and any tool loop) finishes. IterationCompleted fires only per
      // tool round, so a text-only reply would otherwise leave the turn busy.
      return { items: closeStreaming(items), busy: false, phase: null };
    }
    case Ev.IterationCompleted:
      return { ...state, items: closeStreaming(state.items), phase: 'Working' };
    case Ev.IterationStopped:
      return {
        items: [...closeStreaming(state.items), { kind: 'notice', key: `n:${ev.id || at}`, level: 'info', text: 'Stopped.', at }],
        busy: false,
        phase: null,
      };
    case Ev.IterationFailed:
      return {
        items: [
          ...closeStreaming(state.items),
          { kind: 'notice', key: `n:${ev.id || at}`, level: 'error', text: str(d.error) || 'The ensemble failed to complete this turn.', at },
        ],
        busy: false,
        phase: null,
      };
    case Ev.CommandRequested:
    case Ev.CommandStarted:
    case Ev.CommandCompleted:
    case Ev.CommandFailed:
    case Ev.ToolConstraintsRequested:
    case Ev.ToolConstraintsCompleted:
    case Ev.ToolConstraintsFailed:
    case Ev.ToolInvestigationRequested:
    case Ev.ToolInvestigationCompleted:
    case Ev.ToolInvestigationFailed:
    case Ev.ToolWebSearchRequested:
    case Ev.ToolWebSearchCompleted:
    case Ev.ToolWebSearchFailed: {
      const execId = str(d.execution_id) || String(ev.id);
      const isCompleted =
        ev.type === Ev.CommandCompleted ||
        ev.type === Ev.ToolConstraintsCompleted ||
        ev.type === Ev.ToolInvestigationCompleted ||
        ev.type === Ev.ToolWebSearchCompleted;
      const isFailed =
        ev.type === Ev.CommandFailed ||
        ev.type === Ev.ToolConstraintsFailed ||
        ev.type === Ev.ToolInvestigationFailed ||
        ev.type === Ev.ToolWebSearchFailed;
      const isStarted = ev.type === Ev.CommandStarted;
      const status: ToolStatus =
        isCompleted ? 'completed' : isFailed ? 'failed' : isStarted ? 'running' : 'requested';
      const items = upsert(state.items, `t:${execId}`, (prev) => {
        const base = prev && prev.kind === 'tool' ? prev : undefined;
        return {
          kind: 'tool',
          key: `t:${execId}`,
          tool: str(d.tool_name) || base?.tool || 'command',
          detail: str(d.display_detail) || base?.detail || '',
          status: forwardStatus(base?.status, status),
          output: str(d.content) || base?.output,
          error: str(d.error) || base?.error,
          at: base?.at ?? at,
        };
      });
      const phase =
        status === 'running' || status === 'requested'
          ? str(d.display_label) || (d.tool_name ? `Running ${str(d.tool_name)}` : 'Working')
          : 'Working';
      return { ...state, items, busy: true, phase };
    }
    default:
      if (APPROVAL_REQUEST_TYPES.has(ev.type)) {
        const approvalId = str(d.approval_id);
        if (!approvalId) return state;
        const kind = approvalKind(ev.type);
        const items = upsert(state.items, `a:${approvalId}`, (prev) =>
          prev && prev.kind === 'approval'
            ? prev
            : {
                kind: 'approval',
                key: `a:${approvalId}`,
                approvalId,
                approvalKind: kind,
                subject: approvalSubject(kind, d),
                justification: str(d.justification),
                risk: riskLabel(d),
                state: 'pending',
                at,
              },
        );
        return { ...state, items, phase: 'Awaiting your approval' };
      }
      return state;
  }
}

export function setApprovalState(state: TimelineState, approvalId: string, next: ApprovalState): TimelineState {
  return {
    ...state,
    items: state.items.map((it) => (it.kind === 'approval' && it.approvalId === approvalId ? { ...it, state: next } : it)),
    phase: next === 'approved' || next === 'denied' ? 'Working' : state.phase,
  };
}

/** Ends the turn locally, without a notice, when the server reports nothing left to stop. */
export function markIdle(state: TimelineState): TimelineState {
  return { items: closeStreaming(state.items), busy: false, phase: null };
}

/** Optimistically appends the user's message when a turn is submitted. */
export function appendUserMessage(state: TimelineState, text: string): TimelineState {
  const at = new Date().toISOString();
  return { items: [...state.items, { kind: 'user', key: `u:${at}`, text, at }], busy: true, phase: 'Working' };
}

/** True when the event belongs to the given investigation. */
export function eventTargets(ev: StreamEvent, investigationId: string | null): boolean {
  return !!investigationId && ev.data.investigation_id === investigationId;
}
