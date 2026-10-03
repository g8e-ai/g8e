// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { readFileSync } from 'node:fs';
import { beforeEach, describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { evalStore, recordKey } from '../src/state/store';
import { EvaluationsView } from '../src/views/EvaluationsView';
import type { SnapshotRecord } from '../src/contract/types';
import { VIEW_SCHEMA_VERSION } from '../src/contract/types';

const currentRelease = readFileSync('../VERSION', 'utf8').trim();

function Location() {
  return <output data-testid="location">{useLocation().search}</output>;
}

describe('campaign release selection', () => {
  beforeEach(() => {
    evalStore.loadFixtures([
      { run_id: 'run-current', release: currentRelease, release_basis: 'recorded', source_revision: 'abc123' },
      { run_id: 'run-older', release: 'v2.2.6', release_basis: 'asserted' },
      { run_id: 'run-unknown' },
    ].map((identity) => ({
      schema_version: VIEW_SCHEMA_VERSION, kind: 'evaluation_summary', dataset_id: `ds-live-${identity.run_id}`,
      observed_at: '2026-10-02T12:00:00Z', quality_state: 'live_in_progress', suite_id: 'suite', arm: 'homogeneous-model-role',
      evaluation_unit: 'model', lifecycle_state: 'running', assignment_total: 1, assignment_completed: 0, assignment_failed: 0,
      terminal_outcomes: { completed: 0, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0, provider_failed: 0, execution_failed: 0, escalated: 0 },
      verifier_state: 'not_run', headline_metrics: {}, ...identity,
    })) as SnapshotRecord[], []);
  });

  it('defaults to the build release and keeps older and unknown runs out', () => {
    render(<MemoryRouter><EvaluationsView /></MemoryRouter>);
    expect(screen.getByRole('combobox', { name: 'Filter by release' })).toHaveValue(currentRelease);
    expect(screen.getByRole('link', { name: 'run-current' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'run-older' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'run-unknown' })).not.toBeInTheDocument();
    expect(screen.getByText(`${currentRelease} (recorded)`)).toBeInTheDocument();
  });

  it('persists all releases in the URL and labels assertions and unknown data', () => {
    render(<MemoryRouter><EvaluationsView /><Location /></MemoryRouter>);
    fireEvent.change(screen.getByRole('combobox', { name: 'Filter by release' }), { target: { value: 'all' } });
    expect(screen.getByTestId('location')).toHaveTextContent('release=all');
    expect(screen.getByRole('link', { name: 'run-older' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'run-unknown' })).toBeInTheDocument();
    expect(screen.getByText('v2.2.6 (asserted)')).toBeInTheDocument();
    expect(screen.getByText('Unknown release')).toBeInTheDocument();
  });

  it('honors a release link without changing it to the current release', () => {
    render(<MemoryRouter initialEntries={['/evaluations?release=v2.2.6']}><EvaluationsView /></MemoryRouter>);
    expect(screen.getByRole('link', { name: 'run-older' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'run-current' })).not.toBeInTheDocument();
  });

  it('normalizes quality choices against the selected release', () => {
    const older = evalStore.getState().evaluations.get(recordKey('ds-live-run-older', 'run-older'));
    expect(older).toBeDefined();
    older!.quality_state = 'exploratory_verified';
    render(<MemoryRouter><EvaluationsView /></MemoryRouter>);
    expect(screen.getByRole('link', { name: 'run-current' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Filter by quality' })).toHaveValue('all');
  });
});
