// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Descriptor sync: the machine-readable descriptor.json (the cross-language
// mirror Go publication and the frontend campaign adapter consume) must
// exactly match the authoritative TypeScript enum constants in types.ts.
// If this test fails, a worker changed one source without the other; bump the
// contract version and update both together.

import { describe, expect, it } from 'vitest';
import descriptor from '../src/contract/descriptor.json';
import {
  ACTIVITY_AVAILABILITIES,
  DATASET_KINDS,
  ENVIRONMENT_SOURCES,
  ESCALATION_DISPOSITIONS,
  EVALUATION_UNITS,
  FEED_RECORD_TYPES,
  FRESHNESS_STATES,
  LIVE_EVENT_KINDS,
  MODEL_ROLES,
  NATIVE_LANES,
  NATIVE_METRIC_UNITS,
  NATIVE_POSTURES,
  NATIVE_RESULT_STATUSES,
  NATIVE_SCENARIO_STATUSES,
  PUBLIC_ACTIVITY_EVIDENCE_SOURCES,
  PUBLIC_EVIDENCE_KINDS,
  PUBLIC_FINISH_STATES,
  PUBLIC_GRADE_EXPLANATION_CODES,
  PUBLIC_LOAD_STATES,
  PUBLIC_RECEIPT_STATUSES,
  PUBLIC_SEMANTIC_OUTCOMES,
  PUBLIC_TOOL_EXECUTION_OUTCOMES,
  PUBLIC_TOOL_OUTCOMES,
  PUBLIC_USAGE_AVAILABILITIES,
  PUBLIC_UNAVAILABLE_REASONS,
  QUALITY_STATES,
  REPEATABILITY_CLASSES,
  SCENARIO_CATEGORIES,
  SECURITY_PRIVACY_EVENTS,
  SNAPSHOT_KINDS,
  TERMINAL_STATUSES,
  TOOL_SCORE_DIMENSIONS,
  LIFECYCLE_STATUSES,
  VERIFIER_STATES,
} from '../src/contract/types';

type EnumName =
  | 'ActivityAvailability'
  | 'QualityState'
  | 'DatasetKind'
  | 'ModelRole'
  | 'ScenarioCategory'
  | 'EvaluationUnit'
  | 'EnvironmentSource'
  | 'EscalationDisposition'
  | 'ToolScoreDimension'
  | 'SecurityPrivacyEvent'
  | 'TerminalStatus'
  | 'LifecycleStatus'
  | 'RepeatabilityClass'
  | 'VerifierState'
  | 'NativePosture'
  | 'NativeLane'
  | 'NativeResultStatus'
  | 'NativeScenarioStatus'
  | 'NativeMetricUnit'
  | 'SnapshotKind'
  | 'LiveEventKind'
  | 'FeedRecordType'
  | 'FreshnessState'
  | 'PublicUnavailableReason'
  | 'PublicGradeExplanationCode'
  | 'PublicFinishState'
  | 'PublicLoadState'
  | 'PublicEvidenceKind'
  | 'PublicActivityEvidenceSource'
  | 'PublicToolOutcome'
  | 'PublicUsageAvailability'
  | 'PublicToolExecutionOutcome'
  | 'PublicSemanticOutcome'
  | 'PublicReportedPolicyOutcome'
  | 'PublicReceiptStatus';

const tsEnums: Record<EnumName, readonly string[]> = {
  ActivityAvailability: ACTIVITY_AVAILABILITIES,
  QualityState: QUALITY_STATES,
  DatasetKind: DATASET_KINDS,
  ModelRole: MODEL_ROLES,
  ScenarioCategory: SCENARIO_CATEGORIES,
  EvaluationUnit: EVALUATION_UNITS,
  EnvironmentSource: ENVIRONMENT_SOURCES,
  EscalationDisposition: ESCALATION_DISPOSITIONS,
  ToolScoreDimension: TOOL_SCORE_DIMENSIONS,
  SecurityPrivacyEvent: SECURITY_PRIVACY_EVENTS,
  TerminalStatus: TERMINAL_STATUSES,
  LifecycleStatus: LIFECYCLE_STATUSES,
  RepeatabilityClass: REPEATABILITY_CLASSES,
  VerifierState: VERIFIER_STATES,
  NativePosture: NATIVE_POSTURES,
  NativeLane: NATIVE_LANES,
  NativeResultStatus: NATIVE_RESULT_STATUSES,
  NativeScenarioStatus: NATIVE_SCENARIO_STATUSES,
  NativeMetricUnit: NATIVE_METRIC_UNITS,
  SnapshotKind: SNAPSHOT_KINDS,
  LiveEventKind: LIVE_EVENT_KINDS,
  FeedRecordType: FEED_RECORD_TYPES,
  FreshnessState: FRESHNESS_STATES,
  PublicUnavailableReason: PUBLIC_UNAVAILABLE_REASONS,
  PublicGradeExplanationCode: PUBLIC_GRADE_EXPLANATION_CODES,
  PublicFinishState: PUBLIC_FINISH_STATES,
  PublicLoadState: PUBLIC_LOAD_STATES,
  PublicEvidenceKind: PUBLIC_EVIDENCE_KINDS,
  PublicActivityEvidenceSource: PUBLIC_ACTIVITY_EVIDENCE_SOURCES,
  PublicToolOutcome: PUBLIC_TOOL_OUTCOMES,
  PublicUsageAvailability: PUBLIC_USAGE_AVAILABILITIES,
  PublicToolExecutionOutcome: PUBLIC_TOOL_EXECUTION_OUTCOMES,
  PublicSemanticOutcome: PUBLIC_SEMANTIC_OUTCOMES,
  PublicReportedPolicyOutcome: PUBLIC_TOOL_OUTCOMES,
  PublicReceiptStatus: PUBLIC_RECEIPT_STATUSES,
};

describe('descriptor.json stays in sync with types.ts', () => {
  it('exposes the frozen schema version', () => {
    expect(descriptor.schema_version).toBe('1.4.0');
  });

  for (const [name, tsValues] of Object.entries(tsEnums) as Array<[EnumName, readonly string[]]>) {
    it(`enum ${name} matches between descriptor and types.ts`, () => {
      const jsonValues = (descriptor.enums as Record<EnumName, string[]>)[name];
      expect(jsonValues).toEqual([...tsValues]);
    });
  }
});
