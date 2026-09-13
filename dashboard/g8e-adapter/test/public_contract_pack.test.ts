import { describe, expect, it } from 'vitest';
import { createHash, createPublicKey, verify } from 'node:crypto';
import { readFileSync, readdirSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

import { isPublicFeedBatch, isPublicFeedBootstrap } from '../contract-pack/public/models';

const adapterRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const publicPack = join(adapterRoot, 'contract-pack', 'public');

function readJson(name: string): Record<string, unknown> {
  return JSON.parse(readFileSync(join(publicPack, name), 'utf8'));
}

describe('public spectator contract pack', () => {
  it('contains a mirror-only runtime schema', () => {
    const schema = readJson('runtime-config.schema.json') as {
      additionalProperties: boolean;
      required: string[];
      properties: Record<string, unknown>;
    };
    expect(schema.additionalProperties).toBe(false);
    expect(schema.required).toEqual(['schema_version', 'mirror_origin']);
    expect(Object.keys(schema.properties).sort()).toEqual(['mirror_origin', 'schema_version']);
  });

  it('exposes only anonymous read operations', () => {
    const openapi = readJson('public-read.openapi.json') as { paths: Record<string, Record<string, unknown>> };
    expect(Object.keys(openapi.paths).sort()).toEqual([
      '/bootstrap',
      '/history',
      '/proof-catalog',
      '/proof-manifest',
      '/proofs/{artifact_id}',
      '/snapshot',
      '/stream',
    ]);
    for (const methods of Object.values(openapi.paths)) expect(Object.keys(methods)).toEqual(['get']);
    expect(JSON.stringify(openapi)).not.toMatch(/ingest|gateway|passkey|credential|producer/i);
  });

  it('contains disclosure, proof, event, verifier, and honest-state fixtures', () => {
    const disclosure = readJson('disclosure-matrix.json') as { default: string; prohibited_fields: string[] };
    expect(disclosure.default).toBe('prohibited');
    expect(disclosure.prohibited_fields).toEqual(expect.arrayContaining(['raw_prompt', 'model_output', 'private_key', 'token', 'gateway_url']));
    expect(() => readJson('event-schemas.json')).not.toThrow();
    expect(() => readJson('proof-schemas.json')).not.toThrow();
    expect(readFileSync(join(publicPack, 'offline-verifier.md'), 'utf8')).toContain('Ed25519');
    expect(readdirSync(join(publicPack, 'fixtures')).sort()).toEqual([
      'active.json',
      'delayed.json',
      'intentionally-stopped.json',
      'safety-stopped.json',
      'source-offline.json',
      'stale.json',
    ]);
  });

  it('ships structurally valid fixtures with verifiable batch hashes and signatures', () => {
    const fixture = JSON.parse(readFileSync(join(publicPack, 'fixtures', 'active.json'), 'utf8')) as {
      fixture_public_key: string;
      signed_batch: Record<string, unknown>;
      bootstrap: unknown;
    };
    expect(isPublicFeedBatch(fixture.signed_batch)).toBe(true);
    expect(isPublicFeedBootstrap(fixture.bootstrap)).toBe(true);
    const batch = fixture.signed_batch as {
      protocol_version: string;
      schema_version: string;
      source_id: string;
      first_sequence: number;
      last_sequence: number;
      previous_batch_hash: string;
      record_hashes: string[];
      generated_at: string;
      signing_key_id: string;
      content_hash: string;
      signature: string;
      records: Array<{ record_hash: string; record_bytes: string }>;
    };
    expect(createHash('sha256').update(batch.records[0].record_bytes).digest('hex')).toBe(batch.records[0].record_hash);
    const hash = createHash('sha256');
    for (const value of [batch.protocol_version, batch.schema_version, batch.source_id, String(batch.first_sequence), String(batch.last_sequence), batch.previous_batch_hash, ...batch.record_hashes, batch.generated_at, batch.signing_key_id]) hash.update(value);
    expect(hash.digest('hex')).toBe(batch.content_hash);
    const publicKey = createPublicKey({ key: Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), Buffer.from(fixture.fixture_public_key, 'hex')]), format: 'der', type: 'spki' });
    expect(verify(null, Buffer.from(batch.content_hash, 'hex'), publicKey, Buffer.from(batch.signature, 'hex'))).toBe(true);
  });
});
