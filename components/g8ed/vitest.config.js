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
import os from 'os';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const isCi = process.env.CI === 'true';
const defaultWorkers = Math.max(1, Math.min(4, Math.floor(os.availableParallelism() / 2)));

export default defineConfig({
  resolve: {
    alias: {
      '@g8ed': __dirname,
      '@test': path.join(__dirname, 'test'),
      '@shared': path.join(__dirname, '../../shared'),
    }
  },
    cacheDir: path.join(__dirname, 'node_modules/.vitest-cache'),
    test: {
      globals: true,
      environment: 'node',
  
      setupFiles: ['./test/setup.js'],
      // Each test file is responsible for cleaning up its own data via
      // TestCleanupHelper, so files can safely run in parallel forks.
      // Within a single file, tests still run sequentially (default).
      pool: 'forks',
      maxWorkers: isCi ? 2 : defaultWorkers,
      minWorkers: isCi ? 1 : Math.min(2, defaultWorkers),
      fileParallelism: true,
      reporters: isCi ? ['default'] : ['dot'],
      // Suppress stdout/stderr output during tests
      silent: !isCi,
      coverage: {
        provider: 'v8',
        reporter: ['text', 'html', 'lcov'],
        reportsDirectory: path.join(__dirname, 'coverage'),
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
