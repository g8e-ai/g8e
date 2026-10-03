// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  approveTransaction,
  b64urlToBuffer,
  bufferToB64url,
  decodeCreationOptions,
  encodeAssertion,
  registerEnrollmentPasskey,
} from './webauthn';

const bytes = (...b: number[]) => new Uint8Array(b).buffer;

describe('base64url', () => {
  it('round-trips unpadded base64url including - and _', () => {
    const buf = bytes(0xfb, 0xff, 0xbf, 0x00, 0x01);
    const s = bufferToB64url(buf);
    expect(s).toBe('-_-_AAE');
    expect(s).not.toContain('=');
    expect(new Uint8Array(b64urlToBuffer(s))).toEqual(new Uint8Array(buf));
  });
});

describe('decodeCreationOptions', () => {
  it('decodes challenge, user.id, and excluded credential IDs', () => {
    const opts = decodeCreationOptions({
      challenge: 'AQID',
      user: { id: 'BAU', name: 'n', displayName: 'd' },
      excludeCredentials: [{ id: 'Bg', type: 'public-key' }],
      rp: { id: 'localhost', name: 'g8e' },
    });
    expect(new Uint8Array(opts.challenge as ArrayBuffer)).toEqual(new Uint8Array([1, 2, 3]));
    expect(new Uint8Array(opts.user.id as ArrayBuffer)).toEqual(new Uint8Array([4, 5]));
    expect(new Uint8Array(opts.excludeCredentials![0]!.id as ArrayBuffer)).toEqual(new Uint8Array([6]));
  });
});

function fakeAssertion() {
  return {
    id: 'cred',
    rawId: bytes(1),
    response: {
      clientDataJSON: bytes(2),
      authenticatorData: bytes(3),
      signature: bytes(4),
      userHandle: null,
    },
  } as unknown as PublicKeyCredential;
}

describe('encodeAssertion', () => {
  it('produces the Gateway flat assertion model', () => {
    expect(encodeAssertion(fakeAssertion())).toEqual({
      id: 'cred',
      rawId: 'AQ',
      clientDataJSON: 'Ag',
      authenticatorData: 'Aw',
      signature: 'BA',
      userHandle: null,
    });
  });
});

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('ceremonies', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('sends the approval assertion as the entire verify body', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(200, { publicKey: { challenge: 'AQ', allowCredentials: [] } }))
      .mockResolvedValueOnce(jsonResponse(200, { receipt: {} }));
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('navigator', { credentials: { get: vi.fn().mockResolvedValue(fakeAssertion()) } });

    const out = await approveTransaction('tx/1');

    expect(out.ok).toBe(true);
    const [url, init] = fetchMock.mock.calls[1]!;
    expect(url).toBe('/api/v1/approvals/tx%2F1/verify');
    expect(init.credentials).toBe('include');
    expect(JSON.parse(init.body)).toEqual(encodeAssertion(fakeAssertion()));
  });

  it('maps enrollment token failures to specific messages without pre-validating', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(409, { error: 'conflict' }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(registerEnrollmentPasskey('tok')).rejects.toThrow('already been used');
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/auth/passkeys/enrollment/register/challenge');
    expect(JSON.parse(fetchMock.mock.calls[0]![1].body)).toEqual({ enrollment_token: 'tok' });
  });
});
