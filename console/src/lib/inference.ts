// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Provider connections and role selections are saved to the caller's g8ee user
// settings. Provider secrets are write-only; responses report only whether a key
// is present.

import type { LlmProviderOption, LlmRole, LlmRoleView, LlmSettings } from './types';

export const ROLES: readonly LlmRole[] = ['primary', 'assistant', 'lite'];

export const ROLE_INFO: Record<LlmRole, { label: string; description: string }> = {
  primary: { label: 'Primary', description: 'Reasoning and tool use: the model that works the investigation.' },
  assistant: { label: 'Assistant', description: 'Supporting steps such as summaries and command review.' },
  lite: { label: 'Lite', description: 'Fast, small tasks: triage, titles, and memory.' },
};

export interface RoleForm {
  provider: string;
  model: string;
}

export type InferenceForm = Record<LlmRole, RoleForm>;

export interface ProviderForm {
  endpoint: string;
  apiKey: string;
  clearKey: boolean;
  keyStored: boolean;
}

export interface RoleUpdateBody {
  provider: string | null;
  model: string | null;
}

export interface ProviderUpdateBody {
  provider: string;
  endpoint?: string | null;
  api_key?: string;
}

export interface InferenceUpdateBody {
  primary?: RoleUpdateBody;
  assistant?: RoleUpdateBody;
  lite?: RoleUpdateBody;
  providers?: ProviderUpdateBody[];
}

export function roleForm(view: LlmRoleView): RoleForm {
  return { provider: view.provider ?? '', model: view.model ?? '' };
}

export function formFromSettings(settings: LlmSettings): InferenceForm {
  return { primary: roleForm(settings.primary), assistant: roleForm(settings.assistant), lite: roleForm(settings.lite) };
}

export function providerForm(option: LlmProviderOption): ProviderForm {
  return {
    endpoint: option.configured_endpoint ?? '',
    apiKey: '',
    clearKey: false,
    keyStored: Boolean(option.api_key_set),
  };
}

export function withProvider(form: RoleForm, provider: string, stored: LlmRoleView): RoleForm {
  if (provider === form.provider) return form;
  if (provider && provider === stored.provider) return roleForm(stored);
  return { provider, model: '' };
}

export function providerOption(settings: LlmSettings | null, provider: string): LlmProviderOption | undefined {
  return settings?.providers.find((item) => item.provider === provider);
}

export function roleUpdateBody(form: InferenceForm, role: LlmRole): InferenceUpdateBody {
  const value = form[role];
  const update: RoleUpdateBody = {
    provider: value.provider || null,
    model: value.provider ? value.model.trim() || null : null,
  };
  if (role === 'primary') return { primary: update };
  if (role === 'assistant') return { assistant: update };
  return { lite: update };
}

export function providerUpdateBody(provider: string, form: ProviderForm, option: LlmProviderOption): InferenceUpdateBody {
  const update: ProviderUpdateBody = { provider };
  if (option.endpoint !== 'none' && form.endpoint.trim() !== (option.configured_endpoint ?? '')) {
    update.endpoint = form.endpoint.trim() || null;
  }
  if (form.clearKey) update.api_key = '';
  else if (form.apiKey.trim()) update.api_key = form.apiKey.trim();
  return { providers: [update] };
}

export function formErrors(form: InferenceForm): Partial<Record<LlmRole, string>> {
  const errors: Partial<Record<LlmRole, string>> = {};
  for (const role of ROLES) {
    const value = form[role];
    if (!value.provider) {
      if (role === 'primary') errors[role] = 'Choose a provider for the primary role.';
    } else if (!value.model.trim()) {
      errors[role] = 'Choose a model.';
    }
  }
  return errors;
}

export function isRoleDirty(form: RoleForm, settings: LlmSettings, role: LlmRole): boolean {
  const saved = roleForm(settings[role]);
  return form.provider !== saved.provider || form.model.trim() !== saved.model;
}

export function isProviderDirty(form: ProviderForm, option: LlmProviderOption): boolean {
  return (
    form.endpoint.trim() !== (option.configured_endpoint ?? '') ||
    form.apiKey.trim() !== '' ||
    form.clearKey
  );
}

export interface EffectiveRole {
  provider: string | null;
  model: string | null;
  inherited: boolean;
}

export function effectiveRole(settings: LlmSettings, role: LlmRole): EffectiveRole {
  const chain: LlmRole[] = role === 'lite' ? ['lite', 'assistant', 'primary'] : role === 'assistant' ? ['assistant', 'primary'] : ['primary'];
  for (const current of chain) {
    const value = settings[current];
    if (value.provider) return { provider: value.provider, model: value.model, inherited: current !== role };
  }
  return { provider: null, model: null, inherited: false };
}

export function roleModelLabel(settings: LlmSettings, role: LlmRole): string {
  return effectiveRole(settings, role).model ?? '—';
}

export function isConfigured(settings: LlmSettings | null): boolean {
  const primary = settings?.primary;
  return Boolean(primary?.provider && primary.model);
}
