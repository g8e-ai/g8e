import { describe, expect, it } from 'vitest';

import {
  RUNTIME_CONFIG_SCHEMA_VERSION,
  RuntimeConfigError,
  parseRuntimeConfig,
} from '../src/config/runtime_config';

function validInput(): Record<string, unknown> {
  return {
    schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
    gateway_base_url: 'https://g8e.example.com',
    passkey_rp_id: 'g8e.example.com',
    passkey_rp_name: 'g8e Gateway',
    app_name: 'g8e Observe',
  };
}

describe('FrontendRuntimeConfig', () => {
  describe('valid configs', () => {
    it('accepts a minimal https config', () => {
      const cfg = parseRuntimeConfig(validInput());
      expect(cfg.schema_version).toBe(RUNTIME_CONFIG_SCHEMA_VERSION);
      expect(cfg.gateway_base_url).toBe('https://g8e.example.com');
      expect(cfg.passkey_rp_id).toBe('g8e.example.com');
      expect(cfg.passkey_rp_name).toBe('g8e Gateway');
      expect(cfg.app_name).toBe('g8e Observe');
      expect(cfg.documentation_base_url).toBeUndefined();
      expect(cfg.features).toBeUndefined();
    });

    it('accepts a loopback http origin', () => {
      const input = validInput();
      input.gateway_base_url = 'http://localhost:8080';
      input.passkey_rp_id = 'localhost';
      const cfg = parseRuntimeConfig(input);
      expect(cfg.gateway_base_url).toBe('http://localhost:8080');
    });

    it('accepts 127.0.0.1 loopback http', () => {
      const input = validInput();
      input.gateway_base_url = 'http://127.0.0.1:3000';
      input.passkey_rp_id = '127.0.0.1';
      const cfg = parseRuntimeConfig(input);
      expect(cfg.gateway_base_url).toBe('http://127.0.0.1:3000');
    });

    it('accepts [::1] loopback http', () => {
      const input = validInput();
      input.gateway_base_url = 'http://[::1]:3000';
      input.passkey_rp_id = 'localhost';
      const cfg = parseRuntimeConfig(input);
      expect(cfg.gateway_base_url).toBe('http://[::1]:3000');
    });

    it('strips a trailing slash from gateway_base_url', () => {
      const input = validInput();
      input.gateway_base_url = 'https://g8e.example.com/';
      const cfg = parseRuntimeConfig(input);
      expect(cfg.gateway_base_url).toBe('https://g8e.example.com');
    });

    it('accepts optional documentation_base_url and features', () => {
      const input = validInput();
      input.documentation_base_url = 'https://docs.example.com';
      input.features = { design_preview: true, eval_publication: false, downloads: true };
      const cfg = parseRuntimeConfig(input);
      expect(cfg.documentation_base_url).toBe('https://docs.example.com');
      expect(cfg.features).toEqual({ design_preview: true, eval_publication: false, downloads: true });
    });

    it('accepts a partial features object', () => {
      const input = validInput();
      input.features = { downloads: true };
      const cfg = parseRuntimeConfig(input);
      expect(cfg.features).toEqual({ downloads: true });
    });
  });

  describe('invalid origins', () => {
    it('rejects non-https non-loopback http', () => {
      const input = validInput();
      input.gateway_base_url = 'http://g8e.example.com';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects a malformed URL', () => {
      const input = validInput();
      input.gateway_base_url = 'not a url';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects userinfo in the origin', () => {
      const input = validInput();
      input.gateway_base_url = 'https://user:pass@g8e.example.com';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects a URL fragment', () => {
      const input = validInput();
      input.gateway_base_url = 'https://g8e.example.com#token=secret';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects a path in the origin', () => {
      const input = validInput();
      input.gateway_base_url = 'https://g8e.example.com/api';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects a query string in the origin', () => {
      const input = validInput();
      input.gateway_base_url = 'https://g8e.example.com?x=1';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects an empty gateway_base_url', () => {
      const input = validInput();
      input.gateway_base_url = '';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects insecure documentation_base_url', () => {
      const input = validInput();
      input.documentation_base_url = 'http://docs.example.com';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });
  });

  describe('invalid RP ID', () => {
    it('rejects an empty passkey_rp_id', () => {
      const input = validInput();
      input.passkey_rp_id = '';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects a protocol in the RP ID', () => {
      const input = validInput();
      input.passkey_rp_id = 'https://g8e.example.com';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects a port in the RP ID', () => {
      const input = validInput();
      input.passkey_rp_id = 'g8e.example.com:443';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects a path in the RP ID', () => {
      const input = validInput();
      input.passkey_rp_id = 'g8e.example.com/path';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });
  });

  describe('missing required fields', () => {
    it('rejects missing schema_version', () => {
      const input = validInput();
      delete input.schema_version;
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects unsupported schema_version', () => {
      const input = validInput();
      input.schema_version = '2.0.0';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects missing gateway_base_url', () => {
      const input = validInput();
      delete input.gateway_base_url;
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects missing passkey_rp_id', () => {
      const input = validInput();
      delete input.passkey_rp_id;
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects missing passkey_rp_name', () => {
      const input = validInput();
      delete input.passkey_rp_name;
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects missing app_name', () => {
      const input = validInput();
      delete input.app_name;
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });
  });

  describe('unknown fields', () => {
    it('rejects an unknown top-level field', () => {
      const input = validInput();
      (input as Record<string, unknown>).extra_field = 'x';
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });

    it('rejects an unknown features field', () => {
      const input = validInput();
      input.features = { unknown_flag: true };
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });
  });

  describe('secret/credential fields', () => {
    const secretKeys = [
      'token', 'secret', 'password', 'credential', 'api_key',
      'user_id', 'session_id', 'cookie', 'authorization', 'access_token',
    ];

    for (const key of secretKeys) {
      it(`rejects a top-level ${key} field`, () => {
        const input = validInput();
        (input as Record<string, unknown>)[key] = 'leak';
        expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
      });
    }

    it('rejects a secret features field', () => {
      const input = validInput();
      input.features = { token: 'leak' };
      expect(() => parseRuntimeConfig(input)).toThrow(RuntimeConfigError);
    });
  });

  describe('non-object input', () => {
    it('rejects a string', () => {
      expect(() => parseRuntimeConfig('hello')).toThrow(RuntimeConfigError);
    });

    it('rejects null', () => {
      expect(() => parseRuntimeConfig(null)).toThrow(RuntimeConfigError);
    });

    it('rejects an array', () => {
      expect(() => parseRuntimeConfig([])).toThrow(RuntimeConfigError);
    });
  });

  describe('error detail', () => {
    it('collects multiple errors', () => {
      const input = validInput();
      delete input.schema_version;
      delete input.app_name;
      input.gateway_base_url = 'http://evil.example.com';
      (input as Record<string, unknown>).token = 'x';
      try {
        parseRuntimeConfig(input);
        expect.fail('should have thrown');
      } catch (e) {
        expect(e).toBeInstanceOf(RuntimeConfigError);
        const err = e as RuntimeConfigError;
        expect(err.fields.length).toBeGreaterThanOrEqual(4);
        expect(err.fields.some(f => f.includes('schema_version'))).toBe(true);
        expect(err.fields.some(f => f.includes('app_name'))).toBe(true);
        expect(err.fields.some(f => f.includes('gateway_base_url'))).toBe(true);
        expect(err.fields.some(f => f.includes('token'))).toBe(true);
      }
    });
  });
});
