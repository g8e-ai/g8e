import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import {
  adaptCampaignProjectionEnvelope,
  campaignDatasetId,
  campaignProgressCounts,
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
      completed: 0,
      total: 1,
      stage_label: expect.stringContaining('instruction-exact-format'),
    });
    const summary = records.find((record) => record.kind === 'evaluation_summary');
    expect(summary).toMatchObject({
      assignment_total: 1,
      assignment_completed: 0,
      assignment_failed: 0,
    });
  });

  it('tracks terminal progress separately from passing verdicts', () => {
    const context = createCampaignAdaptContext();
    for (const assignmentId of ['assign-1', 'assign-2', 'assign-3', 'assign-4']) {
      adaptCampaignProjectionEnvelope(
        {
          schema_version: '1.0.0',
          message_type: 'PublicAssignmentLifecycleRecord',
          idempotency_key: `run-1:${assignmentId}:lifecycle:queued`,
          record: {
            assignment_id: assignmentId,
            run_id: 'run-1',
            scenario_id: 'instruction-exact-format',
            lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
            observed_at: '2026-09-16T14:00:00Z',
          },
        },
        context,
      );
    }
    expect(campaignProgressCounts(context.runTotals.get('run-1')!)).toEqual({ completed: 0, total: 4 });

    const failRecords = adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentResultProjection',
        idempotency_key: 'run-1:assign-1:result',
        record: {
          assignment_id: 'assign-1',
          run_id: 'run-1',
          scenario_id: 'instruction-exact-format',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED',
          summary_status: 'EVALUATION_VERDICT_STATUS_FAIL',
          completed_at: '2026-09-16T14:00:05Z',
        },
      },
      context,
    );
    const failEvent = failRecords.find((record) => record.kind === 'assignment_failed');
    expect(failEvent).toMatchObject({ completed: 1, total: 4 });
    expect(context.runTotals.get('run-1')).toMatchObject({
      scheduled: 4,
      terminal: 1,
      passed: 0,
      failed: 1,
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

  it('maps published benchmark_observations onto assignment_result records', () => {
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
          benchmark_observations: {
            timing: {
              generation_ms: { value: 1200 },
            },
            gpu: {
              vram_peak_bytes: { value: 8192 },
              utilization_percent: { value: 77.5 },
            },
            unavailable_reasons: ['provider_boundary_observation_missing:attempt-2'],
          },
        },
      },
      context,
    );

    const assignment = records.find((record) => record.kind === 'assignment_result');
    expect(assignment?.benchmark_observations).toMatchObject({
      timing: { generation_ms: { value: 1200 } },
      gpu: {
        vram_peak_bytes: { value: 8192 },
        utilization_percent: { value: 77.5 },
      },
      unavailable_reasons: ['provider_boundary_observation_missing:attempt-2'],
    });
    expect(() => decodeViewRecord('assignment_result', assignment)).not.toThrow();
  });
});

describe('EvalStore campaign ingest', () => {
  it('indexes model summaries separately for each designated role', () => {
    const store = new EvalStore();
    const runId = 'run-role-matrix';
    const datasetId = campaignDatasetId(runId);
    const enqueue = (assignmentId: string, role: string) => {
      store.acceptProjection({
        sequence: store.getState().observedSequence + 1,
        record_type: 'projection',
        record_bytes: JSON.stringify({
          schema_version: '1.0.0',
          message_type: 'PublicAssignmentLifecycleRecord',
          idempotency_key: `${runId}:${assignmentId}:lifecycle:queued`,
          record: {
            assignment_id: assignmentId,
            run_id: runId,
            scenario_id: 'instruction-exact-format',
            lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
            designated_role: role,
            variant_id: 'qwen3-4b',
            observed_at: '2026-09-16T14:00:00Z',
          },
        }),
      });
    };
    enqueue('assign-primary', 'MODEL_CAMPAIGN_ROLE_PRIMARY');
    enqueue('assign-assistant', 'MODEL_CAMPAIGN_ROLE_ASSISTANT');
    enqueue('assign-lite', 'MODEL_CAMPAIGN_ROLE_LITE');

    const models = store.getModels(datasetId).filter((model) => model.variant_id === 'qwen3-4b');
    expect(models.map((model) => model.role).sort()).toEqual(['assistant', 'lite', 'primary']);
  });

  it('materializes catalog and model aggregates from campaign envelopes', () => {
    const context = createCampaignAdaptContext();
    adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: 'run-1:assign-1:lifecycle:queued',
        record: {
          assignment_id: 'assign-1',
          run_id: 'run-1',
          scenario_id: 'instruction-exact-format',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
          designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
          variant_id: 'qwen3-4b',
          observed_at: '2026-09-16T14:00:00Z',
        },
      },
      context,
    );

    const resultRecords = adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentResultProjection',
        idempotency_key: 'run-1:assign-1:result',
        record: {
          assignment_id: 'assign-1',
          run_id: 'run-1',
          scenario_id: 'instruction-exact-format',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED',
          summary_status: 'EVALUATION_VERDICT_STATUS_PASS',
          designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
          variant_id: 'qwen3-4b',
          completed_at: '2026-09-16T14:00:05Z',
          decomposed_scores: [{ score_id: 'task_score', value: 1 }],
        },
      },
      context,
    );

    const catalog = resultRecords.find((record) => record.kind === 'catalog_snapshot');
    const model = resultRecords.find((record) => record.kind === 'model_summary');
    const methodology = resultRecords.find((record) => record.kind === 'methodology_snapshot');
    expect(catalog).toMatchObject({
      dataset_kind: 'live_run',
      assignment_count: 1,
      model_count: 1,
      evaluated_count: 1,
    });
    expect(model).toMatchObject({
      variant_id: 'qwen3-4b',
      role: 'primary',
      inventory_only: false,
      pass_rate: { estimate: 1, denominator: 1 },
    });
    expect(methodology?.kind).toBe('methodology_snapshot');
    expect(() => decodeViewRecord('catalog_snapshot', catalog)).not.toThrow();
    expect(() => decodeViewRecord('model_summary', model)).not.toThrow();
    expect(() => decodeViewRecord('methodology_snapshot', methodology)).not.toThrow();
  });

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
    expect(store.getCatalog(campaignDatasetId('run-live'))?.dataset_kind).toBe('live_run');
    expect(store.getModels(campaignDatasetId('run-live')).length).toBeGreaterThan(0);
    expect(store.getState().methodology?.kind).toBe('methodology_snapshot');
  });
});
