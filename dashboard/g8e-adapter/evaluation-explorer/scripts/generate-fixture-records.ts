// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Generate mirror-ready JSONL from the deterministic fixtures.
//
// Each output line is a publish input record: {"record_type":"...","record_bytes":"..."}.
// Snapshot records (catalog_snapshot, model_summary, etc.) are emitted as
// "projection" records. Live events (evaluation_queued, etc.) are emitted as
// "event" records. The record_bytes field is a JSON string of the typed view
// record. This is the exact CLI input shape that `g8e public publish` expects.
//
// Usage:  npx vite-node scripts/generate-fixture-records.ts [--out <path>] [--events]
//   --out     output JSONL path (default: scripts/.generated/fixture-records.jsonl)
//   --events  include live events in the output (default: snapshot records only)

import { writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { allFixtureSnapshotRecords, fixtureLiveEvents } from '../src/fixtures/fixtures';
import type { SnapshotRecord, LiveEvent } from '../src/contract/types';

interface PublishInputLine {
  record_type: 'projection' | 'event';
  record_bytes: string;
}

function emitLine(record: SnapshotRecord | LiveEvent, recordType: 'projection' | 'event'): string {
  const recordBytes = JSON.stringify(record);
  const line: PublishInputLine = { record_type: recordType, record_bytes: recordBytes };
  return JSON.stringify(line);
}

function parseArgs(): { out: string; includeEvents: boolean } {
  const args = process.argv.slice(2);
  let out = resolve(dirname(new URL(import.meta.url).pathname), '.generated/fixture-records.jsonl');
  let includeEvents = false;
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--out' && i + 1 < args.length) {
      out = resolve(args[++i]);
    } else if (args[i] === '--events') {
      includeEvents = true;
    }
  }
  return { out, includeEvents };
}

function main(): void {
  const { out, includeEvents } = parseArgs();
  const lines: string[] = [];
  for (const record of allFixtureSnapshotRecords) {
    lines.push(emitLine(record, 'projection'));
  }
  if (includeEvents) {
    for (const event of fixtureLiveEvents) {
      lines.push(emitLine(event, 'event'));
    }
  }
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(out, lines.join('\n') + '\n', 'utf8');
  const projectionCount = allFixtureSnapshotRecords.length;
  const eventCount = includeEvents ? fixtureLiveEvents.length : 0;
  console.log(`Generated ${projectionCount} projection records and ${eventCount} event records -> ${out}`);
}

main();
