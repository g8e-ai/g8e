import { defineConfig } from 'vitest/config';

export default defineConfig({
  cacheDir: '.vitest-cache',
  test: {
    environment: 'node',
    include: ['test/**/*.test.ts'],
    reporters: ['dot'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'lcov'],
      reportsDirectory: 'coverage',
      include: ['src/**/*.ts'],
      exclude: ['test/', '**/*.config.ts', 'src/types/**'],
    },
    testTimeout: 10000,
    hookTimeout: 10000,
  },
});
