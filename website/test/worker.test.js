import assert from 'node:assert/strict';
import test from 'node:test';
import worker, { releaseKey } from '../worker.js';

test('releaseKey selects a Linux artifact for the convenience endpoint', () => {
  assert.equal(releaseKey('/g8e', 'curl aarch64'), 'g8e-linux-arm64');
  assert.equal(releaseKey('/g8e', 'curl x86_64'), 'g8e-linux-amd64');
});

test('releaseKey accepts only known release artifact names', () => {
  assert.equal(releaseKey('/g8e-windows-amd64.exe.sha256'), 'g8e-windows-amd64.exe.sha256');
  assert.equal(releaseKey('/private-file'), null);
  assert.equal(releaseKey('/g8e-linux-ppc64'), null);
});

test('worker serves site paths from static assets with security headers', async () => {
  const assetsResponse = new Response('<h1>g8e</h1>', { headers: { 'content-type': 'text/html' } });
  const response = await worker.fetch(new Request('https://g8e.ai/'), {
    ASSETS: { fetch: async () => assetsResponse },
    RELEASES: { get: async () => { throw new Error('unexpected release lookup'); } }
  });
  assert.equal(await response.text(), '<h1>g8e</h1>');
  assert.equal(response.headers.get('x-content-type-options'), 'nosniff');
  assert.match(response.headers.get('content-security-policy'), /frame-ancestors 'none'/);
});

test('worker serves release artifacts from R2', async () => {
  const response = await worker.fetch(new Request('https://g8e.ai/g8e-linux-amd64.sha256'), {
    ASSETS: { fetch: async () => { throw new Error('unexpected asset lookup'); } },
    RELEASES: { get: async key => ({ body: `${key}-hash`, size: 32, httpEtag: 'etag' }) }
  });
  assert.equal(response.status, 200);
  assert.equal(response.headers.get('access-control-allow-origin'), '*');
  assert.equal(await response.text(), 'g8e-linux-amd64.sha256-hash');
});
