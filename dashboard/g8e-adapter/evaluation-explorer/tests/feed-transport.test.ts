// Feed-transport regression tests. These cover the three real mirror wire
// shapes (/bootstrap recent_projections, /history items, /stream SSE),
// the history-backfill boot order, and cross-dataset record keying. They
// guard against the integration-breaking defects found in the Worker 3
// review pass: wire-format mismatch, history backfill never running, and
// cross-dataset collisions from bare-identity keys.

import { describe, it, expect, beforeEach } from 'vitest';
import {
  normalizeHistoryItem,
  normalizeRecentProjection,
  normalizeStreamRecord,
} from '../src/state/feed';
import { campaignDatasetId } from '../src/state/campaign-adapter';
import { EvalStore, recordKey } from '../src/state/store';
import {
  fixtureCatalogExploratory,
  fixtureCatalogVerified,
  fixtureModelSummaries,
  fixtureVerifiedModelSummaries,
} from '../src/fixtures/fixtures';
import type { FeedSnapshot, ProjectionRecord } from '../src/contract/types';

function snapshotRecord(record: object, sequence: number, recordType: ProjectionRecord['record_type'] = 'projection'): ProjectionRecord {
  return {
    sequence,
    record_type: recordType,
    record_bytes: JSON.stringify(record),
  };
}

/** Build a /history item: decoded payload + merged `sequence` and `record_type`. */
function historyItem(record: object, sequence: number, recordType: string = 'projection'): object {
  return { ...record, sequence, record_type: recordType };
}

/** Build a /bootstrap recent_projections item: decoded payload + merged `sequence`. */
function bootstrapItem(record: object, sequence: number): object {
  return { ...record, sequence };
}

const nativeEvaluation = {
  schema_version: '1.3.0', kind: 'evaluation_summary', dataset_id: 'native-core-execution-boundary', quality_state: 'verified_public', observed_at: '2026-09-15T23:38:22Z',
  run_id: 'native-run-transport', suite_id: 'core-execution-boundary@1.0.0', arm: 'platform', evaluation_unit: 'system', lifecycle_state: 'completed',
  assignment_total: 1, assignment_completed: 1, assignment_failed: 0, terminal_outcomes: { completed: 1, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
  verifier_state: 'passed', headline_metrics: { pass_rate: { value: 1 } },
  native_result: {
    active_posture: 'doctrine', lane: 'platform', summary_status: 'pass', summary: '1/1 required invariants passed', required_verdict_count: 1, passed_verdict_count: 1,
    verification_valid: true, verification_failure_count: 0,
    scenarios: [{ scenario_id: 'allowed-execution-occurs-once', scenario_version: '1.0.0', status: 'completed', verdicts: [{ assertion_id: 'allowed-effect-count', assertion_version: '1.0.0', status: 'pass' }] }],
    metrics: [{ metric_id: 'required-verdict-pass-rate', metric_version: '1.0.0', numerator: 1, denominator: 1, value: 1, unit: 'ratio' }],
  },
};

const SNAP: FeedSnapshot = {
  protocol_version: '1.0.0',
  source_id: 'opendevops-local',
  high_water_sequence: 0,
  feed_chain_hash: '0000000000000000000000000000000000000000000000000000000000000000',
  batch_count: 0,
  generated_at: '2026-09-14T08:00:00Z',
  freshness: 'active',
};

describe('normalizeHistoryItem', () => {
  it('strips sequence and record_type and keeps the payload as record_bytes', () => {
    const payload = { ...fixtureCatalogExploratory };
    const item = historyItem(payload, 5, 'projection');
    const record = normalizeHistoryItem(item);
    expect(record.sequence).toBe(5);
    expect(record.record_type).toBe('projection');
    const decoded = JSON.parse(record.record_bytes);
    expect(decoded.kind).toBe('catalog_snapshot');
    expect(decoded.dataset_id).toBe(fixtureCatalogExploratory.dataset_id);
    expect('sequence' in decoded).toBe(false);
    expect('record_type' in decoded).toBe(false);
  });

  it('rejects non-object input', () => {
    expect(() => normalizeHistoryItem('not-an-object')).toThrow();
    expect(() => normalizeHistoryItem(null)).toThrow();
  });

  it('preserves a native evaluation payload through mirror history normalization', () => {
    const record = normalizeHistoryItem(historyItem(nativeEvaluation, 11));
    expect(JSON.parse(record.record_bytes)).toEqual(nativeEvaluation);
    const store = new EvalStore();
    store.acceptProjection(record);
    expect(store.getEvaluation('native-core-execution-boundary', 'native-run-transport')?.native_result?.verification_valid).toBe(true);
  });
});

describe('normalizeRecentProjection', () => {
  it('strips sequence and implies record_type projection', () => {
    const payload = { ...fixtureCatalogExploratory };
    const item = bootstrapItem(payload, 3);
    const record = normalizeRecentProjection(item);
    expect(record.sequence).toBe(3);
    expect(record.record_type).toBe('projection');
    const decoded = JSON.parse(record.record_bytes);
    expect(decoded.kind).toBe('catalog_snapshot');
    expect('sequence' in decoded).toBe(false);
  });

  it('rejects non-object input', () => {
    expect(() => normalizeRecentProjection(42)).toThrow();
  });
});

describe('normalizeStreamRecord', () => {
  it('builds a projection record from SSE id/event/data', () => {
    const data = JSON.stringify(fixtureCatalogExploratory);
    const record = normalizeStreamRecord('projection', '7', data);
    expect(record.sequence).toBe(7);
    expect(record.record_type).toBe('projection');
    expect(JSON.parse(record.record_bytes).kind).toBe('catalog_snapshot');
  });

  it('preserves event record types', () => {
    const eventPayload = { ...fixtureCatalogExploratory, kind: 'evaluation_queued', event_id: 'evt-x', run_id: 'r1', lifecycle_status: 'queued', completed: 0, total: 5 };
    const record = normalizeStreamRecord('event', '9', JSON.stringify(eventPayload));
    expect(record.record_type).toBe('event');
    expect(record.sequence).toBe(9);
  });
});

describe('history backfill', () => {
  let store: EvalStore;

  beforeEach(() => {
    store = new EvalStore();
  });

  it('pages history from sequence 0 to snapshot high-water and seals', () => {
    // Two pages of history: sequences 1-2 then 3-4, high-water 4.
    const m0 = fixtureModelSummaries[0]!;
    const m1 = fixtureModelSummaries[1]!;
    const m2 = fixtureModelSummaries[2]!;
    const records = [
      snapshotRecord(fixtureCatalogExploratory, 1),
      snapshotRecord(m0, 2),
      snapshotRecord(m1, 3),
      snapshotRecord(m2, 4),
    ];
    const snapshot: FeedSnapshot = { ...SNAP, high_water_sequence: 4, batch_count: 1 };

    store.initBootstrap(snapshot, [], 0);
    expect(store.getState().observedSequence).toBe(0);
    expect(store.getState().pendingSnapshot).not.toBeNull();

    for (const record of records) store.acceptProjection(record);

    const state = store.getState();
    expect(state.observedSequence).toBe(4);
    expect(state.currentSnapshot).not.toBeNull();
    expect(state.pendingSnapshot).toBeNull();
    expect(state.catalogs.size).toBe(1);
    expect(state.models.size).toBe(3);
  });

  it('bootstrap recent projections are indexed before history backfill', () => {
    const recent = [snapshotRecord(fixtureCatalogExploratory, 1)];
    const snapshot: FeedSnapshot = { ...SNAP, high_water_sequence: 1, batch_count: 1 };
    store.initBootstrap(snapshot, recent, 0);
    expect(store.getState().catalogs.size).toBe(1);
    expect(store.getState().observedSequence).toBe(0);
  });

  it('does not inflate campaign progress when bootstrap and history overlap', () => {
    const runId = 'run-overlap';
    const datasetId = campaignDatasetId(runId);
    const queuedEnvelope = {
      schema_version: '1.0.0',
      message_type: 'PublicAssignmentLifecycleRecord',
      idempotency_key: `${runId}:assign-1:lifecycle:queued`,
      record: {
        assignment_id: 'assign-1',
        run_id: runId,
        scenario_id: 'instruction-exact-format',
        lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
        observed_at: '2026-09-16T14:00:00Z',
      },
    };
    const resultEnvelope = {
      schema_version: '1.0.0',
      message_type: 'PublicAssignmentResultProjection',
      idempotency_key: `${runId}:assign-1:result`,
      record: {
        assignment_id: 'assign-1',
        run_id: runId,
        scenario_id: 'instruction-exact-format',
        lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED',
        summary_status: 'EVALUATION_VERDICT_STATUS_FAIL',
        completed_at: '2026-09-16T14:00:05Z',
      },
    };
    const recent = [
      snapshotRecord(
        {
          schema_version: '1.3.0',
          kind: 'evaluation_summary',
          dataset_id: datasetId,
          quality_state: 'live_in_progress',
          observed_at: '2026-09-16T14:00:00.000Z',
          source_revision_label: 'g8e-eval-campaign',
          run_id: runId,
          suite_id: 'north-star-25',
          arm: 'homogeneous-model-role',
          evaluation_unit: 'model',
          model_role_mapping: {},
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
        },
        1,
      ),
      snapshotRecord(queuedEnvelope, 2),
      snapshotRecord(resultEnvelope, 3),
    ];
    const snapshot: FeedSnapshot = { ...SNAP, high_water_sequence: 3, batch_count: 1 };

    store.initBootstrap(snapshot, recent, 0);
    store.acceptProjection(snapshotRecord(queuedEnvelope, 2));
    store.acceptProjection(snapshotRecord(resultEnvelope, 3));

    const run = store.getEvaluation(datasetId, runId);
    expect(run).toMatchObject({
      assignment_total: 1,
      assignment_completed: 0,
      assignment_failed: 0,
    });
    const failEvent = store.getEvents(runId, datasetId).find((event) => event.kind === 'assignment_failed');
    expect(failEvent).toMatchObject({ completed: 1, total: 1 });
  });

  it('accepts the first retained sequence even when above 1', () => {
    // The mirror may prune early records; the first retained record is
    // accepted at whatever sequence the feed starts at.
    const snapshot: FeedSnapshot = { ...SNAP, high_water_sequence: 5, batch_count: 1 };
    store.initBootstrap(snapshot, [], 0);
    store.acceptProjection(snapshotRecord(fixtureCatalogExploratory, 3));
    expect(store.getState().observedSequence).toBe(3);
    expect(store.getState().errors.length).toBe(0);
  });

  it('enforces strict contiguity after the first record', () => {
    const snapshot: FeedSnapshot = { ...SNAP, high_water_sequence: 5, batch_count: 1 };
    store.initBootstrap(snapshot, [], 0);
    store.acceptProjection(snapshotRecord(fixtureCatalogExploratory, 3));
    store.acceptProjection(snapshotRecord(fixtureModelSummaries[0]!, 5));
    expect(store.getState().observedSequence).toBe(3);
    expect(store.getState().errors).toContain('public feed sequence gap');
  });

  it('retains the highest feed sequences after bootstrap-then-history ingest', () => {
    const runId = 'run-feed-tail';
    const datasetId = campaignDatasetId(runId);
    const lifecycleEnvelope = (assignmentId: string, sequence: number) =>
      snapshotRecord(
        {
          schema_version: '1.0.0',
          message_type: 'PublicAssignmentLifecycleRecord',
          idempotency_key: `${runId}:${assignmentId}:lifecycle:queued`,
          record: {
            assignment_id: assignmentId,
            run_id: runId,
            scenario_id: 'instruction-exact-format',
            lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
            observed_at: `2026-09-17T10:${String(sequence).padStart(2, '0')}:00Z`,
          },
        },
        sequence,
      );

    const recent = Array.from({ length: 50 }, (_, index) => lifecycleEnvelope(`recent-${index}`, 951 + index));
    const snapshot: FeedSnapshot = { ...SNAP, high_water_sequence: 1000, batch_count: 1 };
    store.initBootstrap(snapshot, recent, 0);

    for (let sequence = 1; sequence <= 1000; sequence += 1) {
      store.acceptProjection(lifecycleEnvelope(`hist-${sequence}`, sequence));
    }

    const events = store.getEvents(runId, datasetId);
    expect(events.length).toBe(100);
    expect(events.some((event) => event.event_id === `${runId}:hist-1:lifecycle:queued`)).toBe(false);
    expect(events.some((event) => event.event_id === `${runId}:hist-1000:lifecycle:queued`)).toBe(true);
    expect(events.some((event) => event.event_id === `${runId}:recent-49:lifecycle:queued`)).toBe(true);
  });
});

describe('cross-dataset keying', () => {
  it('the same variant_id in two datasets does not collide', () => {
    const store = new EvalStore();
    const snapshot: FeedSnapshot = { ...SNAP, high_water_sequence: 4, batch_count: 1 };
    store.initBootstrap(snapshot, [], 0);
    // Exploratory gemma4-e4b at sequence 1, verified gemma4-e4b at sequence 2.
    store.acceptProjection(snapshotRecord(fixtureModelSummaries[0]!, 1));
    store.acceptProjection(snapshotRecord(fixtureVerifiedModelSummaries[0]!, 2));
    store.acceptProjection(snapshotRecord(fixtureCatalogExploratory, 3));
    store.acceptProjection(snapshotRecord(fixtureCatalogVerified, 4));

    const explo = store.getModel(fixtureModelSummaries[0]!.dataset_id, 'gemma4-e4b');
    const verified = store.getModel(fixtureVerifiedModelSummaries[0]!.dataset_id, 'gemma4-e4b');
    expect(explo).toBeDefined();
    expect(verified).toBeDefined();
    expect(explo!.dataset_id).toBe(fixtureModelSummaries[0]!.dataset_id);
    expect(verified!.dataset_id).toBe(fixtureVerifiedModelSummaries[0]!.dataset_id);
    expect(explo!.quality_state).toBe('exploratory_partial');
    expect(verified!.quality_state).toBe('verified_public');
  });

  it('recordKey is dataset-first so identical ids coexist', () => {
    const a = recordKey('ds-a', 'gemma4-e4b');
    const b = recordKey('ds-b', 'gemma4-e4b');
    expect(a).not.toBe(b);
  });
});
