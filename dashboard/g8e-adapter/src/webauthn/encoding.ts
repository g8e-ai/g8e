// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Unpadded base64url binary conversion, matching the embedded console's
// b64urlToBuf and bufToB64url exactly. The WebAuthn API uses ArrayBuffer for
// challenge and credential IDs, but the wire uses unpadded base64url strings.

export function base64urlToBuffer(b64url: string): ArrayBuffer {
  const b64 = b64url.replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(b64);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) {
    bytes[i] = raw.charCodeAt(i);
  }
  return bytes.buffer;
}

export function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let str = '';
  for (let i = 0; i < bytes.length; i++) {
    str += String.fromCharCode(bytes[i]);
  }
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

// Convert the publicKey options from the wire (base64url strings) to the
// WebAuthn API format (ArrayBuffer). This mirrors the console's
// pk.challenge = b64urlToBuf(pk.challenge); pk.user.id = b64urlToBuf(pk.user.id);
// and excludeCredentials/allowCredentials conversion.
export function preparePublicKeyOptions(
  options: PublicKeyCredentialCreationOptions | PublicKeyCredentialRequestOptions,
): PublicKeyCredentialCreationOptions | PublicKeyCredentialRequestOptions {
  if ('pubKeyCredParams' in options) {
    // Registration options (PublicKeyCredentialCreationOptions)
    const reg = options as PublicKeyCredentialCreationOptions;
    const prepared = { ...reg };
    if (typeof prepared.challenge === 'string') {
      prepared.challenge = base64urlToBuffer(prepared.challenge);
    }
    if (prepared.user && typeof prepared.user.id === 'string') {
      prepared.user = { ...prepared.user, id: base64urlToBuffer(prepared.user.id as string) };
    }
    if (prepared.excludeCredentials) {
      prepared.excludeCredentials = prepared.excludeCredentials.map((c) => ({
        ...c,
        id: typeof c.id === 'string' ? base64urlToBuffer(c.id as string) : c.id,
      }));
    }
    return prepared;
  }
  // Authentication options (PublicKeyCredentialRequestOptions)
  const auth = options as PublicKeyCredentialRequestOptions;
  const prepared = { ...auth };
  if (typeof prepared.challenge === 'string') {
    prepared.challenge = base64urlToBuffer(prepared.challenge as string);
  }
  if (prepared.allowCredentials) {
    prepared.allowCredentials = prepared.allowCredentials.map((c) => ({
      ...c,
      id: typeof c.id === 'string' ? base64urlToBuffer(c.id as string) : c.id,
    }));
  }
  return prepared;
}
