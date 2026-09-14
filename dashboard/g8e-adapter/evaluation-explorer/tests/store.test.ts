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
});
