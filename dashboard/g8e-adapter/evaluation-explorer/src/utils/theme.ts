// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export type Theme = 'dark' | 'light';

const DEFAULT_THEME: Theme = 'dark';
const COOKIE_NAME = 'theme';
const COOKIE_MAX_AGE = 31536000;
const THEME_MESSAGE_TYPE = 'g8e-theme-change';

type ThemeListener = (theme: Theme) => void;

const listeners = new Set<ThemeListener>();

export function isValidTheme(value: string | null | undefined): value is Theme {
  return value === 'dark' || value === 'light';
}

function readCookie(): Theme | null {
  const match = document.cookie.match(/(?:^|;\s*)theme=([^;]+)/);
  const value = match?.[1];
  return isValidTheme(value) ? value : null;
}

function writeCookie(theme: Theme): void {
  document.cookie = `${COOKIE_NAME}=${theme}; path=/; max-age=${COOKIE_MAX_AGE}; SameSite=Lax`;
}

function updateThemeColorMeta(theme: Theme): void {
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) {
    meta.setAttribute('content', theme === 'dark' ? '#0d1117' : '#f5f7fb');
  }
}

export function getDefaultTheme(): Theme {
  return DEFAULT_THEME;
}

export function getSystemTheme(): Theme {
  if (typeof window !== 'undefined' && window.matchMedia) {
    return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  }
  return DEFAULT_THEME;
}

function resolveTheme(): Theme {
  return readCookie() ?? getSystemTheme();
}

export function applyTheme(theme: Theme): void {
  document.documentElement.setAttribute('data-theme', theme);
  updateThemeColorMeta(theme);
}

export function getTheme(): Theme {
  const attr = document.documentElement.getAttribute('data-theme');
  if (isValidTheme(attr)) return attr;
  return resolveTheme();
}

export function setTheme(theme: Theme): void {
  if (!isValidTheme(theme)) return;
  applyTheme(theme);
  writeCookie(theme);
  listeners.forEach((listener) => listener(theme));
}

export function toggleTheme(): Theme {
  const next: Theme = getTheme() === 'dark' ? 'light' : 'dark';
  setTheme(next);
  return next;
}

export function onThemeChange(listener: ThemeListener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function handleThemeMessage(event: MessageEvent): void {
  const data = event.data;
  if (!data || data.type !== THEME_MESSAGE_TYPE || !isValidTheme(data.theme)) return;
  setTheme(data.theme);
}

function handleSystemThemeChange(): void {
  if (readCookie()) return;
  const theme = getSystemTheme();
  applyTheme(theme);
  listeners.forEach((listener) => listener(theme));
}

/** Wire postMessage sync (dashboard iframe host) after the inline boot script runs. */
export function initTheme(): () => void {
  const current = document.documentElement.getAttribute('data-theme');
  if (!isValidTheme(current)) {
    applyTheme(resolveTheme());
  } else {
    updateThemeColorMeta(current);
  }

  const media = window.matchMedia?.('(prefers-color-scheme: light)');
  media?.addEventListener('change', handleSystemThemeChange);

  window.addEventListener('message', handleThemeMessage);
  return () => {
    media?.removeEventListener('change', handleSystemThemeChange);
    window.removeEventListener('message', handleThemeMessage);
  };
}
