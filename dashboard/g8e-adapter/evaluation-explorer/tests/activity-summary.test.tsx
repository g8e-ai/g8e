// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { AssignmentActivitySummary } from '../src/components/AssignmentActivitySummary';

const emptyFamily = { availability: 'observed' as const, records: [] };

describe('AssignmentActivitySummary', () => {
  it('distinguishes observed empty, unavailable, and not-applicable families', () => {
    render(<AssignmentActivitySummary activity={{
      model_activity: emptyFamily,
      tool_decisions: emptyFamily,
      tool_calls: { availability: 'not_applicable', records: [] },
      policy_decisions: { availability: 'unavailable', unavailable_reason: 'historical_not_captured', records: [] },
      governed_actions: { availability: 'unavailable', unavailable_reason: 'source_not_captured', records: [] },
    }} />);
    expect(screen.getAllByText('0 observed')).toHaveLength(2);
    expect(screen.getByText('Not applicable to this scenario')).toBeInTheDocument();
    expect(screen.getByText('Unavailable: Historical not captured')).toBeInTheDocument();
  });
});
