// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useEffect, useState } from 'react';
import { getDefaultTheme, getTheme, onThemeChange, toggleTheme, type Theme } from '../utils/theme';

function themeLabel(theme: Theme): string {
  return theme === getDefaultTheme() ? 'Switch to light mode' : 'Switch to dark mode';
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(() => getTheme());

  useEffect(() => onThemeChange(setTheme), []);

  return (
    <button
      type="button"
      className="theme-toggle"
      onClick={() => toggleTheme()}
      aria-label={themeLabel(theme)}
      title={themeLabel(theme)}
    >
      <span className="theme-toggle-icon" aria-hidden="true">
        {theme === 'dark' ? '☀' : '☾'}
      </span>
    </button>
  );
}
