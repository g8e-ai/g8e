// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';

import type { CredentialedFetch, GatewayResponse } from '../src/api/fetch';
import {
  authenticatePasskey,
  enrollPasskey,
  registerPasskey,
  stripUrlFragment,
} from '../src/webauthn/ceremonies';

// Mock navigator.credentials.create/get with fake PublicKeyCredential objects.
function mockCredentialsCreate(credential: PublicKeyCredential | null) {
  const create = vi.fn(async () => credential);
  const get = vi.fn(async () => credential);
  Object.defineProperty(globalThis, 'navigator', {
    value: { credentials: { create, get } },
    writable: true,
    configurable: true,
  });
  return { create, get };
}

function makeFakeCredential(): PublicKeyCredential {
  const rawId = new Uint8Array([1, 2, 3, 4]).buffer;
  const response = {
    clientDataJSON: new Uint8Array([10, 20, 30]).buffer,
    attestationObject: new Uint8Array([40, 50, 60]).buffer,
    authenticatorData: new Uint8Array([70, 80, 90]).buffer,
    signature: new Uint8Array([100, 110, 120]).buffer,
    userHandle: new Uint8Array([130, 140]).buffer,
    getTransports: () => ['usb', 'nfc'],
  };
  return {
    id: 'cred-id-123',
    rawId,
    response: response as never,
    type: 'public-key',
    getClientExtensionResults: () => ({}),
  } as PublicKeyCredential;
}

function makeFetchMock(
  challengeBody: unknown,
  verifyBody: unknown,
  challengeStatus = 200,
  verifyStatus = 200,
): CredentialedFetch {
  return vi.fn(async (method: string, path: string): Promise<GatewayResponse<unknown>> => {
    if (path.endsWith('/challenge')) {
      return { status: challengeStatus, ok: challengeStatus >= 200 && challengeStatus < 300, body: challengeBody, headers: new Headers() };
    }
    if (path.endsWith('/verify')) {
      return { status: verifyStatus, ok: verifyStatus >= 200 && verifyStatus < 300, body: verifyBody, headers: new Headers() };
    }
    return { status: 404, ok: false, body: undefined, headers: new Headers() };
  }) as unknown as CredentialedFetch;
}

function makeChallengeResponse(publicKey: Record<string, unknown> = {}, userId?: string) {
  return {
    success: true,
    options: { publicKey: { challenge: 'aGVsbG8', user: { id: 'dXNlcg', name: 'U', displayName: 'U' }, pubKeyCredParams: [], ...publicKey } },
    ...(userId ? { user_id: userId } : {}),
  };
}

describe('WebAuthn ceremonies', () => {
  describe('registerPasskey', () => {
    it('completes a successful registration', async () => {
      const { create } = mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(
        makeChallengeResponse({}, 'user-123'),
        { success: true },
      );
      const result = await registerPasskey(fetchImpl, { user_id: 'u1', user_name: 'U', cli_session_id: 'browser' });
      expect(result.success).toBe(true);
      expect(result.user_id).toBe('user-123');
      expect(create).toHaveBeenCalledOnce();
    });

    it('sends flat attestation_response with id, rawId, clientDataJSON, attestationObject, transports', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(makeChallengeResponse(), { success: true }) as unknown as ReturnType<typeof vi.fn>;
      await registerPasskey(fetchImpl, { user_id: 'u1', user_name: 'U', cli_session_id: 'browser' });
      const verifyCall = (fetchImpl as unknown as { mock: { calls: Array<unknown[]> } }).mock.calls.find(
        (c) => (c[1] as string).endsWith('/verify'),
      );
      expect(verifyCall).toBeDefined();
      const body = (verifyCall![2] as { body: Record<string, unknown> }).body;
      expect(body.user_id).toBe('u1');
      expect(body.cli_session_id).toBe('browser');
      const att = body.attestation_response as Record<string, unknown>;
      expect(att.id).toBe('cred-id-123');
      expect(att.rawId).toBe('AQIDBA');
      expect(att.clientDataJSON).toBe('ChQe');
      expect(att.attestationObject).toBe('KDI8');
      expect(att.transports).toEqual(['usb', 'nfc']);
    });

    it('fails when challenge request fails', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(undefined, { success: true }, 500);
      const result = await registerPasskey(fetchImpl, { user_id: 'u1', user_name: 'U', cli_session_id: 'browser' });
      expect(result.success).toBe(false);
      expect(result.error).toBe('challenge request failed');
    });

    it('fails when challenge response is not successful', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock({ success: false, error: 'bad' }, { success: true });
      const result = await registerPasskey(fetchImpl, { user_id: 'u1', user_name: 'U', cli_session_id: 'browser' });
      expect(result.success).toBe(false);
      expect(result.error).toBe('bad');
    });

    it('fails when credential creation is cancelled', async () => {
      mockCredentialsCreate(null);
      const fetchImpl = makeFetchMock(makeChallengeResponse(), { success: true });
      const result = await registerPasskey(fetchImpl, { user_id: 'u1', user_name: 'U', cli_session_id: 'browser' });
      expect(result.success).toBe(false);
      expect(result.error).toBe('credential creation cancelled');
    });

    it('fails when verification fails', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(makeChallengeResponse(), { success: false, error: 'rejected' });
      const result = await registerPasskey(fetchImpl, { user_id: 'u1', user_name: 'U', cli_session_id: 'browser' });
      expect(result.success).toBe(false);
    });
  });

  describe('authenticatePasskey', () => {
    it('completes a successful authentication', async () => {
      const { get } = mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(
        { success: true, options: { publicKey: { challenge: 'aGVsbG8' } } },
        { success: true },
      );
      const result = await authenticatePasskey(fetchImpl, 'user-123');
      expect(result.success).toBe(true);
      expect(result.user_id).toBe('user-123');
      expect(get).toHaveBeenCalledOnce();
    });

    it('requires user_id', async () => {
      // The caller must provide user_id; the function signature enforces it.
      // This test verifies the challenge body includes user_id.
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(
        { success: true, options: { publicKey: { challenge: 'aGVsbG8' } } },
        { success: true },
      ) as unknown as ReturnType<typeof vi.fn>;
      await authenticatePasskey(fetchImpl, 'user-456');
      const challengeCall = (fetchImpl as unknown as { mock: { calls: Array<unknown[]> } }).mock.calls.find(
        (c) => (c[1] as string).endsWith('/challenge'),
      );
      const body = (challengeCall![2] as { body: Record<string, unknown> }).body;
      expect(body.user_id).toBe('user-456');
    });

    it('sends flat assertion_response with id, rawId, clientDataJSON, authenticatorData, signature, userHandle', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(
        { success: true, options: { publicKey: { challenge: 'aGVsbG8' } } },
        { success: true },
      ) as unknown as ReturnType<typeof vi.fn>;
      await authenticatePasskey(fetchImpl, 'user-123');
      const verifyCall = (fetchImpl as unknown as { mock: { calls: Array<unknown[]> } }).mock.calls.find(
        (c) => (c[1] as string).endsWith('/verify'),
      );
      expect(verifyCall).toBeDefined();
      const body = (verifyCall![2] as { body: Record<string, unknown> }).body;
      expect(body.user_id).toBe('user-123');
      const assertion = body.assertion_response as Record<string, unknown>;
      expect(assertion.id).toBe('cred-id-123');
      expect(assertion.rawId).toBe('AQIDBA');
      expect(assertion.clientDataJSON).toBe('ChQe');
      expect(assertion.authenticatorData).toBe('RlBa');
      expect(assertion.signature).toBe('ZG54');
      expect(assertion.userHandle).toBe('gow');
    });

    it('sends null userHandle when not available', async () => {
      const credential = makeFakeCredential();
      (credential.response as { userHandle: ArrayBuffer | null }).userHandle = null;
      mockCredentialsCreate(credential);
      const fetchImpl = makeFetchMock(
        { success: true, options: { publicKey: { challenge: 'aGVsbG8' } } },
        { success: true },
      ) as unknown as ReturnType<typeof vi.fn>;
      await authenticatePasskey(fetchImpl, 'user-123');
      const verifyCall = (fetchImpl as unknown as { mock: { calls: Array<unknown[]> } }).mock.calls.find(
        (c) => (c[1] as string).endsWith('/verify'),
      );
      const body = (verifyCall![2] as { body: Record<string, unknown> }).body;
      const assertion = body.assertion_response as Record<string, unknown>;
      expect(assertion.userHandle).toBeNull();
    });

    it('fails when challenge request fails', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(undefined, { success: true }, 500);
      const result = await authenticatePasskey(fetchImpl, 'user-123');
      expect(result.success).toBe(false);
    });

    it('fails when authentication is cancelled', async () => {
      mockCredentialsCreate(null);
      const fetchImpl = makeFetchMock(
        { success: true, options: { publicKey: { challenge: 'aGVsbG8' } } },
        { success: true },
      );
      const result = await authenticatePasskey(fetchImpl, 'user-123');
      expect(result.success).toBe(false);
      expect(result.error).toBe('authentication cancelled');
    });
  });

  describe('enrollPasskey', () => {
    it('completes a successful enrollment', async () => {
      const { create } = mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(
        { success: true, options: { publicKey: { challenge: 'aGVsbG8', user: { id: 'dXNlcg', name: 'U', displayName: 'U' }, pubKeyCredParams: [] } } },
        { success: true },
      );
      const result = await enrollPasskey(fetchImpl, 'enrollment-token-abc');
      expect(result.success).toBe(true);
      expect(create).toHaveBeenCalledOnce();
    });

    it('presents the token to both challenge and verify', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(
        { success: true, options: { publicKey: { challenge: 'aGVsbG8', user: { id: 'dXNlcg', name: 'U', displayName: 'U' }, pubKeyCredParams: [] } } },
        { success: true },
      ) as unknown as ReturnType<typeof vi.fn>;
      await enrollPasskey(fetchImpl, 'enrollment-token-abc');
      const calls = (fetchImpl as unknown as { mock: { calls: Array<unknown[]> } }).mock.calls;
      const challengeCall = calls.find((c) => (c[1] as string).endsWith('/challenge'));
      const verifyCall = calls.find((c) => (c[1] as string).endsWith('/verify'));
      expect((challengeCall![2] as { body: Record<string, unknown> }).body.enrollment_token).toBe('enrollment-token-abc');
      expect((verifyCall![2] as { body: Record<string, unknown> }).body.enrollment_token).toBe('enrollment-token-abc');
    });

    it('fails when token is empty', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock({ success: true }, { success: true });
      const result = await enrollPasskey(fetchImpl, '');
      expect(result.success).toBe(false);
      expect(result.error).toBe('Missing enrollment token.');
    });

    it('fails when challenge fails', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock({ success: false, error: 'invalid token' }, { success: true });
      const result = await enrollPasskey(fetchImpl, 'bad-token');
      expect(result.success).toBe(false);
      expect(result.error).toBe('invalid token');
    });

    it('does not pre-consume a separate validation endpoint', async () => {
      mockCredentialsCreate(makeFakeCredential());
      const fetchImpl = makeFetchMock(
        { success: true, options: { publicKey: { challenge: 'aGVsbG8', user: { id: 'dXNlcg', name: 'U', displayName: 'U' }, pubKeyCredParams: [] } } },
        { success: true },
      ) as unknown as ReturnType<typeof vi.fn>;
      await enrollPasskey(fetchImpl, 'token-xyz');
      const calls = (fetchImpl as unknown as { mock: { calls: Array<unknown[]> } }).mock.calls;
      // Only challenge and verify calls, no validation endpoint.
      expect(calls.length).toBe(2);
      expect(calls.every((c) => (c[1] as string).endsWith('/challenge') || (c[1] as string).endsWith('/verify'))).toBe(true);
    });
  });

  describe('stripUrlFragment', () => {
    afterEach(() => {
      vi.restoreAllMocks();
    });

    it('calls history.replaceState to remove the fragment', () => {
      const replaceState = vi.fn();
      Object.defineProperty(globalThis, 'history', { value: { replaceState }, writable: true, configurable: true });
      Object.defineProperty(globalThis, 'location', { value: { pathname: '/app', search: '?x=1' }, writable: true, configurable: true });
      stripUrlFragment();
      expect(replaceState).toHaveBeenCalledWith(null, '', '/app?x=1');
    });
  });
});
