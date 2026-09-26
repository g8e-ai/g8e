// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { evalStore } from '../src/state/store';
import { startFeed, stopFeed } from '../src/state/startup';

const mirrorOrigin = 'http://127.0.0.1:8082';
const sourceId = 'proof-bootstrap-source';
const sliceHash = 'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc';

function requestPath(input: RequestInfo | URL): string {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
  return new URL(url, mirrorOrigin).pathname;
}

function snapshot(highWater = 0) {
  return {
    protocol_version: '1.0.0',
    source_id: sourceId,
    high_water_sequence: highWater,
    feed_chain_hash: 'd'.repeat(64),
    batch_count: 1,
    generated_at: '2026-09-20T00:00:00Z',
    freshness: 'active',
  };
}

describe('proof catalog bootstrap', () => {
  beforeEach(() => {
    stopFeed();
    evalStore.loadFixtures([], []);
  });

  it('indexes proof catalog entries during startup when artifacts are published', async () => {
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const path = requestPath(input);
      if (path === '/runtime.json') {
        return new Response(JSON.stringify({ schema_version: '1.0.0', mirror_origin: mirrorOrigin }), { status: 200 });
      }
      if (path === '/bootstrap') {
        return new Response(JSON.stringify({
          protocol_version: '1.0.0',
          snapshot: snapshot(),
          source_freshness: 'active',
          recent_projections: [],
          proof_catalog_summary: { artifact_count: 1, total_byte_size: 512 },
          generated_at: '2026-09-20T00:00:00Z',
        }), { status: 200 });
      }
      if (path === '/proof-catalog') {
        return new Response(JSON.stringify({
          schema_version: '1.0.0',
          generated_at: '2026-09-20T00:00:00Z',
          entries: [{
            artifact_id: sliceHash,
            filename: 'assignment.db',
            media_type: 'application/vnd.sqlite3',
            byte_size: 512,
            sha256: sliceHash,
            classification: 'public_safe',
            campaign_id: 'campaign-1',
            generated_at: '2026-09-20T00:00:00Z',
            verification_command: 'g8e public verify-assignment --db assignment.db --vault-key assignment.vault.key',
            immutable_url: `/proofs/${sliceHash}`,
          }],
        }), { status: 200 });
      }
      if (path === '/snapshot') {
        return new Response(JSON.stringify(snapshot()), { status: 200 });
      }
      if (path === '/history') {
        return new Response(JSON.stringify({ protocol_version: '1.0.0', items: [], has_more: false, limit: 500 }), { status: 200 });
      }
      return new Response('not found', { status: 404 });
    });

    await startFeed({ fetchImpl: fetchImpl as typeof fetch });

    expect(fetchImpl.mock.calls.some(([url]) => requestPath(url) === '/proof-catalog')).toBe(true);
    expect(evalStore.getState().proofArtifacts.get(sliceHash)?.filename).toBe('assignment.db');
    expect(evalStore.getState().proofArtifactCount).toBe(1);
  });

  it('skips proof catalog fetch when bootstrap reports zero artifacts', async () => {
    const fetchImpl = vi.fn(async (input: RequestInfo | URL) => {
      const path = requestPath(input);
      if (path === '/runtime.json') {
        return new Response(JSON.stringify({ schema_version: '1.0.0', mirror_origin: mirrorOrigin }), { status: 200 });
      }
      if (path === '/bootstrap') {
        return new Response(JSON.stringify({
          protocol_version: '1.0.0',
          snapshot: snapshot(),
          source_freshness: 'active',
          recent_projections: [],
          proof_catalog_summary: { artifact_count: 0, total_byte_size: 0 },
          generated_at: '2026-09-20T00:00:00Z',
        }), { status: 200 });
      }
      if (path === '/snapshot') {
        return new Response(JSON.stringify(snapshot()), { status: 200 });
      }
      if (path === '/history') {
        return new Response(JSON.stringify({ protocol_version: '1.0.0', items: [], has_more: false, limit: 500 }), { status: 200 });
      }
      return new Response('not found', { status: 404 });
    });

    await startFeed({ fetchImpl: fetchImpl as typeof fetch });

    expect(fetchImpl.mock.calls.some(([url]) => requestPath(url) === '/proof-catalog')).toBe(false);
    expect(evalStore.getState().proofArtifacts.size).toBe(0);
  });
});
