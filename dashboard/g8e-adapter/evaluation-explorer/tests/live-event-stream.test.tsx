import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import type { LiveEvent } from '../src/contract/types';
import { LiveEventStream } from '../src/components/LiveEventStream';

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
  it('shows a reconcile placeholder while history replay is in progress', () => {
    render(
      <MemoryRouter>
        <LiveEventStream events={[]} connection="live" streamConnection="connecting" isReconciling />
      </MemoryRouter>,
    );

    expect(screen.getByTestId('reconcile-placeholder')).toBeInTheDocument();
    expect(screen.getByText('Loading feed history…')).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
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
});
