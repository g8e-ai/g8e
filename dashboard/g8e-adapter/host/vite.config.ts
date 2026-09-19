// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { defineConfig } from 'vite';

// Minimal vite config for the adapter host. The host exists purely to
// exercise the adapter against a real Gateway fixture in browser contract
// tests. It is not a production build target.
export default defineConfig({
  root: __dirname,
  server: {
    port: 5173,
    https: false,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
