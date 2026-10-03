// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useState } from 'react';
import { Empty, StatusPill, relativeTime, shortId } from '../../components/ui';
import type { Operator } from '../../lib/types';
import { canBind, operatorLabel, useOperators } from '../../state/operators';
import { useSession } from '../../state/session';
import { errorText, useToast } from '../../state/toast';
import { DeployPanel } from './DeployPanel';

export function OperatorsView() {
  const { operators, loaded, error, bound, bind, unbind, stop, reload } = useOperators();
  const { webSessionId } = useSession();
  const toast = useToast();
  const [pending, setPending] = useState<string | null>(null);
  const [showDeploy, setShowDeploy] = useState(false);

  const act = async (key: string, fn: () => Promise<unknown>, done: string) => {
    setPending(key);
    try {
      await fn();
      toast('success', done);
    } catch (err) {
      toast('error', errorText(err));
    } finally {
      setPending(null);
    }
  };

  const bindOne = (op: Operator) =>
    act(
      op.id,
      async () => {
        const res = await bind([op.id]);
        if (!res.success || (res.failed_count ?? 0) > 0) throw new Error(res.error || 'The Gateway could not bind this Operator.');
      },
      `Bound ${operatorLabel(op)} to this session.`,
    );

  const unbindOne = (op: Operator) =>
    act(
      op.id,
      async () => {
        const res = await unbind([op.id]);
        if (!res.success) throw new Error(res.error || 'The Gateway could not unbind this Operator.');
      },
      `Unbound ${operatorLabel(op)}.`,
    );

  const stopOne = (op: Operator) => {
    if (!window.confirm(`Stop ${operatorLabel(op)}? The Operator process will shut down and must be restarted on its host.`)) return;
    void act(op.id, () => stop(op.id), `Stop requested for ${operatorLabel(op)}.`);
  };

  const bindable = operators.filter((op) => canBind(op) && op.bound_web_session_id !== webSessionId);

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>Operators</h1>
          <p>
            Operators execute governed actions on their hosts. Bind an Operator to this session to let investigations act on
            it.
          </p>
        </div>
        <div className="row">
          {bindable.length > 1 && (
            <button
              type="button"
              className="btn"
              disabled={pending !== null}
              onClick={() =>
                void act('all', () => bind(bindable.map((o) => o.id)), `Bound ${bindable.length} Operators to this session.`)
              }
            >
              Bind all ({bindable.length})
            </button>
          )}
          <button type="button" className="btn btn-primary" onClick={() => setShowDeploy((v) => !v)}>
            {showDeploy ? 'Hide deploy' : 'Deploy an Operator'}
          </button>
        </div>
      </div>

      {showDeploy && <DeployPanel />}

      <div className="card" style={{ padding: 0, marginTop: showDeploy ? 16 : 0 }}>
        <div className="card-head" style={{ padding: '16px 20px 0' }}>
          <h2>Inventory</h2>
          <div className="row">
            <span className="muted">
              {bound.length} bound to this session · {operators.length} total
            </span>
            <button type="button" className="btn btn-sm btn-ghost" onClick={() => void reload()}>
              Refresh
            </button>
          </div>
        </div>
        {error && (
          <div className="notice notice-error" style={{ margin: '0 20px 16px' }}>
            {error}
          </div>
        )}
        {loaded && operators.length === 0 ? (
          <Empty title="No Operators yet">Deploy an Operator on a host to see it here.</Empty>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Operator</th>
                  <th>Status</th>
                  <th>Type</th>
                  <th>Last heartbeat</th>
                  <th>Binding</th>
                  <th className="actions" aria-label="Actions" />
                </tr>
              </thead>
              <tbody>
                {operators.map((op) => {
                  const isBound = !!webSessionId && op.bound_web_session_id === webSessionId;
                  const elsewhere = !!op.bound_web_session_id && !isBound;
                  const busy = pending === op.id || pending === 'all';
                  return (
                    <tr key={op.id} className={isBound ? 'bound-row' : undefined}>
                      <td>
                        <div style={{ fontWeight: 500 }}>{operatorLabel(op)}</div>
                        <div className="mono muted">{shortId(op.id, 18)}</div>
                      </td>
                      <td>
                        <StatusPill status={op.status} />
                      </td>
                      <td className="text-2">{op.operator_type || '—'}</td>
                      <td className="text-2">{relativeTime(op.last_heartbeat_at) || '—'}</td>
                      <td>
                        {isBound ? (
                          <span className="pill pill-accent">This session</span>
                        ) : elsewhere ? (
                          <span className="muted">Another session</span>
                        ) : (
                          <span className="muted">Unbound</span>
                        )}
                      </td>
                      <td className="actions">
                        <div className="row" style={{ justifyContent: 'flex-end' }}>
                          {isBound ? (
                            <button type="button" className="btn btn-sm" disabled={busy} onClick={() => void unbindOne(op)}>
                              Unbind
                            </button>
                          ) : (
                            <button
                              type="button"
                              className="btn btn-sm btn-primary"
                              disabled={busy || !canBind(op)}
                              title={canBind(op) ? undefined : 'Only Operators with a live session can be bound.'}
                              onClick={() => void bindOne(op)}
                            >
                              {elsewhere ? 'Take over' : 'Bind'}
                            </button>
                          )}
                          {op.operator_type === 'remote' && op.status === 'active' && (
                            <button type="button" className="btn btn-sm btn-danger" disabled={busy} onClick={() => stopOne(op)}>
                              Stop
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

