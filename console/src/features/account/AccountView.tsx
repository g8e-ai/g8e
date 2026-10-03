// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useCallback, useEffect, useState } from 'react';
import { Empty, shortId } from '../../components/ui';
import { api } from '../../lib/api';
import { Paths } from '../../lib/paths';
import type { PasskeyCredential } from '../../lib/types';
import { useSession } from '../../state/session';
import { errorText, useToast } from '../../state/toast';

/**
 * The credential list carries IDs as standard base64 (Go's []byte encoding);
 * the revoke route matches the unpadded base64url form.
 */
export function credentialPathId(stdBase64: string): string {
  return stdBase64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function fromUnixMs(ms: number | undefined): string {
  return ms ? new Date(ms).toLocaleString() : '—';
}

export function AccountView() {
  const { user, webSessionId, version, signOut } = useSession();
  const toast = useToast();
  const [passkeys, setPasskeys] = useState<PasskeyCredential[]>([]);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api.get<{ credentials?: PasskeyCredential[] }>(Paths.passkeys);
      setPasskeys(res.credentials ?? []);
    } catch (err) {
      toast('error', errorText(err));
    } finally {
      setLoaded(true);
    }
  }, [toast]);

  useEffect(() => {
    void load();
  }, [load]);

  const revoke = async (cred: PasskeyCredential) => {
    if (passkeys.length <= 1) {
      toast('error', 'This is your only passkey. Enroll another with `g8e auth enroll user` before revoking it.');
      return;
    }
    if (!window.confirm('Revoke this passkey? Devices using it will no longer be able to sign in.')) return;
    try {
      await api.del(Paths.passkey(credentialPathId(cred.id)));
      toast('success', 'Passkey revoked.');
    } catch (err) {
      toast('error', errorText(err));
    } finally {
      await load();
    }
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>Account</h1>
          <p>Your identity and passkeys on this Gateway.</p>
        </div>
        <button type="button" className="btn btn-danger" onClick={() => void signOut()}>
          Sign out
        </button>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>Identity</h2>
        </div>
        <dl className="kv">
          <dt>User ID</dt>
          <dd className="mono">{user?.id}</dd>
          <dt>Web session</dt>
          <dd className="mono">{shortId(webSessionId ?? undefined, 20)}</dd>
          <dt>Gateway</dt>
          <dd>{version ?? '—'}</dd>
        </dl>
      </div>

      <div className="card">
        <div className="card-head">
          <h2>Passkeys</h2>
          <span className="muted">
            Add a device with <code>g8e auth enroll user</code>
          </span>
        </div>
        {loaded && passkeys.length === 0 ? (
          <Empty title="No passkeys" />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Credential</th>
                <th>Created</th>
                <th>Last used</th>
                <th className="actions" aria-label="Actions" />
              </tr>
            </thead>
            <tbody>
              {passkeys.map((p) => (
                <tr key={p.id}>
                  <td className="mono" title={p.id}>
                    {shortId(credentialPathId(p.id), 20)}
                  </td>
                  <td className="text-2">{fromUnixMs(p.created_at_unix_ms)}</td>
                  <td className="text-2">{fromUnixMs(p.last_used_at_unix_ms)}</td>
                  <td className="actions">
                    <button type="button" className="btn btn-sm btn-danger" onClick={() => void revoke(p)}>
                      Revoke
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
