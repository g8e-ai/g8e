// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { EvaluationSummary, SnapshotRecord } from '../src/contract/types';
import { evalStore } from '../src/state/store';
import { EvaluationDetailView } from '../src/views/EvaluationDetailView';
import { EvaluationsView } from '../src/views/EvaluationsView';

function evaluation(
  datasetId: string,
  runId: string,
  startedAt: string,
): EvaluationSummary {
  return {
    schema_version: '1.3.0',
    kind: 'evaluation_summary',
    dataset_id: datasetId,
    quality_state: 'live_in_progress',
    observed_at: startedAt,
    run_id: runId,
    suite_id: 'eval-init-suite',
    arm: 'platform',
    evaluation_unit: 'model',
    model_role_mapping: {},
    lifecycle_state: 'running',
    assignment_total: 75,
    assignment_completed: 10,
    assignment_failed: 0,
    terminal_outcomes: { completed: 10, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    started_at: startedAt,
    verifier_state: 'not_applicable',
    headline_metrics: {},
  };
}

describe('EvaluationsView', () => {
  beforeEach(() => {
    evalStore.loadFixtures(
      [
        evaluation('ds-live-init-campaign-1789654273', 'init-campaign-1789654273', '2026-09-17T10:00:00Z'),
        evaluation('ds-live-eval-init-qwen3-4b-1789657337', 'eval-init-qwen3-4b-1789657337', '2026-09-17T12:00:00Z'),
      ] as unknown as SnapshotRecord[],
      [],
    );
  });

  it('only lists quality filter options that exist in the current runs', () => {
    render(
      <MemoryRouter>
        <EvaluationsView />
      </MemoryRouter>,
    );

    const qualityFilter = screen.getByRole('combobox', { name: 'Filter by quality' });
    const labels = Array.from(qualityFilter.querySelectorAll('option')).map((option) => option.textContent);
    expect(labels).toEqual([
      'All quality states',
      'In progress · unverified',
    ]);
    expect(labels).not.toContain('Run-scoped verification passed');
  });

  it('lists runs from every dataset without a dataset selector', () => {
    render(
      <MemoryRouter>
        <EvaluationsView />
      </MemoryRouter>,
    );

    expect(screen.queryByTestId('dataset-selector')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'init-campaign-1789654273' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'eval-init-qwen3-4b-1789657337' })).toBeInTheDocument();
    expect(screen.getByTitle('ds-live-init-campaign-1789654273')).toBeInTheDocument();
    expect(screen.getByTitle('ds-live-eval-init-qwen3-4b-1789657337')).toBeInTheDocument();
  });

  it('renders typed model headline coverage without system-only cards', () => {
    const summary: EvaluationSummary = {
      ...evaluation('ds-current', 'run-current', '2026-09-22T08:00:00Z'),
      schema_version: '1.5.0',
      lifecycle_state: 'completed',
      assignment_completed: 3,
      assignment_failed: 1,
      terminal_outcomes: { completed: 3, model_failed: 1, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
      verifier_state: 'not_run',
      headline_metrics: {
        pass_rate: { value: 0.75, unit: 'ratio', observed_count: 4, eligible_count: 4, unavailable_count: 0 },
        latency_p50_ms: { value: 640, unit: 'milliseconds', observed_count: 3, eligible_count: 4, unavailable_count: 1 },
        output_throughput_p50_tokens_per_second: { unavailable_reason: 'incomplete_contributor_evidence', unit: 'tokens_per_second', observed_count: 0, eligible_count: 4, unavailable_count: 4 },
      },
    };
    evalStore.loadFixtures([summary], []);

    render(
      <MemoryRouter initialEntries={['/evaluations/ds-current/run-current']}>
        <Routes>
          <Route path="/evaluations/:datasetId/:runId" element={<EvaluationDetailView />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByTestId('metric-pass-rate')).toHaveTextContent('4 observed / 4 eligible');
    expect(screen.getByTestId('metric-latency-p50')).toHaveTextContent('3 observed / 4 eligible');
    expect(screen.getByTestId('metric-output-throughput-p50')).toHaveTextContent('0 observed / 4 eligible');
    expect(screen.queryByTestId('metric-primary-invocation-share')).not.toBeInTheDocument();
    expect(screen.queryByTestId('metric-correlated-failure-rate')).not.toBeInTheDocument();
    expect(screen.getByText(/heterogeneous routing and correlation metrics appear only for system evaluations/i)).toBeInTheDocument();
    expect(screen.getAllByText('Not yet verified')).toHaveLength(2);
    expect(screen.queryByText('not_run')).not.toBeInTheDocument();
  });
});
