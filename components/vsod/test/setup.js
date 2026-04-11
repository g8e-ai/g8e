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

/**
 * Per-test-file setup for Vitest
 * Runs before EACH test file (not once globally)
 */

import { afterAll, beforeEach, afterEach, vi } from 'vitest';
import { logger } from '../utils/logger.js';

/**
 * NOTE: Do NOT use beforeAll/afterAll for collection-wide cleanup here.
 * 
 * In Vitest, setup files run per-worker and these hooks execute for EACH test file,
 * not once globally. This causes race conditions when test files run in parallel -
 * one file's cleanup wipes data that other parallel tests are still using.
 * 
 * Each test file must clean up its own data via TestCleanupHelper in afterEach.
 * For leftover data from crashed runs, manually run: npm run test:cleanup
 */

// Suppress logger output during tests
logger.level = 'error';
logger.transports.forEach(transport => {
  if (transport.silent !== undefined) {
    transport.silent = true;
  }
});

/**
 * Global cleanup after all tests complete.
 * 
 * NOTE: Do NOT call cleanAllTestData() here - it wipes entire collections and causes
 * race conditions when test files run in parallel. Individual tests clean up their
 * own data via TestCleanupHelper in their afterEach hooks.
 * 
 * Only cleanup shared service connections here.
 */
afterAll(async () => {
    try {
        const { cleanupTestServices } = await import('./helpers/test-services.js');
        await cleanupTestServices();
    } catch (error) {
        // Ignore cleanup errors - services may already be closed
    }
});

beforeEach(() => {
    // Reset all mocks before each test
    vi.clearAllMocks();
});

afterEach(() => {
    // Individual test cleanup is handled by TestCleanupHelper in each test file
});
