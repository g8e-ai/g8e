// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { EvaluationDetailView } from '../src/views/EvaluationDetailView';
import { evalStore, recordKey } from '../src/state/store';
import { assignmentLifecycleEvents, isFailureTerminalStatus } from '../src/views/derived';
import type { AssignmentResult, EvaluationSummary, LiveEvent } from '../src/contract/types';

const failedAssignment: AssignmentResult = {
  kind: 'assignment_result',
  schema_version: '1.3.0',
  dataset_id: 'ds-live-run-1',
  assignment_id: 'assign-fail-1',
  run_id: 'run-1',
  task_id: 'tool-arg-run-commands',
  variant_id: 'sam860-lfm2-700m',
  role: 'assistant',
  repetition: 1,
  terminal_status: 'model_failed',
  quality_state: 'terminal_failed',
  metric_values: {},
  stage_summary: [],
  observed_at: '2026-09-16T17:01:32.171974517Z',
  scenario_category: 'tool_arguments',
  evaluation_unit: 'model',
};

const passedAssignment: AssignmentResult = {
  ...failedAssignment,
  assignment_id: 'assign-pass-1',
  terminal_status: 'completed',
  quality_state: 'verified_public',
};

const otherRunFailure: AssignmentResult = {
  ...failedAssignment,
  assignment_id: 'assign-fail-other-run',
  run_id: 'run-other',
};

const evaluation: EvaluationSummary = {
  schema_version: '1.3.0',
  kind: 'evaluation_summary',
  dataset_id: 'ds-live-run-1',
  quality_state: 'verified_public',
  observed_at: '2026-09-16T17:01:32.171974517Z',
  run_id: 'run-1',
  suite_id: 'suite-1',
  arm: 'platform',
  evaluation_unit: 'model',
  model_role_mapping: {},
  lifecycle_state: 'completed',
  assignment_total: 2,
  assignment_completed: 1,
  assignment_failed: 1,
  terminal_outcomes: { completed: 1, model_failed: 1, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
  started_at: '2026-09-16T17:00:00Z',
  ended_at: '2026-09-16T17:01:32Z',
  elapsed_seconds: 92,
  verifier_state: 'passed',
  headline_metrics: { pass_rate: { value: 0.5 } },
};

describe('isFailureTerminalStatus', () => {
  it('treats non-completed terminal outcomes as failures', () => {
    expect(isFailureTerminalStatus('model_failed')).toBe(true);
    expect(isFailureTerminalStatus('grader_failed')).toBe(true);
    expect(isFailureTerminalStatus('completed')).toBe(false);
    expect(isFailureTerminalStatus('running')).toBe(false);
  });
});

describe('assignmentLifecycleEvents', () => {
  it('returns assignment-scoped events in chronological order', () => {
    const events: LiveEvent[] = [
      {
        kind: 'stage_updated',
        schema_version: '1.3.0',
        event_id: 'evt-2',
        dataset_id: 'ds-live-run-1',
        run_id: 'run-1',
        assignment_id: 'assign-fail-1',
        observed_at: '2026-09-16T17:01:30Z',
        lifecycle_status: 'running',
        quality_state: 'live_in_progress',
        completed: 0,
        total: 2625,
        stage_label: 'running tool-arg-run-commands',
      },
      {
        kind: 'stage_updated',
        schema_version: '1.3.0',
        event_id: 'evt-1',
        dataset_id: 'ds-live-run-1',
        run_id: 'run-1',
        assignment_id: 'assign-fail-1',
        observed_at: '2026-09-16T17:01:28Z',
        lifecycle_status: 'queued',
        quality_state: 'live_in_progress',
        completed: 0,
        total: 2625,
        stage_label: 'queued tool-arg-run-commands',
      },
      {
        kind: 'stage_updated',
        schema_version: '1.3.0',
        event_id: 'evt-3',
        dataset_id: 'ds-live-run-1',
        run_id: 'run-1',
        assignment_id: 'assign-other',
        observed_at: '2026-09-16T17:01:29Z',
        lifecycle_status: 'queued',
        quality_state: 'live_in_progress',
        completed: 0,
        total: 2625,
      },
    ];

    const scoped = assignmentLifecycleEvents('assign-fail-1', events);
    expect(scoped.map((event) => event.event_id)).toEqual(['evt-1', 'evt-2']);
  });
});

describe('EvaluationDetailView failures section', () => {
  beforeEach(() => {
    evalStore.loadFixtures([], []);
    evalStore.getState().evaluations.set(recordKey('ds-live-run-1', 'run-1'), evaluation);
    evalStore.getState().assignments.set(recordKey('ds-live-run-1', 'assign-fail-1'), failedAssignment);
    evalStore.getState().assignments.set(recordKey('ds-live-run-1', 'assign-pass-1'), passedAssignment);
    evalStore.getState().assignments.set(recordKey('ds-live-run-1', 'assign-fail-other-run'), otherRunFailure);
  });

  it('lists run-scoped terminal failures at the bottom and links to assignment detail', () => {
    render(
      <MemoryRouter initialEntries={['/evaluations/ds-live-run-1/run-1']}>
        <Routes>
          <Route path="/evaluations/:datasetId/:runId" element={<EvaluationDetailView />} />
        </Routes>
      </MemoryRouter>,
    );

    const failuresSection = screen.getByRole('heading', { name: 'Terminal failures' }).closest('.run-failures');
    expect(failuresSection).not.toBeNull();
    expect(failuresSection).toHaveTextContent('assign-fail-1');
    expect(failuresSection).not.toHaveTextContent('assign-pass-1');
    expect(failuresSection).not.toHaveTextContent('assign-fail-other-run');
    expect(failuresSection?.querySelector('a[href="/evaluations/ds-live-run-1/run-1/assignments/assign-fail-1"]')).not.toBeNull();
  });
});
