// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ModelComparisonPanel } from '../src/components/ModelComparisonPanel';
import type { EnvironmentSource, ModelSummary } from '../src/contract/types';

function model(variantId: string, datasetId: string): ModelSummary {
  return {
    schema_version: '1.5.0',
    kind: 'model_summary',
    dataset_id: datasetId,
    quality_state: 'exploratory_partial',
    observed_at: '2026-09-17T00:00:00Z',
    variant_id: variantId,
    display_name: variantId,
    role: 'primary',
    inventory_only: false,
    evaluation_coverage: 1,
  };
}

function renderPanel(environmentSource: EnvironmentSource | undefined, models: ModelSummary[]) {
  render(
    <MemoryRouter>
      <ModelComparisonPanel models={models} environmentSource={environmentSource} onClear={() => undefined} />
    </MemoryRouter>,
  );
}

describe('ModelComparisonPanel', () => {
  it('describes a same-dataset comparison without a dataset row', () => {
    renderPanel(undefined, [model('model-a', 'ds-a'), model('model-b', 'ds-a')]);

    expect(screen.getByText(/from the same dataset/)).toBeInTheDocument();
    expect(screen.queryByRole('rowheader', { name: 'Run dataset' })).not.toBeInTheDocument();
  });

  it('attributes each value to its own run and flags a declared environment as a claim', () => {
    renderPanel('declared', [model('model-a', 'ds-a'), model('model-b', 'ds-b')]);

    expect(screen.getByText(/separate runs that share a declared provider environment/)).toBeInTheDocument();
    expect(screen.getByText(/claim, not an observation/)).toBeInTheDocument();
    expect(screen.getByText(/not a superiority claim/)).toBeInTheDocument();
    expect(screen.getByRole('rowheader', { name: 'Run dataset' })).toBeInTheDocument();
    expect(screen.getByTitle('ds-a')).toBeInTheDocument();
    expect(screen.getByTitle('ds-b')).toBeInTheDocument();
  });

  it('states that an observed match is on capacity only', () => {
    renderPanel('observed', [model('model-a', 'ds-a'), model('model-b', 'ds-b')]);

    expect(screen.getByText(/share an observed provider environment/)).toBeInTheDocument();
    expect(screen.getByText(/GPU memory and system RAM capacity only/)).toBeInTheDocument();
    expect(screen.getByText(/processor and GPU model are not observed/)).toBeInTheDocument();
    expect(screen.queryByText(/claim, not an observation/)).not.toBeInTheDocument();
  });
});
