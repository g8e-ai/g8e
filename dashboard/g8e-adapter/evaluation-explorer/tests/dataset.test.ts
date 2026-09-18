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

  it('prefers live run over exploratory baseline when both are available', () => {
    evalStore.loadFixtures(
      [
        {
          schema_version: VIEW_SCHEMA_VERSION,
          kind: 'catalog_snapshot',
          dataset_id: 'ds-exploratory-baseline-20260914-r2',
          dataset_kind: 'exploratory_baseline',
          quality_state: 'exploratory_partial',
          observed_at: '2026-09-14T08:00:00Z',
          source_revision_label: 'test',
          title: 'Exploratory baseline',
          description: 'fixture',
          limitations: [],
          model_count: 1,
          evaluated_count: 1,
          suite_count: 1,
          run_count: 1,
          assignment_count: 1,
          provider_request_count: 1,
          provider_token_count: 0,
          retry_count: 0,
          verifier_passed_count: 1,
          verifier_failed_count: 0,
          generated_at: '2026-09-14T08:00:00Z',
        },
        {
          schema_version: VIEW_SCHEMA_VERSION,
          kind: 'catalog_snapshot',
          dataset_id: 'ds-live-eval-init-gemma3-1b-1789735715',
          dataset_kind: 'live_run',
          quality_state: 'live_in_progress',
          observed_at: '2026-09-18T12:00:00Z',
          source_revision_label: 'test',
          title: 'Live smoke run',
          description: 'fixture',
          limitations: [],
          model_count: 1,
          evaluated_count: 1,
          suite_count: 1,
          run_count: 1,
          assignment_count: 75,
          provider_request_count: 0,
          provider_token_count: 0,
          retry_count: 0,
          verifier_passed_count: 0,
          verifier_failed_count: 0,
          generated_at: '2026-09-18T12:00:00Z',
        },
      ],
      [],
    );

    const { result } = renderHook(() => useActiveDatasetId(undefined));
    expect(result.current).toBe('ds-live-eval-init-gemma3-1b-1789735715');
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
