// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { initTheme } from './utils/theme';
import './styles.css';

initTheme();

// Migrate legacy hash routes (#/evaluations) to path routes (/evaluations).
if (window.location.hash.startsWith('#/')) {
  const legacyPath = window.location.hash.slice(1);
  window.history.replaceState(null, '', `${legacyPath}${window.location.search}`);
}

const root = document.getElementById('root');
if (!root) throw new Error('root element missing');

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
