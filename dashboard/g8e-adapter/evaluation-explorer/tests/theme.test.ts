// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  applyTheme,
  getDefaultTheme,
  getTheme,
  initTheme,
  isValidTheme,
  onThemeChange,
  setTheme,
  toggleTheme,
} from '../src/utils/theme';

describe('theme', () => {
  beforeEach(() => {
    document.documentElement.removeAttribute('data-theme');
    document.cookie = 'theme=; path=/; max-age=0';
    document.head.innerHTML = '<meta name="theme-color" content="#f5f7fb" />';
  });

  afterEach(() => {
    document.documentElement.removeAttribute('data-theme');
    document.cookie = 'theme=; path=/; max-age=0';
  });

  it('validates theme names', () => {
    expect(isValidTheme('dark')).toBe(true);
    expect(isValidTheme('light')).toBe(true);
    expect(isValidTheme('sepia')).toBe(false);
  });

  it('applies theme to the document root and theme-color meta', () => {
    applyTheme('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute('content')).toBe('#0d1117');

    applyTheme('light');
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute('content')).toBe('#f5f7fb');
  });

  it('persists theme in the shared dashboard cookie', () => {
    setTheme('light');
    expect(document.cookie).toContain('theme=light');
    expect(getTheme()).toBe('light');
  });

  it('toggles between dark and light', () => {
    setTheme('dark');
    expect(toggleTheme()).toBe('light');
    expect(getTheme()).toBe('light');
    expect(toggleTheme()).toBe('dark');
    expect(getTheme()).toBe('dark');
  });

  it('notifies listeners on setTheme', () => {
    const listener = vi.fn();
    const unsubscribe = onThemeChange(listener);
    setTheme('light');
    expect(listener).toHaveBeenCalledWith('light');
    unsubscribe();
    setTheme('dark');
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it('syncs theme from dashboard postMessage', () => {
    const cleanup = initTheme();
    window.dispatchEvent(
      new MessageEvent('message', { data: { type: 'g8e-theme-change', theme: 'light' } }),
    );
    expect(getTheme()).toBe('light');
    cleanup();
  });

  it('defaults to dark to match the g8ed dashboard', () => {
    expect(getDefaultTheme()).toBe('dark');
  });
});
