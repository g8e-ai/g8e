import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { FailuresView } from '../src/views/FailuresView';
import { evalStore } from '../src/state/store';
import { assignmentLifecycleEvents, isFailureTerminalStatus } from '../src/views/derived';
import type { AssignmentResult, LiveEvent } from '../src/contract/types';

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

describe('FailuresView', () => {
  it('lists preserved terminal failures and links to assignment detail', () => {
    evalStore.loadFixtures([], []);
    evalStore.getState().assignments.set('ds-live-run-1:assign-fail-1', failedAssignment);
    evalStore.getState().assignments.set('ds-live-run-1:assign-pass-1', passedAssignment);

    render(
      <MemoryRouter initialEntries={['/failures?dataset=ds-live-run-1']}>
        <FailuresView />
      </MemoryRouter>,
    );

    expect(screen.getByRole('heading', { name: 'Preserved terminal failures' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'assign-fail-1' })).toHaveAttribute(
      'href',
      '/evaluations/ds-live-run-1/run-1/assignments/assign-fail-1',
    );
    expect(screen.queryByRole('link', { name: 'assign-pass-1' })).not.toBeInTheDocument();
  });
});
