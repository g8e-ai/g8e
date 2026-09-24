// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect } from 'vitest';
import {
  isHeadlineMetricId,
  projectMetricAvailabilityToLiveEvent,
  projectModelRoleInvocationToLiveEvent,
} from '../src/state/public-assignment-event-projection';
import { decodeViewRecord } from '../src/contract/validators';
import { campaignDatasetId } from '../src/state/campaign-adapter';

describe('public assignment event projection', () => {
  describe('model role invocation → stage_updated', () => {
    it('projects disclosure-safe invocation identity without provider-boundary fields', () => {
      const event = projectModelRoleInvocationToLiveEvent({
        run_id: 'run-live-1',
        assignment_id: 'assign-1',
        variant_id: 'qwen3-4b',
        role: 'primary',
        task_id: 'instruction-exact-format',
        observed_at: '2026-09-24T12:00:00.000Z',
        event_id: 'run-live-1:assign-1:invocation:primary:42',
        completed: 2,
        total: 5,
      });

      expect(event.kind).toBe('stage_updated');
      expect(event.dataset_id).toBe(campaignDatasetId('run-live-1'));
      expect(event).toMatchObject({
        assignment_id: 'assign-1',
        variant_id: 'qwen3-4b',
        role: 'primary',
        lifecycle_status: 'running',
        completed: 2,
        total: 5,
      });
      expect(event.stage_label).toContain('model role invoked');
      expect(event.stage_label).toContain('primary');
      expect(event.stage_label).toContain('qwen3-4b');
      expect(event).not.toHaveProperty('served_model_tag');
      expect(event).not.toHaveProperty('backend_name');
      expect(event).not.toHaveProperty('quantization');
      expect(() => decodeViewRecord('stage_updated', event)).not.toThrow();
    });
  });

  describe('metric availability → metric_updated', () => {
    it('projects pass_rate delta from metric availability signal', () => {
      const event = projectMetricAvailabilityToLiveEvent({
        run_id: 'run-live-1',
        assignment_id: 'assign-1',
        variant_id: 'qwen3-4b',
        metric_id: 'pass_rate',
        numerator: 3,
        denominator: 4,
        rate: 0.75,
        observed_at: '2026-09-24T12:00:05.000Z',
        event_id: 'run-live-1:assign-1:metric:pass_rate:42',
        completed: 1,
        total: 5,
      });

      expect(event.kind).toBe('metric_updated');
      expect(event.metric_delta).toEqual({ pass_rate: { value: 0.75 } });
      expect(() => decodeViewRecord('metric_updated', event)).not.toThrow();
    });

    it('marks zero denominator as unavailable rather than zero', () => {
      const event = projectMetricAvailabilityToLiveEvent({
        run_id: 'run-live-1',
        assignment_id: 'assign-1',
        variant_id: 'qwen3-4b',
        metric_id: 'pass_rate',
        numerator: 0,
        denominator: 0,
        observed_at: '2026-09-24T12:00:05.000Z',
        event_id: 'run-live-1:assign-1:metric:pass_rate:43',
        completed: 0,
        total: 5,
      });

      expect(event.metric_delta).toEqual({ pass_rate: { unavailable_reason: 'no_scored_calls' } });
    });

    it('derives rate from numerator and denominator when rate is omitted', () => {
      const event = projectMetricAvailabilityToLiveEvent({
        run_id: 'run-live-1',
        assignment_id: 'assign-1',
        variant_id: 'qwen3-4b',
        metric_id: 'pass_rate',
        numerator: 1,
        denominator: 2,
        observed_at: '2026-09-24T12:00:05.000Z',
        event_id: 'run-live-1:assign-1:metric:pass_rate:44',
        completed: 1,
        total: 5,
      });

      expect(event.metric_delta).toEqual({ pass_rate: { value: 0.5 } });
    });
  });

  describe('headline metric inventory', () => {
    it('recognizes explorer headline metric ids', () => {
      expect(isHeadlineMetricId('pass_rate')).toBe(true);
      expect(isHeadlineMetricId('latency_p50_ms')).toBe(true);
      expect(isHeadlineMetricId('output_throughput_p50_tokens_per_second')).toBe(true);
      expect(isHeadlineMetricId('task_score')).toBe(false);
    });
  });

  describe('publication wiring gate (deferred)', () => {
    it.fails('campaign publication exports native metric_updated events', () => {
      // Wiring milestone: CampaignPublicationCoordinator must emit
      // record_type "event" with kind metric_updated when per-assignment
      // metrics become available mid-run. Today all campaign exports are
      // record_type "projection" only.
      expect(false).toBe(true);
    });

    it.fails('campaign publication exports native stage_updated on model-role invocation', () => {
      // Wiring milestone: invocation signals must publish stage_updated events
      // distinct from queued lifecycle synthesis in campaign-adapter.ts.
      expect(false).toBe(true);
    });
  });
});
