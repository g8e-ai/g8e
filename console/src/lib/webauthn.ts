// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// WebAuthn ceremonies against the Gateway (docs/guides/build_frontend.md).
// Binary values travel as unpadded base64url; verification bodies use the
// Gateway's flat credential models (INV-FE-WEBAUTHN-01). Registration and
// authentication challenges wrap options under `options.publicKey`; the
// approval challenge places them directly under `publicKey`.

import { api, ApiError } from './api';
import { Paths } from './paths';

export function b64urlToBuffer(s: string): ArrayBuffer {
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/');
  const pad = b64.length % 4 === 0 ? '' : '='.repeat(4 - (b64.length % 4));
  const raw = atob(b64 + pad);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out.buffer;
}

export function bufferToB64url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let s = '';
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]!);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

interface WireDescriptor {
  id: string;
  type: string;
  transports?: string[];
}

interface WireCreationOptions {
  challenge: string;
  user: { id: string; name: string; displayName: string };
  excludeCredentials?: WireDescriptor[];
  [key: string]: unknown;
}

interface WireRequestOptions {
  challenge: string;
  allowCredentials?: WireDescriptor[];
  [key: string]: unknown;
}

function decodeDescriptors(list: WireDescriptor[] | undefined): PublicKeyCredentialDescriptor[] | undefined {
  return list?.map((c) => ({ ...c, id: b64urlToBuffer(c.id) }) as PublicKeyCredentialDescriptor);
}

export function decodeCreationOptions(pk: WireCreationOptions): PublicKeyCredentialCreationOptions {
  return {
    ...pk,
    challenge: b64urlToBuffer(pk.challenge),
    user: { ...pk.user, id: b64urlToBuffer(pk.user.id) },
    excludeCredentials: decodeDescriptors(pk.excludeCredentials),
  } as unknown as PublicKeyCredentialCreationOptions;
}

export function decodeRequestOptions(pk: WireRequestOptions): PublicKeyCredentialRequestOptions {
  return {
    ...pk,
    challenge: b64urlToBuffer(pk.challenge),
    allowCredentials: decodeDescriptors(pk.allowCredentials),
  } as unknown as PublicKeyCredentialRequestOptions;
}

export interface FlatAttestation {
  id: string;
  rawId: string;
  clientDataJSON: string;
  attestationObject: string;
  transports: string[];
}

export interface FlatAssertion {
  id: string;
  rawId: string;
  clientDataJSON: string;
  authenticatorData: string;
  signature: string;
  userHandle: string | null;
}

export function encodeAttestation(cred: PublicKeyCredential): FlatAttestation {
  const r = cred.response as AuthenticatorAttestationResponse;
  return {
    id: cred.id,
    rawId: bufferToB64url(cred.rawId),
    clientDataJSON: bufferToB64url(r.clientDataJSON),
    attestationObject: bufferToB64url(r.attestationObject),
    transports: typeof r.getTransports === 'function' ? r.getTransports() : [],
  };
}

export function encodeAssertion(cred: PublicKeyCredential): FlatAssertion {
  const r = cred.response as AuthenticatorAssertionResponse;
  return {
    id: cred.id,
    rawId: bufferToB64url(cred.rawId),
    clientDataJSON: bufferToB64url(r.clientDataJSON),
    authenticatorData: bufferToB64url(r.authenticatorData),
    signature: bufferToB64url(r.signature),
    userHandle: r.userHandle ? bufferToB64url(r.userHandle) : null,
  };
}

export function webAuthnAvailable(): boolean {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential && !!navigator.credentials;
}

async function create(options: PublicKeyCredentialCreationOptions): Promise<PublicKeyCredential> {
  const cred = await navigator.credentials.create({ publicKey: options });
  if (!cred) throw new Error('Passkey creation was cancelled.');
  return cred as PublicKeyCredential;
}

async function get(options: PublicKeyCredentialRequestOptions): Promise<PublicKeyCredential> {
  const cred = await navigator.credentials.get({ publicKey: options });
  if (!cred) throw new Error('Passkey prompt was cancelled.');
  return cred as PublicKeyCredential;
}

interface ChallengeResponse<O> {
  success: boolean;
  error?: string;
  user_id?: string;
  needs_setup?: boolean;
  options: { publicKey: O };
}

/** Thrown when the authenticate challenge reports the user has no passkey. */
export class NeedsSetupError extends Error {
  constructor() {
    super('No passkey is registered for this user ID. Enroll one from the CLI with `g8e auth enroll user`.');
    this.name = 'NeedsSetupError';
  }
}

/**
 * First-owner bootstrap registration. The Gateway only accepts this while no
 * owner exists; it creates the user and sets the session cookie on verify.
 */
export async function registerBootstrapPasskey(displayName: string): Promise<void> {
  const ch = await api.post<ChallengeResponse<WireCreationOptions>>(
    Paths.consoleRegisterChallenge,
    { user_name: displayName, cli_session_id: 'browser' },
    { sessionBound: false },
  );
  if (!ch.success || !ch.user_id) throw new Error(ch.error || 'Registration challenge failed.');
  const cred = await create(decodeCreationOptions(ch.options.publicKey));
  const vr = await api.post<{ success: boolean; error?: string }>(
    Paths.consoleRegisterVerify,
    { user_id: ch.user_id, cli_session_id: 'browser', attestation_response: encodeAttestation(cred) },
    { sessionBound: false },
  );
  if (!vr.success) throw new Error(vr.error || 'Registration failed.');
}

export async function authenticatePasskey(userId: string): Promise<void> {
  const ch = await api.post<ChallengeResponse<WireRequestOptions>>(
    Paths.consoleAuthenticateChallenge,
    { user_id: userId },
    { sessionBound: false },
  );
  if (!ch.success) {
    if (ch.needs_setup) throw new NeedsSetupError();
    throw new Error(ch.error || 'Sign-in challenge failed.');
  }
  const cred = await get(decodeRequestOptions(ch.options.publicKey));
  const vr = await api.post<{ success: boolean; error?: string }>(
    Paths.consoleAuthenticateVerify,
    { user_id: userId, assertion_response: encodeAssertion(cred) },
    { sessionBound: false },
  );
  if (!vr.success) throw new Error(vr.error || 'Sign-in failed.');
}

function enrollmentErrorMessage(err: unknown): string | null {
  if (!(err instanceof ApiError)) return null;
  if (err.status === 410) return 'This enrollment link has expired. Generate a new one with `g8e auth enroll user`.';
  if (err.status === 409) return 'This enrollment link has already been used.';
  if (err.status === 401) return 'This enrollment link is invalid.';
  return null;
}

/**
 * CLI-initiated enrollment. The one-time token is the only authorization: the
 * Gateway derives user and CLI session from it, validates it at challenge, and
 * consumes it at verify. Never call /auth/enrollment-token/validate first — it
 * consumes the token.
 */
export async function registerEnrollmentPasskey(token: string): Promise<void> {
  try {
    const ch = await api.post<ChallengeResponse<WireCreationOptions>>(
      Paths.enrollmentRegisterChallenge,
      { enrollment_token: token },
      { sessionBound: false },
    );
    if (!ch.success) throw new Error(ch.error || 'Enrollment challenge failed.');
    const cred = await create(decodeCreationOptions(ch.options.publicKey));
    const vr = await api.post<{ success: boolean; error?: string }>(
      Paths.enrollmentRegisterVerify,
      { enrollment_token: token, attestation_response: encodeAttestation(cred) },
      { sessionBound: false },
    );
    if (!vr.success) throw new Error(vr.error || 'Enrollment failed.');
  } catch (err) {
    const msg = enrollmentErrorMessage(err);
    if (msg) throw new Error(msg);
    throw err;
  }
}

export interface ApprovalOutcome {
  ok: boolean;
  status: number;
  message: string;
}

/**
 * L3 Notary approval of a suspended transaction. The verify body is the flat
 * assertion itself, not nested (INV-FE-APPROVAL-01). A 403 may carry a receipt
 * describing the rejected result; its message is surfaced as-is.
 */
export async function approveTransaction(txHash: string): Promise<ApprovalOutcome> {
  const ch = await api.get<{ publicKey: WireRequestOptions }>(Paths.approvalChallenge(txHash));
  const cred = await get(decodeRequestOptions(ch.publicKey));
  try {
    await api.post(Paths.approvalVerify(txHash), encodeAssertion(cred));
    return { ok: true, status: 200, message: 'Transaction approved. Execution resumed.' };
  } catch (err) {
    if (err instanceof ApiError) {
      const msg = err.status === 404 ? 'This transaction is missing or has expired.' : err.message;
      return { ok: false, status: err.status, message: msg };
    }
    throw err;
  }
}
