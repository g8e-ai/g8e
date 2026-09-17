// Normalized client store. Page components never parse transport records
// directly; they read normalized slices from this store. The store ingests
// projection records, decodes and validates them, indexes by identity, and
// deduplicates by stable event ID. It uses useSyncExternalStore so React
// re-renders only when a subscribed slice changes.
//
// Index keys are composite (dataset_id + record identity): the real corpus
// publishes the same variant_id in multiple datasets (the registry models
// appear in both the exploratory baseline and the verified public snapshot),
// so bare-identity keys would let one dataset overwrite another.

import { useRef, useSyncExternalStore } from 'react';
import type {
  AssignmentResult,
  CatalogSnapshot,
  EvaluationSummary,
  FeedSnapshot,
  FreshnessState,
  LiveEvent,
  MethodologySnapshot,
  ModelRole,
  ModelSummary,
  ProjectionRecord,
  SnapshotRecord,
  SuiteSummary,
} from '../contract/types';
import { decodeViewRecord, isProjectionRecord, ValidationError } from '../contract/validators';
import {
  deriveFreshness,
  type FeedConnectionState,
  type FeedStatus,
  type StreamConnectionState,
} from '../utils/feed-state';
import {
  adaptCampaignProjectionEnvelope,
  campaignProgressCounts,
  campaignRunIdFromDatasetId,
  createCampaignAdaptContext,
  isCampaignProjectionEnvelope,
  recordCampaignMatrixTotal,
  type CampaignAdaptContext,
} from './campaign-adapter';

/** Composite index key: dataset first so the same identity can coexist. */
export function recordKey(datasetId: string, id: string): string {
  return `${datasetId}${id}`;
}

/** Model summaries are unique per dataset, variant, and designated role. */
export function modelRecordKey(datasetId: string, variantId: string, role: ModelRole): string {
  return recordKey(datasetId, `${variantId}:${role}`);
}

export function modelComparisonId(model: ModelSummary): string {
  return modelRecordKey(model.dataset_id, model.variant_id, model.role);
}

const MODEL_COMPARISON_ID = /^(.+):(primary|assistant|lite)$/;

/** Resolve a model from a comparison id stored in user prefs or URL params. */
export function resolveModelFromComparisonId(
  models: Map<string, ModelSummary>,
  comparisonId: string,
): ModelSummary | undefined {
  const direct = models.get(comparisonId);
  if (direct) return direct;
  const legacy = MODEL_COMPARISON_ID.exec(comparisonId);
  if (legacy?.[1] && legacy[2]) {
    for (const model of models.values()) {
      if (model.variant_id === legacy[1] && model.role === legacy[2]) return model;
    }
  }
  return undefined;
}

export function resolveModelSummary(
  models: Map<string, ModelSummary>,
  datasetId: string,
  modelId: string,
): ModelSummary | undefined {
  const roleMatch = MODEL_COMPARISON_ID.exec(modelId);
  if (roleMatch?.[1] && roleMatch[2]) {
    return models.get(modelRecordKey(datasetId, roleMatch[1], roleMatch[2] as ModelRole));
  }
  let fallback: ModelSummary | undefined;
  for (const model of models.values()) {
    if (model.dataset_id !== datasetId || model.variant_id !== modelId) continue;
    if (model.pass_rate) return model;
    fallback = fallback ?? model;
  }
  return fallback;
}

export interface StoreState {
  connection: FeedConnectionState;
  streamConnection: StreamConnectionState;
  feedStatus: FeedStatus | null;
  sourceId: string | null;
  observedSequence: number;
  currentSnapshot: FeedSnapshot | null;
  pendingSnapshot: FeedSnapshot | null;
  catalogs: Map<string, CatalogSnapshot>;
  models: Map<string, ModelSummary>;
  suites: Map<string, SuiteSummary>;
  evaluations: Map<string, EvaluationSummary>;
  assignments: Map<string, AssignmentResult>;
  methodology: MethodologySnapshot | null;
  events: LiveEvent[];
  eventIds: Set<string>;
  errors: string[];
  lastAcceptedAt: string | undefined;
}

function snapshotPinnedFreshness(freshness: FeedSnapshot['freshness']): FreshnessState | undefined {
  return freshness === 'intentionally_stopped' || freshness === 'safety_stopped' ? freshness : undefined;
}

function withFeedSnapshot(state: StoreState, snapshot: FeedSnapshot, message: string): FeedStatus {
  const pinnedFreshness = snapshotPinnedFreshness(snapshot.freshness);
  const lastAcceptedAt = state.lastAcceptedAt ?? snapshot.generated_at;
  return {
    connection: state.connection,
    freshness: deriveFreshness(lastAcceptedAt, pinnedFreshness),
    pinnedFreshness,
    highWaterSequence: snapshot.high_water_sequence,
    lastAcceptedAt,
    message,
  };
}

function withAcceptedRecord(state: StoreState): FeedStatus | null {
  if (!state.feedStatus) return null;
  const freshness = deriveFreshness(state.lastAcceptedAt, state.feedStatus.pinnedFreshness);
  return { ...state.feedStatus, freshness, lastAcceptedAt: state.lastAcceptedAt };
}

function emptyState(): StoreState {
  return {
    connection: 'connecting',
    streamConnection: 'disconnected',
    feedStatus: null,
    sourceId: null,
    observedSequence: 0,
    currentSnapshot: null,
    pendingSnapshot: null,
    catalogs: new Map(),
    models: new Map(),
    suites: new Map(),
    evaluations: new Map(),
    assignments: new Map(),
    methodology: null,
    events: [],
    eventIds: new Set(),
    errors: [],
    lastAcceptedAt: undefined,
  };
}

type Listener = () => void;

export class EvalStore {
  private state: StoreState = emptyState();
  private listeners = new Set<Listener>();
  private campaignContext: CampaignAdaptContext = createCampaignAdaptContext();

  getState = (): StoreState => this.state;

  subscribe = (listener: Listener): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  private emit(): void {
    for (const listener of this.listeners) listener();
  }

  private setState(updater: (state: StoreState) => StoreState): void {
    this.state = updater(this.state);
    this.emit();
  }

  setConnection(connection: FeedConnectionState, message?: string): void {
    this.setState((state) => ({
      ...state,
      connection,
      feedStatus: state.feedStatus
        ? { ...state.feedStatus, connection, message: message ?? state.feedStatus.message }
        : null,
    }));
  }

  setStreamConnection(streamConnection: StreamConnectionState): void {
    this.setState((state) => ({ ...state, streamConnection }));
  }

  /** Initialize from a mirror bootstrap. The snapshot is the seal target:
   *  it stays pending until reconciled history reaches its high-water
   *  sequence. Recent projections are indexed immediately for first paint;
   *  history replay re-indexes them idempotently. */
  initBootstrap(
    snapshot: FeedSnapshot,
    recentProjections: ProjectionRecord[],
    proofCount: number,
  ): void {
    this.campaignContext = createCampaignAdaptContext();
    const state = emptyState();
    state.connection = 'live';
    state.sourceId = snapshot.source_id;
    state.observedSequence = 0;
    if (snapshot.high_water_sequence === 0) {
      state.currentSnapshot = snapshot;
    } else {
      state.pendingSnapshot = snapshot;
    }
    state.feedStatus = withFeedSnapshot(state, snapshot, 'Reconciling feed history.');
    state.feedStatus.connection = 'live';
    void proofCount;
    for (const record of recentProjections) {
      this.ingestProjection(state, record);
    }
    this.state = state;
    this.emit();
  }

  acceptSnapshot(snapshot: FeedSnapshot): void {
    if (this.state.sourceId && snapshot.source_id !== this.state.sourceId) {
      this.pushError('public snapshot source changed');
      return;
    }
    const reference = this.state.currentSnapshot ?? this.state.pendingSnapshot;
    if (
      reference &&
      (snapshot.high_water_sequence < reference.high_water_sequence ||
        snapshot.high_water_sequence < this.state.observedSequence)
    ) {
      this.pushError('public snapshot sequence regressed');
      return;
    }
    if (
      reference &&
      snapshot.high_water_sequence === reference.high_water_sequence &&
      snapshot.feed_chain_hash !== reference.feed_chain_hash
    ) {
      this.pushError('public snapshot equivocation');
      return;
    }
    if (snapshot.high_water_sequence > this.state.observedSequence) {
      this.setState((state) => ({
        ...state,
        pendingSnapshot: snapshot,
        feedStatus: state.feedStatus
          ? { ...withFeedSnapshot(state, snapshot, 'Reconciling feed history.'), connection: state.connection }
          : null,
      }));
      return;
    }
    this.setState((state) => ({
      ...state,
      currentSnapshot: snapshot,
      pendingSnapshot: null,
      connection: 'live',
      feedStatus: { ...withFeedSnapshot(state, snapshot, 'Snapshot sealed.'), connection: 'live' },
    }));
  }

  acceptProjection(record: ProjectionRecord): void {
    if (!Number.isSafeInteger(record.sequence) || record.sequence < 1) {
      this.pushError('invalid public feed record');
      return;
    }
    if (record.sequence <= this.state.observedSequence) return;
    if (this.state.observedSequence > 0 && record.sequence !== this.state.observedSequence + 1) {
      this.pushError('public feed sequence gap');
      return;
    }
    // observedSequence === 0: accept whatever sequence the retained feed
    // starts at (the mirror may have pruned early records); strict +1
    // ordering applies to every record after the first.
    const state: StoreState = { ...this.state };
    this.ingestProjection(state, record);
    state.observedSequence = record.sequence;
    state.lastAcceptedAt = new Date().toISOString();
    if (state.pendingSnapshot && state.pendingSnapshot.high_water_sequence === state.observedSequence) {
      const sealed = state.pendingSnapshot;
      state.currentSnapshot = sealed;
      state.pendingSnapshot = null;
      state.connection = 'live';
      state.feedStatus = { ...withFeedSnapshot(state, sealed, 'Snapshot sealed.'), connection: 'live' };
    } else {
      const nextFeedStatus = withAcceptedRecord(state);
      if (nextFeedStatus) state.feedStatus = nextFeedStatus;
    }
    this.state = state;
    this.emit();
  }

  private ingestProjection(state: StoreState, record: ProjectionRecord): void {
    try {
      isProjectionRecord(record);
      if (record.record_type !== 'projection' && record.record_type !== 'event') return;
      const payload = JSON.parse(record.record_bytes);
      if (
        payload.kind === 'evaluation_summary' &&
        payload.native_result === undefined &&
        (payload.model_role_mapping === undefined || payload.model_role_mapping === null)
      ) {
        payload.model_role_mapping = {};
      }
      if (isCampaignProjectionEnvelope(payload)) {
        const adapted = adaptCampaignProjectionEnvelope(payload, this.campaignContext);
        for (const decoded of adapted) {
          this.indexRecord(state, decoded);
        }
        return;
      }
      const decoded = decodeViewRecord(payload.kind, payload);
      this.indexRecord(state, decoded);
    } catch (error) {
      const message = error instanceof ValidationError ? `${error.path}: ${error.message}` : 'invalid public feed record';
      state.errors = [...state.errors, message].slice(-20);
    }
  }

  private indexRecord(state: StoreState, record: SnapshotRecord | LiveEvent): void {
    switch (record.kind) {
      case 'catalog_snapshot':
        state.catalogs.set(record.dataset_id, record);
        if (record.assignment_count > 0) {
          const runId = campaignRunIdFromDatasetId(record.dataset_id);
          if (runId) {
            recordCampaignMatrixTotal(this.campaignContext, runId, record.assignment_count);
            this.refreshRunProgressTotals(state, record.dataset_id, runId);
          }
        }
        break;
      case 'model_summary':
        state.models.set(modelRecordKey(record.dataset_id, record.variant_id, record.role), record);
        break;
      case 'suite_summary':
        state.suites.set(recordKey(record.dataset_id, record.suite_id), record);
        break;
      case 'evaluation_summary':
        state.evaluations.set(recordKey(record.dataset_id, record.run_id), record);
        break;
      case 'assignment_result':
        state.assignments.set(recordKey(record.dataset_id, record.assignment_id), record);
        break;
      case 'methodology_snapshot':
        state.methodology = record;
        break;
      default:
        if ('event_id' in record) {
          const event = record as LiveEvent;
          if (!state.eventIds.has(event.event_id)) {
            state.eventIds.add(event.event_id);
            state.events = [...state.events, event];
            this.applyLiveEvent(state, event);
          }
        }
        break;
    }
  }

  /** Reconcile assignment_total on summaries and live events after the matrix size is known. */
  private refreshRunProgressTotals(state: StoreState, datasetId: string, runId: string): void {
    const progress = this.campaignContext.runTotals.get(runId);
    if (!progress) return;
    const { total } = campaignProgressCounts(progress);
    if (total <= 0) return;

    const evalKey = recordKey(datasetId, runId);
    const existing = state.evaluations.get(evalKey);
    if (existing) {
      state.evaluations.set(evalKey, { ...existing, assignment_total: total });
    }
    state.events = state.events.map((event) =>
      event.run_id === runId && event.dataset_id === datasetId ? { ...event, total } : event,
    );
  }

  /** A committed live event updates its run's summary so detail pages
   *  reflect lifecycle, progress, and metric deltas without a refresh. */
  private applyLiveEvent(state: StoreState, event: LiveEvent): void {
    const key = recordKey(event.dataset_id, event.run_id);
    const existing = state.evaluations.get(key);
    if (!existing) return;
    state.evaluations.set(key, {
      ...existing,
      lifecycle_state: event.lifecycle_status,
      assignment_total: event.total > 0 ? event.total : existing.assignment_total,
      quality_state: event.quality_state,
      headline_metrics: event.metric_delta
        ? { ...existing.headline_metrics, ...event.metric_delta }
        : existing.headline_metrics,
      observed_at: event.observed_at,
    });
  }

  pushError(message: string): void {
    this.setState((state) => ({ ...state, errors: [...state.errors, message].slice(-20) }));
  }

  clearErrors(): void {
    this.setState((state) => ({ ...state, errors: [] }));
  }

  setOffline(message: string): void {
    this.setState((state) => ({
      ...state,
      connection: 'offline',
      streamConnection: 'disconnected',
      feedStatus: state.feedStatus
        ? { ...state.feedStatus, connection: 'offline', freshness: 'source_offline', message }
        : {
            connection: 'offline',
            freshness: 'source_offline',
            highWaterSequence: 0,
            lastAcceptedAt: undefined,
            message,
          },
    }));
  }

  /** Load deterministic fixtures supplied by test code. */
  loadFixtures(snapshotRecords: SnapshotRecord[], liveEvents: LiveEvent[]): void {
    this.campaignContext = createCampaignAdaptContext();
    const state = emptyState();
    state.connection = 'offline';
    state.feedStatus = {
      connection: 'offline',
      freshness: 'source_offline',
      highWaterSequence: 0,
      lastAcceptedAt: undefined,
      message: 'Using explicit design-preview data. The public mirror is not connected.',
    };
    for (const record of snapshotRecords) {
      this.indexRecord(state, record);
    }
    for (const event of liveEvents) {
      if (!state.eventIds.has(event.event_id)) {
        state.eventIds.add(event.event_id);
        state.events = [...state.events, event];
        this.applyLiveEvent(state, event);
      }
    }
    this.state = state;
    this.emit();
  }

  getCatalogs(): CatalogSnapshot[] {
    return Array.from(this.state.catalogs.values());
  }

  getCatalog(datasetId: string): CatalogSnapshot | undefined {
    return this.state.catalogs.get(datasetId);
  }

  getModels(datasetId: string): ModelSummary[] {
    return Array.from(this.state.models.values()).filter((m) => m.dataset_id === datasetId);
  }

  getModel(datasetId: string, variantId: string, role?: ModelRole): ModelSummary | undefined {
    if (role) {
      return this.state.models.get(modelRecordKey(datasetId, variantId, role));
    }
    let fallback: ModelSummary | undefined;
    for (const model of this.state.models.values()) {
      if (model.dataset_id !== datasetId || model.variant_id !== variantId) continue;
      if (model.pass_rate) return model;
      fallback = fallback ?? model;
    }
    return fallback;
  }

  getSuites(datasetId: string): SuiteSummary[] {
    return Array.from(this.state.suites.values()).filter((s) => s.dataset_id === datasetId);
  }

  getEvaluations(datasetId: string): EvaluationSummary[] {
    return Array.from(this.state.evaluations.values()).filter((e) => e.dataset_id === datasetId);
  }

  getEvaluation(datasetId: string, runId: string): EvaluationSummary | undefined {
    return this.state.evaluations.get(recordKey(datasetId, runId));
  }

  getAssignments(runId: string, datasetId?: string): AssignmentResult[] {
    return Array.from(this.state.assignments.values()).filter(
      (a) => a.run_id === runId && (datasetId === undefined || a.dataset_id === datasetId),
    );
  }

  getAssignment(datasetId: string, assignmentId: string): AssignmentResult | undefined {
    return this.state.assignments.get(recordKey(datasetId, assignmentId));
  }

  getEvents(runId?: string, datasetId?: string): LiveEvent[] {
    return this.state.events.filter(
      (e) =>
        (runId === undefined || e.run_id === runId) &&
        (datasetId === undefined || e.dataset_id === datasetId),
    );
  }
}

export const evalStore = new EvalStore();

// React binding hooks. Selectors that derive collections must return a
// stable reference while the underlying records are unchanged; otherwise
// useSyncExternalStore's consistency check re-renders forever.
const UNSET = Symbol('unset');

function defaultEqual<T>(a: T, b: T): boolean {
  if (Object.is(a, b)) return true;
  if (Array.isArray(a) && Array.isArray(b)) {
    return a.length === b.length && a.every((value, index) => Object.is(value, b[index]));
  }
  return false;
}

export function useStoreState<T>(
  selector: (state: StoreState) => T,
  isEqual: (a: T, b: T) => boolean = defaultEqual,
): T {
  const cache = useRef<T | typeof UNSET>(UNSET);
  return useSyncExternalStore(
    evalStore.subscribe,
    () => {
      const next = selector(evalStore.getState());
      if (cache.current !== UNSET && isEqual(cache.current as T, next)) {
        return cache.current as T;
      }
      cache.current = next;
      return next;
    },
    () => selector(evalStore.getState()),
  );
}

export function useFeedStatus(): FeedStatus | null {
  return useSyncExternalStore(
    evalStore.subscribe,
    () => evalStore.getState().feedStatus,
    () => evalStore.getState().feedStatus,
  );
}

export function useConnection(): FeedConnectionState {
  return useSyncExternalStore(
    evalStore.subscribe,
    () => evalStore.getState().connection,
    () => evalStore.getState().connection,
  );
}

export function useStreamConnection(): StreamConnectionState {
  return useSyncExternalStore(
    evalStore.subscribe,
    () => evalStore.getState().streamConnection,
    () => evalStore.getState().streamConnection,
  );
}
