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

  it('accepts protojson null for empty benchmark unavailable reasons', () => {
    const record = fixtureCampaignResultEnvelope.record as CampaignResultRecord;
    expect(() =>
      decodeCampaignProjectionEnvelope({
        ...fixtureCampaignResultEnvelope,
        record: {
          ...record,
          benchmark_observations: {
            ...record.benchmark_observations,
            unavailable_reasons: null,
          },
        },
      }),
    ).not.toThrow();
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

  it('accepts canonical wire unavailable reasons for tool scorecards and resource summaries', () => {
    const record = fixtureCampaignResultEnvelope.record as CampaignResultRecord;
    const observations = record.benchmark_observations ?? {};
    expect(() =>
      decodeCampaignProjectionEnvelope({
        ...fixtureCampaignResultEnvelope,
        record: {
          ...record,
          benchmark_observations: {
            ...observations,
            tool_scorecard: {
              ...(observations.tool_scorecard ?? {}),
              argument_schema: { unavailable_reason: 'scenario_not_applicable' },
            },
          },
          resource_summary: {
            ...record.resource_summary,
            cache_tokens: { unavailable_reason: 'no_scored_calls' },
          },
        },
      }),
    ).not.toThrow();
  });

  it('accepts multiline model_response and failure_output exceeding 128 bytes', () => {
    const record = fixtureCampaignResultEnvelope.record as CampaignResultRecord;
    const longResponse = 'Line 1: Model generated output.\nLine 2: Continuing with more tokens.\n'.repeat(10);
    expect(() =>
      decodeCampaignProjectionEnvelope({
        ...fixtureCampaignResultEnvelope,
        record: {
          ...record,
          model_response: longResponse,
          failure_output: 'Error: execution timeout\nTraceback:\n  file.py:10',
        },
      }),
    ).not.toThrow();
  });

  describe('role_transcripts', () => {
    const record = fixtureCampaignResultEnvelope.record as CampaignResultRecord;
    const toolCall = {
      tool_name: 'read_file',
      arguments_json: '{"path":"/tmp/retry.conf"}',
      arguments_hash: 'b'.repeat(64),
      command: 'cat /tmp/retry.conf',
      success: true,
      result_json: '{"content":"retries=3"}',
      result_redaction: 'truncated',
    };
    const transcript = { role: 'lite', response: 'retries=3', finish_reason: 'stop', trace_digest: 'c'.repeat(64), tool_calls: [toolCall] };
    const withTranscripts = (roleTranscripts: unknown, envelope = fixtureCampaignResultEnvelope) => ({
      ...envelope,
      record: { ...record, role_transcripts: roleTranscripts },
    });

    it('accepts a valid formation transcript list', () => {
      const decoded = decodeCampaignProjectionEnvelope(withTranscripts([transcript, { ...transcript, role: 'assistant' }, { role: 'primary' }]));
      expect((decoded.record as CampaignResultRecord).role_transcripts).toHaveLength(3);
    });

    it.each([
      ['an unknown transcript field', [{ ...transcript, prompt: 'secret' }]],
      ['an unknown tool call field', [{ ...transcript, tool_calls: [{ ...toolCall, operator_id: 'op-1' }] }]],
      ['an unknown role', [{ ...transcript, role: 'judge' }]],
      ['a non-hex arguments_hash', [{ ...transcript, tool_calls: [{ ...toolCall, arguments_hash: 'Z'.repeat(64) }] }]],
      ['a short trace_digest', [{ ...transcript, trace_digest: 'c'.repeat(63) }]],
      ['an unknown result_redaction', [{ ...transcript, tool_calls: [{ ...toolCall, result_redaction: 'hidden' }] }]],
      ['a missing tool success flag', [{ ...transcript, tool_calls: [{ tool_name: 'read_file' }] }]],
      ['a non-array value', { role: 'lite' }],
    ])('rejects %s', (_label, roleTranscripts) => {
      expect(() => decodeCampaignProjectionEnvelope(withTranscripts(roleTranscripts))).toThrow();
    });

    it('rejects role_transcripts in a 1.0.0 envelope', () => {
      expect(() =>
        decodeCampaignProjectionEnvelope({ ...historicalResultEnvelope, record: { ...historicalResultEnvelope.record, role_transcripts: [transcript] } }),
      ).toThrow(/requires campaign envelope 1.1.0/);
    });
  });
});


describe('campaign result envelope 1.2.0 (trajectory fields)', () => {
  const baseRecord = fixtureCampaignResultEnvelope.record as CampaignResultRecord;
  const toolCallFamily = {
    availability: 'PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED',
    records: [
      {
        tool_label: 'recursive_grep_search',
        execution_outcome: 'EVALUATION_VERDICT_STATUS_FAIL',
        semantic_outcome: 'EVALUATION_VERDICT_STATUS_FAIL',
        evidence_source: 'PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED',
        loop_turn: 2,
        error_type: 'validation.error',
        guidance_shown: true,
      },
    ],
  } as const;
  const trajectoryRecord = {
    ...baseRecord,
    scenario_summary: {
      ...baseRecord.scenario_summary!,
      trajectory_policy: 'EVALUATION_TRAJECTORY_POLICY_GUIDED',
      prompt_hint: {
        hinted_tools: ['recursive_grep_search'],
        arguments: [
          { tool_name: 'recursive_grep_search', argument_name: 'pattern', source: 'EVALUATION_HINT_ARGUMENT_SOURCE_PROMPT' },
        ],
      },
    },
    activity_summary: { ...baseRecord.activity_summary!, tool_calls: toolCallFamily },
    trajectory_outcome: 'EVALUATION_TRAJECTORY_OUTCOME_IGNORED_GUIDANCE',
    guided_retry_count: 1,
    failure_reason: '`recursive_grep_search` was declared to the model and hinted by the prompt. The model made no tool call.',
    tools_declared: ['recursive_grep_search', 'file_read_on_operator'],
  };
  const envelope = (schemaVersion: string, record: unknown) => ({
    ...fixtureCampaignResultEnvelope,
    schema_version: schemaVersion,
    record,
  });

  it('accepts trajectory fields at envelope 1.2.0', () => {
    expect(() => decodeCampaignProjectionEnvelope(envelope('1.2.0', trajectoryRecord))).not.toThrow();
  });

  it('keeps accepting 1.1.0 envelopes without trajectory fields', () => {
    expect(() => decodeCampaignProjectionEnvelope(envelope('1.1.0', baseRecord))).not.toThrow();
  });

  it.each([
    ['trajectory_outcome', { trajectory_outcome: 'EVALUATION_TRAJECTORY_OUTCOME_NO_TOOL_CALL' }],
    ['failure_reason', { failure_reason: 'The model made no tool call.' }],
    ['tools_declared', { tools_declared: ['file_read_on_operator'] }],
    ['scenario policy', { scenario_summary: trajectoryRecord.scenario_summary }],
    ['tool call guidance', { activity_summary: trajectoryRecord.activity_summary }],
  ])('rejects %s on a 1.1.0 envelope', (_label, extra) => {
    expect(() => decodeCampaignProjectionEnvelope(envelope('1.1.0', { ...baseRecord, ...extra }))).toThrow(
      /require campaign envelope 1\.2\.0/,
    );
  });

  it.each([
    ['an unknown trajectory outcome', { trajectory_outcome: 'EVALUATION_TRAJECTORY_OUTCOME_MYSTERY' }],
    ['a non-integer retry count', { guided_retry_count: 1.5 }],
    ['an oversized failure reason', { failure_reason: 'x'.repeat(513) }],
    ['a hint with a value field', {
      scenario_summary: {
        ...trajectoryRecord.scenario_summary,
        prompt_hint: {
          hinted_tools: ['recursive_grep_search'],
          arguments: [{ tool_name: 'recursive_grep_search', argument_name: 'pattern', source: 'EVALUATION_HINT_ARGUMENT_SOURCE_PROMPT', value: 'AUTH_FAILURE' }],
        },
      },
    }],
    ['an unknown hint source', {
      scenario_summary: {
        ...trajectoryRecord.scenario_summary,
        prompt_hint: { hinted_tools: [], arguments: [{ tool_name: 'a', argument_name: 'b', source: 'EVALUATION_HINT_ARGUMENT_SOURCE_GUESS' }] },
      },
    }],
  ])('rejects %s', (_label, extra) => {
    expect(() => decodeCampaignProjectionEnvelope(envelope('1.2.0', { ...trajectoryRecord, ...extra }))).toThrow();
  });
});
