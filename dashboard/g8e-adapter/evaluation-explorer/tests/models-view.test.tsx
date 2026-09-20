// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { allFixtureSnapshotRecords, fixtureLiveEvents, FIXTURE_DATASET_IDS } from '../src/fixtures/fixtures';
import type { ModelSummary } from '../src/contract/types';
import { evalStore } from '../src/state/store';
import { ModelsView } from '../src/views/ModelsView';

describe('ModelsView', () => {
  beforeEach(() => {
    evalStore.loadFixtures(allFixtureSnapshotRecords, fixtureLiveEvents);
  });

  it('renders verifier-passed model summaries with the exploratory verified quality label', () => {
    const source = allFixtureSnapshotRecords.find((record): record is ModelSummary => record.kind === 'model_summary' && Boolean(record.pass_rate));
    expect(source).toBeDefined();
    evalStore.loadFixtures([{ ...source!, quality_state: 'exploratory_verified' }], []);
    render(
      <MemoryRouter>
        <ModelsView />
      </MemoryRouter>,
    );

    expect(screen.getAllByTestId('quality-exploratory_verified').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Exploratory · verifier passed').length).toBeGreaterThan(0);
  });

  it('lists models from every dataset without a dataset selector', () => {
    render(
      <MemoryRouter>
        <ModelsView />
      </MemoryRouter>,
    );

    expect(screen.queryByTestId('dataset-selector')).not.toBeInTheDocument();
    expect(screen.getAllByTitle(FIXTURE_DATASET_IDS.exploratory).length).toBeGreaterThan(0);
    expect(screen.getAllByTitle(FIXTURE_DATASET_IDS.verified).length).toBeGreaterThan(0);
    const gemmaLinks = screen.getAllByRole('link', { name: 'Gemma 4 E4B' });
    expect(gemmaLinks.map((link) => link.getAttribute('href'))).toEqual(
      expect.arrayContaining([
        `/models/${FIXTURE_DATASET_IDS.exploratory}/gemma4-e4b?role=primary`,
        `/models/${FIXTURE_DATASET_IDS.verified}/gemma4-e4b?role=primary`,
      ]),
    );
  });

  it('quality filter includes verified exploratory rows without promoting partial rows', () => {
    const source = allFixtureSnapshotRecords.find((record): record is ModelSummary => record.kind === 'model_summary' && Boolean(record.pass_rate));
    expect(source).toBeDefined();
    evalStore.loadFixtures([
      { ...source!, variant_id: 'verified-model', display_name: 'Verified model', quality_state: 'exploratory_verified' },
      { ...source!, variant_id: 'partial-model', display_name: 'Partial model', quality_state: 'exploratory_partial' },
      { ...source!, variant_id: 'other-dataset-model', display_name: 'Other dataset model', dataset_id: FIXTURE_DATASET_IDS.verified, quality_state: 'exploratory_partial' },
    ], []);

    render(
      <MemoryRouter>
        <ModelsView />
      </MemoryRouter>,
    );

    fireEvent.change(screen.getByRole('combobox', { name: 'Filter by quality state' }), { target: { value: 'exploratory_verified' } });
    expect(screen.getByRole('link', { name: 'Verified model' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Partial model' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Other dataset model' })).not.toBeInTheDocument();
    expect(screen.getByTestId('quality-exploratory_verified')).toHaveTextContent('Exploratory · verifier passed');
  });
});
