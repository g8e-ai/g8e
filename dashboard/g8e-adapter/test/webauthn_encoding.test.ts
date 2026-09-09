import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';

import { base64urlToBuffer, bufferToBase64url } from '../src/webauthn/encoding';

describe('WebAuthn encoding', () => {
  describe('base64urlToBuffer', () => {
    it('converts a base64url string to an ArrayBuffer', () => {
      // "hello" in base64url = "aGVsbG8"
      const buf = base64urlToBuffer('aGVsbG8');
      const bytes = new Uint8Array(buf);
      expect(bytes.length).toBe(5);
      expect(String.fromCharCode(...bytes)).toBe('hello');
    });

    it('handles URL-safe characters - and _', () => {
      // A buffer that produces + and / in standard base64
      // bytes [0xfb, 0xff, 0xbf] -> base64 = "+/+/"
      // base64url = "-_-_"
      const buf = base64urlToBuffer('-_-_');
      const bytes = new Uint8Array(buf);
      expect(bytes[0]).toBe(0xfb);
      expect(bytes[1]).toBe(0xff);
      expect(bytes[2]).toBe(0xbf);
    });

    it('does not require padding', () => {
      // "test" in base64 = "dGVzdA==" -> base64url = "dGVzdA"
      const buf = base64urlToBuffer('dGVzdA');
      const bytes = new Uint8Array(buf);
      expect(String.fromCharCode(...bytes)).toBe('test');
    });

    it('handles empty string', () => {
      const buf = base64urlToBuffer('');
      expect(buf.byteLength).toBe(0);
    });
  });

  describe('bufferToBase64url', () => {
    it('converts an ArrayBuffer to a base64url string', () => {
      const bytes = new Uint8Array([104, 101, 108, 108, 111]); // "hello"
      const b64url = bufferToBase64url(bytes.buffer);
      expect(b64url).toBe('aGVsbG8');
    });

    it('produces URL-safe characters (no +, /, or =)', () => {
      const bytes = new Uint8Array([0xfb, 0xff, 0xbf]);
      const b64url = bufferToBase64url(bytes.buffer);
      expect(b64url).toBe('-_-_');
      expect(b64url).not.toContain('+');
      expect(b64url).not.toContain('/');
      expect(b64url).not.toContain('=');
    });

    it('does not include padding', () => {
      const bytes = new Uint8Array([116, 101, 115, 116]); // "test"
      const b64url = bufferToBase64url(bytes.buffer);
      expect(b64url).toBe('dGVzdA');
      expect(b64url).not.toContain('=');
    });

    it('handles empty buffer', () => {
      const b64url = bufferToBase64url(new ArrayBuffer(0));
      expect(b64url).toBe('');
    });
  });

  describe('round-trip', () => {
    it('round-trips arbitrary bytes', () => {
      const original = new Uint8Array([0, 1, 2, 254, 255, 128, 64, 32]);
      const b64url = bufferToBase64url(original.buffer);
      const recovered = new Uint8Array(base64urlToBuffer(b64url));
      expect(Array.from(recovered)).toEqual(Array.from(original));
    });
  });
});
