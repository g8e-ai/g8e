// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Reference frontend entry point. Wires the audited g8e-adapter modules to
// render the homepage matrix from typed stores. This is a reference
// implementation that a builder can replace; the audited adapter code in src/
// is not replaced by generated presentation.
//
// In live mode, the reference frontend connects to a Gateway, authenticates
// with WebAuthn, reads observe data, and renders normalized SSE updates. In
// design-preview mode (features.design_preview), it loads typed fixtures
// from the contract pack instead. Fixtures never mix with connected data.
//
// Accessibility: semantic landmarks, keyboard navigation, visible focus,
// non-color status labels, reduced motion, screen-reader connection
// announcements, mobile-first layout.

import { parseRuntimeConfig } from '../src/config/runtime_config';
import { createCredentialedFetch } from '../src/api/fetch';
import { createObserveClient } from '../src/api/observe_client';
import { SseStream, type SseConnectionState } from '../src/sse/stream';
import {
  adapterReducer,
  INITIAL_ADAPTER_STATE,
  type AdapterState,
} from '../src/state/stores';
import type { NormalizedGatewayEvent } from '../src/sse/normalizer';
import {
  renderPage,
  renderConnectionSection,
} from './render';
import { isDesignPreview, loadFixtureIntoState, type PreviewFixture } from './preview';

function readRuntimeConfig(): ReturnType<typeof parseRuntimeConfig> {
  const el = document.getElementById('g8e-runtime-config');
  if (!el || !el.textContent) {
    throw new Error('runtime config script tag not found');
  }
  return parseRuntimeConfig(JSON.parse(el.textContent));
}

function updateMainContent(html: string): void {
  const main = document.getElementById('main-content');
  if (main) main.innerHTML = html;
}

function updateConnectionStatus(connState: SseConnectionState): void {
  const el = document.getElementById('connection-live');
  if (!el) return;
  el.innerHTML = renderConnectionSection(connState);
  // Screen-reader announcement for connection state changes.
  el.setAttribute('aria-live', 'polite');
}

async function runLiveMode(config: ReturnType<typeof parseRuntimeConfig>): Promise<void> {
  const fetchImpl = createCredentialedFetch(config);
  const observeClient = createObserveClient(fetchImpl);

  let state: AdapterState = INITIAL_ADAPTER_STATE;
  state = adapterReducer(state, { type: 'runtimeFeatures', action: { type: 'config_loaded', config } });
  updateMainContent(renderPage(state, 'disconnected', false));

  const stream = new SseStream({
    config,
    callbacks: {
      onStateChange: (s: SseConnectionState) => {
        state = adapterReducer(state, { type: 'transport', action: { type: 'connection_state', state: s } });
        updateConnectionStatus(s);
      },
      onEvent: (event: NormalizedGatewayEvent) => {
        state = adapterReducer(state, { type: 'narrative', action: { type: 'event', event } });
        if (event.recognized) {
          observeClient.getBootstrap().then((snapshot) => {
            state = adapterReducer(state, { type: 'projections', action: { type: 'bootstrap_loaded', snapshot } });
            updateMainContent(renderPage(state, state.transport.connectionState, false));
          }).catch(() => { /* best-effort */ });
        }
        updateMainContent(renderPage(state, state.transport.connectionState, false));
      },
      onReconcile: (_reason: string) => {
        observeClient.getBootstrap().then((snapshot) => {
          state = adapterReducer(state, { type: 'projections', action: { type: 'bootstrap_loaded', snapshot } });
          updateMainContent(renderPage(state, state.transport.connectionState, false));
        }).catch(() => { /* best-effort */ });
      },
      onUnauthenticated: () => {
        state = adapterReducer(state, { type: 'auth', action: { type: 'auth_error', error: 'Session expired' } });
        updateMainContent(renderPage(state, state.transport.connectionState, false));
      },
    },
  });

  try {
    const snapshot = await observeClient.getBootstrap();
    state = adapterReducer(state, { type: 'projections', action: { type: 'bootstrap_loaded', snapshot } });
    updateMainContent(renderPage(state, state.transport.connectionState, false));
  } catch {
    state = adapterReducer(state, { type: 'auth', action: { type: 'auth_error', error: 'Failed to load' } });
    updateMainContent(renderPage(state, state.transport.connectionState, false));
  }

  stream.connect();

  document.addEventListener('visibilitychange', () => {
    stream.onVisibilityChange(!document.hidden);
  });

  // "Watch Live" focuses the narrative section; it never starts work.
  const watchLiveBtn = document.getElementById('watch-live');
  if (watchLiveBtn) {
    watchLiveBtn.addEventListener('click', () => {
      const narrative = document.getElementById('narrative');
      if (narrative) narrative.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  }

  (window as unknown as Record<string, unknown>).g8eAdapter = { config, stream, observeClient, getState: () => state };
}

async function runDesignPreviewMode(_config: ReturnType<typeof parseRuntimeConfig>): Promise<void> {
  // Load the bootstrap fixture as a representative preview.
  const fixture: PreviewFixture = {
    scenario: 'bootstrap',
    description: 'Design preview fixture',
    snapshot: {
      schema_version: '1.0.0',
      agents: [{
        schema_version: '1.0.0',
        agent_id: 'preview:sage',
        display_name: 'Sage',
        role: 'sage',
        status: 'running',
        freshness: 'observed',
        observed_at: '2026-09-09T12:00:00Z',
      }],
      active_run: {
        schema_version: '1.0.0',
        run_id: 'preview-inv-001',
        run_kind: 'investigation',
        display_name: 'Preview investigation',
        status: 'running',
        completed_tasks: 0,
        total_tasks: 0,
        has_receipts: false,
        evidence_count: 0,
        observed_at: '2026-09-09T12:00:00Z',
      },
      overview: {
        schema_version: '1.0.0',
        agents_running: 1,
        agents_running_freshness: 'observed',
        tasks_in_queue: 0,
        tasks_in_queue_freshness: 'observed',
        generated_at: '2026-09-09T12:00:00Z',
      },
      measurements: { schema_version: '1.0.0' },
      recent_runs: [{
        schema_version: '1.0.0',
        run_id: 'preview-inv-001',
        run_kind: 'investigation',
        display_name: 'Preview investigation',
        status: 'running',
        completed_tasks: 0,
        total_tasks: 0,
        has_receipts: false,
        evidence_count: 0,
        observed_at: '2026-09-09T12:00:00Z',
      }],
      latest_evals: [],
      downloads: [],
      generated_at: '2026-09-09T12:00:00Z',
    },
    auth: { status: 'authenticated' },
  };

  const preview = loadFixtureIntoState(fixture);
  updateMainContent(renderPage(preview.state, preview.connectionState as SseConnectionState, true));
}

async function main(): Promise<void> {
  const config = readRuntimeConfig();
  const designPreview = isDesignPreview(config.features as Record<string, boolean> | undefined);

  if (designPreview) {
    await runDesignPreviewMode(config);
  } else {
    await runLiveMode(config);
  }
}

main().catch((err) => {
  console.error('g8e reference frontend failed to start:', err);
  const el = document.getElementById('main-content');
  if (el) el.innerHTML = '<section class="error" role="alert"><h2>Error</h2><p>Failed to start.</p></section>';
});
