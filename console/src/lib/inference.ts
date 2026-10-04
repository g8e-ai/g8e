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
  /** Legacy role overrides remain available to older settings responses. */
  endpoint: string;
  apiKey: string;
  clearKey: boolean;
  keyStored: boolean;
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
  endpoint?: string | null;
  api_key?: string;
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
  return {
    provider: view.provider ?? '',
    model: view.model ?? '',
    endpoint: view.endpoint ?? '',
    apiKey: '',
    clearKey: false,
    keyStored: view.provider !== null && view.api_key_set,
  };
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

/** Switching provider restores this role's saved model when returning to its saved provider. */
export function withProvider(form: RoleForm, provider: string, stored: LlmRoleView): RoleForm {
  if (provider === form.provider) return form;
  if (provider && provider === stored.provider) return roleForm(stored);
  return { provider, model: '', endpoint: '', apiKey: '', clearKey: false, keyStored: false };
}

export function providerOption(settings: LlmSettings | null, provider: string): LlmProviderOption | undefined {
  return settings?.providers.find((p) => p.provider === provider);
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

/** Compatibility serializer for consumers of the original all-role editor. */
export function updateBody(form: InferenceForm): Required<Pick<InferenceUpdateBody, 'primary' | 'assistant' | 'lite'>> {
  const role = (value: RoleForm): RoleUpdateBody => {
    const update: RoleUpdateBody = {
      provider: value.provider || null,
      model: value.provider ? value.model.trim() || null : null,
      endpoint: value.provider ? value.endpoint.trim() || null : null,
    };
    if (value.clearKey) update.api_key = '';
    else if (value.apiKey.trim()) update.api_key = value.apiKey.trim();
    return update;
  };
  return { primary: role(form.primary), assistant: role(form.assistant), lite: role(form.lite) };
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

/** Compatibility dirty check for the original all-role editor. */
export function isDirty(form: InferenceForm, settings: LlmSettings): boolean {
  return ROLES.some((role) => {
    const value = form[role];
    const saved = roleForm(settings[role]);
    return (
      value.provider !== saved.provider ||
      value.model.trim() !== saved.model ||
      value.endpoint.trim() !== saved.endpoint ||
      value.apiKey.trim() !== '' ||
      value.clearKey
    );
  });
}

/** Compatibility warning for credentials saved per role by earlier console versions. */
export function missingKey(
  form: Pick<RoleForm, 'provider' | 'apiKey' | 'clearKey' | 'keyStored'>,
  option: LlmProviderOption | undefined,
): boolean {
  if (!option || option.api_key !== 'required') return false;
  if (form.clearKey) return !form.apiKey.trim();
  return !form.keyStored && !form.apiKey.trim();
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

/** The provider and model a role will use, following g8ee's fallback chain. */
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
