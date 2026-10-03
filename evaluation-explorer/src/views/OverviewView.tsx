// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Overview view — the live event stream, recent campaigns, and public data.

import { useMemo } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { CURRENT_PLATFORM_RELEASE, matchesRelease, releaseLabel } from '../content/release';
import { useDatasetOptions } from '../state/dataset';
import { useStoreState } from '../state/store';
import { useMirrorOrigin } from '../state/mirror';
import { LiveEventStream } from '../components/LiveEventStream';
import { WhatAmILookingAt } from '../components/WhatAmILookingAt';
import { formatNumber } from '../components/shared';
import { formatRelativeTime } from '../utils/format';
import { recentCampaignRows } from './derived';
import type { CatalogSnapshot, EvaluationSummary } from '../contract/types';

function shortCampaignLabel(label: string): string {
  return label.length > 22 ? `…${label.slice(-20)}` : label;
}

function capitalize(value: string): string {
  return value.length > 0 ? `${value.charAt(0).toUpperCase()}${value.slice(1)}` : value;
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
                  <span>{releaseLabel(campaign)}</span>
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

type PublicResourceCard = {
  label: string;
  eyebrow: string;
  description: string;
  format: string;
  href?: string;
  disabledReason?: string;
};

/** Public-safe evaluation datasets. Transport and schema links live in the footer. */
function DownloadsPanel() {
  const origin = useMirrorOrigin();
  const proofArtifactCount = useStoreState((state) => state.proofArtifactCount);
  const proofsPublished = proofArtifactCount > 0;
  const unavailable = origin ? undefined : 'Mirror origin unavailable.';

  const resources: PublicResourceCard[] = [
    {
      label: 'Campaign summaries',
      eyebrow: 'Analyze runs',
      description: 'Lifecycle, terminal outcomes, headline metrics, verifier disposition.',
      format: 'JSON · paginated',
      href: origin ? `${origin}/history?kind=evaluation_summary&cursor=0&limit=500` : undefined,
      disabledReason: unavailable,
    },
    {
      label: 'Assignment results',
      eyebrow: 'Analyze tasks',
      description: 'Scenario outcomes, closed grades, bounded resources, evidence bindings.',
      format: 'JSON · paginated',
      href: origin ? `${origin}/history?kind=assignment_result&cursor=0&limit=500` : undefined,
      disabledReason: unavailable,
    },
    {
      label: 'Cryptographic proofs',
      eyebrow: 'Verify offline',
      description: proofsPublished
        ? 'Signed manifest and content-addressed artifacts for offline verification.'
        : 'No verified proof package published yet.',
      format: proofsPublished ? `JSON · ${formatNumber(proofArtifactCount)} artifact${proofArtifactCount === 1 ? '' : 's'}` : 'JSON · pending',
      href: origin && proofsPublished ? `${origin}/proof-catalog` : undefined,
      disabledReason: origin ? undefined : unavailable,
    },
  ];

  return (
    <section className="panel public-data-panel" id="downloads" aria-label="Public data and APIs">
      <div className="public-data-head">
        <h2>Public data &amp; APIs</h2>
        <p className="panel-note">
          Anonymous, public-safe projection. Prompts, outputs, identities, paths, and receipt bodies stay private.
        </p>
      </div>
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
              <div className="public-resource-card public-resource-disabled" aria-disabled="true">
                <span className="public-resource-eyebrow">{resource.eyebrow}</span>
                <strong>{resource.label}</strong>
                <span className="public-resource-description">
                  {resource.disabledReason ?? resource.description}
                </span>
                <span className="public-resource-format">{resource.format}</span>
              </div>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

export function OverviewView() {
  const [params] = useSearchParams();
  const release = params.get('release') ?? CURRENT_PLATFORM_RELEASE;
  const activeDatasetId = useDatasetOptions(release).find((option) => option.available && option.kind === 'live_run')?.id ?? '';
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()).filter((record) => matchesRelease(record, release)));
  const allEvaluations = useStoreState((state) => Array.from(state.evaluations.values()).filter((record) => matchesRelease(record, release)));
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
          <RecentCampaigns catalogs={catalogs} evaluations={allEvaluations} />
          <DownloadsPanel />
        </div>
        <WhatAmILookingAt />
      </div>
    </div>
  );
}
