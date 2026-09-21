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
import type { DescEnum, DescMessage } from '@bufbuild/protobuf';
import { nestedTypes } from '@bufbuild/protobuf/reflect';

import { parseCanonical, serializeCanonical } from '../src/canonical.ts';
import { file_g8e_eval_v1_eval } from '../src/gen/g8e/eval/v1/eval_pb.ts';
import {
  EvaluationAssignmentResultSchema,
  EvaluationCampaignSpecSchema,
  EvaluationReportSchema,
  EvaluationVerificationReportSchema,
  PublicAssignmentResultProjectionSchema,
} from '../src/gen/g8e/eval/v1/eval_pb.ts';

const __dirname = dirname(fileURLToPath(import.meta.url));
const PROTOCOL_ROOT = join(__dirname, '..', '..');
const DESCRIPTOR_PATH = join(PROTOCOL_ROOT, 'descriptors', 'eval', 'v1', 'model_campaign.json');
const VECTOR_DIR = join(PROTOCOL_ROOT, 'vectors', 'eval');

type MessageDescriptor = {
  visibility: 'private' | 'public';
  fields: string[];
  prohibited_fields?: string[];
  oneof_groups?: string[];
};

type ModelCampaignDescriptor = {
  schema_version: string;
  protobuf_package: string;
  enums: Record<string, string[]>;
  messages: Record<string, MessageDescriptor>;
  canonical_vectors: Array<{ message_type: string; vector_path: string }>;
};

const descriptor = JSON.parse(readFileSync(DESCRIPTOR_PATH, 'utf8')) as ModelCampaignDescriptor;

function loadVector(name: string): { message_type: string; canonical_json: string } {
  return JSON.parse(readFileSync(join(VECTOR_DIR, name), 'utf8'));
}

function findEnum(enumName: string): DescEnum {
  const fullName = `${descriptor.protobuf_package}.${enumName}`;
  for (const candidate of nestedTypes(file_g8e_eval_v1_eval)) {
    if (candidate.kind === 'enum' && candidate.typeName === fullName) {
      return candidate;
    }
  }
  throw new Error(`enum not found: ${fullName}`);
}

function findMessage(messageName: string): DescMessage {
  const fullName = `${descriptor.protobuf_package}.${messageName}`;
  for (const candidate of nestedTypes(file_g8e_eval_v1_eval)) {
    if (candidate.kind === 'message' && candidate.typeName === fullName) {
      return candidate;
    }
  }
  throw new Error(`message not found: ${fullName}`);
}

function messageFieldNames(message: DescMessage): { fields: string[]; oneofGroups: string[] } {
  const fields: string[] = [];
  const oneofGroups: string[] = [];
  for (const field of message.fields) {
    if (field.oneof !== undefined) {
      if (!oneofGroups.includes(field.oneof.name)) {
        oneofGroups.push(field.oneof.name);
      }
      continue;
    }
    fields.push(field.name);
  }
  fields.sort();
  oneofGroups.sort();
  return { fields, oneofGroups };
}

describe('evaluation model campaign descriptor stays in sync with generated protobuf', () => {
  it('descriptor metadata matches the eval package', () => {
    assert.equal(descriptor.schema_version, '1.0.0');
    assert.equal(descriptor.protobuf_package, 'g8e.eval.v1');
    assert.equal(descriptor.canonical_vectors.length, 7);
  });

  for (const [enumName, values] of Object.entries(descriptor.enums)) {
    it(`enum ${enumName} matches generated protobuf values`, () => {
      const enumDescriptor = findEnum(enumName);
      const liveValues = enumDescriptor.values.map((value) => value.name).sort();
      assert.deepEqual(liveValues, [...values].sort());
    });
  }

  for (const [messageName, messageDescriptor] of Object.entries(descriptor.messages)) {
    it(`message ${messageName} matches generated protobuf fields`, () => {
      const schema = findMessage(messageName);
      const live = messageFieldNames(schema);
      assert.deepEqual(live.fields, [...messageDescriptor.fields].sort());
      assert.deepEqual(live.oneofGroups, [...(messageDescriptor.oneof_groups ?? [])].sort());

      for (const prohibited of messageDescriptor.prohibited_fields ?? []) {
        assert.equal(
          schema.fields.some((field) => field.name === prohibited),
          false,
          `public projection must not expose ${prohibited}`,
        );
      }
    });
  }
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
