// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Per-role model selection. The ensemble stores one provider, model, endpoint,
// and API key per role and reads them on every chat request, so a saved change
// applies to the next message. Keys are write-only from the console. Every
// provider, g8e included, stores the model the user picks: the Inference Operator
// is a worker and never decides it.

import type { LlmProviderOption, LlmRole, LlmRoleView, LlmSettings } from './types';

export const ROLES: readonly LlmRole[] = ['primary', 'assistant', 'lite'];

export const ROLE_INFO: Record<LlmRole, { label: string; description: string }> = {
  primary: { label: 'Primary', description: 'Reasoning and tool use: the model that works the investigation.' },
  assistant: { label: 'Assistant', description: 'Supporting steps such as summaries and command review.' },
  lite: { label: 'Lite', description: 'Fast, small tasks: triage, titles, and memory.' },
};

export interface RoleForm {
  /** '' means "same as Primary" (assistant and lite only). */
  provider: string;
  model: string;
  endpoint: string;
  /** A newly typed key; empty keeps the stored one. */
  apiKey: string;
  clearKey: boolean;
  /** A key is stored for this provider on this role. */
  keyStored: boolean;
}

export type InferenceForm = Record<LlmRole, RoleForm>;

export interface RoleUpdateBody {
  provider: string | null;
  model: string | null;
  endpoint: string | null;
  api_key?: string;
}

export type InferenceUpdateBody = Record<LlmRole, RoleUpdateBody>;

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

/** Switching provider restores what is stored for that provider, or starts blank. */
export function withProvider(form: RoleForm, provider: string, stored: LlmRoleView): RoleForm {
  if (provider === form.provider) return form;
  if (provider && provider === stored.provider) return roleForm(stored);
  return { provider, model: '', endpoint: '', apiKey: '', clearKey: false, keyStored: false };
}

export function providerOption(settings: LlmSettings | null, provider: string): LlmProviderOption | undefined {
  return settings?.providers.find((p) => p.provider === provider);
}

export function updateBody(form: InferenceForm): InferenceUpdateBody {
  const role = (f: RoleForm): RoleUpdateBody => {
    if (!f.provider) return { provider: null, model: null, endpoint: null };
    const body: RoleUpdateBody = { provider: f.provider, model: f.model.trim() || null, endpoint: f.endpoint.trim() || null };
    if (f.clearKey) body.api_key = '';
    else if (f.apiKey.trim()) body.api_key = f.apiKey.trim();
    return body;
  };
  return { primary: role(form.primary), assistant: role(form.assistant), lite: role(form.lite) };
}

/** Problems that block saving, keyed by role. */
export function formErrors(form: InferenceForm): Partial<Record<LlmRole, string>> {
  const errors: Partial<Record<LlmRole, string>> = {};
  for (const role of ROLES) {
    const f = form[role];
    if (!f.provider) {
      if (role === 'primary') errors[role] = 'Choose a provider for the primary role.';
    } else if (!f.model.trim()) {
      errors[role] = 'Choose a model.';
    }
  }
  return errors;
}

/** A required key that is neither stored nor typed. The server may still hold one, so this only warns. */
export function missingKey(form: RoleForm, option: LlmProviderOption | undefined): boolean {
  if (!option || option.api_key !== 'required') return false;
  if (form.clearKey) return !form.apiKey.trim();
  return !form.keyStored && !form.apiKey.trim();
}

export function isDirty(form: InferenceForm, settings: LlmSettings): boolean {
  return ROLES.some((role) => {
    const a = form[role];
    const b = roleForm(settings[role]);
    return (
      a.provider !== b.provider ||
      a.model.trim() !== b.model ||
      a.endpoint.trim() !== b.endpoint ||
      a.apiKey.trim() !== '' ||
      a.clearKey
    );
  });
}

export interface EffectiveRole {
  provider: string | null;
  model: string | null;
  /** Taken from another role because this one is unset. */
  inherited: boolean;
}

/** The provider and model a role will actually use, following the ensemble's fallback chain. */
export function effectiveRole(settings: LlmSettings, role: LlmRole): EffectiveRole {
  const chain: LlmRole[] = role === 'lite' ? ['lite', 'assistant', 'primary'] : role === 'assistant' ? ['assistant', 'primary'] : ['primary'];
  for (const r of chain) {
    const v = settings[r];
    if (v.provider) return { provider: v.provider, model: v.model, inherited: r !== role };
  }
  return { provider: null, model: null, inherited: false };
}

/** A role's model for display. */
export function roleModelLabel(settings: LlmSettings, role: LlmRole): string {
  return effectiveRole(settings, role).model ?? '—';
}

export function isConfigured(settings: LlmSettings | null): boolean {
  const primary = settings?.primary;
  return Boolean(primary?.provider && primary.model);
}
