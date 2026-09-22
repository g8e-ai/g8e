import { beforeEach, describe, expect, it } from 'vitest';
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
    expect(panel.getByText('Live')).toBeInTheDocument();
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
    expect(panel.getByRole('link', { name: 'View Details' })).toHaveAttribute(
      'href',
      `/evaluations/${datasetId}/${evaluation.run_id}`,
    );
    expect(panel.getByRole('link', { name: 'Methodology' })).toHaveAttribute('href', '/methodology');
    expect(panel.queryByText('Models evaluated')).not.toBeInTheDocument();
    expect(panel.queryByText('Current task')).not.toBeInTheDocument();
  });
});
