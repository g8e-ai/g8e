import { describe, it, expect } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useActiveDatasetId } from '../src/state/dataset';
import { evalStore } from '../src/state/store';
import { VIEW_SCHEMA_VERSION } from '../src/contract/types';

describe('useActiveDatasetId', () => {
  it('honors explicit ds-live route ids without a catalog snapshot', () => {
    const { result } = renderHook(() =>
      useActiveDatasetId('ds-live-north-star-smoke-1789575779'),
    );
    expect(result.current).toBe('ds-live-north-star-smoke-1789575779');
  });

  it('auto-selects the most recent live dataset when no catalog snapshots exist', () => {
    evalStore.loadFixtures([], []);
    evalStore.getState().evaluations.set('ds-live-north-star-smoke-1789575779:north-star-smoke-1789575779', {
      schema_version: VIEW_SCHEMA_VERSION,
      kind: 'evaluation_summary',
      dataset_id: 'ds-live-north-star-smoke-1789575779',
      quality_state: 'live_in_progress',
      observed_at: '2026-09-16T10:00:00Z',
      source_revision_label: 'test',
      run_id: 'north-star-smoke-1789575779',
      suite_id: 'north-star-25',
      arm: 'homogeneous-model-role',
      evaluation_unit: 'model',
      lifecycle_state: 'running',
      assignment_total: 2625,
      assignment_completed: 3,
      assignment_failed: 3,
      terminal_outcomes: {
        completed: 3,
        model_failed: 3,
        grader_failed: 0,
        invalid_evidence: 0,
        stopped: 0,
      },
      verifier_state: 'not_applicable',
      headline_metrics: {},
    });

    const { result } = renderHook(() => useActiveDatasetId(undefined));
    expect(result.current).toBe('ds-live-north-star-smoke-1789575779');
  });

  it('honors route ids that already have evaluation summaries indexed', () => {
    evalStore.loadFixtures([], []);
    evalStore.getState().evaluations.set('ds-custom-run:run-abc', {
      schema_version: VIEW_SCHEMA_VERSION,
      kind: 'evaluation_summary',
      dataset_id: 'ds-custom-run',
      quality_state: 'live_in_progress',
      observed_at: '2026-09-16T00:00:00Z',
      source_revision_label: 'test',
      run_id: 'run-abc',
      suite_id: 'suite-1',
      arm: 'homogeneous-model-role',
      evaluation_unit: 'model',
      lifecycle_state: 'running',
      assignment_total: 1,
      assignment_completed: 0,
      assignment_failed: 0,
      terminal_outcomes: {
        completed: 0,
        model_failed: 0,
        grader_failed: 0,
        invalid_evidence: 0,
        stopped: 0,
      },
      verifier_state: 'not_applicable',
      headline_metrics: {},
    });

    const { result } = renderHook(() => useActiveDatasetId('ds-custom-run'));
    expect(result.current).toBe('ds-custom-run');
  });
});
