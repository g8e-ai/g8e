// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { allFixtureSnapshotRecords, fixtureLiveEvents, FIXTURE_DATASET_IDS } from '../src/fixtures/fixtures';
import { evalStore } from '../src/state/store';
import { ModelsView } from '../src/views/ModelsView';

describe('ModelsView', () => {
  beforeEach(() => {
    evalStore.loadFixtures(allFixtureSnapshotRecords, fixtureLiveEvents);
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
});
