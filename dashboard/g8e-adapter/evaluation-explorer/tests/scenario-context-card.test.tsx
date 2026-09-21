// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ScenarioContextCard } from '../src/components/ScenarioContextCard';

const scenario = {
  scenario_id: 'scenario-1', scenario_version: '1.0.0', category: 'tool_selection' as const,
  public_description: 'Select an approved tool.', grading_method: 'deterministic' as const,
  allowed_tools: ['search'], expected_tools: [], forbidden_tools: [], criteria: [], tool_score_dimensions: [],
};

describe('ScenarioContextCard', () => {
  it('shows bounded public context and an unavailable legacy state', () => {
    const { rerender } = render(<ScenarioContextCard scenario={scenario} />);
    expect(screen.getByText('Select an approved tool.')).toBeInTheDocument();
    expect(screen.getByText('search')).toBeInTheDocument();
    rerender(<ScenarioContextCard scenario={undefined} />);
    expect(screen.getByText('Scenario context is unavailable for this historical assignment.')).toBeInTheDocument();
  });
});
