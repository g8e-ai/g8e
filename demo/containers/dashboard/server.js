const express = require('express');
const path = require('path');

const app = express();
const PORT = 3000;

// Serve static files from public directory
app.use(express.static('public'));

const NODES = [
  { id: 'node-01', profile: 'healthy' },
  { id: 'node-02', profile: 'healthy' },
  { id: 'node-03', profile: 'healthy' },
  { id: 'node-04', profile: 'healthy' },
  { id: 'node-05', profile: 'healthy' },
  { id: 'node-06', profile: 'bad_upstream' },
  { id: 'node-07', profile: 'ssl_expired' },
  { id: 'node-08', profile: 'wrong_root' },
  { id: 'node-09', profile: 'high_load' },
  { id: 'node-10', profile: 'crashed' },
];

const PROFILE_LABELS = {
  healthy: 'Healthy',
  bad_upstream: 'Bad Upstream',
  ssl_expired: 'SSL Expired',
  wrong_root: 'Wrong Root',
  high_load: 'High Load',
  crashed: 'Crashed',
};

async function checkNode(node) {
  const appUrl   = `http://${node.id}:5000/health`;
  const nginxUrl = `http://${node.id}:8181/health`;
  const start = Date.now();

  const [appRes, nginxRes] = await Promise.all([
    fetch(appUrl, { timeout: 3000 }).catch(err => ({ ok: false, _err: err.message })),
    fetch(nginxUrl, { timeout: 3000 }).catch(err => ({ ok: false, status: null, _err: err.message })),
  ]);
  const elapsed = Date.now() - start;

  let uptime = null, requests = null, appBody = null;
  if (appRes.ok) {
    try { appBody = await appRes.json(); } catch (_) {}
    uptime   = appBody?.uptime_seconds ?? null;
    requests = appBody?.requests_served ?? null;
  }

  if (nginxRes.ok) {
    return {
      ...node,
      http_status: nginxRes.status,
      app_status: 'healthy',
      uptime,
      requests,
      latency_ms: elapsed,
      error: null,
    };
  }

  const status = nginxRes.status || null;
  const errMsg = nginxRes._err || `HTTP ${status}`;
  return {
    ...node,
    http_status: status,
    app_status: nginxRes._err ? 'unreachable' : 'error',
    uptime,
    requests,
    latency_ms: elapsed,
    error: errMsg,
  };
}

app.get('/api/fleet', async (req, res) => {
  const results = await Promise.all(NODES.map(checkNode));
  res.json({ nodes: results, checked_at: new Date().toISOString() });
});

app.get('/', (req, res) => {
  res.sendFile(path.join(__dirname, 'public', 'index.html'));
});

app.listen(PORT, () => {
  console.log(`Fleet dashboard running on http://0.0.0.0:${PORT}`);
});
