// Pure render functions for the reference frontend. These map typed AdapterState
// to safe HTML strings using the audited presentation registry. No DOM access,
// no network calls, no side effects. Every dynamic value is escaped and bounded.
// Every absent typed source produces an explicit unavailable/stale/empty state.
//
// These functions are testable without a DOM. The reference-ui/main.ts module
// wires them to the adapter's event sources and injects the resulting HTML into
// the document.

import {
  authViewState,
  connectionViewState,
  escapeHtml,
  boundString,
  presentAgent,
  presentRun,
  presentEval,
  presentNarrativeRow,
} from '../src/presentation/registry';
import type { AdapterState } from '../src/state/stores';
import type { SseConnectionState } from '../src/sse/stream';

const MAX_NARRATIVE_ROWS = 100;

function unavailable(label: string): string {
  return `<span class="state-unavailable" role="status">${escapeHtml(label)}</span>`;
}

function empty(label: string): string {
  return `<span class="state-empty" role="status">${escapeHtml(label)}</span>`;
}

function loading(label: string): string {
  return `<span class="state-loading" role="status" aria-live="polite">${escapeHtml(label)}</span>`;
}

export function renderAuthSection(state: AdapterState): string {
  const vs = authViewState(state.auth.status);
  const statusLabel = vs.status === 'ready' ? 'Authenticated' : vs.message;
  const userName = state.auth.userName ? escapeHtml(state.auth.userName) : '';
  return `<section id="auth" aria-label="Authentication" class="section-auth">
    <h2>Authentication</h2>
    <p class="status-label" data-status="${vs.status}" role="status">${escapeHtml(statusLabel)}${userName ? ' — ' + userName : ''}</p>
    ${state.auth.error ? `<p class="error" role="alert">${escapeHtml(state.auth.error)}</p>` : ''}
  </section>`;
}

export function renderConnectionSection(connState: SseConnectionState): string {
  const vs = connectionViewState(connState);
  const label = connState.charAt(0).toUpperCase() + connState.slice(1);
  return `<section id="connection" aria-label="Connection" class="section-connection">
    <h2>Connection</h2>
    <p class="status-label" data-status="${vs.status}" role="status" aria-live="polite">${escapeHtml(label)}</p>
  </section>`;
}

export function renderOverviewSection(state: AdapterState): string {
  let body: string;
  if (!state.projections.loaded) {
    body = loading('Loading overview…');
  } else {
    const o = state.projections.overview;
    if (!o) {
      body = unavailable('Overview unavailable');
    } else {
      const agentsFreshness = escapeHtml(o.agents_running_freshness);
      const tasksFreshness = escapeHtml(o.tasks_in_queue_freshness);
      const successRate = o.success_rate
        ? `${escapeHtml(String(o.success_rate.value))} ${escapeHtml(o.success_rate.unit)}`
        : unavailable('Success rate unavailable');
      body = `<div class="overview-grid">
      <div class="metric-card" data-freshness="${agentsFreshness}">
        <span class="metric-label">Agents running</span>
        <span class="metric-value">${escapeHtml(String(o.agents_running))}</span>
        <span class="metric-freshness">${agentsFreshness}</span>
      </div>
      <div class="metric-card" data-freshness="${tasksFreshness}">
        <span class="metric-label">Tasks in queue</span>
        <span class="metric-value">${escapeHtml(String(o.tasks_in_queue))}</span>
        <span class="metric-freshness">${tasksFreshness}</span>
      </div>
      <div class="metric-card">
        <span class="metric-label">Success rate</span>
        <span class="metric-value">${successRate}</span>
      </div>
    </div>`;
    }
  }
  return `<section id="overview" aria-label="Overview" class="section-overview">
    <h2>Overview</h2>
    ${body}
  </section>`;
}

export function renderAgentsSection(state: AdapterState): string {
  let body: string;
  if (!state.projections.loaded) {
    body = loading('Loading agents…');
  } else {
    const agents = state.projections.agents;
    if (agents.length === 0) {
      body = empty('No agents');
    } else {
      body = `<table class="data-table"><thead><tr><th>Name</th><th>Role</th><th>Status</th><th>Freshness</th></tr></thead><tbody>${agents.map((a) => {
        const p = presentAgent(a);
        return `<tr>
      <td>${p.displayName}</td>
      <td>${p.role}</td>
      <td class="status-label" data-status="${p.statusLabel}">${escapeHtml(p.statusLabel)}</td>
      <td>${escapeHtml(p.freshness)}</td>
    </tr>`;
      }).join('')}</tbody></table>`;
    }
  }
  return `<section id="agents" aria-label="Agent roster" class="section-agents">
    <h2>Agents</h2>
    ${body}
  </section>`;
}

export function renderRunsSection(state: AdapterState): string {
  let body: string;
  if (!state.projections.loaded) {
    body = loading('Loading runs…');
  } else {
    const runs = state.projections.recentRuns;
    if (runs.length === 0) {
      body = empty('No runs');
    } else {
      const rows = runs.map((r) => {
        const p = presentRun(r);
        return `<tr>
      <td>${p.displayName}</td>
      <td>${escapeHtml(p.runKind)}</td>
      <td class="status-label" data-status="${p.statusLabel}">${escapeHtml(p.statusLabel)}</td>
      <td>${p.completedTasks}/${p.totalTasks}</td>
    </tr>`;
      }).join('');
      body = `<table class="data-table"><thead><tr><th>Name</th><th>Kind</th><th>Status</th><th>Tasks</th></tr></thead><tbody>${rows}</tbody></table>`;
    }
  }
  return `<section id="runs" aria-label="Recent runs" class="section-runs">
    <h2>Recent runs</h2>
    ${body}
  </section>`;
}

export function renderEvalsSection(state: AdapterState): string {
  let body: string;
  if (!state.projections.loaded) {
    body = unavailable('No evals available');
  } else {
    const evals = state.projections.latestEvals;
    if (evals.length === 0) {
      body = empty('No evals available');
    } else {
      const rows = evals.map((e) => {
        const p = presentEval(e);
        return `<tr>
      <td>${boundString(p.runId, 40)}</td>
      <td class="status-label" data-status="${p.viewState.status}">${escapeHtml(p.verificationLabel)}</td>
      <td>${p.receiptCount}</td>
      <td>${p.metricCount}</td>
    </tr>`;
      }).join('');
      body = `<table class="data-table"><thead><tr><th>Run</th><th>Verification</th><th>Receipts</th><th>Metrics</th></tr></thead><tbody>${rows}</tbody></table>`;
    }
  }
  return `<section id="evals" aria-label="Latest evals" class="section-evals">
    <h2>Latest evals</h2>
    ${body}
  </section>`;
}

export function renderDownloadsSection(state: AdapterState): string {
  let body: string;
  if (!state.projections.loaded) {
    body = loading('Loading downloads…');
  } else {
    const downloads = state.projections.downloads;
    if (downloads.length === 0) {
      body = empty('No downloads available');
    } else {
      const rows = downloads.map((d) => {
        return `<tr>
      <td>${escapeHtml(d.filename)}</td>
      <td>${escapeHtml(d.media_type)}</td>
      <td>${escapeHtml(d.sha256.slice(0, 12))}…</td>
      <td>${escapeHtml(d.privacy_classification)}</td>
    </tr>`;
      }).join('');
      body = `<table class="data-table"><thead><tr><th>Filename</th><th>Media type</th><th>SHA-256</th><th>Classification</th></tr></thead><tbody>${rows}</tbody></table>`;
    }
  }
  return `<section id="downloads" aria-label="Downloads" class="section-downloads">
    <h2>Downloads</h2>
    ${body}
  </section>`;
}

export function renderNarrativeSection(state: AdapterState): string {
  const rows = state.narrative.rows.slice(-MAX_NARRATIVE_ROWS);
  let body: string;
  if (rows.length === 0) {
    body = empty('No events');
  } else {
    const items = rows.map((row) => {
      const p = presentNarrativeRow(row);
      return `<li class="narrative-row" data-recognized="${row.recognized}">
      <span class="narrative-label">${escapeHtml(p.label)}</span>
      <span class="narrative-detail">${escapeHtml(boundString(p.safeDetail, 200))}</span>
    </li>`;
    }).join('');
    body = `<ul class="narrative-list" role="log" aria-live="polite" aria-relevant="additions">${items}</ul>`;
  }
  return `<section id="narrative" aria-label="Live narrative" class="section-narrative">
    <h2>Live narrative</h2>
    ${body}
  </section>`;
}

export function renderMeasurementsSection(state: AdapterState): string {
  let body: string;
  if (!state.projections.loaded) {
    body = loading('Loading measurements…');
  } else {
    const m = state.projections.measurements;
    if (!m) {
      body = unavailable('Measurements unavailable');
    } else {
      // Resource and throughput cards remain unavailable until a real host
      // telemetry collector exists. Show explicit unavailable states.
      body = `<div class="measurements-grid">${['total_throughput', 'cpu', 'ram', 'vram', 'disk'].map((key) => {
        const val = (m as unknown as Record<string, unknown>)[key];
        const label = key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
        if (val === undefined || val === null) {
          return `<div class="metric-card unavailable" data-metric="${escapeHtml(key)}">
        <span class="metric-label">${escapeHtml(label)}</span>
        <span class="metric-value">${unavailable('Unavailable')}</span>
      </div>`;
        }
        return `<div class="metric-card" data-metric="${escapeHtml(key)}">
        <span class="metric-label">${escapeHtml(label)}</span>
        <span class="metric-value">${escapeHtml(String((val as { value: unknown }).value))}</span>
      </div>`;
      }).join('')}</div>`;
    }
  }
  return `<section id="measurements" aria-label="Resource measurements" class="section-measurements">
    <h2>Resource measurements</h2>
    ${body}
  </section>`;
}

export function renderDesignPreviewBanner(): string {
  return `<div id="design-preview-banner" role="banner" class="design-preview-banner">
    <strong>Design Preview Mode</strong> — Showing fixture data, not live Gateway data. Fixtures never mix with connected data.
  </div>`;
}

export function renderPage(state: AdapterState, connState: SseConnectionState, designPreview: boolean): string {
  const banner = designPreview ? renderDesignPreviewBanner() : '';
  return `${banner}
${renderAuthSection(state)}
${renderConnectionSection(connState)}
${renderOverviewSection(state)}
${renderMeasurementsSection(state)}
${renderAgentsSection(state)}
${renderRunsSection(state)}
${renderEvalsSection(state)}
${renderDownloadsSection(state)}
${renderNarrativeSection(state)}`;
}
