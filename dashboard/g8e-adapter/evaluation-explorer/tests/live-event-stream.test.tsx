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
  });

  it('renders an active evaluation timeline in the scrollable paginated stream container', () => {
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

    const stream = screen.getByRole('region', { name: 'Live timeline' });
    expect(stream).toHaveClass('stream-panel');
    expect(stream.querySelector('.stream-scroll')).not.toBeNull();
    expect(screen.getByText('Page 1 of 2 (30 events)')).toBeInTheDocument();
    expect(screen.getAllByRole('row')).toHaveLength(26);
  });
});
