import { describe, expect, it } from 'vitest';
import type { CatalogSnapshot, EvaluationSummary, LiveEvent, ModelSummary } from '../src/contract/types';
import { recentCampaignRows, roleLabel, roleLeaderRows, upsertLiveEvent, visibleStreamEvents } from '../src/views/derived';

describe('roleLabel', () => {
  it('maps wire roles to display labels', () => {
    expect(roleLabel('primary')).toBe('Primary');
    expect(roleLabel('assistant')).toBe('Assistant');
    expect(roleLabel('lite')).toBe('Light');
  });
});

function modelSummary(partial: Partial<ModelSummary> & Pick<ModelSummary, 'variant_id' | 'role'>): ModelSummary {
  return {
    schema_version: '1.3.0',
    kind: 'model_summary',
    dataset_id: 'ds-test',
    quality_state: 'live_in_progress',
    observed_at: '2026-09-17T00:00:00Z',
    display_name: partial.display_name ?? partial.variant_id,
    inventory_only: false,
    evaluation_coverage: partial.evaluation_coverage ?? 1,
    ...partial,
  };
}

describe('roleLeaderRows', () => {
  it('returns one leader per role ordered Primary, Assistant, Light', () => {
    const models = [
      modelSummary({
        variant_id: 'qwen3-4b',
        role: 'lite',
        pass_rate: { estimate: 0.82, lower: 0.82, upper: 0.82, denominator: 18 },
        evaluation_coverage: div(18, 25),
      }),
      modelSummary({
        variant_id: 'qwen3-4b',
        role: 'assistant',
        pass_rate: { estimate: 0.77, lower: 0.77, upper: 0.77, denominator: 18 },
        evaluation_coverage: div(18, 25),
      }),
      modelSummary({
        variant_id: 'qwen3-4b',
        role: 'primary',
        pass_rate: { estimate: 0.78, lower: 0.78, upper: 0.78, denominator: 18 },
        evaluation_coverage: div(18, 25),
      }),
    ];
    const rows = roleLeaderRows(models);
    expect(rows.map((row) => row.role)).toEqual(['primary', 'assistant', 'lite']);
    expect(rows.every((row) => row.leader?.model.variant_id === 'qwen3-4b')).toBe(true);
    expect(rows[0]?.leader?.pass_rate).toBe(0.78);
  });

  it('picks the highest pass-rate model per role', () => {
    const models = [
      modelSummary({
        variant_id: 'model-a',
        role: 'primary',
        pass_rate: { estimate: 0.6, lower: 0.6, upper: 0.6, denominator: 10 },
      }),
      modelSummary({
        variant_id: 'model-b',
        role: 'primary',
        pass_rate: { estimate: 0.9, lower: 0.9, upper: 0.9, denominator: 10 },
      }),
    ];
    const primary = roleLeaderRows(models).find((row) => row.role === 'primary');
    expect(primary?.leader?.model.variant_id).toBe('model-b');
  });
});

function liveEvent(partial: Partial<LiveEvent> & Pick<LiveEvent, 'event_id' | 'kind'>): LiveEvent {
  return {
    schema_version: '1.3.0',
    dataset_id: 'ds-live-a',
    quality_state: 'live_in_progress',
    observed_at: '2026-09-17T08:00:00Z',
    run_id: 'run-a',
    lifecycle_status: 'running',
    completed: 0,
    total: 75,
    ...partial,
  };
}

describe('visibleStreamEvents', () => {
  const events = [
    liveEvent({ event_id: 'evt-1', kind: 'evaluation_started', observed_at: '2026-09-17T08:00:00Z' }),
    liveEvent({
      event_id: 'evt-2',
      kind: 'assignment_completed',
      variant_id: 'gemma4-e4b',
      completed: 75,
      total: 75,
      observed_at: '2026-09-17T08:30:00Z',
    }),
    liveEvent({
      event_id: 'evt-3',
      kind: 'evaluation_started',
      dataset_id: 'ds-live-b',
      run_id: 'run-b',
      completed: 1,
      total: 1,
      observed_at: '2026-09-17T08:31:00Z',
    }),
  ];

  it('returns the most recent events newest first', () => {
    const visible = visibleStreamEvents(events, { modelFilter: 'all', kindFilter: 'all' });
    expect(visible.map((event) => event.event_id)).toEqual(['evt-3', 'evt-2', 'evt-1']);
  });

  it('applies model and kind filters before slicing', () => {
    const visible = visibleStreamEvents(events, { modelFilter: 'gemma4-e4b', kindFilter: 'assignment_completed' });
    expect(visible.map((event) => event.event_id)).toEqual(['evt-2']);
  });

  it('shows all matching events when no limit is set', () => {
    const many = Array.from({ length: 30 }, (_, index) =>
      liveEvent({
        event_id: `evt-${index}`,
        kind: 'assignment_completed',
        observed_at: `2026-09-17T08:${String(index).padStart(2, '0')}:00Z`,
      }),
    );
    const visible = visibleStreamEvents(many, { modelFilter: 'all', kindFilter: 'all' });
    expect(visible).toHaveLength(30);
    expect(visible[0]?.event_id).toBe('evt-29');
    expect(visible[29]?.event_id).toBe('evt-0');
  });

  it('limits to the last N events by feed sequence, not array position', () => {
    const batch = Array.from({ length: 120 }, (_, index) =>
      liveEvent({
        event_id: `assign-${String(index).padStart(3, '0')}`,
        kind: 'stage_updated',
        observed_at: '2026-09-17T12:00:00Z',
        feed_sequence: index + 1,
      }),
    );
    const scrambled = [...batch.slice(60), ...batch.slice(0, 60)];
    const visible = visibleStreamEvents(scrambled, { modelFilter: 'all', kindFilter: 'all', limit: 100 });
    expect(visible).toHaveLength(100);
    expect(visible.some((event) => event.event_id === 'assign-000')).toBe(false);
    expect(visible.some((event) => event.event_id === 'assign-119')).toBe(true);
  });

  it('sorts by observed_at even when store order is bootstrap newest-first', () => {
    const bootstrapOrder = [
      liveEvent({ event_id: 'evt-3', kind: 'evaluation_started', observed_at: '2026-09-17T08:31:00Z' }),
      liveEvent({ event_id: 'evt-2', kind: 'assignment_completed', observed_at: '2026-09-17T08:30:00Z' }),
      liveEvent({ event_id: 'evt-1', kind: 'evaluation_started', observed_at: '2026-09-17T08:00:00Z' }),
    ];
    const visible = visibleStreamEvents(bootstrapOrder, { modelFilter: 'all', kindFilter: 'all' });
    expect(visible.map((event) => event.event_id)).toEqual(['evt-3', 'evt-2', 'evt-1']);
  });

  it('collapses bootstrap/history twins at the same observed_at into one result', () => {
    const twins = [
      liveEvent({
        event_id: 'run:a1:lifecycle:COMPLETED',
        kind: 'assignment_completed',
        assignment_id: 'a1',
        variant_id: 'gemma3-4b',
        completed: 1,
        total: 75,
        observed_at: '2026-09-17T19:24:47Z',
      }),
      liveEvent({
        event_id: 'run:a1:result:verified:event',
        kind: 'assignment_completed',
        assignment_id: 'a1',
        variant_id: 'gemma3-4b',
        completed: 75,
        total: 75,
        observed_at: '2026-09-17T19:24:47Z',
        metric_delta: { pass: { value: 1 } },
      }),
    ];
    const visible = visibleStreamEvents(twins, { modelFilter: 'all', kindFilter: 'all' });
    expect(visible).toHaveLength(1);
    expect(visible[0]?.event_id).toBe('run:a1:result:verified:event');
    expect(visible[0]?.completed).toBe(1);
  });

  it('restamps progress from observed_at order so newest complete is N/N', () => {
    const ingestOrder = [
      liveEvent({
        event_id: 'boot:a2',
        kind: 'assignment_completed',
        assignment_id: 'a2',
        completed: 1,
        total: 2,
        observed_at: '2026-09-17T19:24:47Z',
      }),
      liveEvent({
        event_id: 'boot:a1',
        kind: 'assignment_completed',
        assignment_id: 'a1',
        completed: 2,
        total: 2,
        observed_at: '2026-09-17T19:20:00Z',
      }),
      liveEvent({
        event_id: 'hist:a2:start',
        kind: 'assignment_started',
        assignment_id: 'a2',
        completed: 3,
        total: 2,
        observed_at: '2026-09-17T19:24:12Z',
      }),
    ];
    const visible = visibleStreamEvents(ingestOrder, { modelFilter: 'all', kindFilter: 'all' });
    expect(visible.map((event) => `${event.kind}:${event.completed}/${event.total}`)).toEqual([
      'assignment_completed:2/2',
      'assignment_started:2/2',
      'assignment_completed:1/2',
    ]);
  });

  it('upsertLiveEvent replaces bootstrap/history twins instead of growing raw rows', () => {
    const lifecycle = liveEvent({
      event_id: 'run:a1:lifecycle:COMPLETED',
      kind: 'assignment_completed',
      assignment_id: 'a1',
      observed_at: '2026-09-17T19:24:47Z',
    });
    const verified = liveEvent({
      event_id: 'run:a1:result:verified:event',
      kind: 'assignment_completed',
      assignment_id: 'a1',
      observed_at: '2026-09-17T19:24:47Z',
      metric_delta: { pass: { value: 1 } },
    });
    const first = upsertLiveEvent([], lifecycle);
    expect(first.events).toHaveLength(1);
    const second = upsertLiveEvent(first.events, verified);
    expect(second.events).toHaveLength(1);
    expect(second.events[0]?.event_id).toBe('run:a1:result:verified:event');
    expect(second.replacedId).toBe('run:a1:lifecycle:COMPLETED');
  });
});

function div(n: number, d: number): number {
  return n / d;
}

function evaluationSummary(
  partial: Partial<EvaluationSummary> & Pick<EvaluationSummary, 'dataset_id' | 'run_id'>,
): EvaluationSummary {
  return {
    schema_version: '1.3.0',
    kind: 'evaluation_summary',
    quality_state: 'live_in_progress',
    observed_at: '2026-09-17T00:00:00Z',
    suite_id: 'north-star-25',
    arm: 'platform',
    evaluation_unit: 'model',
    model_role_mapping: {},
    lifecycle_state: 'running',
    assignment_total: 75,
    assignment_completed: 10,
    assignment_failed: 0,
    terminal_outcomes: { completed: 10, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    verifier_state: 'not_applicable',
    headline_metrics: {},
    ...partial,
  };
}

describe('recentCampaignRows', () => {
  it('returns cross-dataset campaigns sorted by newest activity', () => {
    const catalogs: CatalogSnapshot[] = [
      {
        schema_version: '1.3.0',
        kind: 'catalog_snapshot',
        dataset_id: 'ds-exploratory',
        dataset_kind: 'exploratory_baseline',
        quality_state: 'exploratory_partial',
        observed_at: '2026-09-14T00:00:00Z',
        title: 'Exploratory baseline (2026-09-14 r2)',
        description: '',
        limitations: [],
        model_count: 1,
        evaluated_count: 1,
        suite_count: 1,
        run_count: 1,
        assignment_count: 1,
        provider_request_count: 0,
        provider_token_count: 0,
        retry_count: 0,
        verifier_passed_count: 1,
        verifier_failed_count: 0,
        generated_at: '2026-09-14T06:00:00Z',
      },
    ];
    const evaluations = [
      evaluationSummary({
        dataset_id: 'ds-live-init-campaign-1789654273',
        run_id: 'init-campaign-1789654273',
        campaign_id: 'init-campaign',
        started_at: '2026-09-17T10:00:00Z',
        observed_at: '2026-09-17T10:00:00Z',
      }),
      evaluationSummary({
        dataset_id: 'ds-live-eval-init-qwen3-4b-1789657337',
        run_id: 'eval-init-qwen3-4b-1789657337',
        campaign_id: 'eval-init-qwen3-4b',
        started_at: '2026-09-17T12:00:00Z',
        observed_at: '2026-09-17T12:00:00Z',
        lifecycle_state: 'completed',
        quality_state: 'verified_public',
      }),
    ];

    const rows = recentCampaignRows(catalogs, evaluations);
    expect(rows.map((row) => row.label)).toEqual([
      'eval-init-qwen3-4b',
      'init-campaign',
      'Exploratory baseline (2026-09-14 r2)',
    ]);
    expect(rows[0]?.detail).toBe('north-star-25');
    expect(rows[0]?.runId).toBe('eval-init-qwen3-4b-1789657337');
  });
});
