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
 * Service mock fixtures
 *
 * Factory functions that return fresh vi.fn() mocks backed by fixture data.
 * Use these in beforeEach so each test starts with a clean mock state —
 * no need to re-assert return values after vi.clearAllMocks().
 */

import { vi } from 'vitest';
import { mockOperators } from '@test/fixtures/operators.fixture.js';
import { mockUsers } from '@test/fixtures/users.fixture.js';

export function createOperatorServiceMock() {
    return {
        collectionName: 'operators',
        getOperator: vi.fn().mockResolvedValue(mockOperators.unclaimed.forDB()),
        claimOperatorSlot: vi.fn().mockResolvedValue(true),
        createOperatorSlot: vi.fn().mockResolvedValue('new_operator_slot_id'),
    };
}

export function createOperatorSessionServiceMock() {
    return {
        createOperatorSession: vi.fn().mockResolvedValue({
            id: 'test_session_123',
            expires_at: '2099-01-01T00:00:00.000Z'
        }),
        validateSession: vi.fn().mockResolvedValue({ is_active: true }),
        endSession: vi.fn().mockResolvedValue(true)
    };
}

export function createUserServiceMock() {
    return {
        getUser: vi.fn().mockResolvedValue(mockUsers.primary),
        updateUserOperator: vi.fn().mockResolvedValue(true)
    };
}

export function createSSEServiceMock() {
    return {
        publishEvent: vi.fn().mockResolvedValue(true)
    };
}

export function createPasskeyUserServiceMock(overrides = {}) {
    return {
        getUser:         vi.fn().mockResolvedValue(null),
        updateUser:      vi.fn().mockResolvedValue({}),
        updateLastLogin: vi.fn().mockResolvedValue({}),
        ...overrides,
    };
}
