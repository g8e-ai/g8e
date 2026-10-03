// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { consumeFragment } from './lib/fragment';
import { SessionProvider } from './state/session';
import { ToastProvider } from './state/toast';
import './styles.css';

// Read deep-link intents once, before anything renders, and clear the
// fragment so one-time tokens do not survive in history or on refresh.
const intent = consumeFragment();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ToastProvider>
      <SessionProvider>
        <App intent={intent} />
      </SessionProvider>
    </ToastProvider>
  </StrictMode>,
);
