const express = require('express');
const path = require('path');

const app = express();
const PORT = 3000;

// Serve static files from public directory
app.use(express.static('public'));

const NODES = [
  { id: 'node-01' },
  { id: 'node-02' },
  { id: 'node-03' },
  { id: 'node-04' },
  { id: 'node-05' },
  { id: 'node-06' },
  { id: 'node-07' },
  { id: 'node-08' },
  { id: 'node-09' },
  { id: 'node-10' },
];

async function checkNode(node) {
  const appUrl   = `http://${node.id}:5000/health`;
  const nginxUrl = `http://${node.id}:8181/`;
  const start = Date.now();

  // Check nginx HTTP status
  const nginxRes = await fetch(nginxUrl, { timeout: 3000 }).catch(err => ({ ok: false, status: null, _err: err.message }));
  
  // Check app health
  const appRes = await fetch(appUrl, { timeout: 3000 }).catch(err => ({ ok: false, _err: err.message }));
  const elapsed = Date.now() - start;

  let uptime = null, requests = null, appBody = null;
  if (appRes.ok) {
    try { appBody = await appRes.json(); } catch (_) {}
    uptime   = appBody?.uptime_seconds ?? null;
    requests = appBody?.requests_served ?? null;
  }

  // Determine real status based on actual HTTP response
  let profile, app_status, error, nginx_running;
  
  if (nginxRes.ok && nginxRes.status === 200) {
    profile = 'healthy';
    app_status = 'healthy';
    error = null;
    nginx_running = true;
  } else if (nginxRes.status === 502) {
    profile = 'bad_upstream';
    app_status = 'error';
    error = '502 Bad Gateway';
    nginx_running = true;
  } else if (nginxRes.status === 404) {
    profile = 'wrong_root';
    app_status = 'error';
    error = '404 Not Found';
    nginx_running = true;
  } else if (nginxRes.status === 504) {
    profile = 'high_load';
    app_status = 'warning';
    error = '504 Gateway Timeout';
    nginx_running = true;
  } else if (nginxRes._err) {
    profile = 'crashed';
    app_status = 'down';
    error = 'nginx unreachable';
    nginx_running = false;
  } else {
    profile = 'degraded';
    app_status = 'error';
    error = `HTTP ${nginxRes.status}`;
    nginx_running = true;
  }

  return {
    ...node,
    profile,
    http_status: nginxRes.status,
    app_status,
    uptime,
    requests,
    latency_ms: elapsed,
    error,
    nginx_running,
  };
}

app.get('/api/fleet', async (req, res) => {
  const results = await Promise.all(NODES.map(checkNode));
  res.json({ nodes: results, checked_at: new Date().toISOString() });
});

app.listen(PORT, () => {
  console.log(`Fleet dashboard running on http://0.0.0.0:${PORT}`);
});
