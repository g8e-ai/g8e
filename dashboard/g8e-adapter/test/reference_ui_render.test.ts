// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Reference frontend render tests (Packet 8C). Verifies that pure render
// functions produce safe HTML from typed state, that every absent typed
// source produces an explicit unavailable/stale/empty state, that no
// fabricated operational numbers appear, that design-preview mode is visibly
// labeled and isolated, and that accessibility markers are present.

import { describe, it, expect } from 'vitest';
import {
  renderAuthSection,
  renderConnectionSection,
  renderOverviewSection,
  renderAgentsSection,
  renderRunsSection,
  renderEvalsSection,
  renderDownloadsSection,
  renderNarrativeSection,
  renderMeasurementsSection,
  renderDesignPreviewBanner,
  renderPage,
} from '../reference-ui/render';
import { isDesignPreview, loadFixtureIntoState, type PreviewFixture } from '../reference-ui/preview';
import { INITIAL_ADAPTER_STATE, adapterReducer, type AdapterState } from '../src/state/stores';
import type { ObserveBootstrapSnapshot } from '../src/types/observe';
import type { NormalizedGatewayEvent } from '../src/sse/normalizer';

const ISO = '2026-09-09T12:00:00Z';

function bootstrapSnapshot(overrides: Partial<ObserveBootstrapSnapshot> = {}): ObserveBootstrapSnapshot {
  return {
    schema_version: '1.0.0',
    agents: [],
    overview: {
      schema_version: '1.0.0',
      agents_running: 0,
      agents_running_freshness: 'observed',
      tasks_in_queue: 0,
      tasks_in_queue_freshness: 'observed',
      generated_at: ISO,
    },
    measurements: { schema_version: '1.0.0' },
    recent_runs: [],
    latest_evals: [],
    downloads: [],
    generated_at: ISO,
    ...overrides,
  };
}

function stateWithSnapshot(snapshot: ObserveBootstrapSnapshot): AdapterState {
  return adapterReducer(INITIAL_ADAPTER_STATE, { type: 'projections', action: { type: 'bootstrap_loaded', snapshot } });
}

describe('reference-ui render: auth section', () => {
  it('shows unauthenticated state', () => {
    const html = renderAuthSection(INITIAL_ADAPTER_STATE);
    expect(html).toContain('Authentication');
    expect(html).toContain('unauthenticated');
    expect(html).toContain('role="status"');
  });

  it('shows authenticated state with user name', () => {
    const state = adapterReducer(INITIAL_ADAPTER_STATE, {
      type: 'auth',
      action: { type: 'auth_success', userId: 'u1', userName: 'Alice', webSessionId: 's1' },
    });
    const html = renderAuthSection(state);
    expect(html).toContain('Alice');
    expect(html).toContain('Authenticated');
  });

  it('shows error with role=alert', () => {
    const state = adapterReducer(INITIAL_ADAPTER_STATE, { type: 'auth', action: { type: 'auth_error', error: 'Bad token' } });
    const html = renderAuthSection(state);
    expect(html).toContain('role="alert"');
    expect(html).toContain('Bad token');
  });
});

describe('reference-ui render: connection section', () => {
  it('renders each connection state with a non-color label', () => {
    for (const cs of ['disconnected', 'connecting', 'connected', 'reconnecting', 'unauthenticated'] as const) {
      const html = renderConnectionSection(cs);
      expect(html).toContain(cs.charAt(0).toUpperCase() + cs.slice(1));
      expect(html).toContain('aria-live="polite"');
    }
  });
});

describe('reference-ui render: overview section', () => {
  it('shows loading before bootstrap', () => {
    const html = renderOverviewSection(INITIAL_ADAPTER_STATE);
    expect(html).toContain('Loading');
  });

  it('shows agents running and tasks in queue with freshness labels', () => {
    const state = stateWithSnapshot(bootstrapSnapshot({
      overview: {
        schema_version: '1.0.0',
        agents_running: 3,
        agents_running_freshness: 'observed',
        tasks_in_queue: 5,
        tasks_in_queue_freshness: 'observed',
        generated_at: ISO,
      },
    }));
    const html = renderOverviewSection(state);
    expect(html).toContain('Agents running');
    expect(html).toContain('3');
    expect(html).toContain('Tasks in queue');
    expect(html).toContain('5');
    expect(html).toContain('observed');
  });

  it('shows unavailable for success rate when absent', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderOverviewSection(state);
    expect(html).toContain('Success rate unavailable');
  });

  it('shows stale freshness label', () => {
    const state = stateWithSnapshot(bootstrapSnapshot({
      overview: {
        schema_version: '1.0.0',
        agents_running: 1,
        agents_running_freshness: 'stale',
        tasks_in_queue: 0,
        tasks_in_queue_freshness: 'stale',
        generated_at: ISO,
      },
    }));
    const html = renderOverviewSection(state);
    expect(html).toContain('stale');
  });
});

describe('reference-ui render: measurements section', () => {
  it('shows unavailable for all resource cards when no telemetry', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderMeasurementsSection(state);
    expect(html).toContain('Unavailable');
    expect(html).toContain('Total Throughput');
    expect(html).toContain('Cpu');
    expect(html).toContain('Ram');
    expect(html).toContain('Vram');
    expect(html).toContain('Disk');
  });

  it('does not fabricate CPU/RAM/throughput values', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderMeasurementsSection(state);
    // No numeric values in the measurement cards (only "Unavailable").
    const cardMatches = html.match(/metric-value">[^<]*<\/span/g) ?? [];
    for (const m of cardMatches) {
      expect(m).toContain('Unavailable');
    }
  });
});

describe('reference-ui render: agents section', () => {
  it('shows empty state when no agents', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderAgentsSection(state);
    expect(html).toContain('No agents');
  });

  it('renders agent roster with escaped display names', () => {
    const state = stateWithSnapshot(bootstrapSnapshot({
      agents: [{
        schema_version: '1.0.0',
        agent_id: 'u1:sage',
        display_name: 'Sage<script>',
        role: 'sage',
        status: 'running',
        freshness: 'observed',
        observed_at: ISO,
      }],
    }));
    const html = renderAgentsSection(state);
    // The display name is escaped; the raw script tag must not appear.
    expect(html).toContain('Sage');
    expect(html).not.toContain('<script>');
    // The escaped form uses HTML entities.
    expect(html).toContain('\x26lt;script\x26gt;');
  });
});

describe('reference-ui render: runs section', () => {
  it('shows empty state when no runs', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderRunsSection(state);
    expect(html).toContain('No runs');
  });

  it('renders run with status label and task counts', () => {
    const state = stateWithSnapshot(bootstrapSnapshot({
      recent_runs: [{
        schema_version: '1.0.0',
        run_id: 'inv-1',
        run_kind: 'investigation',
        display_name: 'Case A',
        status: 'running',
        completed_tasks: 2,
        total_tasks: 5,
        has_receipts: true,
        evidence_count: 3,
        observed_at: ISO,
      }],
    }));
    const html = renderRunsSection(state);
    expect(html).toContain('Case A');
    expect(html).toContain('investigation');
    expect(html).toContain('2/5');
  });
});

describe('reference-ui render: evals section', () => {
  it('shows unavailable when not loaded', () => {
    const html = renderEvalsSection(INITIAL_ADAPTER_STATE);
    expect(html).toContain('No evals available');
  });

  it('shows empty when loaded with no evals', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderEvalsSection(state);
    expect(html).toContain('No evals available');
  });

  it('shows projection_validated, never verified', () => {
    const state = stateWithSnapshot(bootstrapSnapshot({
      latest_evals: [{
        schema_version: '1.0.0',
        run_id: 'eval-1',
        suite_id: 's1',
        suite_version: '1.0.0',
        arm_id: 'a1',
        status: 'completed',
        verification_status: 'projection_validated',
        receipt_count: 2,
        metric_count: 1,
        published_projection_sha256: 'a'.repeat(64),
        completed_at: ISO,
        observed_at: ISO,
      }],
    }));
    const html = renderEvalsSection(state);
    expect(html).toContain('Projection validated');
    expect(html).not.toMatch(/verified["<]/i);
  });
});

describe('reference-ui render: downloads section', () => {
  it('shows empty when no downloads', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderDownloadsSection(state);
    expect(html).toContain('No downloads available');
  });

  it('renders public_safe artifacts with truncated hash', () => {
    const state = stateWithSnapshot(bootstrapSnapshot({
      downloads: [{
        schema_version: '1.0.0',
        artifact_id: 'art-1',
        filename: 'report.json',
        media_type: 'application/json',
        byte_size: 1024,
        sha256: 'b'.repeat(64),
        privacy_classification: 'public_safe',
        download_url: '/api/v1/observe/downloads/art-1',
        generated_at: ISO,
      }],
    }));
    const html = renderDownloadsSection(state);
    expect(html).toContain('report.json');
    expect(html).toContain('public_safe');
    expect(html).toContain('bbbbbbbbbbbb…');
  });
});

describe('reference-ui render: narrative section', () => {
  it('shows empty when no events', () => {
    const html = renderNarrativeSection(INITIAL_ADAPTER_STATE);
    expect(html).toContain('No events');
  });

  it('renders bounded narrative rows with aria-live', () => {
    let state = INITIAL_ADAPTER_STATE;
    for (let i = 1; i <= 5; i++) {
      const event: NormalizedGatewayEvent = {
        id: i,
        type: 'app.agent.status.updated',
        timestamp: ISO,
        recognized: true,
        sentinel: false,
        payload: { schema_version: '1.0.0', agent_id: 'u1:sage', display_name: 'Sage', role: 'sage', status: 'running', observed_at: ISO },
        raw: '',
      };
      state = adapterReducer(state, { type: 'narrative', action: { type: 'event', event } });
    }
    const html = renderNarrativeSection(state);
    expect(html).toContain('aria-live="polite"');
    expect(html).toContain('role="log"');
    expect(html).toContain('Sage');
  });
});

describe('reference-ui render: design-preview banner', () => {
  it('is visibly labeled', () => {
    const html = renderDesignPreviewBanner();
    expect(html).toContain('Design Preview Mode');
    expect(html).toContain('fixture data');
    expect(html).toContain('role="banner"');
  });
});

describe('reference-ui render: full page', () => {
  it('includes all sections in live mode (no banner)', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderPage(state, 'connected', false);
    expect(html).not.toContain('Design Preview Mode');
    expect(html).toContain('Authentication');
    expect(html).toContain('Connection');
    expect(html).toContain('Overview');
    expect(html).toContain('Resource measurements');
    expect(html).toContain('Agents');
    expect(html).toContain('Recent runs');
    expect(html).toContain('Latest evals');
    expect(html).toContain('Downloads');
    expect(html).toContain('Live narrative');
  });

  it('includes design-preview banner in preview mode', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderPage(state, 'disconnected', true);
    expect(html).toContain('Design Preview Mode');
  });
});

describe('reference-ui preview: design-preview mode', () => {
  it('isDesignPreview returns true only when design_preview is true', () => {
    expect(isDesignPreview({ design_preview: true })).toBe(true);
    expect(isDesignPreview({ design_preview: false })).toBe(false);
    expect(isDesignPreview(undefined)).toBe(false);
    expect(isDesignPreview({})).toBe(false);
  });

  it('loadFixtureIntoState populates projections from fixture snapshot', () => {
    const fixture: PreviewFixture = {
      scenario: 'bootstrap',
      description: 'test',
      snapshot: bootstrapSnapshot({
        agents: [{
          schema_version: '1.0.0',
          agent_id: 'preview:sage',
          display_name: 'Sage',
          role: 'sage',
          status: 'running',
          freshness: 'observed',
          observed_at: ISO,
        }],
      }),
    };
    const result = loadFixtureIntoState(fixture);
    expect(result.designPreview).toBe(true);
    expect(result.state.projections.loaded).toBe(true);
    expect(result.state.projections.agents).toHaveLength(1);
    expect(result.state.projections.agents[0].display_name).toBe('Sage');
  });

  it('loadFixtureIntoState populates narrative from fixture events', () => {
    const fixture: PreviewFixture = {
      scenario: 'live-stream',
      description: 'test',
      events: [
        { id: 1, type: 'app.agent.status.updated', payload: { schema_version: '1.0.0', agent_id: 'u1:sage', display_name: 'Sage', role: 'sage', status: 'running', observed_at: ISO }, recognized: true },
      ],
    };
    const result = loadFixtureIntoState(fixture);
    expect(result.state.narrative.rows).toHaveLength(1);
    expect(result.state.narrative.rows[0].type).toBe('app.agent.status.updated');
  });

  it('loadFixtureIntoState sets authenticated when auth status is authenticated', () => {
    const fixture: PreviewFixture = {
      scenario: 'authenticated',
      description: 'test',
      auth: { status: 'authenticated' },
    };
    const result = loadFixtureIntoState(fixture);
    expect(result.state.auth.status).toBe('authenticated');
    expect(result.state.auth.userName).toBe('Preview User');
  });

  it('fixtures never mix with connected data (preview state is isolated)', () => {
    const fixture: PreviewFixture = {
      scenario: 'bootstrap',
      description: 'test',
      snapshot: bootstrapSnapshot(),
    };
    const result = loadFixtureIntoState(fixture);
    // The preview state is independent from INITIAL_ADAPTER_STATE.
    expect(result.state).not.toBe(INITIAL_ADAPTER_STATE);
    expect(result.state.projections.loaded).toBe(true);
    // No live connection in preview mode.
    expect(result.connectionState).toBe('disconnected');
  });
});

describe('reference-ui accessibility markers', () => {
  it('all sections have aria-label and heading', () => {
    const state = stateWithSnapshot(bootstrapSnapshot());
    const html = renderPage(state, 'connected', false);
    // Each section has an aria-label.
    const ariaLabels = html.match(/aria-label="[^"]*"/g) ?? [];
    expect(ariaLabels.length).toBeGreaterThanOrEqual(8);
    // Each section has an h2.
    const headings = html.match(/<h2>[^<]*<\/h2>/g) ?? [];
    expect(headings.length).toBeGreaterThanOrEqual(8);
  });

  it('status labels use data-status attributes for non-color labeling', () => {
    const state = stateWithSnapshot(bootstrapSnapshot({
      overview: {
        schema_version: '1.0.0',
        agents_running: 1,
        agents_running_freshness: 'stale',
        tasks_in_queue: 0,
        tasks_in_queue_freshness: 'stale',
        generated_at: ISO,
      },
    }));
    const html = renderOverviewSection(state);
    expect(html).toContain('data-freshness="stale"');
    expect(html).toContain('stale');
  });

  it('narrative has role=log and aria-live', () => {
    let state = INITIAL_ADAPTER_STATE;
    const event: NormalizedGatewayEvent = {
      id: 1, type: 'app.agent.status.updated', timestamp: ISO, recognized: true, sentinel: false,
      payload: {}, raw: '',
    };
    state = adapterReducer(state, { type: 'narrative', action: { type: 'event', event } });
    const html = renderNarrativeSection(state);
    expect(html).toContain('role="log"');
    expect(html).toContain('aria-live="polite"');
  });
});
