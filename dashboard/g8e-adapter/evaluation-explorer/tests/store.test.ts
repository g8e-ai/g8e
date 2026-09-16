import { describe, it, expect, beforeEach } from 'vitest';
import { EvalStore } from '../src/state/store';
import { allFixtureSnapshotRecords, fixtureLiveEvents } from '../src/fixtures/fixtures';

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
    expect(store.getState().feedStatus?.message).toBe('mirror unreachable');
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
