// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Minimal host entry point. Wires the audited adapter modules together to
// exercise them against a real Gateway fixture in browser contract tests.
// This is NOT a generated UI — it renders honest states from typed stores
// and does not fabricate data.

import { parseRuntimeConfig } from '../src/config/runtime_config';
import { createCredentialedFetch } from '../src/api/fetch';
import { createObserveClient } from '../src/api/observe_client';
import { SseStream, type SseConnectionState } from '../src/sse/stream';
import {
  adapterReducer,
  INITIAL_ADAPTER_STATE,
  type AdapterState,
  type AuthStatus,
} from '../src/state/stores';
import {
  authViewState,
  connectionViewState,
  presentAgent,
  presentEval,
  presentNarrativeRow,
  presentRun,
} from '../src/presentation/registry';
import type { NormalizedGatewayEvent } from '../src/sse/normalizer';

function readRuntimeConfig(): ReturnType<typeof parseRuntimeConfig> {
  const el = document.getElementById('g8e-runtime-config');
  if (!el || !el.textContent) {
    throw new Error('runtime config script tag not found');
  }
  return parseRuntimeConfig(JSON.parse(el.textContent));
}

function setStateClass(el: HTMLElement, status: string): void {
  el.className = 'state state-' + status;
}

function renderAuthState(state: AdapterState): void {
  const el = document.getElementById('auth-state');
  if (!el) return;
  const vs = authViewState(state.auth.status as AuthStatus);
  el.textContent = vs.message || vs.status;
  setStateClass(el, vs.status);
}

function renderConnectionState(state: SseConnectionState): void {
  const el = document.getElementById('connection-state');
  if (!el) return;
  const vs = connectionViewState(state);
  el.textContent = state;
  setStateClass(el, vs.status);
}

function renderOverview(state: AdapterState): void {
  const el = document.getElementById('overview');
  if (!el) return;
  if (!state.projections.loaded) {
    el.innerHTML = '<span class="unavailable">Loading…</span>';
    return;
  }
  const o = state.projections.overview;
  if (!o) {
    el.innerHTML = '<span class="unavailable">Unavailable</span>';
    return;
  }
  const agentsFreshness = o.agents_running_freshness === 'unavailable' ? 'unavailable' : o.agents_running_freshness;
  const tasksFreshness = o.tasks_in_queue_freshness === 'unavailable' ? 'unavailable' : o.tasks_in_queue_freshness;
  el.innerHTML = `<div>Agents running: ${o.agents_running} (${agentsFreshness})</div>` +
    `<div>Tasks in queue: ${o.tasks_in_queue} (${tasksFreshness})</div>` +
    `<div>Success rate: ${o.success_rate ? o.success_rate.value + ' ' + o.success_rate.unit : '<span class="unavailable">unavailable</span>'}</div>`;
}

function renderAgents(state: AdapterState): void {
  const el = document.getElementById('agents');
  if (!el) return;
  if (!state.projections.loaded) {
    el.innerHTML = '<span class="unavailable">Loading…</span>';
    return;
  }
  if (state.projections.agents.length === 0) {
    el.innerHTML = '<span class="unavailable">No agents</span>';
    return;
  }
  el.innerHTML = state.projections.agents.map((a) => {
    const p = presentAgent(a);
    return `<div class="row">${p.displayName} (${p.role}) — ${p.statusLabel} [${p.freshness}]</div>`;
  }).join('');
}

function renderRuns(state: AdapterState): void {
  const el = document.getElementById('runs');
  if (!el) return;
  if (!state.projections.loaded) {
    el.innerHTML = '<span class="unavailable">Loading…</span>';
    return;
  }
  if (state.projections.recentRuns.length === 0) {
    el.innerHTML = '<span class="unavailable">No runs</span>';
    return;
  }
  el.innerHTML = state.projections.recentRuns.map((r) => {
    const p = presentRun(r);
    return `<div class="row">${p.displayName} (${p.runKind}) — ${p.statusLabel} [${p.completedTasks}/${p.totalTasks} tasks]</div>`;
  }).join('');
}

function renderEvals(state: AdapterState): void {
  const el = document.getElementById('evals');
  if (!el) return;
  if (!state.projections.loaded) {
    el.innerHTML = '<span class="unavailable">No evals available</span>';
    return;
  }
  if (state.projections.latestEvals.length === 0) {
    el.innerHTML = '<span class="unavailable">No evals available</span>';
    return;
  }
  el.innerHTML = state.projections.latestEvals.map((e) => {
    const p = presentEval(e);
    return `<div class="row">${p.runId} — ${p.verificationLabel} [${p.receiptCount} receipts, ${p.metricCount} metrics]</div>`;
  }).join('');
}

function renderNarrative(state: AdapterState): void {
  const el = document.getElementById('narrative');
  if (!el) return;
  if (state.narrative.rows.length === 0) {
    el.innerHTML = '<span class="unavailable">No events</span>';
    return;
  }
  el.innerHTML = state.narrative.rows.map((row) => {
    const p = presentNarrativeRow(row);
    return `<div class="row">${p.label}: ${p.safeDetail}</div>`;
  }).join('');
}

function renderAll(state: AdapterState): void {
  renderAuthState(state);
  renderOverview(state);
  renderAgents(state);
  renderRuns(state);
  renderEvals(state);
  renderNarrative(state);
}

async function main(): Promise<void> {
  const config = readRuntimeConfig();
  const fetchImpl = createCredentialedFetch(config);
  const observeClient = createObserveClient(fetchImpl);

  let state: AdapterState = INITIAL_ADAPTER_STATE;
  state = adapterReducer(state, { type: 'runtimeFeatures', action: { type: 'config_loaded', config } });
  renderAll(state);

  // Wire SSE stream.
  const stream = new SseStream({
    config,
    callbacks: {
      onStateChange: (s: SseConnectionState) => {
        state = adapterReducer(state, { type: 'transport', action: { type: 'connection_state', state: s } });
        renderConnectionState(s);
      },
      onEvent: (event: NormalizedGatewayEvent) => {
        state = adapterReducer(state, { type: 'narrative', action: { type: 'event', event } });
        if (event.recognized && event.type === 'app.agent.status.updated') {
          const payload = event.payload as Record<string, unknown>;
          if (payload && typeof payload.agent_id === 'string') {
            // The agent projection update comes from the observe API, not
            // from the SSE event directly. SSE is invalidation; the snapshot
            // is truth. Re-fetch the bootstrap snapshot to reconcile.
            observeClient.getBootstrap().then((snapshot) => {
              state = adapterReducer(state, { type: 'projections', action: { type: 'bootstrap_loaded', snapshot } });
              renderAll(state);
            }).catch(() => {
              // Best-effort: if the re-fetch fails, keep the existing state.
            });
          }
        }
        renderNarrative(state);
      },
      onReconcile: (reason: string) => {
        state = adapterReducer(state, { type: 'transport', action: { type: 'reconcile', reason } });
        // On reconciliation, re-fetch the bootstrap snapshot as truth.
        observeClient.getBootstrap().then((snapshot) => {
          state = adapterReducer(state, { type: 'projections', action: { type: 'bootstrap_loaded', snapshot } });
          renderAll(state);
        }).catch(() => {
          // Best-effort reconciliation.
        });
      },
      onUnauthenticated: () => {
        state = adapterReducer(state, { type: 'auth', action: { type: 'auth_error', error: 'Session expired' } });
        renderAuthState(state);
      },
    },
  });

  // Initial bootstrap fetch.
  try {
    const snapshot = await observeClient.getBootstrap();
    state = adapterReducer(state, { type: 'projections', action: { type: 'bootstrap_loaded', snapshot } });
    renderAll(state);
  } catch {
    // If the initial fetch fails (e.g. unauthenticated), show the error state.
    state = adapterReducer(state, { type: 'auth', action: { type: 'auth_error', error: 'Failed to load' } });
    renderAll(state);
  }

  // Connect SSE.
  stream.connect();

  // Visibility reconciliation.
  document.addEventListener('visibilitychange', () => {
    stream.onVisibilityChange(!document.hidden);
  });

  // Expose for browser contract tests.
  (window as unknown as Record<string, unknown>).g8eAdapter = {
    config,
    stream,
    observeClient,
    getState: () => state,
  };
}

main().catch((err) => {
  console.error('g8e adapter host failed to start:', err);
  const el = document.getElementById('connection-state');
  if (el) {
    el.textContent = 'Error';
    setStateClass(el, 'error');
  }
});
