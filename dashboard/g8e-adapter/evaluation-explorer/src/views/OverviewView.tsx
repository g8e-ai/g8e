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
  G8E_REPO_URL,
  PLATFORM_CONTACT_CALENDLY,
  PLATFORM_CONTACT_EMAIL,
  PLATFORM_FLOW_STEPS,
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
  if (run.lifecycle_state === 'running') return 'Live';
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

function PlatformFlowStep({ step }: { step: (typeof PLATFORM_FLOW_STEPS)[number] }) {
  return (
    <li className={step.id === 'g8e' ? 'sys-platform-step-accent' : undefined}>
      <div className="sys-platform-flow-content">
        <strong>{step.label}</strong>
        <span>{step.detail}</span>
      </div>
    </li>
  );
}

function PlatformFlowArrow({ direction }: { direction: 'h' | 'hl' | 'v' }) {
  const arrow = direction === 'h' ? '→' : direction === 'hl' ? '←' : '↓';

  return (
    <li
      className={`sys-platform-flow-arrow sys-platform-flow-arrow-${direction}`}
      aria-hidden="true"
    >
      {arrow}
    </li>
  );
}

function PlatformFlow() {
  const [home, g8e, mirror, page] = PLATFORM_FLOW_STEPS;

  return (
    <div className="sys-platform-flow-wrap">
      <p className="sys-platform-flow-heading" aria-hidden="true">Outbound only</p>
      <ol className="sys-platform-flow sys-platform-flow-grid" aria-label="Platform data flow, outbound only">
        <PlatformFlowStep step={home} />
        <PlatformFlowArrow direction="h" />
        <PlatformFlowStep step={g8e} />
        <PlatformFlowArrow direction="v" />
        <PlatformFlowStep step={page} />
        <PlatformFlowArrow direction="hl" />
        <PlatformFlowStep step={mirror} />
      </ol>
    </div>
  );
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
        <p className="sys-platform-lede">{PLATFORM_LEDE}</p>
        <PlatformFlow />
        <div className="sys-platform-cta">
          <p className="sys-platform-cta-note">{PLATFORM_OVERVIEW_PORTFOLIO_NOTE}</p>
          <div className="sys-platform-links">
            <a
              className="sys-platform-cta-hire"
              href={PLATFORM_CONTACT_CALENDLY}
              target="_blank"
              rel="noopener noreferrer"
            >
              Book a Call
            </a>
            <a
              className="sys-platform-cta-run"
              href={G8E_REPO_URL}
              target="_blank"
              rel="noopener noreferrer"
            >
              Run this yourself
            </a>
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
          <span className={`campaign-status status-${currentRun?.lifecycle_state ?? 'neutral'}`}>
            <span className="status-dot" aria-hidden="true" />
            {campaignStatusLabel(currentRun)}
          </span>
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
    <section className="panel" aria-label="Recent campaigns">
      <div className="panel-head">
        <h2>Recent campaigns</h2>
        <Link to="/evaluations" className="panel-link">
          View all campaigns →
        </Link>
      </div>
      {recent.length === 0 ? (
        <p className="panel-empty">No campaigns recorded yet.</p>
      ) : (
        <ul className="mini-runs">
          {recent.map((campaign) => {
            const status = campaign.lifecycleState ?? campaign.qualityState;
            const href = campaign.runId
              ? `/evaluations/${campaign.datasetId}/${campaign.runId}`
              : `/?dataset=${campaign.datasetId}`;
            return (
              <li key={campaign.datasetId}>
                <Link to={href} className="mini-run">
                  <code className="mini-run-id" title={campaign.label}>
                    {shortCampaignLabel(campaign.label)}
                  </code>
                  <span className="mini-run-name">{campaign.detail}</span>
                  <span className="mini-run-when">
                    {formatRelativeTime(campaign.observedAt)}
                  </span>
                  <span className={`status-dot status-${status}`} aria-label={status} />
                </Link>
              </li>
            );
          })}
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
