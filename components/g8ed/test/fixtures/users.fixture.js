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
 * User test fixtures
 * 
 * NOTE: IDs are generated uniquely per test run to avoid test pollution.
 * Each test file import gets fresh unique IDs.
 */

import { v4 as uuidv4 } from 'uuid';
import { now } from '@test/fixtures/base.fixture.js';
import { UserDocument, PasskeyCredential } from '@g8ed/models/user_model.js';
import { UserRole, AuthProvider } from '@g8ed/constants/auth.js';

// Generate unique IDs per import to avoid test pollution between test files
const generateUniqueIds = () => {
  const timestamp = Date.now();
  const suffix = Math.random().toString(36).substring(2, 8);
  return {
    id: uuidv4(),
    operator_id: uuidv4(),
    organization_id: uuidv4(),
    api_key: `g8e_test_${timestamp}_${suffix}`
  };
};

const primaryIds = generateUniqueIds();
const secondaryIds = generateUniqueIds();
const basicIds = generateUniqueIds();
const expiredIds = generateUniqueIds();
const adminIds = generateUniqueIds();

export const mockUsers = {
  primary: {
    id: primaryIds.id,
    email: 'primary@example.com',
    name: 'Primary Test User',
    organization_id: primaryIds.organization_id,
    roles: [UserRole.USER],
    api_key: primaryIds.api_key,
    operator_id: primaryIds.operator_id,
    created_at: now(),
    updated_at: now()
  },

  secondary: {
    id: secondaryIds.id,
    email: 'secondary@example.com',
    name: 'Secondary Test User',
    organization_id: secondaryIds.organization_id,
    roles: [UserRole.USER],
    api_key: secondaryIds.api_key,
    operator_id: secondaryIds.operator_id,
    created_at: now(),
    updated_at: now()
  },

  basic: {
    id: basicIds.id,
    email: 'basic@example.com',
    name: 'Basic Test User',
    organization_id: basicIds.organization_id,
    roles: [UserRole.USER],
    created_at: now(),
    updated_at: now()
  },

  expired: {
    id: expiredIds.id,
    email: 'expired@example.com',
    name: 'Expired User',
    organization_id: expiredIds.organization_id,
    roles: [UserRole.USER],
    created_at: now(),
    updated_at: now()
  },

  admin: {
    id: adminIds.id,
    email: 'admin@example.com',
    name: 'Admin User',
    organization_id: adminIds.organization_id,
    roles: [UserRole.ADMIN],
    api_key: adminIds.api_key,
    operator_id: adminIds.operator_id,
    created_at: now(),
    updated_at: now()
  }
};

/**
 * Factory for typed PasskeyCredential instances.
 * @param {Object} overrides
 * @returns {PasskeyCredential}
 */
export function makePasskeyCredential(overrides = {}) {
    return PasskeyCredential.parse({
        id:           'cred-id-base64url',
        public_key:   'pubkey-base64url',
        counter:      0,
        transports:   ['internal'],
        created_at:   now(),
        last_used_at: null,
        ...overrides,
    });
}

/**
 * Factory for typed UserDocument instances.
 * Use in unit tests that need to seed KV via forKV() or assert on model properties.
 * @param {Object} overrides
 * @returns {UserDocument}
 */
export function makeUserDoc(overrides = {}) {
    const id = overrides.id || uuidv4();
    const ts = now();
    return UserDocument.parse({
        id,
        email: 'unit@example.com',
        name: 'Unit User',
        g8e_key: null,
        g8e_key_created_at: null,
        organization_id: id,
        roles: [UserRole.USER],
        operator_id: null,
        created_at: ts,
        updated_at: ts,
        last_login: ts,
        provider: AuthProvider.PASSKEY,
        sessions: [],
        dev_logs_enabled: true,
        ...overrides,
    });
}
