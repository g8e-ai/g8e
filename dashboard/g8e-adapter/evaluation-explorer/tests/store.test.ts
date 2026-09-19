// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { EvalStore } from '../src/state/store';
import { allFixtureSnapshotRecords, fixtureLiveEvents } from '../src/fixtures/fixtures';
import { LIVE_EVENT_STORE_LIMIT } from '../src/constants';
import type { LiveEvent } from '../src/contract/types';

function loadFixtures(store: EvalStore): void {
  store.loadFixtures(allFixtureSnapshotRecords, fixtureLiveEvents);
}

describe('EvalStore', () => {
  let store: EvalStore;

  beforeEach(() => {
    store = new EvalStore();
  });

  it('starts empty with connecting state', () => {
    const state = store.getState();
    expect(state.connection).toBe('connecting');
    expect(state.streamConnection).toBe('disconnected');
    expect(state.catalogs.size).toBe(0);
    expect(state.models.size).toBe(0);
  });

  it('loadFixtures populates all records', () => {
    loadFixtures(store);
    const state = store.getState();
    expect(state.catalogs.size).toBeGreaterThan(0);
    expect(state.models.size).toBeGreaterThan(0);
    expect(state.suites.size).toBeGreaterThan(0);
    expect(state.evaluations.size).toBeGreaterThan(0);
    expect(state.methodology).not.toBeNull();
    expect(state.events.length).toBe(fixtureLiveEvents.length);
  });

  it('deduplicates live events by event_id', () => {
    loadFixtures(store);
    const initialCount = store.getState().events.length;
    // Re-ingest the same events through indexRecord by loading fixtures again
    loadFixtures(store);
    expect(store.getState().events.length).toBe(initialCount);
  });

  it('retains only the most recent live events by ingest order', () => {
    const extra: LiveEvent[] = Array.from({ length: LIVE_EVENT_STORE_LIMIT + 5 }, (_, index) => ({
      schema_version: '1.3.0',
      kind: 'stage_updated',
      dataset_id: 'ds-live-a',
      quality_state: 'live_in_progress',
      observed_at: `2026-09-17T10:${String(index).padStart(2, '0')}:00Z`,
      run_id: 'run-retention-test',
      event_id: `evt-retention-${index}`,
      lifecycle_status: 'running',
      completed: 0,
      total: 75,
    }));
    store.loadFixtures([], extra);
    expect(store.getState().events.length).toBe(LIVE_EVENT_STORE_LIMIT);
    expect(store.getState().events.some((event) => event.event_id === 'evt-retention-0')).toBe(false);
    expect(store.getState().events.some((event) => event.event_id === `evt-retention-${LIVE_EVENT_STORE_LIMIT + 4}`)).toBe(true);
  });

  it('keeps newly ingested live events even when observed_at is older than retained rows', () => {
    const runId = 'run-retention-ingest';
    const datasetId = `ds-live-${runId}`;
    const completions: LiveEvent[] = Array.from({ length: LIVE_EVENT_STORE_LIMIT }, (_, index) => ({
      schema_version: '1.3.0',
      kind: 'assignment_completed',
      dataset_id: datasetId,
      quality_state: 'live_in_progress',
      observed_at: `2026-09-17T12:${String(index).padStart(2, '0')}:00Z`,
      run_id: runId,
      event_id: `${runId}:assign-${index}:result:event`,
      assignment_id: `assign-${index}`,
      lifecycle_status: 'completed',
      completed: index + 1,
      total: 75,
      feed_sequence: index + 1,
    }));
    store.loadFixtures([], completions);
    expect(store.getState().events.length).toBe(LIVE_EVENT_STORE_LIMIT);

    store.acceptProjection({
      sequence: LIVE_EVENT_STORE_LIMIT + 1,
      record_type: 'projection',
      record_bytes: JSON.stringify({
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: `${runId}:assign-live:lifecycle:running`,
        record: {
          assignment_id: 'assign-live',
          run_id: runId,
          scenario_id: 'instruction-exact-format',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING',
          designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
          variant_id: 'qwen3-4b',
          observed_at: '2026-09-17T08:00:00Z',
        },
      }),
    });

    const events = store.getEvents(runId, datasetId);
    expect(events.some((event) => event.event_id === `${runId}:assign-live:lifecycle:running`)).toBe(true);
    expect(events.length).toBe(LIVE_EVENT_STORE_LIMIT);
    expect(events.some((event) => event.event_id === `${runId}:assign-0:result:event`)).toBe(false);
  });

  it('getModels returns models for a specific dataset', () => {
    loadFixtures(store);
    const models = store.getModels('ds-exploratory-baseline-20260914-r2');
    expect(models.length).toBeGreaterThan(0);
    expect(models.every((m) => m.dataset_id === 'ds-exploratory-baseline-20260914-r2')).toBe(true);
  });

  it('getModel returns a model by variant_id', () => {
    loadFixtures(store);
    const model = store.getModel('ds-exploratory-baseline-20260914-r2', 'gemma4-e4b');
    expect(model).toBeDefined();
    expect(model?.display_name).toBe('Gemma 4 E4B');
  });

  it('getEvaluations returns evaluations for a specific dataset', () => {
    loadFixtures(store);
    const evals = store.getEvaluations('ds-exploratory-baseline-20260914-r2');
    expect(evals.length).toBe(9);
  });

  it('getEvents filters by run_id', () => {
    loadFixtures(store);
    const events = store.getEvents('run-live-demo-20260914');
    expect(events.length).toBeGreaterThan(0);
    expect(events.every((e) => e.run_id === 'run-live-demo-20260914')).toBe(true);
  });

  it('setOffline sets connection to offline with a message', () => {
    store.setOffline('mirror unreachable');
    expect(store.getState().connection).toBe('offline');
    expect(store.getState().streamConnection).toBe('disconnected');
    expect(store.getState().feedStatus?.message).toBe('mirror unreachable');
    expect(store.getState().feedStatus?.freshness).toBe('source_offline');
  });

  it('batches projection updates until endBatch', () => {
    const listener = vi.fn();
    store.subscribe(listener);
    store.initBootstrap(
      {
        protocol_version: '1.0.0',
        source_id: 'opendevops-local',
        high_water_sequence: 2,
        feed_chain_hash: '0'.repeat(64),
        batch_count: 0,
        generated_at: '2026-09-14T08:00:00Z',
        freshness: 'active',
      },
      [],
      0,
    );
    listener.mockClear();

    const catalogBytes = JSON.stringify({
      schema_version: '1.3.0',
      kind: 'catalog_snapshot',
      dataset_id: 'ds-test',
      quality_state: 'live_in_progress',
      observed_at: '2026-09-14T08:00:00Z',
      dataset_kind: 'live_run',
      title: 'Test',
      description: 'Test',
      limitations: [],
      model_count: 1,
      evaluated_count: 0,
      suite_count: 1,
      run_count: 1,
      assignment_count: 1,
      provider_request_count: 0,
      provider_token_count: 0,
      retry_count: 0,
      verifier_passed_count: 0,
      verifier_failed_count: 0,
      generated_at: '2026-09-14T08:00:00Z',
    });

    store.beginBatch();
    store.acceptProjection({ sequence: 1, record_type: 'projection', record_bytes: catalogBytes });
    store.acceptProjection({ sequence: 2, record_type: 'projection', record_bytes: catalogBytes });
    expect(listener).not.toHaveBeenCalled();
    expect(store.getState().observedSequence).toBe(2);
    store.endBatch();
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it('reports reconciliation while the bootstrap snapshot is pending', () => {
    store.initBootstrap(
      {
        protocol_version: '1.0.0',
        source_id: 'opendevops-local',
        high_water_sequence: 3,
        feed_chain_hash: '0'.repeat(64),
        batch_count: 0,
        generated_at: '2026-09-14T08:00:00Z',
        freshness: 'active',
      },
      [],
      0,
    );
    expect(store.isReconciling()).toBe(true);
    store.beginBatch();
    store.acceptProjection({
      sequence: 1,
      record_type: 'projection',
      record_bytes: JSON.stringify({
        schema_version: '1.3.0',
        kind: 'catalog_snapshot',
        dataset_id: 'ds-test',
        quality_state: 'live_in_progress',
        observed_at: '2026-09-14T08:00:00Z',
        dataset_kind: 'live_run',
        title: 'Test',
        description: 'Test',
        limitations: [],
        model_count: 1,
        evaluated_count: 0,
        suite_count: 1,
        run_count: 1,
        assignment_count: 1,
        provider_request_count: 0,
        provider_token_count: 0,
        retry_count: 0,
        verifier_passed_count: 0,
        verifier_failed_count: 0,
        generated_at: '2026-09-14T08:00:00Z',
      }),
    });
    store.acceptProjection({
      sequence: 2,
      record_type: 'projection',
      record_bytes: JSON.stringify({
        schema_version: '1.3.0',
        kind: 'catalog_snapshot',
        dataset_id: 'ds-test',
        quality_state: 'live_in_progress',
        observed_at: '2026-09-14T08:00:00Z',
        dataset_kind: 'live_run',
        title: 'Test',
        description: 'Test',
        limitations: [],
        model_count: 1,
        evaluated_count: 1,
        suite_count: 1,
        run_count: 1,
        assignment_count: 1,
        provider_request_count: 0,
        provider_token_count: 0,
        retry_count: 0,
        verifier_passed_count: 0,
        verifier_failed_count: 0,
        generated_at: '2026-09-14T08:00:00Z',
      }),
    });
    store.acceptProjection({
      sequence: 3,
      record_type: 'projection',
      record_bytes: JSON.stringify({
        schema_version: '1.3.0',
        kind: 'catalog_snapshot',
        dataset_id: 'ds-test',
        quality_state: 'live_in_progress',
        observed_at: '2026-09-14T08:00:00Z',
        dataset_kind: 'live_run',
        title: 'Test',
        description: 'Test',
        limitations: [],
        model_count: 1,
        evaluated_count: 1,
        suite_count: 1,
        run_count: 1,
        assignment_count: 1,
        provider_request_count: 0,
        provider_token_count: 0,
        retry_count: 0,
        verifier_passed_count: 0,
        verifier_failed_count: 0,
        generated_at: '2026-09-14T08:00:00Z',
      }),
    });
    store.endBatch();
    expect(store.isReconciling()).toBe(false);
    expect(store.getState().pendingSnapshot).toBeNull();
  });

  it('acceptProjection refreshes freshness from the last accepted timestamp', () => {
    const snapshot = {
      protocol_version: '1.0.0',
      source_id: 'opendevops-local',
      high_water_sequence: 0,
      feed_chain_hash: '0'.repeat(64),
      batch_count: 0,
      generated_at: '2026-09-14T08:00:00Z',
      freshness: 'stale' as const,
    };
    store.initBootstrap(snapshot, [], 0);
    store.setStreamConnection('connected');
    store.acceptProjection({
      sequence: 1,
      record_type: 'projection',
      record_bytes: JSON.stringify({
        schema_version: '1.3.0',
        kind: 'catalog_snapshot',
        dataset_id: 'ds-test',
        quality_state: 'live_in_progress',
        observed_at: new Date().toISOString(),
        dataset_kind: 'live_run',
        title: 'Test',
        description: 'Test',
        limitations: [],
        model_count: 1,
        evaluated_count: 0,
        suite_count: 1,
        run_count: 1,
        assignment_count: 1,
        provider_request_count: 0,
        provider_token_count: 0,
        retry_count: 0,
        verifier_passed_count: 0,
        verifier_failed_count: 0,
        generated_at: new Date().toISOString(),
      }),
    });
    expect(store.getState().feedStatus?.freshness).toBe('active');
  });

  it('pushError appends errors without losing prior errors', () => {
    store.pushError('first error');
    store.pushError('second error');
    const errors = store.getState().errors;
    expect(errors).toContain('first error');
    expect(errors).toContain('second error');
  });

  it('clearErrors empties the error list', () => {
    store.pushError('a');
    store.pushError('b');
    expect(store.getState().errors.length).toBe(2);
    store.clearErrors();
    expect(store.getState().errors.length).toBe(0);
  });

  it('all fixture records are valid and indexed', () => {
    loadFixtures(store);
    const state = store.getState();
    // Exploratory: 9 evaluated + 22 unevaluated + 1 inventory = 32; verified: 3
    expect(state.models.size).toBe(35);
    // Exploratory: 9 suites; verified: 1
    expect(state.suites.size).toBe(10);
    // Exploratory: 9 evaluations + 1 live; verified: 2
    expect(state.evaluations.size).toBe(12);
    // Exploratory: 2 assignments; verified: 10
    expect(state.assignments.size).toBe(12);
  });

  it('verified dataset renders all record kinds', () => {
    loadFixtures(store);
    const models = store.getModels('ds-verified-public-20260914');
    expect(models.length).toBe(3);
    const suites = store.getSuites('ds-verified-public-20260914');
    expect(suites.length).toBe(1);
    const evals = store.getEvaluations('ds-verified-public-20260914');
    expect(evals.length).toBe(2);
    const assignments = store.getAssignments('run-verified-ifeval-1', 'ds-verified-public-20260914');
    expect(assignments.length).toBe(5);
  });

  it('does not downgrade evaluation_summary quality from assignment live events', () => {
    const runId = 'run-quality-guard';
    const datasetId = `ds-live-${runId}`;
    store.acceptProjection({
      sequence: 1,
      record_type: 'projection',
      record_bytes: JSON.stringify({
        schema_version: '1.3.0',
        kind: 'evaluation_summary',
        dataset_id: datasetId,
        quality_state: 'exploratory_verified',
        observed_at: '2026-09-17T12:00:00Z',
        run_id: runId,
        suite_id: 'north-star-25',
        arm: 'homogeneous-model-role',
        evaluation_unit: 'model',
        model_role_mapping: {},
        lifecycle_state: 'completed',
        assignment_total: 2,
        assignment_completed: 1,
        assignment_failed: 1,
        terminal_outcomes: { completed: 1, model_failed: 1, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
        verifier_state: 'passed',
        headline_metrics: { pass_rate: { value: 0.5 } },
      }),
    });
    store.acceptProjection({
      sequence: 2,
      record_type: 'event',
      record_bytes: JSON.stringify({
        schema_version: '1.3.0',
        kind: 'assignment_completed',
        dataset_id: datasetId,
        quality_state: 'live_in_progress',
        observed_at: '2026-09-17T12:01:00Z',
        event_id: `${runId}:assign-1:result:event`,
        run_id: runId,
        assignment_id: 'assign-1',
        lifecycle_status: 'completed',
        completed: 2,
        total: 2,
      }),
    });
    expect(store.getEvaluation(datasetId, runId)?.quality_state).toBe('exploratory_verified');
  });

  it('indexes native scenario and verdict detail without fake assignments', () => {
    const native = {
      schema_version: '1.3.0', kind: 'evaluation_summary', dataset_id: 'native-core-execution-boundary', quality_state: 'verified_public', observed_at: '2026-09-15T23:38:22Z',
      run_id: 'native-run-1', suite_id: 'core-execution-boundary@1.0.0', arm: 'platform', evaluation_unit: 'system', model_role_mapping: {}, lifecycle_state: 'completed',
      assignment_total: 2, assignment_completed: 2, assignment_failed: 0, terminal_outcomes: { completed: 1, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
      verifier_state: 'passed', headline_metrics: { pass_rate: { value: 1 } },
      native_result: {
        active_posture: 'doctrine', lane: 'platform', summary_status: 'pass', summary: '2/2 required invariants passed', required_verdict_count: 2, passed_verdict_count: 2,
        verification_valid: true, verification_failure_count: 0,
        scenarios: [
          { scenario_id: 'allowed-execution-occurs-once', scenario_version: '1.0.0', status: 'completed', verdicts: [{ assertion_id: 'allowed-effect-count', assertion_version: '1.0.0', status: 'pass' }] },
          { scenario_id: 'prohibited-equivalent-causes-no-additional-effect', scenario_version: '1.0.0', status: 'rejected', verdicts: [{ assertion_id: 'prohibited-no-additional-effect', assertion_version: '1.0.0', status: 'pass' }] },
        ],
        metrics: [{ metric_id: 'required-verdict-pass-rate', metric_version: '1.0.0', numerator: 2, denominator: 2, value: 1, unit: 'ratio' }],
      },
    };
    store.acceptProjection({ sequence: 1, record_type: 'projection', record_bytes: JSON.stringify(native) });
    const indexed = store.getEvaluation('native-core-execution-boundary', 'native-run-1');
    expect(indexed?.native_result?.scenarios).toHaveLength(2);
    expect(indexed?.native_result?.scenarios.flatMap((scenario) => scenario.verdicts)).toHaveLength(2);
    expect(store.getAssignments('native-run-1', 'native-core-execution-boundary')).toHaveLength(0);
  });
});
