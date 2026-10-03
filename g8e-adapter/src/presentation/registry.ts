// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Safe event presentation registry. Recognized renderers use escaped bounded
// fields and safe labels. Thinking events become phase labels, never raw
// chain-of-thought. Unknown valid events appear only in a bounded diagnostic
// row and cannot mutate projections or counters.
//
// This module is pure: it maps a NormalizedGatewayEvent or a projection to a
// presentation model. It does not touch the DOM, network, or state stores.

import type { NormalizedGatewayEvent } from '../sse/normalizer';
import type {
  AgentStateProjection,
  EvalSummary,
  RunSummary,
} from '../types/observe';
import type { NarrativeRow } from '../state/stores';

// --- View states ---

export type ViewStatus =
  | 'loading'
  | 'empty'
  | 'stale'
  | 'unavailable'
  | 'unsupported'
  | 'partial_verification'
  | 'disconnected'
  | 'unauthenticated'
  | 'error'
  | 'ready';

export interface ViewState {
  readonly status: ViewStatus;
  readonly message: string;
}

export const VIEW_STATES = {
  loading: { status: 'loading' as const, message: 'Loading…' },
  empty: { status: 'empty' as const, message: 'No data available' },
  stale: { status: 'stale' as const, message: 'Data may be stale' },
  unavailable: { status: 'unavailable' as const, message: 'Unavailable' },
  unsupported: { status: 'unsupported' as const, message: 'Not supported' },
  partialVerification: { status: 'partial_verification' as const, message: 'Partially verified' },
  disconnected: { status: 'disconnected' as const, message: 'Disconnected from live stream' },
  unauthenticated: { status: 'unauthenticated' as const, message: 'Authentication required' },
  error: { status: 'error' as const, message: 'An error occurred' },
  ready: { status: 'ready' as const, message: '' },
} as const;

// --- Escaping ---

// Escape a string for safe HTML display. Bounded fields are escaped to
// prevent injection from event payloads. The adapter does not render raw
// HTML from any event source.
export function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '\x26amp;')
    .replace(/</g, '\x26lt;')
    .replace(/>/g, '\x26gt;')
    .replace(/"/g, '\x26quot;')
    .replace(/'/g, '\x26#39;');
}

// Bound a string to a maximum length, appending an ellipsis if truncated.
export function boundString(value: string, maxLen: number): string {
  if (value.length <= maxLen) return value;
  return value.slice(0, maxLen - 1) + '\u2026';
}

const MAX_DISPLAY_NAME = 100;
const MAX_LABEL = 80;

// --- Event presentation ---

export type EventPresentationKind = 'agent_status' | 'run_status' | 'eval_completed' | 'eval_metric' | 'sentinel' | 'diagnostic';

export interface EventPresentation {
  readonly kind: EventPresentationKind;
  readonly label: string;
  readonly safeDetail: string;
  readonly recognized: boolean;
  readonly sentinel: boolean;
}

// Map a normalized event to a safe presentation model. Unknown events become
// diagnostic rows with only the type and a bounded raw indicator — never the
// raw payload. Thinking/chain-of-thought events are never rendered as raw
// text; if a thinking event type is introduced, it maps to a phase label.
export function presentEvent(event: NormalizedGatewayEvent): EventPresentation {
  if (event.sentinel) {
    return {
      kind: 'sentinel',
      label: sentinelLabel(event),
      safeDetail: sentinelDetail(event),
      recognized: true,
      sentinel: true,
    };
  }

  if (!event.recognized) {
    return {
      kind: 'diagnostic',
      label: 'Unknown event',
      safeDetail: boundString(`type: ${event.type}`, MAX_LABEL),
      recognized: false,
      sentinel: false,
    };
  }

  const payload = event.payload as Record<string, unknown> | null;

  switch (event.type) {
    case 'app.agent.status.updated': {
      const displayName = typeof payload?.display_name === 'string' ? payload.display_name : '';
      const status = typeof payload?.status === 'string' ? payload.status : '';
      return {
        kind: 'agent_status',
        label: 'Agent status',
        safeDetail: `${escapeHtml(boundString(displayName, MAX_DISPLAY_NAME))} → ${escapeHtml(status)}`,
        recognized: true,
        sentinel: false,
      };
    }

    case 'app.run.status.updated': {
      const displayName = typeof payload?.display_name === 'string' ? payload.display_name : '';
      const status = typeof payload?.status === 'string' ? payload.status : '';
      const runKind = typeof payload?.run_kind === 'string' ? payload.run_kind : '';
      return {
        kind: 'run_status',
        label: 'Run status',
        safeDetail: `${escapeHtml(boundString(displayName, MAX_DISPLAY_NAME))} (${escapeHtml(runKind)}) → ${escapeHtml(status)}`,
        recognized: true,
        sentinel: false,
      };
    }

    case 'ai.eval.run.completed': {
      const runId = typeof payload?.run_id === 'string' ? payload.run_id : '';
      const verification = typeof payload?.verification_status === 'string' ? payload.verification_status : '';
      return {
        kind: 'eval_completed',
        label: 'Eval completed',
        safeDetail: `${escapeHtml(boundString(runId, MAX_LABEL))} — ${escapeHtml(verification)}`,
        recognized: true,
        sentinel: false,
      };
    }

    case 'ai.eval.metric.recorded': {
      const metricId = typeof payload?.metric_id === 'string' ? payload.metric_id : '';
      const unit = typeof payload?.unit === 'string' ? payload.unit : '';
      return {
        kind: 'eval_metric',
        label: 'Eval metric',
        safeDetail: `${escapeHtml(boundString(metricId, MAX_LABEL))} (${escapeHtml(unit)})`,
        recognized: true,
        sentinel: false,
      };
    }

    default:
      return {
        kind: 'diagnostic',
        label: 'Unknown event',
        safeDetail: boundString(`type: ${event.type}`, MAX_LABEL),
        recognized: false,
        sentinel: false,
      };
  }
}

function sentinelLabel(event: NormalizedGatewayEvent): string {
  if (event.type === 'truncated') return 'Stream truncated';
  if (event.type === 'error') return 'Stream error';
  return 'Sentinel';
}

function sentinelDetail(event: NormalizedGatewayEvent): string {
  const payload = event.payload as Record<string, unknown> | null;
  if (event.type === 'truncated') {
    const limit = typeof payload?.limit === 'number' ? payload.limit : '?';
    return `Replay limited to ${limit} events`;
  }
  if (event.type === 'error') {
    const reason = typeof payload?.reason === 'string' ? payload.reason : 'unknown';
    return escapeHtml(boundString(reason, MAX_LABEL));
  }
  return '';
}

// --- Agent presentation ---

export interface AgentPresentation {
  readonly agentId: string;
  readonly displayName: string;
  readonly role: string;
  readonly status: string;
  readonly statusLabel: string;
  readonly freshness: string;
  readonly model: string | null;
  readonly viewState: ViewState;
}

const AGENT_STATUS_LABELS: Record<string, string> = {
  idle: 'Idle',
  queued: 'Queued',
  running: 'Running',
  waiting: 'Waiting',
  completed: 'Completed',
  failed: 'Failed',
  offline: 'Offline',
};

export function presentAgent(agent: AgentStateProjection): AgentPresentation {
  return {
    agentId: escapeHtml(agent.agent_id),
    displayName: escapeHtml(boundString(agent.display_name, MAX_DISPLAY_NAME)),
    role: escapeHtml(agent.role),
    status: agent.status,
    statusLabel: AGENT_STATUS_LABELS[agent.status] ?? agent.status,
    freshness: agent.freshness,
    model: agent.model ?? null,
    viewState: agent.freshness === 'unavailable' ? VIEW_STATES.unavailable : VIEW_STATES.ready,
  };
}

// --- Run presentation ---

export interface RunPresentation {
  readonly runId: string;
  readonly displayName: string;
  readonly runKind: string;
  readonly status: string;
  readonly statusLabel: string;
  readonly completedTasks: number;
  readonly totalTasks: number;
  readonly hasReceipts: boolean;
  readonly evidenceCount: number;
  readonly viewState: ViewState;
}

const RUN_STATUS_LABELS: Record<string, string> = {
  queued: 'Queued',
  running: 'Running',
  waiting: 'Waiting',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
};

export function presentRun(run: RunSummary): RunPresentation {
  return {
    runId: escapeHtml(run.run_id),
    displayName: escapeHtml(boundString(run.display_name, MAX_DISPLAY_NAME)),
    runKind: run.run_kind,
    status: run.status,
    statusLabel: RUN_STATUS_LABELS[run.status] ?? run.status,
    completedTasks: run.completed_tasks,
    totalTasks: run.total_tasks,
    hasReceipts: run.has_receipts,
    evidenceCount: run.evidence_count,
    viewState: VIEW_STATES.ready,
  };
}

// --- Eval presentation ---

export interface EvalPresentation {
  readonly runId: string;
  readonly suiteId: string;
  readonly armId: string;
  readonly status: string;
  readonly statusLabel: string;
  readonly verificationStatus: string;
  readonly verificationLabel: string;
  readonly receiptCount: number;
  readonly metricCount: number;
  readonly viewState: ViewState;
}

const EVAL_VERIFICATION_LABELS: Record<string, string> = {
  projection_validated: 'Projection validated',
  receipt_verification_not_applicable: 'Receipt verification not applicable',
  verified: 'Verified',
};

export function presentEval(eval_: EvalSummary): EvalPresentation {
  const viewState = eval_.verification_status === 'projection_validated'
    ? VIEW_STATES.partialVerification
    : VIEW_STATES.ready;
  return {
    runId: escapeHtml(eval_.run_id),
    suiteId: escapeHtml(eval_.suite_id),
    armId: escapeHtml(eval_.arm_id),
    status: eval_.status,
    statusLabel: RUN_STATUS_LABELS[eval_.status] ?? eval_.status,
    verificationStatus: eval_.verification_status,
    verificationLabel: EVAL_VERIFICATION_LABELS[eval_.verification_status] ?? eval_.verification_status,
    receiptCount: eval_.receipt_count,
    metricCount: eval_.metric_count,
    viewState,
  };
}

// --- Narrative row presentation ---

export interface NarrativeRowPresentation {
  readonly id: number;
  readonly kind: EventPresentationKind;
  readonly label: string;
  readonly safeDetail: string;
  readonly timestamp: string;
  readonly recognized: boolean;
}

export function presentNarrativeRow(row: NarrativeRow): NarrativeRowPresentation {
  const event: NormalizedGatewayEvent = {
    id: row.id,
    type: row.type,
    timestamp: row.timestamp,
    payload: row.payload,
    raw: row.raw,
    recognized: row.recognized,
    sentinel: row.sentinel,
  };
  const p = presentEvent(event);
  return {
    id: row.id,
    kind: p.kind,
    label: p.label,
    safeDetail: p.safeDetail,
    timestamp: row.timestamp,
    recognized: row.recognized,
  };
}

// --- Connection view state ---

import type { SseConnectionState } from '../sse/stream';
import type { AuthStatus } from '../state/stores';

export function connectionViewState(state: SseConnectionState): ViewState {
  switch (state) {
    case 'disconnected':
      return VIEW_STATES.disconnected;
    case 'connecting':
      return VIEW_STATES.loading;
    case 'connected':
      return VIEW_STATES.ready;
    case 'reconnecting':
      return VIEW_STATES.loading;
    case 'unauthenticated':
      return VIEW_STATES.unauthenticated;
  }
}

export function authViewState(status: AuthStatus): ViewState {
  switch (status) {
    case 'unauthenticated':
      return VIEW_STATES.unauthenticated;
    case 'authenticating':
    case 'enrolling':
      return VIEW_STATES.loading;
    case 'authenticated':
      return VIEW_STATES.ready;
    case 'error':
      return VIEW_STATES.error;
  }
}

// --- Overview presentation ---

export interface OverviewPresentation {
  readonly agentsRunning: number;
  readonly agentsRunningFreshness: ViewState;
  readonly tasksInQueue: number;
  readonly tasksInQueueFreshness: ViewState;
  readonly hasSuccessRate: boolean;
  readonly viewState: ViewState;
}

export function presentOverview(
  agentsRunning: number,
  agentsRunningFreshness: string,
  tasksInQueue: number,
  tasksInQueueFreshness: string,
  hasSuccessRate: boolean,
): OverviewPresentation {
  const freshToView = (f: string): ViewState =>
    f === 'unavailable' ? VIEW_STATES.unavailable : f === 'stale' ? VIEW_STATES.stale : VIEW_STATES.ready;
  return {
    agentsRunning,
    agentsRunningFreshness: freshToView(agentsRunningFreshness),
    tasksInQueue,
    tasksInQueueFreshness: freshToView(tasksInQueueFreshness),
    hasSuccessRate,
    viewState: VIEW_STATES.ready,
  };
}

// --- Measurements presentation ---

export type MeasurementViewState = 'observed' | 'stale' | 'unavailable' | 'unsupported';

export interface MeasurementPresentation {
  readonly metricId: string;
  readonly value: number;
  readonly unit: string;
  readonly status: MeasurementViewState;
  readonly viewState: ViewState;
}

export function presentMeasurement(
  metricId: string,
  value: number,
  unit: string,
  status: string,
): MeasurementPresentation {
  const viewState: ViewState = status === 'unavailable'
    ? VIEW_STATES.unavailable
    : status === 'unsupported'
      ? VIEW_STATES.unsupported
      : status === 'stale'
        ? VIEW_STATES.stale
        : VIEW_STATES.ready;
  return {
    metricId: escapeHtml(boundString(metricId, MAX_LABEL)),
    value,
    unit: escapeHtml(unit),
    status: status as MeasurementViewState,
    viewState,
  };
}
