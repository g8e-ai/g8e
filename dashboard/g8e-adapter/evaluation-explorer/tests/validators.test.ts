import { describe, it, expect } from 'vitest';
import {
  ValidationError,
  isCatalogSnapshot,
  isModelSummary,
  isFeedSnapshot,
  isFeedBootstrap,
  decodeViewRecord,
} from '../src/contract/validators';
import {
  fixtureAssignmentResults,
  fixtureCatalogExploratory,
  fixtureModelSummaries,
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

  it('rejects an invalid role', () => {
    const bad = { ...fixtureModelSummaries[0], role: 'super' };
    expect(() => isModelSummary(bad)).toThrow(ValidationError);
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
