const PROFILE_LABELS = {
  healthy: 'Healthy',
  bad_upstream: 'Bad Upstream',
  ssl_expired: 'SSL Expired',
  wrong_root: 'Wrong Root',
  high_load: 'High Load',
  crashed: 'Crashed',
};

function fmt(n) {
  if (n == null) return '—';
  return n.toLocaleString();
}

function esc(s) {
  if (s == null) return '';
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function renderCard(node) {
  const isWarning  = node.profile === 'high_load';
  const isHealthy  = node.profile === 'healthy';
  const cardClass  = isHealthy ? 'healthy' : (isWarning ? 'warning' : 'degraded');
  const dotClass   = isHealthy ? 'dot-green' : (isWarning ? 'dot-yellow' : 'dot-red');
  const badgeClass = isHealthy ? 'badge-healthy' : (isWarning ? 'badge-warning' : 'badge-degraded');
  const label = PROFILE_LABELS[node.profile] || esc(node.profile);

  return `
        <div class="card ${cardClass}">
          <div class="card-header">
            <span class="node-id">${esc(node.id)}</span>
            <span class="status-dot ${dotClass}"></span>
          </div>
          <span class="profile-badge ${badgeClass}">${label}</span>
          <div class="metrics">
            <span><span>HTTP Status</span><span class="val">${esc(node.http_status) || '—'}</span></span>
            <span><span>App Status</span><span class="val">${esc(node.app_status)}</span></span>
            <span><span>Latency</span><span class="val">${esc(node.latency_ms)}ms</span></span>
            ${node.uptime != null ? `<span><span>Uptime</span><span class="val">${fmt(node.uptime)}s</span></span>` : ''}
            ${node.requests != null ? `<span><span>Requests</span><span class="val">${fmt(node.requests)}</span></span>` : ''}
          </div>
          ${node.error ? `<div class="error-msg">${esc(node.error)}</div>` : ''}
        </div>
      `;
}

async function load() {
  // Set loading state in summary
  document.getElementById('summary').innerHTML = `
    <span class="summary-pill pill-total">Loading...</span>
    <button id="refresh-btn" onclick="load()" disabled>Refreshing...</button>
  `;

  const btn = document.getElementById('refresh-btn');
  if (btn) {
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span>Refreshing';
  }

  try {
    const res = await fetch('/api/fleet');
    const data = await res.json();

    const healthy   = data.nodes.filter(n => n.profile === 'healthy').length;
    const warning   = data.nodes.filter(n => n.profile === 'high_load').length;
    const unhealthy = data.nodes.length - healthy - warning;

    document.getElementById('summary').innerHTML = `
      <table class="summary-table">
        <thead><tr><th>Total</th><th>Healthy</th><th>Degraded</th><th>Unhealthy</th></tr></thead>
        <tbody><tr><td>${data.nodes.length}</td><td class="val-healthy">${healthy}</td><td class="val-warning">${warning}</td><td class="val-degraded">${unhealthy}</td></tr></tbody>
      </table>
      <div class="updated-section">
        <div class="updated-label">Last Updated</div>
        <div class="updated-row">
          <div class="updated-value">${new Date(data.checked_at).toLocaleTimeString()}</div>
          <button id="refresh-btn" onclick="load()">Refresh</button>
        </div>
      </div>
    `;

    document.getElementById('grid').innerHTML = data.nodes.map(renderCard).join('');
  } catch (e) {
    document.getElementById('summary').innerHTML = `
      <div class="updated-section">
        <div class="updated-label">Last Updated</div>
        <div class="updated-row">
          <div class="updated-value">Error loading fleet data</div>
          <button id="refresh-btn" onclick="load()">Refresh</button>
        </div>
      </div>
    `;
  }

  // Re-enable refresh button if it exists
  const finalBtn = document.getElementById('refresh-btn');
  if (finalBtn) {
    finalBtn.disabled = false;
    finalBtn.textContent = 'Refresh';
  }
}

// Initialize the dashboard
load();
setInterval(load, 15000);
