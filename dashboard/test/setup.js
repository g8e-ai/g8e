// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Per-test-file setup for Vitest
 * Runs before EACH test file (not once globally)
 */

import { beforeEach, vi } from 'vitest';

beforeEach(() => {
    // Reset all mocks before each test
    vi.clearAllMocks();
});
