// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Credentialed fetch helper that enforces the endpoint allowlist. This is the
// only request surface components use; there is no generic arbitrary-path
// request helper. Every fetch uses credentials: 'include' against the absolute
// configured Gateway origin.

import type { FrontendRuntimeConfig } from '../config/runtime_config';
import { EndpointNotAllowedError, resolveEndpoint } from './allowlist';

export interface GatewayResponse<T> {
  readonly status: number;
  readonly ok: boolean;
  readonly body: T | undefined;
  readonly headers: Headers;
}

export class GatewayFetchError extends Error {
  constructor(
    public readonly status: number,
    public readonly path: string,
    message: string,
  ) {
    super(message);
    this.name = 'GatewayFetchError';
  }
}

export interface CredentialedFetch {
  (method: string, path: string, opts?: FetchOptions): Promise<GatewayResponse<unknown>>;
}

export interface FetchOptions {
  body?: unknown;
  headers?: Record<string, string>;
  signal?: AbortSignal;
}

export function createCredentialedFetch(
  config: FrontendRuntimeConfig,
  fetchImpl: typeof fetch = fetch,
): CredentialedFetch {
  const base = config.gateway_base_url.replace(/\/$/, '');
  return async (method: string, path: string, opts?: FetchOptions): Promise<GatewayResponse<unknown>> => {
    const verb = method.toUpperCase();
    // Enforce the allowlist before any network call.
    resolveEndpoint(verb, path);

    const url = `${base}${path}`;
    const headers = new Headers(opts?.headers);
    let body: BodyInit | undefined;
    if (opts?.body !== undefined) {
      headers.set('Content-Type', 'application/json');
      body = JSON.stringify(opts.body);
    }

    const res = await fetchImpl(url, {
      method: verb,
      credentials: 'include',
      headers,
      body,
      signal: opts?.signal,
    });

    let parsed: unknown;
    const text = await res.text();
    if (text) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = undefined;
      }
    } else {
      parsed = undefined;
    }

    return { status: res.status, ok: res.ok, body: parsed, headers: res.headers };
  };
}

export { EndpointNotAllowedError };
