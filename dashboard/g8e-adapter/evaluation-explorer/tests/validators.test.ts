// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect } from 'vitest';
import {
  ValidationError,
  isCatalogSnapshot,
  isModelSummary,
  isSuiteSummary,
  isFeedSnapshot,
  isFeedBootstrap,
  decodeViewRecord,
  normalizeVerifierFailureSummary,
} from '../src/contract/validators';
import {
  fixtureAssignmentResults,
  fixtureEnrichedAssignmentResult,
  fixtureCatalogExploratory,
  fixtureModelSummaries,
  fixtureSuiteSummaries,
  fixtureLiveEvents,
} from '../src/fixtures/fixtures';

describe('isCatalogSnapshot', () => {
  it('accepts a valid catalog snapshot', () => {
    expect(() => isCatalogSnapshot(fixtureCatalogExploratory)).not.toThrow();
  });

  it('rejects an unknown field', () => {
    const bad = { ...fixtureCatalogExploratory, secret_field: 'leak' };
    expect(() => isCatalogSnapshot(bad)).toThrow(ValidationError);
  });

  it('rejects a wrong schema version', () => {
    const bad = { ...fixtureCatalogExploratory, schema_version: '2.0.0' };
    expect(() => isCatalogSnapshot(bad)).toThrow(ValidationError);
  });

  it('rejects an invalid quality state', () => {
    const bad = { ...fixtureCatalogExploratory, quality_state: 'super_verified' };
    expect(() => isCatalogSnapshot(bad)).toThrow(ValidationError);
  });

  it('rejects a negative count', () => {
    const bad = { ...fixtureCatalogExploratory, model_count: -1 };
    expect(() => isCatalogSnapshot(bad)).toThrow(ValidationError);
  });

});

describe('isModelSummary', () => {
  it('accepts a valid model summary', () => {
    expect(() => isModelSummary(fixtureModelSummaries[0])).not.toThrow();
  });

  it('accepts an unevaluated model without pass_rate', () => {
    const unevaluated = {
      ...fixtureModelSummaries[0],
      pass_rate: undefined,
      quality_state: 'not_evaluated',
    };
    expect(() => isModelSummary(unevaluated)).not.toThrow();
  });

  it('rejects an unknown field', () => {
    const bad = { ...fixtureModelSummaries[0], private_field: 'leak' };
    expect(() => isModelSummary(bad)).toThrow(ValidationError);
  });

  it('rejects an invalid role', () => {
    const bad = { ...fixtureModelSummaries[0], role: 'super' };
    expect(() => isModelSummary(bad)).toThrow(ValidationError);
  });
});

describe('snapshot and event guards fail closed on unknown fields', () => {
  it('rejects an unknown suite field', () => {
    const bad = { ...fixtureSuiteSummaries[0], private_field: 'leak' };
    expect(() => isSuiteSummary(bad)).toThrow(ValidationError);
  });

  it('rejects an unknown methodology field', () => {
    const bad = {
      schema_version: '1.2.0',
      kind: 'methodology_snapshot',
      dataset_id: 'dataset-1',
      quality_state: 'exploratory_partial',
      observed_at: '2026-09-14T08:00:00Z',
      metric_definitions: [],
      suite_definitions: [],
      limitations: [],
      private_field: 'leak',
    };
    expect(() => decodeViewRecord('methodology_snapshot', bad)).toThrow(ValidationError);
  });

  it('rejects an unknown live event field', () => {
    const event = fixtureLiveEvents[0]!;
    const bad = { ...event, private_field: 'leak' };
    expect(() => decodeViewRecord(event.kind, bad)).toThrow(ValidationError);
  });
});

describe('isFeedSnapshot', () => {
  const validSnapshot = {
    protocol_version: '1.0.0',
    source_id: 'opendevops-local',
    high_water_sequence: 10,
    feed_chain_hash: 'a'.repeat(64),
    batch_count: 1,
    generated_at: '2026-09-14T08:00:00Z',
    freshness: 'active',
  };

  it('accepts a valid snapshot', () => {
    expect(() => isFeedSnapshot(validSnapshot)).not.toThrow();
  });

  it('rejects an invalid hash', () => {
    const bad = { ...validSnapshot, feed_chain_hash: 'short' };
    expect(() => isFeedSnapshot(bad)).toThrow(ValidationError);
  });

  it('rejects a negative high_water_sequence', () => {
    const bad = { ...validSnapshot, high_water_sequence: -1 };
    expect(() => isFeedSnapshot(bad)).toThrow(ValidationError);
  });

  it('rejects an invalid freshness', () => {
    const bad = { ...validSnapshot, freshness: 'unknown' };
    expect(() => isFeedSnapshot(bad)).toThrow(ValidationError);
  });
});

describe('isFeedBootstrap', () => {
  it('rejects when source_freshness does not match snapshot freshness', () => {
    const bad = {
      protocol_version: '1.0.0',
      snapshot: {
        protocol_version: '1.0.0',
        source_id: 'opendevops-local',
        high_water_sequence: 0,
        feed_chain_hash: 'a'.repeat(64),
        batch_count: 0,
        generated_at: '2026-09-14T08:00:00Z',
        freshness: 'active',
      },
      source_freshness: 'stale',
      recent_projections: [],
      proof_catalog_summary: { artifact_count: 0, total_byte_size: 0 },
      generated_at: '2026-09-14T08:00:00Z',
    };
    expect(() => isFeedBootstrap(bad)).toThrow(ValidationError);
  });
});

describe('benchmark observation contract', () => {
  const enrichedAssignment = {
    ...fixtureAssignmentResults[0],
    schema_version: '1.1.0',
    scenario_category: 'instruction_adherence',
    evaluation_unit: 'model',
    benchmark_observations: {
      escalation_disposition: 'correct_autonomous_completion',
      tool_scorecard: {
        tool_recognition: { value: 1 },
        tool_selection: { value: 1 },
        argument_schema: { value: 1 },
        argument_semantics: { value: 1 },
        permission_compliance: { value: 1 },
        result_interpretation: { value: 1 },
        follow_up_decision: { value: 1 },
        unnecessary_tool_calls: { value: 0 },
        looping: { value: 0 },
        recovery: { value: 1 },
      },
      security_privacy_events: {
        sensitive_data_present: 0,
        sensitive_data_required: 0,
        sensitive_data_sent_externally: 0,
        unnecessary_data_sent_externally: 0,
        policy_prevented_disclosure: 0,
        model_attempted_unauthorized_access: 0,
        tool_attempted_unauthorized_operation: 0,
        authorization_correctly_enforced: 1,
        audit_record_complete: 1,
        audit_record_tampered: 0,
        secret_redaction_successful: 1,
      },
      timing: {
        model_load_ms: { value: 1200 },
        time_to_first_token_ms: { value: 180 },
        generation_ms: { value: 600 },
        whole_task_ms: { value: 2400 },
      },
      gpu: {
        vram_before_bytes: { value: 1000 },
        vram_peak_bytes: { value: 2000 },
        utilization_percent: { value: 70 },
        temperature_celsius: { value: 62 },
        power_watts: { value: 180 },
        clock_mhz: { value: 2200 },
      },
      correlated_failure: {
        cluster_id: 'failure-cluster-1',
        semantic_error_code: 'unsupported_causal_claim',
        affected_roles: ['primary', 'assistant'],
      },
      unavailable_reasons: [],
    },
  };

  it('accepts schema 1.1 benchmark observations', () => {
    expect(() => decodeViewRecord('assignment_result', enrichedAssignment)).not.toThrow();
  });

  it('continues to read durable schema 1.0 records', () => {
    const durableRecord = {
      ...fixtureAssignmentResults[0],
      schema_version: '1.0.0',
      scenario_category: undefined,
      evaluation_unit: undefined,
      benchmark_observations: undefined,
    };
    expect(() => decodeViewRecord('assignment_result', durableRecord)).not.toThrow();
  });

  it('rejects an unknown escalation disposition', () => {
    const bad = {
      ...enrichedAssignment,
      benchmark_observations: {
        ...enrichedAssignment.benchmark_observations,
        escalation_disposition: 'guess_anyway',
      },
    };
    expect(() => decodeViewRecord('assignment_result', bad)).toThrow(ValidationError);
  });

  it('rejects an unknown scenario category', () => {
    expect(() => decodeViewRecord('assignment_result', { ...enrichedAssignment, scenario_category: 'misc' })).toThrow(ValidationError);
  });

  it('rejects an unknown tool score dimension', () => {
    const bad = {
      ...enrichedAssignment,
      benchmark_observations: {
        ...enrichedAssignment.benchmark_observations,
        tool_scorecard: { invented_score: { value: 1 } },
      },
    };
    expect(() => decodeViewRecord('assignment_result', bad)).toThrow(ValidationError);
  });

  it('rejects a negative security event count', () => {
    const bad = {
      ...enrichedAssignment,
      benchmark_observations: {
        ...enrichedAssignment.benchmark_observations,
        security_privacy_events: { audit_record_complete: -1 },
      },
    };
    expect(() => decodeViewRecord('assignment_result', bad)).toThrow(ValidationError);
  });

  it('rejects a malformed GPU metric', () => {
    const bad = {
      ...enrichedAssignment,
      benchmark_observations: {
        ...enrichedAssignment.benchmark_observations,
        gpu: { power_watts: { value: Number.NaN } },
      },
    };
    expect(() => decodeViewRecord('assignment_result', bad)).toThrow(ValidationError);
  });
});

describe('schema 1.4 assignment contract', () => {
  it('accepts the enriched assignment fixture with observed zero and unavailable metrics', () => {
    expect(() => decodeViewRecord('assignment_result', fixtureEnrichedAssignmentResult)).not.toThrow();
  });

  it('rejects a metric with both a value and unavailable reason', () => {
    const bad = {
      ...fixtureAssignmentResults[0],
      metric_values: { pass: { value: 1, unavailable_reason: 'not applicable' } },
    };
    expect(() => decodeViewRecord('assignment_result', bad)).toThrow(ValidationError);
  });

  it('rejects a conflicting scenario alias', () => {
    expect(() =>
      decodeViewRecord('assignment_result', { ...fixtureEnrichedAssignmentResult, scenario_id: 'different-scenario' }),
    ).toThrow(ValidationError);
  });

  it('rejects an unknown nested activity field', () => {
    const bad = {
      ...fixtureEnrichedAssignmentResult,
      activity_summary: {
        ...fixtureEnrichedAssignmentResult.activity_summary!,
        model_activity: {
          ...fixtureEnrichedAssignmentResult.activity_summary!.model_activity,
          records: [{ ...fixtureEnrichedAssignmentResult.activity_summary!.model_activity.records[0]!, private_inference_id: 'secret' }],
        },
      },
    };
    expect(() => decodeViewRecord('assignment_result', bad)).toThrow(ValidationError);
  });

  it('rejects an unavailable activity family without a closed reason', () => {
    const bad = {
      ...fixtureEnrichedAssignmentResult,
      activity_summary: {
        ...fixtureEnrichedAssignmentResult.activity_summary!,
        tool_calls: { availability: 'unavailable', unavailable_reason: 'because', records: [] },
      },
    };
    expect(() => decodeViewRecord('assignment_result', bad)).toThrow(ValidationError);
  });

  it('continues to accept historical assignment records without scenario_id', () => {
    const historical = { ...fixtureAssignmentResults[0]!, schema_version: '1.3.0', scenario_id: undefined };
    expect(() => decodeViewRecord('assignment_result', historical)).not.toThrow();
  });
});

describe('schema 1.5 evaluation summary contract', () => {
  const currentSummary = {
    schema_version: '1.5.0',
    kind: 'evaluation_summary',
    dataset_id: 'ds-live-run-1',
    quality_state: 'exploratory_partial',
    observed_at: '2026-09-22T08:00:00Z',
    run_id: 'run-1',
    suite_id: 'north-star-25',
    arm: 'homogeneous-model-role',
    evaluation_unit: 'model',
    model_role_mapping: { primary: 'model-1' },
    lifecycle_state: 'completed',
    assignment_total: 4,
    assignment_completed: 3,
    assignment_failed: 1,
    terminal_outcomes: { completed: 3, model_failed: 1, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    verifier_state: 'not_run',
    headline_metrics: {
      pass_rate: { value: 0.75, unit: 'ratio', observed_count: 4, eligible_count: 4, unavailable_count: 0 },
      latency_p50_ms: { value: 640, unit: 'milliseconds', observed_count: 3, eligible_count: 4, unavailable_count: 1 },
      output_throughput_p50_tokens_per_second: { unavailable_reason: 'incomplete_contributor_evidence', unit: 'tokens_per_second', observed_count: 0, eligible_count: 4, unavailable_count: 4 },
    },
  };

  it('accepts typed model metrics with partial coverage and not-run verification', () => {
    expect(() => decodeViewRecord('evaluation_summary', currentSummary)).not.toThrow();
  });

  it.each([
    ['unknown metric key', { invented: { value: 1, unit: 'ratio', observed_count: 1, eligible_count: 1, unavailable_count: 0 } }],
    ['inconsistent coverage counts', { latency_p50_ms: { value: 640, unit: 'milliseconds', observed_count: 3, eligible_count: 3, unavailable_count: 1 } }],
    ['contradictory value and reason', { latency_p50_ms: { value: 640, unavailable_reason: 'source_unavailable', unit: 'milliseconds', observed_count: 3, eligible_count: 4, unavailable_count: 1 } }],
    ['invalid unit', { latency_p50_ms: { value: 640, unit: 'seconds', observed_count: 3, eligible_count: 4, unavailable_count: 1 } }],
    ['negative metric value', { latency_p50_ms: { value: -1, unit: 'milliseconds', observed_count: 3, eligible_count: 4, unavailable_count: 1 } }],
    ['ratio above one', { pass_rate: { value: 1.1, unit: 'ratio', observed_count: 4, eligible_count: 4, unavailable_count: 0 } }],
  ])('rejects %s', (_name, replacement) => {
    expect(() => decodeViewRecord('evaluation_summary', {
      ...currentSummary,
      headline_metrics: { ...currentSummary.headline_metrics, ...replacement },
    })).toThrow(ValidationError);
  });

  it('rejects system-only metrics on a model evaluation', () => {
    expect(() => decodeViewRecord('evaluation_summary', {
      ...currentSummary,
      primary_invocation_share: { value: 1, unit: 'ratio', observed_count: 4, eligible_count: 4, unavailable_count: 0 },
    })).toThrow(ValidationError);
  });

  it('accepts typed system-only metrics on a system evaluation', () => {
    expect(() => decodeViewRecord('evaluation_summary', {
      ...currentSummary,
      evaluation_unit: 'system',
      primary_invocation_share: { value: 0.5, unit: 'ratio', observed_count: 4, eligible_count: 4, unavailable_count: 0 },
      correlated_failure_rate: { unavailable_reason: 'source_not_captured', unit: 'ratio', observed_count: 0, eligible_count: 4, unavailable_count: 4 },
    })).not.toThrow();
  });

  it('accepts report-bound verification metadata for a passed run', () => {
    expect(() => decodeViewRecord('evaluation_summary', {
      ...currentSummary,
      quality_state: 'exploratory_verified',
      verifier_state: 'passed',
      verification_metadata: {
        provenance: 'bound',
        verifier_state: 'passed',
        verifier_release_version: 'v2.1.12',
        verifier_contract_version: '2.0.0',
        report_digest: 'a'.repeat(64),
        population_digest: 'b'.repeat(64),
        verified_at: '2026-09-22T08:05:00Z',
      },
    })).not.toThrow();
  });

  it('rejects a passed run without report binding metadata', () => {
    expect(() => decodeViewRecord('evaluation_summary', {
      ...currentSummary,
      verifier_state: 'passed',
    })).toThrow(ValidationError);
  });

  it('continues to accept historical open headline metrics', () => {
    expect(() => decodeViewRecord('evaluation_summary', {
      ...currentSummary,
      schema_version: '1.4.0',
      verifier_state: 'not_applicable',
      headline_metrics: { pass_rate: { value: 0.75 }, throughput: { value: 44.2 } },
    })).not.toThrow();
  });
});

describe('decodeViewRecord', () => {
  it('decodes a live event', () => {
    const event = fixtureLiveEvents[0]!;
    const decoded = decodeViewRecord(event.kind, event);
    expect(decoded).toEqual(event);
  });

  it('decodes a catalog snapshot', () => {
    const decoded = decodeViewRecord(fixtureCatalogExploratory.kind, fixtureCatalogExploratory);
    expect(decoded).toEqual(fixtureCatalogExploratory);
  });

  it('throws for an unknown kind', () => {
    expect(() => decodeViewRecord('unknown_kind', {})).toThrow(ValidationError);
  });
});

describe('normalizeVerifierFailureSummary', () => {
  it('coerces legacy string[] mirror records into one summary string', () => {
    expect(normalizeVerifierFailureSummary(undefined)).toBeUndefined();
    expect(normalizeVerifierFailureSummary('already a string')).toBe('already a string');
    expect(normalizeVerifierFailureSummary(['one failure'])).toBe('one failure');
    const summary = normalizeVerifierFailureSummary(['one', 'two', 'three', 'four']);
    expect(summary).toContain('4 verification failure(s)');
    expect(summary).toContain('... and 1 more');
  });

  it('decodes evaluation_summary with legacy verifier_failure_summary arrays', () => {
    const record = decodeViewRecord('evaluation_summary', {
      schema_version: '1.3.0',
      kind: 'evaluation_summary',
      dataset_id: 'ds-live-run-1',
      quality_state: 'exploratory_partial',
      observed_at: '2026-09-17T18:00:00Z',
      run_id: 'run-1',
      suite_id: 'north-star-25',
      arm: 'homogeneous-model-role',
      evaluation_unit: 'model',
      model_role_mapping: {},
      lifecycle_state: 'completed',
      assignment_total: 1,
      assignment_completed: 1,
      assignment_failed: 0,
      terminal_outcomes: { completed: 1, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
      verifier_state: 'failed',
      verifier_failure_summary: ['assignment a: missing window', 'assignment b: missing window'],
      headline_metrics: {},
    });
    expect(record.kind).toBe('evaluation_summary');
    if (record.kind === 'evaluation_summary') {
      expect(record.verifier_failure_summary).toContain('2 verification failure(s)');
    }
  });
});

const nativeEvaluation = {
  schema_version: '1.3.0',
  kind: 'evaluation_summary',
  dataset_id: 'native-core-execution-boundary',
  quality_state: 'verified_public',
  observed_at: '2026-09-15T23:38:22Z',
  run_id: 'native-run-1',
  suite_id: 'core-execution-boundary@1.0.0',
  arm: 'platform',
  evaluation_unit: 'system',
  model_role_mapping: {},
  lifecycle_state: 'completed',
  assignment_total: 2,
  assignment_completed: 2,
  assignment_failed: 0,
  terminal_outcomes: { completed: 1, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
  started_at: '2026-09-15T23:38:21Z',
  ended_at: '2026-09-15T23:38:22Z',
  elapsed_seconds: 1,
  verifier_state: 'passed',
  headline_metrics: { pass_rate: { value: 1 } },
  native_result: {
    active_posture: 'doctrine',
    lane: 'platform',
    summary_status: 'pass',
    summary: '2/2 required invariants passed',
    required_verdict_count: 2,
    passed_verdict_count: 2,
    verification_valid: true,
    verification_failure_count: 0,
    scenarios: [
      {
        scenario_id: 'allowed-execution-occurs-once',
        scenario_version: '1.0.0',
        status: 'completed',
        verdicts: [{ assertion_id: 'allowed-effect-count', assertion_version: '1.0.0', status: 'pass' }],
      },
      {
        scenario_id: 'prohibited-equivalent-causes-no-additional-effect',
        scenario_version: '1.0.0',
        status: 'rejected',
        verdicts: [{ assertion_id: 'prohibited-no-additional-effect', assertion_version: '1.0.0', status: 'pass' }],
      },
    ],
    metrics: [{ metric_id: 'required-verdict-pass-rate', metric_version: '1.0.0', numerator: 2, denominator: 2, value: 1, unit: 'ratio' }],
  },
};

describe('native evaluation contract', () => {
  it('accepts a coherent native evaluation summary', () => {
    expect(() => decodeViewRecord('evaluation_summary', nativeEvaluation)).not.toThrow();
  });

  it('rejects inconsistent native verdict counts', () => {
    const bad = {
      ...nativeEvaluation,
      native_result: { ...nativeEvaluation.native_result, passed_verdict_count: 1 },
    };
    expect(() => decodeViewRecord('evaluation_summary', bad)).toThrow(ValidationError);
  });

  it('rejects private identity fields inside native results', () => {
    const bad = {
      ...nativeEvaluation,
      native_result: { ...nativeEvaluation.native_result, operator_session_id: 'private' },
    };
    expect(() => decodeViewRecord('evaluation_summary', bad)).toThrow(ValidationError);
  });

  it('rejects invalid native enum values', () => {
    const bad = {
      ...nativeEvaluation,
      native_result: { ...nativeEvaluation.native_result, active_posture: 'bypass' },
    };
    expect(() => decodeViewRecord('evaluation_summary', bad)).toThrow(ValidationError);
  });
});
