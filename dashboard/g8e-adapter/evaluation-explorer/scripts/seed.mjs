// Seed the local public mirror with projection records for local development.
//
// Real campaign and native evaluation data is published by the Go evaluation
// service through `g8e eval campaign execute --publish` or
// `g8e eval campaign publish`. This script only loads pre-generated JSONL
// (fixtures or an explicit --records file) and publishes it through the real
// `g8e public publish` CLI in batches that fit the configured feed limits
// (max 100 records / 4 MiB of record bytes per batch), followed by
// `g8e public push` and `g8e public status`.
//
// Reseeding is idempotent: if the mirror already holds every dataset with the
// identical catalog_snapshot content, publishing is skipped. If the mirror
// holds a stale or partial copy of a seeded dataset, the disposable local
// feed state is reset (mirror restart + `g8e public init`) before publishing,
// so duplicate or conflicting logical identities are never produced.
//
// Usage:  node scripts/seed.mjs [--reset] [--records <path>] [--fixtures] [--append-revision <version>]
//   --reset      force reset of disposable local public-feed state first
//   --records    publish a pre-generated JSONL file
//   --fixtures   publish the deterministic TypeScript fixtures (mock dev only)
//   --append-revision appends one exact schema revision without resetting prior history
//
// Environment:
//   G8E_BIN          path to the g8e binary (auto-detected if unset)
//   G8E_CWD          working directory for g8e commands (default: repo root)
//   G8E_PRIVATE_PORT private ingest listener port (default: 8081)
//   G8E_PUBLIC_PORT  public read-only listener port (default: 8082)

import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(__dirname, '..');

const PRIVATE_PORT = process.env.G8E_PRIVATE_PORT ?? '8081';
const PUBLIC_PORT = process.env.G8E_PUBLIC_PORT ?? '8082';
const PUBLIC_ORIGIN = `http://127.0.0.1:${PUBLIC_PORT}`;
const SOURCE_ID = 'opendevops-local';
// Fallback limits; the actual limits are read from the export config.
const DEFAULT_BATCH_MAX_RECORDS = 100;
const DEFAULT_BATCH_MAX_BYTES = 4 << 20;

function g8eBin() {
  const env = process.env.G8E_BIN;
  if (env) return env;
  let dir = projectRoot;
  for (let i = 0; i < 5; i++) {
    const candidate = resolve(dir, 'g8e');
    if (existsSync(candidate)) return candidate;
    dir = resolve(dir, '..');
  }
  throw new Error('g8e binary not found; set G8E_BIN to the full path');
}

function g8eCwd() {
  return process.env.G8E_CWD ?? resolve(g8eBin(), '..');
}

function runG8e(args) {
  const result = spawnSync(g8eBin(), args, { cwd: g8eCwd(), encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.status !== 0) {
    throw new Error(`g8e ${args.join(' ')} exited with code ${result.status}`);
  }
  return result.stdout ?? '';
}

function g8eStatus() {
  const result = spawnSync(g8eBin(), ['public', 'status'], {
    cwd: g8eCwd(), encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
  });
  if (result.status !== 0 || !result.stdout || !result.stdout.trim()) return null;
  try {
    return JSON.parse(result.stdout);
  } catch {
    return null;
  }
}

function checkPort(port) {
  const result = spawnSync('ss', ['-ltn', `sport = :${port}`], { encoding: 'utf8' });
  if (result.status !== 0) return false;
  return result.stdout.includes(`:${port}`);
}

function waitForPort(port, label, timeoutMs) {
  return new Promise((resolvePromise, reject) => {
    const deadline = Date.now() + timeoutMs;
    const check = () => {
      if (checkPort(port)) return resolvePromise();
      if (Date.now() > deadline) return reject(new Error(`${label} on port ${port} did not start within ${timeoutMs}ms`));
      setTimeout(check, 200);
    };
    check();
  });
}

function waitForPortFree(port, timeoutMs) {
  return new Promise((resolvePromise, reject) => {
    const deadline = Date.now() + timeoutMs;
    const check = () => {
      if (!checkPort(port)) return resolvePromise();
      if (Date.now() > deadline) return reject(new Error(`port ${port} still in use after ${timeoutMs}ms`));
      setTimeout(check, 200);
    };
    check();
  });
}

function mirrorRunning() {
  return checkPort(PRIVATE_PORT) && checkPort(PUBLIC_PORT);
}

function stopMirror() {
}

function startMirrorDetached() {
  const proc = spawn(g8eBin(), [
    'public', 'mirror', 'run',
    '--listen', `127.0.0.1:${PRIVATE_PORT}`,
    '--public-listen', `127.0.0.1:${PUBLIC_PORT}`,
  ], { cwd: g8eCwd(), detached: true, stdio: 'ignore' });
  proc.unref();
}

async function ensureMirror() {
  if (mirrorRunning()) return;
  console.log('Mirror not running; starting it detached...');
  startMirrorDetached();
  await waitForPort(PUBLIC_PORT, 'mirror public listener', 10000);
  if (!checkPort(PRIVATE_PORT)) {
    throw new Error(`mirror private listener on port ${PRIVATE_PORT} did not start`);
  }
  console.log(`Mirror is up on ${PRIVATE_PORT} (private) and ${PUBLIC_PORT} (public).`);
}

async function stopMirrorAndWait() {
  if (!checkPort(PRIVATE_PORT) && !checkPort(PUBLIC_PORT)) return;
  console.log('Stopping running mirror so feed state can be reset...');
  stopMirror();
  await waitForPortFree(PRIVATE_PORT, 10000);
  await waitForPortFree(PUBLIC_PORT, 10000);
}

function feedDir() {
  return resolve(g8eCwd(), '.g8e/public-feed');
}

function mirrorStatePath() {
  return resolve(g8eCwd(), '.g8e/public-mirror/state.json');
}

function initFeed() {
  runG8e(['public', 'init', '--source-id', SOURCE_ID, '--mirror-origin', `http://127.0.0.1:${PRIVATE_PORT}`]);
}

async function resetFeedState() {
  await stopMirrorAndWait();
  for (const dir of [feedDir(), resolve(g8eCwd(), '.g8e/public-mirror')]) {
    if (existsSync(dir)) rmSync(dir, { recursive: true, force: true });
  }
  console.log('Reset disposable local public-feed and mirror state.');
  initFeed();
  await ensureMirror();
}

function generateFixtureRecords() {
  const outPath = resolve(__dirname, '.generated/fixture-records.jsonl');
  const viteNode = resolve(projectRoot, 'node_modules/.bin/vite-node');
  if (!existsSync(viteNode)) {
    throw new Error('vite-node not found; run npm install in the frontend project first');
  }
  const result = spawnSync(viteNode, [resolve(__dirname, 'generate-fixture-records.ts'), '--out', outPath], {
    cwd: projectRoot, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
  });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.status !== 0) {
    throw new Error(`fixture generation exited with code ${result.status}`);
  }
  return outPath;
}

// Load publish-input lines and index the dataset identities plus the
// per-dataset catalog_snapshot record hash. The record hash is what the
// mirror stores per record, so its presence in mirror state proves the
// identical record was already accepted.
function loadRecords(recordsPath) {
  const content = readFileSync(recordsPath, 'utf8');
  const lines = content.split('\n').filter((line) => line.trim().length > 0);
  if (lines.length === 0) throw new Error(`records file is empty: ${recordsPath}`);
  const datasetIds = new Set();
  const catalogHashes = new Map();
  const schemaVersions = new Set();
  const entries = lines.map((line, index) => {
    let parsed;
    try {
      parsed = JSON.parse(line);
    } catch (err) {
      throw new Error(`records file line ${index + 1}: invalid JSON: ${err.message}`);
    }
    if (typeof parsed.record_type !== 'string' || typeof parsed.record_bytes !== 'string') {
      throw new Error(`records file line ${index + 1}: missing record_type/record_bytes`);
    }
    let record;
    let datasetId;
    try {
      record = JSON.parse(parsed.record_bytes);
    } catch {
      record = null;
    }
    if (record && typeof record.schema_version === 'string') schemaVersions.add(record.schema_version);
    if (record && typeof record.dataset_id === 'string') {
      datasetId = record.dataset_id;
      datasetIds.add(datasetId);
      if (record.kind === 'catalog_snapshot' && !catalogHashes.has(datasetId)) {
        const hash = createHash('sha256').update(parsed.record_bytes, 'utf8').digest('hex');
        catalogHashes.set(datasetId, hash);
      }
    }
    return { line, bytes: Buffer.byteLength(parsed.record_bytes, 'utf8'), datasetId };
  });
  return { entries, datasetIds, catalogHashes, schemaVersions };
}

// Read the mirror's stored records for this source from
// .g8e/public-mirror/state.json. Each entry has
// {sequence, record_type, record_hash, record_bytes}.
function mirrorSourceRecords() {
  try {
    const state = JSON.parse(readFileSync(mirrorStatePath(), 'utf8'));
    const records = state?.sources?.[SOURCE_ID]?.records;
    return Array.isArray(records) ? records : [];
  } catch {
    return [];
  }
}

// Classify each dataset in the records file against the mirror state:
//   seeded  - every record present and the catalog content hash matches
//   absent  - no records present
//   drifted - partially present, or present with different catalog content
function classifyDatasets(mirrorRecords, entries, datasetIds, catalogHashes) {
  const result = { seeded: [], absent: [], drifted: [] };
  const mirrorHashes = new Set(mirrorRecords.map((r) => r.record_hash));
  for (const id of datasetIds) {
    const expected = entries.filter((e) => e.datasetId === id).length;
    const needle = `"dataset_id":"${id}"`;
    const present = mirrorRecords.filter((r) => typeof r.record_bytes === 'string' && r.record_bytes.includes(needle)).length;
    const catalogHash = catalogHashes.get(id);
    const catalogVerified = catalogHash !== undefined ? mirrorHashes.has(catalogHash) : true;
    if (present === 0) {
      result.absent.push(id);
    } else if (present === expected && catalogVerified) {
      result.seeded.push(id);
    } else {
      result.drifted.push({ id, expected, present });
    }
  }
  return result;
}

function exportConfigLimits() {
  try {
    const raw = JSON.parse(readFileSync(resolve(feedDir(), 'export-config.json'), 'utf8'));
    return {
      maxRecords: raw.batch_max_records ?? DEFAULT_BATCH_MAX_RECORDS,
      maxBytes: raw.batch_max_bytes ?? DEFAULT_BATCH_MAX_BYTES,
    };
  } catch {
    return { maxRecords: DEFAULT_BATCH_MAX_RECORDS, maxBytes: DEFAULT_BATCH_MAX_BYTES };
  }
}

function chunkEntries(entries, maxRecords, maxBytes) {
  const chunks = [];
  let current = [];
  let currentBytes = 0;
  for (const entry of entries) {
    if (entry.bytes > maxBytes) {
      throw new Error(`single record_bytes exceeds batch byte limit (${entry.bytes} > ${maxBytes})`);
    }
    if (current.length >= maxRecords || currentBytes + entry.bytes > maxBytes) {
      chunks.push(current);
      current = [];
      currentBytes = 0;
    }
    current.push(entry);
    currentBytes += entry.bytes;
  }
  if (current.length > 0) chunks.push(current);
  return chunks;
}

async function fetchSnapshot() {
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 3000);
    const response = await fetch(`${PUBLIC_ORIGIN}/snapshot?source=${SOURCE_ID}`, {
      credentials: 'omit', signal: controller.signal,
    });
    clearTimeout(timer);
    if (!response.ok) return null;
    return await response.json();
  } catch {
    return null;
  }
}

function publishChunks(entries) {
  const { maxRecords, maxBytes } = exportConfigLimits();
  const chunks = chunkEntries(entries, maxRecords, maxBytes);
  console.log(`Publishing ${entries.length} records in ${chunks.length} batch(es) (max ${maxRecords} records / ${maxBytes} bytes per batch)...`);
  const tmpDir = mkdtempSync(resolve(tmpdir(), 'g8e-seed-'));
  try {
    for (let i = 0; i < chunks.length; i++) {
      const chunkPath = resolve(tmpDir, `seed-batch-${String(i + 1).padStart(3, '0')}.jsonl`);
      writeFileSync(chunkPath, chunks[i].map((e) => e.line).join('\n') + '\n', 'utf8');
      runG8e(['public', 'publish', chunkPath]);
    }
  } finally {
    rmSync(tmpDir, { recursive: true, force: true });
  }
  runG8e(['public', 'push']);
}

function parseArgs() {
  const args = process.argv.slice(2);
  let reset = false;
  let records = null;
  let fixtures = false;
  let appendRevision = null;
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--reset') {
      reset = true;
    } else if (args[i] === '--records' && i + 1 < args.length) {
      records = resolve(args[++i]);
    } else if (args[i] === '--fixtures') {
      fixtures = true;
    } else if (args[i] === '--append-revision' && i + 1 < args.length) {
      appendRevision = args[++i];
    }
  }
  return { reset, records, fixtures, appendRevision };
}

async function main() {
  const { reset, records, fixtures, appendRevision } = parseArgs();
  if (appendRevision && (reset || fixtures || !records)) {
    throw new Error('--append-revision requires --records and cannot be combined with --reset or --fixtures');
  }

  if (!records && !fixtures) {
    throw new Error('Specify --fixtures for mock fixture seeding or --records <path> for a pre-generated JSONL file. Real evaluations publish through g8e eval campaign publish.');
  }
  const recordsPath = records ?? generateFixtureRecords();
  if (!existsSync(recordsPath)) {
    throw new Error(`records file not found: ${recordsPath}`);
  }
  const { entries, datasetIds, catalogHashes, schemaVersions } = loadRecords(recordsPath);
  console.log(`Loaded ${entries.length} records from ${recordsPath}`);
  console.log(`Datasets: ${[...datasetIds].join(', ')}`);

  if (appendRevision) {
    if (schemaVersions.size !== 1 || !schemaVersions.has(appendRevision)) {
      throw new Error(`--append-revision ${appendRevision} requires every record to carry that exact schema version`);
    }
    await ensureMirror();
    const status = g8eStatus();
    const mirrorSnap = await fetchSnapshot();
    if (!status || !mirrorSnap || status.high_water_sequence !== mirrorSnap.high_water_sequence) {
      throw new Error('publisher and mirror must be synchronized before appending a schema revision');
    }
    publishChunks(entries);
  } else if (reset) {
    await resetFeedState();
  } else {
    await ensureMirror();
    const status = g8eStatus();
    const publisherHighWater = status?.high_water_sequence ?? 0;
    const classified = classifyDatasets(mirrorSourceRecords(), entries, datasetIds, catalogHashes);

    if (classified.drifted.length > 0) {
      for (const d of classified.drifted) {
        console.log(`Dataset ${d.id} is stale or partial in the mirror (${d.present}/${d.expected} records or changed catalog); resetting disposable feed state.`);
      }
      await resetFeedState();
    } else if (classified.seeded.length === datasetIds.size) {
      console.log('All seed datasets are already present with identical catalog content; skipping publish.');
      runG8e(['public', 'push']);
    } else {
      const mirrorSnap = await fetchSnapshot();
      const mirrorHighWater = mirrorSnap?.high_water_sequence ?? 0;
      if (publisherHighWater !== mirrorHighWater) {
        console.log(`Publisher high-water (${publisherHighWater}) differs from mirror (${mirrorHighWater}); retrying outbox.`);
        try {
          runG8e(['public', 'push']);
        } catch (err) {
          console.error(`push failed: ${err.message}`);
        }
        const retrySnap = await fetchSnapshot();
        if ((retrySnap?.high_water_sequence ?? 0) !== publisherHighWater) {
          console.log('Mirror could not be reconciled from the outbox; resetting disposable feed state.');
          await resetFeedState();
        }
      }
    }
  }

  const absentNow = classifyDatasets(mirrorSourceRecords(), entries, datasetIds, catalogHashes).absent;
  const absentSet = new Set(absentNow);
  const entriesToPublish = entries.filter((e) => e.datasetId === undefined || absentSet.has(e.datasetId));
  if (entriesToPublish.length > 0) {
    publishChunks(entriesToPublish);
  } else {
    console.log('Nothing to publish.');
  }

  console.log('--- status ---');
  runG8e(['public', 'status']);
  const snap = await fetchSnapshot();
  const status = g8eStatus();
  if (snap && status && snap.high_water_sequence !== status.high_water_sequence) {
    console.warn(`WARNING: mirror high-water ${snap.high_water_sequence} != publisher high-water ${status.high_water_sequence}; run ./g8e public push`);
  }
  console.log('Seed complete.');
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
