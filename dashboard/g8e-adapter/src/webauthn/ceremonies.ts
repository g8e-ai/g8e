// WebAuthn registration, authentication, and enrollment ceremonies. These
// translate the embedded console's contract exactly:
// - options.publicKey for challenge/response
// - unpadded base64url binary conversion (see encoding.ts)
// - flat attestation/assertion verification models
// - returning-user user_id
// - token enrollment without pre-consuming the validation endpoint
// - history.replaceState token stripping
//
// The ceremonies use the credentialed fetch helper so every request goes
// through the endpoint allowlist with credentials: 'include'.

import type { CredentialedFetch } from '../api/fetch';

import { bufferToBase64url, preparePublicKeyOptions } from './encoding';

// ---- Wire types matching the console contract ----

export interface PublicKeyOptionsResponse {
  success: boolean;
  options: { publicKey: PublicKeyCredentialCreationOptions | PublicKeyCredentialRequestOptions };
  user_id?: string;
  needs_setup?: boolean;
  error?: string;
}

export interface AttestationResponse {
  id: string;
  rawId: string;
  clientDataJSON: string;
  attestationObject: string;
  transports: string[];
}

export interface AssertionResponse {
  id: string;
  rawId: string;
  clientDataJSON: string;
  authenticatorData: string;
  signature: string;
  userHandle: string | null;
}

export interface CeremonyResult {
  success: boolean;
  user_id?: string;
  error?: string;
}

// ---- Registration (bootstrap) ----

export interface RegisterParams {
  user_id: string;
  user_name: string;
  cli_session_id: string;
}

export async function registerPasskey(
  fetchImpl: CredentialedFetch,
  params: RegisterParams,
): Promise<CeremonyResult> {
  const challengeRes = await fetchImpl('POST', '/api/v1/auth/passkeys/console/register/challenge', {
    body: {
      user_id: params.user_id,
      user_name: params.user_name || params.user_id,
      cli_session_id: params.cli_session_id,
    },
  });
  if (!challengeRes.ok || !challengeRes.body) {
    return { success: false, error: 'challenge request failed' };
  }
  const challenge = challengeRes.body as PublicKeyOptionsResponse;
  if (!challenge.success || !challenge.options?.publicKey) {
    return { success: false, error: challenge.error || 'challenge failed' };
  }

  let userId = params.user_id;
  if (challenge.user_id) userId = challenge.user_id;

  const pk = preparePublicKeyOptions(challenge.options.publicKey) as PublicKeyCredentialCreationOptions;
  const credential = await navigator.credentials.create({ publicKey: pk }) as PublicKeyCredential | null;
  if (!credential) {
    return { success: false, error: 'credential creation cancelled' };
  }

  const response = credential.response as AuthenticatorAttestationResponse;
  const attestation: AttestationResponse = {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    clientDataJSON: bufferToBase64url(response.clientDataJSON),
    attestationObject: bufferToBase64url(response.attestationObject),
    transports: response.getTransports ? response.getTransports() : [],
  };

  const verifyRes = await fetchImpl('POST', '/api/v1/auth/passkeys/console/register/verify', {
    body: {
      user_id: userId,
      cli_session_id: params.cli_session_id,
      attestation_response: attestation,
    },
  });
  if (!verifyRes.ok || !verifyRes.body) {
    return { success: false, error: 'verification failed' };
  }
  const result = verifyRes.body as CeremonyResult;
  return { success: result.success !== false, user_id: userId, error: result.error };
}

// ---- Authentication (returning user) ----

export async function authenticatePasskey(
  fetchImpl: CredentialedFetch,
  userId: string,
): Promise<CeremonyResult> {
  const challengeRes = await fetchImpl('POST', '/api/v1/auth/passkeys/console/authenticate/challenge', {
    body: { user_id: userId },
  });
  if (!challengeRes.ok || !challengeRes.body) {
    return { success: false, error: 'challenge request failed' };
  }
  const challenge = challengeRes.body as PublicKeyOptionsResponse;
  if (!challenge.success || !challenge.options?.publicKey) {
    return { success: false, error: challenge.error || 'challenge failed' };
  }

  const pk = preparePublicKeyOptions(challenge.options.publicKey) as PublicKeyCredentialRequestOptions;
  const assertion = await navigator.credentials.get({ publicKey: pk }) as PublicKeyCredential | null;
  if (!assertion) {
    return { success: false, error: 'authentication cancelled' };
  }

  const response = assertion.response as AuthenticatorAssertionResponse;
  const assertionResponse: AssertionResponse = {
    id: assertion.id,
    rawId: bufferToBase64url(assertion.rawId),
    clientDataJSON: bufferToBase64url(response.clientDataJSON),
    authenticatorData: bufferToBase64url(response.authenticatorData),
    signature: bufferToBase64url(response.signature),
    userHandle: response.userHandle ? bufferToBase64url(response.userHandle) : null,
  };

  const verifyRes = await fetchImpl('POST', '/api/v1/auth/passkeys/console/authenticate/verify', {
    body: { user_id: userId, assertion_response: assertionResponse },
  });
  if (!verifyRes.ok || !verifyRes.body) {
    return { success: false, error: 'verification failed' };
  }
  const result = verifyRes.body as CeremonyResult;
  return { success: result.success !== false, user_id: userId, error: result.error };
}

// ---- Enrollment (CLI-initiated) ----

export async function enrollPasskey(
  fetchImpl: CredentialedFetch,
  enrollmentToken: string,
): Promise<CeremonyResult> {
  if (!enrollmentToken) {
    return { success: false, error: 'Missing enrollment token.' };
  }

  // Token is presented to BOTH challenge and verify; challenge validates, verify
  // consumes (one-time). No pre-consumption of a separate validation endpoint.
  const challengeRes = await fetchImpl('POST', '/api/v1/auth/passkeys/enrollment/register/challenge', {
    body: { enrollment_token: enrollmentToken },
  });
  if (!challengeRes.ok || !challengeRes.body) {
    return { success: false, error: 'challenge request failed' };
  }
  const challenge = challengeRes.body as PublicKeyOptionsResponse;
  if (!challenge.success || !challenge.options?.publicKey) {
    return { success: false, error: challenge.error || 'Challenge failed' };
  }

  const pk = preparePublicKeyOptions(challenge.options.publicKey) as PublicKeyCredentialCreationOptions;
  const credential = await navigator.credentials.create({ publicKey: pk }) as PublicKeyCredential | null;
  if (!credential) {
    return { success: false, error: 'credential creation cancelled' };
  }

  const response = credential.response as AuthenticatorAttestationResponse;
  const attestation: AttestationResponse = {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    clientDataJSON: bufferToBase64url(response.clientDataJSON),
    attestationObject: bufferToBase64url(response.attestationObject),
    transports: response.getTransports ? response.getTransports() : [],
  };

  const verifyRes = await fetchImpl('POST', '/api/v1/auth/passkeys/enrollment/register/verify', {
    body: { enrollment_token: enrollmentToken, attestation_response: attestation },
  });
  if (!verifyRes.ok || !verifyRes.body) {
    return { success: false, error: 'verification failed' };
  }
  const result = verifyRes.body as CeremonyResult;
  return { success: result.success !== false, error: result.error };
}

// ---- Token stripping ----

// Remove enrollment/recovery tokens from the URL fragment immediately after
// reading, keeping them in memory only. Mirrors the console's
// history.replaceState(null, '', location.pathname + location.search).
export function stripUrlFragment(): void {
  if (typeof history !== 'undefined' && history.replaceState) {
    history.replaceState(null, '', location.pathname + location.search);
  }
}
