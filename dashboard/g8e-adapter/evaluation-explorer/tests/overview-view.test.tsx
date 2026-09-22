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

describe('OverviewView active campaign', () => {
  beforeEach(() => {
    evalStore.loadFixtures([catalog, archivedCatalog, suite, evaluation] as SnapshotRecord[], events);
    evalStore.setConnection('live');
    evalStore.setStreamConnection('connected');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('prioritizes campaign progress, scope, verification, and latest activity', () => {
    render(
      <MemoryRouter initialEntries={['/?dataset=ds-verified-archive']}>
        <OverviewView />
      </MemoryRouter>,
    );

    const panel = within(screen.getByRole('region', { name: 'System overview' }));
    expect(panel.getByText('Live smoke run')).toBeInTheDocument();
    expect(panel.queryByText('Archived campaign')).not.toBeInTheDocument();
    expect(panel.queryByTestId('dataset-selector')).not.toBeInTheDocument();
    expect(panel.getByText(catalog.description)).toBeInTheDocument();
    const campaignHeader = panel.getByText('Active campaign').parentElement;
    expect(campaignHeader).not.toBeNull();
    const campaignStatus = within(campaignHeader as HTMLElement).getByTestId('stream-status');
    expect(campaignStatus).toHaveTextContent('Live');
    expect(campaignStatus).toHaveClass('status-ok');
    expect(panel.getByText('6 of 75 assignments complete')).toBeInTheDocument();
    expect(panel.getByText('8%')).toBeInTheDocument();
    expect(panel.getByText('1 failed')).toBeInTheDocument();
    const scope = panel.getByRole('list', { name: 'Campaign scope and verification' });
    expect(scope).toHaveTextContent('1 model');
    expect(scope).toHaveTextContent('1 suite');
    expect(scope).toHaveTextContent('Verification pending');
    expect(panel.getByText('Now evaluating')).toBeInTheDocument();
    expect(panel.getByText('ministral-3-8b')).toBeInTheDocument();
    expect(panel.getByText('tool-selection-04')).toBeInTheDocument();
    expect(panel.getByText('Semantic grading')).toBeInTheDocument();
    expect(panel.getByText('Primary role')).toBeInTheDocument();
    expect(panel.queryByText('in the future')).not.toBeInTheDocument();
    expect(panel.getByRole('link', { name: 'View Details' })).toHaveAttribute(
      'href',
      `/evaluations/${datasetId}/${evaluation.run_id}`,
    );
    expect(panel.getByRole('link', { name: 'Methodology' })).toHaveAttribute('href', '/methodology');
    expect(panel.queryByText('Models evaluated')).not.toBeInTheDocument();
    expect(panel.queryByText('Current task')).not.toBeInTheDocument();
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
    expect(await data.findByRole('link', { name: /Campaign records/i })).toHaveAttribute(
      'href',
      'https://mirror.example/history?cursor=0&limit=500',
    );
    expect(data.getByRole('link', { name: /Public proof index/i })).toHaveAttribute(
      'href',
      'https://mirror.example/proof-catalog',
    );
    expect(data.getByRole('link', { name: /Live updates/i })).toHaveAttribute(
      'href',
      'https://mirror.example/stream',
    );
    expect(data.queryByText('Download data')).not.toBeInTheDocument();
  });

  it('does not report a running campaign as live while SSE is reconnecting', () => {
    evalStore.setStreamConnection('reconnecting');

    render(
      <MemoryRouter>
        <OverviewView />
      </MemoryRouter>,
    );

    const panel = within(screen.getByRole('region', { name: 'System overview' }));
    const campaignHeader = panel.getByText('Active campaign').parentElement;
    expect(campaignHeader).not.toBeNull();
    const campaignStatus = within(campaignHeader as HTMLElement).getByTestId('stream-status');
    expect(campaignStatus).toHaveTextContent('Reconnecting');
    expect(campaignStatus).toHaveClass('status-warn');
    expect(campaignStatus).not.toHaveTextContent('Live');
  });
});
