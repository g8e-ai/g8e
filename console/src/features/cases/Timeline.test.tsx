// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { TimelineItem } from '../../lib/timeline';
import { Timeline } from './Timeline';

const tool = (over: Partial<Extract<TimelineItem, { kind: 'tool' }>>): TimelineItem => ({
  kind: 'tool',
  key: 't:x',
  tool: 'command',
  detail: 'uptime',
  status: 'completed',
  at: 't1',
  ...over,
});

function renderItems(items: TimelineItem[]) {
  return render(<Timeline timeline={{ items, busy: false, phase: null }} onRespond={() => {}} />);
}

describe('Timeline tool items', () => {
  it('collapses completed output behind a details disclosure', () => {
    const { container } = renderItems([tool({ output: 'up 3 days' })]);
    const details = container.querySelector('details.tool') as HTMLDetailsElement;
    expect(details.open).toBe(false);
    expect(screen.getByText('up 3 days')).toBeTruthy();
  });

  it('opens failed output by default', () => {
    const { container } = renderItems([tool({ status: 'failed', error: 'exit 1' })]);
    expect((container.querySelector('details.tool') as HTMLDetailsElement).open).toBe(true);
  });

  it('renders no disclosure when there is nothing to reveal', () => {
    const { container } = renderItems([tool({ status: 'running' })]);
    expect(container.querySelector('details')).toBeNull();
    expect(container.querySelector('.tool-head')).toBeTruthy();
  });
});
