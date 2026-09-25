// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ScenarioContextCard } from '../src/components/ScenarioContextCard';

const scenario = {
  scenario_id: 'scenario-1', scenario_version: '1.0.0', category: 'tool_selection' as const,
  public_description: 'Select an approved tool.', grading_method: 'deterministic' as const,
  allowed_tools: ['search'], expected_tools: [], forbidden_tools: [],
  criteria: [{ criterion_id: 'selection', public_label: 'Tool choice', public_description: 'Pick one.', grading_method: 'deterministic' as const, required: true }],
  tool_score_dimensions: [],
};

describe('ScenarioContextCard', () => {
  it('renders scenario prose and criterion chips without a section heading', () => {
    render(<ScenarioContextCard scenario={scenario} />);
    expect(screen.getByText('Select an approved tool.')).toBeInTheDocument();
    expect(screen.getByLabelText('Public criteria')).toHaveTextContent('Tool choice · required');
    expect(screen.getByText('Allowed: search')).toBeInTheDocument();
    expect(screen.queryByRole('heading')).not.toBeInTheDocument();
  });

  it('renders nothing when scenario context is absent', () => {
    const { container } = render(<ScenarioContextCard scenario={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });
});
