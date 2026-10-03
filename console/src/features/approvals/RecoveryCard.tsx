// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// CLI recovery approval: a new CLI asks to enroll for this Gateway and an
// existing signed-in user authorizes it. The opaque token came from the
// #recovery= fragment and lives only in memory.

import { useCallback, useEffect, useState } from 'react';
import { StatusPill } from '../../components/ui';
import { api, ApiError } from '../../lib/api';
import { Paths } from '../../lib/paths';
import { errorText, useToast } from '../../state/toast';

const LABEL: Record<string, string> = {
  pending: 'Awaiting your decision',
  approved: 'Approved — the new CLI may now complete enrollment',
  completed: 'Completed — the new CLI finished enrollment',
  denied: 'Denied',
  expired: 'Expired',
  not_found: 'This recovery link is invalid',
  consumed: 'This recovery link was already used',
  error: 'Could not reach the Gateway',
};

export function RecoveryCard({ token }: { token: string }) {
  const toast = useToast();
  const [state, setState] = useState<string>('loading');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const d = await api.get<{ state: string }>(Paths.cliRecoveryStatus(token), { sessionBound: false });
      setState(d.state);
    } catch (err) {
      const s = err instanceof ApiError ? err.status : 0;
      setState(s === 404 ? 'not_found' : s === 410 ? 'expired' : s === 409 ? 'consumed' : 'error');
    }
  }, [token]);

  useEffect(() => {
    void load();
  }, [load]);

  const decide = async (approve: boolean) => {
    setBusy(true);
    try {
      const d = await api.post<{ success: boolean; state?: string; error?: string }>(Paths.cliRecoveryApprove, { token, approve });
      if (!d.success) throw new Error(d.error || 'Recovery decision failed.');
      setState(d.state ?? (approve ? 'approved' : 'denied'));
      toast('success', approve ? 'CLI recovery approved.' : 'CLI recovery denied.');
    } catch (err) {
      toast('error', errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className={`card ${state === 'pending' ? 'card-attention' : ''}`} style={{ marginBottom: 16 }}>
      <div className="card-head">
        <h2>CLI recovery request</h2>
        {state !== 'loading' && <StatusPill status={state} label={state.replace(/_/g, ' ')} />}
      </div>
      <p className="text-2" style={{ marginTop: 0 }}>
        A new CLI is asking for enrollment credentials on this Gateway. Approve only if you started this recovery.
      </p>
      <p className="muted">{LABEL[state] ?? (state === 'loading' ? 'Checking request…' : state)}</p>
      {state === 'pending' && (
        <div className="row">
          <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void decide(true)}>
            Approve
          </button>
          <button type="button" className="btn btn-danger" disabled={busy} onClick={() => void decide(false)}>
            Deny
          </button>
        </div>
      )}
    </div>
  );
}
