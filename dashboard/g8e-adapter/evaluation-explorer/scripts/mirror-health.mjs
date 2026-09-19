// Mirror health check. Reports mirror process, public endpoint reachability,
// snapshot high-water, source freshness, record counts, last accepted record,
// and publisher-to-mirror drift. Also verifies the public boundary: mutation
// routes must be absent from the public listener and unauthenticated ingest
// on the private listener must be rejected.
//
// Never prints secrets, tokens, or private endpoint details.
//
// Usage:  node scripts/mirror-health.mjs
//
// Exits with code 0 if the public listener is reachable and responds to
// bootstrap; exits with code 1 otherwise.

import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(__dirname, '..');

const PUBLIC_ORIGIN = process.env.G8E_PUBLIC_ORIGIN ?? 'http://127.0.0.1:8082';
const PRIVATE_ORIGIN = process.env.G8E_PRIVATE_ORIGIN ?? 'http://127.0.0.1:8081';
const SOURCE_ID = process.env.G8E_SOURCE_ID ?? 'opendevops-local';
const HISTORY_PAGE_LIMIT = 100;
const HISTORY_MAX_PAGES = 50;

async function fetchJSON(url, timeoutMs) {
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    const response = await fetch(url, {
      method: 'GET',
      credentials: 'omit',
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    });
    clearTimeout(timer);
    if (!response.ok) {
      return { ok: false, status: response.status, body: null };
    }
    const body = await response.json();
    return { ok: true, status: response.status, body };
  } catch {
    return { ok: false, status: 0, body: null };
  }
}

async function fetchPOST(url, timeoutMs) {
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    const response = await fetch(url, {
      method: 'POST',
      credentials: 'omit',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
      signal: controller.signal,
    });
    clearTimeout(timer);
    return { ok: response.ok, status: response.status, body: null };
  } catch {
    return { ok: false, status: 0, body: null };
  }
}

// Probe the SSE endpoint: verify it responds with an event stream, then
// abort. Does not consume the stream.
async function probeStream(url, timeoutMs) {
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    const response = await fetch(url, { credentials: 'omit', signal: controller.signal });
    const contentType = response.headers.get('content-type') ?? '';
    const ok = response.ok && contentType.includes('text/event-stream');
    controller.abort();
    clearTimeout(timer);
    return { ok, status: response.status, contentType };
  } catch {
    return { ok: false, status: 0, contentType: '' };
  }
}

function g8eBin() {
  const env = process.env.G8E_BIN;
  if (env) return env;
  let dir = projectRoot;
  for (let i = 0; i < 5; i++) {
    const candidate = resolve(dir, 'g8e');
    if (existsSync(candidate)) return candidate;
    dir = resolve(dir, '..');
  }
  return '';
}

function g8eStatus() {
  const bin = g8eBin();
  if (!bin) return null;
  const cwd = resolve(bin, '..');
  const result = spawnSync(bin, ['public', 'status'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  if (result.status !== 0 || !result.stdout.trim()) return null;
  try {
    return JSON.parse(result.stdout);
  } catch {
    return null;
  }
}

async function countHistory() {
  let cursor = '0';
  let total = 0;
  for (let page = 0; page < HISTORY_MAX_PAGES; page++) {
    const result = await fetchJSON(`${PUBLIC_ORIGIN}/history?source=${SOURCE_ID}&cursor=${cursor}&limit=${HISTORY_PAGE_LIMIT}`, 5000);
    if (!result.ok) return { total: -1, error: result.status };
    const items = Array.isArray(result.body?.items) ? result.body.items : [];
    total += items.length;
    if (!result.body?.has_more || items.length === 0) return { total };
    cursor = String(result.body.cursor ?? items[items.length - 1].sequence);
  }
  return { total, truncated: true };
}

async function main() {
  console.log('=== Mirror Health Check ===');
  console.log(`Public endpoint:  ${PUBLIC_ORIGIN}`);
  console.log(`Private endpoint: ${PRIVATE_ORIGIN} (auth required, not tested for data)`);

  const status = g8eStatus();
  let publisherHighWater = null;
  if (status) {
    publisherHighWater = status.high_water_sequence ?? 0;
    console.log('\n--- Publisher Status ---');
    console.log(`  source_id:           ${status.source_id ?? 'unknown'}`);
    console.log(`  mirror_origin:       ${status.mirror_origin ?? 'unknown'}`);
    console.log(`  enabled:             ${status.enabled ?? 'unknown'}`);
    console.log(`  high_water_sequence: ${status.high_water_sequence ?? 0}`);
    console.log(`  batch_count:         ${status.batch_count ?? 0}`);
    console.log(`  feed_chain_hash:     ${status.feed_chain_hash ?? 'unknown'}`);
  } else {
    console.log('\n--- Publisher Status ---');
    console.log('  (g8e public status unavailable)');
  }

  console.log(`\n--- Public Listener (${PUBLIC_ORIGIN}) ---`);
  const bootstrap = await fetchJSON(`${PUBLIC_ORIGIN}/bootstrap?source=${SOURCE_ID}`, 3000);
  if (!bootstrap.ok) {
    console.log(`  bootstrap:  FAILED (status ${bootstrap.status})`);
    console.log('\n  MIRROR UNHEALTHY: public listener not reachable.');
    process.exit(1);
  }
  const bs = bootstrap.body;
  const snapshot = bs?.snapshot;
  const recent = Array.isArray(bs?.recent_projections) ? bs.recent_projections.length : 0;
  console.log('  bootstrap:  OK');
  console.log(`  freshness:   ${snapshot?.freshness ?? 'unknown'}`);
  console.log(`  high_water:  ${snapshot?.high_water_sequence ?? 0}`);
  console.log(`  batch_count: ${snapshot?.batch_count ?? 0}`);
  console.log(`  recent projections: ${recent}`);
  const catalogSummary = bs?.proof_catalog_summary;
  if (catalogSummary) {
    console.log(`  proof artifacts:    ${catalogSummary.artifact_count ?? 0} (${catalogSummary.total_byte_size ?? 0} bytes)`);
  }

  if (publisherHighWater !== null) {
    const mirrorHighWater = snapshot?.high_water_sequence ?? 0;
    if (mirrorHighWater === publisherHighWater) {
      console.log(`  drift:       none (publisher and mirror at ${mirrorHighWater})`);
    } else {
      console.log(`  drift:       publisher=${publisherHighWater} mirror=${mirrorHighWater} -- run ./g8e public push`);
    }
  }

  const snap = await fetchJSON(`${PUBLIC_ORIGIN}/snapshot?source=${SOURCE_ID}`, 3000);
  console.log(`  snapshot:    ${snap.ok ? 'OK' : `FAILED (status ${snap.status})`}`);

  const counted = await countHistory();
  if (counted.total >= 0) {
    console.log(`  history:     ${counted.total} record(s) total${counted.truncated ? ' (truncated count)' : ''}`);
  } else {
    console.log(`  history:     FAILED (status ${counted.error})`);
  }

  const mirrorHighWater = snapshot?.high_water_sequence ?? 0;
  if (mirrorHighWater > 0) {
    const last = await fetchJSON(`${PUBLIC_ORIGIN}/history?source=${SOURCE_ID}&cursor=${mirrorHighWater - 1}&limit=1`, 3000);
    const lastItem = Array.isArray(last.body?.items) ? last.body.items[0] : null;
    if (lastItem) {
      const label = lastItem.run_id ? ` run=${lastItem.run_id}` : lastItem.dataset_id ? ` dataset=${lastItem.dataset_id}` : '';
      console.log(`  last record: seq=${lastItem.sequence} type=${lastItem.record_type} kind=${lastItem.kind ?? 'n/a'}${label}`);
    }
  }

  const proofCatalog = await fetchJSON(`${PUBLIC_ORIGIN}/proof-catalog`, 3000);
  console.log(`  proof-catalog:  ${proofCatalog.ok ? 'OK' : `status ${proofCatalog.status}`}`);
  const proofManifest = await fetchJSON(`${PUBLIC_ORIGIN}/proof-manifest`, 3000);
  console.log(`  proof-manifest: ${proofManifest.ok ? 'OK' : `status ${proofManifest.status} (absent until proofs are published)`}`);

  const stream = await probeStream(`${PUBLIC_ORIGIN}/stream?source=${SOURCE_ID}&since_id=0`, 3000);
  console.log(`  stream:      ${stream.ok ? 'OK (text/event-stream)' : `FAILED (status ${stream.status})`}`);

  console.log('\n--- Public Boundary ---');
  for (const route of ['/ingest', '/keys/register', '/proof-ingest']) {
    const result = await fetchJSON(`${PUBLIC_ORIGIN}${route}`, 2000);
    const isolated = result.status === 404;
    console.log(`  ${route}: ${isolated ? '404 (isolated)' : `status ${result.status} (${isolated ? 'ok' : 'NOT ISOLATED'})`}`);
  }

  const privateIngest = await fetchPOST(`${PRIVATE_ORIGIN}/ingest`, 2000);
  console.log(`  private /ingest no-auth: ${privateIngest.status === 401 ? '401 (auth enforced)' : `status ${privateIngest.status}`}`);

  console.log('\n=== Health Check Complete ===');
  process.exit(0);
}

main().catch((err) => {
  console.error('Health check failed:', err);
  process.exit(1);
});
