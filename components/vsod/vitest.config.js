// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { defineConfig } from 'vitest/config';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  resolve: {
    alias: {
      '@vsod': __dirname,
      '@test': path.join(__dirname, 'test'),
      '@shared': path.join(__dirname, '../../shared'),
    }
  },
    cacheDir: '/home/g8e/.vitest-cache',
    test: {
      globals: true,
      environment: 'node',
  
      setupFiles: ['./test/setup.js'],
      // Run tests sequentially to avoid VSODB KV flushdb() conflicts
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
        reportsDirectory: '/home/g8e/coverage-reports/vsod',
      include: ['middleware/**/*.js', 'services/**/*.js', 'routes/**/*.js', 'models/**/*.js', 'utils/**/*.js', 'public/js/**/*.js'],
      exclude: [
        'node_modules/',
        'test/',
        '**/*.config.js',
        'public/css/',
        'public/fonts/',
        'public/media/',
        'views/'
      ]
    },
    testTimeout: 10000,
    hookTimeout: 10000
  }
});
