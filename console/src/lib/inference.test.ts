// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import {
  effectiveRole,
  formErrors,
  formFromSettings,
  isConfigured,
  isDirty,
  liteBackendDiffers,
  missingKey,
  updateBody,
  withProvider,
} from './inference';
import type { LlmProviderOption, LlmSettings } from './types';

const unset = { provider: null, model: null, endpoint: null, api_key_set: false };
const openai: LlmProviderOption = { provider: 'openai', label: 'OpenAI', endpoint: 'optional', api_key: 'required', lists_models: true };

function settings(over: Partial<LlmSettings> = {}): LlmSettings {
  return {
    providers: [openai],
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

  it('warns about a required key that is neither stored nor typed', () => {
    const form = { provider: 'openai', model: 'gpt', endpoint: '', apiKey: '', clearKey: false, keyStored: false };
    expect(missingKey(form, openai)).toBe(true);
    expect(missingKey({ ...form, keyStored: true }, openai)).toBe(false);
    expect(missingKey({ ...form, keyStored: true, clearKey: true }, openai)).toBe(true);
    expect(missingKey({ ...form, apiKey: 'sk' }, openai)).toBe(false);
  });
});

describe('lite backend check', () => {
  it('flags a Lite backend that differs from the effective Assistant backend', () => {
    const form = formFromSettings(settings());
    expect(liteBackendDiffers(form)).toBe(false);
    const sameBackend = { ...form, lite: { ...form.primary, model: 'qwen3:0.6b' } };
    expect(liteBackendDiffers(sameBackend)).toBe(false);
    const otherBackend = { ...form, lite: { ...form.primary, endpoint: 'http://other:11434' } };
    expect(liteBackendDiffers(otherBackend)).toBe(true);
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
    expect(isConfigured(null)).toBe(false);
  });
});
