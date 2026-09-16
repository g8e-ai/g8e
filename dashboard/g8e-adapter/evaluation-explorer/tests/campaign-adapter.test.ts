import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import {
  adaptCampaignProjectionEnvelope,
  campaignDatasetId,
  createCampaignAdaptContext,
  isCampaignProjectionEnvelope,
} from '../src/state/campaign-adapter';
import { decodeViewRecord } from '../src/contract/validators';
import { EvalStore } from '../src/state/store';

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '../../../..');
const publicResultVector = JSON.parse(
  readFileSync(join(repoRoot, 'protocol/vectors/eval/public_assignment_result.json'), 'utf8'),
) as { canonical_json: string };

describe('isCampaignProjectionEnvelope', () => {
  it('accepts typed campaign envelopes', () => {
    expect(
      isCampaignProjectionEnvelope({
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: 'run-1:assign-1:lifecycle:queued',
        record: { run_id: 'run-1', assignment_id: 'assign-1' },
      }),
    ).toBe(true);
  });

  it('rejects explorer view records', () => {
    expect(isCampaignProjectionEnvelope({ kind: 'assignment_result', dataset_id: 'ds-live-run-1' })).toBe(false);
  });
});

describe('adaptCampaignProjectionEnvelope', () => {
  it('maps lifecycle queued records to stage updates and run summaries', () => {
    const context = createCampaignAdaptContext();
    const records = adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: 'run-1:assign-1:lifecycle:queued',
        record: {
          assignment_id: 'assign-1',
          run_id: 'run-1',
          scenario_id: 'instruction-exact-format',
          scenario_category: 'EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE',
          lane: 'EVALUATION_LANE_MODEL_ROLE',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
          repetition: 1,
          designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
          variant_id: 'qwen3-4b',
          observed_at: '2026-09-16T14:00:00Z',
        },
      },
      context,
    );

    expect(records.some((record) => record.kind === 'evaluation_summary')).toBe(true);
    const event = records.find((record) => record.kind === 'stage_updated');
    expect(event).toMatchObject({
      dataset_id: campaignDatasetId('run-1'),
      run_id: 'run-1',
      assignment_id: 'assign-1',
      lifecycle_status: 'queued',
      stage_label: expect.stringContaining('instruction-exact-format'),
    });
  });

  it('maps terminal result projections to assignment_result records', () => {
    const context = createCampaignAdaptContext();
    adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: 'run-1:assign-1:lifecycle:running',
        record: {
          assignment_id: 'assign-1',
          run_id: 'run-1',
          scenario_id: 'instruction-exact-format',
          scenario_category: 'EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE',
          lane: 'EVALUATION_LANE_MODEL_ROLE',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING',
          repetition: 1,
          designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
          variant_id: 'qwen3-4b',
          observed_at: '2026-09-16T14:00:01Z',
        },
      },
      context,
    );

    const resultRecord = JSON.parse(publicResultVector.canonical_json) as Record<string, unknown>;
    const records = adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentResultProjection',
        idempotency_key: 'run-1:assign-1:result',
        record: {
          ...resultRecord,
          decomposed_scores: [{ score_id: 'task_score', value: 1 }],
        },
      },
      context,
    );

    const assignment = records.find((record) => record.kind === 'assignment_result');
    expect(assignment).toBeDefined();
    expect(() => decodeViewRecord('assignment_result', assignment)).not.toThrow();
    expect(assignment).toMatchObject({
      dataset_id: campaignDatasetId('run-1'),
      assignment_id: 'assign-1',
      run_id: 'run-1',
      role: 'primary',
      scenario_category: 'instruction_adherence',
      evaluation_unit: 'model',
      terminal_status: 'completed',
      metric_values: expect.objectContaining({
        task_score: { value: 1 },
        pass: { value: 1 },
      }),
      verification_disposition: 'passed',
    });
  });
});

describe('EvalStore campaign ingest', () => {
  it('indexes campaign envelopes without validation errors', () => {
    const store = new EvalStore();
    store.acceptProjection({
      sequence: 1,
      record_type: 'projection',
      record_bytes: JSON.stringify({
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: 'run-live:assign-live:lifecycle:queued',
        record: {
          assignment_id: 'assign-live',
          run_id: 'run-live',
          scenario_id: 'instruction-exact-format',
          scenario_category: 'EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE',
          lane: 'EVALUATION_LANE_MODEL_ROLE',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
          repetition: 1,
          designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
          variant_id: 'qwen3-4b',
          observed_at: '2026-09-16T14:00:00Z',
        },
      }),
    });

    expect(store.getState().errors).toEqual([]);
    expect(store.getEvaluations(campaignDatasetId('run-live')).length).toBe(1);
    expect(store.getEvents('run-live', campaignDatasetId('run-live')).length).toBe(1);
  });
});
