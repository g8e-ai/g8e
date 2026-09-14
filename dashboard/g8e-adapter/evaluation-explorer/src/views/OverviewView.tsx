// Overview view — the first viewport. Per the plan, the first screen
// contains feed state, a live evaluation panel, dataset selector, aggregate
// counts, top measured model per role, recent runs, and concise metric
// explanations. Architecture marketing moves below evaluation content.

import { useSearchParams } from 'react-router-dom';
import { Link } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { recordKey, useStoreState, useFeedStatus } from '../state/store';
import { DatasetSelector } from '../components/DatasetSelector';
import {
  EmptyState,
  ErrorState,
  ProgressBar,
  QualityBadge,
  StatTile,
  Timeline,
  formatPercent,
  formatNumber,
  formatDuration,
  formatTimestamp,
} from '../components/shared';
import type { ModelSummary } from '../contract/types';

function topModelPerRole(models: ModelSummary[]): Record<string, ModelSummary | undefined> {
  const byRole: Record<string, ModelSummary | undefined> = { primary: undefined, assistant: undefined, lite: undefined };
  for (const model of models) {
    if (model.inventory_only || !model.pass_rate) continue;
    const current = byRole[model.role];
    if (!current || !current.pass_rate || model.pass_rate.estimate > current.pass_rate.estimate) {
      byRole[model.role] = model;
    }
  }
  return byRole;
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
  const events = useStoreState((state) => state.events);
  const connection = useStoreState((state) => state.connection);

  // The live panel tracks the most recently active run, including its
  // terminal state — a finished run does not silently revert to "idle".
  const latestEvent = events.length > 0 ? events[events.length - 1] : undefined;
  const liveRunId = latestEvent?.run_id;
  const liveRunDataset = latestEvent?.dataset_id;
  const liveEvents = liveRunId
    ? events.filter((e) => e.run_id === liveRunId && e.dataset_id === liveRunDataset).slice(-8)
    : [];
  const liveRun = useStoreState((state) =>
    liveRunId && liveRunDataset ? state.evaluations.get(recordKey(liveRunDataset, liveRunId)) : undefined,
  );

  const recentEvaluations = evaluations.slice(-5).reverse();
  const evaluatedModels = models.filter((m) => !m.inventory_only && m.pass_rate);
  const topPerRole = topModelPerRole(evaluatedModels);

  return (
    <div className="overview">
      <section className="overview-hero">
        <div className="hero-copy">
          <p className="section-kicker">OPEN EVALUATION INTELLIGENCE</p>
          <h1>AI evaluations. <span>Working in public.</span></h1>
          <p className="hero-description">Explore measured model performance, inspect every run and watch public-safe evaluation events arrive through the live mirror.</p>
          <div className="hero-actions">
            <Link to="/evaluations" className="hero-primary">Explore evaluations</Link>
            <Link to="/models" className="hero-secondary">Compare models</Link>
          </div>
          <ul className="hero-facts" aria-label="Public evaluation properties">
            <li>Live SSE telemetry</li>
            <li>Real measured workloads</li>
            <li>Typed public records</li>
            <li>Private data excluded</li>
          </ul>
          <DatasetSelector activeId={activeDatasetId} />
        </div>
        <div className="hero-flow" role="img" aria-label="Committed evaluation reports publish outward through the public mirror to this browser">
          <p className="hero-flow-label">Outbound-only public visibility</p>
          <div className="flow-nodes">
            <div className="flow-node"><span>Source</span><strong>Eval reports</strong><small>Committed records</small></div>
            <div className="flow-arrow" aria-hidden="true">→</div>
            <div className="flow-node flow-node-accent"><span>Read model</span><strong>Public mirror</strong><small>Signed · read-only</small></div>
            <div className="flow-arrow" aria-hidden="true">→</div>
            <div className="flow-node"><span>Consumer</span><strong>This browser</strong><small>No credentials</small></div>
          </div>
          <p className="hero-flow-note">The mirror reports visibility. It does not authorize execution or replace Operator evidence.</p>
        </div>
      </section>

      {feedStatus ? (
        <section className="feed-status-bar" aria-label="Feed status">
          <span className="feed-message">{feedStatus.message}</span>
          {feedStatus.lastAcceptedAt ? (
            <span className="feed-last">Last accepted: {formatTimestamp(feedStatus.lastAcceptedAt)}</span>
          ) : null}
          <span className="feed-seq">High-water sequence: {formatNumber(feedStatus.highWaterSequence)}</span>
        </section>
      ) : null}

      <section className="evaluation-lenses" aria-labelledby="evaluation-lenses-title">
        <div className="section-heading-row">
          <div>
            <p className="section-kicker">BENCHMARK FOUNDATION</p>
            <h2 id="evaluation-lenses-title">Four questions, not one score</h2>
          </div>
          <Link to="/methodology" className="methodology-link">See the campaign design</Link>
        </div>
        <div className="lens-grid">
          <article><span>01 · Capability</span><h3>Can the model do the job?</h3><p>Task accuracy, instruction following, reasoning, and tool selection.</p></article>
          <article><span>02 · Protocol</span><h3>Can it behave inside the protocol?</h3><p>Schemas, routing, handoffs, retries, escalation, and recovery.</p></article>
          <article><span>03 · Governance</span><h3>Can the platform govern it safely?</h3><p>Policy enforcement, exposure, privilege boundaries, and audit completeness.</p></article>
          <article><span>04 · Local value</span><h3>Is it worth running locally?</h3><p>Cold start, latency, throughput, memory, power, tokens, and compute efficiency.</p></article>
        </div>
      </section>

      <div className="overview-dashboard-grid">
      <section className="live-panel" aria-label="Live evaluation">
        <h2>Live evaluation</h2>
        {liveEvents.length === 0 || !latestEvent || !liveRunId || !liveRunDataset ? (
          <p className="live-idle">
            No evaluation has been observed yet. Historical and scripted runs are available below.
          </p>
        ) : (
          <div className="live-current">
            <div className="live-meta">
              <Link to={`/evaluations/${liveRunDataset}/${liveRunId}`} className="live-run">
                {liveRunId}
              </Link>
              <span className={`lifecycle-state lifecycle-${latestEvent.lifecycle_status}`}>
                {latestEvent.lifecycle_status}
              </span>
              {liveRun ? <span className="live-suite">Suite: {liveRun.suite_id}</span> : null}
              <ProgressBar
                completed={latestEvent.completed}
                total={latestEvent.total}
                label="Assignment progress"
              />
            </div>
            {liveRun ? (
              <ul className="role-mapping live-roles">
                {Object.entries(liveRun.model_role_mapping).map(([role, variantId]) => (
                  <li key={role}>
                    <span className="role-label">{role === 'lite' ? 'Light' : role}</span>
                    <Link to={`/models/${liveRunDataset}/${variantId}`}>{variantId}</Link>
                  </li>
                ))}
              </ul>
            ) : null}
            <Timeline events={liveEvents} />
          </div>
        )}
      </section>

      {catalog ? (
        <section className="overview-stats" aria-label="Dataset summary">
          <h2>{catalog.title}</h2>
          <QualityBadge state={catalog.quality_state} />
          <p className="catalog-description">{catalog.description}</p>
          {catalog.limitations.length > 0 ? (
            <ul className="catalog-limitations">
              {catalog.limitations.map((lim, i) => (
                <li key={i}>{lim}</li>
              ))}
            </ul>
          ) : null}
          <div className="stat-grid">
            <StatTile label="Models" value={formatNumber(catalog.model_count)} hint={`${catalog.evaluated_count} evaluated`} />
            <StatTile label="Suites" value={formatNumber(catalog.suite_count)} hint={`${catalog.verifier_passed_count} passed, ${catalog.verifier_failed_count} failed`} />
            <StatTile label="Assignments" value={formatNumber(catalog.assignment_count)} />
            <StatTile label="Provider calls" value={formatNumber(catalog.provider_request_count)} />
            <StatTile label="Tokens" value={formatNumber(catalog.provider_token_count)} hint="reported total" />
            <StatTile label="Retries" value={formatNumber(catalog.retry_count)} />
          </div>
        </section>
      ) : connection === 'offline' ? (
        <EmptyState hasRecords={false} hasFilters={false} connection="offline" />
      ) : (
        <ErrorState message="No catalog loaded for the selected dataset." />
      )}
      </div>

      <div className="overview-secondary-grid">
      <section className="top-models" aria-label="Top measured model per role">
        <h2>Top measured model per role</h2>
        <p className="disclosure">
          Exploratory pass-rate leaders by role. This is not a superiority claim; intervals reflect sampling uncertainty.
        </p>
        <div className="role-grid">
          {(['primary', 'assistant', 'lite'] as const).map((role) => {
            const model = topPerRole[role];
            return (
              <div key={role} className="role-card">
                <h3>{role === 'lite' ? 'Light' : role}</h3>
                {model ? (
                  <Link to={`/models/${activeDatasetId}/${model.variant_id}`} className="role-model-link">
                    <span className="role-model-name">{model.display_name}</span>
                    {model.pass_rate ? (
                      <span className="role-model-rate">
                        {formatPercent(model.pass_rate.estimate)}
                        <span className="role-model-interval">
                          {' '}[{formatPercent(model.pass_rate.lower)}, {formatPercent(model.pass_rate.upper)}]
                        </span>
                      </span>
                    ) : null}
                    <QualityBadge state={model.quality_state} />
                  </Link>
                ) : (
                  <span className="role-no-model">No measured model for this role.</span>
                )}
              </div>
            );
          })}
        </div>
      </section>

      <section className="recent-runs" aria-label="Recent evaluation runs">
        <h2>Recent evaluation runs</h2>
        {recentEvaluations.length === 0 ? (
          <EmptyState hasRecords={false} hasFilters={false} connection={connection} />
        ) : (
          <ul className="run-list">
            {recentEvaluations.map((run) => (
              <li key={run.run_id}>
                <Link to={`/evaluations/${run.dataset_id}/${run.run_id}`} className="run-link">
                  <span className="run-id">{run.run_id}</span>
                  <span className="run-suite">{run.suite_id}</span>
                  <span className="run-status">{run.lifecycle_state}</span>
                  {run.elapsed_seconds ? <span className="run-elapsed">{formatDuration(run.elapsed_seconds)}</span> : null}
                  <QualityBadge state={run.quality_state} />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
      </div>

      <section className="metric-explainer" aria-label="Metric explanations">
        <h2>What these metrics mean</h2>
        <dl className="metric-explainer-list">
          <div>
            <dt>Pass rate</dt>
            <dd>The fraction of eligible assignments that passed. The interval is a bootstrap confidence bound, not a superiority claim.</dd>
          </div>
          <div>
            <dt>Agreement</dt>
            <dd>How often repetitions of the same task agree. Higher means more consistent results.</dd>
          </div>
          <div>
            <dt>Repeatability</dt>
            <dd>How consistently a model produces the same outcome across repetitions: consistently correct, consistently wrong, inconsistent, or insufficient.</dd>
          </div>
        </dl>
        <Link to="/methodology" className="methodology-link">Read the full methodology</Link>
      </section>
    </div>
  );
}
