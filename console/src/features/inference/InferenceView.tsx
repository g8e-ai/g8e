// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Provider credentials live here. Each model role has a separate view and saves
// its own provider/model pair to the caller's user settings.

import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../../lib/api';
import {
  ROLE_INFO,
  effectiveRole,
  formErrors,
  formFromSettings,
  isProviderDirty,
  isRoleDirty,
  providerForm,
  providerOption,
  providerUpdateBody,
  roleUpdateBody,
  withProvider,
  type InferenceForm,
  type ProviderForm,
  type RoleForm,
} from '../../lib/inference';
import { Paths } from '../../lib/paths';
import type { LlmModelList, LlmProviderOption, LlmRole, LlmSettings } from '../../lib/types';
import { useInference } from '../../state/inference';
import { errorText, useToast } from '../../state/toast';

const MANUAL = '__manual__';

export function InferenceView({ role, onViewApprovals }: { role?: LlmRole; onViewApprovals?: () => void } = {}) {
  const { settings, loaded, error, ensembleStatus, reload, save } = useInference();
  const toast = useToast();
  const [roleForms, setRoleForms] = useState<InferenceForm | null>(null);
  const [providerForms, setProviderForms] = useState<Record<string, ProviderForm> | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!settings) return;
    setRoleForms(formFromSettings(settings));
    setProviderForms(Object.fromEntries(settings.providers.map((option) => [option.provider, providerForm(option)])));
  }, [settings]);

  useEffect(() => {
    if (!settings) void reload();
  }, [settings, reload]);

  const title = role ? `${ROLE_INFO[role].label} model` : 'Inference';

  if (!loaded) {
    return (
      <div className="page">
        <div className="spinner" aria-label="Loading" />
      </div>
    );
  }

  if (!settings || !roleForms || !providerForms) {
    if (ensembleStatus !== 'ready') {
      return (
        <div className="page">
          <div className="page-head">
            <div>
              <h1>{title}</h1>
              <p>{role ? ROLE_INFO[role].description : 'Configure provider connections for your model roles.'}</p>
            </div>
          </div>
          <div className="empty" style={{ margin: '48px auto', textAlign: 'center', maxWidth: 480 }}>
            <div className="spinner" style={{ width: 24, height: 24, marginBottom: 16 }} />
            <strong>{ensembleStatus === 'enrolling' ? 'Ensemble enrolling' : 'Connecting to ensemble…'}</strong>
            <span style={{ display: 'block', marginTop: 8, color: 'var(--muted)', fontSize: '13px' }}>
              {ensembleStatus === 'enrolling'
                ? 'The agentic ensemble (g8ee) is enrolling with the Gateway and waiting for approval.'
                : 'Waiting for g8ee to finish starting up. This view will update automatically once ready.'}
            </span>
            {ensembleStatus === 'enrolling' && onViewApprovals && (
              <button type="button" className="btn btn-sm btn-primary" style={{ marginTop: 16 }} onClick={onViewApprovals}>
                Review in Approvals
              </button>
            )}
          </div>
        </div>
      );
    }

    return (
      <div className="page">
        <div className="page-head">
          <div>
            <h1>{title}</h1>
          </div>
        </div>
        <div className="notice notice-error">
          Could not load model settings from the ensemble{error ? `: ${error}` : '.'}{' '}
          <button type="button" className="btn btn-sm" onClick={() => void reload()}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  const onSaveProvider = async (provider: string) => {
    const option = providerOption(settings, provider);
    const form = providerForms[provider];
    if (!option || !form) return;
    setSaving(true);
    try {
      await save(providerUpdateBody(provider, form, option));
      toast('success', 'Provider connection saved to your settings.');
    } catch (err) {
      toast('error', errorText(err));
    } finally {
      setSaving(false);
    }
  };

  const onSaveRole = async () => {
    if (!role) return;
    setSaving(true);
    try {
      await save(roleUpdateBody(roleForms, role));
      toast('success', 'Model selection saved. The next message uses it.');
    } catch (err) {
      toast('error', errorText(err));
    } finally {
      setSaving(false);
    }
  };

  if (!role) {
    return (
      <div className="page">
        <div className="page-head">
          <div>
            <h1>Inference</h1>
            <p>Set up provider connections once, then choose a provider and model for each role.</p>
          </div>
        </div>
        <div className="provider-list">
          {settings.providers.map((option) => (
            <ProviderCard
              key={option.provider}
              option={option}
              form={providerForms[option.provider] ?? providerForm(option)}
              disabled={saving}
              onSave={() => void onSaveProvider(option.provider)}
              onChange={(next) => setProviderForms((forms) => (forms ? { ...forms, [option.provider]: next } : forms))}
            />
          ))}
        </div>
      </div>
    );
  }

  const roleFormValue = roleForms[role];
  const errors = formErrors(roleForms);
  const dirty = isRoleDirty(roleFormValue, settings, role);
  const canSave = dirty && !errors[role] && !saving;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>{title}</h1>
          <p>{ROLE_INFO[role].description} Changes apply to the next chat message.</p>
        </div>
        <div className="row">
          <button type="button" className="btn" disabled={!dirty || saving} onClick={() => setRoleForms(formFromSettings(settings))}>
            Revert
          </button>
          <button type="button" className="btn btn-primary" disabled={!canSave} onClick={() => void onSaveRole()}>
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>

      <RoleCard
        role={role}
        form={roleFormValue}
        settings={settings}
        error={errors[role]}
        onChange={(next) => setRoleForms((forms) => (forms ? { ...forms, [role]: next } : forms))}
      />
    </div>
  );
}

function ProviderCard({
  option,
  form,
  disabled,
  onSave,
  onChange,
}: {
  option: LlmProviderOption;
  form: ProviderForm;
  disabled: boolean;
  onSave: () => void;
  onChange: (next: ProviderForm) => void;
}) {
  const id = `provider-${option.provider}`;
  const dirty = isProviderDirty(form, option);
  const endpointValue = option.endpoint === 'none' ? '' : form.endpoint;
  const needsKey = option.api_key === 'required' && (form.clearKey || (!form.keyStored && !form.apiKey.trim()));

  return (
    <section className="card provider-card" aria-labelledby={`${id}-title`}>
      <div className="card-head">
        <div>
          <h2 id={`${id}-title`}>{option.label}</h2>
          <div className="muted">
            {option.endpoint === 'none' && option.api_key === 'none'
              ? 'Uses the inference service connected to this Gateway.'
              : 'This connection is available to Primary, Assistant, and Lite.'}
          </div>
        </div>
        <button type="button" className="btn btn-primary" disabled={!dirty || disabled} onClick={onSave}>
          {disabled ? 'Saving…' : 'Save'}
        </button>
      </div>

      {(option.endpoint !== 'none' || option.api_key !== 'none') && (
        <div className="field-grid">
          {option.endpoint !== 'none' && (
            <div className="field">
              <label htmlFor={`${id}-endpoint`}>Endpoint</label>
              <input
                id={`${id}-endpoint`}
                className="input mono"
                value={endpointValue}
                placeholder={option.default_endpoint ?? 'https://…'}
                spellCheck={false}
                autoComplete="url"
                onChange={(event) => onChange({ ...form, endpoint: event.target.value })}
              />
              <span className="hint">Leave blank to use the provider default endpoint.</span>
            </div>
          )}

          {option.api_key !== 'none' && (
            <div className="field">
              <label htmlFor={`${id}-key`}>API key</label>
              <div className="row nowrap">
                <input
                  id={`${id}-key`}
                  className="input mono"
                  type="password"
                  value={form.apiKey}
                  autoComplete="new-password"
                  placeholder={form.clearKey ? 'Saved key will be removed' : form.keyStored ? 'Saved — leave blank to keep' : option.api_key === 'required' ? 'Required' : 'Optional'}
                  onChange={(event) => onChange({
                    ...form,
                    apiKey: event.target.value,
                    clearKey: event.target.value.trim() ? false : form.clearKey,
                  })}
                />
                {form.keyStored && (
                  <button
                    type="button"
                    className="btn btn-sm"
                    aria-pressed={form.clearKey}
                    onClick={() => onChange({ ...form, clearKey: !form.clearKey, apiKey: '' })}
                  >
                    {form.clearKey ? 'Keep saved key' : 'Remove key'}
                  </button>
                )}
              </div>
              {needsKey && <span className="hint hint-warn">Add an API key here before using this provider.</span>}
              {form.keyStored && !form.clearKey && !form.apiKey.trim() && (
                <span className="hint">A saved API key is available to all three roles.</span>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

interface RoleCardProps {
  role: LlmRole;
  form: RoleForm;
  settings: LlmSettings;
  error?: string;
  onChange: (next: RoleForm) => void;
}

function RoleCard({ role, form, settings, error, onChange }: RoleCardProps) {
  const info = ROLE_INFO[role];
  const id = `llm-${role}`;
  const inherited = effectiveRole(settings, role);
  const inheritedProvider = providerOption(settings, inherited.provider ?? '');

  return (
    <section className="card" aria-labelledby={`${id}-title`}>
      <div className="card-head">
        <div>
          <h2 id={`${id}-title`}>{info.label}</h2>
          <div className="muted">Select a provider and model for this role.</div>
        </div>
      </div>

      <div className="field-grid">
        <div className="field">
          <label htmlFor={`${id}-provider`}>Provider</label>
          <select
            id={`${id}-provider`}
            className="input"
            value={form.provider}
            onChange={(event) => onChange(withProvider(form, event.target.value, settings[role]))}
          >
            {role === 'primary' ? (
              <option value="" disabled>Choose a provider</option>
            ) : (
              <option value="">Use the fallback model</option>
            )}
            {settings.providers.map((provider) => (
              <option key={provider.provider} value={provider.provider}>{provider.label}</option>
            ))}
          </select>
        </div>

        {form.provider && (
          <ModelField
            role={role}
            provider={form.provider}
            model={form.model}
            lists={providerOption(settings, form.provider)?.lists_models ?? false}
            onChange={(model) => onChange({ ...form, model })}
          />
        )}
      </div>

      {!form.provider && role !== 'primary' && (
        <div className="muted role-fallback">
          {inherited.provider ? (
            <>Currently uses <span className="mono">{inherited.model}</span> via {inheritedProvider?.label ?? inherited.provider}.</>
          ) : (
            'Uses the primary model once one is chosen.'
          )}
        </div>
      )}
      {error && <div className="hint hint-error">{error}</div>}
    </section>
  );
}

function ModelField({
  role,
  provider,
  model,
  lists,
  onChange,
}: {
  role: LlmRole;
  provider: string;
  model: string;
  lists: boolean;
  onChange: (model: string) => void;
}) {
  const [models, setModels] = useState<string[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [manual, setManual] = useState(false);
  const latest = useRef(0);
  const fieldId = `llm-${role}-model`;

  const load = useCallback(async () => {
    const sequence = ++latest.current;
    setLoading(true);
    setListError(null);
    try {
      const response = await api.post<LlmModelList>(Paths.llmModels, { context: {}, provider });
      if (sequence !== latest.current) return;
      setModels(response.models ?? []);
    } catch (err) {
      if (sequence !== latest.current) return;
      setModels(null);
      setListError(errorText(err));
    } finally {
      if (sequence === latest.current) setLoading(false);
    }
  }, [provider, role]);

  useEffect(() => {
    setModels(null);
    setListError(null);
    setManual(false);
    if (lists) void load();
  }, [lists, load]);

  const listed = models && models.length > 0 && !manual;
  const options = listed && model && !models.includes(model) ? [model, ...models] : (models ?? []);

  return (
    <div className="field field-wide">
      <label htmlFor={fieldId}>Model</label>
      <div className="row nowrap">
        {listed ? (
          <select id={fieldId} className="input mono" value={model} onChange={(event) => {
            if (event.target.value === MANUAL) setManual(true);
            else onChange(event.target.value);
          }}>
            <option value="" disabled>Choose a model ({models.length} available)</option>
            {options.map((name) => <option key={name} value={name}>{name}</option>)}
            <option value={MANUAL}>Enter a model name…</option>
          </select>
        ) : (
          <input
            id={fieldId}
            className="input mono"
            value={model}
            placeholder={lists ? 'Model name' : 'Model name served by the inference node'}
            spellCheck={false}
            autoComplete="off"
            onChange={(event) => onChange(event.target.value)}
          />
        )}
        {lists && (
          <button type="button" className="btn btn-sm" disabled={loading} onClick={() => void load()}>
            {loading ? 'Loading…' : models ? 'Refresh' : 'Load models'}
          </button>
        )}
        {lists && manual && models && models.length > 0 && (
          <button type="button" className="btn btn-sm btn-ghost" onClick={() => setManual(false)}>Pick from list</button>
        )}
      </div>
      {listError && <span className="hint hint-warn">Could not list models: {listError}</span>}
      {models && models.length === 0 && !listError && <span className="hint">The endpoint reported no models.</span>}
    </div>
  );
}
