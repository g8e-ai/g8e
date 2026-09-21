// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Overview view — the landing page. Layout mirrors the OpenDevOps.ai
// surface: the live event stream, a system overview
// with dataset coverage, recent campaigns, and the
// public mirror download endpoints. Every panel renders real store data;
// nothing on this page is decorative.

import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  G8E_REPO_URL,
  PLATFORM_CONTACT_CALENDLY,
  PLATFORM_CONTACT_EMAIL,
  PLATFORM_FLOW_STEPS,
  PLATFORM_LEDE,
  PLATFORM_OVERVIEW_PORTFOLIO_NOTE,
} from '../content/platform';
import { useActiveDatasetId } from '../state/dataset';
import { recordKey, useStoreState } from '../state/store';
import { loadRuntimeConfig } from '../state/feed';
import { DatasetSelector } from '../components/DatasetSelector';
import { LiveEventStream } from '../components/LiveEventStream';
import {
  ProgressBar,
  ReconcilePlaceholder,
  StreamStatusIndicator,
  formatNumber,
} from '../components/shared';
import { formatRelativeTime } from '../utils/format';
import {
  type FeedConnectionState,
  type StreamConnectionState,
} from '../utils/feed-state';
import { campaignTerminalProgress, recentCampaignRows } from './derived';
import descriptorUrl from '../contract/descriptor.json?url';
import type {
  CatalogSnapshot,
  EvaluationSummary,
  LiveEvent,
  SuiteSummary,
} from '../contract/types';

function shortRunId(runId: string): string {
  return runId.length > 14 ? `…${runId.slice(-12)}` : runId;
}

function shortCampaignLabel(label: string): string {
  return label.length > 22 ? `…${label.slice(-20)}` : label;
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
  streamConnection: StreamConnectionState;
  isReconciling: boolean;
};

/** System overview: platform context, active campaign progress, and current run. */
function SystemOverviewPanel({
  catalog,
  evaluations,
  suites,
  events,
  activeDatasetId,
  connection,
  streamConnection,
  isReconciling,
}: SystemOverviewPanelProps) {
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
              Book a Time
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
        <h3 className="sys-section-title">Active campaign</h3>
        <DatasetSelector activeId={activeDatasetId} />
        {catalog ? <p className="panel-note">{catalog.title}</p> : null}

        {isReconciling ? (
          <ReconcilePlaceholder label="Loading feed history…" />
        ) : catalog ? (
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
        {isReconciling ? (
          <ReconcilePlaceholder label="Loading current task…" />
        ) : currentRun && latestEvent ? (
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
  const [params] = useSearchParams();
  const routeDataset = params.get('dataset') ?? undefined;
  const activeDatasetId = useActiveDatasetId(routeDataset);
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
          activeDatasetId={activeDatasetId}
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
