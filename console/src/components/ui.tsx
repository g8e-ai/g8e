// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useState, type ReactNode } from 'react';

const OK = new Set(['active', 'open', 'approved', 'completed', 'resolved']);
const ACCENT = new Set(['bound']);
const WARN = new Set(['available', 'stale', 'pending', 'issuing', 'requested', 'running', 'in_progress', 'escalated']);
const BAD = new Set(['offline', 'stopped', 'terminated', 'unavailable', 'failed', 'denied', 'expired', 'closed']);

export function StatusPill({ status, label }: { status: string; label?: string }) {
  const s = status.toLowerCase();
  const tone = OK.has(s) ? 'pill-ok' : ACCENT.has(s) ? 'pill-accent' : WARN.has(s) ? 'pill-warn' : BAD.has(s) ? 'pill-bad' : '';
  return <span className={`pill ${tone}`}>{label ?? s.replace(/_/g, ' ')}</span>;
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <strong>{title}</strong>
      {children}
    </div>
  );
}

async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.style.position = 'fixed';
  ta.style.opacity = '0';
  document.body.appendChild(ta);
  ta.select();
  document.execCommand('copy');
  ta.remove();
}

export function CodeLine({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="code-line">
      <code>{text}</code>
      <button
        type="button"
        className="btn btn-sm"
        onClick={() => {
          void copyText(text).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          });
        }}
      >
        {copied ? 'Copied' : 'Copy'}
      </button>
    </div>
  );
}

export function shortId(id: string | undefined, n = 12): string {
  if (!id) return '';
  return id.length > n ? `${id.slice(0, n)}…` : id;
}

export function relativeTime(iso: string | undefined | null, now = Date.now()): string {
  if (!iso) return '';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  const s = Math.round((now - t) / 1000);
  if (s < 45) return 'just now';
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.round(h / 24);
  if (d < 30) return `${d}d ago`;
  return new Date(t).toLocaleDateString();
}

export function dateTime(iso: string | undefined | null): string {
  if (!iso) return '';
  const t = Date.parse(iso);
  return Number.isNaN(t) ? iso : new Date(t).toLocaleString();
}
