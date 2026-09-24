// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { EvaluationSummary, LiveEvent } from '../src/contract/types';
import { LiveEventStream } from '../src/components/LiveEventStream';
import { evalStore, recordKey } from '../src/state/store';
import { EvaluationDetailView } from '../src/views/EvaluationDetailView';

function liveEvent(partial: Partial<LiveEvent> & Pick<LiveEvent, 'event_id' | 'kind'>): LiveEvent {
  return {
    schema_version: '1.3.0',
    dataset_id: 'ds-live-a',
    quality_state: 'live_in_progress',
    observed_at: '2026-09-17T08:00:00Z',
    run_id: 'run-a',
    lifecycle_status: 'running',
    completed: 0,
    total: 75,
    ...partial,
  };
}

describe('LiveEventStream', () => {
  it('renders bootstrap events while history replay is in progress', () => {
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[liveEvent({ event_id: 'evt-bootstrap', kind: 'assignment_started' })]}
          connection="live"
          streamConnection="connecting"
          isReconciling
        />
      </MemoryRouter>,
    );

    expect(screen.getByText('Syncing feed history…')).toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
  });

  it('shows only the 100 most recent events', () => {
    const events = Array.from({ length: 120 }, (_, index) =>
      liveEvent({
        event_id: `evt-${index}`,
        variant_id: `model-${index}`,
        kind: 'assignment_completed',
        observed_at: `2026-09-17T${String(Math.floor(index / 60)).padStart(2, '0')}:${String(index % 60).padStart(2, '0')}:00Z`,
      }),
    );

    render(
      <MemoryRouter>
        <LiveEventStream events={events} connection="live" streamConnection="connected" />
      </MemoryRouter>,
    );

    expect(screen.getByText('Page 1 of 4 (100 events)')).toBeInTheDocument();
    expect(screen.queryByText('model-0')).not.toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: 'model-119' })).toHaveLength(1);
  });

  it('paginates events and scrolls per page', async () => {
    const user = userEvent.setup();
    const events = Array.from({ length: 30 }, (_, index) =>
      liveEvent({
        event_id: `evt-${index}`,
        kind: 'assignment_completed',
        observed_at: `2026-09-17T08:${String(index).padStart(2, '0')}:00Z`,
      }),
    );

    render(
      <MemoryRouter>
        <LiveEventStream events={events} connection="live" streamConnection="connected" />
      </MemoryRouter>,
    );

    expect(screen.getByTestId('stream-pagination')).toBeInTheDocument();
    expect(screen.getByText('Page 1 of 2 (30 events)')).toBeInTheDocument();
    expect(screen.getAllByRole('row')).toHaveLength(26); // header + 25 rows

    await user.click(screen.getByRole('button', { name: 'Next page' }));

    expect(screen.getByText('Page 2 of 2 (30 events)')).toBeInTheDocument();
    expect(screen.getAllByRole('row')).toHaveLength(6); // header + 5 rows
  });

  it('labels a running assignment start as Started while preserving terminal statuses', () => {
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[
            liveEvent({ event_id: 'evt-started', kind: 'assignment_started', lifecycle_status: 'running' }),
            liveEvent({ event_id: 'evt-completed', kind: 'assignment_completed', lifecycle_status: 'completed' }),
          ]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    expect(screen.getByText('Started')).toBeInTheDocument();
    expect(screen.queryByText('Running')).not.toBeInTheDocument();
    expect(screen.getByText('Completed')).toBeInTheDocument();
  });

  it('colors role labels by event outcome instead of only lifecycle status', () => {
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[
            liveEvent({ event_id: 'evt-starting', kind: 'assignment_started', lifecycle_status: 'running' }),
            liveEvent({ event_id: 'evt-succeeding', kind: 'assignment_completed', lifecycle_status: 'completed' }),
            liveEvent({ event_id: 'evt-failing', kind: 'assignment_failed', lifecycle_status: 'completed' }),
          ]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    expect(screen.getByRole('link', { name: 'Assignment Started' }).closest('tr')?.querySelector('.stream-role')).toHaveClass('status-running');
    expect(screen.getByRole('link', { name: 'Assignment Completed' }).closest('tr')?.querySelector('.stream-role')).toHaveClass('status-completed');
    expect(screen.getByRole('link', { name: 'Assignment Failed' }).closest('tr')?.querySelector('.stream-role')).toHaveClass('status-failed');
  });

  it('renders each event and assignment detail as its own table column', () => {
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[
            liveEvent({
              event_id: 'evt-complete',
              kind: 'assignment_completed',
              assignment_id: 'assignment-1',
              task_id: 'security-policy-block-run',
              lifecycle_status: 'completed',
              metric_delta: {
                pass: { value: 1 },
                latency_ms: { value: 780 },
              },
            }),
          ]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    expect(screen.getByRole('columnheader', { name: 'Event' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Status' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Category' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Task' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Pass' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Pass Rate' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Latency' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'security-policy-block-run' })).toBeInTheDocument();
    expect(screen.getByText('Pass', { selector: '.stream-metric-value' })).toBeInTheDocument();
    expect(screen.getByText('780 ms')).toBeInTheDocument();
  });

  it('renders -- for absent or explicitly unavailable metrics', () => {
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[
            liveEvent({
              event_id: 'evt-platform',
              kind: 'stage_updated',
            }),
            liveEvent({
              event_id: 'evt-partial',
              kind: 'assignment_completed',
              assignment_id: 'assignment-1',
              metric_delta: {
                pass: { value: 1 },
                latency_ms: { unavailable_reason: 'no_scored_calls' },
              },
            }),
          ]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    const platformRow = screen.getByRole('link', { name: 'Stage Updated' }).closest('tr');
    expect(platformRow).not.toBeNull();
    for (const cell of platformRow?.querySelectorAll('.stream-metric-value') ?? []) {
      expect(cell).toHaveTextContent('--');
    }

    const assignmentRow = screen.getByRole('link', { name: 'Assignment Completed' }).closest('tr');
    expect(assignmentRow).not.toBeNull();
    const metricCells = Array.from(assignmentRow?.querySelectorAll('.stream-metric-value') ?? []).map(
      (cell) => cell.textContent,
    );
    expect(metricCells).toContain('Pass');
    expect(metricCells.filter((text) => text === '--').length).toBeGreaterThan(0);
    expect(metricCells).not.toContain('Unavailable');
  });

  it('hides thinking and cache token columns from the live stream', () => {
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[liveEvent({ event_id: 'evt-resource-metrics', kind: 'assignment_completed' })]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    expect(screen.queryByRole('columnheader', { name: 'Thinking tokens' })).not.toBeInTheDocument();
    expect(screen.queryByRole('columnheader', { name: 'Cache tokens' })).not.toBeInTheDocument();
  });

  it('sorts the Event and Assignment details columns independently', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[
            liveEvent({
              event_id: 'evt-zeta',
              kind: 'assignment_started',
              assignment_id: 'assignment-zeta',
              task_id: 'task-zeta',
              stage_label: 'Zeta stage',
            }),
            liveEvent({
              event_id: 'evt-alpha',
              kind: 'assignment_completed',
              assignment_id: 'assignment-alpha',
              task_id: 'task-alpha',
              stage_label: 'Alpha stage',
            }),
          ]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    const rows = () => screen.getAllByRole('row').slice(1).map((row) => row.textContent ?? '');
    await user.click(screen.getByRole('button', { name: 'Sort by Event' }));
    expect(rows()[0]).toContain('Assignment Completed');
    expect(screen.getByRole('columnheader', { name: 'Event' })).toHaveAttribute('aria-sort', 'ascending');

    await user.click(screen.getByRole('button', { name: 'Sort by Task' }));
    expect(rows()[0]).toContain('task-alpha');
    expect(screen.getByRole('columnheader', { name: 'Task' })).toHaveAttribute('aria-sort', 'ascending');
  });

  it('makes every live stream column sortable', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[
            liveEvent({
              event_id: 'evt-sortable-columns',
              kind: 'assignment_completed',
              variant_id: 'model-sortable',
              role: 'primary',
              completed: 3,
              total: 10,
            }),
          ]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    const labels = [
      'Time',
      'Role',
      'Event',
      'Status',
      'Category',
      'Task',
      'Pass',
      'Task score',
      'Pass Rate',
      'Latency',
      'Input tokens',
      'Output tokens',
      'Retries',
      'Model',
      'Progress',
    ];

    for (const label of labels) {
      const header = screen.getByRole('columnheader', { name: label });
      await user.click(screen.getByRole('button', { name: `Sort by ${label}` }));
      expect(header).toHaveAttribute('aria-sort', 'ascending');
    }
  });

  it('sorts Progress by completion ratio instead of the displayed fraction text', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[
            liveEvent({ event_id: 'evt-progress-15', kind: 'stage_updated', completed: 15, total: 75 }),
            liveEvent({ event_id: 'evt-progress-3', kind: 'stage_updated', completed: 3, total: 75 }),
            liveEvent({ event_id: 'evt-progress-30', kind: 'stage_updated', completed: 30, total: 75 }),
          ]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    await user.click(screen.getByRole('button', { name: 'Sort by Progress' }));

    const progressValues = screen
      .getAllByRole('row')
      .slice(1)
      .map((row) => row.querySelector('.stream-progress')?.textContent);
    expect(progressValues).toEqual(['3/75', '15/75', '30/75']);
  });

  it('shows designated role from the event before model summaries exist', () => {
    render(
      <MemoryRouter>
        <LiveEventStream
          events={[
            liveEvent({
              event_id: 'evt-lite',
              kind: 'stage_updated',
              variant_id: 'gemma2-9b',
              role: 'lite',
              stage_label: 'Stage Updated · Queued · Routing Delegation · Route-Lite-Triage',
            }),
          ]}
          connection="live"
          streamConnection="connected"
        />
      </MemoryRouter>,
    );

    const roleCell = screen.getByText('Lite').closest('.stream-role');
    expect(roleCell).not.toBeNull();
    expect(roleCell).toHaveTextContent('Lite');
    expect(roleCell).not.toHaveTextContent('gemma2-9b');
    expect(roleCell?.querySelector('.status-dot')).not.toBeInTheDocument();
  });

  it('links active evaluation details to the live page without duplicating the event stream', () => {
    const evaluation: EvaluationSummary = {
      schema_version: '1.3.0',
      kind: 'evaluation_summary',
      dataset_id: 'ds-live-a',
      quality_state: 'live_in_progress',
      observed_at: '2026-09-17T08:00:00Z',
      run_id: 'run-a',
      suite_id: 'suite-a',
      arm: 'platform',
      evaluation_unit: 'model',
      model_role_mapping: {},
      lifecycle_state: 'running',
      assignment_total: 30,
      assignment_completed: 0,
      assignment_failed: 0,
      terminal_outcomes: { completed: 0, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
      started_at: '2026-09-17T08:00:00Z',
      verifier_state: 'not_applicable',
      headline_metrics: {},
    };
    const events = Array.from({ length: 30 }, (_, index) =>
      liveEvent({
        event_id: `evt-detail-${index}`,
        kind: 'stage_updated',
        observed_at: `2026-09-17T08:${String(index).padStart(2, '0')}:00Z`,
      }),
    );
    evalStore.loadFixtures([], events);
    evalStore.getState().evaluations.set(recordKey(evaluation.dataset_id, evaluation.run_id), evaluation);

    render(
      <MemoryRouter initialEntries={['/evaluations/ds-live-a/run-a']}>
        <Routes>
          <Route path="/evaluations/:datasetId/:runId" element={<EvaluationDetailView />} />
        </Routes>
      </MemoryRouter>,
    );

    const liveLink = screen.getByRole('link', { name: 'Watch it Live' });
    expect(liveLink).toHaveAttribute('href', '/');
    expect(screen.queryByRole('region', { name: 'Live timeline' })).not.toBeInTheDocument();
    expect(screen.queryByText('Page 1 of 2 (30 events)')).not.toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });
});
