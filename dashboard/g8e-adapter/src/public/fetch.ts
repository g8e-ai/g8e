// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { PublicRuntimeConfig } from './runtime_config';

const PUBLIC_ENDPOINTS = {
  bootstrap: '/bootstrap',
  snapshot: '/snapshot',
  history: '/history',
  stream: '/stream',
  proofCatalog: '/proof-catalog',
  proofManifest: '/proof-manifest',
} as const;

export type PublicEndpoint = keyof typeof PUBLIC_ENDPOINTS;

export class PublicEndpointNotAllowedError extends Error {
  constructor(public readonly endpoint: string, public readonly suffix: string) {
    super(`public endpoint not allowed: ${endpoint}${suffix}`);
    this.name = 'PublicEndpointNotAllowedError';
  }
}

export interface PublicResponse<T = unknown> {
  readonly status: number;
  readonly ok: boolean;
  readonly body: T | undefined;
  readonly headers: Headers;
}

export interface PublicFetch {
  <T = unknown>(endpoint: PublicEndpoint, query?: string, signal?: AbortSignal): Promise<PublicResponse<T>>;
}

function resolvePublicPath(endpoint: PublicEndpoint, query: string): string {
  const path = PUBLIC_ENDPOINTS[endpoint];
  if (!path || (query !== '' && !query.startsWith('?'))) {
    throw new PublicEndpointNotAllowedError(endpoint, query);
  }
  const parsed = new URL(`${path}${query}`, 'https://public.invalid');
  if (parsed.pathname !== path || parsed.origin !== 'https://public.invalid') {
    throw new PublicEndpointNotAllowedError(endpoint, query);
  }
  return `${path}${parsed.search}`;
}

export function createPublicFetch(config: PublicRuntimeConfig, fetchImpl: typeof fetch = fetch): PublicFetch {
  return async <T = unknown>(endpoint: PublicEndpoint, query = '', signal?: AbortSignal): Promise<PublicResponse<T>> => {
    const path = resolvePublicPath(endpoint, query);
    const response = await fetchImpl(`${config.mirror_origin}${path}`, {
      method: 'GET',
      credentials: 'omit',
      signal,
    });
    const text = await response.text();
    let body: T | undefined;
    if (text !== '') {
      try {
        body = JSON.parse(text) as T;
      } catch {
        body = undefined;
      }
    }
    return { status: response.status, ok: response.ok, body, headers: response.headers };
  };
}
