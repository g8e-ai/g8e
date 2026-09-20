// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { AssignmentResult, EvaluationSummary, SnapshotRecord } from '../src/contract/types';
import { evalStore } from '../src/state/store';
import { AssignmentDetailView } from '../src/views/AssignmentDetailView';

const DATASET_ID = 'ds-assignment-test';
const RUN_ID = 'run-assignment-test';
const ASSIGNMENT_ID = 'assignment-1';

function evaluationSummary(): EvaluationSummary {
  return {
    schema_version: '1.3.0',
    kind: 'evaluation_summary',
    dataset_id: DATASET_ID,
    quality_state: 'exploratory_partial',
    observed_at: '2026-09-17T00:00:00Z',
    run_id: RUN_ID,
    suite_id: 'suite-test',
    arm: 'ensemble_ungoverned',
    lifecycle_state: 'completed',
    assignment_total: 1,
    assignment_completed: 1,
    assignment_failed: 0,
    terminal_outcomes: { completed: 1, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    verifier_state: 'not_applicable',
    headline_metrics: {},
  };
}

function assignmentResult(partial: Partial<AssignmentResult> = {}): AssignmentResult {
  return {
    schema_version: '1.3.0',
    kind: 'assignment_result',
    dataset_id: DATASET_ID,
    quality_state: 'exploratory_partial',
    observed_at: '2026-09-17T00:00:00Z',
    assignment_id: ASSIGNMENT_ID,
    run_id: RUN_ID,
    task_id: 'scenario-1',
    variant_id: 'model-1',
    role: 'primary',
    repetition: 1,
    terminal_status: 'completed',
    metric_values: { pass: { value: 0 } },
    stage_summary: [],
    ...partial,
  };
}

function renderAssignment(records: AssignmentResult[]): void {
  evalStore.loadFixtures([evaluationSummary(), ...records] as SnapshotRecord[], []);
  render(
    <MemoryRouter initialEntries={[`/evaluations/${DATASET_ID}/${RUN_ID}/assignments/${ASSIGNMENT_ID}`]}>
      <Routes>
        <Route path="/evaluations/:datasetId/:runId/assignments/:assignmentId" element={<AssignmentDetailView />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('AssignmentDetailView', () => {
  beforeEach(() => {
    evalStore.loadFixtures([], []);
  });

  it('shows the scoring summary, scenario label, unpublished stages, and unavailable resources', () => {
    renderAssignment([assignmentResult()]);

    expect(screen.getByRole('heading', { name: 'Scoring summary' })).toBeInTheDocument();
    expect(screen.getByText('Scenario').closest('.detail-row')).toHaveTextContent('scenario-1');
    expect(screen.getByText('Fail')).toBeInTheDocument();
    expect(screen.getByText('Stage timeline not published for this assignment.')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Resource observations' })).toBeInTheDocument();
    expect(screen.getByTestId('metric-latency')).toHaveTextContent('Unavailable');
    expect(screen.getByTestId('metric-input-tokens')).toHaveTextContent('Unavailable');
    expect(screen.getByTestId('metric-output-tokens')).toHaveTextContent('Unavailable');
    expect(screen.getByTestId('metric-retries')).toHaveTextContent('Unavailable');
  });

  it('preserves explicit zero resource observations', () => {
    renderAssignment([
      assignmentResult({
        resource_summary: {
          latency_ms: { value: 0 },
          input_tokens: { value: 0 },
          output_tokens: { value: 0 },
          retries: { value: 0 },
        },
      }),
    ]);

    expect(screen.getByTestId('metric-latency')).toHaveTextContent('0 ms');
    expect(screen.getByTestId('metric-input-tokens')).toHaveTextContent('0 tok');
    expect(screen.getByTestId('metric-output-tokens')).toHaveTextContent('0 tok');
    expect(screen.getByTestId('metric-retries')).toHaveTextContent('0');
  });

  it('keeps an all-unavailable resource summary visible', () => {
    renderAssignment([
      assignmentResult({
        resource_summary: {
          latency_ms: { unavailable_reason: 'resource observation missing' },
          input_tokens: { unavailable_reason: 'usage not reported' },
          output_tokens: { unavailable_reason: 'usage not reported' },
          retries: { unavailable_reason: 'retry count not reported' },
        },
      }),
    ]);

    expect(screen.getByRole('heading', { name: 'Resource observations' })).toBeInTheDocument();
    expect(screen.getByTestId('metric-latency')).toHaveTextContent('resource observation missing');
    expect(screen.getByTestId('metric-input-tokens')).toHaveTextContent('usage not reported');
    expect(screen.getByTestId('metric-output-tokens')).toHaveTextContent('usage not reported');
    expect(screen.getByTestId('metric-retries')).toHaveTextContent('retry count not reported');
  });

  it('collapses only the exact scenario-not-applicable scorecard dimensions', () => {
    renderAssignment([
      assignmentResult({
        benchmark_observations: {
          tool_scorecard: {
            tool_recognition: { unavailable_reason: 'tool use not required by this scenario' },
            tool_selection: { unavailable_reason: 'tool use not required by this scenario' },
            argument_schema: { unavailable_reason: 'required evidence not observed' },
          },
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.queryByText('Tool recognition')).not.toBeInTheDocument();
    expect(screen.queryByText('Tool selection')).not.toBeInTheDocument();
    expect(screen.getByText('Argument schema')).toBeInTheDocument();
    expect(screen.getByText('required evidence not observed')).toBeInTheDocument();
  });

  it('renders isolated sibling repetitions in deterministic order', () => {
    renderAssignment([
      assignmentResult(),
      assignmentResult({ assignment_id: 'assignment-3', repetition: 3 }),
      assignmentResult({ assignment_id: 'assignment-2', repetition: 2 }),
      assignmentResult({ assignment_id: 'assignment-other-role', repetition: 2, role: 'assistant' }),
    ]);

    expect(screen.getByRole('heading', { name: 'Sibling repetitions' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'assignment-2' })).toHaveAttribute(
      'href',
      `/evaluations/${DATASET_ID}/${RUN_ID}/assignments/assignment-2`,
    );
    expect(screen.getByRole('link', { name: 'assignment-3' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'assignment-other-role' })).not.toBeInTheDocument();
    const siblingLinks = screen.getAllByRole('link').filter((link) => link.textContent?.startsWith('assignment-'));
    expect(siblingLinks.map((link) => link.textContent)).toEqual(['assignment-2', 'assignment-3']);
  });
});
