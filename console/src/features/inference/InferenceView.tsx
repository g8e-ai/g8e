// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Provider credentials live here. Provider connections configure endpoints and
// credentials used by the model roles in the left navigation menu.

import { useEffect, useState } from 'react';
import {
  isProviderDirty,
  providerForm,
  providerOption,
  providerUpdateBody,
  type ProviderForm,
} from '../../lib/inference';
import type { LlmProviderOption } from '../../lib/types';
import { useInference } from '../../state/inference';
import { errorText, useToast } from '../../state/toast';

export function InferenceView({ onViewApprovals }: { onViewApprovals?: () => void } = {}) {
  const { settings, loaded, error, ensembleStatus, reload, save } = useInference();
  const toast = useToast();
  const [providerForms, setProviderForms] = useState<Record<string, ProviderForm> | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!settings) return;
    setProviderForms(Object.fromEntries(settings.providers.map((option) => [option.provider, providerForm(option)])));
  }, [settings]);

  useEffect(() => {
    if (!settings) void reload();
  }, [settings, reload]);

  if (!loaded) {
    return (
      <div className="page">
        <div className="spinner" aria-label="Loading" />
      </div>
    );
  }

  if (!settings || !providerForms) {
    if (ensembleStatus !== 'ready') {
      return (
        <div className="page">
          <div className="page-head">
            <div>
              <h1>Inference</h1>
              <p>Configure provider connections for your model roles.</p>
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

