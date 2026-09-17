import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { EvaluationSummary, SnapshotRecord } from '../src/contract/types';
import { evalStore } from '../src/state/store';
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
});
