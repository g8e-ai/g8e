import { describe, expect, it, vi } from 'vitest';

import { EndpointNotAllowedError } from '../src/api/allowlist';
import { createCredentialedFetch } from '../src/api/fetch';
import { parseRuntimeConfig, RUNTIME_CONFIG_SCHEMA_VERSION } from '../src/config/runtime_config';

function makeConfig() {
  return parseRuntimeConfig({
    schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
    gateway_base_url: 'https://g8e.example.com',
    passkey_rp_id: 'g8e.example.com',
    passkey_rp_name: 'g8e',
    app_name: 'g8e',
  });
}

const EMPTY_BODY = Symbol('empty');

function makeFetchMock(status = 200, body: unknown = {}): typeof fetch & { calls: Array<Record<string, unknown>> } {
  const calls: Array<Record<string, unknown>> = [];
  const mock = vi.fn(async (url: string, opts: Record<string, unknown>) => {
    calls.push({ url, opts });
    return {
      ok: status >= 200 && status < 300,
      status,
      text: async () => (body === EMPTY_BODY ? '' : JSON.stringify(body)),
      headers: new Headers(),
    } as Response;
  }) as unknown as typeof fetch & { calls: Array<Record<string, unknown>> };
  mock.calls = calls;
  return mock;
}

describe('createCredentialedFetch', () => {
  it('sends credentials: include on every request', async () => {
    const fetchMock = makeFetchMock(200, { bootstrapped: true });
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    await f('GET', '/api/v1/auth/bootstrap/status');
    expect(fetchMock.calls[0].opts.credentials).toBe('include');
  });

  it('builds the absolute URL from the configured origin', async () => {
    const fetchMock = makeFetchMock(200, { bootstrapped: true });
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    await f('GET', '/api/v1/auth/bootstrap/status');
    expect(fetchMock.calls[0].url).toBe('https://g8e.example.com/api/v1/auth/bootstrap/status');
  });

  it('strips a trailing slash from the base URL', async () => {
    const cfg = parseRuntimeConfig({
      schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
      gateway_base_url: 'https://g8e.example.com/',
      passkey_rp_id: 'g8e.example.com',
      passkey_rp_name: 'g8e',
      app_name: 'g8e',
    });
    const fetchMock = makeFetchMock(200, {});
    const f = createCredentialedFetch(cfg, fetchMock);
    await f('GET', '/api/v1/health');
    expect(fetchMock.calls[0].url).toBe('https://g8e.example.com/api/v1/health');
  });

  it('rejects a non-allowlisted path before any network call', async () => {
    const fetchMock = makeFetchMock(200, {});
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    await expect(f('POST', '/api/v1/data')).rejects.toBeInstanceOf(EndpointNotAllowedError);
    expect(fetchMock.calls.length).toBe(0);
  });

  it('serializes a JSON body and sets Content-Type', async () => {
    const fetchMock = makeFetchMock(200, { success: true });
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    await f('POST', '/api/v1/auth/passkeys/console/register/challenge', {
      body: { user_id: 'u1', user_name: 'U', cli_session_id: 'browser' },
    });
    const opts = fetchMock.calls[0].opts as Record<string, unknown>;
    expect(opts.body).toBe(JSON.stringify({ user_id: 'u1', user_name: 'U', cli_session_id: 'browser' }));
    const headers = opts.headers as Headers;
    expect(headers.get('Content-Type')).toBe('application/json');
  });

  it('does not set a body or Content-Type when no body is provided', async () => {
    const fetchMock = makeFetchMock(200, { bootstrapped: true });
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    await f('GET', '/api/v1/auth/bootstrap/status');
    const opts = fetchMock.calls[0].opts as Record<string, unknown>;
    expect(opts.body).toBeUndefined();
  });

  it('parses a JSON response body', async () => {
    const fetchMock = makeFetchMock(200, { bootstrapped: true });
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    const res = await f('GET', '/api/v1/auth/bootstrap/status');
    expect(res.status).toBe(200);
    expect(res.ok).toBe(true);
    expect(res.body).toEqual({ bootstrapped: true });
  });

  it('returns undefined body for an empty response', async () => {
    const fetchMock = makeFetchMock(204, EMPTY_BODY);
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    const res = await f('POST', '/api/v1/auth/logout');
    expect(res.status).toBe(204);
    expect(res.body).toBeUndefined();
  });

  it('returns undefined body for non-JSON response', async () => {
    const mock = vi.fn(async () => ({
      ok: true,
      status: 200,
      text: async () => 'not json',
      headers: new Headers(),
    }) as Response) as unknown as typeof fetch;
    const f = createCredentialedFetch(makeConfig(), mock);
    const res = await f('GET', '/api/v1/health');
    expect(res.body).toBeUndefined();
  });

  it('passes through an abort signal', async () => {
    const fetchMock = makeFetchMock(200, {});
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    const controller = new AbortController();
    await f('GET', '/api/v1/health', { signal: controller.signal });
    const opts = fetchMock.calls[0].opts as Record<string, unknown>;
    expect(opts.signal).toBe(controller.signal);
  });

  it('uppercases the method', async () => {
    const fetchMock = makeFetchMock(200, {});
    const f = createCredentialedFetch(makeConfig(), fetchMock);
    await f('get', '/api/v1/health');
    const opts = fetchMock.calls[0].opts as Record<string, unknown>;
    expect(opts.method).toBe('GET');
  });
});
