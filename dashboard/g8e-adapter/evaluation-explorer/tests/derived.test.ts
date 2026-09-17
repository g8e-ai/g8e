import { describe, expect, it } from 'vitest';
import type { LiveEvent, ModelSummary } from '../src/contract/types';
import { roleLabel, roleLeaderRows, visibleStreamEvents } from '../src/views/derived';

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
});

function div(n: number, d: number): number {
  return n / d;
}
