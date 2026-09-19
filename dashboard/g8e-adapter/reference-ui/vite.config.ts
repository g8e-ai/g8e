// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { defineConfig } from 'vite';

export default defineConfig({
  root: 'reference-ui',
  build: {
    outDir: 'reference-ui/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5174,
  },
});
