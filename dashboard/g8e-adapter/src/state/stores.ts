// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Typed state stores for the audited adapter. Separate stores for auth,
// projections, narrative rows, transport/cursor state, and runtime features.
// Reducers are pure functions: (state, action) -> state. No side effects, no
// network calls, no DOM access. The host wires reducers to the adapter's
// event sources (observe client responses, normalized SSE events, auth
// ceremonies, transport callbacks).

import type {
  AgentStateProjection,
  DownloadArtifact,
  EvalSummary,
  ObserveBootstrapSnapshot,
  OverviewCounters,
  OverviewMeasurements,
  RunSummary,
} from '../types/observe';
import type { NormalizedGatewayEvent } from '../sse/normalizer';
import type { SseConnectionState } from '../sse/stream';
import type { FrontendRuntimeConfig } from '../config/runtime_config';

// --- Auth store ---

export type AuthStatus =
  | 'unauthenticated'
  | 'authenticating'
  | 'enrolling'
  | 'authenticated'
  | 'error';

export interface AuthState {
  readonly status: AuthStatus;
  readonly bootstrapped: boolean | null;
  readonly userId: string | null;
  readonly userName: string | null;
  readonly webSessionId: string | null;
  readonly error: string | null;
}

export const INITIAL_AUTH_STATE: AuthState = {
  status: 'unauthenticated',
  bootstrapped: null,
  userId: null,
  userName: null,
  webSessionId: null,
  error: null,
};

export type AuthAction =
  | { type: 'bootstrap_status'; bootstrapped: boolean }
  | { type: 'auth_start' }
  | { type: 'enroll_start' }
  | { type: 'auth_success'; userId: string; userName: string; webSessionId: string }
  | { type: 'auth_error'; error: string }
  | { type: 'logout' };

export function authReducer(state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case 'bootstrap_status':
      return { ...state, bootstrapped: action.bootstrapped };
    case 'auth_start':
      return { ...state, status: 'authenticating', error: null };
    case 'enroll_start':
      return { ...state, status: 'enrolling', error: null };
    case 'auth_success':
      return {
        ...state,
        status: 'authenticated',
        userId: action.userId,
        userName: action.userName,
        webSessionId: action.webSessionId,
        error: null,
      };
    case 'auth_error':
      return { ...state, status: 'error', error: action.error };
    case 'logout':
      return { ...INITIAL_AUTH_STATE, bootstrapped: state.bootstrapped };
  }
}

// --- Projections store ---

export interface ProjectionsState {
  readonly agents: readonly AgentStateProjection[];
  readonly activeRun: RunSummary | null;
  readonly overview: OverviewCounters | null;
  readonly measurements: OverviewMeasurements | null;
  readonly recentRuns: readonly RunSummary[];
  readonly latestEvals: readonly EvalSummary[];
  readonly downloads: readonly DownloadArtifact[];
  readonly lastUpdated: string | null;
  readonly loaded: boolean;
}

export const INITIAL_PROJECTIONS_STATE: ProjectionsState = {
  agents: [],
  activeRun: null,
  overview: null,
  measurements: null,
  recentRuns: [],
  latestEvals: [],
  downloads: [],
  lastUpdated: null,
  loaded: false,
};

export type ProjectionsAction =
  | { type: 'bootstrap_loaded'; snapshot: ObserveBootstrapSnapshot }
  | { type: 'agent_updated'; agent: AgentStateProjection }
  | { type: 'runs_loaded'; runs: readonly RunSummary[] }
  | { type: 'evals_loaded'; evals: readonly EvalSummary[] }
  | { type: 'downloads_loaded'; downloads: readonly DownloadArtifact[] }
  | { type: 'clear' };

export function projectionsReducer(state: ProjectionsState, action: ProjectionsAction): ProjectionsState {
  switch (action.type) {
    case 'bootstrap_loaded': {
      const s = action.snapshot;
      return {
        agents: s.agents,
        activeRun: s.active_run ?? null,
        overview: s.overview,
        measurements: s.measurements,
        recentRuns: s.recent_runs,
        latestEvals: s.latest_evals,
        downloads: s.downloads,
        lastUpdated: s.generated_at,
        loaded: true,
      };
    }
    case 'agent_updated': {
      // Replace the agent with the same agent_id, or append. The agent_id is
      // the durable identity (user_id:persona_id).
      const idx = state.agents.findIndex((a) => a.agent_id === action.agent.agent_id);
      const agents = idx >= 0
        ? state.agents.map((a, i) => (i === idx ? action.agent : a))
        : [...state.agents, action.agent];
      return { ...state, agents, lastUpdated: action.agent.observed_at };
    }
    case 'runs_loaded':
      return { ...state, recentRuns: action.runs };
    case 'evals_loaded':
      return { ...state, latestEvals: action.evals };
    case 'downloads_loaded':
      return { ...state, downloads: action.downloads };
    case 'clear':
      return { ...INITIAL_PROJECTIONS_STATE };
  }
}

// --- Narrative rows store ---

// A bounded list of normalized SSE events for the live narrative feed. The
// store caps the number of retained rows to prevent unbounded memory growth.
// Unknown/unrecognized events are retained as bounded diagnostic rows so the
// presentation layer can display them without mutating projections.

export interface NarrativeRow {
  readonly id: number;
  readonly type: string;
  readonly timestamp: string;
  readonly recognized: boolean;
  readonly sentinel: boolean;
  readonly payload: unknown;
  readonly raw: string;
}

export interface NarrativeState {
  readonly rows: readonly NarrativeRow[];
}

export const NARRATIVE_MAX_ROWS = 200;

export const INITIAL_NARRATIVE_STATE: NarrativeState = {
  rows: [],
};

export type NarrativeAction =
  | { type: 'event'; event: NormalizedGatewayEvent; maxRows?: number }
  | { type: 'clear' };

export function narrativeReducer(state: NarrativeState, action: NarrativeAction): NarrativeState {
  switch (action.type) {
    case 'event': {
      const e = action.event;
      const row: NarrativeRow = {
        id: e.id,
        type: e.type,
        timestamp: e.timestamp,
        recognized: e.recognized,
        sentinel: e.sentinel,
        payload: e.payload,
        raw: e.raw,
      };
      // Dedup by durable id; a duplicate event does not add a new row.
      if (e.id !== 0 && state.rows.some((r) => r.id === e.id)) {
        return state;
      }
      const max = action.maxRows ?? NARRATIVE_MAX_ROWS;
      const rows = [...state.rows, row];
      // Trim to the most recent max rows.
      const trimmed = rows.length > max ? rows.slice(rows.length - max) : rows;
      return { rows: trimmed };
    }
    case 'clear':
      return { ...INITIAL_NARRATIVE_STATE };
  }
}

// --- Transport/cursor store ---

export interface TransportState {
  readonly connectionState: SseConnectionState;
  readonly lastEventId: number;
  readonly cursor: string | null;
  readonly reconcileReason: string | null;
}

export const INITIAL_TRANSPORT_STATE: TransportState = {
  connectionState: 'disconnected',
  lastEventId: 0,
  cursor: null,
  reconcileReason: null,
};

export type TransportAction =
  | { type: 'connection_state'; state: SseConnectionState }
  | { type: 'event_id'; id: number }
  | { type: 'cursor'; cursor: string | null }
  | { type: 'reconcile'; reason: string }
  | { type: 'clear' };

export function transportReducer(state: TransportState, action: TransportAction): TransportState {
  switch (action.type) {
    case 'connection_state':
      return { ...state, connectionState: action.state };
    case 'event_id':
      return { ...state, lastEventId: action.id };
    case 'cursor':
      return { ...state, cursor: action.cursor };
    case 'reconcile':
      return { ...state, reconcileReason: action.reason };
    case 'clear':
      return { ...INITIAL_TRANSPORT_STATE };
  }
}

// --- Runtime features store ---

export interface RuntimeFeaturesState {
  readonly features: Readonly<Record<string, boolean>>;
  readonly config: FrontendRuntimeConfig | null;
}

export const INITIAL_RUNTIME_FEATURES_STATE: RuntimeFeaturesState = {
  features: {},
  config: null,
};

export type RuntimeFeaturesAction =
  | { type: 'config_loaded'; config: FrontendRuntimeConfig }
  | { type: 'clear' };

export function runtimeFeaturesReducer(_state: RuntimeFeaturesState, action: RuntimeFeaturesAction): RuntimeFeaturesState {
  switch (action.type) {
    case 'config_loaded': {
      const features: Record<string, boolean> = {};
      const cfgFeatures = action.config.features as Record<string, boolean> | undefined;
      if (cfgFeatures) {
        for (const [k, v] of Object.entries(cfgFeatures)) {
          features[k] = v;
        }
      }
      return { features, config: action.config };
    }
    case 'clear':
      return { ...INITIAL_RUNTIME_FEATURES_STATE };
  }
}

// --- Combined root state and reducer ---

export interface AdapterState {
  readonly auth: AuthState;
  readonly projections: ProjectionsState;
  readonly narrative: NarrativeState;
  readonly transport: TransportState;
  readonly runtimeFeatures: RuntimeFeaturesState;
}

export const INITIAL_ADAPTER_STATE: AdapterState = {
  auth: INITIAL_AUTH_STATE,
  projections: INITIAL_PROJECTIONS_STATE,
  narrative: INITIAL_NARRATIVE_STATE,
  transport: INITIAL_TRANSPORT_STATE,
  runtimeFeatures: INITIAL_RUNTIME_FEATURES_STATE,
};

export type AdapterAction =
  | { type: 'auth'; action: AuthAction }
  | { type: 'projections'; action: ProjectionsAction }
  | { type: 'narrative'; action: NarrativeAction }
  | { type: 'transport'; action: TransportAction }
  | { type: 'runtimeFeatures'; action: RuntimeFeaturesAction }
  | { type: 'clear_all' };

export function adapterReducer(state: AdapterState, action: AdapterAction): AdapterState {
  switch (action.type) {
    case 'auth':
      return { ...state, auth: authReducer(state.auth, action.action) };
    case 'projections':
      return { ...state, projections: projectionsReducer(state.projections, action.action) };
    case 'narrative':
      return { ...state, narrative: narrativeReducer(state.narrative, action.action) };
    case 'transport':
      return { ...state, transport: transportReducer(state.transport, action.action) };
    case 'runtimeFeatures':
      return { ...state, runtimeFeatures: runtimeFeaturesReducer(state.runtimeFeatures, action.action) };
    case 'clear_all':
      return {
        auth: { ...INITIAL_AUTH_STATE, bootstrapped: state.auth.bootstrapped },
        projections: INITIAL_PROJECTIONS_STATE,
        narrative: INITIAL_NARRATIVE_STATE,
        transport: INITIAL_TRANSPORT_STATE,
        runtimeFeatures: state.runtimeFeatures,
      };
  }
}
