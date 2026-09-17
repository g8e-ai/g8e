// Overview view — the landing page. Layout mirrors the OpenDevOps.ai
// surface: the live event stream, a system overview
// with dataset coverage, the measured-model table, recent runs, and the
// public mirror download endpoints. Every panel renders real store data;
// nothing on this page is decorative.

import { forwardRef, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  G8E_REPO_URL,
  PLATFORM_FLOW_STEPS,
  PLATFORM_LEDE,
  PLATFORM_MEASUREMENT_SUMMARY,
} from '../content/platform';
import { useActiveDatasetId } from '../state/dataset';
import { modelComparisonId, recordKey, resolveModelSummary, useStoreState } from '../state/store';
import { loadRuntimeConfig } from '../state/feed';
import { DatasetSelector } from '../components/DatasetSelector';
import {
  EmptyState,
  ProgressBar,
  formatNumber,
  formatPercent,
  formatLatency,
  formatThroughput,
} from '../components/shared';
import { formatRelativeTime } from '../utils/format';
import { qualityStateLabel, qualityStateTone, type FeedConnectionState } from '../utils/feed-state';
import { campaignTerminalProgress, roleLabel } from './derived';
import descriptorUrl from '../contract/descriptor.json?url';
import type {
  CatalogSnapshot,
  EvaluationSummary,
  LiveEvent,
  ModelSummary,
  QualityState,
  SuiteSummary,
} from '../contract/types';

const ROLE_LABELS: Record<ModelSummary['role'], string> = {
  primary: 'Task owner & delegation',
  assistant: 'Bounded technical work',
  lite: 'Constrained decisions',
};

function agentStatus(state: QualityState): { label: string; tone: string } {
  const tone = qualityStateTone(state);
  switch (state) {
    case 'verified_public':
    case 'exploratory_verified':
      return { label: 'Verified', tone };
    case 'exploratory_partial':
      return { label: 'Partial', tone };
    case 'live_in_progress':
      return { label: 'Running', tone };
    case 'terminal_failed':
      return { label: 'Failed', tone };
    case 'dead_evidence':
      return { label: 'Dead evidence', tone };
    default:
      return { label: qualityStateLabel(state), tone };
  }
}

function eventTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleTimeString('en-US', { hour12: false });
}

function shortRunId(runId: string): string {
  return runId.length > 14 ? `…${runId.slice(-12)}` : runId;
}

/** Measured role candidates in the active dataset. */
function RoleLeadersPanel({ models, datasetId }: { models: ModelSummary[]; datasetId: string }) {
  const evaluatedModels = useMemo(
    () =>
      models
        .filter((m) => !m.inventory_only && m.pass_rate)
        .sort(
          (a, b) =>
            (b.output_throughput_p50?.value ?? 0) - (a.output_throughput_p50?.value ?? 0),
        ),
    [models],
  );

  return (
    <section className="panel ov-models-strip" aria-label="Role leaders">
      <div className="panel-head">
        <h2>
          Role leaders <span className="panel-sub">· {evaluatedModels.length} evaluated</span>
        </h2>
        <Link to={`/models?dataset=${datasetId}`} className="panel-link">
          View all models →
        </Link>
      </div>
      {evaluatedModels.length === 0 ? (
        <p className="panel-empty">No evaluated models in the active dataset.</p>
      ) : (
        <div className="table-scroll">
          <table className="lab-table">
            <thead>
              <tr>
                <th>Model</th>
                <th>Role</th>
                <th>Status</th>
                <th>Quant</th>
                <th>Tokens/s</th>
                <th>Agreement</th>
                <th>Pass rate</th>
                <th>Latency p50</th>
              </tr>
            </thead>
            <tbody>
              {evaluatedModels.map((model) => {
                const status = agentStatus(model.quality_state);
                return (
                  <tr key={modelComparisonId(model)}>
                    <td>
                      <Link to={`/models/${datasetId}/${model.variant_id}?role=${model.role}`}>
                        {model.display_name}
                      </Link>
                    </td>
                    <td>{ROLE_LABELS[model.role]}</td>
                    <td>
                      <span className={`stream-role status-${status.tone}`}>
                        <span className="status-dot" aria-hidden="true" />
                        {status.label}
                      </span>
                    </td>
                    <td>{model.quantization_weight_class?.toUpperCase() ?? '—'}</td>
                    <td>
                      {model.output_throughput_p50?.value !== undefined
                        ? formatThroughput(model.output_throughput_p50.value)
                        : '—'}
                    </td>
                    <td>
                      {model.agreement_pairwise?.value !== undefined
                        ? formatPercent(model.agreement_pairwise.value, 0)
                        : '—'}
                    </td>
                    <td>{model.pass_rate ? formatPercent(model.pass_rate.estimate, 0) : '—'}</td>
                    <td>
                      {model.latency_p50_ms?.value !== undefined
                        ? formatLatency(model.latency_p50_ms.value)
                        : '—'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/** Live event stream table fed by SSE projections, newest first. */
function LiveStreamPanel({
  events,
  connection,
}: {
  events: LiveEvent[];
  connection: FeedConnectionState;
}) {
  const [modelFilter, setModelFilter] = useState('all');
  const [kindFilter, setKindFilter] = useState('all');

  const models = useStoreState((state) => state.models);

  const modelOptions = useMemo(
    () =>
      Array.from(new Set(events.map((e) => e.variant_id).filter((v): v is string => Boolean(v)))).sort(),
    [events],
  );
  const kindOptions = useMemo(() => Array.from(new Set(events.map((e) => e.kind))).sort(), [events]);

  const visible = events
    .filter((e) => modelFilter === 'all' || e.variant_id === modelFilter)
    .filter((e) => kindFilter === 'all' || e.kind === kindFilter)
    .slice(-15)
    .reverse();

  return (
    <section className="panel stream-panel" id="live-stream" aria-label="Live event stream">
      <div className="panel-head">
        <h2>
          Live event stream{' '}
          <span className={`stream-state ${connection === 'live' ? 'status-ok' : 'status-warn'}`}>
            <span className="status-dot" aria-hidden="true" />
            {connection === 'live' ? 'Streaming via SSE' : 'Mirror offline — last accepted data'}
          </span>
        </h2>
        <div className="stream-controls">
          <select
            aria-label="Filter by model"
            value={modelFilter}
            onChange={(e) => setModelFilter(e.target.value)}
          >
            <option value="all">All models</option>
            {modelOptions.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </select>
          <select
            aria-label="Filter by event kind"
            value={kindFilter}
            onChange={(e) => setKindFilter(e.target.value)}
          >
            <option value="all">All events</option>
            {kindOptions.map((kind) => (
              <option key={kind} value={kind}>
                {kind.replace(/_/g, ' ')}
              </option>
            ))}
          </select>
        </div>
      </div>
      <div className="stream-body">
        {visible.length === 0 ? (
          <EmptyState hasRecords={events.length > 0} hasFilters={modelFilter !== 'all' || kindFilter !== 'all'} connection={connection} />
        ) : (
          <div className="table-scroll">
            <table className="stream-table">
            <thead>
              <tr>
                <th>Time</th>
                <th>Role</th>
                <th>Event</th>
                <th>Model</th>
                <th>Progress</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((event) => {
                const model = event.variant_id
                  ? resolveModelSummary(models, event.dataset_id, event.variant_id)
                  : undefined;
                const servedTag = model?.served_model_tag ?? event.variant_id ?? event.kind.split('_')[0];
                const eventLabel = `${event.kind.replace(/_/g, ' ')}${event.stage_label ? ` · ${event.stage_label}` : ''}`;
                const eventHref = event.assignment_id
                  ? `/evaluations/${event.dataset_id}/${event.run_id}/assignments/${event.assignment_id}`
                  : `/evaluations/${event.dataset_id}/${event.run_id}`;
                const modelHref = event.variant_id
                  ? `/models/${event.dataset_id}/${event.variant_id}${model?.role ? `?role=${model.role}` : ''}`
                  : undefined;
                const roleLabelText = model ? roleLabel(model.role) : event.variant_id ?? 'Platform';
                return (
                  <tr key={event.event_id}>
                    <td className="stream-time">{eventTime(event.observed_at)}</td>
                    <td>
                      <span className={`stream-role status-${event.lifecycle_status}`}>
                        <span className="status-dot" aria-hidden="true" />
                        {roleLabelText}
                      </span>
                    </td>
                    <td className="stream-event">
                      <Link to={eventHref} className="stream-event-link">
                        {eventLabel}
                      </Link>
                    </td>
                    <td>
                      {modelHref ? (
                        <Link to={modelHref} className="stream-chip stream-chip-link">
                          {servedTag}
                        </Link>
                      ) : (
                        <code className="stream-chip">{servedTag}</code>
                      )}
                    </td>
                    <td className="stream-progress">
                      {event.total > 0 ? `${event.completed}/${event.total}` : '—'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        )}
      </div>
    </section>
  );
}

function PlatformFlow() {
  return (
    <ol className="sys-platform-flow" aria-label="Platform data flow">
      {PLATFORM_FLOW_STEPS.map((step, index) => (
        <li key={step.id} className={step.id === 'g8e' ? 'sys-platform-step-accent' : undefined}>
          <span className="sys-platform-step-label">{step.label}</span>
          <span className="sys-platform-step-detail">{step.detail}</span>
          {index < PLATFORM_FLOW_STEPS.length - 1 ? (
            <span className="sys-platform-step-arrow" aria-hidden="true">↓</span>
          ) : null}
        </li>
      ))}
    </ol>
  );
}

function CoverageBar({ label, done, total }: { label: string; done: number; total: number }) {
  const pct = total > 0 ? Math.min(100, (done / total) * 100) : 0;
  return (
    <div className="usage-row">
      <div className="usage-label">
        <span>{label}</span>
        <span>
          {formatNumber(done)} / {formatNumber(total)}
        </span>
      </div>
      <div className="usage-track">
        <div className="usage-fill" style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

type SystemOverviewPanelProps = {
  catalog: CatalogSnapshot | undefined;
  evaluations: EvaluationSummary[];
  suites: SuiteSummary[];
  events: LiveEvent[];
  activeDatasetId: string;
  connection: FeedConnectionState;
};

/** System overview: platform context, active campaign progress, and current run. */
const SystemOverviewPanel = forwardRef<HTMLElement, SystemOverviewPanelProps>(function SystemOverviewPanel(
  {
    catalog,
    evaluations,
    suites,
    events,
    activeDatasetId,
    connection,
  },
  ref,
) {
  const completedRuns = evaluations.filter((e) => e.lifecycle_state === 'completed').length;
  const assignmentDone = evaluations.reduce(
    (sum, e) => sum + e.assignment_completed + e.assignment_failed,
    0,
  );
  const assignmentTotal = evaluations.reduce((sum, e) => sum + e.assignment_total, 0);

  const latestEvent = events.length > 0 ? events[events.length - 1] : undefined;
  const currentRun = useStoreState((state) =>
    latestEvent ? state.evaluations.get(recordKey(latestEvent.dataset_id, latestEvent.run_id)) : undefined,
  );
  const currentSuite = currentRun
    ? suites.find((s) => s.suite_id === currentRun.suite_id)
    : undefined;

  return (
    <section ref={ref} className="panel sys-panel" aria-label="System overview">
      <div className="panel-head">
        <h2>System overview</h2>
        <span className={`stream-state ${connection === 'live' ? 'status-ok' : 'status-warn'}`}>
          <span className="status-dot" aria-hidden="true" />
          {connection === 'live' ? 'Live' : 'Offline'}
        </span>
      </div>

      <div className="sys-platform">
        <p className="sys-platform-lede">{PLATFORM_LEDE}</p>
        <PlatformFlow />
        <p className="sys-platform-measure">{PLATFORM_MEASUREMENT_SUMMARY}</p>
        <div className="sys-platform-links">
          <a href={G8E_REPO_URL} target="_blank" rel="noopener noreferrer">g8e on GitHub</a>
          <Link to="/methodology#architecture">Architecture &amp; host specs</Link>
        </div>
      </div>

      <div className="sys-campaign">
        <h3 className="sys-section-title">Active campaign</h3>
        <DatasetSelector activeId={activeDatasetId} />
        {catalog ? <p className="panel-note">{catalog.title}</p> : null}

        {catalog ? (
          <div className="usage-list" aria-label="Dataset coverage">
            <CoverageBar label="Models evaluated" done={catalog.evaluated_count} total={catalog.model_count} />
            <CoverageBar label="Suites verified" done={catalog.verifier_passed_count} total={catalog.suite_count} />
            <CoverageBar label="Runs completed" done={completedRuns} total={evaluations.length} />
            <CoverageBar label="Assignments done" done={assignmentDone} total={assignmentTotal} />
          </div>
        ) : null}
      </div>

      <div className="task-card">
        <div className="panel-head">
          <h3>Current task</h3>
          {currentRun ? (
            <Link to={`/evaluations/${currentRun.dataset_id}/${currentRun.run_id}`} className="panel-link">
              View run →
            </Link>
          ) : null}
        </div>
        {currentRun && latestEvent ? (
          <>
            <p className="task-title">{currentSuite?.display_name ?? currentRun.suite_id}</p>
            <p className="task-sub">
              {shortRunId(currentRun.run_id)} · {currentRun.lifecycle_state}
            </p>
            <ProgressBar
              completed={campaignTerminalProgress(currentRun).done}
              total={campaignTerminalProgress(currentRun).total}
              label="Assignment progress"
            />
            <div className="task-chips">
              <code className="stream-chip">{currentRun.arm}</code>
              <code className="stream-chip">{currentRun.suite_id}</code>
              {currentRun.started_at ? (
                <span className="task-started">Started {formatRelativeTime(currentRun.started_at)}</span>
              ) : null}
            </div>
          </>
        ) : (
          <p className="panel-empty">No evaluation has been observed yet.</p>
        )}
      </div>
    </section>
  );
});

/** Most recent runs for the active dataset. */
function RecentRuns({ evaluations }: { evaluations: EvaluationSummary[] }) {
  const recent = evaluations.slice(-5).reverse();
  return (
    <section className="panel" aria-label="Recent runs">
      <div className="panel-head">
        <h2>Recent runs</h2>
        <Link to="/evaluations" className="panel-link">
          View all runs →
        </Link>
      </div>
      {recent.length === 0 ? (
        <p className="panel-empty">No runs recorded for this dataset.</p>
      ) : (
        <ul className="mini-runs">
          {recent.map((run) => (
            <li key={run.run_id}>
              <Link to={`/evaluations/${run.dataset_id}/${run.run_id}`} className="mini-run">
                <code className="mini-run-id">{shortRunId(run.run_id)}</code>
                <span className="mini-run-name">{run.suite_id}</span>
                <span className="mini-run-when">
                  {formatRelativeTime(run.started_at ?? run.observed_at)}
                </span>
                <span className={`status-dot status-${run.lifecycle_state}`} aria-label={run.lifecycle_state} />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** Anonymous public mirror endpoints as downloadable artifacts. */
function DownloadsPanel() {
  const [origin, setOrigin] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    loadRuntimeConfig()
      .then((config) => {
        if (!cancelled) setOrigin(config.mirror_origin);
      })
      .catch(() => {
        if (!cancelled) setOrigin(null);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const items = [
    { label: 'Feed bootstrap', format: 'JSON', href: origin ? `${origin}/bootstrap` : undefined },
    { label: 'Feed history', format: 'JSON pages', href: origin ? `${origin}/history` : undefined },
    { label: 'Proof catalog', format: 'JSON', href: origin ? `${origin}/proof-catalog` : undefined },
    { label: 'Live event stream', format: 'SSE', href: origin ? `${origin}/stream` : undefined },
    { label: 'View schema descriptor', format: 'JSON', href: descriptorUrl },
  ];

  return (
    <section className="panel" id="downloads" aria-label="Download data">
      <div className="panel-head">
        <div>
          <h2>Download data</h2>
          <p className="panel-note">Everything the mirror publishes is open.</p>
        </div>
      </div>
      <ul className="dl-list">
        {items.map((item) => (
          <li key={item.label} className="dl-item">
            {item.href ? (
              <a href={item.href} target="_blank" rel="noopener noreferrer">
                {item.label}
              </a>
            ) : (
              <span>{item.label}</span>
            )}
            <span className="dl-format">{item.format}</span>
          </li>
        ))}
      </ul>
      {origin ? (
        <a className="dl-browse" href={`${origin}/bootstrap`} target="_blank" rel="noopener noreferrer">
          Browse the public feed →
        </a>
      ) : (
        <p className="panel-empty">Mirror origin unavailable.</p>
      )}
    </section>
  );
}

export function OverviewView() {
  const gridRef = useRef<HTMLDivElement>(null);
  const sysPanelRef = useRef<HTMLElement>(null);
  const [params] = useSearchParams();
  const routeDataset = params.get('dataset') ?? undefined;
  const activeDatasetId = useActiveDatasetId(routeDataset);
  const catalog = useStoreState((state) => state.catalogs.get(activeDatasetId));
  const models = useStoreState((state) =>
    Array.from(state.models.values()).filter((m) => m.dataset_id === activeDatasetId),
  );
  const evaluations = useStoreState((state) =>
    Array.from(state.evaluations.values()).filter((e) => e.dataset_id === activeDatasetId),
  );
  const suites = useStoreState((state) =>
    Array.from(state.suites.values()).filter((s) => s.dataset_id === activeDatasetId),
  );
  const events = useStoreState((state) => state.events);
  const connection = useStoreState((state) => state.connection);

  useLayoutEffect(() => {
    const grid = gridRef.current;
    const sysPanel = sysPanelRef.current;
    if (!grid || !sysPanel) return;

    const syncStreamPanelHeight = () => {
      const stacked = window.matchMedia('(max-width: 1000px)').matches;
      if (stacked) {
        grid.style.removeProperty('--stream-panel-height');
        return;
      }
      grid.style.setProperty('--stream-panel-height', `${sysPanel.getBoundingClientRect().height}px`);
    };

    syncStreamPanelHeight();
    const resizeObserver = new ResizeObserver(syncStreamPanelHeight);
    resizeObserver.observe(sysPanel);
    window.addEventListener('resize', syncStreamPanelHeight);
    return () => {
      resizeObserver.disconnect();
      window.removeEventListener('resize', syncStreamPanelHeight);
    };
  }, []);

  return (
    <div className="overview">
      <div className="ov-grid-main" ref={gridRef}>
        <LiveStreamPanel events={events} connection={connection} />
        <SystemOverviewPanel
          ref={sysPanelRef}
          catalog={catalog}
          evaluations={evaluations}
          suites={suites}
          events={events}
          activeDatasetId={activeDatasetId}
          connection={connection}
        />
      </div>

      <div className="ov-grid-bottom">
        <RoleLeadersPanel models={models} datasetId={activeDatasetId} />
        <RecentRuns evaluations={evaluations} />
        <DownloadsPanel />
      </div>
    </div>
  );
}
