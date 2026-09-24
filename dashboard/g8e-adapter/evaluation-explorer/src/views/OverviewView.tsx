// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Overview view — the landing page. Layout mirrors the OpenDevOps.ai
// surface: the live event stream, recent campaigns, and the
// public mirror download endpoints. Every panel renders real store data;
// nothing on this page is decorative.

import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useDatasetOptions } from '../state/dataset';
import { useStoreState } from '../state/store';
import { loadRuntimeConfig } from '../state/feed';
import { LiveEventStream } from '../components/LiveEventStream';
import { formatNumber } from '../components/shared';
import { formatRelativeTime } from '../utils/format';
import { recentCampaignRows } from './derived';
import descriptorUrl from '../contract/descriptor.json?url';
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

/** Public-safe evaluation datasets, contract, and raw mirror endpoints. */
function DownloadsPanel() {
  const [origin, setOrigin] = useState<string | null>(null);
  const proofArtifactCount = useStoreState((state) => state.proofArtifactCount);
  const proofsPublished = proofArtifactCount > 0;

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

  const resources: PublicResourceCard[] = [
    {
      label: 'Campaign summaries',
      eyebrow: 'Analyze runs',
      description: 'Run identity, lifecycle, terminal outcomes, headline metrics, and verifier disposition.',
      format: 'JSON · paginated',
      href: origin ? `${origin}/history?kind=evaluation_summary&cursor=0&limit=500` : undefined,
      disabledReason: origin ? undefined : 'Mirror origin unavailable.',
    },
    {
      label: 'Assignment results',
      eyebrow: 'Analyze tasks',
      description: 'Scenario outcomes, closed grades, grouped activity, bounded resources, and evidence bindings.',
      format: 'JSON · paginated',
      href: origin ? `${origin}/history?kind=assignment_result&cursor=0&limit=500` : undefined,
      disabledReason: origin ? undefined : 'Mirror origin unavailable.',
    },
    {
      label: 'Cryptographic proofs',
      eyebrow: 'Verify offline',
      description: proofsPublished
        ? 'Signed proof manifest, content-addressed artifacts, and catalog entries for offline verification.'
        : 'No verified proof package has been published yet. Live projections stream separately from signed proof artifacts.',
      format: proofsPublished ? `JSON · ${formatNumber(proofArtifactCount)} artifact${proofArtifactCount === 1 ? '' : 's'}` : 'JSON · pending',
      href: origin && proofsPublished ? `${origin}/proof-catalog` : undefined,
      disabledReason: origin ? undefined : 'Mirror origin unavailable.',
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
        Includes aggregate results, closed grade metadata, bounded resource metrics, verification state, and approved content bindings. Prompts, outputs, identities, paths, and receipt bodies stay private. Signed proof packages publish on a separate mirror path after verification completes.
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
      <div className="public-api-strip">
        <span className="public-api-label">Mirror transport</span>
        {origin ? (
          <>
            <a href={`${origin}/history?cursor=0&limit=500`} target="_blank" rel="noopener noreferrer">History <code>JSON</code></a>
            <a href={`${origin}/bootstrap`} target="_blank" rel="noopener noreferrer">Bootstrap <code>JSON</code></a>
            {proofsPublished ? (
              <a href={`${origin}/proof-manifest`} target="_blank" rel="noopener noreferrer">Proof manifest <code>JSON</code></a>
            ) : null}
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
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()));
  const allEvaluations = useStoreState((state) => Array.from(state.evaluations.values()));
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

      <div className="ov-grid-bottom">
        <RecentCampaigns catalogs={catalogs} evaluations={allEvaluations} />
        <DownloadsPanel />
      </div>
    </div>
  );
}
