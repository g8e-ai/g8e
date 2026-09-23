// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, it } from 'node:test';

import { create } from '@bufbuild/protobuf';

import { parseCanonical, serializeCanonical } from '../src/canonical.ts';
import {
  EvaluationAssignmentResultSchema,
  EvaluationCampaignSpecSchema,
  EvaluationReportSchema,
  EvaluationVerificationReportSchema,
  PublicAssignmentResultProjectionSchema,
} from '../src/gen/g8e/eval/v1/eval_pb.ts';

const __dirname = dirname(fileURLToPath(import.meta.url));
const PROTOCOL_ROOT = join(__dirname, '..', '..');
const VECTOR_DIR = join(PROTOCOL_ROOT, 'vectors', 'eval');

function loadVector(name: string): { message_type: string; canonical_json: string } {
  return JSON.parse(readFileSync(join(VECTOR_DIR, name), 'utf8'));
}

describe('evaluation public projection disclosure boundary', () => {
  it('omits private execution evidence fields', () => {
    const forbidden = new Set([
      'prompt',
      'output',
      'thinking',
      'receipt',
      'transaction_id',
      'operator_session_id',
      'operator_id',
      'governed_receipt_ref',
      'model_inferences',
      'tool_calls',
      'governed_actions',
    ]);
    for (const field of PublicAssignmentResultProjectionSchema.fields) {
      assert.equal(forbidden.has(field.name), false, `public projection must not expose ${field.name}`);
    }
  });
});

describe('evaluation canonical vectors round-trip through TypeScript', () => {
  const vectorCases = [
    {
      filename: 'phase1_report.json',
      schema: EvaluationReportSchema,
      assert: (message: ReturnType<typeof create<typeof EvaluationReportSchema>>) => {
        assert.equal(message.run?.suiteRef?.id, 'core-execution-boundary');
      },
    },
    {
      filename: 'model_campaign_spec.json',
      schema: EvaluationCampaignSpecSchema,
      assert: (message: ReturnType<typeof create<typeof EvaluationCampaignSpecSchema>>) => {
        assert.equal(message.campaignId, 'phase1a-smoke');
        assert.equal(message.scenarioCount, 25);
      },
    },
    {
      filename: 'model_assignment_result.json',
      schema: EvaluationAssignmentResultSchema,
      assert: (message: ReturnType<typeof create<typeof EvaluationAssignmentResultSchema>>) => {
        assert.equal(message.lane, 2);
        assert.equal(message.modelInferences.length, 1);
      },
    },
    {
      filename: 'public_assignment_result.json',
      schema: PublicAssignmentResultProjectionSchema,
      assert: (message: ReturnType<typeof create<typeof PublicAssignmentResultProjectionSchema>>) => {
        assert.equal(message.assignmentId, 'assign-1');
        assert.equal(message.verificationStatus, 'verified');
      },
    },
    {
      filename: 'model_assignment_result_enriched.json',
      schema: EvaluationAssignmentResultSchema,
      assert: (message: ReturnType<typeof create<typeof EvaluationAssignmentResultSchema>>) => {
        assert.equal(message.assignmentId, 'assign-enriched');
        assert.equal(message.modelInferences[0]?.retryCount, 0);
        assert.equal(message.scoredInferenceSpanNanos, 500000000n);
      },
    },
    {
      filename: 'public_assignment_result_enriched.json',
      schema: PublicAssignmentResultProjectionSchema,
      assert: (message: ReturnType<typeof create<typeof PublicAssignmentResultProjectionSchema>>) => {
        assert.equal(message.assignmentId, 'assign-enriched');
        assert.equal(message.scenarioSummary?.scenarioId, 'tool-selection-1');
        assert.equal(message.activitySummary?.modelActivity?.records.length, 1);
      },
    },
    {
      filename: 'run_verification_bound.json',
      schema: EvaluationVerificationReportSchema,
      assert: (message: ReturnType<typeof create<typeof EvaluationVerificationReportSchema>>) => {
        assert.equal(message.verifierContractVersion, '2.0.0');
        assert.equal(message.verifiedAssignmentCount, 25);
      },
    },
  ] as const;

  for (const vectorCase of vectorCases) {
    it(`${vectorCase.filename} round-trips canonical protojson`, () => {
      const vector = loadVector(vectorCase.filename);
      const encoded = vector.canonical_json;
      const message = create(vectorCase.schema);
      parseCanonical(vectorCase.schema, encoded, message);
      assert.equal(serializeCanonical(vectorCase.schema, message), encoded);
      vectorCase.assert(message);
    });
  }
});
