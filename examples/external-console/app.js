// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

/**
 * g8e external browser frontend
 * Implements contracts from:
 *   docs/guides/build_frontend.md
 *   docs/guides/connect_frontend_to_gateway.md
 *   docs/guides/lovable.md
 * Wire shapes aligned with console/src/lib/webauthn.ts
 *
 * Serve this folder over HTTP (not file://), e.g. port 3003, then:
 *   ./g8e gw connect http://localhost:3003
 */

const CLI_SESSION_ID = 'browser';
const USER_ID_KEY = 'g8e_user_id';
const GATEWAY_URL_KEY = 'g8e_gateway_url';
const DEFAULT_GATEWAY = 'https://localhost:8443';
const MAX_EVENTS = 500;

// ---------------------------------------------------------------------------
// Base64url / WebAuthn helpers (mirror dashboard auth.js)
// ---------------------------------------------------------------------------
function base64urlToBuffer(base64url) {
  const base64 = String(base64url).replace(/-/g, '+').replace(/_/g, '/');
  const pad = base64.length % 4 === 0 ? '' : '='.repeat(4 - (base64.length % 4));
  const binary = atob(base64 + pad);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

function bufferToBase64url(buffer) {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

function decodeRegistrationOptions(options) {
  return {
    ...options,
    challenge: base64urlToBuffer(options.challenge),
    user: {
      ...options.user,
      id: base64urlToBuffer(options.user.id),
    },
    excludeCredentials: (options.excludeCredentials || []).map((c) => ({
      ...c,
      id: base64urlToBuffer(c.id),
    })),
  };
}

function decodeAuthenticationOptions(options) {
  return {
    ...options,
    challenge: base64urlToBuffer(options.challenge),
    allowCredentials: (options.allowCredentials || []).map((c) => ({
      ...c,
      id: base64urlToBuffer(c.id),
    })),
  };
}

/** Nested credential shape used by dashboard auth.js attestation/assertion_response */
function serializeCredential(credential) {
  const r = credential.response;
  const response = {};
  if (r.clientDataJSON != null) response.clientDataJSON = bufferToBase64url(r.clientDataJSON);
  if (r.attestationObject != null) response.attestationObject = bufferToBase64url(r.attestationObject);
  if (r.authenticatorData != null) response.authenticatorData = bufferToBase64url(r.authenticatorData);
  if (r.signature != null) response.signature = bufferToBase64url(r.signature);
  if (r.userHandle != null) response.userHandle = bufferToBase64url(r.userHandle);
  const transports = typeof r.getTransports === 'function' ? r.getTransports() : (r.transports ?? []);
  response.transports = transports;
  return {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    clientExtensionResults: credential.getClientExtensionResults?.() ?? {},
    response,
  };
}

/** Flat assertion body for POST /api/v1/approvals/{txHash}/verify */
function serializeAssertionFlat(credential) {
  const s = serializeCredential(credential);
  return {
    id: s.id,
    rawId: s.rawId,
    clientDataJSON: s.response.clientDataJSON,
    authenticatorData: s.response.authenticatorData,
    signature: s.response.signature,
    userHandle: s.response.userHandle,
  };
}

// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------
function getGatewayOrigin() {
  const raw = (localStorage.getItem(GATEWAY_URL_KEY) || DEFAULT_GATEWAY).trim().replace(/\/$/, '');
  return raw || DEFAULT_GATEWAY;
}

function setGatewayOrigin(url) {
  const cleaned = String(url || '').trim().replace(/\/$/, '');
  localStorage.setItem(GATEWAY_URL_KEY, cleaned || DEFAULT_GATEWAY);
}

async function gatewayFetch(path, options = {}) {
  const base = getGatewayOrigin().replace(/\/$/, '');
  // path is absolute on the Gateway (e.g. /api/v1/health or /api/v1/sse/events?x=1)
  const url = path.startsWith('http') ? path : base + (path.startsWith('/') ? path : '/' + path);
  const headers = { ...(options.headers || {}) };
  if (options.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
  const res = await fetch(url, {
    ...options,
    credentials: 'include',
    headers,
  });
  return res;
}

async function gatewayJson(path, options = {}) {
  const res = await gatewayFetch(path, options);
  let data = null;
  const text = await res.text();
  try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
  return { res, data };
}

// ---------------------------------------------------------------------------
// UI helpers
// ---------------------------------------------------------------------------
const $ = (id) => document.getElementById(id);

function toast(message, kind = 'ok') {
  const host = $('toasts');
  const el = document.createElement('div');
  el.className = `toast ${kind === 'err' ? 'err' : 'ok'}`;
  el.textContent = message;
  host.appendChild(el);
  setTimeout(() => el.remove(), 4500);
}

function setBusy(btn, busy) {
  if (!btn) return;
  btn.disabled = !!busy;
  if (busy) {
    btn.dataset.label = btn.dataset.label || btn.textContent;
    btn.textContent = 'Working…';
  } else if (btn.dataset.label) {
    btn.textContent = btn.dataset.label;
  }
}

function truncate(s, n = 24) {
  const t = String(s ?? '');
  return t.length <= n ? t : t.slice(0, n) + '…';
}

function setBadge(el, state, label) {
  el.className = `badge ${state}`;
  el.querySelector('.lbl').textContent = label;
}

// ---------------------------------------------------------------------------
// App state
// ---------------------------------------------------------------------------
const state = {
  loading: true,
  bootstrapped: false,
  user: null,
  webSessionId: null,
  passkeys: [],
  approvals: [],
  enrollments: [],
  events: [],
  sse: null,
  sseStatus: 'disconnected',
  reconnectTimer: null,
  reconnectDelay: 1000,
  lastEventId: null,
  pendingApprove: null,
  pendingEnrollmentToken: null,
  pendingRecovery: null,
  pendingPlatformEnrollment: null,
};

// ---------------------------------------------------------------------------
// Auth flows
// ---------------------------------------------------------------------------
async function checkBootstrap() {
  const { res, data } = await gatewayJson('/api/v1/auth/bootstrap/status');
  if (!res.ok) throw new Error(data?.error || `bootstrap status ${res.status}`);
  // docs: { bootstrapped: bool }
  state.bootstrapped = !!(data?.bootstrapped ?? data?.has_owner ?? data?.owner_exists);
  return state.bootstrapped;
}

async function refreshUser() {
  const { res, data } = await gatewayJson('/api/v1/users/me');
  if (res.status === 401) {
    state.user = null;
    return null;
  }
  if (!res.ok) throw new Error(data?.error || `users/me ${res.status}`);
  // dashboard expects data.success && data.user; docs also allow plain user
  const user = data?.user || (data?.id || data?.user_id ? data : null);
  if (user) {
    state.user = user;
    const uid = user.user_id || user.id || user.UserId;
    if (uid) localStorage.setItem(USER_ID_KEY, String(uid));
  } else if (data?.success === false) {
    state.user = null;
  }
  return state.user;
}

async function refreshSessionId() {
  try {
    const { res, data } = await gatewayJson('/api/v1/auth/sessions/me');
    if (!res.ok) { state.webSessionId = null; return null; }
    state.webSessionId = data?.web_session_id ?? data?.session_id ?? null;
    return state.webSessionId;
  } catch {
    state.webSessionId = null;
    return null;
  }
}

async function registerPasskey(userName) {
  const { res: cRes, data: challengeData } = await gatewayJson(
    '/api/v1/auth/passkeys/console/register/challenge',
    { method: 'POST', body: JSON.stringify({ user_name: userName, cli_session_id: CLI_SESSION_ID }) },
  );
  if (!cRes.ok || challengeData?.success === false) {
    throw new Error(challengeData?.error || `register challenge ${cRes.status}`);
  }
  // user_id for verify: challenge top-level or options.user.id (base64url string before decode)
  const userId =
    challengeData.user_id ||
    challengeData.options?.user?.id ||
    challengeData.options?.publicKey?.user?.id;
  if (!userId) throw new Error('Registration challenge missing user id');

  const publicKeyOptions = challengeData.options?.publicKey ?? challengeData.options;
  if (!publicKeyOptions?.challenge) throw new Error('Registration challenge missing publicKey options');

  const attestation = await navigator.credentials.create({
    publicKey: decodeRegistrationOptions(publicKeyOptions),
  });
  const { res: vRes, data: verifyData } = await gatewayJson(
    '/api/v1/auth/passkeys/console/register/verify',
    {
      method: 'POST',
      body: JSON.stringify({
        user_id: userId,
        cli_session_id: CLI_SESSION_ID,
        attestation_response: serializeCredential(attestation),
      }),
    },
  );
  if (!vRes.ok || verifyData?.success === false) {
    throw new Error(verifyData?.error || `register verify ${vRes.status}`);
  }
  localStorage.setItem(USER_ID_KEY, String(userId));
  await refreshUser();
  await refreshSessionId();
  return true;
}

async function authenticatePasskey(userId) {
  const resolved = String(userId || localStorage.getItem(USER_ID_KEY) || '').trim();
  if (!resolved) throw new Error('User ID required');

  const { res: cRes, data: challengeData } = await gatewayJson(
    '/api/v1/auth/passkeys/console/authenticate/challenge',
    { method: 'POST', body: JSON.stringify({ user_id: resolved }) },
  );
  if (!cRes.ok || challengeData?.success === false) {
    if (challengeData?.needs_setup) {
      const err = new Error('This user has no registered passkey. Run: g8e auth enroll user');
      err.needs_setup = true;
      throw err;
    }
    throw new Error(challengeData?.error || `authenticate challenge ${cRes.status}`);
  }

  const publicKeyOptions = challengeData.options?.publicKey ?? challengeData.options;
  const assertion = await navigator.credentials.get({
    publicKey: decodeAuthenticationOptions(publicKeyOptions),
  });
  const serialized = serializeCredential(assertion);
  const { res: vRes, data: verifyData } = await gatewayJson(
    '/api/v1/auth/passkeys/console/authenticate/verify',
    {
      method: 'POST',
      body: JSON.stringify({ user_id: resolved, assertion_response: serialized }),
    },
  );
  if (!vRes.ok || verifyData?.success === false) {
    throw new Error(verifyData?.error || `authenticate verify ${vRes.status}`);
  }
  localStorage.setItem(USER_ID_KEY, resolved);
  await refreshUser();
  await refreshSessionId();
  return true;
}

async function enrollWithToken(token) {
  const { res: cRes, data: challengeData } = await gatewayJson(
    '/api/v1/auth/passkeys/enrollment/register/challenge',
    { method: 'POST', body: JSON.stringify({ enrollment_token: token }) },
  );
  if (!cRes.ok || challengeData?.success === false) {
    const msg =
      cRes.status === 410 ? 'Enrollment link expired (5 min TTL). Run g8e auth enroll user again.'
      : cRes.status === 409 ? 'Enrollment link already used. Request a new token.'
      : cRes.status === 401 ? 'Invalid enrollment token.'
      : (challengeData?.error || `enrollment challenge ${cRes.status}`);
    throw new Error(msg);
  }
  const publicKeyOptions = challengeData.options?.publicKey ?? challengeData.options;
  const attestation = await navigator.credentials.create({
    publicKey: decodeRegistrationOptions(publicKeyOptions),
  });
  const { res: vRes, data: verifyData } = await gatewayJson(
    '/api/v1/auth/passkeys/enrollment/register/verify',
    {
      method: 'POST',
      body: JSON.stringify({
        enrollment_token: token,
        attestation_response: serializeCredential(attestation),
      }),
    },
  );
  if (!vRes.ok || verifyData?.success === false) {
    throw new Error(verifyData?.error || `enrollment verify ${vRes.status}`);
  }
  const uid = verifyData.user_id || challengeData.options?.user?.id;
  if (uid) localStorage.setItem(USER_ID_KEY, String(uid));
  await refreshUser();
  await refreshSessionId();
  return true;
}

async function logout() {
  try {
    await gatewayJson('/api/v1/auth/logout', { method: 'POST', body: '{}' });
  } catch { /* ignore */ }
  state.user = null;
  state.webSessionId = null;
  disconnectSSE(false);
}

// ---------------------------------------------------------------------------
// Dashboard data
// ---------------------------------------------------------------------------
async function loadPasskeys() {
  const { res, data } = await gatewayJson('/api/v1/auth/passkeys');
  if (res.status === 401) throw Object.assign(new Error('unauthorized'), { status: 401 });
  if (!res.ok) throw new Error(data?.error || `passkeys ${res.status}`);
  const list = data?.passkeys || data?.credentials || data?.items || (Array.isArray(data) ? data : []);
  state.passkeys = list;
  return list;
}

async function revokePasskey(credentialId) {
  const { res, data } = await gatewayJson(`/api/v1/auth/passkeys/${encodeURIComponent(credentialId)}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error(data?.error || `revoke ${res.status}`);
}

async function loadApprovals() {
  const { res, data } = await gatewayJson('/api/v1/approvals');
  if (res.status === 401) throw Object.assign(new Error('unauthorized'), { status: 401 });
  if (!res.ok) throw new Error(data?.error || `approvals ${res.status}`);
  const list = data?.approvals || data?.transactions || data?.items || (Array.isArray(data) ? data : []);
  state.approvals = list;
  return list;
}

async function approveTransaction(txHash) {
  const { res: cRes, data: challengeData } = await gatewayJson(`/api/v1/approvals/${encodeURIComponent(txHash)}/challenge`);
  if (!cRes.ok) throw new Error(challengeData?.error || `approval challenge ${cRes.status}`);
  const publicKeyOptions = challengeData.publicKey ?? challengeData.options?.publicKey ?? challengeData.options;
  const assertion = await navigator.credentials.get({
    publicKey: decodeAuthenticationOptions(publicKeyOptions),
  });
  const body = serializeAssertionFlat(assertion);
  const { res: vRes, data: verifyData } = await gatewayJson(
    `/api/v1/approvals/${encodeURIComponent(txHash)}/verify`,
    { method: 'POST', body: JSON.stringify(body) },
  );
  if (!vRes.ok) {
    // 403 may still contain ActionReceipt
    const msg = verifyData?.error || (vRes.status === 404 ? 'Transaction missing or expired' : `approval verify ${vRes.status}`);
    if (vRes.status === 403 && verifyData) {
      console.warn('Approval receipt (rejected):', verifyData);
    }
    throw new Error(msg);
  }
  return verifyData;
}

async function loadEnrollments() {
  const { res, data } = await gatewayJson('/api/v1/auth/platform-enrollments/pending');
  if (res.status === 401) throw Object.assign(new Error('unauthorized'), { status: 401 });
  if (res.status === 404 || res.status === 403) {
    state.enrollments = [];
    return [];
  }
  if (!res.ok) throw new Error(data?.error || `platform enrollments ${res.status}`);
  const list = data?.requests || data?.enrollments || data?.items || (Array.isArray(data) ? data : []);
  state.enrollments = list;
  return list;
}

async function decideEnrollment(requestId, decision) {
  const { res, data } = await gatewayJson('/api/v1/auth/platform-enrollments/decision', {
    method: 'POST',
    body: JSON.stringify({ request_id: requestId, decision }),
  });
  if (!res.ok) throw new Error(data?.error || `enrollment decision ${res.status}`);
  return data;
}

async function loadHealth() {
  const { res, data } = await gatewayJson('/api/v1/health');
  return { ok: res.ok, data, status: res.status };
}

// ---------------------------------------------------------------------------
// SSE
// ---------------------------------------------------------------------------
function setSseStatus(status) {
  state.sseStatus = status;
  setBadge($('sse-badge'), status, status);
}

function disconnectSSE(manual = true) {
  if (state.reconnectTimer) {
    clearTimeout(state.reconnectTimer);
    state.reconnectTimer = null;
  }
  if (state.sse) {
    state.sse.onopen = null;
    state.sse.onerror = null;
    state.sse.onmessage = null;
    state.sse.close();
    state.sse = null;
  }
  setSseStatus('disconnected');
  $('btn-sse-connect').disabled = false;
  $('btn-sse-disconnect').disabled = true;
  if (manual) state.reconnectDelay = 1000;
}

function connectSSE() {
  disconnectSSE(false);
  setSseStatus('connecting');
  $('btn-sse-connect').disabled = true;
  $('btn-sse-disconnect').disabled = false;

  const base = getGatewayOrigin().replace(/\/$/, '');
  let path = '/api/v1/sse/stream';
  // omit since_id to replay from beginning; use lastEventId for resume
  if (state.lastEventId) path += `?since_id=${encodeURIComponent(state.lastEventId)}`;
  const url = base + path;

  const es = new EventSource(url, { withCredentials: true });
  state.sse = es;

  es.onopen = () => {
    setSseStatus('connected');
    state.reconnectDelay = 1000;
  };

  es.onmessage = (event) => {
    if (event.lastEventId) state.lastEventId = event.lastEventId;
    let envelope;
    try { envelope = JSON.parse(event.data); } catch {
      envelope = { raw: event.data };
    }
    let nested = envelope?.event ?? envelope;
    if (typeof nested === 'string') {
      try { nested = JSON.parse(nested); } catch { /* keep string */ }
    }
    const type =
      (nested && typeof nested === 'object' && (nested.type || nested.event_type || nested.kind)) ||
      envelope?.event_type ||
      'message';
    const row = {
      id: event.lastEventId || String(Date.now()),
      type: String(type),
      ts: (nested && nested.timestamp) || (nested && nested.created_at) || new Date().toISOString(),
      envelope,
      nested,
    };
    state.events.push(row);
    if (state.events.length > MAX_EVENTS) state.events.splice(0, state.events.length - MAX_EVENTS);
    renderEvents();
  };

  es.onerror = () => {
    setSseStatus('reconnecting');
    // EventSource auto-reconnects; also schedule our own if closed
    if (es.readyState === EventSource.CLOSED) {
      disconnectSSE(false);
      const delay = Math.min(state.reconnectDelay, 30000) + Math.floor(Math.random() * 500);
      state.reconnectDelay = Math.min(state.reconnectDelay * 2, 30000);
      setSseStatus('reconnecting');
      state.reconnectTimer = setTimeout(() => {
        if (state.user) connectSSE();
      }, delay);
    }
  };
}

function renderEvents() {
  const filter = ($('sse-filter').value || '').trim().toLowerCase();
  const log = $('sse-log');
  const filtered = filter
    ? state.events.filter((e) => e.type.toLowerCase().includes(filter))
    : state.events;
  $('sse-count').textContent = `${filtered.length} event${filtered.length === 1 ? '' : 's'}`;

  const wasBottom = log.scrollHeight - log.scrollTop - log.clientHeight < 40;
  log.innerHTML = '';
  for (const ev of filtered.slice().reverse().slice(0, 200).reverse()) {
    const div = document.createElement('div');
    div.className = 'ev';
    const typeClass =
      /error|fail/i.test(ev.type) ? 'error'
      : /warn/i.test(ev.type) ? 'warning'
      : /success|ok|info/i.test(ev.type) ? 'success'
      : '';
    div.innerHTML = `<span class="ev-type ${typeClass}">${escapeHtml(ev.type)}</span>` +
      `<span class="ev-meta">${escapeHtml(String(ev.ts))} · id ${escapeHtml(String(ev.id))}</span>` +
      `<pre class="ev-payload"></pre>`;
    const pre = div.querySelector('.ev-payload');
    pre.textContent = JSON.stringify(ev.nested ?? ev.envelope, null, 2);
    div.addEventListener('click', () => div.classList.toggle('open'));
    log.appendChild(div);
  }
  if ($('sse-autoscroll').checked && wasBottom) log.scrollTop = log.scrollHeight;
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, '&')
    .replace(/</g, '<')
    .replace(/>/g, '>')
    .replace(/"/g, '"');
}

// ---------------------------------------------------------------------------
// URL hash handling (enroll / approve / recovery / platform-enrollment)
// ---------------------------------------------------------------------------
function parseHash() {
  if (!window.location.hash) return;
  const params = new URLSearchParams(window.location.hash.slice(1));
  const enroll = params.get('enroll');
  const token = params.get('token');
  const recovery = params.get('recovery');
  const platform = params.get('platform-enrollment');
  const approve = params.get('approve');

  if (recovery) {
    state.pendingRecovery = recovery;
    clearHash();
  }
  if ((enroll === '1' && token) || token) {
    state.pendingEnrollmentToken = token;
    clearHash();
  }
  if (platform) {
    state.pendingPlatformEnrollment = platform;
    clearHash();
  }
  if (approve) {
    state.pendingApprove = approve;
    // keep hash optional; clear after handling when authenticated
  }
}

function clearHash() {
  history.replaceState(null, '', window.location.pathname + window.location.search);
}

async function processPendingAuthenticatedActions() {
  if (state.pendingEnrollmentToken) {
    // handled earlier in boot if present
  }
  if (state.pendingRecovery && state.user) {
    const token = state.pendingRecovery;
    state.pendingRecovery = null;
    const approved = window.confirm('A new CLI is requesting enrollment access to this gateway.\n\nApprove this recovery request?');
    try {
      const { res, data } = await gatewayJson('/api/v1/auth/cli/recovery/approve', {
        method: 'POST',
        body: JSON.stringify({ token, approve: approved }),
      });
      if (!res.ok) throw new Error(data?.error || `recovery ${res.status}`);
      toast(approved ? 'CLI recovery approved' : 'CLI recovery denied');
    } catch (e) {
      toast(e.message, 'err');
    }
  }
  if (state.pendingPlatformEnrollment && state.user) {
    const id = state.pendingPlatformEnrollment;
    state.pendingPlatformEnrollment = null;
    const approved = window.confirm('A workload is requesting platform enrollment.\n\nApprove this enrollment request?');
    try {
      await decideEnrollment(id, approved ? 'approve' : 'deny');
      toast(approved ? 'Platform enrollment approved' : 'Platform enrollment denied');
      await loadDashboard();
    } catch (e) {
      toast(e.message, 'err');
    }
  }
  if (state.pendingApprove && state.user) {
    const tx = state.pendingApprove;
    state.pendingApprove = null;
    clearHash();
    try {
      await approveTransaction(tx);
      toast('Transaction approved');
      await loadDashboard();
    } catch (e) {
      toast(e.message, 'err');
    }
  }
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------
function showView(name) {
  $('view-loading').classList.toggle('hidden', name !== 'loading');
  $('view-login').classList.toggle('hidden', name !== 'login');
  $('view-dashboard').classList.toggle('hidden', name !== 'dashboard');
}

function renderLogin() {
  showView('login');
  $('login-bootstrap').classList.toggle('hidden', state.bootstrapped);
  $('login-sign-in').classList.toggle('hidden', !state.bootstrapped);
  const saved = localStorage.getItem(USER_ID_KEY) || '';
  if (saved) $('user-id').value = saved;
  $('header-user').textContent = 'not signed in';
}

function renderDashboard() {
  showView('dashboard');
  const u = state.user || {};
  const name = u.display_name || u.user_name || u.name || u.user_id || u.id || 'user';
  const uid = u.user_id || u.id || '—';
  $('header-user').textContent = name;
  $('account-user').textContent = `user_id: ${uid}`;
  $('account-session').textContent = `session: ${state.webSessionId || '— (cookie-backed)'}`;

  $('stat-passkeys').textContent = String(state.passkeys.length);
  $('stat-approvals').textContent = String(state.approvals.length);
  $('stat-enrollments').textContent = String(state.enrollments.length);

  // passkeys
  const pkList = $('passkey-list');
  pkList.innerHTML = '';
  $('passkey-empty').classList.toggle('hidden', state.passkeys.length > 0);
  for (const pk of state.passkeys) {
    const id = pk.credential_id || pk.id || pk.credentialId || '';
    const created = pk.created_at || pk.creation_time || pk.CreatedAt || '—';
    const last = pk.last_used_at || pk.last_used || pk.LastUsedAt || '—';
    const li = document.createElement('li');
    li.innerHTML = `<div><div class="mono">${escapeHtml(truncate(id, 28))}</div>
      <div class="muted">created ${escapeHtml(String(created))} · last used ${escapeHtml(String(last))}</div></div>`;
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'btn btn-sm btn-danger';
    btn.textContent = 'Revoke';
    btn.addEventListener('click', async () => {
      if (!confirm('Revoke this passkey?')) return;
      try {
        await revokePasskey(id);
        toast('Passkey revoked');
        await loadDashboard();
      } catch (e) { toast(e.message, 'err'); }
    });
    li.appendChild(btn);
    pkList.appendChild(li);
  }

  // approvals
  const aList = $('approval-list');
  aList.innerHTML = '';
  $('approval-empty').classList.toggle('hidden', state.approvals.length > 0);
  for (const a of state.approvals) {
    const hash = a.tx_hash || a.transaction_hash || a.hash || a.id || '';
    const tool = a.tool_name || a.tool || a.name || 'tool';
    const created = a.created_at || a.CreatedAt || '—';
    const exp = a.expires_at || a.expiry || a.ExpiresAt || '';
    const li = document.createElement('li');
    li.innerHTML = `<div><div><strong>${escapeHtml(String(tool))}</strong></div>
      <div class="mono">${escapeHtml(truncate(hash, 28))}</div>
      <div class="muted">created ${escapeHtml(String(created))}${exp ? ' · expires ' + escapeHtml(String(exp)) : ''}</div></div>`;
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'btn btn-sm btn-primary';
    btn.textContent = 'Approve';
    btn.addEventListener('click', async () => {
      setBusy(btn, true);
      try {
        await approveTransaction(hash);
        toast('Transaction approved');
        await loadDashboard();
      } catch (e) { toast(e.message, 'err'); }
      finally { setBusy(btn, false); }
    });
    li.appendChild(btn);
    aList.appendChild(li);
  }

  // enrollments
  const eList = $('enrollment-list');
  eList.innerHTML = '';
  $('enrollment-empty').classList.toggle('hidden', state.enrollments.length > 0);
  for (const en of state.enrollments) {
    const id = en.request_id || en.id || '';
    const kind = en.component_kind || en.kind || '—';
    const name = en.component_name || en.name || '—';
    const instance = en.instance_id || en.instance || '—';
    const st = en.state || '—';
    const exp = en.expires_at || en.expiry || '';
    const li = document.createElement('li');
    li.innerHTML = `<div>
      <div><strong>${escapeHtml(String(kind))}</strong> · ${escapeHtml(String(name))}</div>
      <div class="mono">request ${escapeHtml(truncate(id, 28))}</div>
      <div class="muted">instance ${escapeHtml(String(instance))} · ${escapeHtml(String(st))}${exp ? ' · exp ' + escapeHtml(String(exp)) : ''}</div>
    </div>`;
    const actions = document.createElement('div');
    actions.className = 'row';
    const approve = document.createElement('button');
    approve.type = 'button';
    approve.className = 'btn btn-sm btn-primary';
    approve.textContent = 'Approve';
    approve.addEventListener('click', async () => {
      try {
        await decideEnrollment(id, 'approve');
        toast('Enrollment approved');
        await loadDashboard();
      } catch (e) { toast(e.message, 'err'); }
    });
    const deny = document.createElement('button');
    deny.type = 'button';
    deny.className = 'btn btn-sm btn-danger';
    deny.textContent = 'Deny';
    deny.addEventListener('click', async () => {
      try {
        await decideEnrollment(id, 'deny');
        toast('Enrollment denied');
        await loadDashboard();
      } catch (e) { toast(e.message, 'err'); }
    });
    actions.append(approve, deny);
    li.appendChild(actions);
    eList.appendChild(li);
  }

  renderEvents();
}

async function loadDashboard() {
  try {
    await Promise.all([
      loadPasskeys().catch((e) => { if (e.status === 401) throw e; state.passkeys = []; }),
      loadApprovals().catch((e) => { if (e.status === 401) throw e; state.approvals = []; }),
      loadEnrollments().catch((e) => { if (e.status === 401) throw e; state.enrollments = []; }),
      loadHealth().then((h) => {
        const ver = h.data?.version || h.data?.gateway_version || (h.ok ? 'ok' : 'down');
        $('stat-version').textContent = String(ver);
        setBadge($('health-badge'), h.ok ? 'connected' : 'error', h.ok ? 'healthy' : `http ${h.status}`);
      }).catch(() => setBadge($('health-badge'), 'error', 'unreachable')),
    ]);
    renderDashboard();
  } catch (e) {
    if (e.status === 401) {
      state.user = null;
      disconnectSSE(true);
      renderLogin();
      toast('Session expired — sign in again', 'err');
      return;
    }
    toast(e.message, 'err');
    renderDashboard();
  }
}

// ---------------------------------------------------------------------------
// Boot
// ---------------------------------------------------------------------------
async function boot() {
  $('gateway-url').value = getGatewayOrigin();

  if (!window.PublicKeyCredential) {
    $('webauthn-warn').classList.remove('hidden');
  }

  parseHash();
  showView('loading');

  try {
    await checkBootstrap();
  } catch (e) {
    toast(`Cannot reach gateway (${getGatewayOrigin()}): ${e.message}`, 'err');
    setBadge($('health-badge'), 'error', 'unreachable');
    // still show login UI so user can fix URL
    state.bootstrapped = true;
    renderLogin();
    return;
  }

  if (state.pendingEnrollmentToken) {
    const token = state.pendingEnrollmentToken;
    state.pendingEnrollmentToken = null;
    try {
      await enrollWithToken(token);
      toast('Passkey enrolled');
      await loadDashboard();
      await processPendingAuthenticatedActions();
      return;
    } catch (e) {
      toast(e.message, 'err');
    }
  }

  try {
    await refreshUser();
    if (state.user) {
      await refreshSessionId();
      await loadDashboard();
      await processPendingAuthenticatedActions();
      return;
    }
  } catch (e) {
    console.warn('session check', e);
  }

  renderLogin();
}

// ---------------------------------------------------------------------------
// Event wiring
// ---------------------------------------------------------------------------
$('btn-save-gateway').addEventListener('click', () => {
  setGatewayOrigin($('gateway-url').value);
  $('gateway-url').value = getGatewayOrigin();
  toast(`Gateway set to ${getGatewayOrigin()}`);
  boot();
});

$('btn-health').addEventListener('click', async () => {
  try {
    const h = await loadHealth();
    setBadge($('health-badge'), h.ok ? 'connected' : 'error', h.ok ? 'healthy' : `http ${h.status}`);
    toast(h.ok ? `Healthy: ${JSON.stringify(h.data).slice(0, 120)}` : `Health failed (${h.status})`, h.ok ? 'ok' : 'err');
  } catch (e) {
    setBadge($('health-badge'), 'error', 'unreachable');
    toast(e.message, 'err');
  }
});

$('btn-register').addEventListener('click', async () => {
  const name = ($('reg-name').value || 'Owner').trim();
  const btn = $('btn-register');
  setBusy(btn, true);
  try {
    await registerPasskey(name);
    toast('Passkey registered — you are signed in');
    await loadDashboard();
  } catch (e) {
    toast(e.name === 'NotAllowedError' ? 'Registration cancelled' : e.message, 'err');
  } finally {
    setBusy(btn, false);
  }
});

$('btn-login').addEventListener('click', async () => {
  const btn = $('btn-login');
  setBusy(btn, true);
  try {
    await authenticatePasskey($('user-id').value);
    toast('Signed in');
    await loadDashboard();
    await processPendingAuthenticatedActions();
  } catch (e) {
    toast(e.name === 'NotAllowedError' ? 'Sign-in cancelled' : e.message, 'err');
  } finally {
    setBusy(btn, false);
  }
});

$('btn-logout').addEventListener('click', async () => {
  await logout();
  toast('Signed out');
  try { await checkBootstrap(); } catch { /* ignore */ }
  renderLogin();
});

$('btn-refresh').addEventListener('click', () => loadDashboard());

$('btn-sse-connect').addEventListener('click', () => connectSSE());
$('btn-sse-disconnect').addEventListener('click', () => disconnectSSE(true));
$('btn-sse-clear').addEventListener('click', () => {
  state.events = [];
  renderEvents();
});
$('sse-filter').addEventListener('input', () => renderEvents());

boot();
