// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { AssignmentActivitySummary } from '../src/components/AssignmentActivitySummary';

const emptyFamily = { availability: 'observed' as const, records: [] };

describe('AssignmentActivitySummary', () => {
  it('omits empty, unavailable, and not-applicable families', () => {
    render(<AssignmentActivitySummary activity={{
      model_activity: emptyFamily,
      tool_decisions: emptyFamily,
      tool_calls: { availability: 'not_applicable', records: [] },
      policy_decisions: { availability: 'unavailable', unavailable_reason: 'historical_not_captured', records: [] },
      governed_actions: { availability: 'unavailable', unavailable_reason: 'source_not_captured', records: [] },
    }} />);
    expect(screen.queryByText('0 observed')).not.toBeInTheDocument();
    expect(screen.queryByText('Not applicable to this scenario')).not.toBeInTheDocument();
    expect(screen.queryByText('Unavailable: Historical not captured')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'What happened' })).toBeInTheDocument();
  });
});
