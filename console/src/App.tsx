// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useState } from 'react';
import { AccountView } from './features/account/AccountView';
import { ApiView } from './features/api/ApiView';
import { ApprovalsView } from './features/approvals/ApprovalsView';
import { AuthScreen } from './features/auth/AuthScreen';
import { CasesView } from './features/cases/CasesView';
import { InferenceView } from './features/inference/InferenceView';
import { OperatorsView } from './features/operators/OperatorsView';
import type { FragmentIntent } from './lib/fragment';
import { ApprovalsProvider, useApprovals } from './state/approvals';
import { InferenceProvider } from './state/inference';
import { OperatorsProvider, useOperators } from './state/operators';
import { useSession } from './state/session';
import { StreamProvider, useStream } from './state/stream';

export type View = 'cases' | 'operators' | 'inference' | 'approvals' | 'api' | 'account';
const VIEWS: readonly View[] = ['cases', 'operators', 'inference', 'approvals', 'api', 'account'];

export function initialView(search: string, intent: FragmentIntent): View {
  if (intent.approveTxHash || intent.recoveryToken || intent.platformEnrollmentId) return 'approvals';
  const v = new URLSearchParams(search).get('view');
  return VIEWS.includes(v as View) ? (v as View) : 'cases';
}

function writeView(view: View): void {
  const p = new URLSearchParams(window.location.search);
  if (view === 'cases') p.delete('view');
  else p.set('view', view);
  const qs = p.toString();
  history.replaceState(history.state, '', `${window.location.pathname}${qs ? `?${qs}` : ''}`);
}

export function App({ intent }: { intent: FragmentIntent }) {
  const { status } = useSession();

  if (status === 'loading') {
    return (
      <div className="auth">
        <div className="spinner" aria-label="Loading" />
      </div>
    );
  }

  if (status === 'signed-out') {
    const reasons: string[] = [];
    if (intent.recoveryToken) reasons.push('Sign in to review a CLI recovery request.');
    if (intent.platformEnrollmentId) reasons.push('Sign in to review a platform workload enrollment request.');
    if (intent.approveTxHash) reasons.push('Sign in to approve a suspended transaction.');
    return <AuthScreen enrollmentToken={intent.enrollmentToken} pendingReasons={reasons} />;
  }

  return (
    <StreamProvider enabled>
      <OperatorsProvider>
        <ApprovalsProvider>
          <InferenceProvider>
            <Shell intent={intent} />
          </InferenceProvider>
        </ApprovalsProvider>
      </OperatorsProvider>
    </StreamProvider>
  );
}

const STREAM_LABEL = { open: 'Live', connecting: 'Connecting', closed: 'Disconnected' } as const;

function Shell({ intent }: { intent: FragmentIntent }) {
  const [view, setViewState] = useState<View>(() => initialView(window.location.search, intent));
  const { total } = useApprovals();
  const { bound } = useOperators();
  const { state } = useStream();
  const { user, version } = useSession();

  const setView = (v: View) => {
    writeView(v);
    setViewState(v);
  };

  const nav: { id: View; label: string; count?: number }[] = [
    { id: 'cases', label: 'Cases' },
    { id: 'operators', label: 'Operators', count: bound.length || undefined },
    { id: 'inference', label: 'Inference' },
    { id: 'approvals', label: 'Approvals', count: total || undefined },
    { id: 'api', label: 'API' },
    { id: 'account', label: 'Account' },
  ];

  return (
    <div className="shell">
      <nav className="sidebar" aria-label="Primary">
        <div className="brand">
          <span className="brand-mark">g8</span>
          <span>
            g8e Console
            <small>{version ? `Gateway ${version}` : 'Gateway'}</small>
          </span>
        </div>
        {nav.map((n) => (
          <button
            key={n.id}
            type="button"
            className="nav-item"
            aria-current={view === n.id ? 'page' : undefined}
            onClick={() => setView(n.id)}
          >
            {n.label}
            {n.count !== undefined && (
              <span className="nav-count" style={n.id === 'operators' ? { background: 'var(--accent-soft)', color: 'var(--accent)' } : undefined}>
                {n.count}
              </span>
            )}
          </button>
        ))}
        <div className="sidebar-foot">
          <span className="row" title="Gateway event stream">
            <span className={`dot dot-${state}`} />
            {STREAM_LABEL[state]}
          </span>
          <span className="mono" title={user?.id}>
            {user?.id ? `${user.id.slice(0, 14)}…` : ''}
          </span>
        </div>
      </nav>
      <main className="main">
        {view === 'cases' && <CasesView onManageOperators={() => setView('operators')} onManageInference={() => setView('inference')} />}
        {view === 'operators' && <OperatorsView />}
        {view === 'inference' && <InferenceView />}
        {view === 'approvals' && (
          <ApprovalsView
            autoApproveTxHash={intent.approveTxHash}
            focusEnrollmentId={intent.platformEnrollmentId}
            recoveryToken={intent.recoveryToken}
          />
        )}
        {view === 'api' && <ApiView />}
        {view === 'account' && <AccountView />}
      </main>
    </div>
  );
}
