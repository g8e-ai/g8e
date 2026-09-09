import { describe, expect, it } from 'vitest';

import {
  ALLOWED_ENDPOINTS,
  EndpointNotAllowedError,
  isAllowed,
  resolveEndpoint,
} from '../src/api/allowlist';

describe('endpoint allowlist', () => {
  describe('allowed operations', () => {
    const allowedCases: Array<[string, string, string]> = [
      ['GET', '/api/v1/auth/bootstrap/status', 'bootstrap_status'],
      ['POST', '/api/v1/auth/passkeys/console/register/challenge', 'passkey_register_challenge'],
      ['POST', '/api/v1/auth/passkeys/console/register/verify', 'passkey_register_verify'],
      ['POST', '/api/v1/auth/passkeys/console/authenticate/challenge', 'passkey_authenticate_challenge'],
      ['POST', '/api/v1/auth/passkeys/console/authenticate/verify', 'passkey_authenticate_verify'],
      ['POST', '/api/v1/auth/passkeys/enrollment/register/challenge', 'passkey_enrollment_challenge'],
      ['POST', '/api/v1/auth/passkeys/enrollment/register/verify', 'passkey_enrollment_verify'],
      ['GET', '/api/v1/users/me', 'users_me'],
      ['GET', '/api/v1/auth/sessions/me', 'sessions_me'],
      ['POST', '/api/v1/auth/logout', 'logout'],
      ['GET', '/api/v1/observe/bootstrap', 'observe_bootstrap'],
      ['GET', '/api/v1/observe/runs', 'observe_runs'],
      ['GET', '/api/v1/observe/runs/r-123', 'observe_run_detail'],
      ['GET', '/api/v1/observe/evals', 'observe_evals'],
      ['GET', '/api/v1/observe/evals/r-456', 'observe_eval_detail'],
      ['GET', '/api/v1/observe/downloads', 'observe_downloads'],
      ['GET', '/api/v1/observe/downloads/art-789', 'observe_download_detail'],
      ['GET', '/api/v1/sse/stream', 'sse_stream'],
      ['GET', '/api/v1/sse/events', 'sse_events'],
      ['GET', '/api/v1/health', 'health'],
    ];

    for (const [method, path, name] of allowedCases) {
      it(`allows ${method} ${path}`, () => {
        const ep = resolveEndpoint(method, path);
        expect(ep.name).toBe(name);
        expect(isAllowed(method, path)).toBe(true);
      });
    }

    it('allows SSE stream with since_id query param', () => {
      // Query params are part of the URL but the allowlist matches the path.
      // The fetch helper receives the path only; query is appended by callers.
      expect(isAllowed('GET', '/api/v1/sse/stream')).toBe(true);
    });
  });

  describe('blocked operations', () => {
    const blockedCases: Array<[string, string]> = [
      // SSE push (producer endpoints are mTLS-only, never browser)
      ['POST', '/api/v1/observe/producer/agent-state'],
      ['POST', '/api/v1/observe/producer/run-state'],
      // Generic data
      ['GET', '/api/v1/data'],
      ['POST', '/api/v1/data'],
      // Audit
      ['GET', '/api/v1/audit'],
      ['GET', '/api/v1/audit/events'],
      // Blobs
      ['GET', '/api/v1/blobs'],
      ['POST', '/api/v1/blobs'],
      // MCP
      ['POST', '/api/v1/mcp'],
      ['GET', '/api/v1/mcp/tools'],
      // A2A
      ['POST', '/api/v1/a2a'],
      // Pub/sub
      ['POST', '/api/v1/pubsub/publish'],
      ['GET', '/api/v1/pubsub/subscribe'],
      // Filesystem
      ['GET', '/api/v1/fs/read'],
      ['POST', '/api/v1/fs/write'],
      // Approvals
      ['GET', '/api/v1/approvals'],
      ['POST', '/api/v1/approvals/tx-123/verify'],
      // Chat
      ['POST', '/api/v1/chat'],
      ['POST', '/api/v1/chat/message'],
      // Tools
      ['POST', '/api/v1/tools/execute'],
      // Eval launch
      ['POST', '/api/v1/evals/launch'],
      ['POST', '/api/v1/evals/run'],
      // Passkey management (revoke) — not in the allowlist
      ['DELETE', '/api/v1/auth/passkeys/cred-1'],
      // Arbitrary unknown paths
      ['GET', '/api/v1/unknown'],
      ['POST', '/api/v1/unknown'],
      ['GET', '/api/v2/observe/bootstrap'],
      // Lookalike prefixes
      ['GET', '/api/v1/observe/producer/agent-state'],
      ['GET', '/api/v1/observee/bootstrap'],
      ['GET', '/api/v1/observe/bootstrap/extra'],
    ];

    for (const [method, path] of blockedCases) {
      it(`blocks ${method} ${path}`, () => {
        expect(() => resolveEndpoint(method, path)).toThrow(EndpointNotAllowedError);
        expect(isAllowed(method, path)).toBe(false);
      });
    }
  });

  describe('method mismatch', () => {
    it('rejects GET on a POST-only endpoint', () => {
      expect(() => resolveEndpoint('GET', '/api/v1/auth/passkeys/console/register/challenge')).toThrow(EndpointNotAllowedError);
    });

    it('rejects POST on a GET-only endpoint', () => {
      expect(() => resolveEndpoint('POST', '/api/v1/observe/bootstrap')).toThrow(EndpointNotAllowedError);
    });

    it('rejects DELETE on observe runs', () => {
      expect(() => resolveEndpoint('DELETE', '/api/v1/observe/runs/r-1')).toThrow(EndpointNotAllowedError);
    });

    it('rejects PUT on observe downloads', () => {
      expect(() => resolveEndpoint('PUT', '/api/v1/observe/downloads/a-1')).toThrow(EndpointNotAllowedError);
    });
  });

  describe('case sensitivity', () => {
    it('rejects lowercase method', () => {
      expect(() => resolveEndpoint('get', '/api/v1/observe/bootstrap')).toThrow(EndpointNotAllowedError);
    });
  });

  describe('error detail', () => {
    it('includes method and path in the error', () => {
      try {
        resolveEndpoint('POST', '/api/v1/data');
        expect.fail('should have thrown');
      } catch (e) {
        expect(e).toBeInstanceOf(EndpointNotAllowedError);
        const err = e as EndpointNotAllowedError;
        expect(err.method).toBe('POST');
        expect(err.path).toBe('/api/v1/data');
        expect(err.message).toContain('POST');
        expect(err.message).toContain('/api/v1/data');
      }
    });
  });

  describe('allowlist completeness', () => {
    it('has exactly 20 allowed endpoints', () => {
      expect(ALLOWED_ENDPOINTS.length).toBe(20);
    });

    it('has no duplicate names', () => {
      const names = ALLOWED_ENDPOINTS.map(e => e.name);
      expect(new Set(names).size).toBe(names.length);
    });
  });
});
