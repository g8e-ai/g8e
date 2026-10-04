// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Credentialed fetch wrapper. The session cookie is HttpOnly; the console never
// reads it and never sends user or session IDs as routing input — the Gateway
// derives both from the cookie (INV-FE-SESSION-01).

export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(status: number, message: string, body: unknown) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
  }
}

type Listener = () => void;
const unauthorizedListeners = new Set<Listener>();

/** Registers a callback fired when a session-authenticated call returns 401. */
export function onUnauthorized(listener: Listener): () => void {
  unauthorizedListeners.add(listener);
  return () => unauthorizedListeners.delete(listener);
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE';
  body?: unknown;
  /** Treat 401 as a lapsed session (default true). Public ceremony routes pass false. */
  sessionBound?: boolean;
  signal?: AbortSignal;
}

function errorMessage(body: unknown, fallback: string): string {
  if (body && typeof body === 'object') {
    const b = body as Record<string, unknown>;
    if (typeof b.error === 'string' && b.error) return b.error;
    // g8ee errors arrive through the browser proxy as {error: {message, ...}}.
    if (b.error && typeof b.error === 'object') {
      const nested = (b.error as Record<string, unknown>).message;
      if (typeof nested === 'string' && nested) return nested;
    }
    if (typeof b.message === 'string' && b.message) return b.message;
    if (typeof b.detail === 'string' && b.detail) return b.detail;
  }
  return fallback;
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, sessionBound = true, signal } = opts;
  const init: RequestInit = { method, credentials: 'include', signal };
  if (body !== undefined) {
    init.headers = { 'Content-Type': 'application/json' };
    init.body = JSON.stringify(body);
  }
  const res = await fetch(path, init);
  const text = await res.text();
  let parsed: unknown = undefined;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = text;
    }
  }
  if (!res.ok) {
    if (res.status === 401 && sessionBound) {
      unauthorizedListeners.forEach((l) => l());
    }
    throw new ApiError(res.status, errorMessage(parsed, `HTTP ${res.status}`), parsed);
  }
  return parsed as T;
}

// Gateway 502 messages for a g8ee that is starting, enrolling, or has not yet
// trusted the Gateway's proxy signing key. Both are transient upstream states,
// never a lapsed browser session: the Gateway maps an upstream 401 to 502 so
// the unauthorized listeners above only ever fire for the Gateway's own
// session check.
const ENSEMBLE_UNAVAILABLE_MESSAGES = ['ensemble upstream unavailable', 'ensemble upstream authentication failed'];

/** True when a proxied g8ee call failed because the ensemble is not reachable or not yet trusted. */
export function isEnsembleUnavailable(err: unknown): boolean {
  const msg = (err instanceof Error ? err.message : String(err)).toLowerCase();
  return ENSEMBLE_UNAVAILABLE_MESSAGES.some((m) => msg.includes(m));
}

export const api = {
  get: <T>(path: string, opts?: Omit<RequestOptions, 'method' | 'body'>) => request<T>(path, { ...opts, method: 'GET' }),
  post: <T>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...opts, method: 'POST', body: body ?? {} }),
  del: <T>(path: string, opts?: Omit<RequestOptions, 'method' | 'body'>) => request<T>(path, { ...opts, method: 'DELETE' }),
};
