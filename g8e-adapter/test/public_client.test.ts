// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it, vi } from 'vitest';

import { PublicClientError, createPublicClient, parsePublicRuntimeConfig } from '../src/public';

const snapshot = {
  protocol_version: '1.0.0',
  source_id: 'deployment-a',
  high_water_sequence: 0,
  feed_chain_hash: '0'.repeat(64),
  batch_count: 0,
  generated_at: '2026-09-13T00:00:00Z',
  freshness: 'source_offline',
};

describe('public spectator typed client', () => {
  it('reads typed bootstrap, history, and snapshot responses anonymously', async () => {
    const artifactID = 'a'.repeat(64);
    const responses = [
      { protocol_version: '1.0.0', snapshot, source_freshness: 'source_offline', recent_projections: [{ sequence: 1, kind: 'catalog_snapshot' }], proof_catalog_summary: { artifact_count: 0, total_byte_size: 0 }, generated_at: '2026-09-13T00:00:00Z' },
      { protocol_version: '1.0.0', items: [], has_more: false, limit: 20 },
      snapshot,
      snapshot,
      { schema_version: '1.0.0', generated_at: '2026-09-13T00:00:00Z', entries: [{ artifact_id: artifactID, filename: 'proof.db', media_type: 'application/vnd.sqlite3', byte_size: 12, sha256: artifactID, classification: 'public_safe', campaign_id: 'campaign-1', generated_at: '2026-09-13T00:00:00Z', verification_command: 'g8e public verify-assignment --db proof.db --vault-key proof.vault.key', immutable_url: `/proofs/${artifactID}` }] },
    ];
    const fetchImpl = vi.fn<typeof fetch>().mockImplementation(async () => new Response(JSON.stringify(responses.shift()), { status: 200 }));
    const client = createPublicClient(parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: 'https://feed.example.com' }), fetchImpl);

    await expect(client.bootstrap('deployment-a')).resolves.toMatchObject({ source_freshness: 'source_offline' });
    await expect(client.history('deployment-a', 0, 20)).resolves.toMatchObject({ has_more: false });
    await expect(client.snapshot('deployment-a')).resolves.toMatchObject({ high_water_sequence: 0 });
    await expect(client.snapshotAt('deployment-a', 20)).resolves.toMatchObject({ high_water_sequence: 0 });
    await expect(client.proofCatalog('deployment-a')).resolves.toMatchObject({ entries: [{ filename: 'proof.db' }] });
    expect(fetchImpl.mock.calls[3]?.[0]).toBe('https://feed.example.com/snapshot?source=deployment-a&sequence=20');
    expect(fetchImpl.mock.calls[4]?.[0]).toBe('https://feed.example.com/proof-catalog?source=deployment-a');
    for (const call of fetchImpl.mock.calls) expect(call[1]).toMatchObject({ credentials: 'omit', method: 'GET' });
  });

  it('fails closed on malformed or unsuccessful mirror responses', async () => {
    const malformed = vi.fn<typeof fetch>().mockResolvedValue(new Response('{"high_water_sequence":"bad"}', { status: 200 }));
    const rejected = vi.fn<typeof fetch>().mockResolvedValue(new Response('{"error":"rate limited"}', { status: 429 }));
    const config = parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: 'https://feed.example.com' });

    await expect(createPublicClient(config, malformed).snapshot()).rejects.toBeInstanceOf(PublicClientError);
    await expect(createPublicClient(config, rejected).snapshot()).rejects.toMatchObject({ status: 429 });
  });
});
