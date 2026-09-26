// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { CatalogSnapshot, EvaluationSummary, LiveEvent, SnapshotRecord, SuiteSummary } from '../src/contract/types';
import { evalStore } from '../src/state/store';
import { OverviewView } from '../src/views/OverviewView';

const datasetId = 'ds-live-smoke-run';

const catalog: CatalogSnapshot = {
  schema_version: '1.4.0',
  kind: 'catalog_snapshot',
  dataset_id: datasetId,
  dataset_kind: 'live_run',
  quality_state: 'live_in_progress',
  observed_at: '2026-09-22T12:05:00Z',
  title: 'Live smoke run',
  description: 'Evaluating model reliability across governed operational scenarios.',
  limitations: [],
  model_count: 1,
  evaluated_count: 1,
  suite_count: 1,
  run_count: 1,
  assignment_count: 75,
  provider_request_count: 6,
  provider_token_count: 1200,
  retry_count: 0,
  verifier_passed_count: 0,
  verifier_failed_count: 0,
  generated_at: '2026-09-22T12:05:00Z',
};

const archivedCatalog: CatalogSnapshot = {
  ...catalog,
  dataset_id: 'ds-verified-archive',
  dataset_kind: 'verified_public_snapshot',
  quality_state: 'verified_public',
  title: 'Archived campaign',
};

const suite: SuiteSummary = {
  schema_version: '1.4.0',
  kind: 'suite_summary',
  dataset_id: datasetId,
  quality_state: 'live_in_progress',
  observed_at: '2026-09-22T12:05:00Z',
  suite_id: 'north-star-25',
  display_name: 'North Star 25',
  task_count: 25,
  assignment_count: 75,
  status: 'running',
  verifier_state: 'not_applicable',
  model_coverage: ['ministral-3-8b'],
  metric_summaries: {},
  limitations: [],
};

const evaluation: EvaluationSummary = {
  schema_version: '1.4.0',
  kind: 'evaluation_summary',
  dataset_id: datasetId,
  quality_state: 'live_in_progress',
  observed_at: '2026-09-22T12:05:00Z',
  run_id: 'eval-init-ministral-3-8b-1790084488',
  campaign_id: 'smoke-run',
  suite_id: 'north-star-25',
  arm: 'homogeneous-model-role',
  lifecycle_state: 'running',
  assignment_total: 75,
  assignment_completed: 5,
  assignment_failed: 1,
  terminal_outcomes: { completed: 5, model_failed: 1, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
  started_at: '2026-09-22T12:00:00Z',
  verifier_state: 'not_applicable',
  headline_metrics: {},
};

const events: LiveEvent[] = [
  {
    schema_version: '1.4.0',
    kind: 'assignment_started',
    dataset_id: datasetId,
    quality_state: 'live_in_progress',
    observed_at: '2026-09-22T12:04:55Z',
    event_id: 'event-1',
    run_id: evaluation.run_id,
    assignment_id: 'assignment-7',
    task_id: 'tool-selection-04',
    variant_id: 'ministral-3-8b',
    role: 'primary',
    lifecycle_status: 'running',
    completed: 6,
    total: 75,
  },
  {
    schema_version: '1.4.0',
    kind: 'stage_updated',
    dataset_id: datasetId,
    quality_state: 'live_in_progress',
    observed_at: '2026-09-22T12:05:00Z',
    event_id: 'event-2',
    run_id: evaluation.run_id,
    assignment_id: 'assignment-7',
    lifecycle_status: 'running',
    completed: 6,
    total: 75,
    stage_label: 'semantic grading',
  },
];

describe('OverviewView', () => {
  beforeEach(() => {
    evalStore.loadFixtures([catalog, archivedCatalog, suite, evaluation] as SnapshotRecord[], events);
    evalStore.setConnection('live');
    evalStore.setStreamConnection('connected');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders the live event stream full width without the system overview panel', () => {
    render(
      <MemoryRouter initialEntries={['/?dataset=ds-verified-archive']}>
        <OverviewView />
      </MemoryRouter>,
    );

    expect(screen.getByRole('region', { name: 'Events' })).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'System overview' })).not.toBeInTheDocument();
    expect(screen.queryByText('Active campaign')).not.toBeInTheDocument();
    expect(screen.queryByText(/live deployment of the g8e AI governance suite/i)).not.toBeInTheDocument();
  });

  it('presents campaign coverage and public-safe data surfaces for engineers', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ schema_version: '1.0.0', mirror_origin: 'https://mirror.example' }),
    }));

    render(
      <MemoryRouter>
        <OverviewView />
      </MemoryRouter>,
    );

    const evidence = within(screen.getByRole('region', { name: 'Campaign evidence' }));
    expect(evidence.getByText('Campaign evidence')).toBeInTheDocument();
    expect(evidence.getByText('Completed')).toBeInTheDocument();
    expect(evidence.getByText('Failed')).toBeInTheDocument();
    expect(evidence.getByText('Remaining')).toBeInTheDocument();
    const coverage = evidence.getByRole('progressbar', { name: 'smoke-run assignment outcomes' });
    expect(coverage).toHaveAttribute('aria-valuenow', '6');
    expect(coverage).toHaveAttribute('aria-valuemax', '75');
    expect(evidence.getByText('6 / 75 terminal')).toBeInTheDocument();
    expect(evidence.getByText('Verification pending')).toBeInTheDocument();

    const data = within(screen.getByRole('region', { name: 'Public data and APIs' }));
    expect(data.getByText('Public data & APIs')).toBeInTheDocument();
    expect(data.getByText(/public-safe projection/i)).toBeInTheDocument();
    expect(await data.findByRole('link', { name: /Campaign summaries/i })).toHaveAttribute(
      'href',
      'https://mirror.example/history?kind=evaluation_summary&cursor=0&limit=500',
    );
    expect(data.getByRole('link', { name: /Assignment results/i })).toHaveAttribute(
      'href',
      'https://mirror.example/history?kind=assignment_result&cursor=0&limit=500',
    );
    expect(data.getByText('Cryptographic proofs')).toBeInTheDocument();
    expect(data.getByText(/No verified proof package has been published yet/i)).toBeInTheDocument();
    expect(data.queryByRole('link', { name: /Cryptographic proofs/i })).not.toBeInTheDocument();
    expect(data.queryByRole('link', { name: /Proof manifest/i })).not.toBeInTheDocument();
    expect(data.getByRole('link', { name: /Live updates/i })).toHaveAttribute(
      'href',
      'https://mirror.example/stream',
    );
    expect(data.queryByText('Download data')).not.toBeInTheDocument();
  });
});
