// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Overview view — the live event stream and recent campaigns.

import { useMemo } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { CURRENT_PLATFORM_RELEASE, matchesRelease, releaseLabel } from '../content/release';
import { useDatasetOptions } from '../state/dataset';
import { useStoreState } from '../state/store';
import { LiveEventStream } from '../components/LiveEventStream';
import { WhatAmILookingAt } from '../components/WhatAmILookingAt';
import { formatNumber } from '../components/shared';
import { formatDuration, formatRelativeTime, formatTimestamp } from '../utils/format';
import { recentCampaignRows, roleLabel, type RecentCampaignRow } from './derived';
import type { CatalogSnapshot, EvaluationSummary, ModelSummary } from '../contract/types';

function capitalize(value: string): string {
  return value.length > 0 ? `${value.charAt(0).toUpperCase()}${value.slice(1)}` : value;
}

function formatClock(iso: string | undefined): string {
  if (!iso) return 'Unavailable';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return 'Unavailable';
  return date.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
}

/** Wall-clock seconds a run has spent executing: recorded elapsed, start→end, or start→now while live. */
function runDurationSeconds(row: RecentCampaignRow, now: number): number | undefined {
  if (row.elapsedSeconds !== undefined) return row.elapsedSeconds;
  if (!row.startedAt) return undefined;
  const start = new Date(row.startedAt).getTime();
  const end = row.endedAt ? new Date(row.endedAt).getTime() : row.lifecycleState === 'running' ? now : NaN;
  return Number.isFinite(start) && Number.isFinite(end) && end >= start ? (end - start) / 1000 : undefined;
}

/** Most recent run for each dataset in the feed. */
function RecentRuns({
  catalogs,
  evaluations,
  models,
}: {
  catalogs: CatalogSnapshot[];
  evaluations: EvaluationSummary[];
  models: ModelSummary[];
}) {
  const recent = useMemo(
    () => recentCampaignRows(catalogs, evaluations, models),
    [catalogs, evaluations, models],
  );
  const now = Date.now();
  return (
    <section className="panel campaign-evidence-panel" aria-label="Recent runs">
      <div className="panel-head campaign-evidence-head">
        <div>
          <h2>Recent Runs</h2>
          <p className="panel-note">
            The latest evaluation run from each dataset: the models under test, when it started and how long it ran,
            and how many of its assignments have reached a final result.
          </p>
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
        <p className="panel-empty">No runs recorded yet.</p>
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
            const title = campaign.modelName ?? campaign.label;
            const otherModels = campaign.models.filter((model) => model.name !== campaign.modelName);
            const duration = runDurationSeconds(campaign, now);
            const running = campaign.lifecycleState === 'running';
            const startedAt = campaign.startedAt ?? campaign.observedAt;
            return (
              <li key={campaign.datasetId} className="campaign-evidence-row">
                <div className="campaign-evidence-title-row">
                  <Link to={href} className="campaign-evidence-title" title={title}>
                    {title}
                  </Link>
                  <span className={`campaign-evidence-state status-${status}`}>
                    <span className="status-dot" aria-hidden="true" />
                    {campaign.lifecycleState ? capitalize(campaign.lifecycleState) : capitalize(campaign.qualityState.replaceAll('_', ' '))}
                  </span>
                </div>
                <div className="campaign-evidence-meta">
                  <span title={campaign.label}>{campaign.label}</span>
                  <span>{campaign.detail}</span>
                  <span>{releaseLabel(campaign)}</span>
                </div>
                {otherModels.length > 0 ? (
                  <ul className="campaign-evidence-roles" aria-label="Models by role">
                    {campaign.models.map((model) => (
                      <li key={model.role} title={model.variantId}>
                        <span className="campaign-evidence-role">{roleLabel(model.role)}</span> {model.name}
                      </li>
                    ))}
                  </ul>
                ) : null}
                {campaign.runId && total > 0 ? (
                  <>
                    <div
                      className="campaign-outcome-track"
                      role="progressbar"
                      aria-label={`${title} assignment outcomes`}
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
                <div className="campaign-evidence-times">
                  <span title={formatTimestamp(startedAt)}>
                    Started {formatClock(startedAt)} · {formatRelativeTime(startedAt)}
                  </span>
                  {duration !== undefined ? (
                    <span title={campaign.endedAt ? `Ended ${formatTimestamp(campaign.endedAt)}` : undefined}>
                      {running ? 'Running for' : 'Ran for'} {formatDuration(duration)}
                    </span>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

export function OverviewView() {
  const [params] = useSearchParams();
  const release = params.get('release') ?? CURRENT_PLATFORM_RELEASE;
  const activeDatasetId = useDatasetOptions(release).find((option) => option.available && option.kind === 'live_run')?.id ?? '';
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()).filter((record) => matchesRelease(record, release)));
  const allEvaluations = useStoreState((state) => Array.from(state.evaluations.values()).filter((record) => matchesRelease(record, release)));
  const allModels = useStoreState((state) => Array.from(state.models.values()).filter((record) => matchesRelease(record, release)));
  const events = useStoreState((state) =>
    state.events.filter((event) => event.dataset_id === activeDatasetId),
  );
  const connection = useStoreState((state) => state.connection);
  const streamConnection = useStoreState((state) => state.streamConnection);
  const isReconciling = useStoreState((state) => state.pendingSnapshot !== null);

  return (
    <div className="overview">
      <LiveEventStream
        events={events}
        connection={connection}
        streamConnection={streamConnection}
        isReconciling={isReconciling}
      />

      <div className="overview-content">
        <div className="overview-primary">
          <RecentRuns catalogs={catalogs} evaluations={allEvaluations} models={allModels} />
        </div>
        <WhatAmILookingAt />
      </div>
    </div>
  );
}
