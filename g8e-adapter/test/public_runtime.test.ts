// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it, vi } from 'vitest';

import {
  PublicEndpointNotAllowedError,
  PublicRuntimeConfigError,
  createPublicFetch,
  parsePublicRuntimeConfig,
} from '../src/public';

describe('public spectator runtime boundary', () => {
  it('accepts only a mirror origin', () => {
    expect(parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: 'https://feed.example.com' })).toEqual({
      schema_version: '1.0.0',
      mirror_origin: 'https://feed.example.com',
    });
  });

  it.each(['gateway_base_url', 'passkey_rp_id', 'credentials', 'token', 'producer_url', 'fallback_origin'])(
    'rejects the private or credential field %s',
    (field) => {
      expect(() => parsePublicRuntimeConfig({
        schema_version: '1.0.0',
        mirror_origin: 'https://feed.example.com',
        [field]: 'forbidden',
      })).toThrow(PublicRuntimeConfigError);
    },
  );

  it('allows loopback HTTP and rejects remote HTTP, paths, queries, fragments, and userinfo', () => {
    expect(parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: 'http://127.0.0.1:8081' }).mirror_origin).toBe('http://127.0.0.1:8081');
    for (const mirror_origin of [
      'http://feed.example.com',
      'https://feed.example.com/path',
      'https://feed.example.com?query=1',
      'https://feed.example.com#fragment',
      'https://user:password@feed.example.com',
    ]) {
      expect(() => parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin })).toThrow(PublicRuntimeConfigError);
    }
  });

  it('omits credentials on every allowlisted request', async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(new Response('{"protocol_version":"1.0.0"}', {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }));
    const publicFetch = createPublicFetch(
      parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: 'https://feed.example.com' }),
      fetchImpl,
    );

    await publicFetch('snapshot', '?source=deployment-a');

    expect(fetchImpl).toHaveBeenCalledWith('https://feed.example.com/snapshot?source=deployment-a', expect.objectContaining({
      method: 'GET',
      credentials: 'omit',
    }));
  });

  it('rejects private, producer, ingest, and arbitrary mirror paths before fetch', async () => {
    const fetchImpl = vi.fn<typeof fetch>();
    const publicFetch = createPublicFetch(
      parsePublicRuntimeConfig({ schema_version: '1.0.0', mirror_origin: 'https://feed.example.com' }),
      fetchImpl,
    );

    for (const path of ['/api/v1/health', '/ingest', '/proof-ingest', '/keys/register', '/arbitrary']) {
      await expect(publicFetch('snapshot', path)).rejects.toBeInstanceOf(PublicEndpointNotAllowedError);
    }
    expect(fetchImpl).not.toHaveBeenCalled();
  });
});
