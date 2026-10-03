// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Named endpoint allowlist for the audited adapter. Operational code exports
// only runtime-config reads, public bootstrap status, passkey ceremony POSTs,
// session restoration, logout, observe GETs, authenticated downloads, SSE
// stream, and SSE polling. No generic arbitrary-path request helper is exposed
// to components.

export type HttpVerb = 'GET' | 'POST';

export type EndpointName =
  | 'bootstrap_status'
  | 'passkey_register_challenge'
  | 'passkey_register_verify'
  | 'passkey_authenticate_challenge'
  | 'passkey_authenticate_verify'
  | 'passkey_enrollment_challenge'
  | 'passkey_enrollment_verify'
  | 'users_me'
  | 'sessions_me'
  | 'logout'
  | 'observe_bootstrap'
  | 'observe_runs'
  | 'observe_run_detail'
  | 'observe_evals'
  | 'observe_eval_detail'
  | 'observe_downloads'
  | 'observe_download_detail'
  | 'sse_stream'
  | 'sse_events'
  | 'health';

export interface AllowedEndpoint {
  readonly name: EndpointName;
  readonly method: HttpVerb;
  readonly pathPattern: RegExp;
  readonly streaming?: boolean;
}

// Path patterns are anchored to match the full path. Parameter segments use
// [^/]+ and are captured for callers that need the resolved value.
const PATH_PARAM = '[^/]+';

function exact(path: string): RegExp {
  return new RegExp(`^${path.replace(/[.+?^${}()|[\]\\]/g, '\\$&')}$`);
}

function withParam(prefix: string): RegExp {
  return new RegExp(`^${prefix.replace(/[.+?^${}()|[\]\\]/g, '\\$&')}${PATH_PARAM}$`);
}

export const ALLOWED_ENDPOINTS: readonly AllowedEndpoint[] = [
  { name: 'bootstrap_status', method: 'GET', pathPattern: exact('/api/v1/auth/bootstrap/status') },
  { name: 'passkey_register_challenge', method: 'POST', pathPattern: exact('/api/v1/auth/passkeys/console/register/challenge') },
  { name: 'passkey_register_verify', method: 'POST', pathPattern: exact('/api/v1/auth/passkeys/console/register/verify') },
  { name: 'passkey_authenticate_challenge', method: 'POST', pathPattern: exact('/api/v1/auth/passkeys/console/authenticate/challenge') },
  { name: 'passkey_authenticate_verify', method: 'POST', pathPattern: exact('/api/v1/auth/passkeys/console/authenticate/verify') },
  { name: 'passkey_enrollment_challenge', method: 'POST', pathPattern: exact('/api/v1/auth/passkeys/enrollment/register/challenge') },
  { name: 'passkey_enrollment_verify', method: 'POST', pathPattern: exact('/api/v1/auth/passkeys/enrollment/register/verify') },
  { name: 'users_me', method: 'GET', pathPattern: exact('/api/v1/users/me') },
  { name: 'sessions_me', method: 'GET', pathPattern: exact('/api/v1/auth/sessions/me') },
  { name: 'logout', method: 'POST', pathPattern: exact('/api/v1/auth/logout') },
  { name: 'observe_bootstrap', method: 'GET', pathPattern: exact('/api/v1/observe/bootstrap') },
  { name: 'observe_runs', method: 'GET', pathPattern: exact('/api/v1/observe/runs') },
  { name: 'observe_run_detail', method: 'GET', pathPattern: withParam('/api/v1/observe/runs/') },
  { name: 'observe_evals', method: 'GET', pathPattern: exact('/api/v1/observe/evals') },
  { name: 'observe_eval_detail', method: 'GET', pathPattern: withParam('/api/v1/observe/evals/') },
  { name: 'observe_downloads', method: 'GET', pathPattern: exact('/api/v1/observe/downloads') },
  { name: 'observe_download_detail', method: 'GET', pathPattern: withParam('/api/v1/observe/downloads/') },
  { name: 'sse_stream', method: 'GET', pathPattern: exact('/api/v1/sse/stream'), streaming: true },
  { name: 'sse_events', method: 'GET', pathPattern: exact('/api/v1/sse/events') },
  { name: 'health', method: 'GET', pathPattern: exact('/api/v1/health') },
] as const;

export class EndpointNotAllowedError extends Error {
  constructor(public readonly method: string, public readonly path: string) {
    super(`endpoint not allowed: ${method} ${path}`);
    this.name = 'EndpointNotAllowedError';
  }
}

export function resolveEndpoint(method: string, path: string): AllowedEndpoint {
  // Strip the query string before matching. Query parameters (cursor, limit,
  // since_id) are part of the request, not the route identity.
  const pathOnly = path.split('?')[0];
  for (const ep of ALLOWED_ENDPOINTS) {
    if (ep.method === method && ep.pathPattern.test(pathOnly)) {
      return ep;
    }
  }
  throw new EndpointNotAllowedError(method, path);
}

export function isAllowed(method: string, path: string): boolean {
  try {
    resolveEndpoint(method, path);
    return true;
  } catch {
    return false;
  }
}
