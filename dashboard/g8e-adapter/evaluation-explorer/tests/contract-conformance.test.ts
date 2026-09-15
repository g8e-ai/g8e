// Contract conformance: every deterministic fixture validates against the
// frozen guards, and every enum value the fixtures use is a member of the
// closed enum set. This is Worker 0's acceptance test for the frozen contract.
// If a fixture fails validation, the contract and fixtures have drifted and
// the contract is not frozen.
//
// Worker 1's projector, Worker 5's bridge, and the replay producer must emit
// records that pass the same guards. Add their outputs here as they land.

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';
import {
  decodeViewRecord,
  ValidationError,
} from '../src/contract/validators';
import {
  LIVE_EVENT_KINDS,
  SNAPSHOT_KINDS,
  VIEW_SCHEMA_VERSION,
} from '../src/contract/types';
import {
  allFixtureSnapshotRecords,
  fixtureLiveEvents,
} from '../src/fixtures/fixtures';

function validate(record: unknown): void {
  const obj = record as { kind: string };
  decodeViewRecord(obj.kind, record);
}

describe('contract schema version', () => {
  it('exposes the frozen schema version', () => {
    expect(VIEW_SCHEMA_VERSION).toBe('1.2.0');
  });
});

describe('fixture snapshot records conform to the frozen contract', () => {
  for (const record of allFixtureSnapshotRecords) {
    it(`validates ${record.kind} (${describeRecord(record)})`, () => {
      expect(() => validate(record)).not.toThrow();
    });
  }

  it('covers every snapshot record kind', () => {
    const covered = new Set(allFixtureSnapshotRecords.map((r) => r.kind));
    for (const kind of SNAPSHOT_KINDS) {
      expect(covered.has(kind), `missing fixture for ${kind}`).toBe(true);
    }
  });
});

describe('projected historical records conform to the frozen contract', () => {
  it('validates every generated projector record', () => {
    const lines = readFileSync(resolve(process.cwd(), 'scripts/fixtures/projected-records.jsonl'), 'utf8')
      .trim()
      .split('\n');
    expect(lines).toHaveLength(1221);
    for (const line of lines) {
      const envelope = JSON.parse(line) as { record_type: string; record_bytes: string };
      expect(envelope.record_type).toBe('projection');
      expect(() => validate(JSON.parse(envelope.record_bytes))).not.toThrow();
    }
  });
});

describe('fixture live events conform to the frozen contract', () => {
  for (const event of fixtureLiveEvents) {
    it(`validates ${event.kind} (${event.event_id})`, () => {
      expect(() => validate(event)).not.toThrow();
    });
  }

  it('covers every live event kind', () => {
    const covered = new Set(fixtureLiveEvents.map((e) => e.kind));
    for (const kind of LIVE_EVENT_KINDS) {
      expect(covered.has(kind), `missing fixture for ${kind}`).toBe(true);
    }
  });
});

describe('guards reject unknown fields and bad enums', () => {
  it('rejects an unknown field on a catalog snapshot', () => {
    const bad = { ...allFixtureSnapshotRecords[0], extra_field: 'no' };
    expect(() => validate(bad)).toThrowError(ValidationError);
  });

  it('rejects an out-of-enum quality state', () => {
    const bad = { ...allFixtureSnapshotRecords[0], quality_state: 'totally_verified' };
    expect(() => validate(bad)).toThrowError(ValidationError);
  });

  it('rejects a wrong schema version', () => {
    const bad = { ...allFixtureSnapshotRecords[0], schema_version: '2.0.0' };
    expect(() => validate(bad)).toThrowError(ValidationError);
  });
});

function describeRecord(record: { kind: string; variant_id?: string; run_id?: string; suite_id?: string; assignment_id?: string }): string {
  return record.variant_id ?? record.run_id ?? record.suite_id ?? record.assignment_id ?? record.kind;
}
