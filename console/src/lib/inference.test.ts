// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import {
  effectiveRole,
  formErrors,
  formFromSettings,
  isConfigured,
  isDirty,
  missingKey,
  roleModelLabel,
  updateBody,
  withProvider,
} from './inference';
import type { LlmProviderOption, LlmSettings } from './types';

const unset = { provider: null, model: null, endpoint: null, api_key_set: false };
const openai: LlmProviderOption = { provider: 'openai', label: 'OpenAI', endpoint: 'optional', api_key: 'required', lists_models: true };
const ollama: LlmProviderOption = { provider: 'ollama', label: 'Ollama', endpoint: 'optional', api_key: 'optional', lists_models: true };
const g8e: LlmProviderOption = { provider: 'g8e', label: 'g8e governed inference', endpoint: 'none', api_key: 'none', lists_models: true };

function settings(over: Partial<LlmSettings> = {}): LlmSettings {
  return {
    providers: [openai, ollama, g8e],
    primary: { provider: 'ollama', model: 'gemma4:e4b', endpoint: 'http://192.168.1.2:11434', api_key_set: false },
    assistant: unset,
    lite: unset,
    ...over,
  };
}

describe('inference form', () => {
  it('round-trips stored settings without marking the form dirty', () => {
    const s = settings();
    const form = formFromSettings(s);
    expect(isDirty(form, s)).toBe(false);
    expect(updateBody(form)).toEqual({
      primary: { provider: 'ollama', model: 'gemma4:e4b', endpoint: 'http://192.168.1.2:11434' },
      assistant: { provider: null, model: null, endpoint: null },
      lite: { provider: null, model: null, endpoint: null },
    });
  });

  it('sends a key only when typed, and an empty key to clear', () => {
    const form = formFromSettings(settings());
    expect(updateBody({ ...form, primary: { ...form.primary, apiKey: ' k ' } }).primary.api_key).toBe('k');
    expect(updateBody({ ...form, primary: { ...form.primary, clearKey: true } }).primary.api_key).toBe('');
    expect('api_key' in updateBody(form).primary).toBe(false);
  });

  it('sends a blank endpoint as null so the provider default applies', () => {
    const form = formFromSettings(settings());
    expect(updateBody({ ...form, primary: { ...form.primary, endpoint: '  ' } }).primary.endpoint).toBeNull();
  });

  it('starts blank when switching to another provider and restores the stored one', () => {
    const s = settings();
    const form = formFromSettings(s).primary;
    const switched = withProvider(form, 'openai', s.primary);
    expect(switched).toMatchObject({ provider: 'openai', model: '', endpoint: '', keyStored: false });
    expect(withProvider(switched, 'ollama', s.primary)).toMatchObject({ model: 'gemma4:e4b', endpoint: 'http://192.168.1.2:11434' });
  });

  it('requires a primary provider and a model for every set role', () => {
    const form = formFromSettings(settings({ primary: unset, lite: { provider: 'ollama', model: '', endpoint: null, api_key_set: false } }));
    expect(Object.keys(formErrors(form)).sort()).toEqual(['lite', 'primary']);
  });

  it('asks for a model on a g8e role like on any other provider', () => {
    const s = settings({ primary: { provider: 'g8e', model: '', endpoint: null, api_key_set: false } });
    expect(Object.keys(formErrors(formFromSettings(s)))).toEqual(['primary']);
    const chosen = settings({ primary: { provider: 'g8e', model: 'qwen3:4b', endpoint: null, api_key_set: false } });
    expect(formErrors(formFromSettings(chosen))).toEqual({});
    expect(updateBody(formFromSettings(chosen)).primary).toEqual({ provider: 'g8e', model: 'qwen3:4b', endpoint: null });
  });

  it('warns about a required key that is neither stored nor typed', () => {
    const form = { provider: 'openai', model: 'gpt', endpoint: '', apiKey: '', clearKey: false, keyStored: false };
    expect(missingKey(form, openai)).toBe(true);
    expect(missingKey({ ...form, keyStored: true }, openai)).toBe(false);
    expect(missingKey({ ...form, keyStored: true, clearKey: true }, openai)).toBe(true);
    expect(missingKey({ ...form, apiKey: 'sk' }, openai)).toBe(false);
  });
});

describe('effective role', () => {
  it('follows the ensemble fallback chain lite → assistant → primary', () => {
    const s = settings({ assistant: { provider: 'openai', model: 'gpt-small', endpoint: null, api_key_set: true } });
    expect(effectiveRole(s, 'lite')).toEqual({ provider: 'openai', model: 'gpt-small', inherited: true });
    expect(effectiveRole(s, 'assistant')).toEqual({ provider: 'openai', model: 'gpt-small', inherited: false });
    expect(effectiveRole(settings(), 'lite')).toEqual({ provider: 'ollama', model: 'gemma4:e4b', inherited: true });
  });

  it('is configured only when primary has a provider and a model', () => {
    expect(isConfigured(settings())).toBe(true);
    expect(isConfigured(settings({ primary: unset }))).toBe(false);
    expect(isConfigured(settings({ primary: { ...unset, provider: 'ollama' } }))).toBe(false);
    expect(isConfigured(null)).toBe(false);
  });

  it('labels a role with the model the user chose, g8e included', () => {
    const s = settings({ primary: { provider: 'g8e', model: 'qwen3:4b', endpoint: null, api_key_set: false } });
    expect(roleModelLabel(s, 'primary')).toBe('qwen3:4b');
    expect(roleModelLabel(s, 'lite')).toBe('qwen3:4b');
    expect(roleModelLabel(settings(), 'lite')).toBe('gemma4:e4b');
  });
});
