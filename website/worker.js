const RELEASE_PATH = /^g8e-(?:linux-(?:amd64|arm64|386)|darwin-(?:amd64|arm64)|windows-(?:amd64|arm64)\.exe)(?:\.sha256|\.sig)?$/;
const SECURITY_HEADERS = {
  'content-security-policy': "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' https: data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'",
  'permissions-policy': 'camera=(), microphone=(), geolocation=()',
  'referrer-policy': 'strict-origin-when-cross-origin',
  'strict-transport-security': 'max-age=31536000; includeSubDomains',
  'x-content-type-options': 'nosniff',
  'x-frame-options': 'DENY'
};

export function releaseKey(pathname, userAgent = '') {
  const key = pathname.replace(/^\/+/, '');
  if (key !== 'g8e') return RELEASE_PATH.test(key) ? key : null;
  if (/arm64|aarch64/i.test(userAgent)) return 'g8e-linux-arm64';
  if (/i386|i686/i.test(userAgent)) return 'g8e-linux-386';
  return 'g8e-linux-amd64';
}

function withSecurityHeaders(response) {
  const secured = new Response(response.body, response);
  Object.entries(SECURITY_HEADERS).forEach(([name, value]) => secured.headers.set(name, value));
  return secured;
}

async function releaseResponse(request, env, key) {
  if (request.method === 'OPTIONS' && /\.(?:sha256|sig)$/.test(key)) {
    return new Response(null, { status: 204, headers: { 'access-control-allow-origin': '*', 'access-control-allow-methods': 'GET, HEAD, OPTIONS' } });
  }
  if (!['GET', 'HEAD'].includes(request.method)) return new Response('method not allowed', { status: 405, headers: { allow: 'GET, HEAD, OPTIONS' } });
  const object = await env.RELEASES.get(key);
  if (!object) return new Response('not found', { status: 404 });
  const textArtifact = /\.(?:sha256|sig)$/.test(key);
  const headers = new Headers({
    'cache-control': 'public, max-age=300',
    'content-length': String(object.size),
    'content-type': textArtifact ? 'text/plain; charset=utf-8' : 'application/octet-stream',
    etag: object.httpEtag
  });
  if (textArtifact) headers.set('access-control-allow-origin', '*');
  else headers.set('content-disposition', `attachment; filename="${key}"`);
  return new Response(request.method === 'HEAD' ? null : object.body, { headers });
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const key = releaseKey(url.pathname, request.headers.get('user-agent') ?? '');
    if (key) return withSecurityHeaders(await releaseResponse(request, env, key));
    return withSecurityHeaders(await env.ASSETS.fetch(request));
  }
};
