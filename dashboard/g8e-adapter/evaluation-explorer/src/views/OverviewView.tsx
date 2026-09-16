// Overview view — the landing page. Layout mirrors the OpenDevOps.ai
// surface: evaluated role agents, the live event stream, a system overview
// with dataset coverage, the measured-model table, recent runs, and the
// public mirror download endpoints. Every panel renders real store data;
// nothing on this page is decorative.

import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { modelComparisonId, recordKey, resolveModelSummary, useStoreState, useFeedStatus } from '../state/store';
import { loadRuntimeConfig } from '../state/feed';
import { DatasetSelector } from '../components/DatasetSelector';
import {
  EmptyState,
  ProgressBar,
  formatNumber,
  formatPercent,
  formatTimestamp,
  formatLatency,
  formatThroughput,
} from '../components/shared';
import { formatCompact, formatRelativeTime } from '../utils/format';
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

/** Evaluated models as role agents: name, role duty, status, throughput. */
function AgentsStrip({ models, datasetId }: { models: ModelSummary[]; datasetId: string }) {
  const agents = useMemo(
    () =>
      models
        .filter((m) => !m.inventory_only && m.pass_rate)
        .sort(
          (a, b) =>
            (b.output_throughput_p50?.value ?? 0) - (a.output_throughput_p50?.value ?? 0),
        )
        .slice(0, 8),
    [models],
  );

  return (
    <section className="panel" aria-label="Models">
      <div className="panel-head">
        <h2>
          Models <span className="panel-sub">· {agents.length} evaluated</span>
        </h2>
        <Link to={`/models?dataset=${datasetId}`} className="panel-link">
          View all models →
        </Link>
      </div>
      {agents.length === 0 ? (
        <p className="panel-empty">No evaluated models in the active dataset.</p>
      ) : (
        <ul className="agents-grid">
          {agents.map((model) => {
            const status = agentStatus(model.quality_state);
            const tps = model.output_throughput_p50?.value;
            return (
              <li key={modelComparisonId(model)} className="agent-card">
                <div className="agent-icon" aria-hidden="true">
                  {model.display_name.slice(0, 1)}
                </div>
                <div className="agent-body">
                  <Link
                    to={`/models/${datasetId}/${model.variant_id}?role=${model.role}`}
                    className="agent-name"
                  >
                    {model.display_name}
                  </Link>
                  <span className="agent-role">{ROLE_LABELS[model.role]}</span>
                  <span className={`agent-status status-${status.tone}`}>
                    <span className="status-dot" aria-hidden="true" />
                    {status.label}
                    {tps !== undefined ? ` · ${formatCompact(tps)} t/s` : ''}
                  </span>
                </div>
              </li>
            );
          })}
        </ul>
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
  const [agentFilter, setAgentFilter] = useState('all');
  const [kindFilter, setKindFilter] = useState('all');

  const models = useStoreState((state) => state.models);

  const agentOptions = useMemo(
    () =>
      Array.from(new Set(events.map((e) => e.variant_id).filter((v): v is string => Boolean(v)))).sort(),
    [events],
  );
  const kindOptions = useMemo(() => Array.from(new Set(events.map((e) => e.kind))).sort(), [events]);

  const visible = events
    .filter((e) => agentFilter === 'all' || e.variant_id === agentFilter)
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
            aria-label="Filter by agent"
            value={agentFilter}
            onChange={(e) => setAgentFilter(e.target.value)}
          >
            <option value="all">All agents</option>
            {agentOptions.map((id) => (
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
      {visible.length === 0 ? (
        <EmptyState hasRecords={events.length > 0} hasFilters={agentFilter !== 'all' || kindFilter !== 'all'} connection={connection} />
      ) : (
        <div className="table-scroll">
          <table className="stream-table">
            <thead>
              <tr>
                <th>Time</th>
                <th>Agent</th>
                <th>Event</th>
                <th>Model / Tool</th>
                <th>Progress</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((event) => {
                const model = event.variant_id
                  ? resolveModelSummary(models, event.dataset_id, event.variant_id)
                  : undefined;
                const tool = model?.served_model_tag ?? event.variant_id ?? event.kind.split('_')[0];
                return (
                  <tr key={event.event_id}>
                    <td className="stream-time">{eventTime(event.observed_at)}</td>
                    <td>
                      <span className={`stream-agent status-${event.lifecycle_status}`}>
                        <span className="status-dot" aria-hidden="true" />
                        {event.variant_id ?? 'platform'}
                      </span>
                    </td>
                    <td className="stream-event">
                      {event.kind.replace(/_/g, ' ')}
                      {event.stage_label ? ` · ${event.stage_label}` : ''}
                    </td>
                    <td>
                      <code className="stream-chip">{tool}</code>
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
    </section>
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

/** System overview: headline counts, dataset coverage bars, current run. */
function SystemOverviewPanel({
  catalog,
  models,
  evaluations,
  suites,
  events,
  activeDatasetId,
  connection,
}: {
  catalog: CatalogSnapshot | undefined;
  models: ModelSummary[];
  evaluations: EvaluationSummary[];
  suites: SuiteSummary[];
  events: LiveEvent[];
  activeDatasetId: string;
  connection: FeedConnectionState;
}) {
  const evaluatedCount = models.filter((m) => m.pass_rate).length;
  const totalThroughput = models.reduce(
    (sum, m) => sum + (m.output_throughput_p50?.value ?? 0),
    0,
  );
  const completedRuns = evaluations.filter((e) => e.lifecycle_state === 'completed').length;
  const assignmentDone = evaluations.reduce(
    (sum, e) => sum + e.assignment_completed + e.assignment_failed,
    0,
  );
  const assignmentTotal = evaluations.reduce((sum, e) => sum + e.assignment_total, 0);
  const verifierTotal = catalog
    ? catalog.verifier_passed_count + catalog.verifier_failed_count
    : 0;

  const latestEvent = events.length > 0 ? events[events.length - 1] : undefined;
  const currentRun = useStoreState((state) =>
    latestEvent ? state.evaluations.get(recordKey(latestEvent.dataset_id, latestEvent.run_id)) : undefined,
  );
  const currentSuite = currentRun
    ? suites.find((s) => s.suite_id === currentRun.suite_id)
    : undefined;

  return (
    <section className="panel sys-panel" aria-label="System overview">
      <div className="panel-head">
        <h2>System overview</h2>
        <span className={`stream-state ${connection === 'live' ? 'status-ok' : 'status-warn'}`}>
          <span className="status-dot" aria-hidden="true" />
          {connection === 'live' ? 'Live' : 'Offline'}
        </span>
      </div>

      <div className="sys-stats">
        <div>
          <strong>{formatNumber(catalog?.evaluated_count ?? evaluatedCount)}</strong>
          <span>Models evaluated</span>
        </div>
        <div>
          <strong>{formatNumber(catalog?.run_count ?? evaluations.length)}</strong>
          <span>Runs recorded</span>
        </div>
        <div>
          <strong>
            {verifierTotal > 0 && catalog
              ? formatPercent(catalog.verifier_passed_count / verifierTotal)
              : '—'}
          </strong>
          <span>Verifier pass</span>
        </div>
        <div>
          <strong>{totalThroughput > 0 ? `${formatCompact(totalThroughput)} t/s` : '—'}</strong>
          <span>Throughput p50</span>
        </div>
      </div>

      <DatasetSelector activeId={activeDatasetId} />
      {catalog ? <p className="panel-note">{catalog.title}</p> : null}

      {catalog ? (
        <div className="usage-list" aria-label="Dataset coverage">
          <h3>Dataset coverage</h3>
          <CoverageBar label="Models evaluated" done={catalog.evaluated_count} total={catalog.model_count} />
          <CoverageBar label="Suites verified" done={catalog.verifier_passed_count} total={catalog.suite_count} />
          <CoverageBar label="Runs completed" done={completedRuns} total={evaluations.length} />
          <CoverageBar label="Assignments done" done={assignmentDone} total={assignmentTotal} />
        </div>
      ) : null}

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
}

/** Measured-model comparison table for the active dataset. */
function ModelLab({ models, datasetId }: { models: ModelSummary[]; datasetId: string }) {
  const measured = useMemo(
    () =>
      models
        .filter((m) => m.pass_rate)
        .sort((a, b) => (b.pass_rate?.estimate ?? 0) - (a.pass_rate?.estimate ?? 0))
        .slice(0, 6),
    [models],
  );

  return (
    <section className="panel" aria-label="Model evaluation lab">
      <div className="panel-head">
        <div>
          <h2>Model evaluation lab</h2>
          <p className="panel-note">Measured models in the active dataset</p>
        </div>
        <Link to={`/models?dataset=${datasetId}`} className="panel-link">
          View all results →
        </Link>
      </div>
      {measured.length === 0 ? (
        <p className="panel-empty">No measured models in this dataset.</p>
      ) : (
        <div className="table-scroll">
          <table className="lab-table">
            <thead>
              <tr>
                <th>Model</th>
                <th>Role</th>
                <th>Quant</th>
                <th>Tokens/s</th>
                <th>Agreement</th>
                <th>Pass rate</th>
                <th>Latency p50</th>
              </tr>
            </thead>
            <tbody>
              {measured.map((model) => (
                <tr key={modelComparisonId(model)}>
                  <td>
                    <Link to={`/models/${datasetId}/${model.variant_id}?role=${model.role}`}>
                      {model.display_name}
                    </Link>
                  </td>
                  <td>{roleLabel(model.role)}</td>
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
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

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
  const [params] = useSearchParams();
  const routeDataset = params.get('dataset') ?? undefined;
  const activeDatasetId = useActiveDatasetId(routeDataset);
  const feedStatus = useFeedStatus();

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

  return (
    <div className="overview">
      {feedStatus ? (
        <section className="feed-status-bar" aria-label="Feed status">
          <span className="feed-message">{feedStatus.message}</span>
          {feedStatus.lastAcceptedAt ? (
            <span className="feed-last">Last accepted: {formatTimestamp(feedStatus.lastAcceptedAt)}</span>
          ) : null}
          <span className="feed-seq">High-water sequence: {formatNumber(feedStatus.highWaterSequence)}</span>
        </section>
      ) : null}

      <AgentsStrip models={models} datasetId={activeDatasetId} />

      <div className="ov-grid-main">
        <LiveStreamPanel events={events} connection={connection} />
        <SystemOverviewPanel
          catalog={catalog}
          models={models}
          evaluations={evaluations}
          suites={suites}
          events={events}
          activeDatasetId={activeDatasetId}
          connection={connection}
        />
      </div>

      <div className="ov-grid-bottom">
        <ModelLab models={models} datasetId={activeDatasetId} />
        <RecentRuns evaluations={evaluations} />
        <DownloadsPanel />
      </div>
    </div>
  );
}
