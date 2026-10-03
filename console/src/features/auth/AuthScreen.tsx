// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useEffect, useState, type FormEvent } from 'react';
import {
  authenticatePasskey,
  registerBootstrapPasskey,
  registerEnrollmentPasskey,
  webAuthnAvailable,
} from '../../lib/webauthn';
import { useSession } from '../../state/session';
import { errorText } from '../../state/toast';

// One-time tokens are attempted at most once per page load, even if the
// screen remounts after a later sign-out.
const attemptedTokens = new Set<string>();

interface Props {
  /** One-time CLI enrollment token from #enroll=1&token=…, already cleared from the URL. */
  enrollmentToken?: string;
  /** Deep-link reasons to sign in, shown above the form. */
  pendingReasons: string[];
}

export function AuthScreen({ enrollmentToken, pendingReasons }: Props) {
  const { bootstrapped, refresh, version } = useSession();
  const [userId, setUserId] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);

  const run = async (fn: () => Promise<void>, prompt: string) => {
    setBusy(true);
    setError(null);
    setInfo(prompt);
    try {
      await fn();
      setInfo(null);
      await refresh();
    } catch (err) {
      setInfo(null);
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    if (!enrollmentToken || attemptedTokens.has(enrollmentToken)) return;
    attemptedTokens.add(enrollmentToken);
    void run(() => registerEnrollmentPasskey(enrollmentToken), 'Follow your browser prompt to create your passkey…');
  }, [enrollmentToken]);

  const supported = webAuthnAvailable();

  const onSignIn = (e: FormEvent) => {
    e.preventDefault();
    const id = userId.trim();
    if (!id) return;
    void run(() => authenticatePasskey(id), 'Follow your browser prompt to sign in…');
  };

  const onBootstrap = (e: FormEvent) => {
    e.preventDefault();
    void run(() => registerBootstrapPasskey(displayName.trim()), 'Follow your browser prompt to create your passkey…');
  };

  let body;
  if (enrollmentToken && (busy || error || info)) {
    body = (
      <>
        <h1>Enroll your passkey</h1>
        <p>Your CLI started a passkey enrollment for this Gateway. Complete the browser prompt to finish.</p>
        {error && (
          <p className="muted">
            Enrollment links are single-use. Run <code>g8e auth enroll user</code> again to get a new one.
          </p>
        )}
      </>
    );
  } else if (!bootstrapped) {
    body = (
      <form onSubmit={onBootstrap}>
        <h1>Set up this Gateway</h1>
        <p>No owner exists yet. Create the owner passkey to secure this Gateway.</p>
        <div className="field">
          <label htmlFor="display-name">Display name</label>
          <input
            id="display-name"
            className="input"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder="e.g. Platform owner"
            autoComplete="name"
          />
        </div>
        <button type="submit" className="btn btn-primary" disabled={busy || !supported}>
          Create owner passkey
        </button>
      </form>
    );
  } else {
    body = (
      <form onSubmit={onSignIn}>
        <h1>Sign in</h1>
        <p>Use the passkey registered to your g8e user.</p>
        <div className="field">
          <label htmlFor="user-id">User ID</label>
          <input
            id="user-id"
            className="input mono"
            value={userId}
            onChange={(e) => setUserId(e.target.value)}
            placeholder="Your g8e user ID"
            autoComplete="username webauthn"
            spellCheck={false}
            required
          />
        </div>
        <button type="submit" className="btn btn-primary" disabled={busy || !supported || !userId.trim()}>
          Sign in with passkey
        </button>
        <div className="auth-foot">
          Find your user ID with <code>g8e auth context</code>. New device? Run <code>g8e auth enroll user</code>.
        </div>
      </form>
    );
  }

  return (
    <div className="auth">
      <div className="auth-panel">
        <div className="brand">
          <span className="brand-mark">g8</span>
          <span>g8e Console</span>
        </div>
        {pendingReasons.map((r) => (
          <div key={r} className="notice notice-warn" style={{ marginBottom: 12 }}>
            {r}
          </div>
        ))}
        <div className="card">
          {!supported && (
            <div className="notice notice-error" style={{ marginBottom: 16 }}>
              This browser does not support passkeys (WebAuthn). Use a current Chrome, Edge, Firefox, or Safari over HTTPS.
            </div>
          )}
          {body}
          {info && (
            <div className="notice" style={{ marginTop: 14 }}>
              {info}
            </div>
          )}
          {error && (
            <div className="notice notice-error" style={{ marginTop: 14 }} role="alert">
              {error}
            </div>
          )}
        </div>
        <div className="auth-foot">{version ? `Gateway ${version}` : 'g8e Gateway'} · © 2026 Lateralus Labs, LLC.</div>
      </div>
    </div>
  );
}
