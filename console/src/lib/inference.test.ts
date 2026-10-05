// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import {
  effectiveRole,
  formErrors,
  formFromSettings,
  isConfigured,
  isProviderDirty,
  isRoleDirty,
  providerForm,
  providerUpdateBody,
  roleModelLabel,
  roleUpdateBody,
  withProvider,
} from './inference';
import type { LlmProviderOption, LlmRoleView, LlmSettings } from './types';

const unset: LlmRoleView = { provider: null, model: null };
const openai: LlmProviderOption = {
  provider: 'openai',
  label: 'OpenAI',
  endpoint: 'optional',
  api_key: 'required',
  configured_endpoint: 'https://proxy.example/v1',
  api_key_set: true,
  lists_models: true,
};
const ollama: LlmProviderOption = {
  provider: 'ollama',
  label: 'Ollama',
  endpoint: 'optional',
  api_key: 'optional',
  configured_endpoint: 'http://ollama:11434',
  api_key_set: false,
  lists_models: true,
};
const g8e: LlmProviderOption = {
  provider: 'g8e',
  label: 'g8e governed inference',
  endpoint: 'none',
  api_key: 'none',
  lists_models: true,
};

function settings(over: Partial<LlmSettings> = {}): LlmSettings {
  return {
    providers: [openai, ollama, g8e],
    primary: { provider: 'ollama', model: 'gemma4:e4b' },
    assistant: unset,
    lite: unset,
    ...over,
  };
}

describe('model role forms', () => {
  it('serializes only the selected role provider and model', () => {
    const s = settings();
    const form = formFromSettings(s);
    expect(isRoleDirty(form.primary, s, 'primary')).toBe(false);
    expect(roleUpdateBody(form, 'primary')).toEqual({
      primary: { provider: 'ollama', model: 'gemma4:e4b' },
    });
  });

  it('starts the model blank on a provider change and restores the saved selection', () => {
    const s = settings();
    const form = formFromSettings(s).primary;
    const switched = withProvider(form, 'openai', s.primary);
    expect(switched).toEqual({ provider: 'openai', model: '' });
    expect(withProvider(switched, 'ollama', s.primary)).toEqual({
      provider: 'ollama',
      model: 'gemma4:e4b',
    });
  });

  it('requires a primary provider and a model for every explicitly set role', () => {
    const form = formFromSettings(
      settings({ primary: unset, lite: { provider: 'ollama', model: null } }),
    );
    expect(Object.keys(formErrors(form)).sort()).toEqual(['lite', 'primary']);
  });

  it('allows optional roles to select the fallback', () => {
    const form = formFromSettings(settings({ assistant: { provider: 'openai', model: 'gpt' } }));
    form.assistant = { provider: '', model: '' };
    expect(roleUpdateBody(form, 'assistant')).toEqual({
      assistant: { provider: null, model: null },
    });
    expect(formErrors(form).assistant).toBeUndefined();
  });
});

describe('provider connection forms', () => {
  it('keeps a stored key write-only and omits it when unchanged', () => {
    const form = providerForm(openai);
    expect(form).toEqual({
      endpoint: 'https://proxy.example/v1',
      apiKey: '',
      clearKey: false,
      keyStored: true,
    });
    expect(providerUpdateBody('openai', form, openai)).toEqual({
      providers: [{ provider: 'openai' }],
    });
  });

  it('sends only changed connection fields and uses an empty key to remove it', () => {
    const form = providerForm(openai);
    expect(
      providerUpdateBody(
        'openai',
        { ...form, endpoint: ' https://other.example/v1 ', apiKey: ' sk-new ' },
        openai,
      ),
    ).toEqual({
      providers: [
        { provider: 'openai', endpoint: 'https://other.example/v1', api_key: 'sk-new' },
      ],
    });
    expect(providerUpdateBody('openai', { ...form, clearKey: true }, openai)).toEqual({
      providers: [{ provider: 'openai', api_key: '' }],
    });
  });

  it('tracks provider connection dirtiness independently from model roles', () => {
    const form = providerForm(ollama);
    expect(isProviderDirty(form, ollama)).toBe(false);
    expect(isProviderDirty({ ...form, endpoint: 'http://other:11434' }, ollama)).toBe(true);
    expect(isProviderDirty({ ...form, apiKey: 'secret' }, ollama)).toBe(true);
  });
});

describe('effective role', () => {
  it('follows the lite to assistant to primary fallback chain', () => {
    const s = settings({ assistant: { provider: 'openai', model: 'gpt-small' } });
    expect(effectiveRole(s, 'lite')).toEqual({
      provider: 'openai',
      model: 'gpt-small',
      inherited: true,
    });
    expect(effectiveRole(settings(), 'lite')).toEqual({
      provider: 'ollama',
      model: 'gemma4:e4b',
      inherited: true,
    });
  });

  it('is configured only when primary has both a provider and model', () => {
    expect(isConfigured(settings())).toBe(true);
    expect(isConfigured(settings({ primary: unset }))).toBe(false);
    expect(isConfigured(settings({ primary: { provider: 'ollama', model: null } }))).toBe(false);
    expect(isConfigured(null)).toBe(false);
  });

  it('labels inherited roles with the chosen model', () => {
    const s = settings({ primary: { provider: 'g8e', model: 'qwen3:4b' } });
    expect(roleModelLabel(s, 'primary')).toBe('qwen3:4b');
    expect(roleModelLabel(s, 'lite')).toBe('qwen3:4b');
  });
});
