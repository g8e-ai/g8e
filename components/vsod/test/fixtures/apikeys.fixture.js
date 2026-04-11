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
 * API Key test fixtures
 */

import { ApiKeyStatus, ApiKeyClientName } from '@vsod/constants/auth.js';
import { now } from '@test/fixtures/base.fixture.js';

export const mockApiKeys = {
  validOperator: {
    key: 'g8e_valid_operator_key_123456789',
    data: {
      user_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      organization_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      operator_id: '01234567-89ab-4cde-f012-345678901234',
      client_name: 'Test Operator',
      status: ApiKeyStatus.ACTIVE,
      created_at: now(),
      permissions: ['operator:execute']
    }
  },

  validOperator2: {
    key: 'g8e_valid_operator2_key_987654321',
    data: {
      user_id: 'b2c3d4e5-f6g7-4890-b1c2-d3e4f5678901',
      organization_id: 'b2c3d4e5-f6g7-4890-b1c2-d3e4f5678901',
      operator_id: '12345678-90ab-4def-0123-456789012345',
      client_name: 'Test Operator 2',
      status: ApiKeyStatus.ACTIVE,
      created_at: now(),
      permissions: ['operator:execute']
    }
  },

  inactive: {
    key: 'g8e_inactive_key_000000000',
    data: {
      user_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      organization_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      operator_id: '67890123-45ef-4012-5678-901234567890',
      client_name: 'Inactive Operator',
      status: ApiKeyStatus.SUSPENDED,
      created_at: now(),
      permissions: ['operator:execute']
    }
  },

  expired: {
    key: 'g8e_expired_key_111111111',
    data: {
      user_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      organization_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      operator_id: '78901234-56f0-4123-6789-012345678901',
      client_name: 'Expired Operator',
      status: ApiKeyStatus.ACTIVE,
      created_at: now(),
      expires_at: new Date(Date.now() - 86400000).toISOString(), // 1 day ago
      permissions: ['operator:execute']
    }
  },

  noOperator: {
    key: 'g8e_no_operator_key_222222222',
    data: {
      user_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      organization_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      client_name: 'User API Key',
      status: ApiKeyStatus.ACTIVE,
      created_at: now(),
      permissions: ['api:read']
      // No operator_id
    }
  },

  userDownload: {
    key: 'g8e_user_download_key_333333333',
    data: {
      user_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      organization_id: 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789',
      client_name: ApiKeyClientName.USER,
      status: ApiKeyStatus.ACTIVE,
      created_at: now(),
      permissions: ['operator:download']
      // No operator_id - this is a user-level download key
    }
  },

  invalidFormat: {
    key: 'invalid_format_key',
    data: null
  }
};
