// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Overview view — the landing page. Layout mirrors the OpenDevOps.ai
// surface: the live event stream, a system overview
// with dataset coverage, recent campaigns, and the
// public mirror download endpoints. Every panel renders real store data;
// nothing on this page is decorative.

import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  PLATFORM_CONTACT_EMAIL,
  PLATFORM_LEDE,
  PLATFORM_OVERVIEW_PORTFOLIO_NOTE,
} from '../content/platform';
import { useDatasetOptions } from '../state/dataset';
import { useStoreState } from '../state/store';
import { loadRuntimeConfig } from '../state/feed';
import { LiveEventStream } from '../components/LiveEventStream';
import {
  ReconcilePlaceholder,
  StreamStatusIndicator,
  formatNumber,
} from '../components/shared';
import { formatRelativeTime } from '../utils/format';
import {
  type FeedConnectionState,
  type StreamConnectionState,
} from '../utils/feed-state';
import { recentCampaignRows, roleLabel } from './derived';
import descriptorUrl from '../contract/descriptor.json?url';
import type {
  CatalogSnapshot,
  EvaluationSummary,
  LiveEvent,
  SuiteSummary,
} from '../contract/types';

function shortCampaignLabel(label: string): string {
  return label.length > 22 ? `…${label.slice(-20)}` : label;
}

function capitalize(value: string): string {
  return value.length > 0 ? `${value.charAt(0).toUpperCase()}${value.slice(1)}` : value;
}

function campaignStatusLabel(run: EvaluationSummary | undefined): string {
  if (!run) return 'Awaiting data';
  if (run.lifecycle_state === 'completed') return 'Complete';
  return capitalize(run.lifecycle_state);
}

function verificationLabel(run: EvaluationSummary | undefined, catalog: CatalogSnapshot): string {
  if (run?.verifier_state === 'failed' || catalog.verifier_failed_count > 0) return 'Verification failed';
  if (run?.verifier_state === 'passed') return 'Verification passed';
  if (run?.lifecycle_state === 'running' || run?.lifecycle_state === 'queued') return 'Verification pending';
  return 'Verification not applicable';
}

function lastMatchingEvent(events: LiveEvent[], predicate: (event: LiveEvent) => boolean): LiveEvent | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event && predicate(event)) return event;
  }
  return undefined;
}

function latestAssignmentActivity(events: LiveEvent[]) {
  const latest = lastMatchingEvent(events, (event) => event.assignment_id !== undefined);
  if (!latest?.assignment_id) return undefined;
  const matching = events.filter((event) => event.assignment_id === latest.assignment_id);
  return {
    assignmentId: latest.assignment_id,
    taskId: lastMatchingEvent(matching, (event) => event.task_id !== undefined)?.task_id,
    variantId: lastMatchingEvent(matching, (event) => event.variant_id !== undefined)?.variant_id,
    role: lastMatchingEvent(matching, (event) => event.role !== undefined)?.role,
    stageLabel: lastMatchingEvent(matching, (event) => event.stage_label !== undefined)?.stage_label,
  };
}

type SystemOverviewPanelProps = {
  catalog: CatalogSnapshot | undefined;
  evaluations: EvaluationSummary[];
  suites: SuiteSummary[];
  events: LiveEvent[];
  connection: FeedConnectionState;
  streamConnection: StreamConnectionState;
  isReconciling: boolean;
};

/** System overview: platform context, active campaign progress, and current run. */
function SystemOverviewPanel({
  catalog,
  evaluations,
  suites,
  events,
  connection,
  streamConnection,
  isReconciling,
}: SystemOverviewPanelProps) {
  const assignmentDone = evaluations.reduce(
    (sum, evaluation) => sum + evaluation.assignment_completed + evaluation.assignment_failed,
    0,
  );
  const assignmentFailed = evaluations.reduce((sum, evaluation) => sum + evaluation.assignment_failed, 0);
  const assignmentTotal = evaluations.reduce((sum, evaluation) => sum + evaluation.assignment_total, 0);
  const assignmentPercent = assignmentTotal > 0 ? Math.min(100, Math.round((assignmentDone / assignmentTotal) * 100)) : 0;
  const latestEvent = events[events.length - 1];
  const eventRun = latestEvent
    ? evaluations.find((evaluation) => evaluation.run_id === latestEvent.run_id)
    : undefined;
  const orderedEvaluations = [...evaluations].sort((a, b) => b.observed_at.localeCompare(a.observed_at));
  const currentRun = eventRun
    ?? orderedEvaluations.find((evaluation) => evaluation.lifecycle_state === 'running' || evaluation.lifecycle_state === 'queued')
    ?? orderedEvaluations[0];
  const currentSuite = currentRun ? suites.find((suite) => suite.suite_id === currentRun.suite_id) : undefined;
  const currentRunEvents = currentRun ? events.filter((event) => event.run_id === currentRun.run_id) : [];
  const activity = latestAssignmentActivity(currentRunEvents);
  const verification = catalog ? verificationLabel(currentRun, catalog) : undefined;

  return (
    <section className="panel sys-panel" aria-label="System overview">
      <div className="panel-head">
        <h2>System overview</h2>
        <StreamStatusIndicator
          streamConnection={streamConnection}
          feedConnection={connection}
        />
      </div>

      <div className="sys-platform">
        <h3 className="sys-platform-heading">Need help with AI governance and security?</h3>
        <p className="sys-platform-lede">{PLATFORM_LEDE}</p>
        <div className="sys-platform-cta">
          <p className="sys-platform-cta-note">{PLATFORM_OVERVIEW_PORTFOLIO_NOTE}</p>
          <div className="sys-platform-links">
            <Link className="sys-platform-cta-architecture" to="/methodology#architecture">
              Architecture Overview
            </Link>
            <span className="sys-platform-email-label">Email:</span>
            <a className="sys-platform-email" href={`mailto:${PLATFORM_CONTACT_EMAIL}`}>
              {PLATFORM_CONTACT_EMAIL}
            </a>
          </div>
        </div>
      </div>

      <div className="sys-campaign">
        <div className="campaign-section-head">
          <h3 className="sys-section-title">Active campaign</h3>
          {currentRun?.lifecycle_state === 'running' ? (
            <StreamStatusIndicator
              streamConnection={streamConnection}
              feedConnection={connection}
              className="campaign-status"
            />
          ) : (
            <span className={`campaign-status status-${currentRun?.lifecycle_state ?? 'neutral'}`}>
              <span className="status-dot" aria-hidden="true" />
              {campaignStatusLabel(currentRun)}
            </span>
          )}
        </div>

        {catalog ? (
          <>
            <div className="campaign-identity">
              <div>
                <h4>{catalog.title}</h4>
                <p>{catalog.description}</p>
              </div>
            </div>

            {currentRun ? (
              <div className="campaign-run-context">
                <span>{currentSuite?.display_name ?? currentRun.suite_id}</span>
                <span aria-hidden="true">·</span>
                <code>{currentRun.arm}</code>
              </div>
            ) : null}

            <div className="campaign-progress">
              <div className="campaign-progress-head">
                <strong>{formatNumber(assignmentDone)} of {formatNumber(assignmentTotal)} assignments complete</strong>
                <strong>{assignmentPercent}%</strong>
              </div>
              <div
                className="campaign-progress-track"
                role="progressbar"
                aria-label="Campaign assignment progress"
                aria-valuenow={assignmentDone}
                aria-valuemin={0}
                aria-valuemax={assignmentTotal || 1}
                aria-valuetext={`${assignmentDone} of ${assignmentTotal} assignments complete`}
              >
                <div className="campaign-progress-fill" style={{ width: `${assignmentPercent}%` }} />
              </div>
              <div className="campaign-progress-meta">
                <span>{currentRun?.started_at ? `Started ${formatRelativeTime(currentRun.started_at)}` : 'Start time unavailable'}</span>
                <span className={assignmentFailed > 0 ? 'campaign-failures' : undefined}>{formatNumber(assignmentFailed)} failed</span>
              </div>
            </div>

            <ul className="campaign-scope" aria-label="Campaign scope and verification">
              <li><strong>{formatNumber(catalog.model_count)}</strong> {catalog.model_count === 1 ? 'model' : 'models'}</li>
              <li><strong>{formatNumber(catalog.suite_count)}</strong> {catalog.suite_count === 1 ? 'suite' : 'suites'}</li>
              <li className={verification === 'Verification failed' ? 'campaign-verification-failed' : undefined}>
                {verification}
              </li>
            </ul>

            {currentRun && activity ? (
              <div className="campaign-activity" aria-live="polite">
                <h4>{currentRun.lifecycle_state === 'running' ? 'Now evaluating' : 'Latest activity'}</h4>
                <div className="campaign-activity-primary">
                  {activity.variantId ? (
                    <Link to={`/models/${currentRun.dataset_id}/${activity.variantId}`}>{activity.variantId}</Link>
                  ) : (
                    <span>Model unavailable</span>
                  )}
                  <span aria-hidden="true">·</span>
                  <span>{activity.taskId ?? activity.assignmentId}</span>
                </div>
                <div className="campaign-activity-meta">
                  {activity.stageLabel ? <span>{capitalize(activity.stageLabel.replaceAll('_', ' '))}</span> : null}
                  {activity.role ? <span>{roleLabel(activity.role)} role</span> : null}
                </div>
              </div>
            ) : null}

            <div className="campaign-actions">
              {currentRun ? (
                <Link to={`/evaluations/${currentRun.dataset_id}/${currentRun.run_id}`} className="campaign-primary-action">
                  View Details
                </Link>
              ) : null}
              <Link to="/methodology" className="campaign-secondary-action">Methodology</Link>
            </div>
          </>
        ) : isReconciling ? (
          <ReconcilePlaceholder label="Loading feed history…" />
        ) : (
          <p className="panel-empty">No campaign has been observed yet.</p>
        )}
      </div>
    </section>
  );
}

/** Most recent campaigns across every dataset in the feed. */
function RecentCampaigns({
  catalogs,
  evaluations,
}: {
  catalogs: CatalogSnapshot[];
  evaluations: EvaluationSummary[];
}) {
  const recent = useMemo(
    () => recentCampaignRows(catalogs, evaluations),
    [catalogs, evaluations],
  );
  return (
    <section className="panel campaign-evidence-panel" aria-label="Campaign evidence">
      <div className="panel-head campaign-evidence-head">
        <div>
          <h2>Campaign evidence</h2>
          <p className="panel-note">Terminal assignment coverage for the latest public datasets.</p>
        </div>
        <Link to="/evaluations" className="panel-link">
          Explore all →
        </Link>
      </div>
      <ul className="campaign-legend" aria-label="Assignment outcome legend">
        <li><span className="campaign-legend-swatch campaign-legend-completed" />Completed</li>
        <li><span className="campaign-legend-swatch campaign-legend-failed" />Failed</li>
        <li><span className="campaign-legend-swatch campaign-legend-remaining" />Remaining</li>
      </ul>
      {recent.length === 0 ? (
        <p className="panel-empty">No campaigns recorded yet.</p>
      ) : (
        <ul className="campaign-evidence-list">
          {recent.map((campaign) => {
            const status = campaign.lifecycleState ?? campaign.qualityState;
            const href = campaign.runId
              ? `/evaluations/${campaign.datasetId}/${campaign.runId}`
              : `/?dataset=${campaign.datasetId}`;
            const terminal = campaign.assignmentCompleted + campaign.assignmentFailed;
            const total = Math.max(campaign.assignmentTotal, terminal);
            const completedWidth = total > 0 ? (campaign.assignmentCompleted / total) * 100 : 0;
            const failedWidth = total > 0 ? (campaign.assignmentFailed / total) * 100 : 0;
            const verification = campaign.verifierState === 'passed'
              ? 'Verification passed'
              : campaign.verifierState === 'failed'
                ? 'Verification failed'
                : campaign.lifecycleState === 'running' || campaign.lifecycleState === 'queued'
                  ? 'Verification pending'
                  : campaign.verifierState === 'not_applicable'
                    ? 'Verification not applicable'
                    : 'Verification not run';
            return (
              <li key={campaign.datasetId} className="campaign-evidence-row">
                <div className="campaign-evidence-title-row">
                  <Link to={href} className="campaign-evidence-title" title={campaign.label}>
                    {shortCampaignLabel(campaign.label)}
                  </Link>
                  <span className="campaign-evidence-time">{formatRelativeTime(campaign.observedAt)}</span>
                </div>
                <div className="campaign-evidence-meta">
                  <span>{campaign.detail}</span>
                  <span className={`campaign-evidence-state status-${status}`}>
                    <span className="status-dot" aria-hidden="true" />
                    {campaign.lifecycleState ? capitalize(campaign.lifecycleState) : capitalize(campaign.qualityState.replaceAll('_', ' '))}
                  </span>
                </div>
                {campaign.runId && total > 0 ? (
                  <>
                    <div
                      className="campaign-outcome-track"
                      role="progressbar"
                      aria-label={`${campaign.label} assignment outcomes`}
                      aria-valuemin={0}
                      aria-valuemax={total}
                      aria-valuenow={terminal}
                      aria-valuetext={`${campaign.assignmentCompleted} completed, ${campaign.assignmentFailed} failed, ${total - terminal} remaining`}
                    >
                      <span className="campaign-outcome-completed" style={{ width: `${completedWidth}%` }} />
                      <span className="campaign-outcome-failed" style={{ width: `${failedWidth}%` }} />
                    </div>
                    <div className="campaign-evidence-foot">
                      <span>{formatNumber(terminal)} / {formatNumber(total)} terminal</span>
                      <span className={campaign.verifierState === 'failed' ? 'campaign-verification-failed' : undefined}>{verification}</span>
                    </div>
                  </>
                ) : (
                  <div className="campaign-evidence-foot campaign-evidence-foot-unavailable">
                    <span>Run-level outcomes unavailable</span>
                    <span>{verification}</span>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

/** Public-safe evaluation datasets, contract, and raw mirror endpoints. */
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

  const resources = [
    {
      label: 'Campaign summaries',
      eyebrow: 'Analyze runs',
      description: 'Run identity, lifecycle, terminal outcomes, headline metrics, and verifier disposition.',
      format: 'JSON · paginated',
      href: origin ? `${origin}/history?kind=evaluation_summary&cursor=0&limit=500` : undefined,
    },
    {
      label: 'Assignment results',
      eyebrow: 'Analyze tasks',
      description: 'Scenario outcomes, closed grades, grouped activity, bounded resources, and evidence bindings.',
      format: 'JSON · paginated',
      href: origin ? `${origin}/history?kind=assignment_result&cursor=0&limit=500` : undefined,
    },
    {
      label: 'View schema',
      eyebrow: 'Integrate',
      description: 'Versioned record kinds, enums, public field allowlist, and prohibited private fields.',
      format: 'JSON · v1.5',
      href: descriptorUrl,
    },
  ];

  return (
    <section className="panel public-data-panel" id="downloads" aria-label="Public data and APIs">
      <div className="panel-head">
        <div>
          <h2>Public data &amp; APIs</h2>
          <p className="panel-note">Use the same anonymous, public-safe projection that powers this page.</p>
        </div>
      </div>
      <p className="public-boundary-note">
        Includes aggregate results, closed grade metadata, bounded resource metrics, verification state, and approved content bindings. Prompts, outputs, identities, paths, and receipt bodies stay private.
      </p>
      <ul className="public-resource-grid">
        {resources.map((resource) => (
          <li key={resource.label}>
            {resource.href ? (
              <a className="public-resource-card" href={resource.href} target="_blank" rel="noopener noreferrer">
                <span className="public-resource-eyebrow">{resource.eyebrow}</span>
                <strong>{resource.label}</strong>
                <span className="public-resource-description">{resource.description}</span>
                <span className="public-resource-format">{resource.format}<span aria-hidden="true"> ↗</span></span>
              </a>
            ) : (
              <div className="public-resource-card public-resource-disabled">
                <span className="public-resource-eyebrow">{resource.eyebrow}</span>
                <strong>{resource.label}</strong>
                <span className="public-resource-description">Mirror origin unavailable.</span>
                <span className="public-resource-format">{resource.format}</span>
              </div>
            )}
          </li>
        ))}
      </ul>
      <div className="public-api-strip">
        <span className="public-api-label">Raw mirror endpoints</span>
        {origin ? (
          <>
            <a href={`${origin}/history?cursor=0&limit=500`} target="_blank" rel="noopener noreferrer">History <code>JSON</code></a>
            <a href={`${origin}/bootstrap`} target="_blank" rel="noopener noreferrer">Bootstrap <code>JSON</code></a>
            <a href={`${origin}/proof-catalog`} target="_blank" rel="noopener noreferrer">Proofs <code>JSON</code></a>
            <a href={`${origin}/stream`} target="_blank" rel="noopener noreferrer">Live updates <code>SSE</code></a>
          </>
        ) : (
          <span className="public-api-unavailable">Mirror origin unavailable</span>
        )}
      </div>
    </section>
  );
}

export function OverviewView() {
  const activeDatasetId = useDatasetOptions().find((option) => option.available && option.kind === 'live_run')?.id ?? '';
  const catalog = useStoreState((state) => state.catalogs.get(activeDatasetId));
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()));
  const evaluations = useStoreState((state) =>
    Array.from(state.evaluations.values()).filter((e) => e.dataset_id === activeDatasetId),
  );
  const allEvaluations = useStoreState((state) => Array.from(state.evaluations.values()));
  const suites = useStoreState((state) =>
    Array.from(state.suites.values()).filter((s) => s.dataset_id === activeDatasetId),
  );
  const events = useStoreState((state) =>
    state.events.filter((event) => event.dataset_id === activeDatasetId),
  );
  const connection = useStoreState((state) => state.connection);
  const streamConnection = useStoreState((state) => state.streamConnection);
  const isReconciling = useStoreState((state) => state.pendingSnapshot !== null);

  return (
    <div className="overview">
      <div className="ov-grid-main">
        <LiveEventStream
          events={events}
          connection={connection}
          streamConnection={streamConnection}
          isReconciling={isReconciling}
        />
        <SystemOverviewPanel
          catalog={catalog}
          evaluations={evaluations}
          suites={suites}
          events={events}
          connection={connection}
          streamConnection={streamConnection}
          isReconciling={isReconciling}
        />
      </div>

      <div className="ov-grid-bottom">
        <RecentCampaigns catalogs={catalogs} evaluations={allEvaluations} />
        <DownloadsPanel />
      </div>
    </div>
  );
}
