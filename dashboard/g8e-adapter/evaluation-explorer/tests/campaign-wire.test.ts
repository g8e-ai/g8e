// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import {
  decodeCampaignProjectionEnvelope,
  isCampaignProjectionEnvelope,
  type CampaignResultRecord,
} from '../src/contract/campaign-wire';
import { fixtureCampaignResultEnvelope } from '../src/fixtures/fixtures';

const lifecycleEnvelope = {
  schema_version: '1.0.0',
  message_type: 'PublicAssignmentLifecycleRecord',
  idempotency_key: 'run-1:assign-1:lifecycle:queued',
  record: {
    assignment_id: 'assign-1',
    run_id: 'run-1',
    scenario_id: 'instruction-exact-format',
    scenario_category: 'EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE',
    lane: 'EVALUATION_LANE_MODEL_ROLE',
    designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
    variant_id: 'qwen3-4b',
    lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
    repetition: 1,
    observed_at: '2026-09-20T14:00:00Z',
  },
} as const;

const historicalResultEnvelope = {
  schema_version: '1.0.0',
  message_type: 'PublicAssignmentResultProjection',
  idempotency_key: 'run-1:assign-1:result',
  record: {
    assignment_id: 'assign-1',
    run_id: 'run-1',
    scenario_id: 'instruction-exact-format',
    scenario_category: 'EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE',
    lane: 'EVALUATION_LANE_MODEL_ROLE',
    designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
    variant_id: 'qwen3-4b',
    lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED',
    summary_status: 'EVALUATION_VERDICT_STATUS_PASS',
    result_digest: 'a'.repeat(64),
    verification_status: 'unverified',
    unavailable_metric_reasons: [],
    completed_at: '2026-09-20T14:00:00Z',
  },
} as const;

describe('campaign projection wire contract', () => {
  it('accepts lifecycle envelopes at version 1.0.0', () => {
    expect(decodeCampaignProjectionEnvelope(lifecycleEnvelope)).toMatchObject({
      message_type: 'PublicAssignmentLifecycleRecord',
      schema_version: '1.0.0',
    });
  });

  it('accepts historical result envelopes at version 1.0.0', () => {
    expect(decodeCampaignProjectionEnvelope(historicalResultEnvelope)).toMatchObject({
      message_type: 'PublicAssignmentResultProjection',
      schema_version: '1.0.0',
    });
  });

  it('accepts enriched result envelopes at version 1.1.0', () => {
    expect(decodeCampaignProjectionEnvelope(fixtureCampaignResultEnvelope)).toEqual(fixtureCampaignResultEnvelope);
  });

  it('rejects enriched fields on historical result envelopes', () => {
    const enriched = fixtureCampaignResultEnvelope.record as CampaignResultRecord;
    expect(() =>
      decodeCampaignProjectionEnvelope({
        ...historicalResultEnvelope,
        record: { ...historicalResultEnvelope.record, scenario_summary: enriched.scenario_summary },
      }),
    ).toThrow(/requires campaign envelope 1\.1\.0/);
  });

  it('rejects a lifecycle envelope with the enriched result version', () => {
    expect(() => decodeCampaignProjectionEnvelope({ ...lifecycleEnvelope, schema_version: '1.1.0' })).toThrow();
  });

  it('rejects unknown nested fields instead of dropping them', () => {
    const record = fixtureCampaignResultEnvelope.record as CampaignResultRecord;
    expect(() =>
      decodeCampaignProjectionEnvelope({
        ...fixtureCampaignResultEnvelope,
        record: {
          ...record,
          scenario_summary: { ...record.scenario_summary!, private_prompt: 'restricted' },
        },
      }),
    ).toThrow(/unknown field/);
  });

  it('rejects non-canonical hashes and non-finite metrics', () => {
    expect(() =>
      decodeCampaignProjectionEnvelope({
        ...historicalResultEnvelope,
        record: { ...historicalResultEnvelope.record, result_digest: 'A'.repeat(64) },
      }),
    ).toThrow();
    expect(() =>
      decodeCampaignProjectionEnvelope({
        ...fixtureCampaignResultEnvelope,
        record: {
          ...fixtureCampaignResultEnvelope.record,
          resource_summary: { latency_ms: { value: Number.NaN } },
        },
      }),
    ).toThrow();
  });

  it('rejects decimal uint64 values outside browser-safe range', () => {
    const record = fixtureCampaignResultEnvelope.record as CampaignResultRecord;
    expect(() =>
      decodeCampaignProjectionEnvelope({
        ...fixtureCampaignResultEnvelope,
        record: {
          ...record,
          activity_summary: {
            ...record.activity_summary!,
            model_activity: {
              ...record.activity_summary!.model_activity,
              availability: 'PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED',
              unavailable_reason: undefined,
              records: [
                {
                  model_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
                  variant_id: 'qwen3-4b',
                  usage_availability: 'EVALUATION_USAGE_AVAILABILITY_REPORTED',
                  input_tokens: '9007199254740992',
                  finish_state: 'PUBLIC_FINISH_STATE_STOP',
                  load_state: 'EVALUATION_LOAD_STATE_WARM',
                },
              ],
            },
          },
        },
      }),
    ).toThrow(/browser-safe/);
  });

  it('provides a non-throwing predicate for malformed input', () => {
    expect(isCampaignProjectionEnvelope(fixtureCampaignResultEnvelope)).toBe(true);
    expect(isCampaignProjectionEnvelope({ ...historicalResultEnvelope, record: { ...historicalResultEnvelope.record, result_digest: 'bad' } })).toBe(false);
  });
});
