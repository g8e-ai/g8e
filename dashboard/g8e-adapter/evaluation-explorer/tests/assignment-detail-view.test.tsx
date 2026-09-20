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

  it('renders rich public context, activity, grades, and evidence without private detail', () => {
    renderAssignment([
      assignmentResult({
        scenario_summary: {
          scenario_id: 'scenario-1', scenario_version: '1.0.0', category: 'tool_selection',
          public_description: '<b>Choose</b> the approved tool.', grading_method: 'deterministic',
          allowed_tools: ['search'], expected_tools: ['search'], forbidden_tools: [],
          criteria: [{ criterion_id: 'selection', public_label: 'Tool choice', public_description: 'Select the matching tool.', grading_method: 'deterministic', required: true }],
          tool_score_dimensions: [],
        },
        semantic_grade_summaries: [{ criterion_id: 'selection', status: 'pass', grading_method: 'deterministic', explanation_code: 'criterion_passed' }],
        activity_summary: {
          model_activity: { availability: 'observed', records: [{ model_role: 'primary', variant_id: 'model-1', usage_availability: 'reported', input_tokens: { value: 0 }, output_tokens: { value: 2 }, retry_count: { value: 0 }, finish_state: 'stop', load_state: 'warm' }] },
          tool_decisions: { availability: 'observed', records: [] },
          tool_calls: { availability: 'not_applicable', records: [] },
          policy_decisions: { availability: 'unavailable', unavailable_reason: 'historical_not_captured', records: [] },
          governed_actions: { availability: 'unavailable', unavailable_reason: 'source_not_captured', records: [] },
        },
        evidence_bindings: [{ sha256: 'a'.repeat(64), schema_ref: 'eval/v1', kind: 'evaluation_projection' }],
        verification_metadata: { provenance: 'bound', verifier_state: 'passed', verifier_contract_version: '2.0.0' },
      }),
    ]);

    expect(screen.getByRole('heading', { name: 'Assignment context' })).toBeInTheDocument();
    expect(screen.getByText('<b>Choose</b> the approved tool.')).toBeInTheDocument();
    expect(screen.queryByText('Choose', { selector: 'b' })).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'What happened' })).toBeInTheDocument();
    expect(screen.getByText('1 observed')).toBeInTheDocument();
    expect(screen.getByText('Deterministic')).toBeInTheDocument();
    expect(screen.getByText('Criterion passed')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Evidence and methodology' })).toBeInTheDocument();
    expect(screen.getByText('Evaluation projection')).toBeInTheDocument();
    expect(screen.queryByText('private')).not.toBeInTheDocument();
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
