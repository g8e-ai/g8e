import { describe, expect, it } from 'vitest';

import { parseRuntimeConfig, RUNTIME_CONFIG_SCHEMA_VERSION } from '../src/config/runtime_config';
import { normalizeGatewayEvent } from '../src/sse/normalizer';
import {
  adapterReducer,
  INITIAL_ADAPTER_STATE,
  INITIAL_AUTH_STATE,
  INITIAL_NARRATIVE_STATE,
  INITIAL_PROJECTIONS_STATE,
  INITIAL_RUNTIME_FEATURES_STATE,
  INITIAL_TRANSPORT_STATE,
  NARRATIVE_MAX_ROWS,
  authReducer,
  narrativeReducer,
  projectionsReducer,
  runtimeFeaturesReducer,
  transportReducer,
} from '../src/state/stores';
import type {
  AgentStateProjection,
  DownloadArtifact,
  EvalSummary,
  ObserveBootstrapSnapshot,
  OverviewCounters,
  OverviewMeasurements,
  RunSummary,
} from '../src/types/observe';

function makeConfig() {
  return parseRuntimeConfig({
    schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
    gateway_base_url: 'https://g8e.example.com',
    passkey_rp_id: 'g8e.example.com',
    passkey_rp_name: 'g8e',
    app_name: 'g8e',
  });
}

function agentProjection(id: string, status: AgentStateProjection['status'] = 'running'): AgentStateProjection {
  return {
    schema_version: '1.0.0',
    agent_id: id,
    display_name: 'Sage',
    role: 'sage',
    status,
    freshness: 'observed',
    observed_at: '2026-01-01T00:00:00Z',
  };
}

function runSummary(id: string): RunSummary {
  return {
    schema_version: '1.0.0',
    run_id: id,
    run_kind: 'investigation',
    display_name: 'Case',
    status: 'running',
    completed_tasks: 0,
    total_tasks: 0,
    has_receipts: false,
    evidence_count: 0,
    observed_at: '2026-01-01T00:00:00Z',
  };
}

function evalSummary(id: string): EvalSummary {
  return {
    schema_version: '1.0.0',
    run_id: id,
    suite_id: 'suite-1',
    suite_version: '1.0.0',
    arm_id: 'arm-1',
    status: 'completed',
    verification_status: 'verified',
    receipt_count: 1,
    metric_count: 1,
    observed_at: '2026-01-01T00:00:00Z',
  };
}

function downloadArtifact(id: string): DownloadArtifact {
  return {
    schema_version: '1.0.0',
    artifact_id: id,
    filename: 'report.json',
    media_type: 'application/json',
    byte_size: 100,
    sha256: 'abc123',
    privacy_classification: 'public_safe',
    download_url: '/api/v1/observe/downloads/' + id,
    generated_at: '2026-01-01T00:00:00Z',
  };
}

function overview(): OverviewCounters {
  return {
    schema_version: '1.0.0',
    agents_running: 1,
    agents_running_freshness: 'observed',
    tasks_in_queue: 0,
    tasks_in_queue_freshness: 'observed',
    generated_at: '2026-01-01T00:00:00Z',
  };
}

function measurements(): OverviewMeasurements {
  return { schema_version: '1.0.0' };
}

function bootstrapSnapshot(): ObserveBootstrapSnapshot {
  return {
    schema_version: '1.0.0',
    agents: [agentProjection('u:sage')],
    overview: overview(),
    measurements: measurements(),
    recent_runs: [runSummary('inv-1')],
    latest_evals: [evalSummary('eval-1')],
    downloads: [downloadArtifact('dl-1')],
    generated_at: '2026-01-01T00:00:00Z',
  };
}

function agentEvent(id: number, agentId = 'u:sage'): string {
  return JSON.stringify({
    id,
    event: JSON.stringify({
      type: 'app.agent.status.updated',
      data: {
        schema_version: '1.0.0',
        agent_id: agentId,
        display_name: 'Sage',
        role: 'sage',
        status: 'running',
        observed_at: '2026-01-01T00:00:00Z',
      },
    }),
  });
}

describe('authReducer', () => {
  it('initial state is unauthenticated with null fields', () => {
    expect(INITIAL_AUTH_STATE.status).toBe('unauthenticated');
    expect(INITIAL_AUTH_STATE.bootstrapped).toBeNull();
    expect(INITIAL_AUTH_STATE.userId).toBeNull();
  });

  it('bootstrap_status sets the bootstrapped flag', () => {
    const s = authReducer(INITIAL_AUTH_STATE, { type: 'bootstrap_status', bootstrapped: true });
    expect(s.bootstrapped).toBe(true);
    expect(s.status).toBe('unauthenticated');
  });

  it('auth_start transitions to authenticating and clears error', () => {
    const errState = { ...INITIAL_AUTH_STATE, status: 'error', error: 'bad' };
    const s = authReducer(errState, { type: 'auth_start' });
    expect(s.status).toBe('authenticating');
    expect(s.error).toBeNull();
  });

  it('enroll_start transitions to enrolling', () => {
    const s = authReducer(INITIAL_AUTH_STATE, { type: 'enroll_start' });
    expect(s.status).toBe('enrolling');
  });

  it('auth_success sets user and session', () => {
    const s = authReducer(INITIAL_AUTH_STATE, {
      type: 'auth_success',
      userId: 'u1',
      userName: 'User',
      webSessionId: 'ws1',
    });
    expect(s.status).toBe('authenticated');
    expect(s.userId).toBe('u1');
    expect(s.userName).toBe('User');
    expect(s.webSessionId).toBe('ws1');
  });

  it('auth_error sets error and status', () => {
    const s = authReducer(INITIAL_AUTH_STATE, { type: 'auth_error', error: 'denied' });
    expect(s.status).toBe('error');
    expect(s.error).toBe('denied');
  });

  it('logout resets to initial but preserves bootstrapped', () => {
    const authed = { ...INITIAL_AUTH_STATE, status: 'authenticated' as const, userId: 'u1', bootstrapped: true };
    const s = authReducer(authed, { type: 'logout' });
    expect(s.status).toBe('unauthenticated');
    expect(s.userId).toBeNull();
    expect(s.bootstrapped).toBe(true);
  });
});

describe('projectionsReducer', () => {
  it('initial state is empty and not loaded', () => {
    expect(INITIAL_PROJECTIONS_STATE.agents).toEqual([]);
    expect(INITIAL_PROJECTIONS_STATE.loaded).toBe(false);
  });

  it('bootstrap_loaded populates all projections', () => {
    const s = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: bootstrapSnapshot(),
    });
    expect(s.loaded).toBe(true);
    expect(s.agents.length).toBe(1);
    expect(s.overview?.agents_running).toBe(1);
    expect(s.recentRuns.length).toBe(1);
    expect(s.latestEvals.length).toBe(1);
    expect(s.downloads.length).toBe(1);
    expect(s.lastUpdated).toBe('2026-01-01T00:00:00Z');
  });

  it('bootstrap_loaded with active_run sets activeRun', () => {
    const snap = bootstrapSnapshot();
    const s = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: { ...snap, active_run: runSummary('inv-active') },
    });
    expect(s.activeRun?.run_id).toBe('inv-active');
  });

  it('bootstrap_loaded without active_run sets activeRun to null', () => {
    const s = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: bootstrapSnapshot(),
    });
    expect(s.activeRun).toBeNull();
  });

  it('agent_updated replaces an existing agent by agent_id', () => {
    const loaded = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: bootstrapSnapshot(),
    });
    const updated = projectionsReducer(loaded, {
      type: 'agent_updated',
      agent: agentProjection('u:sage', 'completed'),
    });
    expect(updated.agents.length).toBe(1);
    expect(updated.agents[0].status).toBe('completed');
  });

  it('agent_updated appends a new agent', () => {
    const loaded = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: bootstrapSnapshot(),
    });
    const updated = projectionsReducer(loaded, {
      type: 'agent_updated',
      agent: agentProjection('u:dash'),
    });
    expect(updated.agents.length).toBe(2);
    expect(updated.agents[1].agent_id).toBe('u:dash');
  });

  it('runs_loaded replaces the recent runs list', () => {
    const loaded = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: bootstrapSnapshot(),
    });
    const s = projectionsReducer(loaded, {
      type: 'runs_loaded',
      runs: [runSummary('inv-2'), runSummary('inv-3')],
    });
    expect(s.recentRuns.length).toBe(2);
    expect(s.recentRuns[0].run_id).toBe('inv-2');
  });

  it('evals_loaded replaces the latest evals list', () => {
    const loaded = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: bootstrapSnapshot(),
    });
    const s = projectionsReducer(loaded, {
      type: 'evals_loaded',
      evals: [evalSummary('eval-2')],
    });
    expect(s.latestEvals.length).toBe(1);
    expect(s.latestEvals[0].run_id).toBe('eval-2');
  });

  it('downloads_loaded replaces the downloads list', () => {
    const loaded = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: bootstrapSnapshot(),
    });
    const s = projectionsReducer(loaded, {
      type: 'downloads_loaded',
      downloads: [downloadArtifact('dl-2')],
    });
    expect(s.downloads.length).toBe(1);
    expect(s.downloads[0].artifact_id).toBe('dl-2');
  });

  it('clear resets to initial state', () => {
    const loaded = projectionsReducer(INITIAL_PROJECTIONS_STATE, {
      type: 'bootstrap_loaded',
      snapshot: bootstrapSnapshot(),
    });
    const s = projectionsReducer(loaded, { type: 'clear' });
    expect(s.loaded).toBe(false);
    expect(s.agents).toEqual([]);
  });
});

describe('narrativeReducer', () => {
  it('initial state has no rows', () => {
    expect(INITIAL_NARRATIVE_STATE.rows).toEqual([]);
  });

  it('event adds a row from a normalized event', () => {
    const e = normalizeGatewayEvent(agentEvent(1), '1');
    const s = narrativeReducer(INITIAL_NARRATIVE_STATE, { type: 'event', event: e });
    expect(s.rows.length).toBe(1);
    expect(s.rows[0].id).toBe(1);
    expect(s.rows[0].type).toBe('app.agent.status.updated');
    expect(s.rows[0].recognized).toBe(true);
  });

  it('event deduplicates by durable id', () => {
    const e = normalizeGatewayEvent(agentEvent(5), '5');
    const s1 = narrativeReducer(INITIAL_NARRATIVE_STATE, { type: 'event', event: e });
    const s2 = narrativeReducer(s1, { type: 'event', event: e });
    expect(s2.rows.length).toBe(1);
  });

  it('event with id 0 does not dedup (allowing multiple id-less rows)', () => {
    const e1 = normalizeGatewayEvent(JSON.stringify({ event: JSON.stringify({ type: 'unknown', data: {} }) }), '0');
    const e2 = normalizeGatewayEvent(JSON.stringify({ event: JSON.stringify({ type: 'unknown', data: {} }) }), '0');
    const s1 = narrativeReducer(INITIAL_NARRATIVE_STATE, { type: 'event', event: e1 });
    const s2 = narrativeReducer(s1, { type: 'event', event: e2 });
    expect(s2.rows.length).toBe(2);
  });

  it('event trims to maxRows keeping the most recent', () => {
    let s = INITIAL_NARRATIVE_STATE;
    for (let i = 1; i <= 5; i++) {
      const e = normalizeGatewayEvent(agentEvent(i), String(i));
      s = narrativeReducer(s, { type: 'event', event: e, maxRows: 3 });
    }
    expect(s.rows.length).toBe(3);
    expect(s.rows[0].id).toBe(3);
    expect(s.rows[2].id).toBe(5);
  });

  it('event uses default maxRows when not specified', () => {
    expect(NARRATIVE_MAX_ROWS).toBe(200);
    let s = INITIAL_NARRATIVE_STATE;
    for (let i = 1; i <= NARRATIVE_MAX_ROWS + 5; i++) {
      const e = normalizeGatewayEvent(agentEvent(i), String(i));
      s = narrativeReducer(s, { type: 'event', event: e });
    }
    expect(s.rows.length).toBe(NARRATIVE_MAX_ROWS);
    expect(s.rows[0].id).toBe(6);
  });

  it('clear resets to empty', () => {
    const e = normalizeGatewayEvent(agentEvent(1), '1');
    const s1 = narrativeReducer(INITIAL_NARRATIVE_STATE, { type: 'event', event: e });
    const s2 = narrativeReducer(s1, { type: 'clear' });
    expect(s2.rows).toEqual([]);
  });

  it('retains unrecognized events as diagnostic rows', () => {
    const e = normalizeGatewayEvent(
      JSON.stringify({ event: JSON.stringify({ type: 'unknown.type', data: {} }) }),
      '1',
    );
    const s = narrativeReducer(INITIAL_NARRATIVE_STATE, { type: 'event', event: e });
    expect(s.rows.length).toBe(1);
    expect(s.rows[0].recognized).toBe(false);
  });

  it('retains sentinel events as rows', () => {
    const e = normalizeGatewayEvent(
      JSON.stringify({ event: JSON.stringify({ type: 'truncated', data: { since_id: 1, limit: 100 } }) }),
      '0',
    );
    const s = narrativeReducer(INITIAL_NARRATIVE_STATE, { type: 'event', event: e });
    expect(s.rows.length).toBe(1);
    expect(s.rows[0].sentinel).toBe(true);
  });
});

describe('transportReducer', () => {
  it('initial state is disconnected with zero event id', () => {
    expect(INITIAL_TRANSPORT_STATE.connectionState).toBe('disconnected');
    expect(INITIAL_TRANSPORT_STATE.lastEventId).toBe(0);
  });

  it('connection_state updates the connection state', () => {
    const s = transportReducer(INITIAL_TRANSPORT_STATE, { type: 'connection_state', state: 'connected' });
    expect(s.connectionState).toBe('connected');
  });

  it('event_id updates the last event id', () => {
    const s = transportReducer(INITIAL_TRANSPORT_STATE, { type: 'event_id', id: 42 });
    expect(s.lastEventId).toBe(42);
  });

  it('cursor sets the pagination cursor', () => {
    const s = transportReducer(INITIAL_TRANSPORT_STATE, { type: 'cursor', cursor: 'abc' });
    expect(s.cursor).toBe('abc');
  });

  it('reconcile sets the reconcile reason', () => {
    const s = transportReducer(INITIAL_TRANSPORT_STATE, { type: 'reconcile', reason: 'truncated' });
    expect(s.reconcileReason).toBe('truncated');
  });

  it('clear resets to initial', () => {
    const s = transportReducer(
      { ...INITIAL_TRANSPORT_STATE, connectionState: 'connected', lastEventId: 10 },
      { type: 'clear' },
    );
    expect(s.connectionState).toBe('disconnected');
    expect(s.lastEventId).toBe(0);
  });
});

describe('runtimeFeaturesReducer', () => {
  it('initial state has empty features and null config', () => {
    expect(INITIAL_RUNTIME_FEATURES_STATE.features).toEqual({});
    expect(INITIAL_RUNTIME_FEATURES_STATE.config).toBeNull();
  });

  it('config_loaded extracts features from the config', () => {
    const cfg = parseRuntimeConfig({
      schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
      gateway_base_url: 'https://g8e.example.com',
      passkey_rp_id: 'g8e.example.com',
      passkey_rp_name: 'g8e',
      app_name: 'g8e',
      features: { design_preview: true, eval_publication: false },
    });
    const s = runtimeFeaturesReducer(INITIAL_RUNTIME_FEATURES_STATE, { type: 'config_loaded', config: cfg });
    expect(s.features.design_preview).toBe(true);
    expect(s.features.eval_publication).toBe(false);
    expect(s.config).toBe(cfg);
  });

  it('config_loaded with no features sets empty features', () => {
    const s = runtimeFeaturesReducer(INITIAL_RUNTIME_FEATURES_STATE, { type: 'config_loaded', config: makeConfig() });
    expect(s.features).toEqual({});
  });

  it('clear resets to initial', () => {
    const cfg = makeConfig();
    const s = runtimeFeaturesReducer(
      { features: { x: true }, config: cfg },
      { type: 'clear' },
    );
    expect(s.features).toEqual({});
    expect(s.config).toBeNull();
  });
});

describe('adapterReducer', () => {
  it('initial state has all sub-stores at initial', () => {
    expect(INITIAL_ADAPTER_STATE.auth).toEqual(INITIAL_AUTH_STATE);
    expect(INITIAL_ADAPTER_STATE.projections).toEqual(INITIAL_PROJECTIONS_STATE);
    expect(INITIAL_ADAPTER_STATE.narrative).toEqual(INITIAL_NARRATIVE_STATE);
    expect(INITIAL_ADAPTER_STATE.transport).toEqual(INITIAL_TRANSPORT_STATE);
    expect(INITIAL_ADAPTER_STATE.runtimeFeatures).toEqual(INITIAL_RUNTIME_FEATURES_STATE);
  });

  it('auth action delegates to authReducer', () => {
    const s = adapterReducer(INITIAL_ADAPTER_STATE, {
      type: 'auth',
      action: { type: 'bootstrap_status', bootstrapped: true },
    });
    expect(s.auth.bootstrapped).toBe(true);
    expect(s.projections).toEqual(INITIAL_PROJECTIONS_STATE);
  });

  it('projections action delegates to projectionsReducer', () => {
    const s = adapterReducer(INITIAL_ADAPTER_STATE, {
      type: 'projections',
      action: { type: 'bootstrap_loaded', snapshot: bootstrapSnapshot() },
    });
    expect(s.projections.loaded).toBe(true);
    expect(s.auth).toEqual(INITIAL_AUTH_STATE);
  });

  it('narrative action delegates to narrativeReducer', () => {
    const e = normalizeGatewayEvent(agentEvent(1), '1');
    const s = adapterReducer(INITIAL_ADAPTER_STATE, { type: 'narrative', action: { type: 'event', event: e } });
    expect(s.narrative.rows.length).toBe(1);
  });

  it('transport action delegates to transportReducer', () => {
    const s = adapterReducer(INITIAL_ADAPTER_STATE, {
      type: 'transport',
      action: { type: 'connection_state', state: 'connected' },
    });
    expect(s.transport.connectionState).toBe('connected');
  });

  it('runtimeFeatures action delegates to runtimeFeaturesReducer', () => {
    const s = adapterReducer(INITIAL_ADAPTER_STATE, {
      type: 'runtimeFeatures',
      action: { type: 'config_loaded', config: makeConfig() },
    });
    expect(s.runtimeFeatures.config).not.toBeNull();
  });

  it('clear_all resets projections, narrative, and transport but preserves bootstrapped and runtime config', () => {
    const cfg = makeConfig();
    const populated = adapterReducer(
      adapterReducer(
        adapterReducer(
          adapterReducer(
            adapterReducer(INITIAL_ADAPTER_STATE, {
              type: 'auth',
              action: { type: 'bootstrap_status', bootstrapped: true },
            }),
            { type: 'projections', action: { type: 'bootstrap_loaded', snapshot: bootstrapSnapshot() } },
          ),
          { type: 'narrative', action: { type: 'event', event: normalizeGatewayEvent(agentEvent(1), '1') } },
        ),
        { type: 'transport', action: { type: 'connection_state', state: 'connected' } },
      ),
      { type: 'runtimeFeatures', action: { type: 'config_loaded', config: cfg } },
    );
    const s = adapterReducer(populated, { type: 'clear_all' });
    expect(s.auth.bootstrapped).toBe(true);
    expect(s.auth.status).toBe('unauthenticated');
    expect(s.projections.loaded).toBe(false);
    expect(s.narrative.rows).toEqual([]);
    expect(s.transport.connectionState).toBe('disconnected');
    expect(s.runtimeFeatures.config).toBe(cfg);
  });
});
