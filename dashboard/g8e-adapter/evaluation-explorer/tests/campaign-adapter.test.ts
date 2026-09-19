// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

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
  liveEventProgressCounts,
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
  it('maps lifecycle queued records to stage updates without synthesizing evaluation summaries', () => {
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
          observed_at: '2026-09-16T14:00:00.000Z',
        },
      },
      context,
    );

    expect(records.some((record) => record.kind === 'evaluation_summary')).toBe(false);
    const event = records.find((record) => record.kind === 'stage_updated');
    expect(event).toMatchObject({
      dataset_id: campaignDatasetId('run-1'),
      run_id: 'run-1',
      assignment_id: 'assign-1',
      role: 'primary',
      lifecycle_status: 'queued',
      completed: 0,
      total: 1,
      stage_label: expect.stringContaining('instruction-exact-format'),
    });
  });

  it('maps result projections to assignment results and live events only', () => {
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
          lane: 'EVALUATION_LANE_MODEL_ROLE',
          observed_at: '2026-09-16T14:00:00.000Z',
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
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED',
          summary_status: 'EVALUATION_VERDICT_STATUS_PASS',
          completed_at: '2026-09-16T14:05:00.000Z',
        },
      },
      context,
    );
    expect(resultRecords.some((record) => record.kind === 'evaluation_summary')).toBe(false);
    expect(resultRecords.some((record) => record.kind === 'assignment_result')).toBe(true);
    expect(resultRecords.some((record) => record.kind === 'assignment_completed')).toBe(true);
  });

  it('does not inflate terminal progress when the same result is adapted twice', () => {
    const context = createCampaignAdaptContext();
    const envelope = {
      schema_version: '1.0.0',
      message_type: 'PublicAssignmentResultProjection' as const,
      idempotency_key: 'run-1:assign-1:result',
      record: {
        assignment_id: 'assign-1',
        run_id: 'run-1',
        scenario_id: 'instruction-exact-format',
        lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED',
        summary_status: 'EVALUATION_VERDICT_STATUS_FAIL',
        completed_at: '2026-09-16T14:00:05Z',
      },
    };

    adaptCampaignProjectionEnvelope(envelope, context);
    const secondPass = adaptCampaignProjectionEnvelope(envelope, context);

    expect(campaignProgressCounts(context.runTotals.get('run-1')!)).toEqual({ completed: 1, total: 1 });
    const failEvent = secondPass.find((record) => record.kind === 'assignment_failed');
    expect(failEvent).toMatchObject({ completed: 1, total: 1 });
  });

  it('reports finished assignments against the full queued matrix size', () => {
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
            observed_at: '2026-09-16T14:00:00.000Z',
          },
        },
        context,
      );
    }

    adaptCampaignProjectionEnvelope(
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

    const runningRecords = adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: 'run-1:assign-2:lifecycle:running',
        record: {
          assignment_id: 'assign-2',
          run_id: 'run-1',
          scenario_id: 'instruction-exact-format',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING',
          observed_at: '2026-09-16T14:00:06Z',
        },
      },
      context,
    );

    const started = runningRecords.find((record) => record.kind === 'assignment_started');
    expect(started).toMatchObject({ completed: 2, total: 4 });
  });

  it('bumps live-event progress when a later model starts after earlier terminals', () => {
    const context = createCampaignAdaptContext();
    const runId = 'run-multi-model';
    const datasetId = campaignDatasetId(runId);

    adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: `${runId}:assign-1:lifecycle:queued`,
        record: {
          assignment_id: 'assign-1',
          run_id: runId,
          scenario_id: 'tool-select-constraints',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED',
          designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
          variant_id: 'qwen30:6b',
          observed_at: '2026-09-17T04:39:37Z',
        },
      },
      context,
    );

    adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentResultProjection',
        idempotency_key: `${runId}:assign-1:result`,
        record: {
          assignment_id: 'assign-1',
          run_id: runId,
          scenario_id: 'tool-select-constraints',
          variant_id: 'qwen30:6b',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED',
          summary_status: 'EVALUATION_VERDICT_STATUS_FAIL',
          completed_at: '2026-09-17T04:40:00Z',
        },
      },
      context,
    );

    const startedRecords = adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: `${runId}:assign-2:lifecycle:running`,
        record: {
          assignment_id: 'assign-2',
          run_id: runId,
          scenario_id: 'tool-select-constraints',
          lifecycle_status: 'EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING',
          designated_role: 'MODEL_CAMPAIGN_ROLE_PRIMARY',
          variant_id: 'gemma3:4b',
          observed_at: '2026-09-17T04:40:12Z',
        },
      },
      context,
    );

    const started = startedRecords.find((record) => record.kind === 'assignment_started');
    expect(started).toMatchObject({
      dataset_id: datasetId,
      variant_id: 'gemma3:4b',
      completed: 2,
      total: 1,
    });
    expect(liveEventProgressCounts(context.runTotals.get(runId)!, 'assignment_started')).toEqual({
      completed: 2,
      total: 1,
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
            observed_at: '2026-09-16T14:00:00.000Z',
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

  it('maps deterministic_pass_rate decomposed scores onto pass and dimension keys', () => {
    const context = createCampaignAdaptContext();
    adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: 'run-1:assign-2:lifecycle:running',
        record: {
          assignment_id: 'assign-2',
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
        idempotency_key: 'run-1:assign-2:result',
        record: {
          ...resultRecord,
          assignment_id: 'assign-2',
          decomposed_scores: [
            {
              score_id: 'DE75C1F7F43365D204C26347CD2B7DD2FE030:deterministic-pass-rate',
              dimension: 'deterministic_pass_rate',
              value: 1,
            },
          ],
        },
      },
      context,
    );

    const assignment = records.find((record) => record.kind === 'assignment_result');
    expect(assignment).toMatchObject({
      assignment_id: 'assign-2',
      metric_values: {
        deterministic_pass_rate: { value: 1 },
        pass: { value: 1 },
      },
    });
    expect(assignment?.metric_values).not.toHaveProperty('DE75C1F7F43365D204C26347CD2B7DD2FE030:deterministic-pass-rate');
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

  it('forwards grade summaries and tool scorecard benchmark_observations fields', () => {
    const context = createCampaignAdaptContext();
    adaptCampaignProjectionEnvelope(
      {
        schema_version: '1.0.0',
        message_type: 'PublicAssignmentLifecycleRecord',
        idempotency_key: 'run-1:assign-1:lifecycle:running',
        record: {
          assignment_id: 'assign-1',
          run_id: 'run-1',
          scenario_id: 'tool-select-grep',
          scenario_category: 'EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION',
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
            grade_summaries: [
              {
                criterion_id: 'tool-selection',
                status: 'fail',
                detail: 'expected tool selection evidence is missing',
              },
            ],
            tool_scorecard: {
              tool_selection: { value: 0 },
              tool_recognition: { unavailable_reason: 'not required by this scenario' },
            },
          },
        },
      },
      context,
    );

    const assignment = records.find((record) => record.kind === 'assignment_result');
    expect(assignment?.benchmark_observations).toMatchObject({
      grade_summaries: [
        {
          criterion_id: 'tool-selection',
          status: 'fail',
          detail: 'expected tool selection evidence is missing',
        },
      ],
      tool_scorecard: {
        tool_selection: { value: 0 },
        tool_recognition: { unavailable_reason: 'not required by this scenario' },
      },
    });
    expect(() => decodeViewRecord('assignment_result', assignment)).not.toThrow();
  });
});

describe('EvalStore campaign ingest', () => {
  it('indexes published model summaries separately for each designated role', () => {
    const store = new EvalStore();
    const runId = 'run-role-matrix';
    const datasetId = campaignDatasetId(runId);
    const ingestModel = (sequence: number, role: 'primary' | 'assistant' | 'lite') => {
      store.acceptProjection({
        sequence,
        record_type: 'projection',
        record_bytes: JSON.stringify({
          schema_version: '1.3.0',
          kind: 'model_summary',
          dataset_id: datasetId,
          quality_state: 'live_in_progress',
          observed_at: '2026-09-16T14:00:00.000Z',
          variant_id: 'qwen3-4b',
          display_name: 'qwen3-4b',
          role,
          inventory_only: false,
          evaluation_coverage: 1,
        }),
      });
    };
    ingestModel(1, 'primary');
    ingestModel(2, 'assistant');
    ingestModel(3, 'lite');

    const models = store.getModels(datasetId).filter((model) => model.variant_id === 'qwen3-4b');
    expect(models.map((model) => model.role).sort()).toEqual(['assistant', 'lite', 'primary']);
  });

  it('does not synthesize catalog, model, or methodology aggregates from campaign envelopes', () => {
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
          observed_at: '2026-09-16T14:00:00.000Z',
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
    expect(catalog).toBeUndefined();
    expect(model).toBeUndefined();
    expect(methodology).toBeUndefined();
    expect(resultRecords.some((record) => record.kind === 'assignment_result')).toBe(true);
    expect(resultRecords.some((record) => record.kind === 'assignment_completed')).toBe(true);
  });

  it('uses catalog assignment_count as the campaign matrix total', () => {
    const store = new EvalStore();
    const runId = 'run-catalog-total';
    const datasetId = campaignDatasetId(runId);

    store.acceptProjection({
      sequence: 1,
      record_type: 'projection',
      record_bytes: JSON.stringify({
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
      }),
    });

    store.acceptProjection({
      sequence: 2,
      record_type: 'projection',
      record_bytes: JSON.stringify({
        schema_version: '1.3.0',
        kind: 'catalog_snapshot',
        dataset_id: datasetId,
        dataset_kind: 'live_run',
        quality_state: 'live_in_progress',
        observed_at: '2026-09-16T14:00:00Z',
        source_revision_label: 'g8e-eval-campaign',
        title: 'Live smoke run',
        description: 'test',
        limitations: [],
        model_count: 1,
        evaluated_count: 0,
        suite_count: 1,
        run_count: 1,
        assignment_count: 4,
        provider_request_count: 0,
        provider_token_count: 0,
        retry_count: 0,
        verifier_passed_count: 0,
        verifier_failed_count: 0,
        generated_at: '2026-09-16T14:00:00Z',
      }),
    });

    store.acceptProjection({
      sequence: 3,
      record_type: 'projection',
      record_bytes: JSON.stringify({
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
      }),
    });

    const run = store.getEvaluation(datasetId, runId);
    const failEvent = store.getEvents(runId, datasetId).find((event) => event.kind === 'assignment_failed');
    expect(run).toMatchObject({ assignment_total: 4 });
    expect(failEvent).toMatchObject({ completed: 1, total: 4 });
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
          observed_at: '2026-09-16T14:00:00.000Z',
        },
      }),
    });

    expect(store.getState().errors).toEqual([]);
    expect(store.getEvaluations(campaignDatasetId('run-live')).length).toBe(0);
    expect(store.getEvents('run-live', campaignDatasetId('run-live')).length).toBe(1);
    expect(store.getCatalog(campaignDatasetId('run-live'))).toBeUndefined();
    expect(store.getModels(campaignDatasetId('run-live'))).toEqual([]);
    expect(store.getState().methodology).toBeNull();
  });
});
