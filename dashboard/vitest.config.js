// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { defineConfig } from 'vitest/config';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      '@g8ed': __dirname,
      '@test': path.join(__dirname, 'test'),
    }
  },
    cacheDir: '.vitest-cache',
    test: {
      globals: true,
      environment: 'node',
  
      setupFiles: ['./test/setup.js'],
      // Run tests sequentially to avoid g8edB KV flushdb() conflicts
      pool: 'forks',
      forks: {
        singleFork: true
      },
      reporters: ['dot'],
      // Suppress stdout/stderr output during tests
      silent: true,
      coverage: {
        provider: 'v8',
        reporter: ['text', 'html', 'lcov'],
        reportsDirectory: 'coverage',
        include: ['public/js/**/*.js'],
        exclude: [
          'node_modules/',
          'test/',
          '**/*.config.js',
        ]
      },
    testTimeout: 10000,
    hookTimeout: 10000
  }
});
