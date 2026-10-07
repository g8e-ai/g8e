// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useState } from 'react';
import { AccountView } from './features/account/AccountView';
import { ApiView } from './features/api/ApiView';
import { ApprovalsView } from './features/approvals/ApprovalsView';
import { AuthScreen } from './features/auth/AuthScreen';
import { CasesView } from './features/cases/CasesView';
import { InferenceView } from './features/inference/InferenceView';
import { ROLE_INFO, ROLES, effectiveRole, type RoleUpdateBody } from './lib/inference';
import type { LlmRole, LlmSettings } from './lib/types';
import { OperatorsView } from './features/operators/OperatorsView';
import type { FragmentIntent } from './lib/fragment';
import { ApprovalsProvider, useApprovals } from './state/approvals';
import { InferenceProvider, useInference } from './state/inference';
import { OperatorsProvider, useOperators } from './state/operators';
import { useSession } from './state/session';
import { StreamProvider, useStream } from './state/stream';
import { errorText, useToast } from './state/toast';

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
  p.delete('role');
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
  const [savingRole, setSavingRole] = useState<LlmRole | null>(null);
  const { total } = useApprovals();
  const { bound } = useOperators();
  const { state } = useStream();
  const { user, version } = useSession();
  const { settings, modelsByProvider, modelsLoading, save } = useInference();
  const toast = useToast();

  const setView = (v: View) => {
    writeView(v);
    setViewState(v);
  };

  const handleRoleSave = async (role: LlmRole, update: RoleUpdateBody) => {
    setSavingRole(role);
    try {
      await save({ [role]: update });
      toast('success', 'Model selection saved. The next message uses it.');
    } catch (err) {
      toast('error', errorText(err));
    } finally {
      setSavingRole(null);
    }
  };

  const afterInference: { id: View; label: string; count?: number }[] = [
    { id: 'approvals', label: 'Approvals', count: total || undefined },
    { id: 'api', label: 'API' },
    { id: 'account', label: 'Account' },
  ];

  const renderNavItem = (n: { id: View; label: string; count?: number }) => (
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
  );

  return (
    <div className="shell">
      <nav className="sidebar" aria-label="Sidebar">
        <div className="brand">
          <span className="brand-mark">g8e</span>
          <span>
            g8e Console
            <small>{version ? `Gateway ${version}` : 'Gateway'}</small>
          </span>
        </div>
        {renderNavItem({ id: 'cases', label: 'Cases' })}
        {renderNavItem({ id: 'operators', label: 'Operators', count: bound.length || undefined })}
        {renderNavItem({ id: 'inference', label: 'Inference' })}
        <div className="nav-section" aria-label="Model roles">
          <div className="nav-section-label">MODEL ROLES</div>
          {ROLES.map((role) => (
            <NavRoleSelect
              key={role}
              role={role}
              settings={settings}
              modelsByProvider={modelsByProvider}
              modelsLoading={modelsLoading}
              saving={savingRole === role}
              onSave={handleRoleSave}
            />
          ))}
        </div>
        {afterInference.map(renderNavItem)}
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
        {view === 'cases' && (
          <CasesView
            onManageOperators={() => setView('operators')}
            onManageInference={() => {
              const el = document.getElementById('role-select-primary');
              if (el) el.focus();
              else setView('inference');
            }}
            onViewApprovals={() => setView('approvals')}
          />
        )}
        {view === 'operators' && <OperatorsView />}
        {view === 'inference' && <InferenceView onViewApprovals={() => setView('approvals')} />}
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

function NavRoleSelect({
  role,
  settings,
  modelsByProvider,
  modelsLoading,
  saving,
  onSave,
}: {
  role: LlmRole;
  settings: LlmSettings | null;
  modelsByProvider: Record<string, string[]>;
  modelsLoading: boolean;
  saving: boolean;
  onSave: (role: LlmRole, update: RoleUpdateBody) => Promise<void>;
}) {
  const current = settings ? settings[role] : { provider: null, model: null };
  const inherited = settings ? effectiveRole(settings, role) : { provider: null, model: null, inherited: false };
  const providers = settings?.providers ?? [];
  const selectedValue = current.provider && current.model ? `${current.provider}::${current.model}` : '';

  const providerGroups = providers
    .map((p) => {
      const list = modelsByProvider[p.provider] ?? [];
      const currentForThis = current.provider === p.provider && current.model ? current.model : null;
      const models = currentForThis && !list.includes(currentForThis) ? [currentForThis, ...list] : list;
      return { provider: p, models };
    })
    .filter((g) => g.models.length > 0);

  const hasSelectedGroup = providerGroups.some((g) => g.provider.provider === current.provider);
  const orphanCurrent = current.provider && current.model && !hasSelectedGroup;
  const totalModels = providerGroups.reduce((acc, g) => acc + g.models.length, 0);

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const val = e.target.value;
    if (!val) {
      if (role === 'primary') return;
      void onSave(role, { provider: null, model: null });
      return;
    }
    const idx = val.indexOf('::');
    if (idx === -1) return;
    const provider = val.slice(0, idx);
    const model = val.slice(idx + 2);
    void onSave(role, { provider, model });
  };

  return (
    <div className="nav-role">
      <div className="nav-role-header">
        <label htmlFor={`role-select-${role}`} className="nav-role-label" title={ROLE_INFO[role].description}>
          {ROLE_INFO[role].label}
        </label>
        {saving && <span className="nav-role-saving">Saving…</span>}
      </div>
      <select
        id={`role-select-${role}`}
        className="nav-role-select"
        value={selectedValue}
        disabled={saving || !settings}
        onChange={handleChange}
      >
        {role === 'primary' ? (
          (!current.provider || !current.model) && (
            <option value="" disabled>
              {modelsLoading && totalModels === 0 ? 'Loading models…' : 'Choose a model'}
            </option>
          )
        ) : (
          <option value="">
            {inherited.model ? `Fallback (${inherited.model})` : 'Use fallback'}
          </option>
        )}
        {providerGroups.map(({ provider, models }) => (
          <optgroup key={provider.provider} label={provider.label}>
            {models.map((m) => (
              <option key={`${provider.provider}::${m}`} value={`${provider.provider}::${m}`}>
                {m}
              </option>
            ))}
          </optgroup>
        ))}
        {orphanCurrent && (
          <optgroup label={current.provider!}>
            <option value={selectedValue}>{current.model}</option>
          </optgroup>
        )}
      </select>
    </div>
  );
}
