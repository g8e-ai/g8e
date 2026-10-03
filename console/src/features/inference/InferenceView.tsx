// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Provider, endpoint, credentials, and model per role. Saved selections apply
// to the next chat message; nothing is cached in the browser.

import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../../lib/api';
import {
  ROLES,
  ROLE_INFO,
  effectiveRole,
  formErrors,
  formFromSettings,
  isDirty,
  liteBackendDiffers,
  missingKey,
  providerOption,
  updateBody,
  withProvider,
  type InferenceForm,
  type RoleForm,
} from '../../lib/inference';
import { Paths } from '../../lib/paths';
import type { LlmRole, LlmSettings } from '../../lib/types';
import { useInference } from '../../state/inference';
import { errorText, useToast } from '../../state/toast';

const MANUAL = '__manual__';

export function InferenceView() {
  const { settings, loaded, error, reload, save } = useInference();
  const toast = useToast();
  const [form, setForm] = useState<InferenceForm | null>(null);
  const [saving, setSaving] = useState(false);

  // The ensemble's response is authoritative: reset the form on load and after save.
  useEffect(() => {
    if (settings) setForm(formFromSettings(settings));
  }, [settings]);

  if (!loaded) {
    return (
      <div className="page">
        <div className="spinner" aria-label="Loading" />
      </div>
    );
  }

  if (!settings || !form) {
    return (
      <div className="page">
        <div className="page-head">
          <div>
            <h1>Inference</h1>
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

  const errors = formErrors(form);
  const dirty = isDirty(form, settings);
  const canSave = dirty && Object.keys(errors).length === 0 && !saving;

  const onSave = async () => {
    setSaving(true);
    try {
      await save(updateBody(form));
      toast('success', 'Saved. The next message uses these models.');
    } catch (err) {
      toast('error', errorText(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1>Inference</h1>
          <p>Choose the provider and model for each role. Changes apply to the next chat message.</p>
        </div>
        <div className="row">
          <button type="button" className="btn" disabled={!dirty || saving} onClick={() => setForm(formFromSettings(settings))}>
            Revert
          </button>
          <button type="button" className="btn btn-primary" disabled={!canSave} onClick={() => void onSave()}>
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>

      {ROLES.map((role) => (
        <RoleCard
          key={role}
          role={role}
          form={form[role]}
          settings={settings}
          error={errors[role]}
          warning={
            role === 'lite' && liteBackendDiffers(form)
              ? 'Simple chat turns run the Assistant model through the Lite provider. Use the same backend for Lite and Assistant, or make sure this backend also serves the Assistant model.'
              : undefined
          }
          onChange={(next) => setForm((f) => (f ? { ...f, [role]: next } : f))}
        />
      ))}
    </div>
  );
}

interface RoleCardProps {
  role: LlmRole;
  form: RoleForm;
  settings: LlmSettings;
  error?: string;
  warning?: string;
  onChange: (next: RoleForm) => void;
}

function RoleCard({ role, form, settings, error, warning, onChange }: RoleCardProps) {
  const info = ROLE_INFO[role];
  const option = providerOption(settings, form.provider);
  const id = (field: string) => `llm-${role}-${field}`;
  const inherited = effectiveRole(settings, role);

  return (
    <section className="card" aria-labelledby={id('title')}>
      <div className="card-head">
        <div>
          <h2 id={id('title')}>{info.label}</h2>
          <div className="muted">{info.description}</div>
        </div>
      </div>

      <div className="field-grid">
        <div className="field">
          <label htmlFor={id('provider')}>Provider</label>
          <select
            id={id('provider')}
            className="input"
            value={form.provider}
            onChange={(e) => onChange(withProvider(form, e.target.value, settings[role]))}
          >
            {role === 'primary' ? (
              <option value="" disabled>
                Choose a provider
              </option>
            ) : (
              <option value="">Same as {role === 'lite' ? 'Assistant' : 'Primary'}</option>
            )}
            {settings.providers.map((p) => (
              <option key={p.provider} value={p.provider}>
                {p.label}
              </option>
            ))}
          </select>
        </div>

        {option && option.endpoint !== 'none' && (
          <div className="field">
            <label htmlFor={id('endpoint')}>Endpoint</label>
            <input
              id={id('endpoint')}
              className="input mono"
              value={form.endpoint}
              placeholder={option.default_endpoint ?? 'https://…'}
              spellCheck={false}
              autoComplete="off"
              onChange={(e) => onChange({ ...form, endpoint: e.target.value })}
            />
          </div>
        )}

        {option && option.api_key !== 'none' && (
          <div className="field">
            <label htmlFor={id('key')}>API key</label>
            <div className="row nowrap">
              <input
                id={id('key')}
                className="input mono"
                type="password"
                value={form.apiKey}
                autoComplete="new-password"
                placeholder={
                  form.keyStored && !form.clearKey
                    ? 'Saved — leave blank to keep'
                    : option.api_key === 'required'
                      ? 'Required'
                      : 'Optional'
                }
                onChange={(e) => onChange({ ...form, apiKey: e.target.value })}
              />
              {form.keyStored && (
                <button
                  type="button"
                  className="btn btn-sm"
                  aria-pressed={form.clearKey}
                  onClick={() => onChange({ ...form, clearKey: !form.clearKey, apiKey: '' })}
                >
                  {form.clearKey ? 'Keep saved key' : 'Clear'}
                </button>
              )}
            </div>
            {missingKey(form, option) && <span className="hint hint-warn">This provider needs an API key unless the server supplies one.</span>}
          </div>
        )}

        {option && <ModelField role={role} form={form} lists={option.lists_models} onChange={onChange} />}
      </div>

      {!form.provider && role !== 'primary' && (
        <div className="muted">
          {inherited.provider ? (
            <>
              Uses <span className="mono">{inherited.model}</span> via {providerOption(settings, inherited.provider)?.label ?? inherited.provider}.
            </>
          ) : (
            'Uses the primary model once one is chosen.'
          )}
        </div>
      )}
      {warning && <div className="hint hint-warn">{warning}</div>}
      {error && <div className="hint hint-error">{error}</div>}
    </section>
  );
}

interface ModelFieldProps {
  role: LlmRole;
  form: RoleForm;
  lists: boolean;
  onChange: (next: RoleForm) => void;
}

function ModelField({ role, form, lists, onChange }: ModelFieldProps) {
  const [models, setModels] = useState<string[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState<string | null>(null);
  const [manual, setManual] = useState(false);
  const fieldId = `llm-${role}-model`;
  const latest = useRef(0);
  const current = useRef(form);
  current.current = form;

  // Credentials typed in this form are used for the listing; blanks fall back to
  // what the ensemble has stored for the role.
  const load = useCallback(async () => {
    const seq = ++latest.current;
    setLoading(true);
    setListError(null);
    const f = current.current;
    try {
      const res = await api.post<{ models?: string[] }>(Paths.llmModels, {
        context: {},
        role,
        provider: f.provider,
        ...(f.endpoint.trim() ? { endpoint: f.endpoint.trim() } : {}),
        ...(f.apiKey.trim() ? { api_key: f.apiKey.trim() } : {}),
      });
      if (seq !== latest.current) return;
      setModels(res.models ?? []);
    } catch (err) {
      if (seq !== latest.current) return;
      setModels(null);
      setListError(errorText(err));
    } finally {
      if (seq === latest.current) setLoading(false);
    }
    // Re-created per provider so a provider switch reloads; endpoint and key
    // edits reload through the button and are read from the ref.
  }, [role, form.provider]);

  useEffect(() => {
    setModels(null);
    setListError(null);
    setManual(false);
    if (lists) void load();
  }, [lists, load]);

  const listed = models && models.length > 0 && !manual;
  const options = listed && form.model && !models.includes(form.model) ? [form.model, ...models] : (models ?? []);

  return (
    <div className="field field-wide">
      <label htmlFor={fieldId}>Model</label>
      <div className="row nowrap">
        {listed ? (
          <select
            id={fieldId}
            className="input mono"
            value={form.model}
            onChange={(e) => {
              if (e.target.value === MANUAL) setManual(true);
              else onChange({ ...form, model: e.target.value });
            }}
          >
            <option value="" disabled>
              Choose a model ({models.length} available)
            </option>
            {options.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
            <option value={MANUAL}>Enter a model name…</option>
          </select>
        ) : (
          <input
            id={fieldId}
            className="input mono"
            value={form.model}
            placeholder={lists ? 'Model name' : 'Model name served by the inference node'}
            spellCheck={false}
            autoComplete="off"
            onChange={(e) => onChange({ ...form, model: e.target.value })}
          />
        )}
        {lists && (
          <button type="button" className="btn btn-sm" disabled={loading} onClick={() => void load()}>
            {loading ? 'Loading…' : models ? 'Refresh' : 'Load models'}
          </button>
        )}
        {lists && manual && models && models.length > 0 && (
          <button type="button" className="btn btn-sm btn-ghost" onClick={() => setManual(false)}>
            Pick from list
          </button>
        )}
      </div>
      {listError && <span className="hint hint-warn">Could not list models: {listError}</span>}
      {models && models.length === 0 && !listError && <span className="hint">The endpoint reported no models.</span>}
    </div>
  );
}
